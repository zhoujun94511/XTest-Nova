package runner

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zhoujun94511/xtest-nova/agent/internal/artifacts"
	"github.com/zhoujun94511/xtest-nova/agent/internal/companioncontrol"
	"github.com/zhoujun94511/xtest-nova/agent/internal/evidence"
	"github.com/zhoujun94511/xtest-nova/agent/internal/execution"
	"github.com/zhoujun94511/xtest-nova/agent/internal/platform"
)

var ErrAlreadyRunning = errors.New("runner already active")
var ErrRequestConflict = errors.New("requestId was already used with different Runner configuration")
var packagePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*(?:\.[A-Za-z][A-Za-z0-9_]*)+$`)
var requestPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)

type RunConfig struct {
	RequestID          string       `json:"requestId,omitempty"`
	Package            string       `json:"package"`
	DurationSeconds    int          `json:"durationSeconds"`
	ThrottleMillis     int          `json:"throttleMillis"`
	Seed               int64        `json:"seed,omitempty"`
	InputCasesPerField int          `json:"inputCasesPerField,omitempty"`
	InputMaxLength     int          `json:"inputMaxLength,omitempty"`
	LowBatteryExit     bool         `json:"lowBatteryExit,omitempty"`
	MinBattery         int          `json:"minBatteryPercent,omitempty"`
	ActivityMode       string       `json:"activityMode,omitempty"`
	RenderFallbackMode string       `json:"renderFallbackMode,omitempty"`
	Activities         []string     `json:"activities,omitempty"`
	TargetActivities   []string     `json:"targetActivities,omitempty"`
	ControlBlacklist   []string     `json:"controlBlacklist,omitempty"`
	TargetCases        []TargetCase `json:"targetCases,omitempty"`
	TargetToken        string       `json:"-"`
	ControlToken       string       `json:"-"`
}

type TargetCase struct {
	Activity string `json:"activity"`
	Task     string `json:"task,omitempty"`
	Case     string `json:"case"`
}

func (c RunConfig) Validate() error {
	if !packagePattern.MatchString(strings.TrimSpace(c.Package)) {
		return fmt.Errorf("invalid Android package name")
	}
	if c.RequestID != "" && !requestPattern.MatchString(c.RequestID) {
		return fmt.Errorf("invalid requestId")
	}
	if c.DurationSeconds < 1 || c.DurationSeconds > 604800 {
		return fmt.Errorf("durationSeconds must be between 1 and 604800")
	}
	if c.ThrottleMillis < 50 || c.ThrottleMillis > 60000 {
		return fmt.Errorf("throttleMillis must be between 50 and 60000")
	}
	if c.InputCasesPerField < 0 || c.InputCasesPerField > 18 {
		return fmt.Errorf("inputCasesPerField must be between 1 and 18 when set")
	}
	if c.InputMaxLength < 0 || c.InputMaxLength > 256 {
		return fmt.Errorf("inputMaxLength must be between 1 and 256 when set")
	}
	if c.LowBatteryExit && (c.MinBattery < 1 || c.MinBattery > 100) {
		return fmt.Errorf("minBatteryPercent must be between 1 and 100")
	}
	if c.ActivityMode != "" && c.ActivityMode != "none" && c.ActivityMode != "allowlist" && c.ActivityMode != "blocklist" {
		return fmt.Errorf("activityMode must be none, allowlist, or blocklist")
	}
	if c.RenderFallbackMode != "" && c.RenderFallbackMode != "off" && c.RenderFallbackMode != "bounded" && c.RenderFallbackMode != "continuous" {
		return fmt.Errorf("renderFallbackMode must be off, bounded, or continuous")
	}
	if len(c.Activities) > 64 || len(c.TargetActivities) > 64 || len(c.ControlBlacklist) > 64 || len(c.TargetCases) > 32 {
		return fmt.Errorf("advanced Monkey lists are limited to 64 entries")
	}
	activityPattern := regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.$]*(?:/[A-Za-z0-9_.$]+)?$`)
	for _, values := range [][]string{c.Activities, c.TargetActivities} {
		for _, value := range values {
			if !activityPattern.MatchString(strings.TrimSpace(value)) {
				return fmt.Errorf("invalid Activity name")
			}
		}
	}
	caseNamePattern := regexp.MustCompile(`^[A-Za-z0-9._-]{1,80}$`)
	for _, target := range c.TargetCases {
		if !activityPattern.MatchString(strings.TrimSpace(target.Activity)) || !caseNamePattern.MatchString(target.Case) || (target.Task != "" && !caseNamePattern.MatchString(target.Task)) {
			return fmt.Errorf("invalid target-page case mapping")
		}
	}
	if len(c.TargetCases) > 0 && !requestPattern.MatchString(c.TargetToken) {
		return fmt.Errorf("target-page case token is required")
	}
	for _, value := range c.ControlBlacklist {
		if value = strings.TrimSpace(value); value == "" || len(value) > 128 || strings.ContainsRune(value, '\x00') {
			return fmt.Errorf("invalid control blacklist entry")
		}
	}
	return nil
}
func FromLegacy(id string, args []string) (RunConfig, error) {
	c := RunConfig{RequestID: id, DurationSeconds: 600, ThrottleMillis: 500}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-p":
			i++
			if i < len(args) {
				c.Package = args[i]
			}
		case "--running-minutes":
			i++
			var n int
			if i >= len(args) {
				return c, fmt.Errorf("missing running minutes")
			}
			if _, e := fmt.Sscanf(args[i], "%d", &n); e != nil {
				return c, e
			}
			c.DurationSeconds = n * 60
		case "--duration-seconds":
			i++
			if i >= len(args) {
				return c, fmt.Errorf("missing duration")
			}
			_, _ = fmt.Sscanf(args[i], "%d", &c.DurationSeconds)
		case "--throttle":
			i++
			if i >= len(args) {
				return c, fmt.Errorf("missing throttle")
			}
			_, _ = fmt.Sscanf(args[i], "%d", &c.ThrottleMillis)
		}
	}
	return c, c.Validate()
}

type State struct {
	RequestID            string                           `json:"requestId,omitempty"`
	Identity             execution.Identity               `json:"identity"`
	Package              string                           `json:"package,omitempty"`
	RenderFallbackMode   string                           `json:"renderFallbackMode,omitempty"`
	Running              bool                             `json:"running"`
	Finalizing           bool                             `json:"finalizing,omitempty"`
	PID                  int                              `json:"pid,omitempty"`
	StartedAt            *time.Time                       `json:"startedAt,omitempty"`
	EndedAt              *time.Time                       `json:"endedAt,omitempty"`
	Events               int64                            `json:"events"`
	LastEventAt          *time.Time                       `json:"lastEventAt,omitempty"`
	CurrentActivity      string                           `json:"currentActivity,omitempty"`
	CurrentScene         string                           `json:"currentScene,omitempty"`
	LastAction           string                           `json:"lastAction,omitempty"`
	ExternalPackage      string                           `json:"externalPackage,omitempty"`
	StopReason           string                           `json:"stopReason,omitempty"`
	FailureType          string                           `json:"failureType,omitempty"`
	ExitCode             int                              `json:"exitCode"`
	Error                string                           `json:"error,omitempty"`
	CrashCount           int                              `json:"crashCount"`
	ANRCount             int                              `json:"anrCount"`
	NativeCrashCount     int                              `json:"nativeCrashCount"`
	AbnormalExitCount    int                              `json:"abnormalExitCount"`
	DiagnosticsAvailable bool                             `json:"diagnosticsAvailable"`
	DiagnosticsComplete  bool                             `json:"diagnosticsComplete"`
	DiagnosticsTruncated bool                             `json:"diagnosticsTruncated,omitempty"`
	GeneralLogTruncated  bool                             `json:"generalLogTruncated,omitempty"`
	IncidentLogTruncated bool                             `json:"incidentLogTruncated,omitempty"`
	DiagnosticSources    map[string]DiagnosticSourceState `json:"diagnosticSources,omitempty"`
	DiagnosticErrors     []string                         `json:"diagnosticErrors,omitempty"`
	ArtifactDir          string                           `json:"artifactDir,omitempty"`
	LogPath              string                           `json:"logPath"`
	LegacyLogPath        string                           `json:"legacyLogPath,omitempty"`
	ManifestPath         string                           `json:"manifestPath,omitempty"`
	CoveragePath         string                           `json:"coveragePath,omitempty"`
	GraphPath            string                           `json:"graphPath,omitempty"`
	LogcatPath           string                           `json:"logcatPath,omitempty"`
	CrashPath            string                           `json:"crashPath,omitempty"`
	ANRPath              string                           `json:"anrPath,omitempty"`
	NativeCrashPath      string                           `json:"nativeCrashPath,omitempty"`
	DiagnosticsPath      string                           `json:"diagnosticsPath,omitempty"`
	ExitInfoPath         string                           `json:"exitInfoPath,omitempty"`
	EvidenceIndexPath    string                           `json:"evidenceIndexPath,omitempty"`
	Exploration          ExplorationMetrics               `json:"exploration"`
	Screenshots          []string                         `json:"screenshots,omitempty"`
	ArtifactErrors       []string                         `json:"artifactErrors,omitempty"`
	OverlaySuppressed    bool                             `json:"overlaySuppressed"`
	OverlayControlError  string                           `json:"overlayControlError,omitempty"`
}

// ActivityState is the lock-only subset used by health checks and UI polling.
// Unlike State, reading it never parses the growing Runner event log.
type ActivityState struct {
	Running    bool   `json:"running"`
	Finalizing bool   `json:"finalizing,omitempty"`
	Error      string `json:"error,omitempty"`
}

type ExplorationMetrics struct {
	States                       int   `json:"states"`
	Edges                        int   `json:"edges"`
	Taps                         int   `json:"taps"`
	LongClicks                   int   `json:"longClicks"`
	Scrolls                      int   `json:"scrolls"`
	ScrollAttempts               int   `json:"scrollAttempts"`
	ScrollProgress               int   `json:"scrollProgress"`
	ScrollStalls                 int   `json:"scrollStalls"`
	Inputs                       int   `json:"inputs"`
	Backtracks                   int   `json:"backtracks"`
	KnownPathReplays             int   `json:"knownPathReplays"`
	HierarchyFallback            int   `json:"hierarchyFallback"`
	FallbackActions              int   `json:"fallbackActions"`
	CoordinateActions            int   `json:"coordinateActions"`
	CoordinateResults            int   `json:"coordinateResults"`
	CoordinateTarget             int   `json:"coordinateTarget"`
	CoordinateExternal           int   `json:"coordinateExternal"`
	CoordinateSpecial            int   `json:"coordinateSpecial"`
	CoordinateUnobserved         int   `json:"coordinateUnobserved"`
	CoordinateZoneBlocks         int   `json:"coordinateZoneBlocks"`
	RenderFallbackDowngrades     int   `json:"renderFallbackDowngrades"`
	CoordinateFallbackSuppressed int   `json:"coordinateFallbackSuppressed"`
	GenerationPingPongBlocks     int   `json:"generationPingPongBlocks"`
	GenerationPingPongWaits      int   `json:"generationPingPongWaits"`
	GenerationSoftRefreshes      int   `json:"generationSoftRefreshes"`
	RenderContextQuarantines     int   `json:"renderContextQuarantines"`
	RelaunchRequests             int   `json:"relaunchRequests"`
	OnboardingExhaustions        int   `json:"onboardingExhaustions"`
	CoordinateRecoveries         int   `json:"coordinateRecoveries"`
	CoordinateRecoveryMillis     int64 `json:"coordinateRecoveryMillis"`
	CoordinateRecoveryMaxMillis  int64 `json:"coordinateRecoveryMaxMillis"`
	CycleDetections              int   `json:"cycleDetections"`
	BlockedEdges                 int   `json:"blockedEdges"`
	UnstableSnapshots            int   `json:"unstableSnapshots"`
}

type ScreenshotSource interface {
	Screenshot(context.Context) ([]byte, error)
}
type Factory interface {
	Command(context.Context, RunConfig) *exec.Cmd
}
type AppProcessFactory struct{ Classpath, StopFile string }

func (f AppProcessFactory) Command(ctx context.Context, c RunConfig) *exec.Cmd {
	inputCases, inputMaxLength := c.InputCasesPerField, c.InputMaxLength
	if inputCases == 0 {
		inputCases = 1
	}
	if inputMaxLength == 0 {
		inputMaxLength = 64
	}
	args := []string{"/system/bin", "com.openatx.xtest.nova.runner.Main", "-p", c.Package, "--request-id", c.RequestID,
		"--duration-seconds", strconv.Itoa(c.DurationSeconds), "--throttle", strconv.Itoa(c.ThrottleMillis), "--stop-file", f.StopFile,
		"--input-cases-per-field", strconv.Itoa(inputCases), "--input-max-length", strconv.Itoa(inputMaxLength),
		"--render-fallback-mode", normalizedRenderFallbackMode(c.RenderFallbackMode)}
	if c.Seed != 0 {
		args = append(args, "--seed", strconv.FormatInt(c.Seed, 10))
	}
	if c.LowBatteryExit {
		args = append(args, "--low-battery-exit", "--min-battery", strconv.Itoa(c.MinBattery))
	}
	if c.ActivityMode != "" && c.ActivityMode != "none" {
		args = append(args, "--activity-mode", c.ActivityMode)
	}
	for _, value := range c.Activities {
		args = append(args, "--activity", value)
	}
	for _, value := range c.TargetActivities {
		args = append(args, "--target-activity", value)
	}
	for _, value := range c.ControlBlacklist {
		args = append(args, "--blocked-control", value)
	}
	if len(c.TargetCases) > 0 {
		args = append(args, "--target-token", c.TargetToken)
		for _, target := range c.TargetCases {
			args = append(args, "--target-case", target.Activity+"|"+target.Task+"|"+target.Case)
		}
	}
	if c.ControlToken != "" {
		args = append(args, "--control-token", c.ControlToken)
	}
	cmd := exec.CommandContext(ctx, "app_process", args...)
	cmd.Env = append(os.Environ(), "CLASSPATH="+f.Classpath)
	return cmd
}

func normalizedRenderFallbackMode(value string) string {
	if value == "" {
		return "bounded"
	}
	return value
}

type Manager struct {
	mu                sync.Mutex
	factory           Factory
	stopFile, logPath string
	artifactRoot      string
	screenshot        ScreenshotSource
	diagnostics       diagnosticSource
	activitySource    interface {
		Activities(context.Context, string) ([]string, error)
	}
	command            *exec.Cmd
	controlToken       string
	requestFingerprint string
	runGeneration      uint64
	stopRequested      bool
	cancel             context.CancelFunc
	finalizationDone   chan struct{}
	state              State
}

const (
	coverageDiscoveryTimeout       = 10 * time.Second
	coverageFallbackTimeout        = 3 * time.Second
	diagnosticsCollectionTimeout   = 15 * time.Second
	finalScreenshotTimeout         = 3 * time.Second
	overlayRestoreTimeout          = 3 * time.Second
	finalizationFileOperationSlack = 10 * time.Second

	// FinalizationTimeout strictly covers every bounded collection phase plus
	// file hashing, evidence indexing and manifest persistence. Shutdown callers
	// must not use a shorter budget while run artifacts are still being sealed.
	FinalizationTimeout = coverageDiscoveryTimeout + coverageFallbackTimeout +
		diagnosticsCollectionTimeout + finalScreenshotTimeout +
		overlayRestoreTimeout + finalizationFileOperationSlack
)

func runConfigFingerprint(config RunConfig) string {
	config.TargetToken = ""
	config.ControlToken = ""
	encoded, _ := json.Marshal(config)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func NewWithArtifacts(factory Factory, stopFile, legacyLogPath, artifactRoot string, screenshot ScreenshotSource, activitySources ...interface {
	Activities(context.Context, string) ([]string, error)
}) *Manager {
	done := make(chan struct{})
	close(done)
	manager := &Manager{factory: factory, stopFile: stopFile, logPath: legacyLogPath, artifactRoot: artifactRoot,
		screenshot: screenshot, diagnostics: osDiagnosticSource{}, finalizationDone: done, state: State{LogPath: legacyLogPath, LegacyLogPath: legacyLogPath}}
	if len(activitySources) > 0 {
		manager.activitySource = activitySources[0]
	}
	return manager
}
func (m *Manager) State() State {
	m.mu.Lock()
	state := m.state
	m.mu.Unlock()
	if !state.Running || state.LogPath == "" {
		return state
	}
	_, _, metrics, live, err := parseEventLog(state.LogPath)
	if err != nil {
		return state
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state.Running && m.state.RequestID == state.RequestID {
		m.state.Events = live.Events
		m.state.Exploration = metrics
		m.state.LastEventAt = live.LastEventAt
		m.state.CurrentActivity = live.CurrentActivity
		m.state.CurrentScene = live.CurrentScene
		m.state.LastAction = live.LastAction
		m.state.ExternalPackage = live.ExternalPackage
	}
	return m.state
}

func (m *Manager) Activity() ActivityState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return ActivityState{Running: m.state.Running, Finalizing: m.state.Finalizing, Error: m.state.Error}
}

func (m *Manager) AuthorizeControlToken(token string) (State, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	valid := m.state.Running && token != "" && len(token) == len(m.controlToken) && subtle.ConstantTimeCompare([]byte(token), []byte(m.controlToken)) == 1
	return m.state, valid
}

func newControlToken() (string, error) {
	value := make([]byte, 24)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func (m *Manager) Start(c RunConfig) (State, error) {
	if e := c.Validate(); e != nil {
		return m.State(), e
	}
	c.RenderFallbackMode = normalizedRenderFallbackMode(c.RenderFallbackMode)
	if c.RequestID == "" {
		c.RequestID = fmt.Sprintf("nova-%d", time.Now().UnixNano())
	}
	fingerprint := runConfigFingerprint(c)
	m.mu.Lock()
	if m.state.Running || m.state.Finalizing {
		if c.RequestID != "" && c.RequestID == m.state.RequestID {
			s := m.state
			m.mu.Unlock()
			if fingerprint != m.requestFingerprint {
				return s, ErrRequestConflict
			}
			return s, nil
		}
		s := m.state
		m.mu.Unlock()
		return s, ErrAlreadyRunning
	}
	controlToken, tokenErr := newControlToken()
	if tokenErr != nil {
		s := m.state
		m.mu.Unlock()
		return s, fmt.Errorf("create Runner control token: %w", tokenErr)
	}
	c.ControlToken = controlToken
	if c.RequestID == m.state.RequestID && m.state.StartedAt != nil {
		s := m.state
		m.mu.Unlock()
		if fingerprint != m.requestFingerprint {
			return s, ErrRequestConflict
		}
		return s, nil
	}
	_ = os.Remove(m.stopFile)
	session, e := m.openSession(c)
	if e != nil {
		s := m.state
		m.mu.Unlock()
		return s, e
	}
	ctx, cancel := context.WithCancel(context.Background())
	now := time.Now().UTC()
	if session.dir != "" {
		_, supportsSessionCapture := m.diagnostics.(interface {
			CollectIncidents(context.Context, string, time.Time) diagnosticCapture
		})
		if supportsSessionCapture {
			session.diagnosticStream, session.diagnosticStreamError = startSessionLogCapture(ctx, c.Package)
		}
	}
	cmd := m.factory.Command(ctx, c)
	cmd.Stdout = session.writer
	cmd.Stderr = session.writer
	overlayContext, cancelOverlay := context.WithTimeout(ctx, 3*time.Second)
	overlayErr := companioncontrol.SuppressOverlay(overlayContext, platform.OSExecutor{})
	cancelOverlay()
	if e = cmd.Start(); e != nil {
		cancel()
		if session.diagnosticStream != nil {
			_, _, _ = session.diagnosticStream.stop()
		}
		session.close()
		if session.dir != "" {
			_ = os.RemoveAll(session.dir)
		}
		if overlayErr == nil {
			restoreContext, cancelRestore := context.WithTimeout(context.Background(), 3*time.Second)
			_ = companioncontrol.RestoreOverlay(restoreContext, platform.OSExecutor{})
			cancelRestore()
		}
		s := m.state
		m.mu.Unlock()
		return s, e
	}
	m.command = cmd
	m.controlToken = controlToken
	m.requestFingerprint = fingerprint
	m.stopRequested = false
	m.cancel = cancel
	m.finalizationDone = make(chan struct{})
	m.runGeneration++
	identity := execution.NewIdentity("runner", c.RequestID, m.runGeneration)
	m.state = State{RequestID: c.RequestID, Identity: identity, Package: c.Package, RenderFallbackMode: c.RenderFallbackMode, Running: true, PID: cmd.Process.Pid,
		StartedAt: &now, LogPath: session.logPath, LegacyLogPath: session.legacyLogPath,
		ArtifactDir: session.dir, ManifestPath: session.manifestPath, CoveragePath: session.coveragePath, GraphPath: session.graphPath,
		LogcatPath: session.logcatPath, CrashPath: session.crashPath, ANRPath: session.anrPath,
		NativeCrashPath: session.nativeCrashPath, DiagnosticsPath: session.diagnosticsPath, ExitInfoPath: session.exitInfoPath}
	if overlayErr == nil {
		m.state.OverlaySuppressed = true
	} else {
		m.state.OverlayControlError = overlayErr.Error()
	}
	if session.dir != "" && m.screenshot != nil {
		time.Sleep(350 * time.Millisecond)
		if path, captureErr := m.captureScreenshot(session.dir, "start.png"); captureErr == nil {
			m.state.Screenshots = append(m.state.Screenshots, path)
		} else {
			m.state.ArtifactErrors = append(m.state.ArtifactErrors, "start screenshot: "+captureErr.Error())
		}
	}
	state := m.state
	m.mu.Unlock()
	_ = writeManifest(session.manifestPath, c, state)
	go m.wait(cmd, cancel, session, c, overlayErr == nil)
	return state, nil
}
func (m *Manager) wait(cmd *exec.Cmd, cancel context.CancelFunc, session *runSession, config RunConfig, overlaySuppressed bool) {
	e := cmd.Wait()
	streamLog, streamTruncated, streamErr := "", false, session.diagnosticStreamError
	if session.diagnosticStream != nil {
		streamLog, streamTruncated, streamErr = session.diagnosticStream.stop()
	}
	cancel()
	session.close()
	now := time.Now().UTC()
	m.mu.Lock()
	if m.command != cmd {
		m.mu.Unlock()
		return
	}
	state := m.state
	stopRequested := m.stopRequested
	state.Running = false
	state.Finalizing = true
	state.PID = 0
	state.EndedAt = &now
	m.state = state
	m.controlToken = ""
	m.cancel = nil
	m.mu.Unlock()
	summary, activities, explorationMetrics, live, parseErr := parseEventLog(session.logPath)
	if parseErr != nil {
		state.ArtifactErrors = append(state.ArtifactErrors, "event log: "+parseErr.Error())
	} else {
		state.Events = summary.Events
		if state.Events == 0 {
			state.Events = live.Events
		}
		state.Exploration = explorationMetrics
		state.LastEventAt = live.LastEventAt
		state.CurrentActivity = live.CurrentActivity
		state.CurrentScene = live.CurrentScene
		state.LastAction = live.LastAction
		state.ExternalPackage = live.ExternalPackage
		state.StopReason = summary.State
	}
	if state.StopReason == "" {
		state.StopReason = "unexpected_exit"
	}
	if e != nil {
		state.Error = e.Error()
		var exitError *exec.ExitError
		if errors.As(e, &exitError) {
			state.ExitCode = exitError.ExitCode()
		} else {
			state.ExitCode = -1
		}
	}
	if stopRequested {
		// A bounded forced kill is an implementation detail of cooperative
		// Stop, not an unexpected test failure. Diagnostics below may still
		// supersede this when they contain a real target-app incident.
		state.StopReason, state.ExitCode, state.Error = "stopped", 0, ""
	}
	if session.dir != "" {
		if coverageErr := writeCoverage(session.coveragePath, config.Package, activities, m.activitySource); coverageErr != nil {
			state.ArtifactErrors = append(state.ArtifactErrors, "coverage: "+coverageErr.Error())
		}
		if graphErr := writeExplorationSummary(session.graphPath, config.Package, explorationMetrics); graphErr != nil {
			state.ArtifactErrors = append(state.ArtifactErrors, "exploration graph: "+graphErr.Error())
		}
		// Finalization runs asynchronously from Stop. Allow the ordinary, crash,
		// events and dumpsys sources to finish without coupling API latency to
		// diagnostic collection on slower devices.
		diagnosticContext, cancelDiagnostics := context.WithTimeout(context.Background(), diagnosticsCollectionTimeout)
		var capture diagnosticCapture
		if session.diagnosticStream != nil && streamErr == nil {
			if source, ok := m.diagnostics.(interface {
				CollectIncidents(context.Context, string, time.Time) diagnosticCapture
			}); ok {
				capture = source.CollectIncidents(diagnosticContext, config.Package, *state.StartedAt)
			} else {
				capture = m.diagnostics.Collect(diagnosticContext, config.Package, *state.StartedAt)
			}
			capture.Logcat, capture.LogcatAvailable = streamLog, true
			if capture.Sources == nil {
				capture.Sources = map[string]DiagnosticSourceState{}
			}
			capture.Sources["general_log"] = DiagnosticSourceState{Available: true, Truncated: streamTruncated}
			capture.Truncated = capture.Truncated || streamTruncated
		} else {
			capture = m.diagnostics.Collect(diagnosticContext, config.Package, *state.StartedAt)
			if streamErr != nil {
				capture.Errors = append(capture.Errors, "session logcat: "+streamErr.Error())
			}
		}
		cancelDiagnostics()
		diagnostics := analyzeDiagnostics(capture, config.Package, *state.StartedAt)
		if windows, windowErr := parseRelaunchWindows(session.logPath); windowErr == nil {
			applyRelaunchExitCorrelation(&diagnostics, windows)
		}
		state.CrashCount = totalIncidentOccurrences(diagnostics.Crashes)
		state.ANRCount = totalIncidentOccurrences(diagnostics.ANRs)
		state.NativeCrashCount = totalIncidentOccurrences(diagnostics.NativeCrashes)
		state.AbnormalExitCount = diagnostics.AbnormalExits
		state.DiagnosticsAvailable = diagnostics.LogcatAvailable || diagnostics.Sources["crash_buffer"].Available || diagnostics.Sources["event_buffer"].Available || diagnostics.Sources["exit_info"].Available
		state.DiagnosticsComplete = diagnostics.Complete
		state.DiagnosticsTruncated = diagnostics.Truncated
		state.GeneralLogTruncated = diagnostics.GeneralTruncated
		state.IncidentLogTruncated = diagnostics.IncidentTruncated
		state.DiagnosticSources = diagnostics.Sources
		state.DiagnosticErrors = diagnostics.Errors
		if state.CrashCount > 0 || state.NativeCrashCount > 0 {
			state.FailureType, state.StopReason = "app_crash", "app_crash"
		} else if state.ANRCount > 0 {
			state.FailureType, state.StopReason = "app_anr", "app_anr"
		} else if state.AbnormalExitCount > 0 {
			state.FailureType, state.StopReason = "abnormal_exit", "abnormal_exit"
		}
		logcatText := fmt.Sprintf("package=%s\nstartedAt=%s\navailable=%t\ntruncated=%t\nredacted=true\nretention=head-tail\n\n%s", config.Package, state.StartedAt.UTC().Format(time.RFC3339Nano), diagnostics.LogcatAvailable, diagnostics.GeneralTruncated, diagnostics.ScopedLogcat)
		nativeText := fmt.Sprintf("package=%s\nstartedAt=%s\n\n%s", config.Package, state.StartedAt.UTC().Format(time.RFC3339Nano), diagnostics.NativeLog)
		for label, artifact := range map[string]struct {
			path    string
			content string
		}{
			"logcat":       {session.logcatPath, logcatText},
			"native crash": {session.nativeCrashPath, nativeText},
			"diagnostics":  {session.diagnosticsPath, diagnostics.Diagnostics},
		} {
			if err := writeAtomic(artifact.path, []byte(artifact.content), 0o644); err != nil {
				state.ArtifactErrors = append(state.ArtifactErrors, label+": "+err.Error())
			}
		}
		incidentAvailable := diagnostics.Sources["crash_buffer"].Available || diagnostics.Sources["event_buffer"].Available || diagnostics.LogcatAvailable || diagnostics.Sources["exit_info"].Available
		if err := writeDiagnosticReport(session.crashPath, "xtest-crash/v1", config.Package, incidentAvailable, diagnostics.Complete, diagnostics.IncidentTruncated, append(diagnostics.Crashes, diagnostics.NativeCrashes...), diagnostics.Sources, diagnostics.Errors); err != nil {
			state.ArtifactErrors = append(state.ArtifactErrors, "crash report: "+err.Error())
		}
		if err := writeDiagnosticReport(session.anrPath, "xtest-anr/v1", config.Package, incidentAvailable, diagnostics.Complete, diagnostics.IncidentTruncated, diagnostics.ANRs, diagnostics.Sources, diagnostics.Errors); err != nil {
			state.ArtifactErrors = append(state.ArtifactErrors, "ANR report: "+err.Error())
		}
		if err := writeExitInfoReport(session.exitInfoPath, config.Package, diagnostics); err != nil {
			state.ArtifactErrors = append(state.ArtifactErrors, "exit info report: "+err.Error())
		}
		name := "finish.png"
		if state.FailureType != "" || state.StopReason == "target_case_failed" || state.StopReason == "unexpected_exit" || state.ExitCode != 0 {
			name = "failure.png"
		}
		if path, captureErr := m.captureScreenshot(session.dir, name); captureErr == nil {
			state.Screenshots = append(state.Screenshots, path)
		} else if m.screenshot != nil {
			state.ArtifactErrors = append(state.ArtifactErrors, "final screenshot: "+captureErr.Error())
		}
		// The manifest is the final derived artifact. Its persisted state must
		// describe the completed session rather than the in-memory transition
		// used while the files above were still being generated.
		state.Finalizing = false
		evidencePaths := []string{session.logPath, session.coveragePath, session.graphPath, session.logcatPath, session.crashPath, session.anrPath, session.nativeCrashPath, session.diagnosticsPath, session.exitInfoPath}
		evidencePaths = append(evidencePaths, state.Screenshots...)
		index := evidence.Build(session.dir, config.Package, "", state.Identity, evidencePaths)
		if evidenceErr := evidence.Write(session.evidencePath, index); evidenceErr != nil {
			state.ArtifactErrors = append(state.ArtifactErrors, "evidence index: "+evidenceErr.Error())
		} else {
			state.EvidenceIndexPath = filepath.ToSlash(session.evidencePath)
		}
		if manifestErr := writeManifest(session.manifestPath, config, state); manifestErr != nil {
			state.ArtifactErrors = append(state.ArtifactErrors, "manifest: "+manifestErr.Error())
		}
	}
	var restoreErr error
	if overlaySuppressed {
		restoreContext, cancelRestore := context.WithTimeout(context.Background(), overlayRestoreTimeout)
		restoreErr = companioncontrol.RestoreOverlay(restoreContext, platform.OSExecutor{})
		cancelRestore()
	}
	state.OverlaySuppressed = false
	if restoreErr != nil && state.OverlayControlError == "" {
		state.OverlayControlError = restoreErr.Error()
	}
	m.mu.Lock()
	if m.command != cmd {
		m.mu.Unlock()
		return
	}
	state.Finalizing = false
	m.stopRequested = false
	m.state = state
	m.command = nil
	close(m.finalizationDone)
	m.mu.Unlock()
}

const maxLegacyLogBytes int64 = 32 << 20

type runSession struct {
	dir, logPath, legacyLogPath, manifestPath, coveragePath, graphPath, evidencePath string
	logcatPath, crashPath, anrPath, nativeCrashPath, diagnosticsPath, exitInfoPath   string
	writer                                                                           io.Writer
	closers                                                                          []io.Closer
	diagnosticStream                                                                 *sessionLogCapture
	diagnosticStreamError                                                            error
}

func (s *runSession) close() {
	for _, closer := range s.closers {
		_ = closer.Close()
	}
}

func (m *Manager) openSession(config RunConfig) (*runSession, error) {
	session := &runSession{legacyLogPath: m.logPath}
	if m.artifactRoot != "" {
		dir, err := artifacts.NewDeviceSession(m.artifactRoot, config.Package, "Monkey")
		if err != nil {
			return nil, err
		}
		session.dir = dir
		session.logPath = filepath.Join(dir, "events.jsonl")
		session.manifestPath = filepath.Join(dir, "run.json")
		session.coveragePath = filepath.Join(dir, "activity_coverage.json")
		session.graphPath = filepath.Join(dir, "exploration_graph.json")
		session.logcatPath = filepath.Join(dir, "logcat.txt")
		session.crashPath = filepath.Join(dir, "crash.json")
		session.anrPath = filepath.Join(dir, "anr.json")
		session.nativeCrashPath = filepath.Join(dir, "native_crash.txt")
		session.diagnosticsPath = filepath.Join(dir, "diagnostics.txt")
		session.exitInfoPath = filepath.Join(dir, "exit_info.json")
		session.evidencePath = filepath.Join(dir, "evidence.json")
		logFile, err := os.OpenFile(session.logPath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o644)
		if err != nil {
			_ = os.RemoveAll(dir)
			return nil, err
		}
		session.writer = logFile
		session.closers = append(session.closers, logFile)
	} else {
		session.logPath = m.logPath
	}
	if m.logPath != "" {
		if err := rotateLegacyLog(m.logPath); err != nil {
			session.close()
			if session.dir != "" {
				_ = os.RemoveAll(session.dir)
			}
			return nil, err
		}
		legacy, err := os.OpenFile(m.logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			session.close()
			if session.dir != "" {
				_ = os.RemoveAll(session.dir)
			}
			return nil, err
		}
		if session.writer == nil {
			session.writer = legacy
		} else {
			session.writer = io.MultiWriter(session.writer, legacy)
		}
		session.closers = append(session.closers, legacy)
	}
	if session.writer == nil {
		return nil, errors.New("runner log destination is empty")
	}
	return session, nil
}

func rotateLegacyLog(path string) error {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil || info.Size() < maxLegacyLogBytes {
		return err
	}
	backup := path + ".1"
	if err = os.Remove(backup); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(path, backup)
}

type eventRecord struct {
	State          string `json:"state"`
	RequestID      string `json:"requestId"`
	Package        string `json:"package"`
	Activity       string `json:"activity"`
	Events         int64  `json:"events"`
	Scene          string `json:"scene"`
	From           string `json:"from"`
	Action         string `json:"action"`
	To             string `json:"to"`
	Node           string `json:"node"`
	Outcome        string `json:"outcome"`
	SpecialKind    string `json:"specialKind"`
	RecoveryMillis int64  `json:"recoveryMillis"`
	Recovered      bool   `json:"recovered"`
	Reason         string `json:"reason"`
	Time           int64  `json:"time"`
}

type liveState struct {
	Events          int64
	LastEventAt     *time.Time
	CurrentActivity string
	CurrentScene    string
	LastAction      string
	ExternalPackage string
}

func parseEventLog(path string) (eventRecord, []string, ExplorationMetrics, liveState, error) {
	file, err := os.Open(path)
	if err != nil {
		return eventRecord{}, nil, ExplorationMetrics{}, liveState{}, err
	}
	defer func() { _ = file.Close() }()
	activities := map[string]bool{}
	scenes := map[string]bool{}
	edges := map[string]bool{}
	var summary eventRecord
	var metrics ExplorationMetrics
	var live liveState
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64<<10), 1<<20)
	for scanner.Scan() {
		var event eventRecord
		if json.Unmarshal(scanner.Bytes(), &event) != nil {
			continue
		}
		if event.Activity != "" {
			activities[event.Activity] = true
			live.CurrentActivity = event.Activity
			if slash := strings.IndexByte(event.Activity, '/'); slash > 0 {
				if strings.HasPrefix(event.Activity, event.Package+"/") {
					live.ExternalPackage = ""
				} else {
					live.ExternalPackage = event.Activity[:slash]
				}
			}
		}
		if event.Time > 0 {
			value := time.UnixMilli(event.Time).UTC()
			live.LastEventAt = &value
		}
		switch event.State {
		case "scene":
			if event.Scene != "" {
				scenes[event.Scene] = true
				live.CurrentScene = event.Scene
			}
		case "scene_transition":
			if event.From != "" && event.Action != "" && event.To != "" {
				edges[event.From+"|"+event.Action+"|"+event.To] = true
			}
		case "node_tap":
			metrics.Taps++
			live.Events++
			live.LastAction = "tap|" + event.Node
		case "node_long_click":
			metrics.LongClicks++
			live.Events++
			live.LastAction = "long|" + event.Node
		case "node_scroll":
			metrics.Scrolls++
			metrics.ScrollAttempts++
			live.Events++
			live.LastAction = "scroll|" + event.Node
		case "scroll_progress":
			metrics.ScrollProgress++
		case "scroll_stall":
			metrics.ScrollStalls++
		case "node_input":
			metrics.Inputs++
			live.Events++
			live.LastAction = "input|" + event.Node
		case "dfs_backtrack":
			metrics.Backtracks++
			live.Events++
			live.LastAction = "back"
		case "graph_path":
			metrics.KnownPathReplays++
		case "hierarchy_fallback":
			metrics.HierarchyFallback++
		case "fallback_action":
			metrics.FallbackActions++
			live.Events++
			live.LastAction = "fallback"
		case "coordinate_action":
			metrics.CoordinateActions++
			live.LastAction = "coordinate"
		case "coordinate_action_result":
			metrics.CoordinateResults++
			if event.Outcome == "target" {
				metrics.CoordinateTarget++
			} else if event.Outcome == "external" {
				metrics.CoordinateExternal++
			} else if event.Outcome == "special" {
				metrics.CoordinateSpecial++
			} else if event.Outcome == "unobserved" {
				metrics.CoordinateUnobserved++
			}
		case "coordinate_zone_blocked":
			metrics.CoordinateZoneBlocks++
		case "render_fallback_downgraded":
			metrics.RenderFallbackDowngrades++
		case "coordinate_fallback_suppressed":
			metrics.CoordinateFallbackSuppressed++
		case "generation_pingpong_blocked":
			metrics.GenerationPingPongBlocks++
		case "generation_pingpong_wait":
			metrics.GenerationPingPongWaits++
		case "exploration_cycle_refreshed":
			metrics.GenerationSoftRefreshes++
		case "render_context_quarantine":
			metrics.RenderContextQuarantines++
		case "relaunch_requested":
			metrics.RelaunchRequests++
		case "onboarding_exhaustion_guard":
			metrics.OnboardingExhaustions++
		case "coordinate_action_recovery":
			if event.Recovered {
				metrics.CoordinateRecoveries++
				metrics.CoordinateRecoveryMillis += event.RecoveryMillis
				if event.RecoveryMillis > metrics.CoordinateRecoveryMaxMillis {
					metrics.CoordinateRecoveryMaxMillis = event.RecoveryMillis
				}
			}
		case "cycle_detected":
			metrics.CycleDetections++
		case "edge_blocked":
			metrics.BlockedEdges++
		case "unstable_snapshot":
			metrics.UnstableSnapshots++
		case "completed", "stopped", "low_battery", "target_case_failed", "graph_exhausted", "hierarchy_fallback_exhausted", "onboarding_exhausted", "special_handling_failed", "special_unhandled", "external_package":
			summary = event
		}
	}
	metrics.States = len(scenes)
	metrics.Edges = len(edges)
	values := make([]string, 0, len(activities))
	for activity := range activities {
		values = append(values, activity)
	}
	sort.Strings(values)
	return summary, values, metrics, live, scanner.Err()
}

func writeExplorationSummary(path, packageName string, metrics ExplorationMetrics) error {
	value := struct {
		SchemaVersion string             `json:"schemaVersion"`
		Package       string             `json:"package"`
		Metrics       ExplorationMetrics `json:"metrics"`
	}{"xtest-monkey-graph/v1", packageName, metrics}
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(path, append(content, '\n'), 0o644)
}

func (m *Manager) captureScreenshot(directory, name string) (string, error) {
	if m.screenshot == nil {
		return "", errors.New("screenshot source unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), finalScreenshotTimeout)
	defer cancel()
	data, err := m.screenshot.Screenshot(ctx)
	if err != nil {
		return "", err
	}
	if len(data) < 8 || string(data[1:4]) != "PNG" {
		return "", errors.New("screenshot is not PNG")
	}
	path := filepath.Join(directory, name)
	return path, writeAtomic(path, data, 0o644)
}

func writeCoverage(path, packageName string, activities []string, activitySource interface {
	Activities(context.Context, string) ([]string, error)
}) error {
	targetActivities, externalActivities := make([]string, 0, len(activities)), make([]string, 0)
	for _, activity := range activities {
		if strings.HasPrefix(activity, packageName+"/") {
			targetActivities = append(targetActivities, activity)
		} else if activity != "" {
			externalActivities = append(externalActivities, activity)
		}
	}
	var declared []string
	var discoveryErr error
	if activitySource == nil {
		discoveryErr = errors.New("android PackageManager activity source unavailable")
	} else {
		discoveryContext, cancel := context.WithTimeout(context.Background(), coverageDiscoveryTimeout)
		declared, discoveryErr = activitySource.Activities(discoveryContext, packageName)
		cancel()
	}
	basis := "android-package-manager"
	if discoveryErr != nil || len(declared) == 0 {
		declared = parseDeclaredActivities(packageName, packageDump(packageName))
		declared = mergeKnownActivities(declared, targetActivities)
		basis = "package-resolver-plus-observed-lower-bound"
	}
	observedSet := make(map[string]struct{}, len(targetActivities))
	for _, activity := range targetActivities {
		observedSet[activity] = struct{}{}
	}
	covered, uncovered := make([]string, 0), make([]string, 0)
	for _, activity := range declared {
		if _, ok := observedSet[activity]; ok {
			covered = append(covered, activity)
		} else {
			uncovered = append(uncovered, activity)
		}
	}
	var percent *float64
	var coverageValue float64
	if len(declared) > 0 {
		coverageValue = float64(len(covered)) * 100 / float64(len(declared))
		percent = &coverageValue
	}
	unexpected := make([]string, 0)
	declaredSet := make(map[string]struct{}, len(declared))
	for _, activity := range declared {
		declaredSet[activity] = struct{}{}
	}
	for _, activity := range targetActivities {
		if _, ok := declaredSet[activity]; !ok {
			unexpected = append(unexpected, activity)
		}
	}
	value := struct {
		SchemaVersion      string   `json:"schemaVersion"`
		Package            string   `json:"package"`
		Observed           int      `json:"observed"`
		Activities         []string `json:"activities"`
		ExternalObserved   []string `json:"externalObserved,omitempty"`
		CoverageBasis      string   `json:"coverageBasis"`
		Declared           int      `json:"declared,omitempty"`
		Covered            []string `json:"covered,omitempty"`
		Uncovered          []string `json:"uncovered,omitempty"`
		UnexpectedObserved []string `json:"unexpectedObserved,omitempty"`
		CoveragePercent    *float64 `json:"coveragePercent,omitempty"`
	}{"xtest-monkey-coverage/v4", packageName, len(targetActivities), targetActivities, externalActivities, basis, len(declared), covered, uncovered, unexpected, percent}
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err = writeAtomic(path, append(content, '\n'), 0o644); err != nil {
		return err
	}
	textPath := filepath.Join(filepath.Dir(path), "activity_coverage.txt")
	lines := []string{fmt.Sprintf("package=%s", packageName), fmt.Sprintf("observed=%d", len(targetActivities))}
	lines = append(lines, "coverageBasis="+basis)
	if len(declared) > 0 {
		lines = append(lines, fmt.Sprintf("covered=%d/%d", len(covered), len(declared)), fmt.Sprintf("coverage=%.6f%%", coverageValue))
		for _, activity := range uncovered {
			lines = append(lines, "uncovered="+activity)
		}
	}
	for _, activity := range externalActivities {
		lines = append(lines, "externalObserved="+activity)
	}
	lines = append(lines, targetActivities...)
	return writeAtomic(textPath, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

func normalizeActivity(packageName, name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if strings.HasPrefix(name, ".") {
		return packageName + "/" + name
	}
	if !strings.Contains(name, ".") {
		name = packageName + "." + name
	}
	if strings.HasPrefix(name, packageName+".") {
		return packageName + "/." + strings.TrimPrefix(name, packageName+".")
	}
	return packageName + "/" + name
}

func mergeKnownActivities(resolved, observed []string) []string {
	seen := make(map[string]struct{}, len(resolved)+len(observed))
	for _, values := range [][]string{resolved, observed} {
		for _, activity := range values {
			if strings.TrimSpace(activity) != "" {
				seen[activity] = struct{}{}
			}
		}
	}
	result := make([]string, 0, len(seen))
	for activity := range seen {
		result = append(result, activity)
	}
	sort.Strings(result)
	return result
}

func packageDump(packageName string) string {
	ctx, cancel := context.WithTimeout(context.Background(), coverageFallbackTimeout)
	defer cancel()
	output, err := exec.CommandContext(ctx, "dumpsys", "package", packageName).Output()
	if err != nil {
		return ""
	}
	return string(output)
}

func parseDeclaredActivities(packageName, dump string) []string {
	if !packagePattern.MatchString(packageName) || dump == "" {
		return nil
	}
	pattern := regexp.MustCompile(regexp.QuoteMeta(packageName) + `/[A-Za-z0-9_.$]+`)
	seen := map[string]struct{}{}
	for _, activity := range pattern.FindAllString(dump, -1) {
		seen[activity] = struct{}{}
	}
	values := make([]string, 0, len(seen))
	for activity := range seen {
		values = append(values, activity)
	}
	sort.Strings(values)
	return values
}

func writeManifest(path string, config RunConfig, state State) error {
	if path == "" {
		return nil
	}
	hashes := map[string]string{}
	paths := append([]string{state.LogPath, state.CoveragePath, state.GraphPath, state.LogcatPath,
		state.CrashPath, state.ANRPath, state.NativeCrashPath, state.DiagnosticsPath,
		state.ExitInfoPath,
		filepath.Join(state.ArtifactDir, "activity_coverage.txt")}, state.Screenshots...)
	for _, artifactPath := range paths {
		if artifactPath == "" {
			continue
		}
		data, err := os.ReadFile(artifactPath)
		if err != nil {
			continue
		}
		sum := sha256.Sum256(data)
		hashes[filepath.Base(artifactPath)] = strings.ToUpper(hex.EncodeToString(sum[:]))
	}
	state.Identity.OwnerToken = ""
	value := struct {
		SchemaVersion string            `json:"schemaVersion"`
		Config        RunConfig         `json:"config"`
		State         State             `json:"state"`
		SHA256        map[string]string `json:"sha256,omitempty"`
	}{"xtest-monkey-run/v1", config, state, hashes}
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(path, append(content, '\n'), 0o644)
}

func writeAtomic(path string, content []byte, mode os.FileMode) error {
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, content, mode); err != nil {
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return nil
}

func (m *Manager) Stop() (State, error) {
	m.mu.Lock()
	if !m.state.Running {
		s := m.state
		m.mu.Unlock()
		return s, nil
	}
	cmd, cancel := m.command, m.cancel
	m.stopRequested = true
	m.mu.Unlock()
	if e := os.WriteFile(m.stopFile, []byte("stop\\n"), 0644); e != nil {
		m.mu.Lock()
		if m.command == cmd {
			m.stopRequested = false
		}
		m.mu.Unlock()
		return m.State(), e
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !m.State().Running {
			return m.State(), nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	if cancel != nil {
		cancel()
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !m.State().Running {
			return m.State(), nil
		}
		time.Sleep(25 * time.Millisecond)
	}
	return m.State(), errors.New("runner did not converge to stopped state after forced termination")
}

// StopOwned is the external control-plane stop. Internal lifecycle cleanup may
// still call Stop directly, while a delayed client cannot stop a newer run.
func (m *Manager) StopOwned(sessionID, ownerToken string) (State, error) {
	m.mu.Lock()
	state := m.state
	m.mu.Unlock()
	if !state.Running {
		return state, nil
	}
	if state.Identity.SessionID != sessionID || len(ownerToken) != len(state.Identity.OwnerToken) || subtle.ConstantTimeCompare([]byte(ownerToken), []byte(state.Identity.OwnerToken)) != 1 {
		return state, execution.ErrOwnerMismatch
	}
	return m.Stop()
}

// WaitFinalized waits until the current Runner process and its derived artifact
// generation have both completed. Stop intentionally does not perform this wait
// so callers that only stop a run can observe the finalizing state without a
// false process-stop timeout.
func (m *Manager) WaitFinalized(ctx context.Context) (State, error) {
	m.mu.Lock()
	done := m.finalizationDone
	state := m.state
	m.mu.Unlock()
	if done == nil || (!state.Running && !state.Finalizing) {
		return state, nil
	}
	select {
	case <-done:
		return m.State(), nil
	case <-ctx.Done():
		return m.State(), fmt.Errorf("wait for Runner artifact finalization: %w", ctx.Err())
	}
}
