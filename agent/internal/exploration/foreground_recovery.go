package exploration

import (
	"context"
	"fmt"
	"strconv"
	"time"
)

func (m *Manager) recoverOffTarget(ctx context.Context, config Config, loop *runLoop, foreground, activity, windowVersion string) loopSignal {
	if foreground != config.Package && time.Now().Before(loop.settleUntil) && (isTransientController(foreground) || foreground == loop.settleForeground) {
		if err := waitAction(ctx, time.Duration(config.IntervalMillis)*time.Millisecond); err != nil {
			return loopStop
		}
		return loopContinue
	}
	if foreground != config.Package && loop.pending >= 0 && m.steps[loop.pending].Action.Backtrack {
		commandCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		_, launchErr := m.executor.Run(commandCtx, "sh", "/system/bin/monkey", "-p", config.Package, "-c", "android.intent.category.LAUNCHER", "1")
		cancel()
		if launchErr != nil {
			m.finish("safety_stop", fmt.Sprintf("restore target after backtrack: %v", launchErr))
			return loopStop
		}
		loop.pending = -1
		loop.pendingNoteBlocked = false
		loop.emptyCaptures = 0
		loop.exhaustedFingerprint = ""
		loop.exhaustedCaptures = 0
		loop.settleUntil = time.Now().Add(transitionSettleDuration)
		loop.settleGeneration = pageGeneration{activity: activity, windowVersion: windowVersion}
		loop.settleForeground = ""
		m.mu.Lock()
		m.state.Recoveries++
		m.mu.Unlock()
		if err := waitAction(ctx, time.Duration(config.IntervalMillis)*time.Millisecond); err != nil {
			return loopStop
		}
		return loopContinue
	}
	if foreground != config.Package {
		const maxExternalRecoveries = 3
		if loop.externalAttempts[foreground] >= maxExternalRecoveries {
			if config.DurationSeconds > 0 {
				m.mu.Lock()
				elapsed := m.durationElapsedLocked(config)
				if elapsed {
					m.finishLocked("duration_elapsed", "")
				}
				m.mu.Unlock()
				if elapsed {
					return loopStop
				}
				if err := m.relaunchTarget(ctx, config); err != nil {
					m.finish("safety_stop", fmt.Sprintf("restart target after leaving the package: %v", err))
					return loopStop
				}
				clear(loop.externalAttempts)
				armStartupHold(loop, time.Now())
				loop.pending = -1
				loop.pendingNoteBlocked = false
				loop.emptyCaptures = 0
				loop.exhaustedFingerprint = ""
				loop.exhaustedCaptures = 0
				loop.settleUntil = time.Now().Add(transitionSettleDuration)
				loop.settleGeneration = pageGeneration{activity: activity, windowVersion: windowVersion}
				loop.settleForeground = ""
				if err := waitAction(ctx, time.Duration(config.IntervalMillis)*time.Millisecond); err != nil {
					return loopStop
				}
				return loopContinue
			}
			m.finish("safety_stop", fmt.Sprintf("target package is not foreground after %d recovery attempts: %s", maxExternalRecoveries, foreground))
			return loopStop
		}
		loop.externalAttempts[foreground]++
		m.mu.Lock()
		if loop.pending >= 0 {
			pendingStep := &m.steps[loop.pending]
			if source := m.nodes[pendingStep.From]; source != nil {
				m.blockEdgeLocked(source.semantic, semanticActionKey(pendingStep.Action))
			}
			pendingStep.Destination = "external:" + foreground
			m.state.ExternalActions++
		}
		m.state.Recoveries++
		if loop.adActive {
			m.state.AdExternalRecoveries++
		}
		m.mu.Unlock()
		commandCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		_, backErr := m.executor.Run(commandCtx, "input", "keyevent", "BACK")
		cancel()
		if backErr != nil {
			m.finish("safety_stop", fmt.Sprintf("dismiss external package %s: %v", foreground, backErr))
			return loopStop
		}
		commandCtx, cancel = context.WithTimeout(ctx, 5*time.Second)
		_, launchErr := m.executor.Run(commandCtx, "sh", "/system/bin/monkey", "-p", config.Package, "-c", "android.intent.category.LAUNCHER", "1")
		cancel()
		if launchErr != nil {
			m.finish("safety_stop", fmt.Sprintf("restore target from external package %s: %v", foreground, launchErr))
			return loopStop
		}
		loop.pending = -1
		loop.pendingNoteBlocked = false
		loop.emptyCaptures = 0
		loop.exhaustedFingerprint = ""
		loop.exhaustedCaptures = 0
		loop.settleUntil = time.Now().Add(transitionSettleDuration)
		loop.settleGeneration = pageGeneration{activity: activity, windowVersion: windowVersion}
		loop.settleForeground = ""
		if err := waitAction(ctx, time.Duration(config.IntervalMillis)*time.Millisecond); err != nil {
			return loopStop
		}
		return loopContinue
	}
	return loopNext
}

func (m *Manager) runRestart(ctx context.Context, config Config, loop *runLoop, activity string, generation pageGeneration) error {
	cold, err := m.restartExhaustedTarget(ctx, config, loop, activity)
	if err != nil {
		return err
	}
	if cold {
		clear(loop.specialAttempts)
		clear(loop.specialCooldownUntil)
		loop.adActive = false
		loop.adEncounterSequence++
		armStartupHold(loop, time.Now())
	}
	loop.settleUntil = time.Now().Add(transitionSettleDuration)
	loop.settleGeneration = generation
	loop.settleForeground = ""
	return nil
}

const maxStuckEscapeAttempts = 4

const (
	escapeLeftToRight = 1
	escapeRightToLeft = 2
	escapeTopToBottom = 3
)

func (m *Manager) restartExhaustedTarget(ctx context.Context, config Config, loop *runLoop, activity string) (bool, error) {
	if loop.escapeActivity != activity {
		loop.escapeActivity = activity
		loop.escapeAttempts = 0
	}
	if loop.escapeAttempts >= maxStuckEscapeAttempts {
		loop.escapeActivity = ""
		loop.escapeAttempts = 0
		loop.restartedActivity = ""
		return true, m.relaunchTarget(ctx, config)
	}
	loop.escapeAttempts++
	loop.restartedActivity = activity
	switch loop.escapeAttempts {
	case 1:
		return false, m.pressBack(ctx)
	case 2:
		return false, m.swipeEscape(ctx, loop, escapeLeftToRight)
	case 3:
		return false, m.swipeEscape(ctx, loop, escapeRightToLeft)
	default:
		return false, m.swipeEscape(ctx, loop, escapeTopToBottom)
	}
}

func (m *Manager) swipeEscape(ctx context.Context, loop *runLoop, direction int) error {
	width, height := loop.screenWidth, loop.screenHeight
	if width <= 0 || height <= 0 {
		output, err := m.executor.Run(ctx, "wm", "size")
		if err == nil {
			width, height, _ = screenSize(output)
		}
		if width <= 0 || height <= 0 {
			width, height = 1080, 2400
		}
		loop.screenWidth, loop.screenHeight = width, height
	}
	x, y, endX, endY := escapeSwipe(width, height, direction)
	commandCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := m.executor.Run(commandCtx, "input", "swipe", strconv.Itoa(x), strconv.Itoa(y), strconv.Itoa(endX), strconv.Itoa(endY), "350")
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.state.Recoveries++
	m.mu.Unlock()
	return nil
}

// escapeSwipe follows gesture navigation: back starts at the left or right
// edge and moves inward. A downward swipe stays below the status bar and above
// the bottom home zone, so it can dismiss a sheet without opening notifications
// or sending the app to the background.
func escapeSwipe(width, height, direction int) (x, y, endX, endY int) {
	inset := 8
	if inset*4 >= width {
		inset = 1
	}
	midY := height / 2
	switch direction {
	case escapeRightToLeft:
		return width - inset, midY, width * 2 / 3, midY
	case escapeTopToBottom:
		return width / 2, height * 15 / 100, width / 2, height * 55 / 100
	default:
		return inset, midY, width / 3, midY
	}
}

func (m *Manager) pressBack(ctx context.Context) error {
	commandCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := m.executor.Run(commandCtx, "input", "keyevent", "BACK")
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.state.Recoveries++
	m.mu.Unlock()
	return nil
}

func (m *Manager) relaunchTarget(ctx context.Context, config Config) error {
	stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	_, stopErr := m.executor.Run(stopCtx, "am", "force-stop", config.Package)
	cancel()
	if stopErr != nil {
		return stopErr
	}
	commandCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := m.executor.Run(commandCtx, "sh", "/system/bin/monkey", "-p", config.Package, "-c", "android.intent.category.LAUNCHER", "1")
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.state.Recoveries++
	m.mu.Unlock()
	return nil
}
