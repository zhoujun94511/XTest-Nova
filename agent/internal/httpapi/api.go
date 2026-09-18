package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zhoujun94511/xtest-nova/agent/internal/apps"
	"github.com/zhoujun94511/xtest-nova/agent/internal/artifactcatalog"
	"github.com/zhoujun94511/xtest-nova/agent/internal/artifacts"
	"github.com/zhoujun94511/xtest-nova/agent/internal/autopopup"
	"github.com/zhoujun94511/xtest-nova/agent/internal/buildinfo"
	"github.com/zhoujun94511/xtest-nova/agent/internal/componenthealth"
	"github.com/zhoujun94511/xtest-nova/agent/internal/configstore"
	"github.com/zhoujun94511/xtest-nova/agent/internal/device"
	"github.com/zhoujun94511/xtest-nova/agent/internal/diagnosticsoak"
	"github.com/zhoujun94511/xtest-nova/agent/internal/events"
	"github.com/zhoujun94511/xtest-nova/agent/internal/execution"
	"github.com/zhoujun94511/xtest-nova/agent/internal/exploration"
	"github.com/zhoujun94511/xtest-nova/agent/internal/files"
	"github.com/zhoujun94511/xtest-nova/agent/internal/legacyexec"
	"github.com/zhoujun94511/xtest-nova/agent/internal/legacyterm"
	"github.com/zhoujun94511/xtest-nova/agent/internal/logview"
	"github.com/zhoujun94511/xtest-nova/agent/internal/minitouch"
	"github.com/zhoujun94511/xtest-nova/agent/internal/monitor"
	"github.com/zhoujun94511/xtest-nova/agent/internal/perflog"
	"github.com/zhoujun94511/xtest-nova/agent/internal/recordreplay"
	"github.com/zhoujun94511/xtest-nova/agent/internal/runner"
	"github.com/zhoujun94511/xtest-nova/agent/internal/scrcpy"
	"github.com/zhoujun94511/xtest-nova/agent/internal/screenrecord"
	novasystem "github.com/zhoujun94511/xtest-nova/agent/internal/system"
	"github.com/zhoujun94511/xtest-nova/agent/internal/tasks"
)

const Version = buildinfo.Version

const (
	maxStructuredRequestBytes int64 = 4 << 20
	structuredReadTimeout           = 30 * time.Second
	multipartReadTimeout            = 10 * time.Minute
	controlHeaderName               = "X-XTest-Control"
	controlHeaderValue              = "true"
)

type Device interface {
	Info(context.Context) (device.Info, error)
	ForegroundPackage(context.Context) (string, error)
	Wake(context.Context) error
	Shell(context.Context, string, time.Duration) (string, error)
}
type Runner interface {
	State() runner.State
	Start(runner.RunConfig) (runner.State, error)
	Stop() (runner.State, error)
	WaitFinalized(context.Context) (runner.State, error)
}
type Applications interface {
	List(context.Context, bool) ([]apps.Package, error)
	Info(context.Context, string) (apps.Package, error)
	Icon(context.Context, string) ([]byte, error)
	Session(context.Context, string) (apps.Package, string, error)
	Launch(context.Context, string) (string, error)
	Stop(context.Context, string) error
	Uninstall(context.Context, string, bool) (string, error)
	Install(context.Context, string, bool) (string, error)
}
type FileSystem interface {
	Info(string) (files.Entry, error)
	Open(string) (*os.File, fs.FileInfo, error)
	Save(string, io.Reader, fs.FileMode) (files.Entry, error)
	Resolve(string) (string, error)
	ResolveForWrite(string) (string, error)
}
type Automation interface {
	Screenshot(context.Context) ([]byte, error)
	Hierarchy(context.Context) (string, error)
	HierarchyWithScreenshot(context.Context) (map[string]any, error)
	Running() bool
	Start() error
	Stop(context.Context) error
	SetCommandTimeout(time.Duration) error
	ResetCommandTimeout()
}
type ScrcpyController interface {
	InjectText(context.Context, string) error
	ServeWebSocket(http.ResponseWriter, *http.Request, string, string)
}
type API struct {
	device            Device
	runner            Runner
	store             *configstore.Store
	apps              Applications
	files             FileSystem
	automation        Automation
	system            *novasystem.Service
	companionAPK      string
	tasks             *tasks.Manager
	minitouch         *minitouch.Manager
	popup             *autopopup.Manager
	events            *events.Hub
	monitor           *monitor.Service
	recorder          *screenrecord.Manager
	scrcpy            ScrcpyController
	explorer          *exploration.Manager
	recordReplay      *recordreplay.Manager
	performance       *perflog.Manager
	background        *legacyexec.Background
	terminal          *legacyterm.Handler
	unsafe            bool
	legacyUiAutomator bool
	executionMu       sync.Mutex
	shuttingDown      bool
	monkeyTargetMu    sync.Mutex
	monkeyTarget      *monkeyTargetPlan
	shutdownMu        sync.RWMutex
	shutdown          func()
	startedAt         time.Time
	components        *componenthealth.Registry
	artifactIndex     *artifactcatalog.Catalog
	logs              *logview.Reader
	soak              *diagnosticsoak.Manager
	executionOwner    *execution.Coordinator
	lanAuth           *lanAuth
}

func (a *API) SetShutdown(shutdown func()) {
	a.shutdownMu.Lock()
	defer a.shutdownMu.Unlock()
	a.shutdown = shutdown
}
func (a *API) requestShutdown() {
	a.shutdownMu.RLock()
	shutdown := a.shutdown
	a.shutdownMu.RUnlock()
	if shutdown != nil {
		shutdown()
	}
}

func (a *API) StopBackground(ctx context.Context) error {
	return a.background.StopAll(ctx)
}

func (a *API) BeginShutdown() {
	a.executionMu.Lock()
	a.shuttingDown = true
	a.executionMu.Unlock()
}

func New(d Device, r Runner, s *configstore.Store, applications Applications, fileSystem FileSystem, automator Automation, systemService *novasystem.Service, taskManager *tasks.Manager, minitouchManager *minitouch.Manager, popupManager *autopopup.Manager, eventHub *events.Hub, monitorService *monitor.Service, recorder *screenrecord.Manager, scrcpyManager *scrcpy.Manager, explorer *exploration.Manager, recordReplay *recordreplay.Manager, companionAPK string, unsafe bool, lanConfigs ...LANAuthConfig) *API {
	var scrcpyController ScrcpyController
	if scrcpyManager != nil {
		scrcpyController = scrcpyManager
	}
	owner := execution.NewCoordinator()
	if explorer != nil {
		explorer.SetExecutionCoordinator(owner)
		explorer.SetArtifactRoot(artifacts.Root)
	}
	if recordReplay != nil {
		recordReplay.SetExecutionCoordinator(owner)
	}
	performance := perflog.New(artifacts.Root, systemService)
	performance.SetExecutionCoordinator(owner)
	var auth *lanAuth
	if len(lanConfigs) > 0 {
		auth = newLANAuth(lanConfigs[0])
	}
	a := &API{device: d, runner: r, store: s, apps: applications, files: fileSystem, automation: automator, system: systemService, tasks: taskManager, minitouch: minitouchManager, popup: popupManager, events: eventHub, monitor: monitorService, recorder: recorder, scrcpy: scrcpyController, explorer: explorer, recordReplay: recordReplay, performance: performance, background: legacyexec.NewBackground(16), terminal: legacyterm.New(4), companionAPK: companionAPK, unsafe: unsafe, startedAt: time.Now().UTC(), components: componenthealth.NewRegistry(), artifactIndex: artifactcatalog.New(artifacts.Root), logs: logview.New(map[string]string{"agent": "/data/local/tmp/xtest-nova-agent.log", "minitouch": "/data/local/tmp/xtest-nova-minitouch.log"}), executionOwner: owner, lanAuth: auth}
	a.soak = diagnosticsoak.New(artifacts.Root, a.soakSample)
	return a
}

func (a *API) SetAuxiliaryStatus(status componenthealth.Status) { a.components.Set(status) }
func (a *API) SetLegacyUiAutomator(enabled bool)                { a.legacyUiAutomator = enabled }
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, e error) {
	writeJSON(w, status, map[string]any{"success": false, "error": e.Error()})
}

func requestErrorStatus(err error) int {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return http.StatusRequestEntityTooLarge
	}
	return http.StatusBadRequest
}

func executionCredentials(r *http.Request) (string, string) {
	return strings.TrimSpace(r.Header.Get("X-XTest-Session-Id")), strings.TrimSpace(r.Header.Get("X-XTest-Owner-Token"))
}

func requireControlHeader(w http.ResponseWriter, r *http.Request) bool {
	if !strings.EqualFold(strings.TrimSpace(r.Header.Get(controlHeaderName)), controlHeaderValue) {
		fail(w, http.StatusForbidden, errors.New("control request header required"))
		return false
	}
	return true
}

type requestBoundedHandler struct{ next http.Handler }

func (h requestBoundedHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Body == nil || r.Body == http.NoBody || r.Method == http.MethodGet || strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		h.next.ServeHTTP(w, r)
		return
	}
	limit, timeout := maxStructuredRequestBytes, structuredReadTimeout
	if strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "multipart/form-data") {
		limit, timeout = maxMultipartRequestBytes, multipartReadTimeout
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	controller := http.NewResponseController(w)
	_ = controller.SetReadDeadline(time.Now().Add(timeout))
	defer func() { _ = controller.SetReadDeadline(time.Time{}) }()
	h.next.ServeHTTP(w, r)
}

func (h requestBoundedHandler) Handler(r *http.Request) (http.Handler, string) {
	if resolver, ok := h.next.(interface {
		Handler(*http.Request) (http.Handler, string)
	}); ok {
		return resolver.Handler(r)
	}
	return nil, ""
}

func withRequestBounds(next http.Handler) http.Handler { return requestBoundedHandler{next: next} }
func (a *API) Primary() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("/version", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(Version)) })
	m.HandleFunc("/ping", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{"success": true, "pong": true})
	})
	m.HandleFunc("GET /v1/health", func(w http.ResponseWriter, _ *http.Request) {
		monitorReady := a.monitor == nil || a.monitor.Ready()
		status, code := "ok", http.StatusOK
		if !monitorReady {
			status = "degraded"
		}
		w.Header().Set("X-XTest-Version", Version)
		writeJSON(w, code, map[string]any{"status": status, "version": Version, "time": time.Now().UTC(), "ready": map[string]bool{"agent": true, "monitor": monitorReady}})
	})
	m.HandleFunc("GET /v1/auth/lan/status", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		writeJSON(w, http.StatusOK, map[string]bool{"enabled": false, "authenticated": true})
	})
	m.HandleFunc("GET /v1/capabilities", a.capabilities)
	m.HandleFunc("GET /pullConfig", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, a.store.Get()) })
	m.HandleFunc("POST /pushConfig", a.pushConfig)
	m.HandleFunc("/info", a.info)
	m.HandleFunc("GET /v1/device", a.info)
	m.HandleFunc("GET /foregroundPkg", a.foreground)
	m.HandleFunc("GET /wakeupScreen", a.wakeup)
	m.HandleFunc("/shell", a.shell)
	m.HandleFunc("/term", a.term)
	m.HandleFunc("/v1/monkey/runs/current", a.monkey)
	m.HandleFunc("POST /v1/monkey/target-case", a.runMonkeyTargetCase)
	m.HandleFunc("POST /v1/monkey/input-text", a.runMonkeyInputText)
	a.registerAppFileAutomationRoutes(m)
	a.registerDeviceProcessRoutes(m)
	a.registerTaskSessionRoutes(m)
	a.registerUtilityRoutes(m)
	a.registerStreamingRoutes(m)
	a.registerWebConsoleRoutes(m)
	a.registerScrcpyRoutes(m)
	a.registerExplorationRoutes(m)
	a.registerRecordReplayRoutes(m)
	a.registerRuntimeRoutes(m)
	a.registerDiagnosticsRoutes(m)
	a.registerSoakRoutes(m)
	a.registerPerformanceRoutes(m)
	handler := withRequestBounds(m)
	if a.lanAuth != nil {
		return a.lanAuth.handler(handler)
	}
	return handler
}
func (a *API) Companion() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{"success": true, "service": "xtest-nova-companion", "version": Version})
	})
	m.HandleFunc("/monkey", a.monkey)
	m.HandleFunc("GET /pullConfig", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, a.store.Get()) })
	m.HandleFunc("POST /pushConfig", a.pushConfig)
	m.HandleFunc("/uiautomator", a.uiautomatorJSON)
	m.HandleFunc("GET /dumpWindowHierarchy", a.hierarchy)
	m.HandleFunc("GET /dumpWindowHierarchyWithScreenshot", a.hierarchyWithScreenshot)
	m.HandleFunc("GET /takeScreenshot", a.screenshot)
	a.registerCompanionDeviceRoutes(m)
	a.registerCompanionUtilityRoutes(m)
	return withRequestBounds(m)
}
func (a *API) capabilities(w http.ResponseWriter, _ *http.Request) {
	monitorReady := a.monitor != nil && a.monitor.Ready()
	uiautomatorReady := a.automation != nil && a.automation.Running()
	minitouchReady := a.minitouch != nil && a.minitouch.State().Running
	_, companionErr := os.Stat(a.companionAPK)
	companionReady := a.companionAPK != "" && companionErr == nil
	writeJSON(w, 200, map[string]any{
		"deviceInfo": true, "foregroundPackage": true, "wakeScreen": true, "monkeyRunner": true, "configSync": true, "applications": true, "files": true, "screenshot": true, "hierarchy": true, "uiautomator": true, "processes": true, "performance": true, "performanceLogging": true, "network": true, "storage": true, "ime": true, "services": true, "autoPopup": true, "minitouchWebSocket": true, "appEvents": true, "monitor": true, "screenrecord": true, "minicapFallback": true, "touchReader": true, "jsonRPCProxy": a.legacyUiAutomator, "webConsole": true, "scrcpy": true, "intelligentTraversal": true, "recordReplay": a.recordReplay != nil, "runtimeDiagnostics": true, "legacyShell": a.unsafe, "legacyBackgroundShell": a.unsafe, "terminal": a.unsafe,
		"readiness":           map[string]bool{"monitor": monitorReady, "uiautomator": uiautomatorReady, "minitouch": minitouchReady, "companionPayload": companionReady, "recordReplay": a.recordReplay != nil},
		"capabilitySemantics": "top-level booleans describe supported APIs; readiness describes active or staged runtime dependencies",
	})
}

func (a *API) rawHierarchy(w http.ResponseWriter, r *http.Request) {
	xml, err := a.automation.Hierarchy(r.Context())
	if err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, xml)
}
func (a *API) info(w http.ResponseWriter, r *http.Request) {
	v, e := a.device.Info(r.Context())
	if e != nil {
		fail(w, 500, e)
		return
	}
	writeJSON(w, 200, v)
}
func (a *API) foreground(w http.ResponseWriter, r *http.Request) {
	v, e := a.device.ForegroundPackage(r.Context())
	if e != nil {
		fail(w, 500, e)
		return
	}
	writeJSON(w, 200, map[string]any{"package": v})
}
func (a *API) wakeup(w http.ResponseWriter, r *http.Request) {
	if e := a.device.Wake(r.Context()); e != nil {
		fail(w, 500, e)
		return
	}
	writeJSON(w, 200, map[string]any{"success": true})
}
func (a *API) shell(w http.ResponseWriter, r *http.Request) {
	if !a.unsafe {
		fail(w, 403, errors.New("legacy shell API disabled"))
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		w.Header().Set("Allow", "GET, POST")
		fail(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	var commandValue, timeoutValue string
	if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		var body struct {
			Command string `json:"command"`
			C       string `json:"c"`
			Timeout any    `json:"timeout"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			fail(w, 400, err)
			return
		}
		commandValue = body.Command
		if commandValue == "" {
			commandValue = body.C
		}
		if body.Timeout != nil {
			timeoutValue = fmt.Sprint(body.Timeout)
		}
	} else {
		if err := r.ParseForm(); err != nil {
			fail(w, 400, err)
			return
		}
		commandValue = r.FormValue("command")
		if commandValue == "" {
			commandValue = r.FormValue("c")
		}
		timeoutValue = r.FormValue("timeout")
	}
	if strings.TrimSpace(commandValue) == "" {
		fail(w, 400, errors.New("command required"))
		return
	}
	timeout := 60 * time.Second
	if timeoutValue != "" {
		seconds, err := strconv.Atoi(timeoutValue)
		if err != nil || seconds < 1 || seconds > 3600 {
			fail(w, 400, errors.New("timeout must be an integer from 1 to 3600 seconds"))
			return
		}
		timeout = time.Duration(seconds) * time.Second
	}
	out, shellErr := a.device.Shell(r.Context(), commandValue, timeout)
	exitCode, errorText := 0, ""
	if shellErr != nil {
		errorText = shellErr.Error()
		exitCode = -1
		var exitError *exec.ExitError
		if errors.As(shellErr, &exitError) {
			exitCode = exitError.ExitCode()
		}
	}
	writeJSON(w, 200, map[string]any{"output": out, "exitCode": exitCode, "error": errorText})
}
func (a *API) monkey(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		writeJSON(w, 200, a.runner.State())
	case "DELETE":
		a.executionMu.Lock()
		var s runner.State
		var e error
		if owned, ok := a.runner.(interface {
			StopOwned(string, string) (runner.State, error)
		}); ok {
			sessionID, ownerToken := executionCredentials(r)
			s, e = owned.StopOwned(sessionID, ownerToken)
		} else {
			s, e = a.runner.Stop()
		}
		if e != nil {
			a.executionMu.Unlock()
			fail(w, http.StatusConflict, e)
			return
		}
		if a.recordReplay != nil {
			replay := a.recordReplay.ReplayState()
			if replay.Running || replay.Stopping || replay.Finalizing {
				stopContext, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				_, _ = a.recordReplay.StopReplay(stopContext)
				cancel()
			}
		}
		a.clearMonkeyTargetPlan()
		a.executionMu.Unlock()
		writeJSON(w, 200, s)
	case "POST":
		r.Body = http.MaxBytesReader(w, r.Body, 128<<10)
		var raw json.RawMessage
		b := struct {
			RequestID string   `json:"requestId"`
			Args      []string `json:"args"`
		}{}
		if e := json.NewDecoder(r.Body).Decode(&raw); e != nil {
			fail(w, 400, e)
			return
		}
		var c runner.RunConfig
		if e := json.Unmarshal(raw, &c); e != nil {
			fail(w, 400, e)
			return
		}
		if c.Package == "" {
			if e := json.Unmarshal(raw, &b); e != nil {
				fail(w, 400, e)
				return
			}
			var e error
			c, e = runner.FromLegacy(b.RequestID, b.Args)
			if e != nil {
				fail(w, 400, e)
				return
			}
		}
		a.executionMu.Lock()
		if a.shuttingDown {
			a.executionMu.Unlock()
			fail(w, http.StatusServiceUnavailable, errors.New("agent is shutting down"))
			return
		}
		if a.explorer != nil {
			explorationState := a.explorer.State()
			if explorationState.Running || explorationState.Stopping || explorationState.Finalizing {
				a.executionMu.Unlock()
				writeJSON(w, http.StatusConflict, map[string]any{"success": false, "error": "intelligent exploration session already active"})
				return
			}
		}
		if a.recordReplay != nil {
			recording, replay := a.recordReplay.RecordingState(), a.recordReplay.ReplayState()
			if recording.Running || recording.Stopping || recording.Finalizing || replay.Running || replay.Stopping || replay.Finalizing {
				a.executionMu.Unlock()
				writeJSON(w, http.StatusConflict, map[string]any{"success": false, "error": "record or replay session already active"})
				return
			}
		}
		plan, e := a.prepareMonkeyTargetPlan(&c)
		if e != nil {
			a.executionMu.Unlock()
			fail(w, http.StatusBadRequest, e)
			return
		}
		s, e := a.runner.Start(c)
		if e == nil && s.Running {
			a.setMonkeyTargetPlan(plan)
		}
		a.executionMu.Unlock()
		if errors.Is(e, runner.ErrAlreadyRunning) || errors.Is(e, runner.ErrRequestConflict) {
			writeJSON(w, 409, s)
			return
		}
		if e != nil {
			fail(w, 400, e)
			return
		}
		writeJSON(w, 202, s)
	default:
		w.Header().Set("Allow", "GET, POST, DELETE")
		fail(w, 405, errors.New("method not allowed"))
	}
}
func (a *API) pushConfig(w http.ResponseWriter, r *http.Request) {
	if !requireControlHeader(w, r) {
		return
	}
	var v map[string]any
	if e := json.NewDecoder(r.Body).Decode(&v); e != nil {
		fail(w, requestErrorStatus(e), e)
		return
	}
	if e := a.store.Replace(v); e != nil {
		fail(w, 500, e)
		return
	}
	writeJSON(w, 200, map[string]any{"success": true})
}
