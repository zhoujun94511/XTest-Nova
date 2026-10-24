package exploration

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/zhoujun94511/xtest-nova/agent/internal/execution"
)

func (m *Manager) executeChosen(ctx context.Context, config Config, loop *runLoop, node *graphNode, analysis Analysis, activity string, action Action) loopSignal {
	loop.exhaustedFingerprint = ""
	loop.exhaustedCaptures = 0
	loop.actionAttemptNumber++
	stepNumber := loop.actionAttemptNumber
	stepID := fmt.Sprintf("%s:step-%06d", config.RequestID, stepNumber)
	observation := observationID(config.RequestID, stepNumber, analysis.Fingerprint)
	actionDigest := actionFingerprint(action)
	m.mu.Unlock()

	freshFingerprint, freshErr := m.validateObservation(ctx, config, activity, analysis.Fingerprint, config.Rules)
	receipt, duplicate, receiptErr := m.receipts.Accept(execution.ActionReceipt{StepID: stepID, ObservationID: observation, SourceFingerprint: analysis.Fingerprint, ActionFingerprint: actionDigest})
	if receiptErr != nil {
		m.finish("safety_stop", receiptErr.Error())
		return loopStop
	}
	if duplicate { // A retried step is observable but never executes twice.
		if receipt.Status == execution.ReceiptFailed || receipt.Status == execution.ReceiptRejected {
			m.finish("input_failed", receipt.Error)
			return loopStop
		}
		return loopContinue
	}
	if freshErr != nil || freshFingerprint != analysis.Fingerprint {
		message := "page changed before action execution"
		if freshErr != nil {
			message = freshErr.Error()
		}
		m.receipts.Finish(stepID, execution.ReceiptRejected, "stale_observation", message, "", freshFingerprint)
		m.mu.Lock()
		m.state.StaleActions++
		m.mu.Unlock()
		return loopContinue
	}
	commandCtx, cancel := context.WithTimeout(ctx, actionTimeout(action))
	err := m.executeAction(commandCtx, action)
	cancel()
	if err != nil {
		m.receipts.Finish(stepID, execution.ReceiptFailed, "input_failed", err.Error(), "", "")
		m.finish("input_failed", err.Error())
		return loopStop
	}
	m.receipts.Finish(stepID, execution.ReceiptExecuted, "", "", "", "")
	m.mu.Lock()
	if !action.Backtrack {
		node.actionAttempts[action.ID]++
		if action.Type != "swipe" || node.actionAttempts[action.ID] >= maxStableScrollAttempts {
			node.tried[action.ID] = true
		}
		if action.Type == "swipe" {
			node.nonScrollSelections = 0
		} else {
			node.nonScrollSelections++
		}
		if action.Type == "input" {
			m.inputTried[inputExecutionKey(activity, action)] = true
		}
	}
	step := Step{Number: stepNumber, StepID: stepID, ObservationID: observation, At: time.Now().UTC(), From: analysis.Fingerprint, Action: action}
	m.steps = append(m.steps, step)
	loop.pending = len(m.steps) - 1
	loop.pendingNoteBlocked = false
	m.state.Steps = len(m.steps)
	if action.Backtrack {
		m.state.Backtracks++
	}
	switch action.Type {
	case "swipe":
		m.state.Scrolls++
		m.state.ScrollAttempts++
	case "input":
		m.state.Inputs++
	case "tap":
		if action.Recovery {
			m.state.Recoveries++
		}
	}
	last := action
	m.state.LastAction = &last
	m.refreshCountsLocked()
	m.mu.Unlock()
	if err := waitAction(ctx, time.Duration(config.IntervalMillis)*time.Millisecond); err != nil {
		return loopStop
	}
	return loopContinue
}

func actionTimeout(action Action) time.Duration {
	if action.Type == "input" {
		return 12 * time.Second
	}
	return 5 * time.Second
}

func (m *Manager) executeAction(ctx context.Context, action Action) error {
	var err error
	switch action.Type {
	case "tap":
		_, err = m.executor.Run(ctx, "input", "tap", strconv.Itoa(action.X), strconv.Itoa(action.Y))
	case "swipe":
		_, err = m.executor.Run(ctx, "input", "swipe", strconv.Itoa(action.X), strconv.Itoa(action.Y), strconv.Itoa(action.EndX), strconv.Itoa(action.EndY), "350")
	case "back":
		_, err = m.executor.Run(ctx, "input", "keyevent", "BACK")
	case "input":
		if m.textInjector == nil {
			return errors.New("exploration text injector unavailable")
		}
		if _, err = m.executor.Run(ctx, "input", "tap", strconv.Itoa(action.X), strconv.Itoa(action.Y)); err != nil {
			return err
		}
		if err = waitAction(ctx, 100*time.Millisecond); err != nil {
			return err
		}
		if _, err = m.executor.Run(ctx, "input", "keycombination", "113", "29"); err != nil {
			return err
		}
		if err = waitAction(ctx, 100*time.Millisecond); err != nil {
			return err
		}
		if _, err = m.executor.Run(ctx, "input", "keyevent", "67"); err != nil {
			return err
		}
		if action.Text == "" {
			return nil
		}
		return m.textInjector.InjectText(ctx, action.Text)
	default:
		return fmt.Errorf("unsupported exploration action: %s", action.Type)
	}
	return err
}

func waitAction(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
