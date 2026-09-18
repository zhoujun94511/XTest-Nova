package recordreplay

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/zhoujun94511/xtest-nova/agent/internal/artifacts"
	"github.com/zhoujun94511/xtest-nova/agent/internal/evidence"
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
	RequestID       string  `json:"requestId,omitempty"`
	Execute         bool    `json:"execute"`
	Speed           float64 `json:"speed,omitempty"`
	Case            Case    `json:"case"`
	ResumeFrom      int     `json:"resumeFrom,omitempty"`
	CaseFingerprint string  `json:"caseFingerprint,omitempty"`
	Loops           int     `json:"loops,omitempty"`
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

func (m *Manager) captureEvent(generation uint64, at time.Time, event touchreader.Event) {
	var observed *ActionTarget
	if event.Operation == "d" && m.hierarchy != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		document, err := m.hierarchy.Hierarchy(ctx)
		cancel()
		if err == nil {
			m.mu.Lock()
			width, height := m.current.RecordedWidth, m.current.RecordedHeight
			m.mu.Unlock()
			observed = targetAt(document, event.XP, event.YP, width, height)
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.recordGeneration != generation || !m.recording.Running || m.normalizer == nil {
		return
	}
	if event.Operation == "d" {
		m.pendingTargets[event.Index] = observed
	}
	actions := m.normalizer.Feed(at, event)
	if target := m.pendingTargets[event.Index]; target != nil {
		for index := range actions {
			if actions[index].Type == "tap" || actions[index].Type == "double_tap" || actions[index].Type == "long_press" || actions[index].Type == "swipe" {
				actions[index].Target = target
			}
		}
	}
	if event.Operation == "u" {
		delete(m.pendingTargets, event.Index)
	}
	m.appendActionsLocked(actions...)
	m.recording.Actions = len(m.current.Actions)
}

func (m *Manager) recordingPendingLocked() bool {
	return m.current.SchemaVersion != "" && m.recording.Path == ""
}

func (m *Manager) appendActionsLocked(actions ...Action) {
	changed := false
	for _, action := range actions {
		if m.excluded != nil && actionStartsInside(action, *m.excluded) {
			continue
		}
		if action.Type == "tap" && len(m.current.Actions) > 0 {
			previous := &m.current.Actions[len(m.current.Actions)-1]
			gap := action.OffsetMillis - previous.OffsetMillis
			if previous.Type == "tap" && gap >= 0 && gap <= 350 && distance(previous.Start, action.Start) <= 0.03 {
				previous.Type, previous.DurationMillis = "double_tap", gap
				changed = true
				continue
			}
		}
		m.current.Actions = append(m.current.Actions, action)
		changed = true
	}
	if changed {
		if err := m.writeDraftLocked(); err != nil && m.recording.Error == "" {
			m.recording.Error = "recording draft: " + err.Error()
		}
	}
}

func (m *Manager) writeDraftLocked() error {
	if m.draftPath == "" {
		return nil
	}
	return writeJSONAtomic(m.draftPath, m.current)
}

func actionStartsInside(action Action, bounds Bounds) bool {
	switch action.Type {
	case "tap", "long_press", "double_tap", "swipe":
		return bounds.contains(action.Start)
	case "multi_touch":
		for _, contact := range action.Contacts {
			if bounds.contains(contact.Start) {
				return true
			}
		}
	}
	return false
}

func distance(left, right Point) float64 { return math.Hypot(left.X-right.X, left.Y-right.Y) }

func (m *Manager) AppendText(ctx context.Context, text string, focus *Point) (Case, error) {
	if !utf8.ValidString(text) || len(text) > 4096 || strings.ContainsRune(text, '\x00') {
		return m.CurrentCase(), errors.New("invalid UTF-8 text")
	}
	if focus != nil && !validPoint(*focus) {
		return m.CurrentCase(), errors.New("invalid focus coordinates")
	}
	generation, err := m.requireRecordingForeground(ctx)
	if err != nil {
		return m.CurrentCase(), err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.recordGeneration != generation || !m.recording.Running || m.recording.StartedAt == nil {
		return cloneCase(m.current), errors.New("recording session changed while appending text")
	}
	action := Action{Type: "text", OffsetMillis: time.Since(*m.recording.StartedAt).Milliseconds(), Text: text}
	if focus != nil {
		action.Focus, action.Start = true, *focus
	} else if point, ok := m.lastFocusPointLocked(); ok {
		action.Focus, action.Start = true, point
	}
	m.appendActionsLocked(action)
	m.recording.Actions = len(m.current.Actions)
	return cloneCase(m.current), nil
}

func (m *Manager) AppendTextOwned(ctx context.Context, sessionID, ownerToken, text string, focus *Point) (Case, error) {
	if err := m.ValidateRecordingOwner(sessionID, ownerToken); err != nil {
		return m.CurrentCase(), err
	}
	return m.AppendText(ctx, text, focus)
}

func (m *Manager) lastFocusPointLocked() (Point, bool) {
	for index := len(m.current.Actions) - 1; index >= 0; index-- {
		action := m.current.Actions[index]
		switch action.Type {
		case "tap", "double_tap", "long_press":
			if validPoint(action.Start) {
				return action.Start, true
			}
		}
	}
	return Point{}, false
}

func (m *Manager) AppendKey(ctx context.Context, keyCode int) (Case, error) {
	if keyCode != 4 {
		return m.CurrentCase(), errors.New("key is outside the recording allowlist")
	}
	generation, err := m.requireRecordingForeground(ctx)
	if err != nil {
		return m.CurrentCase(), err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.recordGeneration != generation || !m.recording.Running || m.recording.StartedAt == nil {
		return cloneCase(m.current), errors.New("recording session changed while appending key")
	}
	m.appendActionsLocked(Action{Type: "key", OffsetMillis: time.Since(*m.recording.StartedAt).Milliseconds(), KeyCode: keyCode})
	m.recording.Actions = len(m.current.Actions)
	return cloneCase(m.current), nil
}

func (m *Manager) AppendKeyOwned(ctx context.Context, sessionID, ownerToken string, keyCode int) (Case, error) {
	if err := m.ValidateRecordingOwner(sessionID, ownerToken); err != nil {
		return m.CurrentCase(), err
	}
	return m.AppendKey(ctx, keyCode)
}

func (m *Manager) AppendScreenshotAssertion(ctx context.Context, maxDistance int) (Case, error) {
	if maxDistance < 0 || maxDistance > 16 {
		return m.CurrentCase(), errors.New("maxHashDistance must be between 0 and 16")
	}
	if m.screenshot == nil {
		return m.CurrentCase(), errors.New("screenshot source unavailable")
	}
	generation, err := m.requireRecordingForeground(ctx)
	if err != nil {
		return m.CurrentCase(), err
	}
	data, err := m.screenshot.Screenshot(ctx)
	if err != nil {
		return m.CurrentCase(), err
	}
	hash, err := screenshotHash(bytes.NewReader(data))
	if err != nil {
		return m.CurrentCase(), err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.recordGeneration != generation || !m.recording.Running || m.recording.StartedAt == nil {
		return cloneCase(m.current), errors.New("recording session changed while appending screenshot assertion")
	}
	referencePath := fmt.Sprintf("screenshots/step-%06d.png", len(m.current.Actions)+1)
	assetPath := filepath.Join(filepath.Dir(m.draftPath), filepath.FromSlash(referencePath))
	if err = writeBytesAtomic(assetPath, data); err != nil {
		return cloneCase(m.current), err
	}
	m.recordingAssets[referencePath] = append([]byte(nil), data...)
	m.appendActionsLocked(Action{Type: "assert_screenshot", OffsetMillis: time.Since(*m.recording.StartedAt).Milliseconds(), ScreenshotHash: hash, MaxHashDistance: maxDistance, ReferencePath: referencePath})
	m.recording.Actions = len(m.current.Actions)
	return cloneCase(m.current), nil
}

func (m *Manager) AppendScreenshotAssertionOwned(ctx context.Context, sessionID, ownerToken string, maxDistance int) (Case, error) {
	if err := m.ValidateRecordingOwner(sessionID, ownerToken); err != nil {
		return m.CurrentCase(), err
	}
	return m.AppendScreenshotAssertion(ctx, maxDistance)
}

func (m *Manager) requireRecordingForeground(ctx context.Context) (uint64, error) {
	m.mu.Lock()
	running, target, generation := m.recording.Running, m.recording.Package, m.recordGeneration
	m.mu.Unlock()
	if !running {
		return 0, errors.New("recording session is not active")
	}
	if err := m.ensureReplayForeground(ctx, target); err != nil {
		return 0, err
	}
	return generation, nil
}

func (m *Manager) ensureReplayForeground(ctx context.Context, target string) error {
	deadline := time.Now().Add(12 * time.Second)
	last := ""
	for {
		foreground, err := m.foreground.ForegroundPackage(ctx)
		if err != nil {
			return err
		}
		last = foreground
		if foreground == target {
			return nil
		}
		if !isPermissionController(foreground) {
			return fmt.Errorf("target package is not foreground: %s", foreground)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("target package is not foreground: %s", last)
		}
		m.tryDismissPermissionDialog(ctx)
		if err = waitContext(ctx, 350*time.Millisecond); err != nil {
			return err
		}
	}
}

func (m *Manager) tryDismissPermissionDialog(ctx context.Context) {
	if m.hierarchy == nil || m.executor == nil {
		return
	}
	observation, cancel := context.WithTimeout(ctx, 750*time.Millisecond)
	document, err := m.hierarchy.Hierarchy(observation)
	cancel()
	if err != nil || document == "" {
		return
	}
	width, height, err := m.displaySize(ctx)
	if err != nil {
		return
	}
	point, ok := permissionAllowPoint(document, width, height)
	if !ok {
		return
	}
	_, _ = m.executor.Run(ctx, "input", "tap", strconv.Itoa(pixel(point.X, width)), strconv.Itoa(pixel(point.Y, height)))
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

func (m *Manager) finalizeRecording(snapshot recordingSnapshot, hook func(recordingSnapshot) (recordingResult, error), coordinator *execution.Coordinator, done chan struct{}) {
	var result recordingResult
	var err error
	if hook != nil {
		result, err = hook(snapshot)
	} else {
		result, err = persistRecording(snapshot)
	}
	if err != nil {
		m.mu.Lock()
		if m.recordIdentity.SessionID == snapshot.identity.SessionID {
			m.recording.Error = err.Error()
			m.recording.Finalizing = false
			m.recordCancel, m.recordDone = nil, nil
			m.recordFinalizeErr = err
			m.recordPersisting = false
		}
		m.mu.Unlock()
		if coordinator != nil {
			coordinator.Release(snapshot.identity)
		}
		close(done)
		return
	}
	if snapshot.draftPath != "" {
		_ = os.RemoveAll(filepath.Dir(snapshot.draftPath))
	}
	now := time.Now().UTC()
	m.mu.Lock()
	if m.recordIdentity.SessionID == snapshot.identity.SessionID {
		m.current = result.value
		m.recording.Running, m.recording.Stopping = false, false
		m.recording.EndedAt, m.recording.Path = &now, result.path
		m.recording.CaseFingerprint = result.value.Integrity
		m.recording.EvidenceIndexPath = filepath.ToSlash(result.evidencePath)
		m.recording.DraftPath = ""
		m.recordCancel, m.recordDone = nil, nil
		m.draftPath = ""
		m.recordingAssets = nil
	}
	m.mu.Unlock()
	if coordinator != nil {
		coordinator.Release(snapshot.identity)
	}
	m.mu.Lock()
	if m.recordIdentity.SessionID == snapshot.identity.SessionID {
		m.recording.Finalizing = false
		m.recordFinalizeErr = nil
		m.recordPersisting = false
	}
	m.mu.Unlock()
	close(done)
}

func cloneRecordingAssets(source map[string][]byte) map[string][]byte {
	result := make(map[string][]byte, len(source))
	for path, data := range source {
		result[path] = append([]byte(nil), data...)
	}
	return result
}

func persistRecording(snapshot recordingSnapshot) (recordingResult, error) {
	value := snapshot.value
	if err := value.Seal(); err != nil {
		return recordingResult{}, err
	}
	directory := filepath.Join(snapshot.root, value.Package, "Replay")
	if value.Task != "" {
		directory = filepath.Join(directory, value.Task)
	}
	directory = filepath.Join(directory, value.RecordedAt.Format("20060102T150405.000000000Z"))
	transaction, err := artifacts.BeginDirectory(directory)
	if err != nil {
		return recordingResult{}, err
	}
	defer func() { _ = transaction.Abort() }()
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return recordingResult{}, err
	}
	content = append(content, '\n')
	path := filepath.Join(transaction.Staging, "case.json")
	if err = writeBytesAtomic(path, content); err != nil {
		return recordingResult{}, err
	}
	evidenceFiles := []string{path}
	manifestFiles := []RecordingManifestFile{{Path: "case.json", SHA256: sha256Hex(content), SizeBytes: int64(len(content))}}
	for referencePath, data := range snapshot.assets {
		assetPath := filepath.Join(transaction.Staging, filepath.FromSlash(referencePath))
		if err = writeBytesAtomic(assetPath, data); err != nil {
			return recordingResult{}, err
		}
		evidenceFiles = append(evidenceFiles, assetPath)
		manifestFiles = append(manifestFiles, RecordingManifestFile{Path: referencePath, SHA256: sha256Hex(data), SizeBytes: int64(len(data))})
	}
	sort.Slice(manifestFiles, func(i, j int) bool { return manifestFiles[i].Path < manifestFiles[j].Path })
	manifest := RecordingManifest{SchemaVersion: "xtest-nova-recording-manifest/v1", Package: value.Package, Name: value.Name, Task: value.Task, RecordedAt: value.RecordedAt, RecordedWidth: value.RecordedWidth, RecordedHeight: value.RecordedHeight, Actions: len(value.Actions), CaseFingerprint: value.Integrity, Files: manifestFiles}
	manifestPath := filepath.Join(transaction.Staging, "manifest.json")
	if err = writeJSONAtomic(manifestPath, manifest); err != nil {
		return recordingResult{}, err
	}
	evidenceFiles = append(evidenceFiles, manifestPath)
	evidencePath := filepath.Join(transaction.Staging, "evidence.json")
	index := evidence.BuildForPublication(transaction.Staging, transaction.Final, value.Package, value.Integrity, snapshot.identity, evidenceFiles)
	if err = evidence.Write(evidencePath, index); err != nil {
		return recordingResult{}, err
	}
	if err = transaction.Commit(); err != nil {
		return recordingResult{}, err
	}
	finalPath := filepath.Join(transaction.Final, "case.json")
	finalEvidencePath := filepath.Join(transaction.Final, "evidence.json")
	if err = pruneCases(filepath.Join(snapshot.root, value.Package, "Replay"), filepath.Dir(finalPath), 200); err != nil {
		return recordingResult{}, err
	}
	return recordingResult{value: value, path: filepath.ToSlash(finalPath), evidencePath: finalEvidencePath}, nil
}

func writeJSONAtomic(path string, value any) error {
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return writeBytesAtomic(path, append(content, '\n'))
}

func writeBytesAtomic(path string, content []byte) error {
	return artifacts.WriteFileAtomic(path, content, 0o644)
}

func sha256Hex(content []byte) string {
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:])
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

func (m *Manager) SetExcludedBoundsOwned(sessionID, ownerToken string, bounds *Bounds) error {
	if bounds == nil || !bounds.valid() {
		return errors.New("invalid excludedBounds")
	}
	if err := m.ValidateRecordingOwner(sessionID, ownerToken); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.recording.Running {
		return errors.New("recording is not running")
	}
	cloned := *bounds
	m.excluded = &cloned
	return nil
}

func (m *Manager) RecordingState() RecordingState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.recording
}

func (m *Manager) Cases(packageName string) ([]CaseSummary, error) {
	packageName = strings.TrimSpace(packageName)
	if packageName != "" && !packagePattern.MatchString(packageName) {
		return nil, errors.New("invalid Android package name")
	}
	result := make([]CaseSummary, 0)
	searchRoots := make([]string, 0)
	if packageName != "" {
		searchRoots = append(searchRoots, filepath.Join(m.root, packageName, "Replay"))
	} else if packages, readErr := os.ReadDir(m.root); readErr == nil {
		for _, entry := range packages {
			if entry.IsDir() && packagePattern.MatchString(entry.Name()) {
				searchRoots = append(searchRoots, filepath.Join(m.root, entry.Name(), "Replay"))
			}
		}
	} else if !os.IsNotExist(readErr) {
		return nil, readErr
	}
	for _, searchRoot := range searchRoots {
		err := filepath.Walk(searchRoot, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				if os.IsNotExist(walkErr) {
					return nil
				}
				return walkErr
			}
			if info.IsDir() || info.Name() != "case.json" || info.Size() > 4<<20 {
				return nil
			}
			content, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil
			}
			var value Case
			if json.Unmarshal(content, &value) != nil || value.Verify() != nil || (packageName != "" && value.Package != packageName) {
				return nil
			}
			relative, relativeErr := filepath.Rel(m.root, path)
			if relativeErr != nil {
				return nil
			}
			result = append(result, CaseSummary{
				ID:   base64.RawURLEncoding.EncodeToString([]byte(filepath.ToSlash(relative))),
				Name: value.Name, Package: value.Package, RecordedAt: value.RecordedAt, Actions: len(value.Actions), Task: value.Task, CaseFingerprint: value.Integrity,
			})
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].RecordedAt.After(result[j].RecordedAt) })
	if len(result) > 200 {
		result = result[:200]
	}
	return result, nil
}

func (m *Manager) Drafts(packageName string) ([]DraftSummary, error) {
	packageName = strings.TrimSpace(packageName)
	if !packagePattern.MatchString(packageName) {
		return nil, errors.New("invalid Android package name")
	}
	root := filepath.Join(m.root, packageName, "Replay", ".drafts")
	result := make([]DraftSummary, 0)
	err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			if os.IsNotExist(walkErr) {
				return nil
			}
			return walkErr
		}
		if info.IsDir() || info.Name() != "draft.json" || info.Size() > 4<<20 {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		var value Case
		if json.Unmarshal(content, &value) != nil || value.Validate() != nil || value.Package != packageName {
			return nil
		}
		relative, relativeErr := filepath.Rel(m.root, path)
		if relativeErr != nil {
			return nil
		}
		result = append(result, DraftSummary{ID: base64.RawURLEncoding.EncodeToString([]byte(filepath.ToSlash(relative))), Name: value.Name, Package: value.Package, RecordedAt: value.RecordedAt, Actions: len(value.Actions), Task: value.Task})
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	sort.Slice(result, func(i, j int) bool { return result[i].RecordedAt.After(result[j].RecordedAt) })
	return result, nil
}

func (m *Manager) draftPathFromID(id string) (string, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(id)
	if err != nil || len(decoded) == 0 {
		return "", errors.New("invalid recording draft id")
	}
	relative := filepath.Clean(filepath.FromSlash(string(decoded)))
	if filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.Base(relative) != "draft.json" {
		return "", errors.New("invalid recording draft path")
	}
	segments := strings.Split(filepath.ToSlash(relative), "/")
	validDraftPath := false
	for index := 0; index+1 < len(segments); index++ {
		if segments[index] == ".drafts" && segments[index+1] != "" {
			validDraftPath = true
			break
		}
	}
	if !validDraftPath {
		return "", errors.New("invalid recording draft path")
	}
	return filepath.Join(m.root, relative), nil
}

func (m *Manager) FinalizeDraft(ctx context.Context, id string) (RecordingState, error) {
	path, err := m.draftPathFromID(id)
	if err != nil {
		return m.RecordingState(), err
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Size() > 4<<20 {
		if err == nil {
			err = errors.New("invalid recording draft file")
		}
		return m.RecordingState(), err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return m.RecordingState(), err
	}
	var value Case
	if err = json.Unmarshal(content, &value); err != nil {
		return m.RecordingState(), err
	}
	value.Integrity = ""
	if err = value.Validate(); err != nil {
		return m.RecordingState(), err
	}
	assets := map[string][]byte{}
	for _, action := range value.Actions {
		if action.ReferencePath == "" {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(filepath.Dir(path), filepath.FromSlash(action.ReferencePath)))
		if readErr != nil {
			return m.RecordingState(), fmt.Errorf("recording draft reference %s: %w", action.ReferencePath, readErr)
		}
		assets[action.ReferencePath] = data
	}
	m.mu.Lock()
	if m.recording.Running || m.recording.Stopping || m.recording.Finalizing ||
		m.replay.Running || m.replay.Stopping || m.replay.Finalizing || m.recordingPendingLocked() {
		m.mu.Unlock()
		return m.RecordingState(), errors.New("record or replay session already active")
	}
	now := time.Now().UTC()
	requestID := "recover-" + strconv.FormatInt(now.UnixNano(), 10)
	m.current = value
	m.recordGeneration++
	m.recordIdentity = execution.NewIdentity("recording-recovery", requestID, m.recordGeneration)
	m.recording = RecordingState{RequestID: requestID, Identity: m.recordIdentity, Package: value.Package, Task: value.Task, Name: value.Name, StartedAt: &value.RecordedAt, EndedAt: &now, Actions: len(value.Actions), DraftPath: filepath.ToSlash(path)}
	m.draftPath = path
	m.recordingAssets = assets
	m.mu.Unlock()
	return m.StopRecording(ctx)
}

func (m *Manager) DeleteDraft(id string) error {
	path, err := m.draftPathFromID(id)
	if err != nil {
		return err
	}
	m.mu.Lock()
	activeDraft := m.draftPath != "" && filepath.Clean(m.draftPath) == filepath.Clean(path) && m.current.SchemaVersion != "" && m.recording.Path == ""
	m.mu.Unlock()
	if activeDraft {
		return errors.New("active recording draft cannot be deleted")
	}
	return os.RemoveAll(filepath.Dir(path))
}

func pruneCases(root, keep string, limit int) error {
	type storedCase struct {
		directory string
		modified  time.Time
	}
	values := make([]storedCase, 0)
	err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !info.IsDir() && info.Name() == "case.json" {
			values = append(values, storedCase{directory: filepath.Dir(path), modified: info.ModTime()})
		}
		return nil
	})
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	sort.Slice(values, func(i, j int) bool { return values[i].modified.Before(values[j].modified) })
	for len(values) > limit {
		value := values[0]
		values = values[1:]
		if filepath.Clean(value.directory) == filepath.Clean(keep) {
			values = append(values, value)
			continue
		}
		if err = os.RemoveAll(value.directory); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) caseJSONPath(id string) (string, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(id)
	if err != nil || len(decoded) == 0 {
		return "", errors.New("invalid recording id")
	}
	relative := filepath.Clean(filepath.FromSlash(string(decoded)))
	if filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.Base(relative) != "case.json" {
		return "", errors.New("invalid recording path")
	}
	path := filepath.Join(m.root, relative)
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() || info.Size() > 4<<20 {
		return "", errors.New("invalid recording file")
	}
	return path, nil
}

func (m *Manager) DeleteCase(id string) error {
	path, err := m.caseJSONPath(id)
	if err != nil {
		return err
	}
	directory := filepath.Dir(path)
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var value Case
	if err = json.Unmarshal(content, &value); err != nil {
		return err
	}
	m.mu.Lock()
	recordingBusy := m.recording.Running || m.recording.Stopping || m.recording.Finalizing || m.recordingPendingLocked()
	recordingDir := ""
	if m.recording.Path != "" {
		recordingDir = filepath.Clean(filepath.Dir(filepath.FromSlash(m.recording.Path)))
	}
	replayBusy := m.replay.Running || m.replay.Stopping || m.replay.Finalizing
	replayFingerprint := m.replay.CaseFingerprint
	m.mu.Unlock()
	if recordingBusy && recordingDir != "" && recordingDir == filepath.Clean(directory) {
		return errors.New("active recording case cannot be deleted")
	}
	if replayBusy && replayFingerprint != "" && replayFingerprint == value.Integrity {
		return errors.New("active replay case cannot be deleted")
	}
	return os.RemoveAll(directory)
}

func (m *Manager) LoadCase(id string) (Case, error) {
	path, err := m.caseJSONPath(id)
	if err != nil {
		return Case{}, err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return Case{}, err
	}
	var value Case
	if err = json.Unmarshal(content, &value); err != nil {
		return Case{}, err
	}
	if err = value.Verify(); err != nil {
		return Case{}, err
	}
	return value, nil
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
	if m.coordinator != nil {
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

func compatibleDisplay(recordedWidth, recordedHeight, currentWidth, currentHeight int) error {
	recordedLandscape := recordedWidth > recordedHeight
	currentLandscape := currentWidth > currentHeight
	if recordedLandscape != currentLandscape {
		return fmt.Errorf("display orientation differs from recording: recorded=%dx%d current=%dx%d", recordedWidth, recordedHeight, currentWidth, currentHeight)
	}
	recordedAspect := float64(max(recordedWidth, recordedHeight)) / float64(min(recordedWidth, recordedHeight))
	currentAspect := float64(max(currentWidth, currentHeight)) / float64(min(currentWidth, currentHeight))
	if math.Abs(recordedAspect-currentAspect)/recordedAspect > 0.10 {
		return fmt.Errorf("display aspect ratio differs from recording: recorded=%dx%d current=%dx%d", recordedWidth, recordedHeight, currentWidth, currentHeight)
	}
	return nil
}

func (m *Manager) runReplay(ctx context.Context, config ReplayConfig, done chan struct{}) {
	defer close(done)
	width, height, err := m.displaySize(ctx)
	if err != nil {
		m.finishReplay("display_unavailable", err)
		return
	}
	for cycle := 1; config.Loops < 0 || cycle <= config.Loops; cycle++ {
		m.mu.Lock()
		m.replay.Cycle = cycle
		m.replay.Loops = config.Loops
		m.mu.Unlock()
		timelineStart := time.Now()
		startIndex := 0
		if cycle == 1 {
			startIndex = config.ResumeFrom
		}
		baseOffset := int64(0)
		if startIndex < len(config.Case.Actions) {
			baseOffset = config.Case.Actions[startIndex].OffsetMillis
		}
		for index := startIndex; index < len(config.Case.Actions); index++ {
			action := config.Case.Actions[index]
			stepID := fmt.Sprintf("%s:cycle-%04d:step-%06d", config.RequestID, cycle, index+1)
			observationID := fmt.Sprintf("case:%s:%d:%d", strings.TrimPrefix(config.CaseFingerprint, "sha256:"), cycle, index)
			actionBytes, _ := json.Marshal(action)
			actionHash := sha256.Sum256(actionBytes)
			receipt, duplicate, receiptErr := m.replayReceipts.Accept(execution.ActionReceipt{StepID: stepID, ObservationID: observationID, SourceFingerprint: config.CaseFingerprint, ActionFingerprint: hex.EncodeToString(actionHash[:])})
			if receiptErr != nil {
				m.finishReplay("safety_stop", receiptErr)
				return
			}
			if duplicate {
				if receipt.Status == execution.ReceiptFailed || receipt.Status == execution.ReceiptRejected {
					m.finishReplay("input_failed", errors.New(receipt.Error))
					return
				}
				continue
			}
			target := timelineStart.Add(time.Duration(float64(action.OffsetMillis-baseOffset)/config.Speed) * time.Millisecond)
			delay := time.Until(target)
			if delay > 0 {
				timer := time.NewTimer(delay)
				select {
				case <-ctx.Done():
					timer.Stop()
					m.finishReplay("stopped", nil)
					return
				case <-timer.C:
				}
			}
			if foregroundErr := m.ensureReplayForeground(ctx, config.Case.Package); foregroundErr != nil {
				m.finishReplay("safety_stop", foregroundErr)
				return
			}
			if action.Target != nil && m.hierarchy != nil && (action.Type == "tap" || action.Type == "double_tap" || action.Type == "long_press" || action.Type == "swipe") {
				observationContext, cancelObservation := context.WithTimeout(ctx, 750*time.Millisecond)
				document, hierarchyErr := m.hierarchy.Hierarchy(observationContext)
				cancelObservation()
				if hierarchyErr == nil {
					if relocated, found := relocateTarget(document, *action.Target, width, height); found {
						action.Start = relocated
					}
				}
			}
			action.DurationMillis = int64(float64(action.DurationMillis) / config.Speed)
			if action.DurationMillis == 0 && (action.Type == "long_press" || action.Type == "swipe") {
				action.DurationMillis = 1
			}
			if err = m.execute(ctx, action, width, height); err != nil {
				reason := "input_failed"
				if action.Type == "assert_screenshot" {
					reason = "assertion_failed"
					var assertion *screenshotAssertionError
					if errors.As(err, &assertion) {
						if path, saveErr := m.saveAssertionFailure(config.Case, index, assertion); saveErr == nil {
							err = fmt.Errorf("%w; diagnostics=%s", err, filepath.ToSlash(path))
						} else {
							err = fmt.Errorf("%w; diagnostics error: %v", err, saveErr)
						}
					}
				}
				m.replayReceipts.Finish(stepID, execution.ReceiptFailed, reason, err.Error(), "", "")
				m.finishReplay(reason, err)
				return
			}
			m.replayReceipts.Finish(stepID, execution.ReceiptExecuted, "", "", "", "")
			m.mu.Lock()
			m.replay.Completed = index + 1
			m.mu.Unlock()
		}
		if config.Loops > 0 && cycle == config.Loops {
			break
		}
		if waitErr := waitContext(ctx, 400*time.Millisecond); waitErr != nil {
			m.finishReplay("stopped", nil)
			return
		}
		if foregroundErr := m.ensureReplayForeground(ctx, config.Case.Package); foregroundErr != nil {
			m.finishReplay("safety_stop", foregroundErr)
			return
		}
	}
	m.finishReplay("completed", nil)
}

func (m *Manager) execute(ctx context.Context, action Action, width, height int) error {
	x1, y1 := pixel(action.Start.X, width), pixel(action.Start.Y, height)
	var args []string
	switch action.Type {
	case "tap":
		args = []string{"tap", strconv.Itoa(x1), strconv.Itoa(y1)}
	case "double_tap":
		if _, err := m.executor.Run(ctx, "input", "tap", strconv.Itoa(x1), strconv.Itoa(y1)); err != nil {
			return err
		}
		gap := action.DurationMillis
		if gap <= 0 {
			gap = 100
		}
		if err := waitContext(ctx, time.Duration(gap)*time.Millisecond); err != nil {
			return err
		}
		args = []string{"tap", strconv.Itoa(x1), strconv.Itoa(y1)}
	case "long_press":
		args = []string{"swipe", strconv.Itoa(x1), strconv.Itoa(y1), strconv.Itoa(x1), strconv.Itoa(y1), strconv.FormatInt(action.DurationMillis, 10)}
	case "swipe":
		args = []string{"swipe", strconv.Itoa(x1), strconv.Itoa(y1), strconv.Itoa(pixel(action.End.X, width)), strconv.Itoa(pixel(action.End.Y, height)), strconv.FormatInt(action.DurationMillis, 10)}
	case "key":
		args = []string{"keyevent", strconv.Itoa(action.KeyCode)}
	case "text":
		if action.Focus {
			if _, err := m.executor.Run(ctx, "input", "tap", strconv.Itoa(x1), strconv.Itoa(y1)); err != nil {
				return err
			}
			if err := waitContext(ctx, 100*time.Millisecond); err != nil {
				return err
			}
		}
		if _, err := m.executor.Run(ctx, "input", "keycombination", "113", "29"); err != nil {
			return err
		}
		if err := waitContext(ctx, 100*time.Millisecond); err != nil {
			return err
		}
		if _, err := m.executor.Run(ctx, "input", "keyevent", "67"); err != nil {
			return err
		}
		if action.Text == "" {
			return nil
		}
		return m.controller.InjectText(ctx, action.Text)
	case "multi_touch":
		events := make([]scrcpy.Event, 0, len(action.Contacts)*3)
		for _, contact := range action.Contacts {
			events = append(events, scrcpy.Event{Type: 0, Operation: "d", Index: contact.Index, PercentX: contact.Start.X, PercentY: contact.Start.Y})
		}
		for index, contact := range action.Contacts {
			delay := 0
			if index == 0 {
				delay = int(action.DurationMillis)
			}
			events = append(events, scrcpy.Event{Type: 0, Operation: "m", Index: contact.Index, PercentX: contact.End.X, PercentY: contact.End.Y, Milliseconds: delay})
		}
		for _, contact := range action.Contacts {
			events = append(events, scrcpy.Event{Type: 0, Operation: "u", Index: contact.Index, PercentX: contact.End.X, PercentY: contact.End.Y})
		}
		return m.controller.InjectEvents(ctx, events)
	case "assert_screenshot":
		data, err := m.screenshot.Screenshot(ctx)
		if err != nil {
			return err
		}
		actual, err := screenshotHash(bytes.NewReader(data))
		if err != nil {
			return err
		}
		difference, err := hashDistance(action.ScreenshotHash, actual)
		if err != nil {
			return err
		}
		if difference > action.MaxHashDistance {
			return &screenshotAssertionError{data: append([]byte(nil), data...), expected: action.ScreenshotHash, actual: actual, difference: difference, maximum: action.MaxHashDistance}
		}
		return nil
	default:
		return fmt.Errorf("unsupported action: %s", action.Type)
	}
	_, err := m.executor.Run(ctx, "input", args...)
	return err
}

func (m *Manager) saveAssertionFailure(value Case, actionIndex int, assertion *screenshotAssertionError) (string, error) {
	transaction, err := artifacts.NewDeviceSessionTransaction(m.root, value.Package, "ReplayFailures")
	if err != nil {
		return "", err
	}
	defer func() { _ = transaction.Abort() }()
	actualPath := filepath.Join(transaction.Staging, "actual.png")
	if err = artifacts.WriteFileAtomic(actualPath, assertion.data, 0o644); err != nil {
		return "", err
	}
	report := map[string]any{
		"schemaVersion": "xtest-nova-replay-assertion/v1",
		"package":       value.Package, "task": value.Task, "case": value.Name,
		"actionIndex": actionIndex, "expectedHash": assertion.expected, "actualHash": assertion.actual,
		"distance": assertion.difference, "maximumDistance": assertion.maximum, "capturedAt": time.Now().UTC(),
	}
	content, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", err
	}
	if err = artifacts.WriteFileAtomic(filepath.Join(transaction.Staging, "assertion.json"), append(content, '\n'), 0o644); err != nil {
		return "", err
	}
	if err = transaction.Commit(); err != nil {
		return "", err
	}
	if err = artifacts.PruneSessions(filepath.Dir(transaction.Final), artifacts.MaxSessionsPerKind, transaction.Final); err != nil {
		return "", err
	}
	return filepath.Join(transaction.Final, "actual.png"), nil
}

func waitContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

var sizePattern = regexp.MustCompile(`(?m)(?:Override|Physical) size:\s*(\d+)x(\d+)`)

func (m *Manager) displaySize(ctx context.Context) (int, int, error) {
	out, err := m.executor.Run(ctx, "wm", "size")
	if err != nil {
		return 0, 0, err
	}
	matches := sizePattern.FindAllStringSubmatch(out, -1)
	if len(matches) == 0 {
		return 0, 0, errors.New("display size unavailable")
	}
	match := matches[len(matches)-1]
	width, _ := strconv.Atoi(match[1])
	height, _ := strconv.Atoi(match[2])
	if width < 1 || height < 1 {
		return 0, 0, errors.New("invalid display size")
	}
	return width, height, nil
}

func pixel(value float64, maximum int) int {
	result := int(value * float64(maximum-1))
	if result < 0 {
		return 0
	}
	if result >= maximum {
		return maximum - 1
	}
	return result
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

func (m *Manager) persistReplay(identity execution.Identity) error {
	m.mu.Lock()
	state := m.replay
	hasAssertions := m.replayHasAssertions
	var receipts []execution.ActionReceipt
	if m.replayReceipts != nil {
		receipts = m.replayReceipts.All()
	}
	m.mu.Unlock()
	// The owner token is a live-session capability. Persist the remaining
	// correlation identity, but never copy that capability into an artifact.
	state.Running = false
	state.Stopping = false
	state.Finalizing = false
	state.Identity.OwnerToken = ""
	transaction, err := artifacts.NewDeviceSessionTransaction(m.root, state.Package, "ReplayRuns")
	if err != nil {
		return fmt.Errorf("create replay artifact directory: %w", err)
	}
	defer func() { _ = transaction.Abort() }()
	state.ArtifactDir = filepath.ToSlash(transaction.Final)
	state.EvidenceIndexPath = filepath.ToSlash(filepath.Join(transaction.Final, "evidence.json"))
	if state.StopReason == "assertion_failed" {
		state.Verdict, state.VerdictReason = "failed", "screenshot checkpoint failed"
	} else if state.StopReason != "completed" || state.Error != "" {
		state.Verdict, state.VerdictReason = "not_tested", "replay did not complete reliably"
	} else if hasAssertions && len(receipts) == state.Actions {
		state.Verdict, state.VerdictReason = "passed", "all screenshot checkpoints passed with action receipts"
	} else {
		state.Verdict, state.VerdictReason = "not_tested", "replay completed without acceptance checkpoints"
	}
	paths := []string{filepath.Join(transaction.Staging, "replay.json"), filepath.Join(transaction.Staging, "receipts.json")}
	for index, value := range []any{state, receipts} {
		if err := writeJSONAtomic(paths[index], value); err != nil {
			return fmt.Errorf("write replay artifact %s: %w", filepath.Base(paths[index]), err)
		}
	}
	index := evidence.BuildForPublication(transaction.Staging, transaction.Final, state.Package, state.CaseFingerprint, identity, paths)
	if err := evidence.Write(filepath.Join(transaction.Staging, "evidence.json"), index); err != nil {
		return fmt.Errorf("write replay evidence index: %w", err)
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("publish replay artifacts: %w", err)
	}
	if err := artifacts.PruneSessions(filepath.Dir(transaction.Final), artifacts.MaxSessionsPerKind, transaction.Final); err != nil {
		return fmt.Errorf("prune replay artifacts: %w", err)
	}
	m.mu.Lock()
	m.replay.ArtifactDir, m.replay.EvidenceIndexPath, m.replay.Verdict, m.replay.VerdictReason = state.ArtifactDir, state.EvidenceIndexPath, state.Verdict, state.VerdictReason
	m.mu.Unlock()
	return nil
}
