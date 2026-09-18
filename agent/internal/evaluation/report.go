package evaluation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/zhoujun94511/xtest-nova/agent/internal/exploration"
	"github.com/zhoujun94511/xtest-nova/agent/internal/runner"
)

const SchemaVersion = "xtest-evaluation/v1"

type Metrics struct {
	ObservedActivities *int     `json:"observedActivities,omitempty"`
	ActivityCovered    *int     `json:"activityCovered,omitempty"`
	ActivityTotal      *int     `json:"activityTotal,omitempty"`
	ActivityCoverage   *float64 `json:"activityCoveragePercent,omitempty"`
	StatesDiscovered   *int     `json:"statesDiscovered,omitempty"`
	EdgesDiscovered    *int     `json:"edgesDiscovered,omitempty"`
	Steps              *int     `json:"steps,omitempty"`
	Taps               *int     `json:"taps,omitempty"`
	Scrolls            *int     `json:"scrolls,omitempty"`
	ScrollAttempts     *int     `json:"scrollAttempts,omitempty"`
	ScrollProgress     *int     `json:"scrollProgress,omitempty"`
	ScrollStalls       *int     `json:"scrollStalls,omitempty"`
	Inputs             *int     `json:"inputs,omitempty"`
	InputFields        *int     `json:"inputFields,omitempty"`
	Backtracks         *int     `json:"backtracks,omitempty"`
	Recoveries         *int     `json:"recoveries,omitempty"`
	SpecialActions     *int     `json:"specialActions,omitempty"`
	MotionEvents       *int     `json:"motionEvents,omitempty"`
	SafetyStops        *int     `json:"safetyStops,omitempty"`
	DangerousActions   *int     `json:"dangerousActions,omitempty"`
}

type RunReport struct {
	SchemaVersion  string     `json:"schemaVersion"`
	Engine         string     `json:"engine"`
	EngineVersion  string     `json:"engineVersion,omitempty"`
	Scenario       string     `json:"scenario"`
	Package        string     `json:"package"`
	Seed           int64      `json:"seed,omitempty"`
	StartedAt      *time.Time `json:"startedAt,omitempty"`
	EndedAt        *time.Time `json:"endedAt,omitempty"`
	DurationMillis *int64     `json:"durationMillis,omitempty"`
	Completed      bool       `json:"completed"`
	Crashed        bool       `json:"crashed"`
	StopReason     string     `json:"stopReason,omitempty"`
	Error          string     `json:"error,omitempty"`
	SequenceDigest string     `json:"sequenceDigest,omitempty"`
	Metrics        Metrics    `json:"metrics"`
	Evidence       []string   `json:"evidence,omitempty"`
	EvidenceIndex  string     `json:"evidenceIndex,omitempty"`
	Verdict        Verdict    `json:"verdict"`
}

// GoldenReport contains only reproducible behavior and identity fields. Runtime
// timestamps, durations and evidence locations remain in RunReport for audit,
// but are intentionally excluded from byte-for-byte golden comparisons.
type GoldenReport struct {
	SchemaVersion  string  `json:"schemaVersion"`
	Engine         string  `json:"engine"`
	EngineVersion  string  `json:"engineVersion,omitempty"`
	Scenario       string  `json:"scenario"`
	Package        string  `json:"package"`
	Seed           int64   `json:"seed,omitempty"`
	Completed      bool    `json:"completed"`
	Crashed        bool    `json:"crashed"`
	StopReason     string  `json:"stopReason,omitempty"`
	Error          string  `json:"error,omitempty"`
	SequenceDigest string  `json:"sequenceDigest,omitempty"`
	Metrics        Metrics `json:"metrics"`
}

func NormalizeForGolden(report RunReport) GoldenReport {
	return GoldenReport{
		SchemaVersion: report.SchemaVersion, Engine: report.Engine, EngineVersion: report.EngineVersion,
		Scenario: report.Scenario, Package: report.Package, Seed: report.Seed, Completed: report.Completed,
		Crashed: report.Crashed, StopReason: report.StopReason, Error: report.Error,
		SequenceDigest: report.SequenceDigest, Metrics: report.Metrics,
	}
}

func FromNova(scenario, engineVersion string, state exploration.State, graph exploration.Graph, steps []exploration.Step) RunReport {
	taps := 0
	for _, step := range steps {
		if step.Action.Type == "tap" {
			taps++
		}
	}
	safetyStops := 0
	if state.StopReason == "safety_stop" {
		safetyStops = 1
	}
	dangerousActions := 0
	report := RunReport{
		SchemaVersion: SchemaVersion, Engine: "nova", EngineVersion: engineVersion,
		Scenario: scenario, Package: state.Package, Seed: state.Seed, StartedAt: state.StartedAt, EndedAt: state.EndedAt,
		Completed: state.StartedAt != nil && !state.Running && !state.Stopping && !state.Finalizing && state.Error == "", Crashed: false, StopReason: state.StopReason, Error: state.Error,
		SequenceDigest: sequenceDigest(steps),
		Metrics: Metrics{
			ObservedActivities: intPtr(state.ObservedActivities), StatesDiscovered: intPtr(len(graph.States)),
			EdgesDiscovered: intPtr(len(graph.Edges)), Steps: intPtr(len(steps)), Taps: intPtr(taps),
			Scrolls: intPtr(state.Scrolls), ScrollAttempts: intPtr(state.ScrollAttempts), ScrollProgress: intPtr(state.ScrollProgress), ScrollStalls: intPtr(state.ScrollStalls),
			Inputs: intPtr(state.Inputs), InputFields: intPtr(state.InputFields), Backtracks: intPtr(state.Backtracks), Recoveries: intPtr(state.Recoveries),
			SpecialActions: intPtr(state.SpecialActions),
			SafetyStops:    intPtr(safetyStops), DangerousActions: intPtr(dangerousActions),
		},
	}
	if state.ActivityTotal > 0 {
		report.Metrics.ActivityCovered = intPtr(state.ActivityCovered)
		report.Metrics.ActivityTotal = intPtr(state.ActivityTotal)
		report.Metrics.ActivityCoverage = state.ActivityCoverage
	}
	if state.StartedAt != nil && state.EndedAt != nil {
		duration := state.EndedAt.Sub(*state.StartedAt).Milliseconds()
		report.DurationMillis = &duration
	}
	if state.EvidenceIndexPath != "" {
		report.EvidenceIndex = state.EvidenceIndexPath
		report.Evidence = append(report.Evidence, state.EvidenceIndexPath)
	}
	report.Verdict = Judge(report.Completed, report.Error, report.Crashed, nil)
	return report
}

// FromRunner converts the persisted Monkey run manifest into the common
// evaluation model. It preserves only metrics that the Runner actually emits.
func FromRunner(scenario, engineVersion string, config runner.RunConfig, state runner.State) RunReport {
	steps := int(state.Events)
	report := RunReport{
		SchemaVersion: SchemaVersion, Engine: "nova", EngineVersion: engineVersion,
		Scenario: scenario, Package: config.Package, Seed: config.Seed,
		StartedAt: state.StartedAt, EndedAt: state.EndedAt,
		Completed: !state.Running && !state.Finalizing && state.StopReason == "completed" && state.ExitCode == 0 && state.Error == "" && state.CrashCount == 0 && state.ANRCount == 0 && state.NativeCrashCount == 0 && state.AbnormalExitCount == 0,
		Crashed:   state.ExitCode != 0 || state.Error != "" || state.CrashCount > 0 || state.ANRCount > 0 || state.NativeCrashCount > 0 || state.AbnormalExitCount > 0, StopReason: state.StopReason, Error: state.Error,
		Metrics: Metrics{
			StatesDiscovered: intPtr(state.Exploration.States), EdgesDiscovered: intPtr(state.Exploration.Edges),
			Steps: &steps, Taps: intPtr(state.Exploration.Taps), Scrolls: intPtr(state.Exploration.Scrolls),
			ScrollAttempts: intPtr(state.Exploration.ScrollAttempts), ScrollProgress: intPtr(state.Exploration.ScrollProgress), ScrollStalls: intPtr(state.Exploration.ScrollStalls),
			Inputs:     intPtr(state.Exploration.Inputs),
			Backtracks: intPtr(state.Exploration.Backtracks),
		},
		Evidence: append([]string{}, state.Screenshots...),
	}
	if state.ManifestPath != "" {
		report.Evidence = append(report.Evidence, state.ManifestPath)
	}
	if state.EvidenceIndexPath != "" {
		report.EvidenceIndex = state.EvidenceIndexPath
		report.Evidence = append(report.Evidence, state.EvidenceIndexPath)
	}
	if state.StartedAt != nil && state.EndedAt != nil {
		duration := state.EndedAt.Sub(*state.StartedAt).Milliseconds()
		report.DurationMillis = &duration
	}
	report.Verdict = Judge(report.Completed, report.Error, report.Crashed, nil)
	return report
}

type NexusOptions struct {
	Scenario       string
	Package        string
	EngineVersion  string
	Seed           int64
	DurationMillis int64
}

var (
	coveragePattern = regexp.MustCompile(`activity coverage exact=([0-9]+(?:\.[0-9]+)?)% covered=(\d+)/(\d+)`)
	statesPattern   = regexp.MustCompile(`scene states discovered=(\d+)`)
)

func ParseNexusLog(logText string, options NexusOptions) RunReport {
	report := RunReport{SchemaVersion: SchemaVersion, Engine: "nexus", EngineVersion: options.EngineVersion, Scenario: options.Scenario, Package: options.Package, Seed: options.Seed, Metrics: Metrics{}}
	if options.DurationMillis > 0 {
		report.DurationMillis = &options.DurationMillis
	}
	lines := strings.Split(strings.ReplaceAll(logText, "\r\n", "\n"), "\n")
	motion, scrolls := 0, 0
	for _, line := range lines {
		if strings.Contains(line, "random hit motion ") {
			motion++
		}
		if strings.Contains(line, "scroll activity=") {
			scrolls++
		}
		if match := coveragePattern.FindStringSubmatch(line); len(match) == 4 {
			coverage, _ := strconv.ParseFloat(match[1], 64)
			covered, _ := strconv.Atoi(match[2])
			total, _ := strconv.Atoi(match[3])
			report.Metrics.ActivityCoverage = &coverage
			report.Metrics.ActivityCovered = intPtr(covered)
			report.Metrics.ActivityTotal = intPtr(total)
		}
		if match := statesPattern.FindStringSubmatch(line); len(match) == 2 {
			states, _ := strconv.Atoi(match[1])
			report.Metrics.StatesDiscovered = intPtr(states)
		}
		if strings.Contains(line, "// Monkey finished") {
			report.Completed = true
			report.StopReason = "finished"
		}
		if strings.Contains(line, "FATAL EXCEPTION") || (options.Package != "" && strings.Contains(line, "ANR in "+options.Package)) {
			report.Crashed = true
			report.Evidence = append(report.Evidence, strings.TrimSpace(line))
		}
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "java.lang.SecurityException:") || strings.HasPrefix(trimmed, "Exception in thread ") {
			report.Crashed = true
			if report.Error == "" {
				report.Error = trimmed
			}
			report.Evidence = append(report.Evidence, trimmed)
		}
	}
	if report.Crashed && report.StopReason == "" {
		report.StopReason = "engine_crash"
	}
	report.Metrics.MotionEvents = intPtr(motion)
	report.Metrics.Scrolls = intPtr(scrolls)
	return report
}

func sequenceDigest(steps []exploration.Step) string {
	rows := make([]string, 0, len(steps))
	for _, step := range steps {
		rows = append(rows, strings.Join([]string{step.From, step.Action.ID, step.Action.Type, step.Destination}, "|"))
	}
	sum := sha256.Sum256([]byte(strings.Join(rows, "\n")))
	return hex.EncodeToString(sum[:])
}

func intPtr(value int) *int { return &value }

type MetricDelta struct {
	Name           string   `json:"name"`
	Baseline       *float64 `json:"baseline,omitempty"`
	Candidate      *float64 `json:"candidate,omitempty"`
	Delta          *float64 `json:"delta,omitempty"`
	Comparable     bool     `json:"comparable"`
	HigherIsBetter bool     `json:"higherIsBetter"`
}

type Comparison struct {
	SchemaVersion string        `json:"schemaVersion"`
	Verdict       string        `json:"verdict"`
	Reasons       []string      `json:"reasons"`
	Metrics       []MetricDelta `json:"metrics"`
}

func Compare(baseline, candidate RunReport, coverageTolerance float64) Comparison {
	result := Comparison{SchemaVersion: SchemaVersion, Verdict: "pass", Reasons: []string{}, Metrics: []MetricDelta{}}
	result.Metrics = append(result.Metrics,
		delta("activityCoveragePercent", floatPtr(baseline.Metrics.ActivityCoverage), floatPtr(candidate.Metrics.ActivityCoverage), true),
		delta("statesDiscovered", intFloat(baseline.Metrics.StatesDiscovered), intFloat(candidate.Metrics.StatesDiscovered), true),
		delta("scrolls", intFloat(baseline.Metrics.Scrolls), intFloat(candidate.Metrics.Scrolls), true),
		delta("scrollProgress", intFloat(baseline.Metrics.ScrollProgress), intFloat(candidate.Metrics.ScrollProgress), true),
		delta("inputs", intFloat(baseline.Metrics.Inputs), intFloat(candidate.Metrics.Inputs), true),
	)
	if baseline.Scenario != candidate.Scenario || baseline.Package != candidate.Package || (baseline.Seed != 0 && candidate.Seed != 0 && baseline.Seed != candidate.Seed) {
		result.Verdict = "fail"
		result.Reasons = append(result.Reasons, "reports do not describe the same scenario, package, and seed")
	}
	if baseline.Crashed || baseline.Error != "" {
		result.Verdict = "blocked"
		result.Reasons = append(result.Reasons, "baseline engine failed before a comparable behavior run completed")
		for index := range result.Metrics {
			result.Metrics[index].Comparable = false
			result.Metrics[index].Delta = nil
		}
	}
	if candidate.Crashed {
		result.Verdict = "fail"
		result.Reasons = append(result.Reasons, "candidate crashed")
	}
	if candidate.Metrics.DangerousActions != nil && *candidate.Metrics.DangerousActions > 0 {
		result.Verdict = "fail"
		result.Reasons = append(result.Reasons, "candidate executed dangerous actions")
	}
	if candidate.Error != "" && result.Verdict != "fail" {
		result.Verdict = "needs-review"
		result.Reasons = append(result.Reasons, "candidate reported an error")
	}
	if baseline.Metrics.ActivityCoverage != nil && candidate.Metrics.ActivityCoverage != nil && *candidate.Metrics.ActivityCoverage+coverageTolerance < *baseline.Metrics.ActivityCoverage && result.Verdict != "fail" {
		result.Verdict = "needs-review"
		result.Reasons = append(result.Reasons, fmt.Sprintf("activity coverage regressed by %.4f percentage points", *baseline.Metrics.ActivityCoverage-*candidate.Metrics.ActivityCoverage))
	}
	if !candidate.Completed && result.Verdict == "pass" {
		result.Verdict = "needs-review"
		result.Reasons = append(result.Reasons, "candidate run did not complete cleanly")
	}
	if len(result.Reasons) == 0 {
		result.Reasons = append(result.Reasons, "all available safety and regression gates passed")
	}
	return result
}

func delta(name string, baseline, candidate *float64, higherIsBetter bool) MetricDelta {
	value := MetricDelta{Name: name, Baseline: baseline, Candidate: candidate, HigherIsBetter: higherIsBetter, Comparable: baseline != nil && candidate != nil}
	if baseline != nil && candidate != nil {
		difference := *candidate - *baseline
		value.Delta = &difference
	}
	return value
}

func intFloat(value *int) *float64 {
	if value == nil {
		return nil
	}
	converted := float64(*value)
	return &converted
}

func floatPtr(value *float64) *float64 {
	if value == nil {
		return nil
	}
	converted := *value
	return &converted
}

type DeterminismResult struct {
	Deterministic bool     `json:"deterministic"`
	Digests       []string `json:"digests"`
	Reason        string   `json:"reason"`
}

func CheckDeterminism(reports []RunReport) DeterminismResult {
	digests := make([]string, 0, len(reports))
	identity := ""
	for _, report := range reports {
		currentIdentity := fmt.Sprintf("%s|%s|%s|%d", report.Engine, report.EngineVersion, report.Scenario, report.Seed)
		if identity == "" {
			identity = currentIdentity
		}
		if currentIdentity != identity {
			return DeterminismResult{Deterministic: false, Reason: "reports do not share engine, version, scenario, and seed"}
		}
		if report.SequenceDigest == "" {
			return DeterminismResult{Deterministic: false, Reason: "a report has no sequence digest"}
		}
		digests = append(digests, report.SequenceDigest)
	}
	if len(digests) < 2 {
		return DeterminismResult{Deterministic: false, Digests: digests, Reason: "at least two reports are required"}
	}
	sort.Strings(digests)
	for _, digest := range digests[1:] {
		if digest != digests[0] {
			return DeterminismResult{Deterministic: false, Digests: digests, Reason: "action sequence digests differ"}
		}
	}
	return DeterminismResult{Deterministic: true, Digests: digests, Reason: "action sequence digests match"}
}

func Marshal(value any) ([]byte, error) { return json.MarshalIndent(value, "", "  ") }
