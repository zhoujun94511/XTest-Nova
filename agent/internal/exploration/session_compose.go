package exploration

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const boundedRenderAttempts = 5
const targetReplayRetryDelay = 10 * time.Second

// SetTargetReplay installs the target-page replay used by the current session.
// The traversal loop calls it when the foreground activity matches, then continues.
func (m *Manager) SetTargetReplay(replay func(context.Context, string, string, string) error) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.targetReplay = replay
	m.mu.Unlock()
}

func (m *Manager) applySessionGuards(ctx context.Context, config Config, loop *runLoop, activity, hierarchy string, analysis *Analysis) loopSignal {
	m.mu.Lock()
	elapsed := m.durationElapsedLocked(config)
	if elapsed {
		m.finishLocked("duration_elapsed", "")
	}
	m.mu.Unlock()
	if elapsed {
		return loopStop
	}
	if signal := m.stopForLowBattery(ctx, config, loop); signal != loopNext {
		return signal
	}
	if signal := m.leaveGuardedActivity(ctx, config, activity); signal != loopNext {
		return signal
	}
	if signal := m.replayTargetPage(ctx, config, loop, activity); signal != loopNext {
		return signal
	}
	filterBlockedControls(analysis, config.ControlBlacklist)
	if signal := m.coordinateWhenUnlabeled(ctx, config, loop, activity, hierarchy, analysis); signal != loopNext {
		return signal
	}
	return loopNext
}

func (m *Manager) stopForLowBattery(ctx context.Context, config Config, loop *runLoop) loopSignal {
	if !config.LowBatteryExit || loop == nil || time.Now().Before(loop.nextBattery) {
		return loopNext
	}
	loop.nextBattery = time.Now().Add(30 * time.Second)
	output, err := m.executor.Run(ctx, "dumpsys", "battery")
	if err != nil {
		return loopNext
	}
	level, ok := batteryLevel(output)
	if ok && level <= config.MinBattery {
		m.finish("low_battery", "")
		return loopStop
	}
	return loopNext
}

func (m *Manager) leaveGuardedActivity(ctx context.Context, config Config, activity string) loopSignal {
	if activityAllowed(config, activity) {
		return loopNext
	}
	if err := m.executeAction(ctx, Action{Type: "back"}); err != nil {
		m.finish("safety_stop", err.Error())
		return loopStop
	}
	return loopContinue
}

func (m *Manager) replayTargetPage(ctx context.Context, config Config, loop *runLoop, activity string) loopSignal {
	m.mu.Lock()
	replay := m.targetReplay
	if m.playedTargets == nil {
		m.playedTargets = map[string]bool{}
	}
	if m.targetReplayAfter == nil {
		m.targetReplayAfter = map[string]time.Time{}
	}
	var target TargetCase
	key := ""
	found := false
	now := time.Now()
	if replay != nil {
		for _, candidate := range config.TargetCases {
			if !activityListed(config.Package, activity, []string{candidate.Activity}) {
				continue
			}
			candidateKey := candidate.Activity + "\x00" + candidate.Task + "\x00" + candidate.Case
			if m.playedTargets[candidateKey] || now.Before(m.targetReplayAfter[candidateKey]) {
				continue
			}
			target = candidate
			key = candidateKey
			found = true
			break
		}
	}
	m.mu.Unlock()
	if !found {
		return loopNext
	}
	if err := replay(ctx, target.Activity, target.Task, target.Case); err != nil {
		if ctx.Err() != nil {
			return loopStop
		}
		m.mu.Lock()
		m.targetReplayAfter[key] = time.Now().Add(targetReplayRetryDelay)
		m.mu.Unlock()
		return loopNext
	}
	m.mu.Lock()
	m.playedTargets[key] = true
	m.mu.Unlock()
	m.recordSessionAction(loop, activity, Action{Type: "replay", Text: target.Case, Description: target.Activity})
	if ctx.Err() != nil {
		return loopStop
	}
	return loopContinue
}

func (m *Manager) recordSessionAction(loop *runLoop, from string, action Action) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.state.Running {
		return
	}
	number := len(m.steps) + 1
	m.steps = append(m.steps, Step{
		Number: number,
		StepID: fmt.Sprintf("%s:session-%06d", m.state.RequestID, number),
		At:     time.Now().UTC(),
		From:   from,
		Action: action,
	})
	if loop != nil {
		loop.pending = len(m.steps) - 1
		loop.pendingNoteBlocked = false
	}
	m.state.Steps = len(m.steps)
	recorded := action
	m.state.LastAction = &recorded
	if action.Type == "swipe" {
		m.state.Scrolls++
		m.state.ScrollAttempts++
	}
}

func (m *Manager) coordinateWhenUnlabeled(ctx context.Context, config Config, loop *runLoop, activity, hierarchy string, analysis *Analysis) loopSignal {
	if !renderFallbackEnabled(config.RenderFallbackMode) || analysis == nil || len(analysis.Actions) > 0 || !renderSurface(hierarchy) {
		return loopNext
	}
	if loop.renderActivity != activity {
		loop.renderActivity = activity
		loop.renderAttempts = 0
	}
	if config.RenderFallbackMode == "bounded" && loop.renderAttempts >= boundedRenderAttempts {
		return loopNext
	}
	width, height := loop.screenWidth, loop.screenHeight
	if width == 0 || height == 0 {
		output, err := m.executor.Run(ctx, "wm", "size")
		if err != nil {
			return loopNext
		}
		width, height, _ = screenSize(output)
		loop.screenWidth, loop.screenHeight = width, height
	}
	gesture := coordinateGesture(config.Seed, loop.renderAttempts, width, height)
	if gesture.Type == "" {
		return loopNext
	}
	loop.renderAttempts++
	if err := m.executeAction(ctx, gesture); err != nil {
		m.finish("safety_stop", err.Error())
		return loopStop
	}
	m.recordSessionAction(loop, activity, gesture)
	return loopContinue
}

func activityAllowed(config Config, activity string) bool {
	switch config.ActivityMode {
	case "allowlist":
		return activityListed(config.Package, activity, config.Activities)
	case "blocklist":
		return !activityListed(config.Package, activity, config.Activities)
	default:
		return true
	}
}

func activityListed(packageName, activity string, names []string) bool {
	class := activity
	if slash := strings.LastIndex(activity, "/"); slash >= 0 {
		class = activity[slash+1:]
	}
	short := class
	if dot := strings.LastIndex(class, "."); dot >= 0 {
		short = class[dot+1:]
	}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if name == activity || name == class || name == short {
			return true
		}
		if packageName != "" && strings.HasPrefix(name, ".") && packageName+name == class {
			return true
		}
	}
	return false
}

func filterBlockedControls(analysis *Analysis, blocked []string) {
	if analysis == nil || len(blocked) == 0 || len(analysis.Actions) == 0 {
		return
	}
	kept := analysis.Actions[:0]
	for _, action := range analysis.Actions {
		if !controlBlocked(action, blocked) {
			kept = append(kept, action)
		}
	}
	analysis.Actions = kept
}

func controlBlocked(action Action, blocked []string) bool {
	candidate := strings.ToLower(strings.Join([]string{action.Text, action.Description, action.ResourceID}, " "))
	for _, item := range blocked {
		item = strings.ToLower(strings.TrimSpace(item))
		if item != "" && strings.Contains(candidate, item) {
			return true
		}
	}
	return false
}

func renderFallbackEnabled(mode string) bool {
	return mode == "bounded" || mode == "continuous"
}

func renderSurface(hierarchy string) bool {
	return strings.Contains(hierarchy, "android.view.SurfaceView") || strings.Contains(hierarchy, "UnityPlayer")
}

func batteryLevel(output string) (int, bool) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "level:") {
			continue
		}
		value, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "level:")))
		return value, err == nil
	}
	return 0, false
}

func screenSize(output string) (int, int, bool) {
	chosen := ""
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Override size:") {
			chosen = strings.TrimSpace(strings.TrimPrefix(line, "Override size:"))
		} else if chosen == "" && strings.HasPrefix(line, "Physical size:") {
			chosen = strings.TrimSpace(strings.TrimPrefix(line, "Physical size:"))
		}
	}
	parts := strings.Split(chosen, "x")
	if len(parts) != 2 {
		return 0, 0, false
	}
	width, widthErr := strconv.Atoi(parts[0])
	height, heightErr := strconv.Atoi(parts[1])
	if widthErr != nil || heightErr != nil || width <= 0 || height <= 0 {
		return 0, 0, false
	}
	return width, height, true
}

func coordinateGesture(seed int64, attempt, width, height int) Action {
	left, top := width/10, height/6
	right, bottom := width-left, height-height/10
	spanX, spanY := right-left, bottom-top
	if spanX < 1 || spanY < 1 {
		return Action{}
	}
	n := seed + int64(attempt)*17
	if n < 0 {
		n = -n
	}
	x := left + int(n%int64(spanX))
	y := top + int((n/7)%int64(spanY))
	if attempt%3 == 2 {
		return Action{Type: "swipe", X: x, Y: top + spanY*3/4, EndX: x, EndY: top + spanY/4}
	}
	return Action{Type: "tap", X: x, Y: y}
}
