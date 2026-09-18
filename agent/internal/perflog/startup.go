package perflog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/zhoujun94511/xtest-nova/agent/internal/artifacts"
	"github.com/zhoujun94511/xtest-nova/agent/internal/evidence"
	"github.com/zhoujun94511/xtest-nova/agent/internal/execution"
	novasystem "github.com/zhoujun94511/xtest-nova/agent/internal/system"
)

type CommandExecutor interface {
	Run(context.Context, string, ...string) (string, error)
}

const minimumStartupBaselineSamples = 5

type StartupConfig struct {
	Package              string  `json:"package"`
	Mode                 string  `json:"mode"`
	Runs                 int     `json:"runs"`
	CooldownMillis       int     `json:"cooldownMillis,omitempty"`
	BaselineP95Millis    float64 `json:"baselineP95Millis,omitempty"`
	MaxRegressionPercent float64 `json:"maxRegressionPercent,omitempty"`
}

type StartupSample struct {
	Round                             int    `json:"round"`
	Status                            string `json:"status"`
	LaunchState                       string `json:"launchState,omitempty"`
	Activity                          string `json:"activity"`
	ThisTime                          *int64 `json:"thisTimeMillis,omitempty"`
	TotalTime                         int64  `json:"totalTimeMillis"`
	WaitTime                          int64  `json:"waitTimeMillis"`
	DurationSource                    string `json:"durationSource"`
	FirstContentFrameUpperBoundMillis *int64 `json:"firstContentFrameUpperBoundMillis,omitempty"`
	FirstContentFrameSource           string `json:"firstContentFrameSource,omitempty"`
	FirstContentFrameState            string `json:"firstContentFrameState"`
	FirstContentFrameError            string `json:"firstContentFrameError,omitempty"`
}

type StartupStatistics struct {
	Count   int     `json:"count"`
	Minimum float64 `json:"minimumMillis"`
	Maximum float64 `json:"maximumMillis"`
	Average float64 `json:"averageMillis"`
	P50     float64 `json:"p50Millis"`
	P90     float64 `json:"p90Millis"`
	P95     float64 `json:"p95Millis"`
}

type StartupReport struct {
	SchemaVersion               string             `json:"schemaVersion"`
	Identity                    execution.Identity `json:"identity"`
	Package                     string             `json:"package"`
	Mode                        string             `json:"mode"`
	Activity                    string             `json:"activity"`
	StartedAt                   time.Time          `json:"startedAt"`
	EndedAt                     time.Time          `json:"endedAt"`
	Samples                     []StartupSample    `json:"samples"`
	Statistics                  StartupStatistics  `json:"statistics"`
	FirstContentFrameStatistics *StartupStatistics `json:"firstContentFrameStatistics,omitempty"`
	BaselineP95Millis           float64            `json:"baselineP95Millis,omitempty"`
	MaxRegressionPercent        float64            `json:"maxRegressionPercent,omitempty"`
	Verdict                     string             `json:"verdict"`
	Reason                      string             `json:"reason"`
	Path                        string             `json:"path,omitempty"`
	EvidenceIndexPath           string             `json:"evidenceIndexPath,omitempty"`
}

var startupFieldPattern = regexp.MustCompile(`(?m)^\s*(Status|LaunchState|Activity|ThisTime|TotalTime|WaitTime):\s*(.*?)\s*$`)

func (m *Manager) MeasureStartup(ctx context.Context, config StartupConfig) (StartupReport, error) {
	m.operation.Lock()
	defer m.operation.Unlock()
	if m.commands == nil {
		return StartupReport{}, errors.New("startup-time command transport unavailable")
	}
	config.Package, config.Mode = strings.TrimSpace(config.Package), strings.ToLower(strings.TrimSpace(config.Mode))
	if !packagePattern.MatchString(config.Package) {
		return StartupReport{}, errors.New("invalid Android package name")
	}
	if config.Mode == "" {
		config.Mode = "cold"
	}
	if config.Mode != "cold" && config.Mode != "warm" {
		return StartupReport{}, errors.New("startup mode must be cold or warm")
	}
	if config.Runs == 0 {
		config.Runs = 5
	}
	if config.Runs < 1 || config.Runs > 20 || config.CooldownMillis < 0 || config.CooldownMillis > 30000 || config.BaselineP95Millis < 0 || config.MaxRegressionPercent < 0 || config.MaxRegressionPercent > 1000 {
		return StartupReport{}, errors.New("invalid startup measurement bounds")
	}
	started := time.Now().UTC()
	identity := execution.NewIdentity("startup", config.Package+":"+started.Format("20060102T150405.000000000Z"), uint64(started.UnixNano()))
	var err error
	if m.coordinator != nil {
		identity, err := m.coordinator.Acquire("startup", config.Package+":"+started.Format("20060102T150405.000000000Z"))
		if err != nil {
			return StartupReport{}, err
		}
		defer m.coordinator.Release(identity)
	}
	activity, err := m.resolveLauncherActivity(ctx, config.Package)
	if err != nil {
		return StartupReport{}, err
	}
	report := StartupReport{SchemaVersion: "xtest-nova-startup/v2", Identity: identity, Package: config.Package, Mode: config.Mode, Activity: activity, StartedAt: started, Samples: []StartupSample{}, BaselineP95Millis: config.BaselineP95Millis, MaxRegressionPercent: config.MaxRegressionPercent}
	if config.Mode == "warm" {
		if _, err = m.commands.Run(ctx, "am", "start", "-W", "-n", activity); err != nil {
			return report, fmt.Errorf("prepare warm startup: %w", err)
		}
	}
	for round := 1; round <= config.Runs; round++ {
		if config.Mode == "cold" {
			if _, err = m.commands.Run(ctx, "am", "force-stop", config.Package); err != nil {
				return report, fmt.Errorf("force-stop before round %d: %w", round, err)
			}
		} else {
			// Move the pre-warmed task to the background before measuring the
			// activity resume. Re-launching an already,foreground activity can
			// report a misleading zero-time no-op on Android.
			if _, err = m.commands.Run(ctx, "input", "keyevent", "KEYCODE_HOME"); err != nil {
				return report, fmt.Errorf("background warm task before round %d: %w", round, err)
			}
		}
		framePrepared, framePrepareErr := m.prepareFirstContentFrame(ctx)
		launchStarted := time.Now()
		output, runErr := m.commands.Run(ctx, "am", "start", "-W", "-n", activity)
		if runErr != nil {
			return report, fmt.Errorf("measure startup round %d: %w", round, runErr)
		}
		sample, parseErr := parseStartupOutput(output, round, config.Mode)
		if parseErr != nil {
			return report, fmt.Errorf("parse startup round %d: %w", round, parseErr)
		}
		if !framePrepared {
			sample.FirstContentFrameState = "unsupported"
			sample.FirstContentFrameError = framePrepareErr.Error()
		} else if upperBound, observeErr := m.observeFirstContentFrame(ctx, config.Package, launchStarted); observeErr != nil {
			sample.FirstContentFrameState = "failed"
			sample.FirstContentFrameError = observeErr.Error()
		} else {
			sample.FirstContentFrameState = "measured"
			sample.FirstContentFrameSource = "surfaceflinger-timestats-first-observed-upper-bound"
			sample.FirstContentFrameUpperBoundMillis = &upperBound
		}
		report.Samples = append(report.Samples, sample)
		if round < config.Runs && config.CooldownMillis > 0 {
			timer := time.NewTimer(time.Duration(config.CooldownMillis) * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return report, ctx.Err()
			case <-timer.C:
			}
		}
	}
	values := make([]float64, 0, len(report.Samples))
	for _, sample := range report.Samples {
		values = append(values, float64(sample.TotalTime))
	}
	report.Statistics = startupStatistics(values)
	contentFrameValues := make([]float64, 0, len(report.Samples))
	for _, sample := range report.Samples {
		if sample.FirstContentFrameUpperBoundMillis != nil {
			contentFrameValues = append(contentFrameValues, float64(*sample.FirstContentFrameUpperBoundMillis))
		}
	}
	if len(contentFrameValues) > 0 {
		statistics := startupStatistics(contentFrameValues)
		report.FirstContentFrameStatistics = &statistics
	}
	report.EndedAt = time.Now().UTC()
	if config.BaselineP95Millis <= 0 {
		report.Verdict, report.Reason = "not_tested", "measurement completed without a baseline"
	} else if len(report.Samples) < minimumStartupBaselineSamples {
		report.Verdict, report.Reason = "not_tested", fmt.Sprintf("at least %d samples are required for baseline judgment", minimumStartupBaselineSamples)
	} else {
		limit := config.BaselineP95Millis * (1 + config.MaxRegressionPercent/100)
		if report.Statistics.P95 > limit {
			report.Verdict, report.Reason = "failed", fmt.Sprintf("P95 %.2f ms exceeds baseline limit %.2f ms", report.Statistics.P95, limit)
		} else {
			report.Verdict, report.Reason = "passed", "startup P95 is within the configured baseline limit"
		}
	}
	transaction, dirErr := artifacts.NewDeviceSessionTransaction(m.root, config.Package, "Startup")
	if dirErr != nil {
		return report, dirErr
	}
	defer func() { _ = transaction.Abort() }()
	report.Path = filepath.ToSlash(filepath.Join(transaction.Final, "startup.json"))
	report.EvidenceIndexPath = filepath.ToSlash(filepath.Join(transaction.Final, "evidence.json"))
	content, marshalErr := json.MarshalIndent(report, "", "  ")
	if marshalErr != nil {
		return report, marshalErr
	}
	stagingPath := filepath.Join(transaction.Staging, "startup.json")
	if writeErr := artifacts.WriteFileAtomic(stagingPath, append(content, '\n'), 0o644); writeErr != nil {
		return report, writeErr
	}
	index := evidence.BuildForPublication(transaction.Staging, transaction.Final, config.Package, "", identity, []string{stagingPath})
	if indexErr := evidence.Write(filepath.Join(transaction.Staging, "evidence.json"), index); indexErr != nil {
		return report, indexErr
	}
	if commitErr := transaction.Commit(); commitErr != nil {
		return report, commitErr
	}
	if pruneErr := artifacts.PruneSessions(filepath.Dir(transaction.Final), artifacts.MaxSessionsPerKind, transaction.Final); pruneErr != nil {
		return report, pruneErr
	}
	return report, nil
}

func (m *Manager) prepareSessionSample(ctx context.Context, collector Collector, packageName string) (novasystem.Performance, error) {
	if m.commands != nil {
		if err := m.launchLauncher(ctx, packageName); err != nil {
			return novasystem.Performance{}, err
		}
	}
	deadline := time.Now().Add(12 * time.Second)
	var last error
	for {
		sample, err := collector.Performance(ctx, packageName)
		if err == nil {
			return sample, nil
		}
		last = err
		if ctx.Err() != nil {
			return novasystem.Performance{}, ctx.Err()
		}
		if m.commands == nil || !isMissingProcess(err) || time.Now().After(deadline) {
			return novasystem.Performance{}, last
		}
		timer := time.NewTimer(250 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return novasystem.Performance{}, ctx.Err()
		case <-timer.C:
		}
	}
}

func (m *Manager) resolveLauncherActivity(ctx context.Context, packageName string) (string, error) {
	if m.commands == nil {
		return "", errors.New("app launch command transport unavailable")
	}
	resolved, err := m.commands.Run(ctx, "cmd", "package", "resolve-activity", "--brief", packageName)
	if err != nil {
		return "", fmt.Errorf("resolve launcher activity: %w", err)
	}
	activity := resolvedActivity(resolved)
	if activity == "" {
		return "", errors.New("launcher activity unavailable")
	}
	return activity, nil
}

func (m *Manager) launchLauncher(ctx context.Context, packageName string) error {
	activity, err := m.resolveLauncherActivity(ctx, packageName)
	if err != nil {
		return err
	}
	if _, err = m.commands.Run(ctx, "am", "start", "-W", "-n", activity); err != nil {
		return fmt.Errorf("bring target to foreground: %w", err)
	}
	return nil
}

func isMissingProcess(err error) bool {
	return err != nil && strings.Contains(err.Error(), "package not found")
}

func (m *Manager) prepareFirstContentFrame(ctx context.Context) (bool, error) {
	if _, err := m.commands.Run(ctx, "dumpsys", "SurfaceFlinger", "--timestats", "-clear"); err != nil {
		return false, err
	}
	if _, err := m.commands.Run(ctx, "dumpsys", "SurfaceFlinger", "--timestats", "-enable"); err != nil {
		return false, err
	}
	return true, nil
}

func (m *Manager) observeFirstContentFrame(ctx context.Context, packageName string, launchStarted time.Time) (int64, error) {
	deadline := time.Now().Add(5 * time.Second)
	for {
		output, err := m.commands.Run(ctx, "dumpsys", "SurfaceFlinger", "--timestats", "-dump")
		if err == nil {
			if frames, parseErr := novasystem.ParseSurfaceFrames(output, packageName); parseErr == nil && frames > 0 {
				millis := time.Since(launchStarted).Milliseconds()
				if millis < 1 {
					millis = 1
				}
				return millis, nil
			}
		}
		if time.Now().After(deadline) {
			return 0, errors.New("target SurfaceFlinger layer did not present a frame within 5 seconds after activity start")
		}
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return 0, ctx.Err()
		case <-timer.C:
		}
	}
}

func resolvedActivity(output string) string {
	lines := strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		value := strings.TrimSpace(lines[i])
		if strings.Contains(value, "/") && !strings.ContainsAny(value, " \t") {
			return value
		}
	}
	return ""
}

func parseStartupOutput(output string, round int, mode string) (StartupSample, error) {
	fields := map[string]string{}
	for _, match := range startupFieldPattern.FindAllStringSubmatch(strings.ReplaceAll(output, "\r\n", "\n"), -1) {
		fields[match[1]] = strings.TrimSpace(match[2])
	}
	if !strings.EqualFold(fields["Status"], "ok") || fields["Activity"] == "" || fields["WaitTime"] == "" {
		return StartupSample{}, errors.New("incomplete or unsuccessful am start -W output")
	}
	wait, err := strconv.ParseInt(fields["WaitTime"], 10, 64)
	if err != nil || wait < 0 {
		return StartupSample{}, errors.New("invalid WaitTime")
	}
	total, source := wait, "wait_time"
	if fields["TotalTime"] != "" {
		total, err = strconv.ParseInt(fields["TotalTime"], 10, 64)
		if err != nil || total < 0 || wait < total {
			return StartupSample{}, errors.New("invalid TotalTime")
		}
		source = "total_time"
	} else if mode != "warm" {
		return StartupSample{}, errors.New("cold startup output is missing TotalTime")
	}
	value := StartupSample{Round: round, Status: fields["Status"], LaunchState: fields["LaunchState"], Activity: fields["Activity"], TotalTime: total, WaitTime: wait, DurationSource: source}
	if raw := fields["ThisTime"]; raw != "" {
		parsed, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr != nil || parsed < 0 || parsed > total {
			return StartupSample{}, errors.New("invalid ThisTime")
		}
		value.ThisTime = &parsed
	}
	return value, nil
}

func startupStatistics(values []float64) StartupStatistics {
	if len(values) == 0 {
		return StartupStatistics{}
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	sum := 0.0
	for _, value := range sorted {
		sum += value
	}
	return StartupStatistics{Count: len(sorted), Minimum: sorted[0], Maximum: sorted[len(sorted)-1], Average: sum / float64(len(sorted)), P50: percentile(sorted, .50), P90: percentile(sorted, .90), P95: percentile(sorted, .95)}
}
