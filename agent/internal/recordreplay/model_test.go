package recordreplay

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zhoujun94511/xtest-nova/agent/internal/execution"
	"github.com/zhoujun94511/xtest-nova/agent/internal/scrcpy"
	"github.com/zhoujun94511/xtest-nova/agent/internal/touchreader"
)

func TestSavedCasesListAndLoadWithinRoot(t *testing.T) {
	root := t.TempDir()
	manager := New(root, nil, nil, nil, nil, nil)
	value := Case{SchemaVersion: SchemaVersion, Name: "saved-case", Package: "com.example.app", RecordedAt: time.Unix(2, 0).UTC(), Actions: []Action{}}
	if err := value.Seal(); err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(root, "com.example.app", "Replay", "saved")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	content, _ := json.Marshal(value)
	if err := os.WriteFile(filepath.Join(directory, "case.json"), content, 0o644); err != nil {
		t.Fatal(err)
	}
	cases, err := manager.Cases("com.example.app")
	if err != nil || len(cases) != 1 || cases[0].Name != "saved-case" {
		t.Fatalf("cases = %#v, %v", cases, err)
	}
	loaded, err := manager.LoadCase(cases[0].ID)
	if err != nil || loaded.Name != value.Name {
		t.Fatalf("loaded = %#v, %v", loaded, err)
	}
	if _, err := manager.LoadCase("Li4vY2FzZS5qc29u"); err == nil {
		t.Fatal("path traversal id was accepted")
	}
}

func TestCasesIgnoreArtifactsOutsideReplayAndPruneOldest(t *testing.T) {
	root := t.TempDir()
	manager := New(root, nil, nil, nil, nil, nil)
	for index, name := range []string{"old", "middle", "new"} {
		directory := filepath.Join(root, "com.example.app", "Replay", name)
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
		value := Case{SchemaVersion: SchemaVersion, Name: name, Package: "com.example.app", RecordedAt: time.Unix(int64(index+1), 0).UTC(), Actions: []Action{}}
		if err := value.Seal(); err != nil {
			t.Fatal(err)
		}
		content, _ := json.Marshal(value)
		path := filepath.Join(directory, "case.json")
		if err := os.WriteFile(path, content, 0o644); err != nil {
			t.Fatal(err)
		}
		stamp := time.Unix(int64(index+1), 0)
		if err := os.Chtimes(path, stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	noise := filepath.Join(root, "com.example.app", "Monkey", "noise")
	if err := os.MkdirAll(noise, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(noise, "case.json"), []byte("not a replay"), 0o644); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(root, "com.example.app", "Replay", "new")
	if err := pruneCases(filepath.Join(root, "com.example.app", "Replay"), keep, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "com.example.app", "Replay", "old")); !os.IsNotExist(err) {
		t.Fatalf("oldest case was not removed: %v", err)
	}
	cases, err := manager.Cases("com.example.app")
	if err != nil || len(cases) != 2 {
		t.Fatalf("cases=%#v err=%v", cases, err)
	}
}

func TestCaseIntegrityRejectsMutation(t *testing.T) {
	value := Case{SchemaVersion: SchemaVersion, Name: "tap-case", Package: "com.example.app", RecordedAt: time.Unix(1, 0).UTC(), Actions: []Action{{Type: "tap", Start: Point{X: .25, Y: .75}}}}
	if err := value.Seal(); err != nil {
		t.Fatal(err)
	}
	if err := value.Verify(); err != nil {
		t.Fatal(err)
	}
	value.Actions[0].Start.X = .5
	if err := value.Verify(); err == nil {
		t.Fatal("mutated recording passed integrity check")
	}
}

func TestCaseIntegrityCanonicalizesEmptyArrayAndSubMillisecondTime(t *testing.T) {
	value := Case{SchemaVersion: SchemaVersion, Name: "empty", Package: "com.example.app", RecordedAt: time.Unix(1, 987654321).UTC(), Actions: []Action{}}
	if err := value.Seal(); err != nil {
		t.Fatal(err)
	}
	value.Actions = nil
	value.RecordedAt = value.RecordedAt.Truncate(time.Millisecond)
	if err := value.Verify(); err != nil {
		t.Fatal(err)
	}
}

func TestCaseRejectsUnsafeKeyAndInvalidTimeline(t *testing.T) {
	value := Case{SchemaVersion: SchemaVersion, Name: "bad", Package: "com.example.app", Actions: []Action{{Type: "key", OffsetMillis: 10, KeyCode: 3}, {Type: "tap", OffsetMillis: 5, Start: Point{X: .5, Y: .5}}}}
	if err := value.Validate(); err == nil {
		t.Fatal("unsafe case accepted")
	}
}

func TestNormalizerCreatesTapLongPressAndSwipe(t *testing.T) {
	start := time.Unix(100, 0).UTC()
	n := NewNormalizer(start)
	n.Feed(start, touchreader.Event{Operation: "d", Index: 0, XP: .1, YP: .2})
	tap := n.Feed(start.Add(100*time.Millisecond), touchreader.Event{Operation: "u", Index: 0, XP: .1, YP: .2})
	n.Feed(start.Add(time.Second), touchreader.Event{Operation: "d", Index: 0, XP: .3, YP: .4})
	longPress := n.Feed(start.Add(1600*time.Millisecond), touchreader.Event{Operation: "u", Index: 0, XP: .3, YP: .4})
	n.Feed(start.Add(2*time.Second), touchreader.Event{Operation: "d", Index: 0, XP: .2, YP: .8})
	swipe := n.Feed(start.Add(2300*time.Millisecond), touchreader.Event{Operation: "u", Index: 0, XP: .2, YP: .2})
	if len(tap) != 1 || tap[0].Type != "tap" || len(longPress) != 1 || longPress[0].Type != "long_press" || len(swipe) != 1 || swipe[0].Type != "swipe" {
		t.Fatalf("actions = %#v %#v %#v", tap, longPress, swipe)
	}
}

func TestNormalizerCreatesMultiTouchAndManagerMergesDoubleTap(t *testing.T) {
	start := time.Unix(200, 0).UTC()
	n := NewNormalizer(start)
	n.Feed(start, touchreader.Event{Operation: "d", Index: 0, XP: .2, YP: .5})
	n.Feed(start.Add(10*time.Millisecond), touchreader.Event{Operation: "d", Index: 1, XP: .8, YP: .5})
	n.Feed(start.Add(200*time.Millisecond), touchreader.Event{Operation: "u", Index: 0, XP: .1, YP: .5})
	actions := n.Feed(start.Add(210*time.Millisecond), touchreader.Event{Operation: "u", Index: 1, XP: .9, YP: .5})
	if len(actions) != 1 || actions[0].Type != "multi_touch" || len(actions[0].Contacts) != 2 {
		t.Fatalf("actions=%#v", actions)
	}
	m := &Manager{current: Case{Actions: []Action{{Type: "tap", OffsetMillis: 100, Start: Point{X: .5, Y: .5}}}}}
	m.appendActionsLocked(Action{Type: "tap", OffsetMillis: 300, Start: Point{X: .51, Y: .5}})
	if len(m.current.Actions) != 1 || m.current.Actions[0].Type != "double_tap" {
		t.Fatalf("double tap=%#v", m.current.Actions)
	}
}

func TestRecordingExcludesOverlayTouches(t *testing.T) {
	manager := New(t.TempDir(), fakeCapture{}, fakeForeground{"com.example.app"}, &fakeExecutor{}, nil, nil)
	manager.mu.Lock()
	manager.current = Case{Actions: []Action{}}
	manager.excluded = &Bounds{Left: 0.7, Top: 0, Right: 1, Bottom: 0.3}
	manager.appendActionsLocked(
		Action{Type: "tap", Start: Point{X: 0.85, Y: 0.15}},
		Action{Type: "tap", Start: Point{X: 0.5, Y: 0.5}},
		Action{Type: "swipe", Start: Point{X: 0.8, Y: 0.2}, End: Point{X: 0.4, Y: 0.7}},
	)
	manager.mu.Unlock()
	if len(manager.current.Actions) != 1 || manager.current.Actions[0].Start.X != 0.5 {
		t.Fatalf("overlay touch filter retained %#v", manager.current.Actions)
	}
}

type fakeCapture struct{}

func replayBusy(manager *Manager) bool {
	state := manager.ReplayState()
	return state.Running || state.Stopping || state.Finalizing
}

func (fakeCapture) Capture(ctx context.Context, sink func(time.Time, touchreader.Event)) error {
	now := time.Now().UTC()
	sink(now, touchreader.Event{Operation: "d", Index: 0, XP: .25, YP: .5})
	sink(now.Add(100*time.Millisecond), touchreader.Event{Operation: "u", Index: 0, XP: .25, YP: .5})
	<-ctx.Done()
	return ctx.Err()
}

type failedCapture struct{}

func (failedCapture) Capture(_ context.Context, sink func(time.Time, touchreader.Event)) error {
	now := time.Now().UTC()
	sink(now, touchreader.Event{Operation: "d", Index: 0, XP: .25, YP: .5})
	sink(now.Add(50*time.Millisecond), touchreader.Event{Operation: "u", Index: 0, XP: .25, YP: .5})
	return errors.New("capture reader exited")
}

type blockingCapture struct {
	started chan struct{}
	release chan struct{}
}

func (c blockingCapture) Capture(context.Context, func(time.Time, touchreader.Event)) error {
	close(c.started)
	<-c.release
	return context.Canceled
}

type fakeForeground struct{ packageName string }

func (f fakeForeground) ForegroundPackage(context.Context) (string, error) { return f.packageName, nil }

type blockingForeground struct {
	entered     chan struct{}
	release     chan struct{}
	packageName string
}

func (f blockingForeground) ForegroundPackage(context.Context) (string, error) {
	close(f.entered)
	<-f.release
	return f.packageName, nil
}

func TestAppendRejectsAReplacedRecordingGeneration(t *testing.T) {
	foreground := blockingForeground{entered: make(chan struct{}), release: make(chan struct{}), packageName: "com.example.app"}
	manager := New(t.TempDir(), nil, foreground, nil, nil, nil)
	started := time.Now().UTC()
	manager.recordGeneration = 1
	manager.recording = RecordingState{Running: true, Package: foreground.packageName, StartedAt: &started}
	manager.current = Case{SchemaVersion: SchemaVersion, Package: foreground.packageName, Actions: []Action{}}

	result := make(chan error, 1)
	go func() {
		_, err := manager.AppendKey(context.Background(), 4)
		result <- err
	}()
	<-foreground.entered
	manager.mu.Lock()
	manager.recordGeneration++
	manager.current = Case{SchemaVersion: SchemaVersion, Package: foreground.packageName, Actions: []Action{}}
	manager.mu.Unlock()
	close(foreground.release)

	if err := <-result; err == nil || !strings.Contains(err.Error(), "session changed") {
		t.Fatalf("AppendKey error = %v, want replaced-session error", err)
	}
	if len(manager.CurrentCase().Actions) != 0 {
		t.Fatal("stale append mutated the replacement recording")
	}
}

type fakeExecutor struct {
	mu       sync.Mutex
	commands [][]string
}

func (f *fakeExecutor) Run(_ context.Context, name string, args ...string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.commands = append(f.commands, append([]string{name}, args...))
	if name == "wm" {
		return "Physical size: 1080x2400", nil
	}
	return "", nil
}
func (*fakeExecutor) RunBytes(context.Context, string, ...string) ([]byte, error) { return nil, nil }

type fakeController struct {
	texts  []string
	events [][]scrcpy.Event
}

func (f *fakeController) InjectText(_ context.Context, value string) error {
	f.texts = append(f.texts, value)
	return nil
}
func (f *fakeController) InjectEvents(_ context.Context, value []scrcpy.Event) error {
	f.events = append(f.events, append([]scrcpy.Event(nil), value...))
	return nil
}

type fakeScreenshot struct{ data []byte }

func (f fakeScreenshot) Screenshot(context.Context) ([]byte, error) {
	return append([]byte(nil), f.data...), nil
}

type fakeObservation struct {
	data      []byte
	hierarchy string
}

func (f fakeObservation) Screenshot(context.Context) ([]byte, error) {
	return append([]byte(nil), f.data...), nil
}

func (f fakeObservation) Hierarchy(context.Context) (string, error) { return f.hierarchy, nil }

func pngImage(left, right color.Color) []byte {
	value := image.NewRGBA(image.Rect(0, 0, 16, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 16; x++ {
			if x < 8 {
				value.Set(x, y, left)
			} else {
				value.Set(x, y, right)
			}
		}
	}
	var buffer bytes.Buffer
	_ = png.Encode(&buffer, value)
	return buffer.Bytes()
}

func TestScreenshotHashAndDistance(t *testing.T) {
	first, err := screenshotHash(bytes.NewReader(pngImage(color.Black, color.White)))
	if err != nil {
		t.Fatal(err)
	}
	second, err := screenshotHash(bytes.NewReader(pngImage(color.White, color.Black)))
	if err != nil {
		t.Fatal(err)
	}
	distance, err := hashDistance(first, second)
	if err != nil || distance == 0 {
		t.Fatalf("hashes=%s %s distance=%d err=%v", first, second, distance, err)
	}
}

func TestRecordingPersistsSealedNormalizedCase(t *testing.T) {
	manager := New(t.TempDir(), fakeCapture{}, fakeForeground{"com.example.app"}, &fakeExecutor{}, nil, nil)
	if _, err := manager.StartRecording(context.Background(), RecordingConfig{RequestID: "record-1", Package: "com.example.app", Name: "tap-case"}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for manager.RecordingState().Actions == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	state, err := manager.StopRecording(context.Background())
	if err != nil || state.Actions != 1 || state.Path == "" {
		t.Fatalf("state=%#v err=%v", state, err)
	}
	if _, err = os.Stat(state.Path); err != nil {
		t.Fatal(err)
	}
	current := manager.CurrentCase()
	if err = current.Verify(); err != nil {
		t.Fatal(err)
	}
}

func TestRecordingCapturesSemanticTargetAndRelocatesIt(t *testing.T) {
	firstHierarchy := `<hierarchy><node class="android.widget.FrameLayout" bounds="[0,0][1080,2400]"><node resource-id="com.example.app:id/open" text="打开" class="android.widget.Button" bounds="[100,900][500,1500]"/></node></hierarchy>`
	observation := fakeObservation{hierarchy: firstHierarchy}
	manager := New(t.TempDir(), fakeCapture{}, fakeForeground{"com.example.app"}, &fakeExecutor{}, nil, observation)
	if _, err := manager.StartRecording(context.Background(), RecordingConfig{Package: "com.example.app", Name: "semantic"}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for manager.RecordingState().Actions == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	action := manager.CurrentCase().Actions[0]
	if action.Target == nil || action.Target.ResourceID != "com.example.app:id/open" || action.Target.ObservationID == "" {
		t.Fatalf("action=%#v", action)
	}
	movedHierarchy := `<hierarchy><node class="android.widget.FrameLayout" bounds="[0,0][1080,2400]"><node resource-id="com.example.app:id/open" text="打开" class="android.widget.Button" bounds="[600,1200][1000,1800]"/></node></hierarchy>`
	point, found := relocateTarget(movedHierarchy, *action.Target, 1080, 2400)
	if !found || point.X < .7 || point.Y < .6 {
		t.Fatalf("point=%#v found=%t", point, found)
	}
	if _, err := manager.StopRecording(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestRecordingPersistsDraftManifestAndScreenshotReference(t *testing.T) {
	root := t.TempDir()
	screenshot := fakeScreenshot{pngImage(color.Black, color.White)}
	manager := New(root, fakeCapture{}, fakeForeground{"com.example.app"}, &fakeExecutor{}, nil, screenshot)
	started, err := manager.StartRecording(context.Background(), RecordingConfig{RequestID: "record-cn", Package: "com.example.app", Task: "相册流程", Name: "截图用例"})
	if err != nil {
		t.Fatal(err)
	}
	if started.DraftPath == "" {
		t.Fatal("recording did not expose a recovery draft")
	}
	if _, err = os.Stat(filepath.FromSlash(started.DraftPath)); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for manager.RecordingState().Actions == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	value, err := manager.AppendScreenshotAssertion(context.Background(), 8)
	if err != nil {
		t.Fatal(err)
	}
	last := value.Actions[len(value.Actions)-1]
	if last.ReferencePath == "" {
		t.Fatalf("screenshot action=%#v", last)
	}
	state, err := manager.StopRecording(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Dir(state.Path)
	for _, artifact := range []string{"case.json", "manifest.json", "evidence.json", filepath.FromSlash(last.ReferencePath)} {
		if _, statErr := os.Stat(filepath.Join(directory, artifact)); statErr != nil {
			t.Fatalf("artifact %s: %v", artifact, statErr)
		}
	}
	if _, err = os.Stat(filepath.FromSlash(started.DraftPath)); !os.IsNotExist(err) {
		t.Fatalf("finalized draft still exists: %v", err)
	}
	manifestContent, err := os.ReadFile(filepath.Join(directory, "manifest.json"))
	if err != nil || !bytes.Contains(manifestContent, []byte(`"caseFingerprint"`)) || !bytes.Contains(manifestContent, []byte(last.ReferencePath)) {
		t.Fatalf("manifest=%s err=%v", manifestContent, err)
	}
}

func TestRecordingPersistenceFailureRollsBackStagingDirectory(t *testing.T) {
	root := t.TempDir()
	recordedAt := time.Date(2026, 9, 16, 7, 0, 0, 123, time.UTC)
	value := Case{SchemaVersion: SchemaVersion, Name: "rollback", Package: "com.example.app", RecordedAt: recordedAt, Actions: []Action{}}
	_, err := persistRecording(recordingSnapshot{
		value:  value,
		assets: map[string][]byte{"invalid\x00name": []byte("failure")},
		root:   root,
	})
	if err == nil {
		t.Fatal("persistRecording accepted an invalid asset path")
	}
	parent := filepath.Join(root, value.Package, "Replay")
	final := filepath.Join(parent, recordedAt.Format("20060102T150405.000000000Z"))
	if _, statErr := os.Stat(final); !os.IsNotExist(statErr) {
		t.Fatalf("failed recording became visible: %v", statErr)
	}
	entries, readErr := os.ReadDir(parent)
	if readErr != nil {
		t.Fatal(readErr)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".staging-") {
			t.Fatalf("failed recording left staging directory %q", entry.Name())
		}
	}
}

func TestUnexpectedCaptureExitPreservesPendingRecordingUntilStop(t *testing.T) {
	manager := New(t.TempDir(), failedCapture{}, fakeForeground{"com.example.app"}, &fakeExecutor{}, nil, nil)
	coordinator := execution.NewCoordinator()
	manager.SetExecutionCoordinator(coordinator)
	if _, err := manager.StartRecording(context.Background(), RecordingConfig{Package: "com.example.app", Name: "pending"}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for manager.RecordingState().Running && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if state := manager.RecordingState(); state.Running || !state.Finalizing || state.Error == "" || state.EndedAt == nil {
		t.Fatalf("unexpected capture exit was not reflected in state: %#v", state)
	}
	if _, err := coordinator.Acquire("other", "other"); !errors.Is(err, execution.ErrOwned) {
		t.Fatalf("capture exit released coordinator ownership: %v", err)
	}
	if _, err := manager.StartRecording(context.Background(), RecordingConfig{Package: "com.example.app", Name: "replacement"}); err == nil || !strings.Contains(err.Error(), "finalized") {
		t.Fatalf("pending recording was overwritten: %v", err)
	}
	value := Case{SchemaVersion: SchemaVersion, Name: "replay", Package: "com.example.app", RecordedAt: time.Now().UTC(), Actions: []Action{}}
	if err := value.Seal(); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.StartReplay(context.Background(), ReplayConfig{Execute: true, Case: value}); err == nil || !strings.Contains(err.Error(), "finalized") {
		t.Fatalf("replay started over a pending recording: %v", err)
	}
	state, err := manager.StopRecording(context.Background())
	if err != nil || state.Path == "" {
		t.Fatalf("pending recording was not persisted: state=%#v err=%v", state, err)
	}
	if state.Finalizing {
		t.Fatalf("recording remained finalizing after persistence: %#v", state)
	}
	if _, err = coordinator.Acquire("other", "other"); err != nil {
		t.Fatalf("successful recording finalization retained ownership: %v", err)
	}
}

func mustStartRecording(t *testing.T, manager *Manager, config RecordingConfig) RecordingState {
	t.Helper()
	state, err := manager.StartRecording(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func stopRecordingWithTimeout(manager *Manager, state RecordingState, timeout time.Duration) (RecordingState, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return manager.StopRecordingOwned(ctx, state.Identity.SessionID, state.Identity.OwnerToken)
}

func TestRecordingStopTimeoutRemainsOwnedAndPollable(t *testing.T) {
	capture := blockingCapture{started: make(chan struct{}), release: make(chan struct{})}
	coordinator := execution.NewCoordinator()
	manager := New(t.TempDir(), capture, fakeForeground{"com.example.app"}, &fakeExecutor{}, nil, nil)
	manager.SetExecutionCoordinator(coordinator)
	started := mustStartRecording(t, manager, RecordingConfig{RequestID: "record-timeout", Package: "com.example.app", Name: "timeout"})
	<-capture.started
	state, err := stopRecordingWithTimeout(manager, started, 20*time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) || !state.Running || !state.Stopping {
		t.Fatalf("state=%#v error=%v", state, err)
	}
	if _, err = coordinator.Acquire("other", "other"); !errors.Is(err, execution.ErrOwned) {
		t.Fatalf("timed-out recording stop released ownership: %v", err)
	}
	close(capture.release)
	state, err = manager.StopRecordingOwned(context.Background(), started.Identity.SessionID, started.Identity.OwnerToken)
	if err != nil || state.Running || state.Stopping || state.Finalizing || state.Path == "" {
		t.Fatalf("final state=%#v error=%v", state, err)
	}
}

func TestRecordingPersistenceFailureExplicitlyReleasesOwnership(t *testing.T) {
	coordinator := execution.NewCoordinator()
	manager := New(t.TempDir(), fakeCapture{}, fakeForeground{"com.example.app"}, &fakeExecutor{}, nil, nil)
	manager.SetExecutionCoordinator(coordinator)
	manager.persistRecordingHook = func(recordingSnapshot) (recordingResult, error) {
		return recordingResult{}, errors.New("forced persistence failure")
	}
	started := mustStartRecording(t, manager, RecordingConfig{RequestID: "record-failure", Package: "com.example.app", Name: "failure"})
	state, err := manager.StopRecordingOwned(context.Background(), started.Identity.SessionID, started.Identity.OwnerToken)
	if err == nil {
		t.Fatal("recording persistence failure returned nil error")
		return
	}
	if state.Finalizing || !strings.Contains(state.Error, "forced persistence failure") {
		t.Fatalf("state=%#v error=%v", state, err)
	}
	if _, err = coordinator.Acquire("other", "other"); err != nil {
		t.Fatalf("explicit persistence failure retained ownership: %v", err)
	}
}

func TestRecordingFinalizationTimeoutKeepsOwnership(t *testing.T) {
	coordinator := execution.NewCoordinator()
	manager := New(t.TempDir(), fakeCapture{}, fakeForeground{"com.example.app"}, &fakeExecutor{}, nil, nil)
	manager.SetExecutionCoordinator(coordinator)
	persistStarted, persistRelease := make(chan struct{}), make(chan struct{})
	manager.persistRecordingHook = func(snapshot recordingSnapshot) (recordingResult, error) {
		close(persistStarted)
		<-persistRelease
		return persistRecording(snapshot)
	}
	started := mustStartRecording(t, manager, RecordingConfig{RequestID: "record-finalizing", Package: "com.example.app", Name: "finalizing"})
	var err error
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	result := make(chan error, 1)
	go func() {
		_, stopErr := manager.StopRecordingOwned(ctx, started.Identity.SessionID, started.Identity.OwnerToken)
		result <- stopErr
	}()
	<-persistStarted
	if state := manager.RecordingState(); state.Running || !state.Finalizing {
		t.Fatalf("persistence state=%#v", state)
	}
	if err = <-result; !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("finalization timeout error=%v", err)
	}
	cancel()
	if _, err = coordinator.Acquire("other", "other"); !errors.Is(err, execution.ErrOwned) {
		t.Fatalf("recording finalization released ownership early: %v", err)
	}
	close(persistRelease)
	state, err := manager.StopRecordingOwned(context.Background(), started.Identity.SessionID, started.Identity.OwnerToken)
	if err != nil || state.Finalizing || state.Path == "" {
		t.Fatalf("final state=%#v error=%v", state, err)
	}
	if _, err = coordinator.Acquire("other", "other"); err != nil {
		t.Fatalf("recording finalization retained ownership: %v", err)
	}
}

func TestReplayFinalizingBlocksReplacementUntilRelease(t *testing.T) {
	coordinator := execution.NewCoordinator()
	manager := New(t.TempDir(), fakeCapture{}, fakeForeground{"com.example.app"}, &fakeExecutor{}, nil, nil)
	manager.SetExecutionCoordinator(coordinator)
	persistStarted, persistRelease := make(chan struct{}), make(chan struct{})
	manager.persistReplayHook = func(execution.Identity) error {
		close(persistStarted)
		<-persistRelease
		return nil
	}
	value := Case{SchemaVersion: SchemaVersion, Name: "finalizing", Package: "com.example.app", RecordedAt: time.Now().UTC(), Actions: []Action{}}
	if err := value.Seal(); err != nil {
		t.Fatal(err)
	}
	started, err := manager.StartReplay(context.Background(), ReplayConfig{RequestID: "replay-finalizing", Execute: true, Case: value})
	if err != nil {
		t.Fatal(err)
	}
	<-persistStarted
	state := manager.ReplayState()
	if state.Running || !state.Finalizing {
		t.Fatalf("persistence state=%#v", state)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	state, err = manager.StopReplayOwned(ctx, started.Identity.SessionID, started.Identity.OwnerToken)
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) || !state.Finalizing {
		t.Fatalf("timed-out finalization state=%#v error=%v", state, err)
	}
	if _, err = coordinator.Acquire("other", "other"); !errors.Is(err, execution.ErrOwned) {
		t.Fatalf("replay finalization released ownership early: %v", err)
	}
	close(persistRelease)
	state, err = manager.StopReplayOwned(context.Background(), started.Identity.SessionID, started.Identity.OwnerToken)
	if err != nil || state.Finalizing {
		t.Fatalf("final replay state=%#v error=%v", state, err)
	}
	if _, err = coordinator.Acquire("other", "other"); err != nil {
		t.Fatalf("replay finalization retained ownership: %v", err)
	}
}

func TestInterruptedRecordingDraftCanBeFinalizedAfterRestart(t *testing.T) {
	root := t.TempDir()
	first := New(root, failedCapture{}, fakeForeground{"com.example.app"}, &fakeExecutor{}, nil, nil)
	if _, err := first.StartRecording(context.Background(), RecordingConfig{RequestID: "recoverable", Package: "com.example.app", Task: "恢复任务", Name: "中断用例"}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for first.RecordingState().Running && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	restarted := New(root, nil, nil, &fakeExecutor{}, nil, nil)
	drafts, err := restarted.Drafts("com.example.app")
	if err != nil || len(drafts) != 1 || drafts[0].Actions != 1 {
		t.Fatalf("drafts=%#v err=%v", drafts, err)
	}
	state, err := restarted.FinalizeDraft(context.Background(), drafts[0].ID)
	if err != nil || state.Path == "" || state.CaseFingerprint == "" {
		t.Fatalf("state=%#v err=%v", state, err)
	}
	if remaining, listErr := restarted.Drafts("com.example.app"); listErr != nil || len(remaining) != 0 {
		t.Fatalf("remaining=%#v err=%v", remaining, listErr)
	}
	cases, err := restarted.Cases("com.example.app")
	if err != nil || len(cases) != 1 || cases[0].Name != "中断用例" {
		t.Fatalf("cases=%#v err=%v", cases, err)
	}
}

func TestDoubleTapReplayUsesRecordedGap(t *testing.T) {
	executor := &fakeExecutor{}
	manager := New(t.TempDir(), nil, nil, executor, nil, nil)
	started := time.Now()
	if err := manager.execute(context.Background(), Action{Type: "double_tap", Start: Point{X: .5, Y: .5}, DurationMillis: 30}, 100, 100); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed < 25*time.Millisecond {
		t.Fatalf("recorded double-tap gap was ignored: %s", elapsed)
	}
	executor.mu.Lock()
	defer executor.mu.Unlock()
	if len(executor.commands) != 2 {
		t.Fatalf("double tap emitted %d commands", len(executor.commands))
	}
}

func TestRecordingTaskHierarchyAndFinalTextFocus(t *testing.T) {
	manager := New(t.TempDir(), fakeCapture{}, fakeForeground{"com.example.app"}, &fakeExecutor{}, nil, nil)
	if _, err := manager.StartRecording(context.Background(), RecordingConfig{Package: "com.example.app", Task: "checkout", Name: "address"}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for manager.RecordingState().Actions == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	value, err := manager.AppendText(context.Background(), "杭州 😀", nil)
	if err != nil || len(value.Actions) != 2 {
		t.Fatalf("case=%#v err=%v", value, err)
	}
	text := value.Actions[1]
	if text.Type != "text" || !text.Focus || text.Start.X != .25 || text.Start.Y != .5 {
		t.Fatalf("text action did not inherit last target focus: %#v", text)
	}
	state, err := manager.StopRecording(context.Background())
	if err != nil || !strings.Contains(filepath.ToSlash(state.Path), "/Replay/checkout/") {
		t.Fatalf("state=%#v err=%v", state, err)
	}
	cases, err := manager.Cases("com.example.app")
	if err != nil || len(cases) != 1 || cases[0].Task != "checkout" {
		t.Fatalf("cases=%#v err=%v", cases, err)
	}
}

func TestReplayUsesCurrentDisplayAndDeterministicCoordinates(t *testing.T) {
	executor := &fakeExecutor{}
	manager := New(t.TempDir(), fakeCapture{}, fakeForeground{"com.example.app"}, executor, nil, nil)
	value := Case{SchemaVersion: SchemaVersion, Name: "tap-case", Package: "com.example.app", RecordedAt: time.Now().UTC(), Actions: []Action{{Type: "tap", Start: Point{X: .5, Y: .25}}}}
	if err := value.Seal(); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.StartReplay(context.Background(), ReplayConfig{RequestID: "replay-1", Execute: true, Speed: 1, Case: value}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for replayBusy(manager) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	state := manager.ReplayState()
	if state.StopReason != "completed" || state.Completed != 1 {
		t.Fatalf("state=%#v", state)
	}
	executor.mu.Lock()
	defer executor.mu.Unlock()
	joined := make([]string, len(executor.commands))
	for i, command := range executor.commands {
		joined[i] = strings.Join(command, " ")
	}
	if len(joined) != 2 || joined[1] != "input tap 539 599" {
		t.Fatalf("commands=%#v", joined)
	}
}

func TestReplayRequiresExpectedCaseFingerprint(t *testing.T) {
	manager := New(t.TempDir(), fakeCapture{}, fakeForeground{"com.example.app"}, &fakeExecutor{}, nil, nil)
	value := Case{SchemaVersion: SchemaVersion, Name: "fingerprint", Package: "com.example.app", RecordedAt: time.Now().UTC(), Actions: []Action{}}
	if err := value.Seal(); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.StartReplay(context.Background(), ReplayConfig{Execute: true, CaseFingerprint: "sha256:wrong", Case: value}); err == nil || !strings.Contains(err.Error(), "caseFingerprint") {
		t.Fatalf("error=%v", err)
	}
}

func TestReplayArtifactDoesNotPersistOwnerToken(t *testing.T) {
	root := t.TempDir()
	manager := New(root, fakeCapture{}, fakeForeground{"com.example.app"}, &fakeExecutor{}, nil, nil)
	manager.SetExecutionCoordinator(execution.NewCoordinator())
	value := Case{SchemaVersion: SchemaVersion, Name: "owner-token", Package: "com.example.app", RecordedAt: time.Now().UTC(), Actions: []Action{}}
	if err := value.Seal(); err != nil {
		t.Fatal(err)
	}
	started, err := manager.StartReplay(context.Background(), ReplayConfig{RequestID: "owner-token", Execute: true, Case: value})
	if err != nil {
		t.Fatal(err)
	}
	if started.Identity.OwnerToken == "" {
		t.Fatal("live replay state did not expose an owner token")
	}
	deadline := time.Now().Add(time.Second)
	for manager.ReplayState().EvidenceIndexPath == "" && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	state := manager.ReplayState()
	content, err := os.ReadFile(filepath.FromSlash(filepath.Join(state.ArtifactDir, "replay.json")))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(content, []byte(started.Identity.OwnerToken)) || bytes.Contains(content, []byte(`"ownerToken"`)) {
		t.Fatalf("owner token leaked into replay artifact: %s", content)
	}
}

func TestRecordingActionOwnerFencesStaleClients(t *testing.T) {
	manager := New(t.TempDir(), fakeCapture{}, fakeForeground{"com.example.app"}, &fakeExecutor{}, nil, nil)
	started := time.Now().UTC()
	identity := execution.NewIdentity("recording", "owned-actions", 1)
	manager.recordGeneration = 1
	manager.recordIdentity = identity
	manager.recording = RecordingState{Running: true, Package: "com.example.app", StartedAt: &started, Identity: identity}
	manager.current = Case{SchemaVersion: SchemaVersion, Name: "owned-actions", Package: "com.example.app", RecordedAt: started, RecordedWidth: 100, RecordedHeight: 100, Actions: []Action{}}

	if _, err := manager.AppendKeyOwned(context.Background(), identity.SessionID, "stale", 4); !errors.Is(err, execution.ErrOwnerMismatch) {
		t.Fatalf("stale owner append error=%v", err)
	}
	if len(manager.CurrentCase().Actions) != 0 {
		t.Fatal("stale owner mutated the recording")
	}
	if _, err := manager.AppendKeyOwned(context.Background(), identity.SessionID, identity.OwnerToken, 4); err != nil {
		t.Fatal(err)
	}
	if len(manager.CurrentCase().Actions) != 1 {
		t.Fatal("current owner append was not recorded")
	}
}

func TestReplayPersistenceFailureUpdatesState(t *testing.T) {
	root := filepath.Join(t.TempDir(), "artifact-root")
	if err := os.WriteFile(root, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	manager := New(root, fakeCapture{}, fakeForeground{"com.example.app"}, &fakeExecutor{}, nil, nil)
	value := Case{SchemaVersion: SchemaVersion, Name: "persist-failure", Package: "com.example.app", RecordedAt: time.Now().UTC(), Actions: []Action{}}
	if err := value.Seal(); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.StartReplay(context.Background(), ReplayConfig{Execute: true, Case: value}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for replayBusy(manager) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	state := manager.ReplayState()
	if state.Running || !strings.Contains(state.Error, "create replay artifact directory") {
		t.Fatalf("state=%#v", state)
	}
	if state.Verdict != "not_tested" || state.VerdictReason != "replay artifacts could not be persisted" {
		t.Fatalf("verdict=%q reason=%q", state.Verdict, state.VerdictReason)
	}
}

func TestReplayRejectsIncompatibleRecordedDisplay(t *testing.T) {
	manager := New(t.TempDir(), fakeCapture{}, fakeForeground{"com.example.app"}, &fakeExecutor{}, nil, nil)
	for _, test := range []struct {
		name   string
		width  int
		height int
	}{
		{name: "orientation", width: 2400, height: 1080},
		{name: "aspect", width: 1080, height: 1600},
	} {
		t.Run(test.name, func(t *testing.T) {
			value := Case{SchemaVersion: SchemaVersion, Name: "display", Package: "com.example.app", RecordedAt: time.Now().UTC(), RecordedWidth: test.width, RecordedHeight: test.height, Actions: []Action{{Type: "tap", Start: Point{X: .5, Y: .5}}}}
			if err := value.Seal(); err != nil {
				t.Fatal(err)
			}
			if _, err := manager.StartReplay(context.Background(), ReplayConfig{Execute: true, Case: value}); err == nil || !strings.Contains(err.Error(), "display") {
				t.Fatalf("expected display compatibility error, got %v", err)
			}
		})
	}
}

func TestReplaySupportsUnicodeMultiTouchAndScreenshotAssertion(t *testing.T) {
	executor, controller := &fakeExecutor{}, &fakeController{}
	screenshot := fakeScreenshot{pngImage(color.Black, color.White)}
	hash, err := screenshotHash(bytes.NewReader(screenshot.data))
	if err != nil {
		t.Fatal(err)
	}
	manager := New(t.TempDir(), fakeCapture{}, fakeForeground{"com.example.app"}, executor, controller, screenshot)
	value := Case{SchemaVersion: SchemaVersion, Name: "advanced", Package: "com.example.app", RecordedAt: time.Now().UTC(), Actions: []Action{
		{Type: "text", Text: "中文 😀"},
		{Type: "multi_touch", DurationMillis: 100, Contacts: []Contact{{Index: 0, Start: Point{.2, .5}, End: Point{.1, .5}}, {Index: 1, Start: Point{.8, .5}, End: Point{.9, .5}}}},
		{Type: "assert_screenshot", ScreenshotHash: hash, MaxHashDistance: 0},
	}}
	if err = value.Seal(); err != nil {
		t.Fatal(err)
	}
	if _, err = manager.StartReplay(context.Background(), ReplayConfig{Execute: true, Case: value}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for replayBusy(manager) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	state := manager.ReplayState()
	if state.StopReason != "completed" || state.Completed != 3 || len(controller.texts) != 1 || controller.texts[0] != "中文 😀" || len(controller.events) != 1 || len(controller.events[0]) != 6 {
		t.Fatalf("state=%#v controller=%#v", state, controller)
	}
}

func TestScreenshotMismatchHasDedicatedStopReason(t *testing.T) {
	actual := fakeScreenshot{pngImage(color.Black, color.White)}
	expected, err := screenshotHash(bytes.NewReader(pngImage(color.White, color.Black)))
	if err != nil {
		t.Fatal(err)
	}
	manager := New(t.TempDir(), fakeCapture{}, fakeForeground{"com.example.app"}, &fakeExecutor{}, nil, actual)
	value := Case{SchemaVersion: SchemaVersion, Name: "assertion", Package: "com.example.app", RecordedAt: time.Now().UTC(), Actions: []Action{{Type: "assert_screenshot", ScreenshotHash: expected}}}
	if err = value.Seal(); err != nil {
		t.Fatal(err)
	}
	if _, err = manager.StartReplay(context.Background(), ReplayConfig{Execute: true, Case: value}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for replayBusy(manager) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if state := manager.ReplayState(); state.StopReason != "assertion_failed" || !strings.Contains(state.Error, "actual.png") {
		t.Fatalf("state=%#v", state)
	}
	matches, err := filepath.Glob(filepath.Join(manager.root, "com.example.app", "ReplayFailures", "*", "actual.png"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("assertion diagnostics=%v err=%v", matches, err)
	}
}

func TestReplayCanResumeAfterCooperativeStop(t *testing.T) {
	executor := &fakeExecutor{}
	manager := New(t.TempDir(), fakeCapture{}, fakeForeground{"com.example.app"}, executor, nil, nil)
	value := Case{SchemaVersion: SchemaVersion, Name: "resume", Package: "com.example.app", RecordedAt: time.Now().UTC(), Actions: []Action{{Type: "tap", Start: Point{.1, .1}}, {Type: "tap", OffsetMillis: 10000, Start: Point{.2, .2}}}}
	if err := value.Seal(); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.StartReplay(context.Background(), ReplayConfig{Execute: true, Case: value}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for manager.ReplayState().Completed < 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if _, err := manager.StopReplay(context.Background()); err != nil {
		t.Fatal(err)
	}
	if state := manager.ReplayState(); state.StopReason != "stopped" || state.Completed != 1 {
		t.Fatalf("stopped=%#v", state)
	}
	if _, err := manager.StartReplay(context.Background(), ReplayConfig{Execute: true, ResumeFrom: 1, Case: value}); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(time.Second)
	for replayBusy(manager) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if state := manager.ReplayState(); state.StopReason != "completed" || state.Completed != 2 || state.ResumeFrom != 1 {
		t.Fatalf("resumed=%#v", state)
	}
}
