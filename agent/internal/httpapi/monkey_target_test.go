package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zhoujun94511/xtest-nova/agent/internal/platform"
	"github.com/zhoujun94511/xtest-nova/agent/internal/recordreplay"
	"github.com/zhoujun94511/xtest-nova/agent/internal/runner"
)

type blockingStateRunner struct {
	mu      sync.Mutex
	state   runner.State
	entered chan struct{}
	release chan struct{}
	stopped chan struct{}
	once    sync.Once
}

func (f *blockingStateRunner) State() runner.State {
	f.mu.Lock()
	state := f.state
	f.mu.Unlock()
	f.once.Do(func() {
		close(f.entered)
		<-f.release
	})
	return state
}
func (f *blockingStateRunner) Start(runner.RunConfig) (runner.State, error) { return f.State(), nil }
func (f *blockingStateRunner) Stop() (runner.State, error) {
	f.mu.Lock()
	f.state.Running = false
	state := f.state
	f.mu.Unlock()
	close(f.stopped)
	return state, nil
}
func (f *blockingStateRunner) WaitFinalized(context.Context) (runner.State, error) {
	return f.State(), nil
}

type monkeyTargetForeground struct{ packageName string }

func (f monkeyTargetForeground) ForegroundPackage(context.Context) (string, error) {
	return f.packageName, nil
}

type monkeyTargetExecutor struct{}

func (monkeyTargetExecutor) Run(_ context.Context, name string, _ ...string) (string, error) {
	if name == "wm" {
		return "Physical size: 100x200", nil
	}
	return "", nil
}
func (monkeyTargetExecutor) RunBytes(context.Context, string, ...string) ([]byte, error) {
	return nil, nil
}

var _ platform.Executor = monkeyTargetExecutor{}

func TestMonkeyTargetCasePausesRunnerAndExecutesSignedCaseOnce(t *testing.T) {
	value := recordreplay.Case{SchemaVersion: recordreplay.SchemaVersion, Task: "checkout", Name: "pay", Package: "com.example.app", RecordedAt: time.Now().UTC(), Actions: []recordreplay.Action{{Type: "tap", Start: recordreplay.Point{X: .5, Y: .5}}}}
	if err := value.Seal(); err != nil {
		t.Fatal(err)
	}
	runs := &fakeRunner{state: runner.State{RequestID: "run-1", Running: true}}
	replay := recordreplay.New(t.TempDir(), nil, monkeyTargetForeground{"com.example.app"}, monkeyTargetExecutor{}, nil, nil)
	key := targetCaseKey("com.example.app/.Checkout", "checkout", "pay")
	api := &API{runner: runs, recordReplay: replay, monkeyTarget: &monkeyTargetPlan{token: "token-1", requestID: "run-1", cases: map[string]recordreplay.Case{key: value}, used: map[string]bool{}}}
	body := `{"token":"token-1","activity":"com.example.app/.Checkout","task":"checkout","case":"pay"}`
	response := httptest.NewRecorder()
	api.runMonkeyTargetCase(response, httptest.NewRequest(http.MethodPost, "/v1/monkey/target-case", strings.NewReader(body)))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"completedActions":1`) {
		t.Fatalf("response=%d %s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	api.runMonkeyTargetCase(response, httptest.NewRequest(http.MethodPost, "/v1/monkey/target-case", strings.NewReader(body)))
	if response.Code != http.StatusForbidden {
		t.Fatalf("one-shot token reuse returned %d", response.Code)
	}
}

func TestIdempotentTargetPlanReusesTokenAndNormalizedActivities(t *testing.T) {
	config := runner.RunConfig{RequestID: "run-1", Package: "com.example.app", TargetCases: []runner.TargetCase{{Activity: "com.example.app/.Checkout", Task: "checkout", Case: "pay"}}}
	existing := &monkeyTargetPlan{token: "stable-token", requestID: config.RequestID, fingerprint: monkeyTargetFingerprint(config), cases: map[string]recordreplay.Case{}, used: map[string]bool{}}
	api := &API{recordReplay: recordreplay.New(t.TempDir(), nil, monkeyTargetForeground{"com.example.app"}, monkeyTargetExecutor{}, nil, nil), monkeyTarget: existing}
	plan, err := api.prepareMonkeyTargetPlan(&config)
	if err != nil || plan != existing || config.TargetToken != "stable-token" || len(config.TargetActivities) != 1 || config.TargetActivities[0] != "com.example.app/.Checkout" {
		t.Fatalf("plan=%p existing=%p config=%+v err=%v", plan, existing, config, err)
	}
}

func TestRunnerStopCannotCrossTargetCaseAuthorizationBoundary(t *testing.T) {
	value := recordreplay.Case{SchemaVersion: recordreplay.SchemaVersion, Task: "checkout", Name: "pay", Package: "com.example.app", RecordedAt: time.Now().UTC(), Actions: []recordreplay.Action{{Type: "tap", Start: recordreplay.Point{X: .5, Y: .5}}}}
	if err := value.Seal(); err != nil {
		t.Fatal(err)
	}
	runs := &blockingStateRunner{state: runner.State{RequestID: "run-1", Running: true}, entered: make(chan struct{}), release: make(chan struct{}), stopped: make(chan struct{})}
	replay := recordreplay.New(t.TempDir(), nil, monkeyTargetForeground{"com.example.app"}, monkeyTargetExecutor{}, nil, nil)
	key := targetCaseKey("com.example.app/.Checkout", "checkout", "pay")
	api := &API{runner: runs, recordReplay: replay, monkeyTarget: &monkeyTargetPlan{token: "token-1", requestID: "run-1", cases: map[string]recordreplay.Case{key: value}, used: map[string]bool{}}}
	targetDone := make(chan struct{})
	go func() {
		defer close(targetDone)
		body := `{"token":"token-1","activity":"com.example.app/.Checkout","task":"checkout","case":"pay"}`
		api.runMonkeyTargetCase(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/monkey/target-case", strings.NewReader(body)))
	}()
	select {
	case <-runs.entered:
	case <-time.After(time.Second):
		t.Fatal("target request did not reach Runner state validation")
	}
	stopDone := make(chan struct{})
	go func() {
		defer close(stopDone)
		api.monkey(httptest.NewRecorder(), httptest.NewRequest(http.MethodDelete, "/v1/monkey/runs/current", nil))
	}()
	select {
	case <-runs.stopped:
		close(runs.release)
		t.Fatal("Runner Stop crossed target-case authorization")
	case <-time.After(100 * time.Millisecond):
	}
	close(runs.release)
	select {
	case <-stopDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Runner Stop did not finish")
	}
	select {
	case <-targetDone:
	case <-time.After(2 * time.Second):
		t.Fatal("target request did not finish")
	}
}
