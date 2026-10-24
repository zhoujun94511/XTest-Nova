package recordreplay

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/zhoujun94511/xtest-nova/agent/internal/touchreader"
)

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
