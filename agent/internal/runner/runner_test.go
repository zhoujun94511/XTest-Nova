package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zhoujun94511/xtest-nova/agent/internal/execution"
)

type fakeActivitySource struct {
	activities []string
	err        error
}

func (f fakeActivitySource) Activities(_ context.Context, _ string) ([]string, error) {
	return f.activities, f.err
}

type helperFactory struct{}

func (helperFactory) Command(_ context.Context, config RunConfig) *exec.Cmd {
	command := exec.Command(os.Args[0], "-test.run=TestRunnerHelperProcess")
	command.Env = append(os.Environ(), "XTEST_RUNNER_HELPER=1")
	if config.RequestID == "artifact-failure" {
		command.Env = append(command.Env, "XTEST_RUNNER_FAIL=1")
	}
	if config.RequestID == "live-state" {
		command.Env = append(command.Env, "XTEST_RUNNER_LIVE=1")
	}
	if config.RequestID == "forced-stop" {
		command.Env = append(command.Env, "XTEST_RUNNER_BLOCK=1")
	}
	return command
}

type pngScreenshot struct{}

func (pngScreenshot) Screenshot(context.Context) ([]byte, error) {
	return []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 1}, nil
}

type blockingFinalScreenshot struct {
	mu      sync.Mutex
	calls   int
	entered chan struct{}
	release chan struct{}
}

func (s *blockingFinalScreenshot) Screenshot(context.Context) ([]byte, error) {
	s.mu.Lock()
	s.calls++
	call := s.calls
	s.mu.Unlock()
	if call == 2 {
		close(s.entered)
		<-s.release
	}
	return []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 1}, nil
}

func TestProcessExitPublishesFinalizingWithoutBlockingStop(t *testing.T) {
	root := t.TempDir()
	screenshot := &blockingFinalScreenshot{entered: make(chan struct{}), release: make(chan struct{})}
	manager := NewWithArtifacts(helperFactory{}, filepath.Join(root, "stop"), filepath.Join(root, "legacy.log"), filepath.Join(root, "artifacts"), screenshot)
	if _, err := manager.Start(RunConfig{RequestID: "finalizing-state", Package: "com.example.app", DurationSeconds: 2, ThrottleMillis: 100}); err != nil {
		t.Fatal(err)
	}
	manager.mu.Lock()
	token := manager.controlToken
	manager.mu.Unlock()
	select {
	case <-screenshot.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("Runner did not reach artifact finalization")
	}
	state := manager.State()
	if state.Running || !state.Finalizing || state.PID != 0 {
		t.Fatalf("process exit state = %+v", state)
	}
	if _, ok := manager.AuthorizeControlToken(token); ok {
		t.Fatal("control token remained active during finalization")
	}
	started := time.Now()
	if _, err := manager.Stop(); err != nil || time.Since(started) > time.Second {
		t.Fatalf("Stop during finalization = %v after %v", err, time.Since(started))
	}
	if _, err := manager.Start(RunConfig{RequestID: "next-run", Package: "com.example.app", DurationSeconds: 2, ThrottleMillis: 100}); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("new run during finalization error = %v", err)
	}
	timeoutContext, cancelTimeout := context.WithTimeout(context.Background(), 25*time.Millisecond)
	if _, err := manager.WaitFinalized(timeoutContext); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("WaitFinalized while blocked error = %v", err)
	}
	cancelTimeout()
	close(screenshot.release)
	waitContext, cancelWait := context.WithTimeout(context.Background(), 3*time.Second)
	state, err := manager.WaitFinalized(waitContext)
	cancelWait()
	if err != nil {
		t.Fatalf("WaitFinalized after release: %v", err)
	}
	if state.Finalizing || state.StopReason != "completed" {
		t.Fatalf("final state = %+v", state)
	}
}

func TestRunnerHelperProcess(t *testing.T) {
	if os.Getenv("XTEST_RUNNER_HELPER") != "1" {
		return
	}
	fmt.Println(`{"state":"started","requestId":"artifact-1","package":"com.example.app","events":0}`)
	fmt.Println(`{"state":"activity","requestId":"artifact-1","package":"com.example.app","activity":"com.example.app/.MainActivity"}`)
	fmt.Println(`{"state":"scene","package":"com.example.app","activity":"com.example.app/.MainActivity","scene":"scene-a","time":1789034405481}`)
	fmt.Println(`{"state":"node_tap","package":"com.example.app","activity":"com.example.app/.MainActivity","scene":"scene-a","node":"button-a","time":1789034405662}`)
	fmt.Println(`{"state":"node_input","package":"com.example.app","activity":"com.example.app/.MainActivity","scene":"scene-a","inputKind":"cjk"}`)
	fmt.Println(`{"state":"node_scroll","package":"com.example.app","activity":"com.example.app/.MainActivity","scene":"scene-a"}`)
	fmt.Println(`{"state":"scroll_progress","package":"com.example.app","from":"scene-a","to":"scene-b"}`)
	fmt.Println(`{"state":"scene","package":"com.example.app","activity":"com.example.app/.DetailActivity","scene":"scene-b"}`)
	fmt.Println(`{"state":"scene_transition","package":"com.example.app","activity":"com.example.app/.DetailActivity","from":"scene-a","action":"tap|button","to":"scene-b"}`)
	fmt.Println(`{"state":"cycle_detected","package":"com.example.app","period":2,"repetitions":3}`)
	fmt.Println(`{"state":"edge_blocked","package":"com.example.app","scene":"scene-a","action":"tap|button","reason":"repeated_cycle"}`)
	if os.Getenv("XTEST_RUNNER_LIVE") == "1" {
		time.Sleep(2 * time.Second)
	}
	if os.Getenv("XTEST_RUNNER_BLOCK") == "1" {
		time.Sleep(30 * time.Second)
	}
	if os.Getenv("XTEST_RUNNER_FAIL") == "1" {
		fmt.Println(`{"state":"target_case_failed","requestId":"artifact-failure","package":"com.example.app","events":2,"error":"fixture failure"}`)
		os.Exit(1)
	}
	fmt.Println(`{"state":"completed","requestId":"artifact-1","package":"com.example.app","events":3}`)
	os.Exit(0)
}

func TestStateRefreshesMetricsWhileRunnerIsActive(t *testing.T) {
	root := t.TempDir()
	manager := NewWithArtifacts(helperFactory{}, filepath.Join(root, "stop"), filepath.Join(root, "legacy.log"), filepath.Join(root, "artifacts"), nil)
	state, err := manager.Start(RunConfig{RequestID: "live-state", Package: "com.example.app", DurationSeconds: 5, ThrottleMillis: 100})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for state.Events == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
		state = manager.State()
	}
	if !state.Running || state.Events == 0 || state.Exploration.Taps != 1 || state.CurrentActivity == "" || state.CurrentScene == "" || state.LastAction == "" || state.LastEventAt == nil {
		t.Fatalf("live state was not refreshed: %+v", state)
	}
	if _, err := manager.Stop(); err != nil {
		t.Fatal(err)
	}
	waitContext, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := manager.WaitFinalized(waitContext); err != nil {
		t.Fatal(err)
	}
}

func TestValidate(t *testing.T) {
	good := RunConfig{Package: "com.example.app", DurationSeconds: 60, ThrottleMillis: 500}
	if err := good.Validate(); err != nil {
		t.Fatal(err)
	}
	good.Package = "bad"
	if err := good.Validate(); err == nil {
		t.Fatal("invalid package accepted")
	}
	defaultArgs := strings.Join((AppProcessFactory{Classpath: "runner.jar", StopFile: "/tmp/stop"}).Command(context.Background(), RunConfig{Package: "com.example.app", DurationSeconds: 60, ThrottleMillis: 500}).Args, " ")
	if !strings.Contains(defaultArgs, "--render-fallback-mode bounded") {
		t.Fatalf("default Runner policy is not bounded: %s", defaultArgs)
	}
}

func TestRunnerControlTokenIsActiveOnlyWhileRunning(t *testing.T) {
	manager := &Manager{state: State{Running: true, Package: "com.example.app"}, controlToken: "secret-token"}
	if state, ok := manager.AuthorizeControlToken("secret-token"); !ok || state.Package != "com.example.app" {
		t.Fatalf("active token was rejected: %#v %v", state, ok)
	}
	if _, ok := manager.AuthorizeControlToken("wrong-token"); ok {
		t.Fatal("incorrect token was accepted")
	}
	manager.state.Running = false
	if _, ok := manager.AuthorizeControlToken("secret-token"); ok {
		t.Fatal("inactive Runner token was accepted")
	}
}

func TestRequestIDRejectsDifferentRunnerConfiguration(t *testing.T) {
	root := t.TempDir()
	manager := NewWithArtifacts(helperFactory{}, filepath.Join(root, "stop"), filepath.Join(root, "legacy.log"), filepath.Join(root, "artifacts"), nil)
	config := RunConfig{RequestID: "live-state", Package: "com.example.app", DurationSeconds: 5, ThrottleMillis: 100}
	first, err := manager.Start(config)
	if err != nil {
		t.Fatal(err)
	}
	if repeated, repeatErr := manager.Start(config); repeatErr != nil || repeated.StartedAt == nil || !repeated.StartedAt.Equal(*first.StartedAt) {
		t.Fatalf("identical idempotent request = %+v, %v", repeated, repeatErr)
	}
	changed := config
	changed.DurationSeconds++
	if _, conflictErr := manager.Start(changed); !errors.Is(conflictErr, ErrRequestConflict) {
		t.Fatalf("same requestId with changed configuration error = %v", conflictErr)
	}
	_, _ = manager.Stop()
	waitContext, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := manager.WaitFinalized(waitContext); err != nil {
		t.Fatal(err)
	}
}

func TestAdvancedStrategyValidationAndArguments(t *testing.T) {
	config := RunConfig{Package: "com.example.app", DurationSeconds: 60, ThrottleMillis: 500,
		LowBatteryExit: true, MinBattery: 15, ActivityMode: "allowlist", RenderFallbackMode: "continuous",
		Activities: []string{"com.example.app/.MainActivity"}, TargetActivities: []string{"com.example.app/.Checkout"},
		ControlBlacklist: []string{"立即支付"}, TargetToken: "token-123", ControlToken: "control-123",
		TargetCases: []TargetCase{{Activity: "com.example.app/.Checkout", Task: "checkout", Case: "pay"}}}
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}
	factory := AppProcessFactory{Classpath: "runner.jar", StopFile: "/tmp/stop"}
	joined := strings.Join(factory.Command(context.Background(), config).Args, " ")
	for _, expected := range []string{"--input-cases-per-field 1 --input-max-length 64 --render-fallback-mode continuous", "--low-battery-exit --min-battery 15", "--activity-mode allowlist", "--activity com.example.app/.MainActivity", "--target-activity com.example.app/.Checkout", "--blocked-control 立即支付", "--target-token token-123", "--target-case com.example.app/.Checkout|checkout|pay", "--control-token control-123"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("args=%s missing %s", joined, expected)
		}
	}
	config.ActivityMode = "invalid"
	if err := config.Validate(); err == nil {
		t.Fatal("invalid activity mode accepted")
	}
	config.ActivityMode = "none"
	config.RenderFallbackMode = "automatic"
	if err := config.Validate(); err == nil {
		t.Fatal("invalid render fallback mode accepted")
	}
}
func TestLegacy(t *testing.T) {
	c, err := FromLegacy("request-1", []string{"-p", "com.example.app", "--running-minutes", "2", "--throttle", "250"})
	if err != nil {
		t.Fatal(err)
	}
	if c.DurationSeconds != 120 || c.ThrottleMillis != 250 {
		t.Fatalf("unexpected config: %+v", c)
	}
}

func TestArtifactSessionContainsManifestCoverageAndScreenshots(t *testing.T) {
	root := t.TempDir()
	manager := NewWithArtifacts(helperFactory{}, filepath.Join(root, "stop"), filepath.Join(root, "legacy.log"), filepath.Join(root, "artifacts"), pngScreenshot{})
	state, err := manager.Start(RunConfig{RequestID: "artifact-1", Package: "com.example.app", DurationSeconds: 2, ThrottleMillis: 100, Seed: 7, TargetToken: "must-not-be-persisted"})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for (state.Running || state.Finalizing) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
		state = manager.State()
	}
	if state.Running || state.Finalizing || state.StopReason != "completed" || state.Events != 3 || state.Package != "com.example.app" {
		t.Fatalf("unexpected final state: %+v", state)
	}
	for _, path := range []string{state.LogPath, state.ManifestPath, state.CoveragePath,
		state.GraphPath, state.LogcatPath, state.CrashPath, state.ANRPath, state.NativeCrashPath, state.DiagnosticsPath, state.ExitInfoPath, state.EvidenceIndexPath,
		filepath.Join(state.ArtifactDir, "activity_coverage.txt"), filepath.Join(state.ArtifactDir, "start.png"), filepath.Join(state.ArtifactDir, "finish.png")} {
		if _, err = os.Stat(path); err != nil {
			t.Fatalf("missing artifact %s: %v", path, err)
		}
	}
	if state.Exploration.States != 2 || state.Exploration.Edges != 1 || state.Exploration.Taps != 1 || state.Exploration.Inputs != 1 || state.Exploration.ScrollAttempts != 1 || state.Exploration.ScrollProgress != 1 || state.Exploration.CycleDetections != 1 || state.Exploration.BlockedEdges != 1 {
		t.Fatalf("unexpected exploration metrics: %+v", state.Exploration)
	}
	graph, err := os.ReadFile(state.GraphPath)
	if err != nil || !strings.Contains(string(graph), `"schemaVersion": "xtest-monkey-graph/v1"`) {
		t.Fatalf("unexpected graph summary: %s err=%v", graph, err)
	}
	coverage, err := os.ReadFile(state.CoveragePath)
	if err != nil || !strings.Contains(string(coverage), "com.example.app/.MainActivity") {
		t.Fatalf("unexpected coverage: %s err=%v", coverage, err)
	}
	legacy, err := os.ReadFile(state.LegacyLogPath)
	if err != nil || !strings.Contains(string(legacy), `"events":3`) {
		t.Fatalf("unexpected legacy log: %s err=%v", legacy, err)
	}
	manifest, err := os.ReadFile(state.ManifestPath)
	if err != nil || strings.Contains(string(manifest), "must-not-be-persisted") {
		t.Fatalf("target token leaked into manifest: err=%v", err)
	}
	if strings.Contains(string(manifest), `"finalizing": true`) {
		t.Fatalf("final manifest retained transitional finalizing state: %s", manifest)
	}
	if state.Identity.SessionID == "" || state.Identity.OwnerToken == "" || strings.Contains(string(manifest), `"ownerToken"`) {
		t.Fatalf("execution identity missing or owner token persisted: %s", manifest)
	}
}

func TestOwnedStopRejectsStaleRunnerOwner(t *testing.T) {
	manager := NewWithArtifacts(helperFactory{}, filepath.Join(t.TempDir(), "stop"), filepath.Join(t.TempDir(), "runner.log"), "", nil)
	state, err := manager.Start(RunConfig{RequestID: "owned-stop", Package: "com.example.app", DurationSeconds: 2, ThrottleMillis: 100})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = manager.StopOwned(state.Identity.SessionID, "wrong"); !errors.Is(err, execution.ErrOwnerMismatch) {
		t.Fatalf("error=%v", err)
	}
	if !manager.State().Running {
		t.Fatal("stale owner stopped the active runner")
	}
	if _, err = manager.StopOwned(state.Identity.SessionID, state.Identity.OwnerToken); err != nil {
		t.Fatal(err)
	}
}

func TestForcedTerminationAfterOwnedStopRemainsStopped(t *testing.T) {
	root := t.TempDir()
	manager := NewWithArtifacts(helperFactory{}, filepath.Join(root, "stop"), filepath.Join(root, "runner.log"), "", nil)
	state, err := manager.Start(RunConfig{RequestID: "forced-stop", Package: "com.example.app", DurationSeconds: 60, ThrottleMillis: 100})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = manager.StopOwned(state.Identity.SessionID, state.Identity.OwnerToken); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	state, err = manager.WaitFinalized(ctx)
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	if state.StopReason != "stopped" || state.ExitCode != 0 || state.Error != "" {
		t.Fatalf("state=%+v", state)
	}
}

func TestCrashDiagnosticsBecomeFinalArtifacts(t *testing.T) {
	root := t.TempDir()
	manager := NewWithArtifacts(helperFactory{}, filepath.Join(root, "stop"), filepath.Join(root, "legacy.log"), filepath.Join(root, "artifacts"), pngScreenshot{})
	manager.diagnostics = crashDiagnosticSource{}
	state, err := manager.Start(RunConfig{RequestID: "diagnostic-crash", Package: "com.example.app", DurationSeconds: 2, ThrottleMillis: 100})
	if err != nil {
		t.Fatal(err)
	}
	waitContext, cancelWait := context.WithTimeout(context.Background(), 5*time.Second)
	state, err = manager.WaitFinalized(waitContext)
	cancelWait()
	if err != nil {
		t.Fatal(err)
	}
	if state.FailureType != "app_crash" || state.StopReason != "app_crash" || state.CrashCount != 1 || !state.DiagnosticsAvailable {
		t.Fatalf("unexpected crash state: %+v", state)
	}
	if _, err = os.Stat(filepath.Join(state.ArtifactDir, "failure.png")); err != nil {
		t.Fatalf("failure screenshot missing: %v", err)
	}
	manifest, err := os.ReadFile(state.ManifestPath)
	if err != nil || !strings.Contains(string(manifest), `"failureType": "app_crash"`) || !strings.Contains(string(manifest), `"crash.json"`) {
		t.Fatalf("crash manifest incomplete: %s err=%v", manifest, err)
	}
}

func TestParseDeclaredActivitiesDeduplicatesAndScopesPackage(t *testing.T) {
	dump := `Activity Resolver Table:
      com.example.app/.MainActivity filter 1
      com.example.app/com.example.app.SettingsActivity filter 2
      com.example.app/.MainActivity filter 3
      com.other/.OtherActivity filter 4`
	values := parseDeclaredActivities("com.example.app", dump)
	if len(values) != 2 || values[0] != "com.example.app/.MainActivity" || values[1] != "com.example.app/com.example.app.SettingsActivity" {
		t.Fatalf("activities=%#v", values)
	}
}

func TestMergeKnownActivitiesIncludesRuntimeOnlyActivities(t *testing.T) {
	values := mergeKnownActivities(
		[]string{"com.example.app/.MainActivity"},
		[]string{"com.example.app/.DetailActivity", "com.example.app/.MainActivity"},
	)
	if len(values) != 2 || values[0] != "com.example.app/.DetailActivity" || values[1] != "com.example.app/.MainActivity" {
		t.Fatalf("activities=%#v", values)
	}
}

func TestNormalizeActivityMatchesAndroidShortComponentForm(t *testing.T) {
	tests := map[string]string{
		".MainActivity":                  "com.example.app/.MainActivity",
		"MainActivity":                   "com.example.app/.MainActivity",
		"com.example.app.DetailActivity": "com.example.app/.DetailActivity",
		"com.vendor.SharedActivity":      "com.example.app/com.vendor.SharedActivity",
	}
	for input, expected := range tests {
		if actual := normalizeActivity("com.example.app", input); actual != expected {
			t.Fatalf("normalizeActivity(%q)=%q, want %q", input, actual, expected)
		}
	}
}

func TestCoverageUsesAndroidPackageManagerActivities(t *testing.T) {
	path := filepath.Join(t.TempDir(), "activity_coverage.json")
	source := fakeActivitySource{activities: []string{"com.example.app/.MainActivity", "com.example.app/.SettingsActivity"}}
	if err := writeCoverage(path, "com.example.app", []string{"com.example.app/.MainActivity"}, source); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(content), `"coverageBasis": "android-package-manager"`) || !strings.Contains(string(content), `"declared": 2`) {
		t.Fatalf("coverage=%s err=%v", content, err)
	}
}

func TestFailedRunCapturesFailureScreenshot(t *testing.T) {
	root := t.TempDir()
	manager := NewWithArtifacts(helperFactory{}, filepath.Join(root, "stop"), filepath.Join(root, "legacy.log"), filepath.Join(root, "artifacts"), pngScreenshot{})
	state, err := manager.Start(RunConfig{RequestID: "artifact-failure", Package: "com.example.app", DurationSeconds: 2, ThrottleMillis: 100})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for (state.Running || state.Finalizing) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
		state = manager.State()
	}
	if state.Running || state.Finalizing || state.StopReason != "target_case_failed" || state.ExitCode == 0 {
		t.Fatalf("unexpected failed state: %+v", state)
	}
	if _, err = os.Stat(filepath.Join(state.ArtifactDir, "failure.png")); err != nil {
		t.Fatalf("failure screenshot missing: %v", err)
	}
}

func TestLegacyLogRotatesAtLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runner.log")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = file.Truncate(maxLegacyLogBytes); err != nil {
		t.Fatal(err)
	}
	_ = file.Close()
	if err = rotateLegacyLog(path); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(path + ".1"); err != nil {
		t.Fatal(err)
	}
}
