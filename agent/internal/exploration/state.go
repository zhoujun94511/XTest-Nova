package exploration

import (
	"sort"
	"time"

	"github.com/zhoujun94511/xtest-nova/agent/internal/execution"
)

type Step struct {
	Number        int       `json:"number"`
	StepID        string    `json:"stepId"`
	ObservationID string    `json:"observationId"`
	At            time.Time `json:"at"`
	From          string    `json:"from"`
	Action        Action    `json:"action"`
	Destination   string    `json:"destination,omitempty"`
}

type State struct {
	RequestID            string             `json:"requestId,omitempty"`
	Identity             execution.Identity `json:"identity"`
	Running              bool               `json:"running"`
	Stopping             bool               `json:"stopping,omitempty"`
	Finalizing           bool               `json:"finalizing,omitempty"`
	Execute              bool               `json:"execute"`
	EnableScroll         bool               `json:"enableScroll"`
	EnableBacktrack      bool               `json:"enableBacktrack"`
	RecoverPopups        bool               `json:"recoverPopups"`
	MaxBacktracks        int                `json:"maxBacktracks,omitempty"`
	Package              string             `json:"package,omitempty"`
	MaxSteps             int                `json:"maxSteps,omitempty"`
	DurationSeconds      int                `json:"durationSeconds,omitempty"`
	Seed                 int64              `json:"seed,omitempty"`
	StartedAt            *time.Time         `json:"startedAt,omitempty"`
	EndedAt              *time.Time         `json:"endedAt,omitempty"`
	Steps                int                `json:"steps"`
	DiscoveredStates     int                `json:"discoveredStates"`
	DiscoveredEdges      int                `json:"discoveredEdges"`
	Backtracks           int                `json:"backtracks"`
	CycleDetections      int                `json:"cycleDetections"`
	BlockedEdges         int                `json:"blockedEdges"`
	UnstableCaptures     int                `json:"unstableCaptures"`
	StaleActions         int                `json:"staleActions"`
	EffectiveActions     int                `json:"effectiveActions"`
	IneffectiveActions   int                `json:"ineffectiveActions"`
	ExternalActions      int                `json:"externalActions"`
	LastCycle            *CycleEvent        `json:"lastCycle,omitempty"`
	Scrolls              int                `json:"scrolls"`
	ScrollAttempts       int                `json:"scrollAttempts"`
	ScrollProgress       int                `json:"scrollProgress"`
	ScrollStalls         int                `json:"scrollStalls"`
	Inputs               int                `json:"inputs"`
	InputFields          int                `json:"inputFields"`
	Recoveries           int                `json:"recoveries"`
	SpecialActions       int                `json:"specialActions"`
	AdEncounters         int                `json:"adEncounters"`
	AdCloseActions       int                `json:"adCloseActions"`
	AdBackFallbacks      int                `json:"adBackFallbacks"`
	AdExternalRecoveries int                `json:"adExternalRecoveries"`
	AdRecoveries         int                `json:"adRecoveries"`
	AdWaitMillis         int64              `json:"adWaitMillis"`
	LastAdType           string             `json:"lastAdType,omitempty"`
	LastAdPhase          string             `json:"lastAdPhase,omitempty"`
	OverlaySuppressed    bool               `json:"overlaySuppressed"`
	OverlayControlError  string             `json:"overlayControlError,omitempty"`
	LastSpecial          *SpecialEvent      `json:"lastSpecial,omitempty"`
	ObservedActivities   int                `json:"observedActivities"`
	ActivityCovered      int                `json:"activityCovered"`
	ActivityTotal        int                `json:"activityTotal,omitempty"`
	ActivityCoverage     *float64           `json:"activityCoveragePercent,omitempty"`
	CurrentFingerprint   string             `json:"currentFingerprint,omitempty"`
	CurrentObservationID string             `json:"currentObservationId,omitempty"`
	LastAction           *Action            `json:"lastAction,omitempty"`
	StopReason           string             `json:"stopReason,omitempty"`
	Error                string             `json:"error,omitempty"`
	ArtifactDir          string             `json:"artifactDir,omitempty"`
	EvidenceIndexPath    string             `json:"evidenceIndexPath,omitempty"`
}

type CycleEvent struct {
	At          time.Time `json:"at"`
	Period      int       `json:"period,omitempty"`
	Repetitions int       `json:"repetitions"`
	From        string    `json:"from"`
	Action      string    `json:"action"`
	To          string    `json:"to"`
	Reason      string    `json:"reason"`
}

type Graph struct {
	States []GraphState `json:"states"`
	Edges  []GraphEdge  `json:"edges"`
}

type GraphState struct {
	Fingerprint string `json:"fingerprint"`
	Activity    string `json:"activity,omitempty"`
	Parent      string `json:"parent,omitempty"`
	NodeCount   int    `json:"nodeCount"`
	ActionCount int    `json:"actionCount"`
}

type GraphEdge struct {
	From     string `json:"from"`
	ActionID string `json:"actionId"`
	To       string `json:"to"`
}

type graphNode struct {
	analysis            Analysis
	activity            string
	semantic            string
	tried               map[string]bool
	actionAttempts      map[string]int
	nonScrollSelections int
}

func (m *Manager) State() State {
	m.mu.Lock()
	defer m.mu.Unlock()
	return cloneState(m.state)
}

func (m *Manager) Graph() Graph {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.graphLocked()
}

func (m *Manager) graphLocked() Graph {
	graph := Graph{States: make([]GraphState, 0, len(m.nodes)), Edges: append([]GraphEdge(nil), m.edges...)}
	for fingerprint, node := range m.nodes {
		graph.States = append(graph.States, GraphState{Fingerprint: fingerprint, Activity: node.activity, Parent: m.parents[fingerprint], NodeCount: node.analysis.NodeCount, ActionCount: len(node.analysis.Actions)})
	}
	sort.Slice(graph.States, func(i, j int) bool { return graph.States[i].Fingerprint < graph.States[j].Fingerprint })
	return graph
}

func (m *Manager) Steps() []Step {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Step(nil), m.steps...)
}

func (m *Manager) refreshCountsLocked() {
	m.state.DiscoveredStates = len(m.nodes)
	m.state.DiscoveredEdges = len(m.edges)
	m.state.ObservedActivities = len(m.activities)
	m.state.ActivityCovered = len(m.activities)
	m.state.InputFields = len(m.inputFields)
	m.state.ActivityTotal = len(m.config.ExpectedActivities)
	m.state.ActivityCoverage = nil
	if m.state.ActivityTotal > 0 {
		covered := 0
		for _, activity := range m.config.ExpectedActivities {
			if _, exists := m.activities[activity]; exists {
				covered++
			}
		}
		m.state.ActivityCovered = covered
		percent := float64(covered) * 100 / float64(m.state.ActivityTotal)
		m.state.ActivityCoverage = &percent
	}
}

func (m *Manager) blockEdgeLocked(state, action string) {
	key := explorationEdgeKey(state, action)
	if !m.blockedEdges[key] {
		m.blockedEdges[key] = true
		m.state.BlockedEdges++
	}
}

func cloneState(state State) State {
	if state.LastCycle != nil {
		last := *state.LastCycle
		state.LastCycle = &last
	}
	if state.LastAction != nil {
		last := *state.LastAction
		state.LastAction = &last
	}
	if state.LastSpecial != nil {
		special := *state.LastSpecial
		if special.Action != nil {
			action := *special.Action
			special.Action = &action
		}
		state.LastSpecial = &special
	}
	if state.ActivityCoverage != nil {
		coverage := *state.ActivityCoverage
		state.ActivityCoverage = &coverage
	}
	if state.LastSpecial != nil {
		last := *state.LastSpecial
		if last.Action != nil {
			action := *last.Action
			last.Action = &action
		}
		state.LastSpecial = &last
	}
	return state
}
