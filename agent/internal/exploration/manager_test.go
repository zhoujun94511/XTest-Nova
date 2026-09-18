package exploration

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zhoujun94511/xtest-nova/agent/internal/execution"
)

type changingActivityDevice struct{ calls int }

func (d *changingActivityDevice) ForegroundActivity(context.Context) (string, error) {
	d.calls++
	if d.calls == 1 {
		return "com.example/.First", nil
	}
	return "com.example/.Second", nil
}

type alwaysChangingActivityDevice struct{ calls int }

func (d *alwaysChangingActivityDevice) ForegroundActivity(context.Context) (string, error) {
	d.calls++
	if d.calls%2 == 0 {
		return "com.example/.Second", nil
	}
	return "com.example/.First", nil
}
func (*alwaysChangingActivityDevice) ForegroundPackage(context.Context) (string, error) {
	return "com.example", nil
}
func (*alwaysChangingActivityDevice) Hierarchy(context.Context) (string, error) {
	return sampleHierarchy, nil
}
func (*alwaysChangingActivityDevice) Run(context.Context, string, ...string) (string, error) {
	return "", nil
}
func (*alwaysChangingActivityDevice) RunBytes(context.Context, string, ...string) ([]byte, error) {
	return nil, nil
}
func (*changingActivityDevice) ForegroundPackage(context.Context) (string, error) {
	return "com.example", nil
}
func (*changingActivityDevice) Hierarchy(context.Context) (string, error) {
	return sampleHierarchy, nil
}
func (*changingActivityDevice) Run(context.Context, string, ...string) (string, error) {
	return "", nil
}
func (*changingActivityDevice) RunBytes(context.Context, string, ...string) ([]byte, error) {
	return nil, nil
}

func TestCaptureRejectsActivityChangedDuringHierarchy(t *testing.T) {
	device := &changingActivityDevice{}
	manager := New(device, device, device)
	_, err := manager.Preview(context.Background(), "com.example", Rules{})
	if !errors.Is(err, errUnstableCapture) {
		t.Fatalf("error = %v, want unstable capture", err)
	}
}

func TestSessionStopsAfterConsecutiveUnstableCaptures(t *testing.T) {
	device := &alwaysChangingActivityDevice{}
	manager := New(device, device, device)
	if _, err := manager.Start(Config{RequestID: "unstable", Package: "com.example", MaxSteps: 10, IntervalMillis: 100, Execute: true}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		state := manager.State()
		if !state.Running {
			if state.StopReason != "unstable_capture_limit" || state.UnstableCaptures != maximumConsecutiveUnstableCaptures {
				t.Fatalf("state = %#v", state)
			}
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("unstable exploration did not converge")
}

type fakeDevice struct {
	mu         sync.Mutex
	foreground string
	hierarchy  string
	commands   [][]string
}

type fakeTextInjector struct {
	mu     sync.Mutex
	values []string
}

func (f *fakeTextInjector) InjectText(_ context.Context, value string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.values = append(f.values, value)
	return nil
}

func (f *fakeDevice) Hierarchy(context.Context) (string, error)         { return f.hierarchy, nil }
func (f *fakeDevice) ForegroundPackage(context.Context) (string, error) { return f.foreground, nil }
func (f *fakeDevice) ForegroundActivity(context.Context) (string, error) {
	return f.foreground + "/" + f.foreground + ".MainActivity", nil
}
func (f *fakeDevice) Run(_ context.Context, name string, args ...string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.commands = append(f.commands, append([]string{name}, args...))
	return "", nil
}
func (f *fakeDevice) RunBytes(context.Context, string, ...string) ([]byte, error) { return nil, nil }

type cancelCaptureDevice struct {
	started chan struct{}
	once    sync.Once
}

func (d *cancelCaptureDevice) Hierarchy(ctx context.Context) (string, error) {
	d.once.Do(func() { close(d.started) })
	<-ctx.Done()
	return "", errors.Join(errors.New("hierarchy collection failed"), ctx.Err())
}
func (*cancelCaptureDevice) ForegroundPackage(context.Context) (string, error) {
	return "com.example", nil
}
func (*cancelCaptureDevice) ForegroundActivity(context.Context) (string, error) {
	return "com.example/com.example.MainActivity", nil
}
func (*cancelCaptureDevice) Run(context.Context, string, ...string) (string, error) {
	return "", nil
}
func (*cancelCaptureDevice) RunBytes(context.Context, string, ...string) ([]byte, error) {
	return nil, nil
}

type stubbornCaptureDevice struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (d *stubbornCaptureDevice) Hierarchy(context.Context) (string, error) {
	d.once.Do(func() { close(d.started) })
	<-d.release
	return sampleHierarchy, nil
}
func (*stubbornCaptureDevice) ForegroundPackage(context.Context) (string, error) {
	return "com.example", nil
}
func (*stubbornCaptureDevice) ForegroundActivity(context.Context) (string, error) {
	return "com.example/com.example.MainActivity", nil
}
func (*stubbornCaptureDevice) Run(context.Context, string, ...string) (string, error) {
	return "", nil
}
func (*stubbornCaptureDevice) RunBytes(context.Context, string, ...string) ([]byte, error) {
	return nil, nil
}

func TestExplicitStopWinsOverCanceledCaptureError(t *testing.T) {
	device := &cancelCaptureDevice{started: make(chan struct{})}
	manager := New(device, device, device)
	if _, err := manager.Start(Config{RequestID: "cancel-capture", Package: "com.example", MaxSteps: 10, IntervalMillis: 100, Execute: true}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-device.started:
	case <-time.After(time.Second):
		t.Fatal("hierarchy capture did not start")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	state, err := manager.Stop(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if state.StopReason != "stopped" || state.Error != "" {
		t.Fatalf("state = %#v", state)
	}
}

func mustStartExploration(t *testing.T, manager *Manager, config Config) State {
	t.Helper()
	state, err := manager.Start(config)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func stopExplorationWithTimeout(manager *Manager, state State, timeout time.Duration) (State, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return manager.StopOwned(ctx, state.Identity.SessionID, state.Identity.OwnerToken)
}

func TestStopTimeoutKeepsExplorationOwnedAndPollable(t *testing.T) {
	device := &stubbornCaptureDevice{started: make(chan struct{}), release: make(chan struct{})}
	coordinator := execution.NewCoordinator()
	manager := New(device, device, device)
	manager.SetExecutionCoordinator(coordinator)
	started := mustStartExploration(t, manager, Config{RequestID: "stop-timeout", Package: "com.example", MaxSteps: 10, IntervalMillis: 100, Execute: true})
	<-device.started
	state, err := stopExplorationWithTimeout(manager, started, 20*time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) || !state.Running || !state.Stopping {
		t.Fatalf("state=%#v error=%v", state, err)
	}
	if _, err = coordinator.Acquire("other", "other"); !errors.Is(err, execution.ErrOwned) {
		t.Fatalf("timed-out stop released ownership: %v", err)
	}
	close(device.release)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	state, err = manager.StopOwned(ctx, started.Identity.SessionID, started.Identity.OwnerToken)
	if err != nil || state.Running || state.Stopping || state.Finalizing {
		t.Fatalf("final state=%#v error=%v", state, err)
	}
	if _, err = coordinator.Acquire("other", "other"); err != nil {
		t.Fatalf("completed finalization retained ownership: %v", err)
	}
}

func TestExplorationReleasesOnlyAfterPersistence(t *testing.T) {
	device := &fakeDevice{foreground: "com.example", hierarchy: sampleHierarchy}
	coordinator := execution.NewCoordinator()
	manager := New(device, device, device)
	manager.SetExecutionCoordinator(coordinator)
	persistStarted, persistRelease := make(chan struct{}), make(chan struct{})
	manager.persistArtifactsHook = func(Config) {
		close(persistStarted)
		<-persistRelease
	}
	if _, err := manager.Start(Config{RequestID: "persist-boundary", Package: "com.example", MaxSteps: 1, IntervalMillis: 100, Execute: true}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-persistStarted:
	case <-time.After(time.Second):
		t.Fatal("exploration did not enter persistence")
	}
	state := manager.State()
	if state.Running || !state.Finalizing {
		t.Fatalf("persistence state=%#v", state)
	}
	if _, err := coordinator.Acquire("other", "other"); !errors.Is(err, execution.ErrOwned) {
		t.Fatalf("persistence released ownership early: %v", err)
	}
	if _, err := manager.Start(Config{RequestID: "replacement", Package: "com.example", MaxSteps: 1, IntervalMillis: 100, Execute: true}); err == nil {
		t.Fatal("replacement exploration started during persistence")
	}
	close(persistRelease)
	deadline := time.Now().Add(time.Second)
	for manager.State().Finalizing && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if _, err := coordinator.Acquire("other", "other"); err != nil {
		t.Fatalf("persistence completion retained ownership: %v", err)
	}
}

func TestPreviewRequiresForegroundTarget(t *testing.T) {
	device := &fakeDevice{foreground: "com.other", hierarchy: sampleHierarchy}
	manager := New(device, device, device)
	if _, err := manager.Preview(context.Background(), "com.example", Rules{}); err == nil {
		t.Fatal("preview accepted a background target")
	}
}

func TestPreviewReportsCanonicalForegroundActivity(t *testing.T) {
	device := &fakeDevice{foreground: "com.example", hierarchy: sampleHierarchy}
	manager := New(device, device, device)
	analysis, err := manager.Preview(context.Background(), "com.example", Rules{})
	if err != nil || analysis.Activity != "com.example/com.example.MainActivity" {
		t.Fatalf("preview = %#v, %v", analysis, err)
	}
}

func TestSessionExecutesBoundedDeterministicTap(t *testing.T) {
	device := &fakeDevice{foreground: "com.example", hierarchy: sampleHierarchy}
	manager := New(device, device, device)
	state, err := manager.Start(Config{RequestID: "test", Package: "com.example", MaxSteps: 1, IntervalMillis: 100, Seed: 7, Execute: true})
	if err != nil || !state.Running {
		t.Fatalf("start = %#v, %v", state, err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for manager.State().Running && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	state = manager.State()
	if state.Running || state.Steps != 1 || state.StopReason != "max_steps" {
		t.Fatalf("state = %#v", state)
	}
	device.mu.Lock()
	defer device.mu.Unlock()
	if len(device.commands) != 1 || len(device.commands[0]) != 4 || device.commands[0][0] != "input" || device.commands[0][1] != "tap" {
		t.Fatalf("commands = %#v", device.commands)
	}
}

func TestSessionRequiresExplicitExecution(t *testing.T) {
	device := &fakeDevice{foreground: "com.example", hierarchy: sampleHierarchy}
	manager := New(device, device, device)
	if _, err := manager.Start(Config{Package: "com.example"}); err == nil {
		t.Fatal("session started without execute=true")
	}
}

func TestInputStrategyDefaultsAndBounds(t *testing.T) {
	config := Config{Package: "com.example", MaxSteps: 1, IntervalMillis: 100, Execute: true}
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}
	if config.InputStrategy.CasesPerField != defaultInputCasesPerField || config.InputStrategy.MaxLength != defaultInputMaxLength {
		t.Fatalf("input strategy defaults = %#v", config.InputStrategy)
	}
	config.InputStrategy.CasesPerField = maxInputCasesPerField + 1
	if err := config.Validate(); err == nil {
		t.Fatal("excessive input corpus was accepted")
	}
}

func TestActivityCoverageRequiresExpectedDenominator(t *testing.T) {
	device := &fakeDevice{foreground: "com.example", hierarchy: sampleHierarchy}
	manager := New(device, device, device)
	_, err := manager.Start(Config{RequestID: "coverage", Package: "com.example", MaxSteps: 1, IntervalMillis: 100, Execute: true, ExpectedActivities: []string{"com.example/com.example.MainActivity", "com.example/com.example.SettingsActivity"}})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for manager.State().Running && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	state := manager.State()
	if state.ActivityCovered != 1 || state.ActivityTotal != 2 || state.ActivityCoverage == nil || *state.ActivityCoverage != 50 {
		t.Fatalf("activity coverage = %#v", state)
	}
}

func TestCompletedRequestIsIdempotentAndNextRequestCanStart(t *testing.T) {
	device := &fakeDevice{foreground: "com.example", hierarchy: sampleHierarchy}
	manager := New(device, device, device)
	config := Config{RequestID: "first", Package: "com.example", MaxSteps: 1, IntervalMillis: 100, Execute: true}
	if _, err := manager.Start(config); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for manager.State().Running && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	first := manager.State()
	if repeated, err := manager.Start(config); err != nil || repeated.StartedAt == nil || !repeated.StartedAt.Equal(*first.StartedAt) {
		t.Fatalf("idempotent start = %#v, %v", repeated, err)
	}
	config.RequestID = "second"
	if state, err := manager.Start(config); err != nil || !state.Running || state.RequestID != "second" {
		t.Fatalf("second start = %#v, %v", state, err)
	}
	stopContext, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := manager.Stop(stopContext); err != nil {
		t.Fatal(err)
	}
}

func TestRequestIDRejectsDifferentExplorationConfiguration(t *testing.T) {
	device := &fakeDevice{foreground: "com.example", hierarchy: sampleHierarchy}
	manager := New(device, device, device)
	config := Config{RequestID: "same-key", Package: "com.example", MaxSteps: 1, IntervalMillis: 100, Execute: true}
	if _, err := manager.Start(config); err != nil {
		t.Fatal(err)
	}
	changed := config
	changed.MaxSteps = 2
	if _, err := manager.Start(changed); !errors.Is(err, ErrRequestConflict) {
		t.Fatalf("same requestId with changed configuration error = %v", err)
	}
	stopContext, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, _ = manager.Stop(stopContext)
}

type scriptedDevice struct {
	mu            sync.Mutex
	page          string
	foreground    string
	exitOnBack    bool
	externalOnTap bool
	commands      [][]string
}

type staleBeforeActionDevice struct {
	mu             sync.Mutex
	hierarchyCalls int
	commands       int
}

type staleThenStableDevice struct {
	mu             sync.Mutex
	hierarchyCalls int
	commands       int
}

func (d *staleThenStableDevice) Hierarchy(context.Context) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.hierarchyCalls++
	if d.hierarchyCalls == 1 {
		return pageA, nil
	}
	return pageAfterStale, nil
}
func (*staleThenStableDevice) ForegroundPackage(context.Context) (string, error) {
	return "com.example", nil
}
func (*staleThenStableDevice) ForegroundActivity(context.Context) (string, error) {
	return "com.example/com.example.MainActivity", nil
}
func (d *staleThenStableDevice) Run(context.Context, string, ...string) (string, error) {
	d.mu.Lock()
	d.commands++
	d.mu.Unlock()
	return "", nil
}
func (*staleThenStableDevice) RunBytes(context.Context, string, ...string) ([]byte, error) {
	return nil, nil
}

func (d *staleBeforeActionDevice) Hierarchy(context.Context) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.hierarchyCalls++
	if d.hierarchyCalls == 1 {
		return pageA, nil
	}
	return pageB, nil
}
func (*staleBeforeActionDevice) ForegroundPackage(context.Context) (string, error) {
	return "com.example", nil
}
func (*staleBeforeActionDevice) ForegroundActivity(context.Context) (string, error) {
	return "com.example/com.example.MainActivity", nil
}
func (d *staleBeforeActionDevice) Run(context.Context, string, ...string) (string, error) {
	d.mu.Lock()
	d.commands++
	d.mu.Unlock()
	return "", nil
}
func (*staleBeforeActionDevice) RunBytes(context.Context, string, ...string) ([]byte, error) {
	return nil, nil
}

func TestStaleObservationIsRejectedBeforeInput(t *testing.T) {
	device := &staleBeforeActionDevice{}
	manager := New(device, device, device)
	state, err := manager.Start(Config{RequestID: "stale-action", Package: "com.example", MaxSteps: 2, IntervalMillis: 100, Execute: true, FreshnessMode: freshnessStrict})
	if err != nil || state.Identity.SessionID == "" || state.Identity.OwnerToken == "" {
		t.Fatalf("start=%+v err=%v", state, err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for manager.State().StaleActions == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	state = manager.State()
	if state.StaleActions != 1 {
		t.Fatalf("state=%+v", state)
	}
	device.mu.Lock()
	commands := device.commands
	device.mu.Unlock()
	if commands != 0 {
		t.Fatalf("stale action executed %d commands", commands)
	}
	receipts := manager.Receipts()
	if len(receipts) != 1 || receipts[0].Status != execution.ReceiptRejected || receipts[0].ErrorCode != "stale_observation" {
		t.Fatalf("receipts=%+v", receipts)
	}
	if _, err = manager.StopOwned(context.Background(), state.Identity.SessionID, "wrong"); !errors.Is(err, execution.ErrOwnerMismatch) {
		t.Fatalf("stale stop error=%v", err)
	}
	_, _ = manager.StopOwned(context.Background(), state.Identity.SessionID, state.Identity.OwnerToken)
}

func TestStaleObservationDoesNotReuseRejectedStepID(t *testing.T) {
	device := &staleThenStableDevice{}
	manager := New(device, device, device)
	state, err := manager.Start(Config{RequestID: "stale-then-stable", Package: "com.example", MaxSteps: 1, IntervalMillis: 100, Execute: true, FreshnessMode: freshnessStrict})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for manager.State().Running && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	state = manager.State()
	if state.Running || state.StopReason != "max_steps" || state.Error != "" {
		t.Fatalf("state=%+v", state)
	}
	if state.StaleActions != 1 || state.Steps != 1 {
		t.Fatalf("state=%+v", state)
	}
	receipts := manager.Receipts()
	statuses := map[string]execution.ReceiptStatus{}
	for _, receipt := range receipts {
		statuses[receipt.StepID] = receipt.Status
	}
	if len(receipts) != 2 || statuses["stale-then-stable:step-000001"] != execution.ReceiptRejected || statuses["stale-then-stable:step-000002"] != execution.ReceiptExecuted {
		t.Fatalf("receipts=%+v", receipts)
	}
}

const pageA = `<hierarchy><node package="com.example" class="android.widget.Button" resource-id="com.example:id/open" text="打开" bounds="[0,0][200,100]" clickable="true" enabled="true" visible-to-user="true"/></hierarchy>`
const pageB = `<hierarchy><node package="com.example" class="android.widget.TextView" text="详情" bounds="[0,0][200,100]" clickable="false" enabled="true" visible-to-user="true"/></hierarchy>`
const pageAfterStale = `<hierarchy><node package="com.example" class="android.widget.Button" resource-id="com.example:id/continue" text="继续" bounds="[0,0][200,100]" clickable="true" enabled="true" visible-to-user="true"/></hierarchy>`

func (d *scriptedDevice) Hierarchy(context.Context) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.page == "B" {
		return pageB, nil
	}
	return pageA, nil
}
func (d *scriptedDevice) ForegroundPackage(context.Context) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.foreground != "" {
		return d.foreground, nil
	}
	return "com.example", nil
}
func (d *scriptedDevice) Run(_ context.Context, name string, args ...string) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.commands = append(d.commands, append([]string{name}, args...))
	if name == "input" && len(args) > 0 {
		switch args[0] {
		case "tap":
			d.page = "B"
			if d.externalOnTap {
				d.foreground = "com.browser"
			}
		case "keyevent":
			d.page = "A"
			if d.exitOnBack {
				d.foreground = "com.android.launcher"
			}
		}
	} else if name == "monkey" || (name == "sh" && len(args) > 0 && args[0] == "/system/bin/monkey") {
		d.foreground = "com.example"
		d.page = "A"
	}
	return "", nil
}

func TestExternalActionIsBlockedAndTargetIsRecovered(t *testing.T) {
	device := &scriptedDevice{externalOnTap: true}
	manager := New(device, device, device)
	_, err := manager.Start(Config{RequestID: "external-recovery", Package: "com.example", MaxSteps: 10, IntervalMillis: 100, Execute: true})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for manager.State().Running && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	state := manager.State()
	if state.Running || state.StopReason != "graph_exhausted" || state.Recoveries != 1 || state.BlockedEdges == 0 {
		t.Fatalf("state = %#v", state)
	}
	device.mu.Lock()
	defer device.mu.Unlock()
	joined := make([]string, 0, len(device.commands))
	for _, command := range device.commands {
		joined = append(joined, strings.Join(command, " "))
	}
	commands := strings.Join(joined, "\n")
	if !strings.Contains(commands, "input keyevent BACK") || !strings.Contains(commands, "sh /system/bin/monkey -p com.example -c android.intent.category.LAUNCHER 1") {
		t.Fatalf("commands = %s", commands)
	}
}
func (*scriptedDevice) RunBytes(context.Context, string, ...string) ([]byte, error) { return nil, nil }

func TestDFSBacktracksOnlyToKnownParent(t *testing.T) {
	device := &scriptedDevice{}
	manager := New(device, device, device)
	_, err := manager.Start(Config{RequestID: "dfs", Package: "com.example", MaxSteps: 10, IntervalMillis: 100, Execute: true, EnableBacktrack: true, MaxBacktracks: 2})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for manager.State().Running && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	state := manager.State()
	if state.Running || state.StopReason != "graph_exhausted" || state.Steps != 2 || state.Backtracks != 1 || state.DiscoveredStates != 2 || state.DiscoveredEdges != 2 {
		t.Fatalf("state = %#v", state)
	}
	device.mu.Lock()
	defer device.mu.Unlock()
	if len(device.commands) != 2 || device.commands[0][1] != "tap" || device.commands[1][1] != "keyevent" || device.commands[1][2] != "BACK" {
		t.Fatalf("commands = %#v", device.commands)
	}
}

func TestDFSBacktrackExitRelaunchesOnlyTheTarget(t *testing.T) {
	device := &scriptedDevice{exitOnBack: true}
	manager := New(device, device, device)
	_, err := manager.Start(Config{RequestID: "dfs-relaunch", Package: "com.example", MaxSteps: 10, IntervalMillis: 100, Execute: true, EnableBacktrack: true, MaxBacktracks: 2})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(8 * time.Second)
	for manager.State().Running && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	state := manager.State()
	if state.Running || state.StopReason != "graph_exhausted" || state.Backtracks != 1 || state.Recoveries != 1 {
		t.Fatalf("state = %#v", state)
	}
	device.mu.Lock()
	defer device.mu.Unlock()
	if len(device.commands) != 3 || device.commands[2][0] != "sh" || device.commands[2][1] != "/system/bin/monkey" {
		t.Fatalf("commands = %#v", device.commands)
	}
	command := device.commands[2]
	if len(command) != 7 || command[2] != "-p" || command[3] != "com.example" {
		t.Fatalf("relaunch command = %#v", command)
	}
}

func TestRecoveryIsPrioritizedAndScrollRequiresFlag(t *testing.T) {
	node := &graphNode{analysis: Analysis{Actions: []Action{
		{ID: "normal", Type: "tap"}, {ID: "scroll", Type: "swipe"}, {ID: "cancel", Type: "tap", Recovery: true},
	}}, tried: map[string]bool{}}
	action, ok := selectAction(node, 42, false, true)
	if !ok || action.ID != "cancel" {
		t.Fatalf("selected = %#v", action)
	}
	node.tried["cancel"], node.tried["normal"] = true, true
	if _, ok := selectAction(node, 42, false, true); ok {
		t.Fatal("scroll selected while enableScroll=false")
	}
	if action, ok := selectAction(node, 42, true, true); !ok || action.ID != "scroll" {
		t.Fatalf("scroll selection = %#v, %v", action, ok)
	}
}

func TestScrollGetsFairTurnAfterTwoNonScrollSelections(t *testing.T) {
	node := &graphNode{analysis: Analysis{Actions: []Action{
		{ID: "first", Type: "tap"}, {ID: "second", Type: "tap"}, {ID: "third", Type: "tap"}, {ID: "scroll", Type: "swipe"},
	}}, tried: map[string]bool{}, actionAttempts: map[string]int{}, nonScrollSelections: 2}
	action, ok := selectAction(node, 42, true, false)
	if !ok || action.ID != "scroll" {
		t.Fatalf("fair selection = %#v, %v", action, ok)
	}
}

func TestExecuteInputClearsAndInjectsUTF8(t *testing.T) {
	device := &fakeDevice{foreground: "com.example", hierarchy: sampleHierarchy}
	injector := &fakeTextInjector{}
	manager := New(device, device, device, injector)
	action := Action{Type: "input", X: 100, Y: 200, Text: "测试 😀"}
	if err := manager.executeAction(context.Background(), action); err != nil {
		t.Fatal(err)
	}
	device.mu.Lock()
	commands := append([][]string(nil), device.commands...)
	device.mu.Unlock()
	if len(commands) != 3 || commands[0][1] != "tap" || commands[1][1] != "keycombination" || commands[2][1] != "keyevent" {
		t.Fatalf("commands = %#v", commands)
	}
	injector.mu.Lock()
	defer injector.mu.Unlock()
	if len(injector.values) != 1 || injector.values[0] != "测试 😀" {
		t.Fatalf("injected = %#v", injector.values)
	}
}

func TestBoundedInputDoesNotPolluteNavigationCycleGuard(t *testing.T) {
	guard := &cycleGuard{}
	action := Action{Type: "input", InputKind: "cjk", Text: "测试"}
	for index := 0; index < 5; index++ {
		if period := observeExplorationCycle(guard, action, "same", "input", "same", "same"); period != 0 {
			t.Fatalf("input produced cycle period %d", period)
		}
	}
	if len(guard.history) != 0 {
		t.Fatalf("input polluted navigation history: %#v", guard.history)
	}
}

func TestScrollBacktrackUsesReverseSwipe(t *testing.T) {
	forward := Action{ID: "forward", Type: "swipe", Direction: "forward", X: 100, Y: 800, EndX: 100, EndY: 200}
	reverse := inverseAction(forward, "child", "parent")
	if reverse.Type != "swipe" || !reverse.Backtrack || reverse.Direction != "backward" || reverse.X != 100 || reverse.Y != 200 || reverse.EndX != 100 || reverse.EndY != 800 {
		t.Fatalf("reverse = %#v", reverse)
	}
}

type specialFlowDevice struct {
	mu         sync.Mutex
	foreground string
	hierarchy  string
	commands   [][]string
	afterTap   string
	afterPage  string
}

func (d *specialFlowDevice) Hierarchy(context.Context) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.hierarchy, nil
}
func (d *specialFlowDevice) ForegroundPackage(context.Context) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.foreground, nil
}
func (d *specialFlowDevice) ForegroundActivity(context.Context) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.foreground + "/.MainActivity", nil
}
func (d *specialFlowDevice) Run(_ context.Context, name string, args ...string) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.commands = append(d.commands, append([]string{name}, args...))
	if name == "input" && len(args) > 0 && args[0] == "tap" {
		d.foreground = d.afterTap
		d.hierarchy = d.afterPage
	}
	return "", nil
}
func (*specialFlowDevice) RunBytes(context.Context, string, ...string) ([]byte, error) {
	return nil, nil
}

func TestSessionPreprocessesConsentBeforeGraphExploration(t *testing.T) {
	device := &specialFlowDevice{foreground: "com.example", hierarchy: termsHierarchy, afterTap: "com.example", afterPage: pageB}
	manager := New(device, device, device)
	_, err := manager.Start(Config{RequestID: "consent", Package: "com.example", MaxSteps: 5, IntervalMillis: 100, Execute: true})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(8 * time.Second)
	for manager.State().Running && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	state := manager.State()
	if state.Running || state.StopReason != "graph_exhausted" || state.SpecialActions != 1 || state.LastSpecial == nil || state.LastSpecial.Kind != "consent" {
		t.Fatalf("state = %#v", state)
	}
}

type staleSpecialDevice struct {
	mu             sync.Mutex
	hierarchyCalls int
	commands       int
}

func (d *staleSpecialDevice) Hierarchy(context.Context) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.hierarchyCalls++
	if d.hierarchyCalls == 1 {
		return termsHierarchy, nil
	}
	return `<hierarchy/>`, nil
}
func (*staleSpecialDevice) ForegroundPackage(context.Context) (string, error) {
	return "com.example", nil
}
func (*staleSpecialDevice) ForegroundActivity(context.Context) (string, error) {
	return "com.example/.MainActivity", nil
}
func (d *staleSpecialDevice) Run(context.Context, string, ...string) (string, error) {
	d.mu.Lock()
	d.commands++
	d.mu.Unlock()
	return "", nil
}
func (*staleSpecialDevice) RunBytes(context.Context, string, ...string) ([]byte, error) {
	return nil, nil
}

func TestStaleSpecialSceneIsRejectedBeforeInput(t *testing.T) {
	device := &staleSpecialDevice{}
	manager := New(device, device, device)
	if _, err := manager.Start(Config{RequestID: "stale-special", Package: "com.example", MaxSteps: 2, IntervalMillis: 100, Execute: true, FreshnessMode: freshnessStrict}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(4 * time.Second)
	for manager.State().Running && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	state, receipts := manager.State(), manager.Receipts()
	device.mu.Lock()
	commands := device.commands
	device.mu.Unlock()
	if state.Running || state.StaleActions != 1 || state.SpecialActions != 0 || commands != 0 {
		t.Fatalf("state=%#v commands=%d", state, commands)
	}
	if len(receipts) != 1 || receipts[0].Status != execution.ReceiptRejected || receipts[0].ErrorCode != "stale_observation" {
		t.Fatalf("receipts=%#v", receipts)
	}
}

func TestSessionCrossesKnownPermissionController(t *testing.T) {
	permissionPage := `<hierarchy><node package="com.android.permissioncontroller" class="android.widget.Button" text="Allow" bounds="[10,20][210,120]" clickable="true" enabled="true" visible-to-user="true"/></hierarchy>`
	device := &specialFlowDevice{foreground: "com.android.permissioncontroller", hierarchy: permissionPage, afterTap: "com.example", afterPage: pageB}
	manager := New(device, device, device)
	_, err := manager.Start(Config{RequestID: "permission", Package: "com.example", MaxSteps: 5, IntervalMillis: 100, Execute: true})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(8 * time.Second)
	for manager.State().Running && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	state := manager.State()
	if state.Running || state.StopReason != "graph_exhausted" || state.SpecialActions != 1 || state.LastSpecial == nil || state.LastSpecial.Kind != "permission" {
		t.Fatalf("state = %#v", state)
	}
}

type ordinaryPermissionFlowDevice struct {
	mu    sync.Mutex
	stage int
}

func (d *ordinaryPermissionFlowDevice) Hierarchy(context.Context) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	switch d.stage {
	case 1:
		return `<hierarchy><node package="com.android.permissioncontroller" class="android.widget.Button" text="Allow" bounds="[10,20][210,120]" clickable="true" enabled="true" visible-to-user="true"/></hierarchy>`, nil
	case 2:
		return pageB, nil
	default:
		return pageA, nil
	}
}
func (d *ordinaryPermissionFlowDevice) ForegroundPackage(context.Context) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.stage == 1 {
		return "com.android.permissioncontroller", nil
	}
	return "com.example", nil
}
func (d *ordinaryPermissionFlowDevice) ForegroundActivity(ctx context.Context) (string, error) {
	pkg, _ := d.ForegroundPackage(ctx)
	return pkg + "/.MainActivity", nil
}
func (d *ordinaryPermissionFlowDevice) Run(_ context.Context, name string, args ...string) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if name == "input" && len(args) > 0 && args[0] == "tap" && d.stage < 2 {
		d.stage++
	}
	return "", nil
}
func (*ordinaryPermissionFlowDevice) RunBytes(context.Context, string, ...string) ([]byte, error) {
	return nil, nil
}

func TestOrdinaryStepDestinationSurvivesSpecialActionAppend(t *testing.T) {
	device := &ordinaryPermissionFlowDevice{}
	manager := New(device, device, device)
	if _, err := manager.Start(Config{RequestID: "ordinary-permission", Package: "com.example", MaxSteps: 5, IntervalMillis: 100, Execute: true}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(8 * time.Second)
	for manager.State().Running && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	steps, graph := manager.Steps(), manager.Graph()
	if len(steps) != 2 || steps[0].Destination == "" || len(graph.Edges) != 1 || graph.Edges[0].To != steps[0].Destination {
		t.Fatalf("steps=%#v graph=%#v", steps, graph)
	}
}

type repeatedExternalDevice struct {
	mu         sync.Mutex
	foreground string
}

const repeatedExternalPage = `<hierarchy><node package="com.example" class="android.widget.LinearLayout" bounds="[0,0][500,800]"><node package="com.example" class="android.widget.Button" resource-id="com.example:id/a" text="A" bounds="[0,0][100,100]" clickable="true" enabled="true" visible-to-user="true"/><node package="com.example" class="android.widget.Button" resource-id="com.example:id/b" text="B" bounds="[0,120][100,220]" clickable="true" enabled="true" visible-to-user="true"/><node package="com.example" class="android.widget.Button" resource-id="com.example:id/c" text="C" bounds="[0,240][100,340]" clickable="true" enabled="true" visible-to-user="true"/><node package="com.example" class="android.widget.Button" resource-id="com.example:id/d" text="D" bounds="[0,360][100,460]" clickable="true" enabled="true" visible-to-user="true"/></node></hierarchy>`

func (*repeatedExternalDevice) Hierarchy(context.Context) (string, error) {
	return repeatedExternalPage, nil
}
func (d *repeatedExternalDevice) ForegroundPackage(context.Context) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.foreground == "" {
		return "com.example", nil
	}
	return d.foreground, nil
}
func (d *repeatedExternalDevice) Run(_ context.Context, name string, args ...string) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if name == "input" && len(args) > 0 && args[0] == "tap" {
		d.foreground = "com.browser"
	}
	if name == "sh" && len(args) > 0 && args[0] == "/system/bin/monkey" {
		d.foreground = "com.example"
	}
	return "", nil
}
func (*repeatedExternalDevice) RunBytes(context.Context, string, ...string) ([]byte, error) {
	return nil, nil
}

func TestSuccessfulExternalRecoveryResetsConsecutiveBudget(t *testing.T) {
	device := &repeatedExternalDevice{}
	manager := New(device, device, device)
	if _, err := manager.Start(Config{RequestID: "repeated-external", Package: "com.example", MaxSteps: 4, IntervalMillis: 100, Execute: true}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(25 * time.Second)
	for manager.State().Running && manager.State().Recoveries < 4 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	state := manager.State()
	if state.Recoveries != 4 || state.StopReason == "safety_stop" {
		t.Fatalf("state = %#v", state)
	}
	stopContext, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := manager.Stop(stopContext); err != nil {
		t.Fatal(err)
	}
}

func TestSpecialHandlingStopsAfterBoundedAttempts(t *testing.T) {
	device := &fakeDevice{foreground: "com.example", hierarchy: termsHierarchy}
	manager := New(device, device, device)
	_, err := manager.Start(Config{RequestID: "bounded-special", Package: "com.example", MaxSteps: 10, IntervalMillis: 100, Execute: true, SpecialHandling: SpecialHandling{MaxAttempts: 2}})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for manager.State().Running && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	state := manager.State()
	if state.Running || state.StopReason != "special_handling_failed" || state.Steps != 2 || state.LastSpecial == nil || state.LastSpecial.Outcome != "attempt_limit" {
		t.Fatalf("state = %#v", state)
	}
}
