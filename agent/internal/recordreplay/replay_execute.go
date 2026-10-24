package recordreplay

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/zhoujun94511/xtest-nova/agent/internal/artifacts"
	"github.com/zhoujun94511/xtest-nova/agent/internal/execution"
	"github.com/zhoujun94511/xtest-nova/agent/internal/scrcpy"
)

var sizePattern = regexp.MustCompile(`(?m)(?:Override|Physical) size:\s*(\d+)x(\d+)`)

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
