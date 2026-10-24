package runner

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

func rotateLegacyLog(path string) error {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil || info.Size() < maxLegacyLogBytes {
		return err
	}
	backup := path + ".1"
	if err = os.Remove(backup); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(path, backup)
}

type eventRecord struct {
	State          string `json:"state"`
	RequestID      string `json:"requestId"`
	Package        string `json:"package"`
	Activity       string `json:"activity"`
	Events         int64  `json:"events"`
	Scene          string `json:"scene"`
	From           string `json:"from"`
	Action         string `json:"action"`
	To             string `json:"to"`
	Node           string `json:"node"`
	Outcome        string `json:"outcome"`
	SpecialKind    string `json:"specialKind"`
	RecoveryMillis int64  `json:"recoveryMillis"`
	Recovered      bool   `json:"recovered"`
	Reason         string `json:"reason"`
	Time           int64  `json:"time"`
}

type liveState struct {
	Events          int64
	LastEventAt     *time.Time
	CurrentActivity string
	CurrentScene    string
	LastAction      string
	ExternalPackage string
}

func parseEventLog(path string) (eventRecord, []string, ExplorationMetrics, liveState, error) {
	file, err := os.Open(path)
	if err != nil {
		return eventRecord{}, nil, ExplorationMetrics{}, liveState{}, err
	}
	defer func() { _ = file.Close() }()
	activities := map[string]bool{}
	scenes := map[string]bool{}
	edges := map[string]bool{}
	var summary eventRecord
	var metrics ExplorationMetrics
	var live liveState
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64<<10), 1<<20)
	for scanner.Scan() {
		var event eventRecord
		if json.Unmarshal(scanner.Bytes(), &event) != nil {
			continue
		}
		if event.Activity != "" {
			activities[event.Activity] = true
			live.CurrentActivity = event.Activity
			if slash := strings.IndexByte(event.Activity, '/'); slash > 0 {
				if strings.HasPrefix(event.Activity, event.Package+"/") {
					live.ExternalPackage = ""
				} else {
					live.ExternalPackage = event.Activity[:slash]
				}
			}
		}
		if event.Time > 0 {
			value := time.UnixMilli(event.Time).UTC()
			live.LastEventAt = &value
		}
		switch event.State {
		case "scene":
			if event.Scene != "" {
				scenes[event.Scene] = true
				live.CurrentScene = event.Scene
			}
		case "scene_transition":
			if event.From != "" && event.Action != "" && event.To != "" {
				edges[event.From+"|"+event.Action+"|"+event.To] = true
			}
		case "node_tap":
			metrics.Taps++
			live.Events++
			live.LastAction = "tap|" + event.Node
		case "node_long_click":
			metrics.LongClicks++
			live.Events++
			live.LastAction = "long|" + event.Node
		case "node_scroll":
			metrics.Scrolls++
			metrics.ScrollAttempts++
			live.Events++
			live.LastAction = "scroll|" + event.Node
		case "scroll_progress":
			metrics.ScrollProgress++
		case "scroll_stall":
			metrics.ScrollStalls++
		case "node_input":
			metrics.Inputs++
			live.Events++
			live.LastAction = "input|" + event.Node
		case "dfs_backtrack":
			metrics.Backtracks++
			live.Events++
			live.LastAction = "back"
		case "graph_path":
			metrics.KnownPathReplays++
		case "hierarchy_fallback":
			metrics.HierarchyFallback++
		case "fallback_action":
			metrics.FallbackActions++
			live.Events++
			live.LastAction = "fallback"
		case "coordinate_action":
			metrics.CoordinateActions++
			live.LastAction = "coordinate"
		case "coordinate_action_result":
			metrics.CoordinateResults++
			if event.Outcome == "target" {
				metrics.CoordinateTarget++
			} else if event.Outcome == "external" {
				metrics.CoordinateExternal++
			} else if event.Outcome == "special" {
				metrics.CoordinateSpecial++
			} else if event.Outcome == "unobserved" {
				metrics.CoordinateUnobserved++
			}
		case "coordinate_zone_blocked":
			metrics.CoordinateZoneBlocks++
		case "render_fallback_downgraded":
			metrics.RenderFallbackDowngrades++
		case "coordinate_fallback_suppressed":
			metrics.CoordinateFallbackSuppressed++
		case "generation_pingpong_blocked":
			metrics.GenerationPingPongBlocks++
		case "generation_pingpong_wait":
			metrics.GenerationPingPongWaits++
		case "exploration_cycle_refreshed":
			metrics.GenerationSoftRefreshes++
		case "render_context_quarantine":
			metrics.RenderContextQuarantines++
		case "relaunch_requested":
			metrics.RelaunchRequests++
		case "onboarding_exhaustion_guard":
			metrics.OnboardingExhaustions++
		case "coordinate_action_recovery":
			if event.Recovered {
				metrics.CoordinateRecoveries++
				metrics.CoordinateRecoveryMillis += event.RecoveryMillis
				if event.RecoveryMillis > metrics.CoordinateRecoveryMaxMillis {
					metrics.CoordinateRecoveryMaxMillis = event.RecoveryMillis
				}
			}
		case "cycle_detected":
			metrics.CycleDetections++
		case "edge_blocked":
			metrics.BlockedEdges++
		case "unstable_snapshot":
			metrics.UnstableSnapshots++
		case "completed", "stopped", "low_battery", "target_case_failed", "graph_exhausted", "hierarchy_fallback_exhausted", "onboarding_exhausted", "special_handling_failed", "special_unhandled", "external_package":
			summary = event
		}
	}
	metrics.States = len(scenes)
	metrics.Edges = len(edges)
	values := make([]string, 0, len(activities))
	for activity := range activities {
		values = append(values, activity)
	}
	sort.Strings(values)
	return summary, values, metrics, live, scanner.Err()
}

func writeExplorationSummary(path, packageName string, metrics ExplorationMetrics) error {
	value := struct {
		SchemaVersion string             `json:"schemaVersion"`
		Package       string             `json:"package"`
		Metrics       ExplorationMetrics `json:"metrics"`
	}{"xtest-monkey-graph/v1", packageName, metrics}
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(path, append(content, '\n'), 0o644)
}

func (m *Manager) captureScreenshot(directory, name string) (string, error) {
	if m.screenshot == nil {
		return "", errors.New("screenshot source unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), finalScreenshotTimeout)
	defer cancel()
	data, err := m.screenshot.Screenshot(ctx)
	if err != nil {
		return "", err
	}
	if len(data) < 8 || string(data[1:4]) != "PNG" {
		return "", errors.New("screenshot is not PNG")
	}
	path := filepath.Join(directory, name)
	return path, writeAtomic(path, data, 0o644)
}

func writeCoverage(path, packageName string, activities []string, activitySource interface {
	Activities(context.Context, string) ([]string, error)
}) error {
	targetActivities, externalActivities := make([]string, 0, len(activities)), make([]string, 0)
	for _, activity := range activities {
		if strings.HasPrefix(activity, packageName+"/") {
			targetActivities = append(targetActivities, activity)
		} else if activity != "" {
			externalActivities = append(externalActivities, activity)
		}
	}
	var declared []string
	var discoveryErr error
	if activitySource == nil {
		discoveryErr = errors.New("android PackageManager activity source unavailable")
	} else {
		discoveryContext, cancel := context.WithTimeout(context.Background(), coverageDiscoveryTimeout)
		declared, discoveryErr = activitySource.Activities(discoveryContext, packageName)
		cancel()
	}
	basis := "android-package-manager"
	if discoveryErr != nil || len(declared) == 0 {
		declared = parseDeclaredActivities(packageName, packageDump(packageName))
		declared = mergeKnownActivities(declared, targetActivities)
		basis = "package-resolver-plus-observed-lower-bound"
	}
	observedSet := make(map[string]struct{}, len(targetActivities))
	for _, activity := range targetActivities {
		observedSet[activity] = struct{}{}
	}
	covered, uncovered := make([]string, 0), make([]string, 0)
	for _, activity := range declared {
		if _, ok := observedSet[activity]; ok {
			covered = append(covered, activity)
		} else {
			uncovered = append(uncovered, activity)
		}
	}
	var percent *float64
	var coverageValue float64
	if len(declared) > 0 {
		coverageValue = float64(len(covered)) * 100 / float64(len(declared))
		percent = &coverageValue
	}
	unexpected := make([]string, 0)
	declaredSet := make(map[string]struct{}, len(declared))
	for _, activity := range declared {
		declaredSet[activity] = struct{}{}
	}
	for _, activity := range targetActivities {
		if _, ok := declaredSet[activity]; !ok {
			unexpected = append(unexpected, activity)
		}
	}
	value := struct {
		SchemaVersion      string   `json:"schemaVersion"`
		Package            string   `json:"package"`
		Observed           int      `json:"observed"`
		Activities         []string `json:"activities"`
		ExternalObserved   []string `json:"externalObserved,omitempty"`
		CoverageBasis      string   `json:"coverageBasis"`
		Declared           int      `json:"declared,omitempty"`
		Covered            []string `json:"covered,omitempty"`
		Uncovered          []string `json:"uncovered,omitempty"`
		UnexpectedObserved []string `json:"unexpectedObserved,omitempty"`
		CoveragePercent    *float64 `json:"coveragePercent,omitempty"`
	}{"xtest-monkey-coverage/v4", packageName, len(targetActivities), targetActivities, externalActivities, basis, len(declared), covered, uncovered, unexpected, percent}
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err = writeAtomic(path, append(content, '\n'), 0o644); err != nil {
		return err
	}
	textPath := filepath.Join(filepath.Dir(path), "activity_coverage.txt")
	lines := []string{fmt.Sprintf("package=%s", packageName), fmt.Sprintf("observed=%d", len(targetActivities))}
	lines = append(lines, "coverageBasis="+basis)
	if len(declared) > 0 {
		lines = append(lines, fmt.Sprintf("covered=%d/%d", len(covered), len(declared)), fmt.Sprintf("coverage=%.6f%%", coverageValue))
		for _, activity := range uncovered {
			lines = append(lines, "uncovered="+activity)
		}
	}
	for _, activity := range externalActivities {
		lines = append(lines, "externalObserved="+activity)
	}
	lines = append(lines, targetActivities...)
	return writeAtomic(textPath, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

func normalizeActivity(packageName, name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if strings.HasPrefix(name, ".") {
		return packageName + "/" + name
	}
	if !strings.Contains(name, ".") {
		name = packageName + "." + name
	}
	if strings.HasPrefix(name, packageName+".") {
		return packageName + "/." + strings.TrimPrefix(name, packageName+".")
	}
	return packageName + "/" + name
}

func mergeKnownActivities(resolved, observed []string) []string {
	seen := make(map[string]struct{}, len(resolved)+len(observed))
	for _, values := range [][]string{resolved, observed} {
		for _, activity := range values {
			if strings.TrimSpace(activity) != "" {
				seen[activity] = struct{}{}
			}
		}
	}
	result := make([]string, 0, len(seen))
	for activity := range seen {
		result = append(result, activity)
	}
	sort.Strings(result)
	return result
}

func packageDump(packageName string) string {
	ctx, cancel := context.WithTimeout(context.Background(), coverageFallbackTimeout)
	defer cancel()
	output, err := exec.CommandContext(ctx, "dumpsys", "package", packageName).Output()
	if err != nil {
		return ""
	}
	return string(output)
}

func parseDeclaredActivities(packageName, dump string) []string {
	if !packagePattern.MatchString(packageName) || dump == "" {
		return nil
	}
	pattern := regexp.MustCompile(regexp.QuoteMeta(packageName) + `/[A-Za-z0-9_.$]+`)
	seen := map[string]struct{}{}
	for _, activity := range pattern.FindAllString(dump, -1) {
		seen[activity] = struct{}{}
	}
	values := make([]string, 0, len(seen))
	for activity := range seen {
		values = append(values, activity)
	}
	sort.Strings(values)
	return values
}

func writeManifest(path string, config RunConfig, state State) error {
	if path == "" {
		return nil
	}
	hashes := map[string]string{}
	paths := append([]string{state.LogPath, state.CoveragePath, state.GraphPath, state.LogcatPath,
		state.CrashPath, state.ANRPath, state.NativeCrashPath, state.DiagnosticsPath,
		state.ExitInfoPath,
		filepath.Join(state.ArtifactDir, "activity_coverage.txt")}, state.Screenshots...)
	for _, artifactPath := range paths {
		if artifactPath == "" {
			continue
		}
		data, err := os.ReadFile(artifactPath)
		if err != nil {
			continue
		}
		sum := sha256.Sum256(data)
		hashes[filepath.Base(artifactPath)] = strings.ToUpper(hex.EncodeToString(sum[:]))
	}
	state.Identity.OwnerToken = ""
	value := struct {
		SchemaVersion string            `json:"schemaVersion"`
		Config        RunConfig         `json:"config"`
		State         State             `json:"state"`
		SHA256        map[string]string `json:"sha256,omitempty"`
	}{"xtest-monkey-run/v1", config, state, hashes}
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(path, append(content, '\n'), 0o644)
}

func writeAtomic(path string, content []byte, mode os.FileMode) error {
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, content, mode); err != nil {
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return nil
}
