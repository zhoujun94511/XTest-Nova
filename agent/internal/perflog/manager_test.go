package perflog

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zhoujun94511/xtest-nova/agent/internal/execution"
	novasystem "github.com/zhoujun94511/xtest-nova/agent/internal/system"
)

func TestWriteSampleKeepsWarmupAndMissingMetricsBlank(t *testing.T) {
	var output bytes.Buffer
	writer := csv.NewWriter(&output)
	sample := novasystem.Performance{
		CPU:          novasystem.CPUInfo{PID: 42, State: "warming_up"},
		Memory:       map[string]int{"total pss": 100},
		MetricStates: map[string]novasystem.MetricState{"cpu": {State: "warming_up"}, "jank": {State: "idle"}, "gpu": {State: "unsupported"}},
	}
	if err := writer.Write(performanceCSVHeader); err != nil {
		t.Fatal(err)
	}
	if err := writeSample(writer, time.Unix(1, 0), sample); err != nil {
		t.Fatal(err)
	}
	writer.Flush()
	rows, err := csv.NewReader(bytes.NewReader(output.Bytes())).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if rows[1][2] != "" || rows[1][3] != "" || rows[1][4] != "" || rows[1][6] != "" || rows[1][10] != "" || rows[1][11] != "" || rows[1][12] != "" {
		t.Fatalf("warmup or missing metric was serialized as a number: %#v", rows[1])
	}
	if !strings.Contains(rows[1][len(rows[1])-1], "warming_up") || !strings.Contains(rows[1][len(rows[1])-1], "unsupported") {
		t.Fatalf("metric states missing: %q", rows[1][len(rows[1])-1])
	}
}

func TestAccumulatorCountsCachedNetworkRateOnce(t *testing.T) {
	rxRate, txRate := 10.0, 5.0
	sampledAt := time.Unix(20, 0).UTC()
	sample := novasystem.Performance{
		Memory: map[string]int{},
		Network: novasystem.NetInfo{
			RxBytesPerSecond: &rxRate, TxBytesPerSecond: &txRate, RateSampledAt: sampledAt,
		},
		MetricStates: map[string]novasystem.MetricState{"network_rate": {State: "measured"}},
	}
	accumulator := newAccumulator()
	accumulator.Add(sample)
	accumulator.Add(sample)
	metrics := accumulator.Metrics()
	if metrics["network_rx_bytes_per_second"].Count != 1 || metrics["network_tx_bytes_per_second"].Count != 1 {
		t.Fatalf("cached rate counted more than once: %#v", metrics)
	}
	if accumulator.Quality()["network_rate"].Measured != 2 {
		t.Fatalf("row-level quality should still count both available rows: %#v", accumulator.Quality())
	}
}

func TestAccumulatorExcludesIdleFPSWithoutDroppingTrueIdleMetrics(t *testing.T) {
	idleFPS, zeroRate := 0.0, 0.0
	sampledAt := time.Unix(30, 0).UTC()
	accumulator := newAccumulator()
	accumulator.Add(novasystem.Performance{
		CPU:    novasystem.CPUInfo{Percent: 0, CorePercent: 0, SystemPercent: 0},
		Memory: map[string]int{}, FPS: &idleFPS,
		Network:      novasystem.NetInfo{RxBytesPerSecond: &zeroRate, TxBytesPerSecond: &zeroRate, RateSampledAt: sampledAt},
		MetricStates: map[string]novasystem.MetricState{"cpu": {State: "idle"}, "fps": {State: "idle"}, "network_rate": {State: "idle"}},
	})
	metrics := accumulator.Metrics()
	if _, exists := metrics["fps"]; exists {
		t.Fatalf("idle frame window must not be treated as a 0 FPS sample: %#v", metrics["fps"])
	}
	if metrics["app_cpu_device_percent"].Count != 1 || metrics["network_rx_bytes_per_second"].Count != 1 {
		t.Fatalf("true idle CPU/network values should remain statistical samples: %#v", metrics)
	}
}

func TestAccumulatorSeparatesPresentedFPSFromGFXRenderRate(t *testing.T) {
	presented, rendered := 120.0, 155.78
	accumulator := newAccumulator()
	accumulator.Add(novasystem.Performance{Memory: map[string]int{}, FPS: &presented, MetricStates: map[string]novasystem.MetricState{"fps": {State: "measured"}}})
	accumulator.Add(novasystem.Performance{Memory: map[string]int{}, RenderFPS: &rendered, MetricStates: map[string]novasystem.MetricState{"fps": {State: "unsupported"}, "render_rate": {State: "measured"}}})
	metrics := accumulator.Metrics()
	if metrics["fps"].Maximum != 120 || metrics["render_fps"].Maximum != 155.78 {
		t.Fatalf("presented and rendered frame rates were mixed: %#v", metrics)
	}
}

func TestMetricWarningsFlagSparseFrameWindows(t *testing.T) {
	warnings := metricWarnings(1000, map[string]MetricSummary{
		"fps":               {Count: 200},
		"jank_rate_percent": {Count: 9},
	})
	if len(warnings) != 1 || warnings[0].Metric != "jank_rate_percent" || warnings[0].ValidSamples != 9 {
		t.Fatalf("warnings=%#v", warnings)
	}
}

type fakeCollector struct{ sample novasystem.Performance }

func (f fakeCollector) Performance(context.Context, string) (novasystem.Performance, error) {
	return f.sample, nil
}

type blockingCollector struct {
	mu    sync.Mutex
	calls int
}

type transientCollector struct {
	mu    sync.Mutex
	calls int
}

func (f *transientCollector) Performance(context.Context, string) (novasystem.Performance, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.calls == 2 {
		return novasystem.Performance{}, errors.New("process restarting")
	}
	return novasystem.Performance{CPU: novasystem.CPUInfo{PID: 42}, Memory: map[string]int{"total pss": 100}}, nil
}

func (f *blockingCollector) Performance(ctx context.Context, _ string) (novasystem.Performance, error) {
	f.mu.Lock()
	f.calls++
	call := f.calls
	f.mu.Unlock()
	if call == 1 {
		return novasystem.Performance{CPU: novasystem.CPUInfo{PID: 42}, Memory: map[string]int{}}, nil
	}
	<-ctx.Done()
	return novasystem.Performance{}, ctx.Err()
}

func TestSessionUsesLegacyDirectoryAndCSVUnits(t *testing.T) {
	root := t.TempDir()
	gpu, current := 12.5, -420.0
	rxRate, txRate := 256.0, 128.0
	rateSampledAt := time.Unix(10, 0).UTC()
	manager := New(root, fakeCollector{novasystem.Performance{
		CPU:    novasystem.CPUInfo{PID: 42, Percent: 1.25, SystemPercent: 9.5},
		Memory: map[string]int{"total pss": 2048}, GPU: &novasystem.GPUInfo{Percent: gpu},
		Battery: novasystem.BatteryInfo{CurrentMA: &current, Status: "discharging", Level: 80, Temperature: 30.5},
		Network: novasystem.NetInfo{Rx: 1024, Tx: 512, RxBytesPerSecond: &rxRate, TxBytesPerSecond: &txRate, RateWindowMillis: 5000, RateSampledAt: rateSampledAt},
		MetricStates: map[string]novasystem.MetricState{
			"cpu": {State: "measured"}, "network_rate": {State: "measured"}, "gpu": {State: "measured"},
		},
	}})
	state, err := manager.Start(context.Background(), "com.example.app")
	if err != nil {
		t.Fatal(err)
	}
	if !state.Running || !strings.Contains(filepathSlash(state.Path), "/com.example.app/Perf/") || !strings.HasSuffix(state.Path, "/perf.csv") {
		t.Fatalf("state=%#v", state)
	}
	state, err = manager.Stop(context.Background())
	if err != nil || state.Running || state.Rows < 1 {
		t.Fatalf("state=%#v err=%v", state, err)
	}
	data, err := os.ReadFile(state.Path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "app_cpu_device_percent") || !strings.Contains(text, "network_rx_bytes_per_second") || !strings.Contains(text, "network_rate_sampled_at") || !strings.Contains(text, "metric_errors") || !strings.Contains(text, ",42,1.2500,1.2500,9.5000,2048,") || !strings.Contains(text, "30.5,1024,512,256.00,128.00,5000") {
		t.Fatalf("csv=%q", text)
	}
	for _, artifact := range []string{state.SessionPath, state.SummaryPath} {
		if content, readErr := os.ReadFile(artifact); readErr != nil || !strings.Contains(string(content), "schemaVersion") {
			t.Fatalf("artifact=%s content=%q err=%v", artifact, content, readErr)
		}
	}
	summary, err := os.ReadFile(state.SummaryPath)
	if err != nil || strings.Contains(string(summary), "memory_java heap_kb") || !strings.Contains(string(summary), "network_rx_bytes_per_second") || !strings.Contains(string(summary), "metricQuality") {
		t.Fatalf("summary=%q err=%v", summary, err)
	}
}

func TestSessionRejectsTraversalAndConcurrentStart(t *testing.T) {
	manager := New(t.TempDir(), fakeCollector{novasystem.Performance{Memory: map[string]int{}}})
	if _, err := manager.Start(context.Background(), "../bad"); err == nil {
		t.Fatal("unsafe package accepted")
	}
	if _, err := manager.Start(context.Background(), "com.example.app"); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Start(context.Background(), "com.example.other"); !errors.Is(err, ErrRunning) {
		t.Fatalf("concurrent start error=%v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, _ = manager.Stop(ctx)
}

func TestPerformanceStopRequiresCurrentOwner(t *testing.T) {
	coordinator := execution.NewCoordinator()
	manager := New(t.TempDir(), fakeCollector{novasystem.Performance{Memory: map[string]int{}}})
	manager.SetExecutionCoordinator(coordinator)
	state, err := manager.Start(context.Background(), "com.example.app")
	if err != nil {
		t.Fatal(err)
	}
	if state.Identity.OwnerToken == "" {
		t.Fatal("performance state did not expose owner credentials")
	}
	if _, err = manager.StopOwned(context.Background(), state.Identity.SessionID, "stale"); !errors.Is(err, execution.ErrOwnerMismatch) {
		t.Fatalf("stale owner stop error=%v", err)
	}
	if _, err = coordinator.Acquire("other", "other"); !errors.Is(err, execution.ErrOwned) {
		t.Fatalf("running performance session did not hold coordinator: %v", err)
	}
	if _, err = manager.StopOwned(context.Background(), state.Identity.SessionID, state.Identity.OwnerToken); err != nil {
		t.Fatal(err)
	}
	if _, err = coordinator.Acquire("other", "other"); err != nil {
		t.Fatalf("performance stop did not release coordinator: %v", err)
	}
}

func TestCooperativeStopDoesNotReportProbeCancellation(t *testing.T) {
	collector := &blockingCollector{}
	manager := New(t.TempDir(), collector)
	if _, err := manager.Start(context.Background(), "com.example.app"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1100 * time.Millisecond)
	state, err := manager.Stop(context.Background())
	if err != nil || state.Running || state.Error != "" {
		t.Fatalf("state=%#v err=%v", state, err)
	}
}

func TestTransientProbeFailureDoesNotBreakSession(t *testing.T) {
	manager := New(t.TempDir(), &transientCollector{})
	if _, err := manager.Start(context.Background(), "com.example.app"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2300 * time.Millisecond)
	state := manager.State()
	if !state.Running || state.Rows < 2 || state.FailedSamples != 1 || state.ConsecutiveFailures != 0 {
		t.Fatalf("state=%#v", state)
	}
	_, _ = manager.Stop(context.Background())
}

func TestPerformanceSessionsAreRetainedWithinLimit(t *testing.T) {
	manager := New(t.TempDir(), fakeCollector{})
	for index := 0; index <= 200; index++ {
		if _, err := manager.createDirectory("com.example.app", fmt.Sprintf("20260904_%06d", index)); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(filepath.Join(manager.root, "com.example.app", "Perf"))
	if err != nil || len(entries) != 200 {
		t.Fatalf("entries=%d err=%v", len(entries), err)
	}
}

func TestStartLaunchesLauncherWithoutForceStop(t *testing.T) {
	collector := &sessionLaunchCollector{}
	manager := New(t.TempDir(), collector)
	state, err := manager.Start(context.Background(), "com.example.app")
	if err != nil {
		t.Fatal(err)
	}
	if !state.Running {
		t.Fatalf("state=%#v", state)
	}
	_, _ = manager.Stop(context.Background())
	joined := strings.Join(collector.calls, "\n")
	if !strings.Contains(joined, "cmd package resolve-activity --brief com.example.app") {
		t.Fatalf("calls=%v", collector.calls)
	}
	if !strings.Contains(joined, "am start -W -n com.example.app/.MainActivity") {
		t.Fatalf("calls=%v", collector.calls)
	}
	if strings.Contains(joined, "force-stop") {
		t.Fatalf("continuous session must not force-stop: %v", collector.calls)
	}
}

type sessionLaunchCollector struct {
	startupCommands
}

func (s *sessionLaunchCollector) Performance(context.Context, string) (novasystem.Performance, error) {
	return novasystem.Performance{CPU: novasystem.CPUInfo{PID: 9}, Memory: map[string]int{"total pss": 1}}, nil
}

func filepathSlash(value string) string { return strings.ReplaceAll(value, "\\", "/") }
