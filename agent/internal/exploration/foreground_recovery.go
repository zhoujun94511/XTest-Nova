package exploration

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

func (m *Manager) recoverOffTarget(ctx context.Context, config Config, loop *runLoop, foreground, activity, windowVersion string) loopSignal {
	if foreground == "" {
		if err := waitAction(ctx, time.Duration(config.IntervalMillis)*time.Millisecond); err != nil {
			return loopStop
		}
		return loopContinue
	}
	if foreground != config.Package && time.Now().Before(loop.settleUntil) && (isTransientController(foreground) || foreground == loop.settleForeground) {
		if err := waitAction(ctx, time.Duration(config.IntervalMillis)*time.Millisecond); err != nil {
			return loopStop
		}
		return loopContinue
	}
	if foreground != config.Package && isHomeSurface(foreground, activity) {
		if time.Now().Before(loop.homeResumeAfter) {
			if err := waitAction(ctx, time.Duration(config.IntervalMillis)*time.Millisecond); err != nil {
				return loopStop
			}
			return loopContinue
		}
		loop.homeResumeAfter = time.Now().Add(homeResumeCooldown)
		commandCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		_, launchErr := m.executor.Run(commandCtx, "sh", "/system/bin/monkey", "-p", config.Package, "-c", "android.intent.category.LAUNCHER", "1")
		cancel()
		if launchErr != nil {
			m.finish("safety_stop", fmt.Sprintf("resume target from the home screen: %v", launchErr))
			return loopStop
		}
		loop.pending = -1
		loop.pendingNoteBlocked = false
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
const ladderCooldown = 4 * time.Second
const shellRelaunchCooldown = 30 * time.Second
const homeResumeCooldown = 8 * time.Second

const (
	escapeLeftToRight = 1
	escapeRightToLeft = 2
	escapeTopToBottom = 3
)

func (m *Manager) restartExhaustedTarget(ctx context.Context, config Config, loop *runLoop, activity string) (bool, error) {
	if loop.stuckRelaunch {
		loop.stuckRelaunch = false
		return true, m.relaunchTarget(ctx, config)
	}
	// Main, player, and routing shells treat system BACK and edge swipes as
	// leaving the app. Gesture navigation then lands on the launcher, and the
	// home recovery immediately relaunches the target. Pause on the clock
	// instead of bouncing the process.
	if !loop.allowColdStart && !coldStartAllowed(activity) {
		// The dialog is still this activity, so the four-gesture ladder is not
		// used: those edge swipes leave the app. One BACK, and if the same
		// activity is still exhausted, force-stop and relaunch. The next kill
		// waits out the clock so a shell does not bounce.
		if loop.escapeActivity != activity {
			loop.escapeActivity = activity
			loop.escapeAttempts = 0
		}
		if time.Now().Before(loop.ladderCooldownUntil) {
			return false, nil
		}
		if loop.escapeAttempts == 0 {
			loop.escapeAttempts = 1
			return false, m.pressBack(ctx)
		}
		loop.escapeAttempts = 0
		loop.ladderCooldownUntil = time.Now().Add(shellRelaunchCooldown)
		return true, m.relaunchTarget(ctx, config)
	}
	if loop.escapeActivity != activity {
		loop.escapeActivity = activity
		loop.escapeAttempts = 0
	}
	if loop.escapeAttempts >= maxStuckEscapeAttempts {
		loop.escapeActivity = ""
		loop.escapeAttempts = 0
		loop.restartedActivity = ""
		if !loop.allowColdStart && !coldStartAllowed(activity) {
			loop.ladderCooldownUntil = time.Now().Add(ladderCooldown)
			return false, nil
		}
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

func isHomePackage(name string) bool {
	return isHomeSurface(name, "")
}

// systemLauncherPackages are home processes matched as a whole package.
var systemLauncherPackages = map[string]struct{}{
	"com.android.launcher":                  {},
	"com.android.launcher3":                 {},
	"com.google.android.apps.nexuslauncher": {},
	"com.sec.android.app.launcher":          {},
	"com.miui.home":                         {},
	"com.mi.android.globallauncher":         {},
	"com.huawei.android.launcher":           {},
	"com.hihonor.android.launcher":          {},
	"com.oppo.launcher":                     {},
	"net.oneplus.launcher":                  {},
	"com.bbk.launcher2":                     {},
	"com.vivo.launcher":                     {},
	"com.transsion.hilauncher":              {},
	"com.transsion.xoslauncher":             {},
	"com.transsion.itel.launcher":           {},
	"com.motorola.launcher3":                {},
	"com.nothing.launcher":                  {},
	"com.lge.launcher2":                     {},
	"com.lge.launcher3":                     {},
	"com.asus.launcher":                     {},
	"com.sonymobile.home":                   {},
	"com.sonyericsson.home":                 {},
	"com.htc.launcher":                      {},
	"com.meizu.flyme.launcher":              {},
	"cn.nubia.launcher":                     {},
	"com.zte.mifavor.launcher":              {},
	"com.zui.launcher":                      {},
	"com.lenovo.launcher":                   {},
	"com.smartisanos.launcher":              {},
	"com.blackshark.launcher":               {},
}

func isHomeSurface(foreground, activity string) bool {
	if isHandoffSurface(foreground, activity) {
		return false
	}
	if homePackage(foreground) {
		return true
	}
	name := activitySimpleName(activity)
	switch name {
	case "home", "homeactivity", "launcher", "launcheractivity":
		return true
	default:
		return launcherSegment(name)
	}
}

func homePackage(name string) bool {
	value := strings.ToLower(strings.TrimSpace(name))
	if value == "" {
		return false
	}
	if _, ok := systemLauncherPackages[value]; ok || strings.HasSuffix(value, ".home") {
		return true
	}
	segment := value
	if cut := strings.LastIndex(value, "."); cut >= 0 {
		segment = value[cut+1:]
	}
	return launcherSegment(segment)
}

func launcherSegment(segment string) bool {
	index := strings.LastIndex(segment, "launcher")
	if index < 0 {
		return false
	}
	for _, char := range segment[index+len("launcher"):] {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func isHandoffSurface(_, activity string) bool {
	name := activitySimpleName(activity)
	for _, marker := range []string{"share", "chooser", "resolver", "sendintent"} {
		if strings.Contains(name, marker) {
			return true
		}
	}
	return false
}

const surfaceCooldown = 60 * time.Second

type surfaceHold struct {
	until time.Time
}

// nextSurfaceRecovery leaves a handoff page once. The same page stays quiet
// for the cooldown, including if the app opens it again. Other controls on
// the page underneath remain available, and the cooldown does not relaunch.
func nextSurfaceRecovery(now, until time.Time) (string, time.Time) {
	if !until.IsZero() && now.Before(until) {
		return "wait", until
	}
	return "back", now.Add(surfaceCooldown)
}

func coldStartAllowed(activity string) bool {
	if isKnownAdActivity(activity) {
		return true
	}
	name := activitySimpleName(activity)
	for _, marker := range []string{"mainactivity", "fragmentcontainer", "immersionactivity", "routingactivity"} {
		if strings.Contains(name, marker) {
			return false
		}
	}
	return true
}

func isVisibleBackControl(action Action) bool {
	if action.Type != "tap" || action.Backtrack {
		return false
	}
	resource := strings.ToLower(action.ResourceID)
	if strings.Contains(resource, "back_iv") || strings.Contains(resource, "navigation_iv") || strings.Contains(resource, "id/back") {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(firstNonEmpty(action.Text, action.Description))) {
	case "back", "navigate up", "返回", "返回上一页":
		return true
	default:
		return false
	}
}

func visibleBackControl(actions []Action) (Action, bool) {
	for _, action := range actions {
		if isVisibleBackControl(action) {
			return action, true
		}
	}
	return Action{}, false
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
