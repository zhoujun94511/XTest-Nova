package exploration

import (
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/zhoujun94511/xtest-nova/agent/internal/execution"
)

func (m *Manager) recordPageLocked(config Config, analysis *Analysis, activity string, loop *runLoop) *graphNode {
	analysis.ObservationID = observationID(config.RequestID, len(m.steps)+1, analysis.Fingerprint)
	node := m.nodes[analysis.Fingerprint]
	newState := node == nil
	currentSemantic := semanticStateKey(*analysis, activity)
	if !m.seenSemantic[currentSemantic] {
		if len(m.seenSemantic) > 0 {
			m.cycles.resetProgressWindow()
		}
		m.seenSemantic[currentSemantic] = true
	}
	if node == nil {
		node = &graphNode{analysis: *analysis, activity: activity, semantic: currentSemantic, tried: map[string]bool{}, actionAttempts: map[string]int{}}
		m.nodes[analysis.Fingerprint] = node
	}
	for _, candidate := range analysis.Actions {
		if candidate.Type == "input" {
			m.inputFields[inputFieldKey(activity, candidate)] = true
		}
	}
	if activity != "" {
		m.activities[activity] = struct{}{}
		if node.activity == "" {
			node.activity = activity
		}
	}
	if loop.pending >= 0 {
		pendingStep := &m.steps[loop.pending]
		pendingStep.Destination = analysis.Fingerprint
		m.receipts.Finish(pendingStep.StepID, execution.ReceiptExecuted, "", "", observationID(config.RequestID, pendingStep.Number+1, analysis.Fingerprint), analysis.Fingerprint)
		m.edges = append(m.edges, GraphEdge{From: pendingStep.From, ActionID: pendingStep.Action.ID, To: analysis.Fingerprint})
		if pendingStep.Action.Type == "swipe" && !pendingStep.Action.Backtrack {
			if pendingStep.From == analysis.Fingerprint {
				m.state.ScrollStalls++
			} else {
				m.state.ScrollProgress++
			}
		}
		fromSemantic := pendingStep.From
		if fromNode := m.nodes[pendingStep.From]; fromNode != nil {
			fromSemantic = fromNode.semantic
		}
		if fromSemantic == currentSemantic {
			m.state.IneffectiveActions++
		} else {
			m.state.EffectiveActions++
		}
		actionKey := semanticActionKey(pendingStep.Action)
		if pendingStep.Action.Backtrack {
			actionKey = "backtrack"
			expected := m.parents[pendingStep.From]
			if expected == "" || expected != analysis.Fingerprint {
				m.blockEdgeLocked(fromSemantic, actionKey)
			}
		} else if m.activityMemory != nil && activity != "" && !loop.pendingNoteBlocked {
			m.activityMemory.note(actionKey, activity)
			if len(m.steps)%20 == 0 {
				_ = m.activityMemory.save()
			}
		}
		cycleFrom, cycleTo := fromSemantic, currentSemantic
		fromActivity := ""
		if fromNode := m.nodes[pendingStep.From]; fromNode != nil {
			fromActivity = fromNode.activity
		}
		if fromActivity != "" && activity != "" && fromActivity != activity {
			cycleFrom, cycleTo = "activity:"+fromActivity, "activity:"+activity
		}
		if period := observeExplorationCycle(&m.cycles, pendingStep.Action, cycleFrom, actionKey, cycleTo, fromSemantic); period > 0 {
			m.state.CycleDetections++
			reportedPeriod, reason := period, "repeated_cycle"
			if period > cycleMaxPeriod {
				reportedPeriod, reason = 0, "repeated_transition"
			}
			m.state.LastCycle = &CycleEvent{At: time.Now().UTC(), Period: reportedPeriod, Repetitions: cycleRepetitions, From: cycleFrom, Action: actionKey, To: cycleTo, Reason: reason}
			if period <= cycleMaxPeriod && len(m.cycles.history) >= period {
				for _, sample := range m.cycles.history[len(m.cycles.history)-period:] {
					m.blockEdgeLocked(sample.blockFrom, sample.action)
				}
			} else {
				m.blockEdgeLocked(fromSemantic, actionKey)
			}
			if armStallEscalation(currentSemantic, m.state.Steps, loop.escalationCooldownUntil, loop.escalationCoolState) {
				loop.escalationArmed = currentSemantic
			}
		}
		if !pendingStep.Action.Backtrack && newState && pendingStep.From != analysis.Fingerprint {
			m.parents[analysis.Fingerprint] = pendingStep.From
			m.returns[analysis.Fingerprint] = inverseAction(pendingStep.Action, analysis.Fingerprint, pendingStep.From)
		}
		loop.pending = -1
		loop.pendingNoteBlocked = false
	}
	if m.root == "" {
		m.root = analysis.Fingerprint
	}
	m.state.CurrentFingerprint = analysis.Fingerprint
	m.state.CurrentObservationID = analysis.ObservationID
	m.refreshCountsLocked()
	return node
}

func observeExplorationCycle(guard *cycleGuard, action Action, from, actionKey, to, blockFrom string) int {
	// A bounded input corpus intentionally produces several transitions on the
	// same page. Treating those as navigation cycles creates false positives;
	// input repetition is controlled independently by inputTried.
	if action.Type == "input" {
		return 0
	}
	// A feed pager keeps the same controls while the video changes. That swipe
	// is already limited by the scroll attempt budget; blocking it as a cycle
	// ends the session after a few identical pages.
	if action.Type == "swipe" && !action.Backtrack && from == to {
		return 0
	}
	return guard.observe(from, actionKey, to, blockFrom)
}

func inputExecutionKey(activity string, action Action) string {
	return activity + "\x00" + action.ID
}

func inputFieldKey(activity string, action Action) string {
	identity := stableInputFieldIdentity(action.ResourceID, action.Description, action.Class, action.Description, action.Bounds)
	return activity + "\x00" + identity
}

func backAction(from, parent string) Action {
	sum := sha256.Sum256([]byte("back:" + from + ":" + parent))
	return Action{ID: fmt.Sprintf("%x", sum[:8]), Type: "back", Description: "DFS backtrack", Backtrack: true}
}

func inverseAction(action Action, from, parent string) Action {
	if action.Type != "swipe" {
		return backAction(from, parent)
	}
	sum := sha256.Sum256([]byte("reverse-swipe:" + from + ":" + parent + ":" + action.ID))
	return Action{
		ID: fmt.Sprintf("%x", sum[:8]), Type: "swipe", Direction: "backward", Backtrack: true,
		X: action.EndX, Y: action.EndY, EndX: action.X, EndY: action.Y,
		Bounds: action.Bounds, ResourceID: action.ResourceID, Class: action.Class, Description: "DFS scroll backtrack",
	}
}
