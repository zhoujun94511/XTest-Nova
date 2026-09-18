package exploration

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

const (
	cycleHistoryLimit       = 64
	cycleMaxPeriod          = 8
	cycleRepetitions        = 3
	repeatedTransitionLimit = 4
)

type transitionSample struct {
	from, action, to string
	blockFrom        string
}

func (s transitionSample) cycleKey() string { return s.from + "->" + s.to }

type cycleGuard struct {
	history    []transitionSample
	pairCounts map[string]int
}

func (g *cycleGuard) resetProgressWindow() {
	g.history = nil
	g.pairCounts = nil
}

func (g *cycleGuard) observe(from, action, to, blockFrom string) int {
	sample := transitionSample{from: from, action: action, to: to, blockFrom: blockFrom}
	g.history = append(g.history, sample)
	if g.pairCounts == nil {
		g.pairCounts = map[string]int{}
	}
	g.pairCounts[sample.cycleKey()]++
	if len(g.history) > cycleHistoryLimit {
		g.history = append([]transitionSample(nil), g.history[len(g.history)-cycleHistoryLimit:]...)
	}
	for period := 1; period <= cycleMaxPeriod; period++ {
		required := period * cycleRepetitions
		if len(g.history) < required {
			continue
		}
		start := len(g.history) - required
		repeated := true
		for index := start + period; index < len(g.history); index++ {
			if g.history[index].cycleKey() != g.history[index-period].cycleKey() {
				repeated = false
				break
			}
		}
		if repeated {
			return period
		}
	}
	if g.pairCounts[sample.cycleKey()] >= repeatedTransitionLimit {
		// A repeated semantic transition is a non-consecutive cycle signal. The
		// sentinel is greater than cycleMaxPeriod so callers can treat it as a
		// detection without confusing it with a measured contiguous period.
		return cycleMaxPeriod + 1
	}
	return 0
}

func semanticActionKey(action Action) string {
	label := firstNonEmpty(action.Text, action.Description)
	return strings.Join([]string{action.Type, action.ResourceID, action.Class, stableText(label)}, "|")
}

func semanticStateKey(analysis Analysis, activity string) string {
	actions := make([]string, 0, len(analysis.Actions))
	for _, action := range analysis.Actions {
		actions = append(actions, semanticActionKey(action))
	}
	// Cycle identity deliberately ignores coordinates, node indices and passive
	// page content. Exact fingerprints still preserve that detail for exploration;
	// this coarser identity prevents loading churn from hiding a navigation loop.
	sort.Strings(actions)
	sum := sha256.Sum256([]byte(activity + "\n" + strings.Join(actions, "\n")))
	return hex.EncodeToString(sum[:12])
}

func explorationEdgeKey(state, action string) string { return state + "\x00" + action }
