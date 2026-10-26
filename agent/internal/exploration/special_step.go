package exploration

import (
	"context"
	"fmt"
	"time"

	"github.com/zhoujun94511/xtest-nova/agent/internal/execution"
)

func (m *Manager) handleSpecialScene(ctx context.Context, config Config, loop *runLoop, foreground, activity, windowVersion, hierarchy string) loopSignal {
	if foreground == config.Package {
		clear(loop.externalAttempts)
	}
	special, specialFound, err := inspectSpecialActivity(hierarchy, config.Package, foreground, activity, config.SpecialHandling)
	if err != nil {
		loop.consecutiveParseFailures++
		if loop.consecutiveParseFailures >= maximumConsecutiveCaptureFailures {
			m.finish("safety_stop", err.Error())
			return loopStop
		}
		if err := waitAction(ctx, time.Duration(config.IntervalMillis)*time.Millisecond); err != nil {
			return loopStop
		}
		return loopContinue
	}
	if specialFound && special.Kind == "ad" {
		m.mu.Lock()
		if !loop.adActive {
			loop.adActive, loop.adStartedAt = true, time.Now()
			loop.adEncounterSequence++
			m.state.AdEncounters++
		}
		m.state.LastAdType, m.state.LastAdPhase = special.AdType, special.AdPhase
		m.mu.Unlock()
	} else if loop.adActive && foreground == config.Package {
		m.mu.Lock()
		m.state.AdRecoveries++
		m.state.AdWaitMillis += time.Since(loop.adStartedAt).Milliseconds()
		m.mu.Unlock()
		loop.adActive = false
	}
	if !specialFound || special.Kind != "load_failure" {
		loop.suppressErrorScroll = false
	}
	if !specialFound {
		return loopNext
	}
	if special.Kind == "onboarding" && loop.stuckReady && loop.stuckActivity == activity {
		return loopNext
	}
	m.mu.Lock()
	attemptKey := special.Kind + "|" + special.Fingerprint
	if special.Kind == "load_failure" {
		attemptKey = "load_failure|" + activity
	}
	if special.Kind == "ad" {
		// Full-screen ad WebViews animate and mutate text while the same
		// interruption remains active. Bind the budget to the encounter,
		// otherwise each new page fingerprint could reset the attempt cap.
		attemptKey = fmt.Sprintf("ad|%d|%s", loop.adEncounterSequence, activity)
	}
	if special.Kind == "purchase_failure" {
		// The failure copy changes between attempts. Keep one budget for the
		// activity, and stop selecting purchase controls after the first one.
		attemptKey = "purchase_failure|" + activity
		loop.purchaseBlocked = true
	}
	if special.Kind == "load_failure" && loop.specialAttempts[attemptKey] >= 2 {
		loop.suppressErrorScroll = true
		m.mu.Unlock()
		return loopNext
	}
	if special.Outcome == "wait" {
		started, exists := loop.specialWaitStarted[attemptKey]
		if !exists {
			started = time.Now()
			loop.specialWaitStarted[attemptKey] = started
		}
		waitLimit := special.WaitLimitSeconds
		if waitLimit <= 0 {
			waitLimit = config.SpecialHandling.AdMaxWaitSeconds
		}
		if time.Since(started) < time.Duration(waitLimit)*time.Second {
			last := special
			m.state.LastSpecial = &last
			m.mu.Unlock()
			if err := waitAction(ctx, time.Duration(min(config.IntervalMillis, 1000))*time.Millisecond); err != nil {
				return loopStop
			}
			return loopContinue
		}
		if special.FallbackAction != nil {
			fallback := *special.FallbackAction
			special.Action = &fallback
			special.Outcome = "fallback_action"
		}
	} else {
		delete(loop.specialWaitStarted, attemptKey)
	}
	if loop.specialAttempts[attemptKey] > 0 && special.FallbackAction != nil {
		fallback := *special.FallbackAction
		special.Action = &fallback
		special.Outcome = "fallback_action"
	}
	last := special
	m.state.LastSpecial = &last
	if special.Outcome == "explore" {
		m.mu.Unlock()
		return loopNext
	}
	if (special.Outcome != "action" && special.Outcome != "fallback_action") || special.Action == nil {
		reason := "special_unhandled"
		if special.Outcome == "policy_required" {
			reason = "policy_required"
		}
		if reason == "special_unhandled" {
			if m.durationElapsedLocked(config) {
				m.finishLocked("duration_elapsed", "")
				m.mu.Unlock()
				return loopStop
			}
			last.Outcome = "restart"
			last.Reason = special.Reason
			m.state.LastSpecial = &last
			loop.allowColdStart = true
			m.mu.Unlock()
			return loopRestart
		}
		m.finishLocked(reason, special.Reason)
		m.mu.Unlock()
		return loopStop
	}
	if loop.specialAttempts[attemptKey] >= config.SpecialHandling.MaxAttempts {
		if special.Kind == "onboarding" {
			loop.stuckActivity = activity
			loop.stuckReady = true
			last.Outcome = "explore"
			last.Reason = "onboarding did not change; the stuck path leaves the page"
			m.state.LastSpecial = &last
			m.mu.Unlock()
			return loopNext
		}
		if special.Kind == "purchase_failure" {
			m.blockPendingTapLocked(loop)
			m.mu.Unlock()
			return loopRestart
		}
		if m.durationElapsedLocked(config) {
			m.finishLocked("duration_elapsed", "")
			m.mu.Unlock()
			return loopStop
		}
		last.Outcome = "restart"
		last.Reason = "special scene did not change; restarting the session"
		m.state.LastSpecial = &last
		loop.allowColdStart = true
		m.mu.Unlock()
		return loopRestart
	}
	if special.Kind == "purchase_failure" {
		m.blockPendingTapLocked(loop)
	}
	loop.specialAttempts[attemptKey]++
	loop.actionAttemptNumber++
	stepNumber := loop.actionAttemptNumber
	stepID := fmt.Sprintf("%s:step-%06d", config.RequestID, stepNumber)
	observation := observationID(config.RequestID, stepNumber, special.Fingerprint)
	actionDigest := actionFingerprint(*special.Action)
	m.mu.Unlock()

	freshFingerprint, freshErr := m.validateSpecialObservation(ctx, config, special, activity)
	receipt, duplicate, receiptErr := m.receipts.Accept(execution.ActionReceipt{StepID: stepID, ObservationID: observation, SourceFingerprint: special.Fingerprint, ActionFingerprint: actionDigest})
	if receiptErr != nil {
		m.finish("safety_stop", receiptErr.Error())
		return loopStop
	}
	if duplicate {
		if receipt.Status == execution.ReceiptFailed || receipt.Status == execution.ReceiptRejected {
			m.finish("input_failed", receipt.Error)
			return loopStop
		}
		return loopContinue
	}
	if freshErr != nil {
		message := "special scene changed before action execution"
		if freshErr != nil {
			message = freshErr.Error()
		}
		m.receipts.Finish(stepID, execution.ReceiptRejected, "stale_observation", message, "", freshFingerprint)
		m.mu.Lock()
		m.state.StaleActions++
		loop.specialAttempts[attemptKey]--
		m.mu.Unlock()
		return loopContinue
	}
	commandCtx, cancel := context.WithTimeout(ctx, actionTimeout(*special.Action))
	err = m.executeAction(commandCtx, *special.Action)
	cancel()
	if err != nil {
		m.receipts.Finish(stepID, execution.ReceiptFailed, "input_failed", err.Error(), "", "")
		return m.failInput(loop, err)
	}
	loop.consecutiveInputFailures = 0
	m.receipts.Finish(stepID, execution.ReceiptExecuted, "", "", "", "")
	loop.settleUntil = time.Now().Add(transitionSettleDuration)
	loop.settleGeneration = pageGeneration{activity: activity, windowVersion: windowVersion}
	loop.settleForeground = special.Foreground
	loop.emptyCaptures = 0
	loop.exhaustedFingerprint = ""
	loop.exhaustedCaptures = 0
	m.mu.Lock()
	step := Step{Number: stepNumber, StepID: stepID, ObservationID: observation, At: time.Now().UTC(), From: special.Fingerprint, Action: *special.Action}
	m.steps = append(m.steps, step)
	if loop.pending >= 0 {
		loop.pendingNoteBlocked = true
	}
	m.state.Steps = len(m.steps)
	m.state.Recoveries++
	m.state.SpecialActions++
	if special.Kind == "ad" {
		if special.Action.Type == "back" {
			m.state.AdBackFallbacks++
		} else {
			m.state.AdCloseActions++
		}
	}
	lastAction := *special.Action
	m.state.LastAction = &lastAction
	m.refreshCountsLocked()
	m.mu.Unlock()
	if err := waitAction(ctx, time.Duration(config.IntervalMillis)*time.Millisecond); err != nil {
		return loopStop
	}
	return loopContinue
}
