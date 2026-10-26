package exploration

import "time"

// stuckNoProgressLimit is the number of actions that stay on one semantic
// state before that state is stuck. Leaving then uses one back and, if the
// state remains, one cold start. The cooldown is the existing stall clock.
const stuckNoProgressLimit = 2

func noteSemanticProgress(loop *runLoop, semantic, activity string, progressed bool) {
	if loop == nil || semantic == "" {
		return
	}
	if progressed {
		loop.stuckSemantic = ""
		loop.stuckActivity = ""
		loop.stuckBackSent = ""
		loop.stuckReady = false
		return
	}
	if loop.noProgress == nil {
		loop.noProgress = map[string]int{}
	}
	loop.noProgress[semantic]++
	if loop.noProgress[semantic] >= stuckNoProgressLimit {
		loop.stuckSemantic = semantic
		loop.stuckActivity = activity
		loop.stuckReady = true
	}
}

func pageIsStuck(loop *runLoop, activity, semantic string) bool {
	if loop == nil || !loop.stuckReady {
		return false
	}
	if loop.stuckSemantic != "" && loop.stuckSemantic == semantic {
		return true
	}
	return loop.stuckActivity == activity
}

func stuckLeave(loop *runLoop, semantic string, analysis Analysis) chosenAction {
	if loop.stuckBackSent != semantic {
		loop.stuckBackSent = semantic
		if back, ok := visibleBackControl(analysis.Actions); ok {
			return chosenAction{action: back, signal: loopNext}
		}
		return chosenAction{action: Action{Type: "back", ID: "back"}, signal: loopNext}
	}
	if loop.escalationCoolState == semantic && time.Now().Before(loop.escalationCooldownUntil) {
		return chosenAction{signal: loopContinue}
	}
	loop.escalationCoolState = semantic
	loop.escalationCooldownUntil = time.Now().Add(stallEscalationCooldown)
	loop.stuckRelaunch = true
	return chosenAction{signal: loopRestart}
}
