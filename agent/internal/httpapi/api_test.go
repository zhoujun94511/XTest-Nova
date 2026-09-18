package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zhoujun94511/xtest-nova/agent/internal/configstore"
	"github.com/zhoujun94511/xtest-nova/agent/internal/contract"
	"github.com/zhoujun94511/xtest-nova/agent/internal/device"
	"github.com/zhoujun94511/xtest-nova/agent/internal/execution"
	"github.com/zhoujun94511/xtest-nova/agent/internal/exploration"
	"github.com/zhoujun94511/xtest-nova/agent/internal/perflog"
	"github.com/zhoujun94511/xtest-nova/agent/internal/recordreplay"
	"github.com/zhoujun94511/xtest-nova/agent/internal/runner"
	novasystem "github.com/zhoujun94511/xtest-nova/agent/internal/system"
	"github.com/zhoujun94511/xtest-nova/agent/internal/touchreader"
)

type fakeDevice struct{}

func (fakeDevice) Info(context.Context) (device.Info, error) {
	return device.Info{Serial: "test", Platform: "android"}, nil
}
func (fakeDevice) ForegroundPackage(context.Context) (string, error)            { return "com.example.app", nil }
func (fakeDevice) Wake(context.Context) error                                   { return nil }
func (fakeDevice) Shell(context.Context, string, time.Duration) (string, error) { return "ok", nil }

type fakeRunner struct {
	state        runner.State
	controlToken string
	waitCalls    int
	waitErr      error
}

type ownedFakeRunner struct{ fakeRunner }

func (f *ownedFakeRunner) StopOwned(sessionID, ownerToken string) (runner.State, error) {
	if f.state.Identity.SessionID != sessionID || f.state.Identity.OwnerToken != ownerToken {
		return f.state, errors.New("execution owner mismatch")
	}
	return f.Stop()
}

type lightweightRunner struct {
	fakeRunner
	activity   runner.ActivityState
	stateCalls int
}

func (f *lightweightRunner) State() runner.State {
	f.stateCalls++
	return f.fakeRunner.State()
}

func (f *lightweightRunner) Activity() runner.ActivityState { return f.activity }

type stopBoundaryRunner struct {
	mu      sync.Mutex
	state   runner.State
	token   string
	stopped chan struct{}
	once    sync.Once
}

func (f *stopBoundaryRunner) State() runner.State {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.state
}
func (f *stopBoundaryRunner) Start(runner.RunConfig) (runner.State, error) { return f.State(), nil }
func (f *stopBoundaryRunner) Stop() (runner.State, error) {
	f.mu.Lock()
	f.state.Running = false
	state := f.state
	f.mu.Unlock()
	f.once.Do(func() { close(f.stopped) })
	return state, nil
}
func (f *stopBoundaryRunner) WaitFinalized(context.Context) (runner.State, error) {
	return f.State(), nil
}
func (f *stopBoundaryRunner) AuthorizeControlToken(token string) (runner.State, bool) {
	state := f.State()
	return state, state.Running && token == f.token
}

type blockingForegroundDevice struct {
	fakeDevice
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (d *blockingForegroundDevice) ForegroundPackage(context.Context) (string, error) {
	d.once.Do(func() { close(d.entered) })
	<-d.release
	return "com.example.app", nil
}

type recordingScrcpy struct {
	mu       sync.Mutex
	injected []string
}

type blockingRecordCapture struct{}

func (blockingRecordCapture) Capture(ctx context.Context, _ func(time.Time, touchreader.Event)) error {
	<-ctx.Done()
	return ctx.Err()
}

type staticPerformanceCollector struct{}

func (staticPerformanceCollector) Performance(context.Context, string) (novasystem.Performance, error) {
	return novasystem.Performance{Memory: map[string]int{}}, nil
}

type recordingExecutor struct{}

func (recordingExecutor) Run(context.Context, string, ...string) (string, error) {
	return "Physical size: 1080x2400", nil
}

func (recordingExecutor) RunBytes(context.Context, string, ...string) ([]byte, error) {
	return nil, nil
}

func (s *recordingScrcpy) InjectText(_ context.Context, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.injected = append(s.injected, value)
	return nil
}
func (*recordingScrcpy) ServeWebSocket(http.ResponseWriter, *http.Request, string, string) {}

func (f *fakeRunner) State() runner.State { return f.state }
func (f *fakeRunner) Start(c runner.RunConfig) (runner.State, error) {
	f.state = runner.State{RequestID: c.RequestID, Running: true}
	return f.state, nil
}
func (f *fakeRunner) Stop() (runner.State, error) { f.state.Running = false; return f.state, nil }
func (f *fakeRunner) WaitFinalized(context.Context) (runner.State, error) {
	f.waitCalls++
	f.state.Finalizing = false
	return f.state, f.waitErr
}
func (f *fakeRunner) AuthorizeControlToken(token string) (runner.State, bool) {
	return f.state, f.state.Running && token != "" && token == f.controlToken
}

func TestGlobalShutdownWaitsForRunnerFinalization(t *testing.T) {
	runs := &fakeRunner{state: runner.State{Running: true, Finalizing: true}}
	api := &API{runner: runs}
	stopErr, finalizeErr := api.stopRunnerForShutdown()
	if stopErr != nil || finalizeErr != nil {
		t.Fatalf("stopRunnerForShutdown = stop %v, finalize %v", stopErr, finalizeErr)
	}
	if runs.waitCalls != 1 || runs.state.Finalizing {
		t.Fatalf("Runner finalization wait calls=%d state=%+v", runs.waitCalls, runs.state)
	}
	runs.waitErr = context.DeadlineExceeded
	_, finalizeErr = api.stopRunnerForShutdown()
	if !errors.Is(finalizeErr, context.DeadlineExceeded) {
		t.Fatalf("finalization error = %v", finalizeErr)
	}
}

func TestMonkeyStopRequiresCurrentExecutionOwner(t *testing.T) {
	runs := &ownedFakeRunner{fakeRunner: fakeRunner{state: runner.State{Running: true, Identity: execution.Identity{SessionID: "session-1", OwnerToken: "owner-1"}}}}
	api := &API{runner: runs}
	request := httptest.NewRequest(http.MethodDelete, "/v1/monkey/runs/current", nil)
	request.Header.Set("X-XTest-Session-Id", "session-1")
	request.Header.Set("X-XTest-Owner-Token", "stale-owner")
	response := httptest.NewRecorder()
	api.monkey(response, request)
	if response.Code != http.StatusConflict || !runs.state.Running {
		t.Fatalf("stale stop code=%d state=%+v", response.Code, runs.state)
	}
	request = httptest.NewRequest(http.MethodDelete, "/v1/monkey/runs/current", nil)
	request.Header.Set("X-XTest-Session-Id", "session-1")
	request.Header.Set("X-XTest-Owner-Token", "owner-1")
	response = httptest.NewRecorder()
	api.monkey(response, request)
	if response.Code != http.StatusOK || runs.state.Running {
		t.Fatalf("owned stop code=%d state=%+v", response.Code, runs.state)
	}
}

func TestExecutionStopErrorStatus(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
		want int
	}{
		{name: "owner mismatch", err: execution.ErrOwnerMismatch, want: http.StatusConflict},
		{name: "wrapped owner mismatch", err: errors.Join(errors.New("stop failed"), execution.ErrOwnerMismatch), want: http.StatusConflict},
		{name: "deadline", err: context.DeadlineExceeded, want: http.StatusGatewayTimeout},
		{name: "internal", err: errors.New("persist failed"), want: http.StatusInternalServerError},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := executionStopErrorStatus(test.err); got != test.want {
				t.Fatalf("executionStopErrorStatus(%v)=%d, want %d", test.err, got, test.want)
			}
		})
	}
}

func TestMonkeyInputTextRequiresActiveRunnerToken(t *testing.T) {
	runs := &fakeRunner{state: runner.State{Running: true, Package: "com.example.app"}, controlToken: "active-token"}
	api := &API{runner: runs, device: fakeDevice{}}
	response := httptest.NewRecorder()
	api.runMonkeyInputText(response, httptest.NewRequest(http.MethodPost, "/v1/monkey/input-text", strings.NewReader(`{"token":"wrong","text":"测试 😀"}`)))
	if response.Code != http.StatusForbidden {
		t.Fatalf("unauthorized input returned %d: %s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	api.runMonkeyInputText(response, httptest.NewRequest(http.MethodPost, "/v1/monkey/input-text", strings.NewReader(`{"token":"active-token","text":"测试 😀"}`)))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("authorized request did not reach injector gate: %d %s", response.Code, response.Body.String())
	}
}

func TestRunnerStopWaitsForAuthorizedTextInjectionBoundary(t *testing.T) {
	runs := &stopBoundaryRunner{state: runner.State{RequestID: "run-1", Running: true, Package: "com.example.app"}, token: "active-token", stopped: make(chan struct{})}
	foregroundDevice := &blockingForegroundDevice{entered: make(chan struct{}), release: make(chan struct{})}
	controller := &recordingScrcpy{}
	api := &API{runner: runs, device: foregroundDevice, scrcpy: controller}
	inputDone := make(chan struct{})
	go func() {
		defer close(inputDone)
		api.runMonkeyInputText(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/monkey/input-text", strings.NewReader(`{"token":"active-token","text":"boundary"}`)))
	}()
	select {
	case <-foregroundDevice.entered:
	case <-time.After(time.Second):
		t.Fatal("text request did not reach foreground check")
	}
	stopDone := make(chan struct{})
	go func() {
		defer close(stopDone)
		api.monkey(httptest.NewRecorder(), httptest.NewRequest(http.MethodDelete, "/v1/monkey/runs/current", nil))
	}()
	select {
	case <-runs.stopped:
		close(foregroundDevice.release)
		t.Fatal("Runner Stop crossed an in-flight authorized input boundary")
	case <-time.After(100 * time.Millisecond):
	}
	close(foregroundDevice.release)
	select {
	case <-inputDone:
	case <-time.After(time.Second):
		t.Fatal("text request did not finish")
	}
	select {
	case <-stopDone:
	case <-time.After(time.Second):
		t.Fatal("Runner Stop did not finish")
	}
	controller.mu.Lock()
	defer controller.mu.Unlock()
	if len(controller.injected) != 1 || controller.injected[0] != "boundary" {
		t.Fatalf("injected = %#v", controller.injected)
	}
}
func TestPrimaryContracts(t *testing.T) {
	api := New(fakeDevice{}, &fakeRunner{}, configstore.NewEmpty(t.TempDir()+"/config.json"), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "", false)
	for _, path := range []string{"/version", "/ping", "/v1/health", "/v1/capabilities", "/info", "/foregroundPkg", "/wakeupScreen"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		response := httptest.NewRecorder()
		api.Primary().ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("%s returned %d", path, response.Code)
		}
	}
	request := httptest.NewRequest(http.MethodPost, "/shell", strings.NewReader(`{"command":"id"}`))
	response := httptest.NewRecorder()
	api.Primary().ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("unsafe shell returned %d", response.Code)
	}
	for _, path := range []string{"/shell/background?command=echo+test", "/term"} {
		response = httptest.NewRecorder()
		api.Primary().ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusForbidden {
			t.Fatalf("disabled compatibility route %s returned %d", path, response.Code)
		}
	}
}

func TestEveryDeclaredLegacyContractResolvesToARegisteredRoute(t *testing.T) {
	api := New(fakeDevice{}, &fakeRunner{}, configstore.NewEmpty(t.TempDir()+"/config.json"), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "", false)
	handler, ok := api.Primary().(interface {
		Handler(*http.Request) (http.Handler, string)
	})
	if !ok {
		t.Fatal("primary handler does not expose route resolution")
	}
	placeholder := regexp.MustCompile(`\{[^}]+}`)
	for _, route := range contract.Implemented() {
		methods := []string{route.Method}
		if route.Method == "ANY" {
			methods = []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch, http.MethodHead, http.MethodOptions}
		}
		requestPath := placeholder.ReplaceAllString(route.Path, "sample")
		matched := false
		for _, method := range methods {
			request := httptest.NewRequest(method, requestPath, nil)
			_, pattern := handler.Handler(request)
			if pattern == "" {
				continue
			}
			if pattern == "/" && route.Path != "/" && route.Path != "/{path}" {
				continue
			}
			matched = true
		}
		if !matched {
			t.Fatalf("%s %s has no registered handler", route.Method, route.Path)
		}
	}
}

func TestStructuredRequestBodyLimit(t *testing.T) {
	api := New(fakeDevice{}, &fakeRunner{}, configstore.NewEmpty(t.TempDir()+"/config.json"), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "", false)
	body := strings.Repeat(" ", int(maxStructuredRequestBytes)+1)
	request := httptest.NewRequest(http.MethodPost, "/pushConfig", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(controlHeaderName, controlHeaderValue)
	response := httptest.NewRecorder()
	api.Primary().ServeHTTP(response, request)
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized config returned %d: %s", response.Code, response.Body.String())
	}
}

func TestControlHeaderProtectsConfigurationAndStop(t *testing.T) {
	recordings := recordreplay.New(t.TempDir(), nil, nil, nil, nil, nil)
	api := New(fakeDevice{}, &fakeRunner{}, configstore.NewEmpty(t.TempDir()+"/config.json"), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, recordings, "", false)
	for _, test := range []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodPost, "/pushConfig", `{}`},
		{http.MethodPost, "/stop", ""},
		{http.MethodPost, "/v1/recordings/drafts/missing/finalize", `{}`},
		{http.MethodDelete, "/v1/recordings/drafts/missing", ""},
		{http.MethodDelete, "/v1/recordings/cases/missing", ""},
	} {
		response := httptest.NewRecorder()
		api.Primary().ServeHTTP(response, httptest.NewRequest(test.method, test.path, strings.NewReader(test.body)))
		if response.Code != http.StatusForbidden {
			t.Fatalf("%s %s without control header returned %d: %s", test.method, test.path, response.Code, response.Body.String())
		}
	}
	response := httptest.NewRecorder()
	api.Primary().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/stop", nil))
	if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != http.MethodPost {
		t.Fatalf("GET /stop returned %d Allow=%q", response.Code, response.Header().Get("Allow"))
	}
}

func TestCompanionHealthAndConfigControlContract(t *testing.T) {
	store := configstore.NewEmpty(t.TempDir() + "/config.json")
	api := &API{runner: &fakeRunner{}, store: store}
	handler := api.Companion()

	health := httptest.NewRecorder()
	handler.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/health", nil))
	if health.Code != http.StatusOK || !strings.Contains(health.Body.String(), `"service":"xtest-nova-companion"`) {
		t.Fatalf("companion health returned %d: %s", health.Code, health.Body.String())
	}

	denied := httptest.NewRecorder()
	handler.ServeHTTP(denied, httptest.NewRequest(http.MethodPost, "/pushConfig", strings.NewReader(`{"revision":7}`)))
	if denied.Code != http.StatusForbidden {
		t.Fatalf("uncontrolled Companion push returned %d: %s", denied.Code, denied.Body.String())
	}

	pushRequest := httptest.NewRequest(http.MethodPost, "/pushConfig", strings.NewReader(`{"revision":7}`))
	pushRequest.Header.Set(controlHeaderName, " TRUE ")
	pushed := httptest.NewRecorder()
	handler.ServeHTTP(pushed, pushRequest)
	if pushed.Code != http.StatusOK || !strings.Contains(pushed.Body.String(), `"success":true`) {
		t.Fatalf("controlled Companion push returned %d: %s", pushed.Code, pushed.Body.String())
	}

	pulled := httptest.NewRecorder()
	handler.ServeHTTP(pulled, httptest.NewRequest(http.MethodGet, "/pullConfig", nil))
	if pulled.Code != http.StatusOK || !strings.Contains(pulled.Body.String(), `"revision":7`) {
		t.Fatalf("Companion pull returned %d: %s", pulled.Code, pulled.Body.String())
	}
}

func TestRecordingAppendRequiresCurrentHTTPExecutionOwner(t *testing.T) {
	recordings := recordreplay.New(t.TempDir(), blockingRecordCapture{}, fakeDevice{}, recordingExecutor{}, nil, nil)
	api := &API{runner: &fakeRunner{}, recordReplay: recordings}
	handler := api.Primary()

	startedResponse := httptest.NewRecorder()
	handler.ServeHTTP(startedResponse, httptest.NewRequest(http.MethodPost, "/v1/recordings", strings.NewReader(`{"requestId":"http-owner","package":"com.example.app","name":"owner"}`)))
	var started recordreplay.RecordingState
	if startedResponse.Code != http.StatusAccepted || json.Unmarshal(startedResponse.Body.Bytes(), &started) != nil || started.Identity.OwnerToken == "" {
		t.Fatalf("recording start returned %d: %s", startedResponse.Code, startedResponse.Body.String())
	}

	staleRequest := httptest.NewRequest(http.MethodPost, "/v1/recordings/current/text", strings.NewReader(`{"text":"blocked"}`))
	staleRequest.Header.Set("X-XTest-Session-Id", started.Identity.SessionID)
	staleRequest.Header.Set("X-XTest-Owner-Token", "stale-owner")
	staleResponse := httptest.NewRecorder()
	handler.ServeHTTP(staleResponse, staleRequest)
	if staleResponse.Code != http.StatusConflict || len(recordings.CurrentCase().Actions) != 0 {
		t.Fatalf("stale append returned %d case=%+v: %s", staleResponse.Code, recordings.CurrentCase(), staleResponse.Body.String())
	}

	ownedRequest := httptest.NewRequest(http.MethodPost, "/v1/recordings/current/text", strings.NewReader(`{"text":"accepted"}`))
	ownedRequest.Header.Set("X-XTest-Session-Id", started.Identity.SessionID)
	ownedRequest.Header.Set("X-XTest-Owner-Token", started.Identity.OwnerToken)
	ownedResponse := httptest.NewRecorder()
	handler.ServeHTTP(ownedResponse, ownedRequest)
	if ownedResponse.Code != http.StatusOK || len(recordings.CurrentCase().Actions) != 1 {
		t.Fatalf("owned append returned %d case=%+v: %s", ownedResponse.Code, recordings.CurrentCase(), ownedResponse.Body.String())
	}

	stopRequest := httptest.NewRequest(http.MethodDelete, "/v1/recordings/current", nil)
	stopRequest.Header.Set("X-XTest-Session-Id", started.Identity.SessionID)
	stopRequest.Header.Set("X-XTest-Owner-Token", started.Identity.OwnerToken)
	handler.ServeHTTP(httptest.NewRecorder(), stopRequest)
}

func TestPerformanceStopRequiresCurrentHTTPExecutionOwner(t *testing.T) {
	performance := perflog.New(t.TempDir(), staticPerformanceCollector{})
	api := &API{runner: &fakeRunner{}, performance: performance}
	handler := api.Primary()

	startedResponse := httptest.NewRecorder()
	handler.ServeHTTP(startedResponse, httptest.NewRequest(http.MethodPost, "/v1/performance/sessions", strings.NewReader(`{"package":"com.example.app","intervalSeconds":1}`)))
	var started perflog.State
	if startedResponse.Code != http.StatusAccepted || json.Unmarshal(startedResponse.Body.Bytes(), &started) != nil || started.Identity.OwnerToken == "" {
		t.Fatalf("performance start returned %d: %s", startedResponse.Code, startedResponse.Body.String())
	}

	staleRequest := httptest.NewRequest(http.MethodDelete, "/v1/performance/sessions/current", nil)
	staleRequest.Header.Set("X-XTest-Session-Id", started.Identity.SessionID)
	staleRequest.Header.Set("X-XTest-Owner-Token", "stale-owner")
	staleResponse := httptest.NewRecorder()
	handler.ServeHTTP(staleResponse, staleRequest)
	if staleResponse.Code != http.StatusConflict || !performance.State().Running {
		t.Fatalf("stale performance stop returned %d state=%+v: %s", staleResponse.Code, performance.State(), staleResponse.Body.String())
	}

	ownedRequest := httptest.NewRequest(http.MethodDelete, "/v1/performance/sessions/current", nil)
	ownedRequest.Header.Set("X-XTest-Session-Id", started.Identity.SessionID)
	ownedRequest.Header.Set("X-XTest-Owner-Token", started.Identity.OwnerToken)
	ownedResponse := httptest.NewRecorder()
	handler.ServeHTTP(ownedResponse, ownedRequest)
	if ownedResponse.Code != http.StatusOK || performance.State().Running {
		t.Fatalf("owned performance stop returned %d state=%+v: %s", ownedResponse.Code, performance.State(), ownedResponse.Body.String())
	}
}

func TestLongLivedResourcesCannotStartDuringShutdown(t *testing.T) {
	api := New(fakeDevice{}, &fakeRunner{}, configstore.NewEmpty(t.TempDir()+"/config.json"), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "", false)
	api.BeginShutdown()
	for _, test := range []struct{ method, path string }{
		{http.MethodPost, "/services/uiautomator"},
		{http.MethodPost, "/popupBoxAssistant"},
		{http.MethodPut, "/minitouch"},
		{http.MethodPost, "/download"},
	} {
		response := httptest.NewRecorder()
		api.Primary().ServeHTTP(response, httptest.NewRequest(test.method, test.path, nil))
		if response.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s %s during shutdown returned %d: %s", test.method, test.path, response.Code, response.Body.String())
		}
	}
}

func TestRecordingListResponseMatchesCompanionEnvelope(t *testing.T) {
	recordings := recordreplay.New(t.TempDir(), nil, nil, nil, nil, nil)
	api := New(fakeDevice{}, &fakeRunner{}, configstore.NewEmpty(t.TempDir()+"/config.json"), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, recordings, "", false)
	response := httptest.NewRecorder()
	api.Primary().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/recordings?package=com.example.app", nil))
	var envelope struct {
		Cases []recordreplay.CaseSummary `json:"cases"`
	}
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &envelope) != nil || envelope.Cases == nil {
		t.Fatalf("recording list contract returned %d: %s", response.Code, response.Body.String())
	}
}

func TestUnsafeLegacyBackgroundShellAndTerminal(t *testing.T) {
	api := New(fakeDevice{}, &fakeRunner{}, configstore.NewEmpty(t.TempDir()+"/config.json"), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "", true)
	handler := api.Primary()

	for _, test := range []struct {
		method string
		path   string
		body   string
		kind   string
	}{
		{method: http.MethodGet, path: "/shell/background?c=echo+nova-background"},
		{method: http.MethodPost, path: "/shell/background", body: url.Values{"command": {"echo nova-background"}}.Encode(), kind: "application/x-www-form-urlencoded"},
		{method: http.MethodPost, path: "/shell/background", body: `{"c":"echo nova-background"}`, kind: "application/json"},
	} {
		request := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
		request.Header.Set("Content-Type", test.kind)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"success":true`) || !strings.Contains(response.Body.String(), `"pid":`) {
			t.Fatalf("%s %s returned %d: %s", test.method, test.path, response.Code, response.Body.String())
		}
	}

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/shell/background", nil))
	if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != "GET, POST" {
		t.Fatalf("DELETE /shell/background returned %d Allow=%q", response.Code, response.Header().Get("Allow"))
	}

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/term", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "XTest Nova 终端") {
		t.Fatalf("GET /term returned %d: %s", response.Code, response.Body.String())
	}

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/capabilities", nil))
	if !strings.Contains(response.Body.String(), `"legacyBackgroundShell":true`) || !strings.Contains(response.Body.String(), `"terminal":true`) {
		t.Fatalf("capabilities missing enabled legacy routes: %s", response.Body.String())
	}
}

func TestBackgroundShellCannotStartDuringShutdown(t *testing.T) {
	api := New(fakeDevice{}, &fakeRunner{}, configstore.NewEmpty(t.TempDir()+"/config.json"), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "", true)
	api.BeginShutdown()
	response := httptest.NewRecorder()
	api.Primary().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/shell/background?command=echo+too-late", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("background command during shutdown returned %d: %s", response.Code, response.Body.String())
	}
}

type shellProbe struct {
	fakeDevice
	command string
	timeout time.Duration
}

func (s *shellProbe) Shell(_ context.Context, command string, timeout time.Duration) (string, error) {
	s.command, s.timeout = command, timeout
	return "legacy-output", nil
}

func TestLegacyShellGoldenRequests(t *testing.T) {
	tests := []struct {
		name    string
		request *http.Request
		command string
		timeout time.Duration
	}{
		{"GET alias", httptest.NewRequest(http.MethodGet, "/shell?c=pwd&timeout=9", nil), "pwd", 9 * time.Second},
		{"POST form", func() *http.Request {
			request := httptest.NewRequest(http.MethodPost, "/shell", strings.NewReader("command=id&timeout=12"))
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			return request
		}(), "id", 12 * time.Second},
		{"POST JSON", func() *http.Request {
			request := httptest.NewRequest(http.MethodPost, "/shell", strings.NewReader(`{"c":"getprop","timeout":15}`))
			request.Header.Set("Content-Type", "application/json")
			return request
		}(), "getprop", 15 * time.Second},
	}
	for _, test := range tests {
		probe := &shellProbe{}
		api := New(probe, &fakeRunner{}, configstore.NewEmpty(t.TempDir()+"/config.json"), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "", true)
		response := httptest.NewRecorder()
		api.Primary().ServeHTTP(response, test.request)
		if response.Code != http.StatusOK || probe.command != test.command || probe.timeout != test.timeout {
			t.Fatalf("%s returned %d command=%q timeout=%s: %s", test.name, response.Code, probe.command, probe.timeout, response.Body.String())
		}
		var result struct {
			Output   string `json:"output"`
			ExitCode int    `json:"exitCode"`
			Error    string `json:"error"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result.Output != "legacy-output" || result.ExitCode != 0 || result.Error != "" {
			t.Fatalf("%s response mismatch: %#v err=%v", test.name, result, err)
		}
	}
}

func TestShutdownCallbackIsIdempotent(t *testing.T) {
	api := New(fakeDevice{}, &fakeRunner{}, configstore.NewEmpty(t.TempDir()+"/config.json"), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "", false)
	called := 0
	api.SetShutdown(func() { called++ })
	api.requestShutdown()
	if called != 1 {
		t.Fatalf("shutdown called %d times", called)
	}
}

func TestRuntimeDiagnosticsExposeIdleSessions(t *testing.T) {
	api := New(fakeDevice{}, &fakeRunner{}, configstore.NewEmpty(t.TempDir()+"/config.json"), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "", false)
	response := httptest.NewRecorder()
	api.Primary().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/diagnostics/runtime", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"sessions":{"runner":false,"exploration":false,"recording":false,"replay":false,"screenRecord":false,"performance":false}`) {
		t.Fatalf("runtime diagnostics returned %d: %s", response.Code, response.Body.String())
	}
}

func TestSessionActivityEndpointAvoidsFullRunnerState(t *testing.T) {
	runs := &lightweightRunner{activity: runner.ActivityState{Finalizing: true}}
	api := New(fakeDevice{}, runs, configstore.NewEmpty(t.TempDir()+"/config.json"), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "", false)
	for _, path := range []string{"/v1/sessions/current", "/v1/diagnostics/runtime"} {
		response := httptest.NewRecorder()
		api.Primary().ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"runner"`) {
			t.Fatalf("%s returned %d: %s", path, response.Code, response.Body.String())
		}
		if path == "/v1/sessions/current" && (!strings.Contains(response.Body.String(), `"active":true`) || !strings.Contains(response.Body.String(), `"finalizing":true`)) {
			t.Fatalf("session activity missing finalizing state: %s", response.Body.String())
		}
	}
	if runs.stateCalls != 0 {
		t.Fatalf("lightweight endpoints parsed full Runner state %d times", runs.stateCalls)
	}
}

func TestSessionSummaryTreatsTerminalTransitionsAsActive(t *testing.T) {
	for name, sessions := range map[string]sessionActivities{
		"exploration": {Exploration: sessionActivity{Finalizing: true}},
		"recording":   {Recording: sessionActivity{Finalizing: true}},
		"replay":      {Replay: sessionActivity{Finalizing: true}},
		"stopping":    {Recording: sessionActivity{Stopping: true}},
	} {
		t.Run(name, func(t *testing.T) {
			if !sessions.active() {
				t.Fatal("session summary reported an in-flight terminal state as idle")
			}
			legacy := sessions.legacySnapshot()
			if !legacy.Exploration && !legacy.Recording && !legacy.Replay {
				t.Fatalf("legacy snapshot lost busy state: %#v", legacy)
			}
		})
	}
}

func TestConsoleUsesAdaptiveSessionPolling(t *testing.T) {
	data, err := consoleFiles.ReadFile("web/assets/js/app.js")
	if err != nil {
		t.Fatal(err)
	}
	script := string(data)
	for _, required := range []string{"/v1/sessions/current", "window.setTimeout", "document.hidden", "state.stopping", "summary.active ? 3000 : 30000"} {
		if !strings.Contains(script, required) {
			t.Fatalf("adaptive polling marker %q missing", required)
		}
	}
	if strings.Contains(script, "window.setInterval(poll") {
		t.Fatal("fixed interval polling is still enabled")
	}
}

func TestConsolePreservesMutationAndReloadStopContracts(t *testing.T) {
	core, err := consoleFiles.ReadFile("web/assets/js/core.js")
	if err != nil {
		t.Fatal(err)
	}
	coreScript := string(core)
	for _, marker := range []string{
		"'X-XTest-Control': 'true'",
		"...(requestOptions.headers || {})",
	} {
		if !strings.Contains(coreScript, marker) {
			t.Fatalf("API mutation control marker %q is missing", marker)
		}
	}

	app, err := consoleFiles.ReadFile("web/assets/js/app.js")
	if err != nil {
		t.Fatal(err)
	}
	appScript := string(app)
	for _, marker := range []string{
		"if (!model.monkeyState) await refreshMonkey(true)",
		"if (!model.explorationState) await refreshExploration(true)",
		"if (!model.recordState) await refreshRecordReplay('record', true)",
		"if (!model.replayState) await refreshRecordReplay('replay', true)",
		"if (!model.performanceState) await refreshPerformance(true)",
		"state.running || state.stopping || state.finalizing",
		"const busy = state => Boolean(state && (state.running || state.stopping || state.finalizing))",
	} {
		if !strings.Contains(appScript, marker) {
			t.Fatalf("session mutation marker %q is missing", marker)
		}
	}
}

func TestConsoleUsesResponsiveDiagnosticsWorkbench(t *testing.T) {
	files := map[string][]string{
		"web/index.html": {
			"diagnostics-stack", "diagnostics-runtime-panel", "diagnostics-component-list",
			"diagnostics-tools-grid", "component-summary", "刷新全部",
		},
		"web/assets/css/components.css": {
			"repeat(6, minmax(0, 1fr))", "repeat(auto-fit, minmax(245px, 1fr))",
			".diagnostics-tools-grid", "@media (max-width: 700px)",
		},
		"web/assets/js/app.js":       {"const refreshDiagnostics", "Promise.all([", "logs-clear", "renderSoakState"},
		"web/assets/js/renderers.js": {"component-summary-chip", "diagnostics-updated"},
	}
	for name, required := range files {
		data, err := consoleFiles.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		content := string(data)
		for _, marker := range required {
			if !strings.Contains(content, marker) {
				t.Fatalf("responsive diagnostics marker %q missing from %s", marker, name)
			}
		}
	}
	shell, err := consoleFiles.ReadFile("web/assets/css/shell.css")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(shell), "max-width: 1500px") {
		t.Fatal("desktop workspace is still capped at 1500px")
	}
	if strings.Contains(string(shell), "border: 1px solid var(--border); border-width:") {
		t.Fatal("mobile sidebar still overrides its border shorthand")
	}
	index, err := consoleFiles.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(index), `<div class="input-unit">`) {
		t.Fatal("label still contains a non-phrasing input-unit div")
	}
	if !strings.Contains(string(index), `id="selected-app-icon" src=`) {
		t.Fatal("selected application image is missing its required initial src")
	}
	components, err := consoleFiles.ReadFile("web/assets/css/components.css")
	if err != nil {
		t.Fatal(err)
	}
	for _, coupled := range []string{".file-tool, .soak-panel", ".replay-bar, .file-tool, .soak-panel"} {
		if strings.Contains(string(components), coupled) {
			t.Fatalf("business panel remains coupled to generic flex selector %q", coupled)
		}
	}
}

func TestConsoleIncludesLifecycleBoundScrcpyRemoteControl(t *testing.T) {
	files := map[string][]string{
		"web/index.html":       {`data-view="remote"`, `id="remote-canvas"`, `remote-control.js`, `data-remote-key="3"`},
		"web/assets/js/app.js": {"Remote.activate()", "Remote.deactivate()", "Remote.configure({toast})"},
		"web/assets/js/remote-control.js": {
			"/scrcpy/screen/", "/scrcpy/control/original", "new VideoDecoder", "EncodedVideoChunk",
			"pointerdown", "pointermove", "pointerup", "read-clipboard", "requestFullscreen",
		},
	}
	for name, markers := range files {
		data, err := consoleFiles.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, marker := range markers {
			if !strings.Contains(string(data), marker) {
				t.Fatalf("remote control marker %q missing from %s", marker, name)
			}
		}
	}
}

func TestConsoleIncludesStructuredPerformanceWorkbench(t *testing.T) {
	files := map[string][]string{
		"web/index.html":               {`id="perf-interval"`, `id="perf-duration"`, `id="perf-metrics"`, `performance.js`},
		"web/assets/js/app.js":         {"PerformanceView.render(state)", "intervalSeconds", "durationSeconds"},
		"web/assets/js/performance.js": {"应用 CPU", "应用网络", "metricStates", "renderSummary", "network_rx_bytes_per_second", "history.length > 120"},
	}
	for name, markers := range files {
		data, err := consoleFiles.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, marker := range markers {
			if !strings.Contains(string(data), marker) {
				t.Fatalf("Literal performance marker %q missing from %s", marker, name)
			}
		}
	}
}

func TestLegacyMonkeyRequest(t *testing.T) {
	runs := &fakeRunner{}
	api := New(fakeDevice{}, runs, configstore.NewEmpty(t.TempDir()+"/config.json"), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "", false)
	body := `{"requestId":"test-1","args":["-p","com.example.app","--running-minutes","1","--throttle","250"]}`
	request := httptest.NewRequest(http.MethodPost, "/monkey", strings.NewReader(body))
	response := httptest.NewRecorder()
	api.Companion().ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("returned %d: %s", response.Code, response.Body.String())
	}
}
func TestEmbeddedConsole(t *testing.T) {
	api := New(fakeDevice{}, &fakeRunner{}, configstore.NewEmpty(t.TempDir()+"/config.json"), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "", false)
	for _, path := range []string{
		"/",
		"/assets/css/foundation.css",
		"/assets/css/shell.css",
		"/assets/css/components.css",
		"/assets/css/terminal.css",
		"/assets/js/core.js",
		"/assets/js/renderers.js",
		"/assets/js/package-selector.js",
		"/assets/js/remote-control.js",
		"/assets/js/performance.js",
		"/assets/js/app.js",
		"/assets/js/terminal.js",
		"/assets/media/favicon.svg",
		"/assets/media/placeholder.svg",
		"/favicon.ico",
		"/app.js",
		"/app.css",
		"/placeholder.svg",
		"/terminal.js",
		"/terminal.css",
		"/static/js/app.js",
		"/static/css/app.css",
		"/console/device",
	} {
		response := httptest.NewRecorder()
		api.Primary().ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("%s returned %d", path, response.Code)
		}
		if path == "/" {
			for _, label := range []string{"概览", "应用", "自动化", "性能", "产物", "诊断", "随机遍历", "智能探索", "录制与回放"} {
				if !strings.Contains(response.Body.String(), label) {
					t.Fatalf("console is missing %q workflow", label)
				}
			}
			for _, asset := range []string{`href="assets/css/foundation.css?v=2"`, `href="assets/css/shell.css?v=4"`, `src="assets/js/core.js?v=2"`, `src="assets/js/renderers.js?v=4"`, `src="assets/js/package-selector.js?v=3"`, `src="assets/js/remote-control.js?v=2"`, `src="assets/js/performance.js?v=2"`, `src="assets/media/placeholder.svg?v=2"`} {
				if !strings.Contains(response.Body.String(), asset) {
					t.Fatalf("console is missing relative asset %q", asset)
				}
			}
			for _, selector := range []string{`id="automation-package" data-package-select`, `id="performance-package" data-package-select`, `id="artifact-package" data-package-filter`, `id="file-tree"`, `data-action="file-preview"`, `data-action="file-export-directory"`, `data-action="app-uninstall"`} {
				if !strings.Contains(response.Body.String(), selector) {
					t.Fatalf("console is missing package selector %q", selector)
				}
			}
			if strings.Contains(response.Body.String(), `id="global-package"`) {
				t.Fatal("console must not duplicate target selection in the navigation sidebar")
			}
		}
		if path == "/assets/css/foundation.css" && !strings.Contains(response.Body.String(), "[hidden] { display: none !important; }") {
			t.Fatal("foundation stylesheet does not preserve the hidden attribute")
		}
		if path == "/assets/js/renderers.js" {
			for _, expected := range []string{"coreComponentNames", "onDemandComponentNames", "按需启动"} {
				if !strings.Contains(response.Body.String(), expected) {
					t.Fatalf("component readiness renderer is missing %q", expected)
				}
			}
		}
	}
	unsafeAPI := New(fakeDevice{}, &fakeRunner{}, configstore.NewEmpty(t.TempDir()+"/unsafe-config.json"), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "", true)
	terminal := httptest.NewRecorder()
	unsafeAPI.Primary().ServeHTTP(terminal, httptest.NewRequest(http.MethodGet, "/term", nil))
	if terminal.Code != http.StatusOK {
		t.Fatalf("terminal returned %d: %s", terminal.Code, terminal.Body.String())
	}
	for _, asset := range []string{`href="assets/css/terminal.css?v=2"`, `src="assets/js/terminal.js?v=2"`} {
		if !strings.Contains(terminal.Body.String(), asset) {
			t.Fatalf("terminal is missing relative asset %q", asset)
		}
	}
	missing := httptest.NewRecorder()
	api.Primary().ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/static/js/missing.js", nil))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing asset returned %d", missing.Code)
	}
}

func TestConsoleLANLoginStaticContracts(t *testing.T) {
	files := map[string][]string{
		"web/index.html": {
			`id="lan-login-form"`, `aria-labelledby="lan-login-title"`,
			`id="lan-token"`, `type="password"`, `id="lan-logout"`,
		},
		"web/assets/js/app.js": {
			"/v1/auth/lan/status", "/v1/auth/lan/session",
			"credentials: 'same-origin'", "nova-auth-required",
		},
		"web/assets/js/core.js": {
			"credentials: 'same-origin'", "nova-auth-required",
		},
	}
	for name, markers := range files {
		data, err := consoleFiles.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, marker := range markers {
			if !strings.Contains(string(data), marker) {
				t.Fatalf("LAN login marker %q missing from %s", marker, name)
			}
		}
	}

	appData, _ := consoleFiles.ReadFile("web/assets/js/app.js")
	appScript := string(appData)
	if strings.Contains(appScript, "localStorage") || strings.Contains(appScript, "sessionStorage") {
		t.Fatal("console stores state in Web Storage")
	}
	urlToken := regexp.MustCompile(`(?i)[?&](?:token|api[_-]?token|access[_-]?token)=`)
	for _, name := range []string{
		"web/index.html",
		"web/terminal.html",
		"web/assets/js/app.js",
		"web/assets/js/core.js",
		"web/assets/js/renderers.js",
		"web/assets/js/package-selector.js",
		"web/assets/js/remote-control.js",
		"web/assets/js/performance.js",
		"web/assets/js/terminal.js",
	} {
		data, err := consoleFiles.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		script := string(data)
		if strings.Contains(script, "localStorage") || strings.Contains(script, "sessionStorage") {
			t.Fatalf("console file %s uses Web Storage", name)
		}
		if urlToken.MatchString(script) || strings.Contains(script, "Authorization") {
			t.Fatalf("console file %s can place a token in a URL or header", name)
		}
	}
	if got, want := strings.Count(appScript, "fetch("), strings.Count(appScript, "credentials: 'same-origin'"); got != want {
		t.Fatalf("app fetch credentials markers = %d fetches, %d credentials", got, want)
	}
	coreData, _ := consoleFiles.ReadFile("web/assets/js/core.js")
	coreScript := string(coreData)
	if got, want := strings.Count(coreScript, "fetch("), strings.Count(coreScript, "credentials: 'same-origin'"); got != want {
		t.Fatalf("core fetch credentials markers = %d fetches, %d credentials", got, want)
	}
	for _, name := range []string{"web/assets/js/remote-control.js", "web/assets/js/terminal.js"} {
		data, _ := consoleFiles.ReadFile(name)
		script := string(data)
		if strings.Contains(script, "?token=") || strings.Contains(script, "api_token") || strings.Contains(script, "access_token") {
			t.Fatalf("WebSocket script %s includes a query token", name)
		}
		if !strings.Contains(script, "重新认证") {
			t.Fatalf("WebSocket script %s does not explain reauthentication", name)
		}
	}
}

func TestConsoleAuthStatusAndSecurityHeaders(t *testing.T) {
	api := New(fakeDevice{}, &fakeRunner{}, configstore.NewEmpty(t.TempDir()+"/config.json"), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "", false)
	handler := api.Primary()
	for _, path := range []string{"/", "/assets/js/app.js", "/favicon.ico"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK || response.Header().Get("Referrer-Policy") != "no-referrer" {
			t.Fatalf("%s status=%d Referrer-Policy=%q", path, response.Code, response.Header().Get("Referrer-Policy"))
		}
	}
	index := httptest.NewRecorder()
	handler.ServeHTTP(index, httptest.NewRequest(http.MethodGet, "/", nil))
	if index.Header().Get("Cache-Control") != "no-store" || !strings.Contains(index.Body.String(), "运行概览") {
		t.Fatalf("default console response headers/body = %q %q", index.Header().Get("Cache-Control"), index.Body.String())
	}
	status := httptest.NewRecorder()
	handler.ServeHTTP(status, httptest.NewRequest(http.MethodGet, "/v1/auth/lan/status", nil))
	if status.Code != http.StatusOK ||
		status.Header().Get("Cache-Control") != "no-store" ||
		status.Header().Get("Referrer-Policy") != "no-referrer" ||
		!strings.Contains(status.Body.String(), `"enabled":false`) ||
		!strings.Contains(status.Body.String(), `"authenticated":true`) {
		t.Fatalf("default auth status = %d headers=%v body=%s", status.Code, status.Header(), status.Body.String())
	}
}

func TestJSONRPCProxyRejectsOversizedResponseWithoutTruncatingSuccess(t *testing.T) {
	response := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"result":"too-large"}`)),
	}
	recorder := httptest.NewRecorder()
	writeProxiedResponse(recorder, response, 8)
	if recorder.Code != http.StatusBadGateway || !strings.Contains(recorder.Body.String(), "response exceeds") {
		t.Fatalf("oversized response returned %d: %s", recorder.Code, recorder.Body.String())
	}
	if contentType := recorder.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		t.Fatalf("oversized response content type=%q", contentType)
	}
}

func TestJSONRPCProxyPreservesBoundedResponse(t *testing.T) {
	response := &http.Response{
		StatusCode: http.StatusCreated,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
	}
	recorder := httptest.NewRecorder()
	writeProxiedResponse(recorder, response, 64)
	if recorder.Code != http.StatusCreated || recorder.Body.String() != `{"ok":true}` {
		t.Fatalf("bounded response returned %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestScrcpyRouteReportsUnavailableBridge(t *testing.T) {
	api := New(fakeDevice{}, &fakeRunner{}, configstore.NewEmpty(t.TempDir()+"/config.json"), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "", false)
	response := httptest.NewRecorder()
	api.Primary().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/scrcpy/screen/normal", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("scrcpy route returned %d", response.Code)
	}
}

func TestExplorationRouteReportsUnavailableEngine(t *testing.T) {
	api := New(fakeDevice{}, &fakeRunner{}, configstore.NewEmpty(t.TempDir()+"/config.json"), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "", false)
	response := httptest.NewRecorder()
	api.Primary().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/exploration/preview", strings.NewReader(`{"package":"com.example"}`)))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("exploration route returned %d", response.Code)
	}
}

func TestExplorationCannotOverlapRandomRunner(t *testing.T) {
	runs := &fakeRunner{state: runner.State{Running: true}}
	explorer := exploration.New(nil, nil, nil)
	api := New(fakeDevice{}, runs, configstore.NewEmpty(t.TempDir()+"/config.json"), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, explorer, nil, "", false)
	response := httptest.NewRecorder()
	body := `{"package":"com.example.app","execute":true}`
	api.Primary().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/exploration/sessions", strings.NewReader(body)))
	if response.Code != http.StatusConflict {
		t.Fatalf("overlapping exploration returned %d", response.Code)
	}
}

func TestExplorationReportIsAvailableWithoutExecutingInputs(t *testing.T) {
	explorer := exploration.New(nil, nil, nil)
	api := New(fakeDevice{}, &fakeRunner{}, configstore.NewEmpty(t.TempDir()+"/config.json"), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, explorer, nil, "", false)
	response := httptest.NewRecorder()
	api.Primary().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/exploration/report?scenario=read-only-check", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"scenario":"read-only-check"`) || !strings.Contains(response.Body.String(), `"completed":false`) {
		t.Fatalf("report returned %d: %s", response.Code, response.Body.String())
	}
}

func TestRecordReplayRouteReportsUnavailableEngine(t *testing.T) {
	api := New(fakeDevice{}, &fakeRunner{}, configstore.NewEmpty(t.TempDir()+"/config.json"), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "", false)
	response := httptest.NewRecorder()
	api.Primary().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/recordings/current", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("recording route returned %d", response.Code)
	}
}

func TestReplayValidationDoesNotRequireExecutionEngine(t *testing.T) {
	value := recordreplay.Case{SchemaVersion: recordreplay.SchemaVersion, Name: "empty-safe", Package: "com.example.app", RecordedAt: time.Now().UTC(), Actions: []recordreplay.Action{}}
	if err := value.Seal(); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	api := New(fakeDevice{}, &fakeRunner{}, configstore.NewEmpty(t.TempDir()+"/config.json"), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "", false)
	response := httptest.NewRecorder()
	api.Primary().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/replays/validate", strings.NewReader(string(body))))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"valid":true`) {
		t.Fatalf("validation returned %d: %s", response.Code, response.Body.String())
	}
}
