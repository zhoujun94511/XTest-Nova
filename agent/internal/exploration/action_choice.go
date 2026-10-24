package exploration

import "time"

const (
	minimumExhaustedCapturesBeforeStop = 2
	maximumConsecutiveUnstableCaptures = 20
)

type chosenAction struct {
	action Action
	signal loopSignal
}

func (m *Manager) chooseActionLocked(config Config, node *graphNode, analysis Analysis, activity string, loop *runLoop) chosenAction {
	action, ok := selectActionBlocked(node, config.Seed, config.EnableScroll, config.RecoverPopups, func(action Action) bool {
		return m.blockedEdges[explorationEdgeKey(node.semantic, semanticActionKey(action))] || (action.Type == "input" && m.inputTried[inputExecutionKey(activity, action)])
	}, m.activityMemory, m.activities)
	if ok {
		return chosenAction{action: action, signal: loopNext}
	}
	returnAction, hasReturn := m.returns[analysis.Fingerprint]
	returnBlocked := m.blockedEdges[explorationEdgeKey(node.semantic, "backtrack")]
	if config.EnableBacktrack && hasReturn && !returnBlocked && m.returnAttempts[analysis.Fingerprint] == 0 && m.state.Backtracks < config.MaxBacktracks {
		if loop.escalationArmed == node.semantic {
			loop.escalationArmed = ""
			loop.escalationCoolState = node.semantic
			loop.escalationCooldownUntil = m.state.Steps + stallEscalationCooldownSteps
		}
		m.returnAttempts[analysis.Fingerprint]++
		return chosenAction{action: returnAction, signal: loopNext}
	}
	if loop.escalationArmed == node.semantic && config.EnableBacktrack && hasReturn && m.state.Backtracks < config.MaxBacktracks {
		loop.escalationArmed = ""
		loop.escalationCoolState = node.semantic
		loop.escalationCooldownUntil = m.state.Steps + stallEscalationCooldownSteps
		m.returnAttempts[analysis.Fingerprint]++
		return chosenAction{action: returnAction, signal: loopNext}
	}
	if loop.escalationArmed == node.semantic {
		loop.escalationArmed = ""
		loop.escalationCoolState = node.semantic
		loop.escalationCooldownUntil = m.state.Steps + stallEscalationCooldownSteps
	}
	if loop.exhaustedFingerprint != analysis.Fingerprint {
		loop.exhaustedFingerprint = analysis.Fingerprint
		loop.exhaustedCaptures = 1
	} else {
		loop.exhaustedCaptures++
	}
	if loop.exhaustedCaptures < minimumExhaustedCapturesBeforeStop {
		return chosenAction{signal: loopContinue}
	}
	if m.durationElapsedLocked(config) {
		m.finishLocked("duration_elapsed", "")
		return chosenAction{signal: loopStop}
	}
	if config.DurationSeconds > 0 {
		m.resetSelectableRoundLocked()
		loop.exhaustedFingerprint = ""
		loop.exhaustedCaptures = 0
		action, ok = selectActionBlocked(node, config.Seed, config.EnableScroll, config.RecoverPopups, func(action Action) bool {
			return m.blockedEdges[explorationEdgeKey(node.semantic, semanticActionKey(action))] || (action.Type == "input" && m.inputTried[inputExecutionKey(activity, action)])
		}, m.activityMemory, m.activities)
		if ok {
			return chosenAction{action: action, signal: loopNext}
		}
		return chosenAction{signal: loopRestart}
	}
	reason := "state_exhausted"
	if analysis.Fingerprint == m.root {
		reason = "graph_exhausted"
	}
	m.finishLocked(reason, "")
	return chosenAction{signal: loopStop}
}

func stepBudgetReached(config Config, steps int) bool {
	if config.DurationSeconds > 0 {
		return false
	}
	return config.MaxSteps > 0 && steps >= config.MaxSteps
}

func (m *Manager) durationElapsedLocked(config Config) bool {
	if config.DurationSeconds <= 0 || m.state.StartedAt == nil {
		return false
	}
	return !time.Now().Before(m.state.StartedAt.Add(time.Duration(config.DurationSeconds) * time.Second))
}

func (m *Manager) resetSelectableRoundLocked() {
	for _, node := range m.nodes {
		node.tried = map[string]bool{}
		node.actionAttempts = map[string]int{}
		node.nonScrollSelections = 0
	}
	m.returnAttempts = map[string]int{}
	m.inputTried = map[string]bool{}
}
