package exploration

import "time"

const (
	minimumExhaustedCapturesBeforeStop = 2
	maximumConsecutiveUnstableCaptures = 20
	maximumConsecutiveCaptureFailures  = 5
)

type chosenAction struct {
	action Action
	signal loopSignal
}

func (m *Manager) chooseActionLocked(config Config, node *graphNode, analysis Analysis, activity string, loop *runLoop) chosenAction {
	if isStartupLanguageSurface(activity) && !pageIsStuck(loop, activity, node.semantic) {
		if preferred, ok := preferredStartupLanguage(analysis.Actions); ok {
			if !m.seenActivityAction(activity, semanticActionKey(preferred)) {
				m.markActivityAction(activity, preferred)
				return chosenAction{action: preferred, signal: loopNext}
			}
			accepted := loop != nil && loop.startupLanguageAccepted[activity] == semanticActionKey(preferred)
			if accepted {
				if confirm, found := startupLanguageConfirm(analysis.Actions); found && !m.seenActivityAction(activity, semanticActionKey(confirm)) {
					m.markActivityAction(activity, confirm)
					return chosenAction{action: confirm, signal: loopNext}
				}
			}
		}
	}
	blocked := func(action Action) bool {
		if isLanguageSwitch(action) || isLanguageChoice(activity, action) || blocksNamedLanguageResource(activity, analysis.Actions, action) || blocksLanguagePickerTap(activity, analysis.Actions, action) {
			return true
		}
		if isLanguageSettingsActivity(activity) && action.Type != "swipe" && action.Type != "back" && !isVisibleBackControl(action) {
			return true
		}
		if loop.purchaseBlocked && isPurchaseControl(action) {
			return true
		}
		return m.blockedEdges[explorationEdgeKey(node.semantic, semanticActionKey(action))] || (action.Type == "input" && m.inputTried[inputExecutionKey(activity, action)])
	}
	// Cover controls that this activity has not executed yet. External account
	// actions stay out of this pass so a sign-in button does not outrank the
	// rest of the page. The ordinary pass still sees them when nothing else remains.
	// A new fingerprint must not reopen a tap this activity already executed.
	seenTap := func(action Action) bool {
		return blocked(action) || (action.Type == "tap" && m.seenActivityAction(activity, semanticActionKey(action)))
	}
	preferred := func(action Action) bool {
		return seenTap(action) || isExternalRisk(action)
	}
	if action, ok := selectActionBlocked(node, config.Seed, config.EnableScroll, config.RecoverPopups, preferred, m.activityMemory, m.activities); ok {
		m.markActivityAction(activity, action)
		return chosenAction{action: action, signal: loopNext}
	}
	action, ok := selectActionBlocked(node, config.Seed, config.EnableScroll, config.RecoverPopups, seenTap, m.activityMemory, m.activities)
	if ok {
		m.markActivityAction(activity, action)
		return chosenAction{action: action, signal: loopNext}
	}
	returnAction, hasReturn := m.returns[analysis.Fingerprint]
	returnBlocked := m.blockedEdges[explorationEdgeKey(node.semantic, "backtrack")]
	if config.EnableBacktrack && hasReturn && !returnBlocked && m.returnAttempts[analysis.Fingerprint] == 0 && m.state.Backtracks < config.MaxBacktracks {
		if loop.escalationArmed == node.semantic {
			coolEscalation(loop, node.semantic)
		}
		m.returnAttempts[analysis.Fingerprint]++
		return chosenAction{action: returnAction, signal: loopNext}
	}
	if loop.escalationArmed == node.semantic && config.EnableBacktrack && hasReturn && m.state.Backtracks < config.MaxBacktracks {
		coolEscalation(loop, node.semantic)
		m.returnAttempts[analysis.Fingerprint]++
		return chosenAction{action: returnAction, signal: loopNext}
	}
	if loop.escalationArmed == node.semantic {
		coolEscalation(loop, node.semantic)
	}
	if loop.exhaustedFingerprint != analysis.Fingerprint {
		loop.exhaustedFingerprint = analysis.Fingerprint
		loop.exhaustedCaptures = 1
	} else {
		loop.exhaustedCaptures++
	}
	if pageIsStuck(loop, activity, node.semantic) {
		if m.durationElapsedLocked(config) {
			m.finishLocked("duration_elapsed", "")
			return chosenAction{signal: loopStop}
		}
		return stuckLeave(loop, node.semantic, analysis)
	}
	if loop.exhaustedCaptures < minimumExhaustedCapturesBeforeStop {
		return chosenAction{signal: loopContinue}
	}
	if m.durationElapsedLocked(config) {
		m.finishLocked("duration_elapsed", "")
		return chosenAction{signal: loopStop}
	}
	if time.Now().Before(loop.ladderCooldownUntil) {
		return chosenAction{signal: loopContinue}
	}
	m.resetSelectableRoundLocked()
	loop.exhaustedFingerprint = ""
	loop.exhaustedCaptures = 0
	action, ok = selectActionBlocked(node, config.Seed, config.EnableScroll, config.RecoverPopups, seenTap, m.activityMemory, m.activities)
	if ok {
		m.markActivityAction(activity, action)
		return chosenAction{action: action, signal: loopNext}
	}
	if back, backOK := visibleBackControl(analysis.Actions); backOK && loop.backEscapeState != node.semantic {
		loop.backEscapeState = node.semantic
		return chosenAction{action: back, signal: loopNext}
	}
	loop.allowColdStart = false
	return chosenAction{signal: loopRestart}
}

func (m *Manager) seenActivityAction(activity, key string) bool {
	return m.activityActionSeen[activity][key]
}

func (m *Manager) markActivityAction(activity string, action Action) {
	if action.Type != "tap" {
		return
	}
	if m.activityActionSeen == nil {
		m.activityActionSeen = map[string]map[string]bool{}
	}
	if m.activityActionSeen[activity] == nil {
		m.activityActionSeen[activity] = map[string]bool{}
	}
	m.activityActionSeen[activity][semanticActionKey(action)] = true
}

func coolEscalation(loop *runLoop, semantic string) {
	loop.escalationArmed = ""
	loop.escalationCoolState = semantic
	loop.escalationCooldownUntil = time.Now().Add(stallEscalationCooldown)
}

func (m *Manager) stopIfDurationElapsed(config Config) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.durationElapsedLocked(config) {
		return false
	}
	m.finishLocked("duration_elapsed", "")
	return true
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
