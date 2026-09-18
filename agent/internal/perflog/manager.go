package perflog

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/zhoujun94511/xtest-nova/agent/internal/artifacts"
	"github.com/zhoujun94511/xtest-nova/agent/internal/evidence"
	"github.com/zhoujun94511/xtest-nova/agent/internal/execution"
	novasystem "github.com/zhoujun94511/xtest-nova/agent/internal/system"
)

var packagePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*(?:\.[A-Za-z][A-Za-z0-9_]*)+$`)
var ErrRunning = errors.New("performance session already running")

type Collector interface {
	Performance(context.Context, string) (novasystem.Performance, error)
}

type SessionFactory interface {
	NewPerformanceSession() novasystem.PerformanceCollector
}

type Config struct {
	Package  string
	Interval time.Duration
	Duration time.Duration
}

type State struct {
	Running             bool                    `json:"running"`
	Identity            execution.Identity      `json:"identity"`
	Package             string                  `json:"package,omitempty"`
	StartedAt           *time.Time              `json:"startedAt,omitempty"`
	EndedAt             *time.Time              `json:"endedAt,omitempty"`
	Rows                int                     `json:"rows"`
	FailedSamples       int                     `json:"failedSamples"`
	PartialSamples      int                     `json:"partialSamples"`
	ConsecutiveFailures int                     `json:"consecutiveFailures"`
	IntervalSeconds     int                     `json:"intervalSeconds"`
	DurationSeconds     int                     `json:"durationSeconds,omitempty"`
	Path                string                  `json:"path,omitempty"`
	SessionPath         string                  `json:"sessionPath,omitempty"`
	SummaryPath         string                  `json:"summaryPath,omitempty"`
	EvidenceIndexPath   string                  `json:"evidenceIndexPath,omitempty"`
	Last                *novasystem.Performance `json:"last,omitempty"`
	Error               string                  `json:"error,omitempty"`
}

type Manager struct {
	root        string
	collector   Collector
	operation   sync.Mutex
	mu          sync.Mutex
	state       State
	cancel      context.CancelFunc
	done        chan struct{}
	commands    CommandExecutor
	generation  uint64
	coordinator *execution.Coordinator
}

func New(root string, collector Collector) *Manager {
	manager := &Manager{root: root, collector: collector}
	if commands, ok := collector.(CommandExecutor); ok {
		manager.commands = commands
	}
	return manager
}

func (m *Manager) SetExecutionCoordinator(value *execution.Coordinator) {
	m.operation.Lock()
	m.mu.Lock()
	m.coordinator = value
	m.mu.Unlock()
	m.operation.Unlock()
}

func (m *Manager) State() State {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state
}

func (m *Manager) Start(ctx context.Context, packageName string) (State, error) {
	return m.StartConfig(ctx, Config{Package: packageName, Interval: time.Second})
}

func (m *Manager) StartConfig(ctx context.Context, config Config) (State, error) {
	m.operation.Lock()
	defer m.operation.Unlock()
	if !packagePattern.MatchString(config.Package) {
		return m.State(), errors.New("invalid Android package name")
	}
	if config.Interval == 0 {
		config.Interval = time.Second
	}
	if config.Interval < time.Second || config.Interval > time.Minute || config.Duration < 0 || config.Duration > 7*24*time.Hour {
		return m.State(), errors.New("invalid performance sampling interval or duration")
	}
	m.mu.Lock()
	if m.state.Running {
		state := m.state
		m.mu.Unlock()
		return state, ErrRunning
	}
	m.mu.Unlock()
	if m.collector == nil {
		return m.State(), errors.New("performance collector unavailable")
	}
	collector := m.collector
	if factory, ok := m.collector.(SessionFactory); ok {
		collector = factory.NewPerformanceSession()
	}
	started := time.Now().UTC()
	m.generation++
	identity := execution.NewIdentity("performance", config.Package+":"+deviceTimestamp(), m.generation)
	releaseIdentity := false
	var err error
	if m.coordinator != nil {
		identity, err = m.coordinator.Acquire("performance", config.Package+":"+deviceTimestamp())
		if err != nil {
			return m.State(), err
		}
		releaseIdentity = true
		defer func() {
			if releaseIdentity {
				m.coordinator.Release(identity)
			}
		}()
	}
	sample, err := m.prepareSessionSample(ctx, collector, config.Package)
	if err != nil {
		return m.State(), err
	}
	normalizeSample(&sample)
	directory, err := m.createDirectory(config.Package, deviceTimestamp())
	if err != nil {
		return m.State(), err
	}
	path := filepath.Join(directory, "perf.csv")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return m.State(), err
	}
	writer := csv.NewWriter(file)
	if err = writer.Write(performanceCSVHeader); err == nil {
		err = writeSample(writer, sample.CollectedAt, sample)
	}
	writer.Flush()
	if err == nil {
		err = writer.Error()
	}
	if err != nil {
		_ = file.Close()
		return m.State(), err
	}
	sessionPath := artifactPath(directory, "session.json")
	summaryPath := artifactPath(directory, "summary.json")
	metadata := map[string]any{"schemaVersion": "xtest-nova-performance/v4", "identity": identity, "package": config.Package, "startedAt": started, "intervalSeconds": int(config.Interval.Seconds()), "durationSeconds": int(config.Duration.Seconds()), "metrics": []string{"cpu", "memory", "presented_fps", "render_rate", "jank", "gpu", "battery", "network_total", "network_rate"}}
	if err = writeJSONAtomic(sessionPath, metadata); err != nil {
		_ = file.Close()
		return m.State(), err
	}
	runContext, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	accumulator := newAccumulator()
	accumulator.Add(sample)
	m.mu.Lock()
	m.cancel, m.done = cancel, done
	m.state = State{Running: true, Identity: identity, Package: config.Package, StartedAt: &started, Rows: 1, IntervalSeconds: int(config.Interval.Seconds()), DurationSeconds: int(config.Duration.Seconds()), Path: filepath.ToSlash(path), SessionPath: sessionPath, SummaryPath: summaryPath, Last: &sample, PartialSamples: boolInt(len(sample.Errors) > 0)}
	m.mu.Unlock()
	releaseIdentity = false
	go m.run(runContext, file, writer, config, collector, accumulator, directory, done)
	return m.State(), nil
}

// XTest names result directories in the device's civil time. A statically linked
// Android Go process can have UTC time.Local even when the device UI is not UTC,
// so use Android's date command and strictly validate its fixed-format output.
func deviceTimestamp() string {
	return artifacts.DeviceTimestamp()
}

func (m *Manager) createDirectory(packageName, name string) (string, error) {
	base := filepath.Join(m.root, packageName, "Perf")
	if err := os.MkdirAll(base, 0755); err != nil {
		return "", err
	}
	for index := 0; index < 100; index++ {
		candidate := filepath.Join(base, name)
		if index > 0 {
			candidate = filepath.Join(base, fmt.Sprintf("%s_%02d", name, index))
		}
		if err := os.Mkdir(candidate, 0755); err == nil {
			if err = artifacts.PruneSessions(base, artifacts.MaxSessionsPerKind, candidate); err != nil {
				_ = os.RemoveAll(candidate)
				return "", err
			}
			return candidate, nil
		} else if !os.IsExist(err) {
			return "", err
		}
	}
	return "", errors.New("unable to allocate performance session directory")
}

func (m *Manager) run(ctx context.Context, file *os.File, writer *csv.Writer, config Config, collector Collector, accumulator *accumulator, directory string, done chan struct{}) {
	defer close(done)
	timer := time.NewTimer(config.Interval)
	defer timer.Stop()
	message := ""
	defer func() {
		writer.Flush()
		if err := writer.Error(); err != nil && message == "" {
			message = err.Error()
		}
		if err := file.Close(); err != nil && message == "" {
			message = err.Error()
		}
		m.mu.Lock()
		state := m.state
		m.mu.Unlock()
		ended := time.Now().UTC()
		metrics := accumulator.Metrics()
		summary := Summary{SchemaVersion: "xtest-nova-performance-summary/v3", Package: config.Package, StartedAt: *state.StartedAt, EndedAt: ended, Rows: state.Rows, FailedSamples: state.FailedSamples, PartialSamples: state.PartialSamples, Metrics: metrics, MetricQuality: accumulator.Quality(), Warnings: metricWarnings(state.Rows, metrics)}
		if err := writeJSONAtomic(filepath.Join(directory, "summary.json"), summary); err != nil && message == "" {
			message = err.Error()
		}
		indexPath := filepath.Join(directory, "evidence.json")
		index := evidence.Build(directory, config.Package, "", state.Identity, []string{state.Path, state.SessionPath, state.SummaryPath})
		if err := evidence.Write(indexPath, index); err != nil && message == "" {
			message = err.Error()
		} else if err == nil {
			m.mu.Lock()
			m.state.EvidenceIndexPath = filepath.ToSlash(indexPath)
			m.mu.Unlock()
		}
		m.finish(message)
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			if config.Duration > 0 && time.Since(*m.State().StartedAt) >= config.Duration {
				return
			}
			sampleContext, cancel := context.WithTimeout(ctx, 15*time.Second)
			sample, err := collector.Performance(sampleContext, config.Package)
			cancel()
			if err != nil && ctx.Err() != nil {
				return
			}
			if err != nil {
				m.mu.Lock()
				m.state.FailedSamples++
				m.state.ConsecutiveFailures++
				failures := m.state.ConsecutiveFailures
				m.state.Error = err.Error()
				m.mu.Unlock()
				if failures >= 5 {
					message = err.Error()
					return
				}
				timer.Reset(config.Interval)
				continue
			}
			normalizeSample(&sample)
			if err = writeSample(writer, sample.CollectedAt, sample); err != nil {
				message = err.Error()
				return
			}
			writer.Flush()
			if err = writer.Error(); err != nil {
				message = err.Error()
				return
			}
			accumulator.Add(sample)
			m.mu.Lock()
			m.state.Rows++
			m.state.ConsecutiveFailures = 0
			m.state.Error = ""
			if len(sample.Errors) > 0 {
				m.state.PartialSamples++
			}
			m.state.Last = &sample
			m.mu.Unlock()
			timer.Reset(config.Interval)
		}
	}
}

func (m *Manager) finish(message string) {
	ended := time.Now().UTC()
	m.mu.Lock()
	m.state.Running = false
	m.state.EndedAt = &ended
	m.state.Error = message
	m.cancel = nil
	identity, coordinator := m.state.Identity, m.coordinator
	m.mu.Unlock()
	if coordinator != nil {
		coordinator.Release(identity)
	}
}

func (m *Manager) Stop(ctx context.Context) (State, error) {
	m.operation.Lock()
	defer m.operation.Unlock()
	m.mu.Lock()
	cancel, done := m.cancel, m.done
	m.mu.Unlock()
	if cancel == nil {
		return m.State(), nil
	}
	cancel()
	select {
	case <-done:
		m.mu.Lock()
		m.done = nil
		m.mu.Unlock()
		return m.State(), nil
	case <-ctx.Done():
		return m.State(), ctx.Err()
	case <-time.After(3 * time.Second):
		return m.State(), errors.New("performance session stop timed out")
	}
}

func (m *Manager) StopOwned(ctx context.Context, sessionID, ownerToken string) (State, error) {
	m.mu.Lock()
	identity, coordinator, running := m.state.Identity, m.coordinator, m.state.Running
	m.mu.Unlock()
	if identity.SessionID != sessionID || identity.OwnerToken != ownerToken {
		return m.State(), execution.ErrOwnerMismatch
	}
	if coordinator != nil && running {
		if err := coordinator.Validate(sessionID, ownerToken); err != nil {
			return m.State(), err
		}
	}
	return m.Stop(ctx)
}

func writeSample(writer *csv.Writer, at time.Time, sample novasystem.Performance) error {
	memory := performanceMemory(sample)
	appCPU, coreCPU, systemCPU := "", "", ""
	if state := sample.MetricStates["cpu"].State; state == "" || state == "measured" || state == "idle" {
		appCPU = strconv.FormatFloat(sample.CPU.Percent, 'f', 4, 64)
		coreCPU = strconv.FormatFloat(sample.CPU.CorePercent, 'f', 4, 64)
		systemCPU = strconv.FormatFloat(sample.CPU.SystemPercent, 'f', 4, 64)
	}
	fps := ""
	if sample.FPS != nil {
		fps = strconv.FormatFloat(*sample.FPS, 'f', 2, 64)
	}
	renderFPS := ""
	if sample.RenderFPS != nil {
		renderFPS = strconv.FormatFloat(*sample.RenderFPS, 'f', 2, 64)
	}
	gpu := ""
	if sample.GPU != nil {
		gpu = strconv.FormatFloat(sample.GPU.Percent, 'f', 2, 64)
	}
	current := ""
	if sample.Battery.CurrentMA != nil {
		current = strconv.FormatFloat(*sample.Battery.CurrentMA, 'f', 2, 64)
	}
	errorsJSON, _ := json.Marshal(sample.Errors)
	jankCount, jankRate := "", ""
	if sample.Frame != nil && sample.MetricStates["jank"].State != "idle" {
		jankCount = strconv.FormatInt(sample.Frame.JankCount, 10)
		jankRate = strconv.FormatFloat(sample.Frame.JankRate, 'f', 2, 64)
	}
	statesJSON, _ := json.Marshal(sample.MetricStates)
	rxRate, txRate := "", ""
	if sample.Network.RxBytesPerSecond != nil {
		rxRate = strconv.FormatFloat(*sample.Network.RxBytesPerSecond, 'f', 2, 64)
	}
	if sample.Network.TxBytesPerSecond != nil {
		txRate = strconv.FormatFloat(*sample.Network.TxBytesPerSecond, 'f', 2, 64)
	}
	return writer.Write([]string{
		at.UTC().Format(time.RFC3339Nano), strconv.Itoa(sample.CPU.PID),
		appCPU, coreCPU, systemCPU,
		strconv.Itoa(memory), optionalMemory(sample.Memory, "java heap"), optionalMemory(sample.Memory, "native heap"), optionalMemory(sample.Memory, "graphics"), optionalMemory(sample.Memory, "private other"), fps, renderFPS, jankCount, jankRate, gpu, current, sample.Battery.Status, strconv.Itoa(sample.Battery.Level),
		strconv.FormatFloat(sample.Battery.Temperature, 'f', 1, 64), strconv.FormatInt(sample.Network.Rx, 10), strconv.FormatInt(sample.Network.Tx, 10), rxRate, txRate, strconv.FormatInt(sample.Network.RateWindowMillis, 10), optionalTime(sample.Network.RateSampledAt), sample.Network.Scope,
		sample.Sources["fps"], sample.Sources["renderRate"], sample.Sources["gpu"], sample.Sources["network"], strconv.FormatInt(sample.CollectionDurationMillis, 10), string(errorsJSON), string(statesJSON),
	})
}

func optionalMemory(memory map[string]int, key string) string {
	value, exists := memory[key]
	if !exists {
		return ""
	}
	return strconv.Itoa(value)
}

var performanceCSVHeader = []string{"timestamp", "pid", "app_cpu_device_percent", "app_cpu_core_percent", "system_cpu_percent", "memory_kb", "java_heap_kb", "native_heap_kb", "graphics_kb", "private_other_kb", "fps", "render_fps", "jank_count", "jank_rate_percent", "gpu_percent", "battery_current_ma", "battery_status", "battery_level_percent", "battery_temperature_c", "network_rx_bytes", "network_tx_bytes", "network_rx_bytes_per_second", "network_tx_bytes_per_second", "network_rate_window_ms", "network_rate_sampled_at", "network_scope", "fps_source", "render_fps_source", "gpu_source", "network_source", "collection_duration_ms", "metric_errors", "metric_states"}

func optionalTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func normalizeSample(sample *novasystem.Performance) {
	if sample.CollectedAt.IsZero() {
		sample.CollectedAt = time.Now().UTC()
	}
	if sample.CPU.CorePercent == 0 && sample.CPU.Percent != 0 {
		cores := sample.CPU.CoreCount
		if cores < 1 {
			cores = 1
		}
		sample.CPU.CorePercent = sample.CPU.Percent * float64(cores)
	}
}
