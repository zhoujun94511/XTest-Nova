package recordreplay

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/zhoujun94511/xtest-nova/agent/internal/execution"
	"github.com/zhoujun94511/xtest-nova/agent/internal/platform"
	"github.com/zhoujun94511/xtest-nova/agent/internal/scrcpy"
	"github.com/zhoujun94511/xtest-nova/agent/internal/touchreader"
)

type CaptureSource interface {
	Capture(context.Context, func(time.Time, touchreader.Event)) error
}

type ForegroundSource interface {
	ForegroundPackage(context.Context) (string, error)
}

type ControlInjector interface {
	InjectText(context.Context, string) error
	InjectEvents(context.Context, []scrcpy.Event) error
}

type ScreenshotSource interface {
	Screenshot(context.Context) ([]byte, error)
}

type HierarchySource interface {
	Hierarchy(context.Context) (string, error)
}

type screenshotAssertionError struct {
	data       []byte
	expected   string
	actual     string
	difference int
	maximum    int
}

func (e *screenshotAssertionError) Error() string {
	return fmt.Sprintf("screenshot assertion failed: distance %d exceeds %d", e.difference, e.maximum)
}

type RecordingConfig struct {
	RequestID      string  `json:"requestId,omitempty"`
	Package        string  `json:"package"`
	Task           string  `json:"task,omitempty"`
	Name           string  `json:"name"`
	ExcludedBounds *Bounds `json:"excludedBounds,omitempty"`
}

type RecordingState struct {
	RequestID         string             `json:"requestId,omitempty"`
	Identity          execution.Identity `json:"identity"`
	Running           bool               `json:"running"`
	Stopping          bool               `json:"stopping,omitempty"`
	Finalizing        bool               `json:"finalizing,omitempty"`
	Package           string             `json:"package,omitempty"`
	Task              string             `json:"task,omitempty"`
	Name              string             `json:"name,omitempty"`
	StartedAt         *time.Time         `json:"startedAt,omitempty"`
	EndedAt           *time.Time         `json:"endedAt,omitempty"`
	Actions           int                `json:"actions"`
	Path              string             `json:"path,omitempty"`
	Error             string             `json:"error,omitempty"`
	CaseFingerprint   string             `json:"caseFingerprint,omitempty"`
	EvidenceIndexPath string             `json:"evidenceIndexPath,omitempty"`
	DraftPath         string             `json:"draftPath,omitempty"`
}

type RecordingManifestFile struct {
	Path      string `json:"path"`
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"sizeBytes"`
}

type RecordingManifest struct {
	SchemaVersion   string                  `json:"schemaVersion"`
	Package         string                  `json:"package"`
	Name            string                  `json:"name"`
	Task            string                  `json:"task,omitempty"`
	RecordedAt      time.Time               `json:"recordedAt"`
	RecordedWidth   int                     `json:"recordedWidth,omitempty"`
	RecordedHeight  int                     `json:"recordedHeight,omitempty"`
	Actions         int                     `json:"actions"`
	CaseFingerprint string                  `json:"caseFingerprint"`
	Files           []RecordingManifestFile `json:"files"`
}

type ReplayConfig struct {
	RequestID       string             `json:"requestId,omitempty"`
	Execute         bool               `json:"execute"`
	Speed           float64            `json:"speed,omitempty"`
	Case            Case               `json:"case"`
	ResumeFrom      int                `json:"resumeFrom,omitempty"`
	CaseFingerprint string             `json:"caseFingerprint,omitempty"`
	Loops           int                `json:"loops,omitempty"`
	HostOwner       execution.Identity `json:"-"`
}

type ReplayState struct {
	RequestID         string             `json:"requestId,omitempty"`
	Identity          execution.Identity `json:"identity"`
	CaseFingerprint   string             `json:"caseFingerprint,omitempty"`
	Running           bool               `json:"running"`
	Stopping          bool               `json:"stopping,omitempty"`
	Finalizing        bool               `json:"finalizing,omitempty"`
	Package           string             `json:"package,omitempty"`
	StartedAt         *time.Time         `json:"startedAt,omitempty"`
	EndedAt           *time.Time         `json:"endedAt,omitempty"`
	Actions           int                `json:"actions"`
	Completed         int                `json:"completedActions"`
	ResumeFrom        int                `json:"resumeFrom,omitempty"`
	Cycle             int                `json:"cycle,omitempty"`
	Loops             int                `json:"loops,omitempty"`
	StopReason        string             `json:"stopReason,omitempty"`
	Error             string             `json:"error,omitempty"`
	ArtifactDir       string             `json:"artifactDir,omitempty"`
	EvidenceIndexPath string             `json:"evidenceIndexPath,omitempty"`
	Verdict           string             `json:"verdict,omitempty"`
	VerdictReason     string             `json:"verdictReason,omitempty"`
}

type CaseSummary struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Package         string    `json:"package"`
	RecordedAt      time.Time `json:"recordedAt"`
	Actions         int       `json:"actions"`
	Task            string    `json:"task,omitempty"`
	CaseFingerprint string    `json:"caseFingerprint"`
}

type DraftSummary struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Package    string    `json:"package"`
	RecordedAt time.Time `json:"recordedAt"`
	Actions    int       `json:"actions"`
	Task       string    `json:"task,omitempty"`
}

type Manager struct {
	root                 string
	capture              CaptureSource
	foreground           ForegroundSource
	executor             platform.Executor
	controller           ControlInjector
	screenshot           ScreenshotSource
	hierarchy            HierarchySource
	mu                   sync.Mutex
	recording            RecordingState
	replay               ReplayState
	current              Case
	normalizer           *Normalizer
	recordCancel         context.CancelFunc
	recordDone           chan struct{}
	recordFinalizeDone   chan struct{}
	recordFinalizeErr    error
	recordPersisting     bool
	recordGeneration     uint64
	excluded             *Bounds
	replayCancel         context.CancelFunc
	replayDone           chan struct{}
	coordinator          *execution.Coordinator
	recordIdentity       execution.Identity
	replayIdentity       execution.Identity
	replayReceipts       *execution.ReceiptStore
	replayHasAssertions  bool
	draftPath            string
	recordingAssets      map[string][]byte
	pendingTargets       map[int]*ActionTarget
	persistRecordingHook func(recordingSnapshot) (recordingResult, error)
	persistReplayHook    func(execution.Identity) error
}

type recordingSnapshot struct {
	value     Case
	assets    map[string][]byte
	root      string
	draftPath string
	identity  execution.Identity
}

type recordingResult struct {
	value        Case
	path         string
	evidencePath string
}

func New(root string, capture CaptureSource, foreground ForegroundSource, executor platform.Executor, controller ControlInjector, screenshot ScreenshotSource) *Manager {
	manager := &Manager{root: root, capture: capture, foreground: foreground, executor: executor, controller: controller, screenshot: screenshot}
	if hierarchy, ok := screenshot.(HierarchySource); ok {
		manager.hierarchy = hierarchy
	}
	return manager
}

func (m *Manager) SetExecutionCoordinator(value *execution.Coordinator) {
	m.mu.Lock()
	m.coordinator = value
	m.mu.Unlock()
}

func (m *Manager) StartRecording(ctx context.Context, config RecordingConfig) (RecordingState, error) {
	config.Package, config.Task, config.Name = strings.TrimSpace(config.Package), strings.TrimSpace(config.Task), strings.TrimSpace(config.Name)
	if !packagePattern.MatchString(config.Package) || !validLabel(config.Name) || (config.Task != "" && !validLabel(config.Task)) {
		return m.RecordingState(), errors.New("invalid recording package or name")
	}
	if config.RequestID != "" && !requestPattern.MatchString(config.RequestID) {
		return m.RecordingState(), errors.New("invalid requestId")
	}
	if config.ExcludedBounds != nil && !config.ExcludedBounds.valid() {
		return m.RecordingState(), errors.New("invalid excludedBounds")
	}
	if m.capture == nil {
		return m.RecordingState(), errors.New("touch capture unavailable")
	}
	foreground, err := m.foreground.ForegroundPackage(ctx)
	if err != nil || foreground != config.Package {
		if err != nil {
			return m.RecordingState(), err
		}
		return m.RecordingState(), fmt.Errorf("target package is not foreground: %s", foreground)
	}
	width, height, err := m.displaySize(ctx)
	if err != nil {
		return m.RecordingState(), err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.recording.Running && m.recordingPendingLocked() {
		return m.recording, errors.New("unsaved recording must be finalized before starting another session")
	}
	if m.recording.Running || m.recording.Stopping || m.recording.Finalizing ||
		m.replay.Running || m.replay.Stopping || m.replay.Finalizing {
		return m.recording, errors.New("record or replay session already active")
	}
	if config.RequestID == "" {
		config.RequestID = fmt.Sprintf("record-%d", time.Now().UnixNano())
	}
	identity := execution.NewIdentity("recording", config.RequestID, m.recordGeneration+1)
	if m.coordinator != nil {
		identity, err = m.coordinator.Acquire("recording", config.RequestID)
		if err != nil {
			return m.recording, err
		}
	}
	now := time.Now().UTC()
	m.current = Case{SchemaVersion: SchemaVersion, Task: config.Task, Name: config.Name, Package: config.Package, RecordedAt: now, RecordedWidth: width, RecordedHeight: height, Actions: []Action{}}
	draftDirectory := filepath.Join(m.root, config.Package, "Replay", ".drafts", config.RequestID)
	if err = os.MkdirAll(draftDirectory, 0o755); err != nil {
		if m.coordinator != nil {
			m.coordinator.Release(identity)
		}
		return m.recording, err
	}
	m.draftPath = filepath.Join(draftDirectory, "draft.json")
	m.recordingAssets = map[string][]byte{}
	m.pendingTargets = map[int]*ActionTarget{}
	if err = m.writeDraftLocked(); err != nil {
		_ = os.RemoveAll(draftDirectory)
		if m.coordinator != nil {
			m.coordinator.Release(identity)
		}
		m.draftPath = ""
		return m.recording, err
	}
	captureContext, cancel := context.WithCancel(context.Background())
	m.normalizer = NewNormalizer(now)
	m.excluded = config.ExcludedBounds
	m.recording = RecordingState{RequestID: config.RequestID, Identity: identity, Running: true, Package: config.Package, Task: config.Task, Name: config.Name, StartedAt: &now, DraftPath: filepath.ToSlash(m.draftPath)}
	m.recordIdentity = identity
	m.recordCancel, m.recordDone = cancel, make(chan struct{})
	m.recordFinalizeDone, m.recordFinalizeErr, m.recordPersisting = nil, nil, false
	m.recordGeneration++
	generation := m.recordGeneration
	done := m.recordDone
	go func() {
		err := m.capture.Capture(captureContext, func(at time.Time, event touchreader.Event) {
			m.captureEvent(generation, at, event)
		})
		m.mu.Lock()
		if m.recordGeneration != generation {
			m.mu.Unlock()
			close(done)
			return
		}
		if err != nil && !errors.Is(err, context.Canceled) && captureContext.Err() == nil {
			m.recording.Error = err.Error()
		}
		m.recording.Running = false
		m.recording.Stopping = false
		m.recording.Finalizing = true
		now := time.Now().UTC()
		m.recording.EndedAt = &now
		m.recordCancel = nil
		m.mu.Unlock()
		close(done)
	}()
	return m.recording, nil
}

func (m *Manager) StopRecording(ctx context.Context) (RecordingState, error) {
	m.mu.Lock()
	cancel, done := m.recordCancel, m.recordDone
	if m.recording.Running {
		m.recording.Stopping = true
	}
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			return m.RecordingState(), ctx.Err()
		case <-time.After(3 * time.Second):
			return m.RecordingState(), fmt.Errorf("recording stop timed out: %w", context.DeadlineExceeded)
		}
	}
	m.mu.Lock()
	if m.current.SchemaVersion == "" || m.recording.Path != "" {
		state := m.recording
		m.mu.Unlock()
		return state, nil
	}
	if m.recordPersisting {
		finalizeDone := m.recordFinalizeDone
		m.mu.Unlock()
		return m.waitRecordingFinalization(ctx, finalizeDone)
	}
	m.recording.Running = false
	m.recording.Stopping = false
	m.recording.Finalizing = true
	snapshot := recordingSnapshot{
		value: cloneCase(m.current), assets: cloneRecordingAssets(m.recordingAssets),
		root: m.root, draftPath: m.draftPath, identity: m.recordIdentity,
	}
	hook, coordinator := m.persistRecordingHook, m.coordinator
	finalizeDone := make(chan struct{})
	m.recordFinalizeDone, m.recordFinalizeErr, m.recordPersisting = finalizeDone, nil, true
	m.mu.Unlock()
	go m.finalizeRecording(snapshot, hook, coordinator, finalizeDone)
	return m.waitRecordingFinalization(ctx, finalizeDone)
}

func (m *Manager) waitRecordingFinalization(ctx context.Context, done <-chan struct{}) (RecordingState, error) {
	select {
	case <-done:
		m.mu.Lock()
		state, err := m.recording, m.recordFinalizeErr
		m.mu.Unlock()
		return state, err
	case <-ctx.Done():
		return m.RecordingState(), ctx.Err()
	case <-time.After(3 * time.Second):
		return m.RecordingState(), fmt.Errorf("recording stop timed out: %w", context.DeadlineExceeded)
	}
}

func (m *Manager) StopRecordingOwned(ctx context.Context, sessionID, ownerToken string) (RecordingState, error) {
	if err := m.ValidateRecordingOwner(sessionID, ownerToken); err != nil {
		return m.RecordingState(), err
	}
	return m.StopRecording(ctx)
}

func (m *Manager) ValidateRecordingOwner(sessionID, ownerToken string) error {
	m.mu.Lock()
	identity, coordinator := m.recordIdentity, m.coordinator
	active := m.recording.Running || m.recording.Stopping || m.recording.Finalizing
	m.mu.Unlock()
	return validateExecutionOwner(identity, coordinator, active, sessionID, ownerToken)
}

func (m *Manager) RecordingState() RecordingState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.recording
}

func (m *Manager) CurrentCase() Case        { m.mu.Lock(); defer m.mu.Unlock(); return cloneCase(m.current) }
func (m *Manager) ReplayState() ReplayState { m.mu.Lock(); defer m.mu.Unlock(); return m.replay }

func (m *Manager) StartReplay(ctx context.Context, config ReplayConfig) (ReplayState, error) {
	if !config.Execute {
		return m.ReplayState(), errors.New("execute=true is required")
	}
	if config.RequestID != "" && !requestPattern.MatchString(config.RequestID) {
		return m.ReplayState(), errors.New("invalid requestId")
	}
	if config.Speed == 0 {
		config.Speed = 1
	}
	if config.Speed < 0.1 || config.Speed > 4 {
		return m.ReplayState(), errors.New("speed must be between 0.1 and 4")
	}
	if config.Loops == 0 {
		config.Loops = 1
	}
	if config.Loops < -1 || config.Loops > 50 {
		return m.ReplayState(), errors.New("loops must be -1 or 1 through 50")
	}
	config.Case = cloneCase(config.Case)
	caseFingerprint, fingerprintErr := config.Case.Fingerprint()
	if fingerprintErr != nil {
		return m.ReplayState(), fingerprintErr
	}
	if config.CaseFingerprint != "" && config.CaseFingerprint != caseFingerprint {
		return m.ReplayState(), errors.New("caseFingerprint does not match the verified case snapshot")
	}
	config.CaseFingerprint = caseFingerprint
	if err := config.Case.Verify(); err != nil {
		return m.ReplayState(), err
	}
	if config.ResumeFrom < 0 || config.ResumeFrom > len(config.Case.Actions) {
		return m.ReplayState(), errors.New("resumeFrom is outside the action range")
	}
	for _, action := range config.Case.Actions[config.ResumeFrom:] {
		if (action.Type == "text" || action.Type == "multi_touch") && m.controller == nil {
			return m.ReplayState(), errors.New("scrcpy control unavailable")
		}
		if action.Type == "assert_screenshot" && m.screenshot == nil {
			return m.ReplayState(), errors.New("screenshot source unavailable")
		}
	}
	if config.Case.RecordedWidth > 0 {
		width, height, sizeErr := m.displaySize(ctx)
		if sizeErr != nil {
			return m.ReplayState(), sizeErr
		}
		if err := compatibleDisplay(config.Case.RecordedWidth, config.Case.RecordedHeight, width, height); err != nil {
			return m.ReplayState(), err
		}
	}
	if err := m.ensureReplayForeground(ctx, config.Case.Package); err != nil {
		return m.ReplayState(), err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.recording.Running && m.recordingPendingLocked() {
		return m.replay, errors.New("unsaved recording must be finalized before starting replay")
	}
	if m.recording.Running || m.recording.Stopping || m.recording.Finalizing ||
		m.replay.Running || m.replay.Stopping || m.replay.Finalizing {
		return m.replay, errors.New("record or replay session already active")
	}
	if config.RequestID == "" {
		config.RequestID = fmt.Sprintf("replay-%d", time.Now().UnixNano())
	}
	identity := execution.NewIdentity("replay", config.RequestID, uint64(time.Now().UnixNano()))
	if config.HostOwner.SessionID != "" {
		if m.coordinator != nil {
			var err error
			identity, err = m.coordinator.AcquireHosted(config.HostOwner, "replay", config.RequestID)
			if err != nil {
				return m.replay, err
			}
		}
	} else if m.coordinator != nil {
		var err error
		identity, err = m.coordinator.Acquire("replay", config.RequestID)
		if err != nil {
			return m.replay, err
		}
	}
	now := time.Now().UTC()
	replayContext, cancel := context.WithCancel(context.Background())
	m.replay = ReplayState{RequestID: config.RequestID, Identity: identity, CaseFingerprint: caseFingerprint, Running: true, Package: config.Case.Package, StartedAt: &now, Actions: len(config.Case.Actions), Completed: config.ResumeFrom, ResumeFrom: config.ResumeFrom, Cycle: 1, Loops: config.Loops}
	m.replayIdentity = identity
	m.replayReceipts = execution.NewReceiptStore()
	m.replayHasAssertions = false
	for _, replayAction := range config.Case.Actions {
		if replayAction.Type == "assert_screenshot" {
			m.replayHasAssertions = true
			break
		}
	}
	m.replayCancel, m.replayDone = cancel, make(chan struct{})
	done := m.replayDone
	go m.runReplay(replayContext, config, done)
	return m.replay, nil
}

func (m *Manager) StopReplay(ctx context.Context) (ReplayState, error) {
	m.mu.Lock()
	cancel, done := m.replayCancel, m.replayDone
	if m.replay.Running {
		m.replay.Stopping = true
	}
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			return m.ReplayState(), ctx.Err()
		case <-time.After(3 * time.Second):
			return m.ReplayState(), fmt.Errorf("replay stop timed out: %w", context.DeadlineExceeded)
		}
	}
	return m.ReplayState(), nil
}

func (m *Manager) StopReplayOwned(ctx context.Context, sessionID, ownerToken string) (ReplayState, error) {
	m.mu.Lock()
	identity, coordinator := m.replayIdentity, m.coordinator
	active := m.replay.Running || m.replay.Stopping || m.replay.Finalizing
	m.mu.Unlock()
	if err := validateExecutionOwner(identity, coordinator, active, sessionID, ownerToken); err != nil {
		return m.ReplayState(), err
	}
	return m.StopReplay(ctx)
}

func validateExecutionOwner(identity execution.Identity, coordinator *execution.Coordinator, running bool, sessionID, ownerToken string) error {
	if identity.SessionID != sessionID || identity.OwnerToken != ownerToken {
		return execution.ErrOwnerMismatch
	}
	if coordinator != nil && running {
		return coordinator.Validate(sessionID, ownerToken)
	}
	return nil
}

func (m *Manager) finishReplay(reason string, err error) {
	m.mu.Lock()
	if !m.replay.Running {
		m.mu.Unlock()
		return
	}
	now := time.Now().UTC()
	m.replay.Running = false
	m.replay.Stopping = false
	m.replay.Finalizing = true
	m.replay.EndedAt, m.replay.StopReason = &now, reason
	if err != nil {
		m.replay.Error = err.Error()
	}
	m.replayCancel = nil
	identity, coordinator, persistHook := m.replayIdentity, m.coordinator, m.persistReplayHook
	m.mu.Unlock()
	var persistErr error
	if persistHook != nil {
		persistErr = persistHook(identity)
	} else {
		persistErr = m.persistReplay(identity)
	}
	m.mu.Lock()
	if persistErr != nil {
		m.replay.Error = persistErr.Error()
		m.replay.Verdict = "not_tested"
		m.replay.VerdictReason = "replay artifacts could not be persisted"
	}
	m.mu.Unlock()
	if coordinator != nil {
		coordinator.Release(identity)
	}
	m.mu.Lock()
	if m.replayIdentity.SessionID == identity.SessionID {
		m.replay.Finalizing = false
	}
	m.mu.Unlock()
}
