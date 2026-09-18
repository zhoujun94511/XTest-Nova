package diagnosticsoak

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/zhoujun94511/xtest-nova/agent/internal/artifacts"
)

const maxRecentSamples = 120

var ErrRunning = errors.New("diagnostic soak already running")

type Config struct {
	DurationSeconds int `json:"durationSeconds"`
	IntervalSeconds int `json:"intervalSeconds"`
}
type Sample struct {
	At     time.Time `json:"at"`
	Values any       `json:"values"`
}
type State struct {
	Running                bool       `json:"running"`
	StartedAt              *time.Time `json:"startedAt,omitempty"`
	EndedAt                *time.Time `json:"endedAt,omitempty"`
	DurationSeconds        int        `json:"durationSeconds,omitempty"`
	IntervalSeconds        int        `json:"intervalSeconds,omitempty"`
	Samples                int        `json:"samples"`
	Path                   string     `json:"path,omitempty"`
	Error                  string     `json:"error,omitempty"`
	Recent                 []Sample   `json:"recent"`
	RecoveredInterruptions int        `json:"recoveredInterruptions,omitempty"`
}

type Manager struct {
	operation sync.Mutex
	mu        sync.Mutex
	root      string
	sample    func() any
	state     State
	cancel    context.CancelFunc
	done      chan struct{}
}

func New(root string, sample func() any) *Manager {
	manager := &Manager{root: root, sample: sample, state: State{Recent: []Sample{}}}
	manager.state.RecoveredInterruptions = recoverInterrupted(root)
	return manager
}
func (m *Manager) State() State { m.mu.Lock(); defer m.mu.Unlock(); return cloneState(m.state) }

func (m *Manager) Start(config Config) (State, error) {
	m.operation.Lock()
	defer m.operation.Unlock()
	if config.DurationSeconds < 60 || config.DurationSeconds > 7*24*60*60 {
		return m.State(), errors.New("durationSeconds must be between 60 and 604800")
	}
	if config.IntervalSeconds < 5 || config.IntervalSeconds > 300 {
		return m.State(), errors.New("intervalSeconds must be between 5 and 300")
	}
	m.mu.Lock()
	if m.state.Running {
		state := cloneState(m.state)
		m.mu.Unlock()
		return state, ErrRunning
	}
	m.mu.Unlock()
	directory, err := artifacts.NewDeviceSession(m.root, "device.runtime", "Diagnostics")
	if err != nil {
		return m.State(), err
	}
	path := filepath.Join(directory, "soak.jsonl")
	marker := filepath.Join(directory, ".active")
	if err = os.WriteFile(marker, []byte(time.Now().UTC().Format(time.RFC3339Nano)), 0o644); err != nil {
		return m.State(), err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		_ = os.Remove(marker)
		return m.State(), err
	}
	fileOwned := true
	defer func() {
		if fileOwned {
			_ = file.Close()
		}
	}()
	ctx, cancel := context.WithCancel(context.Background())
	now := time.Now().UTC()
	done := make(chan struct{})
	m.mu.Lock()
	recovered := m.state.RecoveredInterruptions
	m.cancel, m.done = cancel, done
	m.state = State{Running: true, StartedAt: &now, DurationSeconds: config.DurationSeconds, IntervalSeconds: config.IntervalSeconds, Path: filepath.ToSlash(path), Recent: []Sample{}, RecoveredInterruptions: recovered}
	m.mu.Unlock()
	fileOwned = false
	go m.run(ctx, config, file, marker, done)
	return m.State(), nil
}

func (m *Manager) run(ctx context.Context, config Config, file *os.File, marker string, done chan struct{}) {
	defer close(done)
	defer func() {
		closeErr := file.Close()
		removeErr := os.Remove(marker)
		if closeErr == nil && (removeErr == nil || errors.Is(removeErr, os.ErrNotExist)) {
			return
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		cleanupErr := errors.Join(closeErr, removeErr)
		if m.state.Error == "" {
			m.state.Error = cleanupErr.Error()
		}
	}()
	encoder := json.NewEncoder(file)
	ticker := time.NewTicker(time.Duration(config.IntervalSeconds) * time.Second)
	defer ticker.Stop()
	timer := time.NewTimer(time.Duration(config.DurationSeconds) * time.Second)
	defer timer.Stop()
	record := func() bool {
		sample := Sample{At: time.Now().UTC(), Values: m.sample()}
		err := encoder.Encode(sample)
		if err == nil {
			err = file.Sync()
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		if err != nil {
			m.state.Error = err.Error()
			return false
		}
		m.state.Samples++
		m.state.Recent = append(m.state.Recent, sample)
		if len(m.state.Recent) > maxRecentSamples {
			m.state.Recent = append([]Sample(nil), m.state.Recent[len(m.state.Recent)-maxRecentSamples:]...)
		}
		return true
	}
	if !record() {
		m.finish()
		return
	}
	for {
		select {
		case <-ctx.Done():
			m.finish()
			return
		case <-timer.C:
			m.finish()
			return
		case <-ticker.C:
			if !record() {
				m.finish()
				return
			}
		}
	}
}

func (m *Manager) finish() {
	now := time.Now().UTC()
	m.mu.Lock()
	m.state.Running = false
	m.state.EndedAt = &now
	m.cancel = nil
	m.mu.Unlock()
}
func (m *Manager) Stop(ctx context.Context) (State, error) {
	m.operation.Lock()
	defer m.operation.Unlock()
	m.mu.Lock()
	cancel, done, running := m.cancel, m.done, m.state.Running
	m.mu.Unlock()
	if !running {
		return m.State(), nil
	}
	cancel()
	select {
	case <-done:
		return m.State(), nil
	case <-ctx.Done():
		return m.State(), ctx.Err()
	}
}
func cloneState(state State) State {
	state.Recent = append([]Sample(nil), state.Recent...)
	return state
}

func recoverInterrupted(root string) int {
	base := artifacts.PackageKind(root, "device.runtime", "Diagnostics")
	entries, err := os.ReadDir(base)
	if err != nil {
		return 0
	}
	recovered := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		active := filepath.Join(base, entry.Name(), ".active")
		interrupted := filepath.Join(base, entry.Name(), ".interrupted")
		if err = os.Rename(active, interrupted); err == nil {
			recovered++
		}
	}
	return recovered
}
