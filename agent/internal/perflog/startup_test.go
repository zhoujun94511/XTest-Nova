package perflog

import (
	"context"
	"strings"
	"testing"

	novasystem "github.com/zhoujun94511/xtest-nova/agent/internal/system"
)

type startupCommands struct{ calls []string }

func (s *startupCommands) Run(_ context.Context, name string, args ...string) (string, error) {
	s.calls = append(s.calls, name+" "+strings.Join(args, " "))
	if name == "cmd" {
		return "com.example.app/.MainActivity\n", nil
	}
	if name == "am" && len(args) > 0 && args[0] == "force-stop" {
		return "", nil
	}
	if name == "dumpsys" {
		return "layerName = SurfaceView[com.example.app/Main]#42\ntotalFrames = 1\n", nil
	}
	return "Status: ok\nLaunchState: COLD\nActivity: com.example.app/.MainActivity\nThisTime: 100\nTotalTime: 120\nWaitTime: 140\n", nil
}

type startupCollector struct{ *startupCommands }

func (startupCollector) Performance(context.Context, string) (novasystem.Performance, error) {
	return novasystem.Performance{}, nil
}

func TestMeasureStartupProducesPercentilesAndBaselineVerdict(t *testing.T) {
	commands := &startupCommands{}
	manager := New(t.TempDir(), startupCollector{commands})
	report, err := manager.MeasureStartup(context.Background(), StartupConfig{Package: "com.example.app", Mode: "cold", Runs: 5, BaselineP95Millis: 130, MaxRegressionPercent: 0})
	if err != nil {
		t.Fatal(err)
	}
	if report.Statistics.P95 != 120 || report.Verdict != "passed" || report.Path == "" || report.FirstContentFrameStatistics == nil || report.FirstContentFrameStatistics.Count != 5 {
		t.Fatalf("report = %+v", report)
	}
	if len(commands.calls) != 26 {
		t.Fatalf("calls = %v", commands.calls)
	}
}

func TestMeasureWarmStartupBackgroundsTaskAndRequiresEnoughBaselineSamples(t *testing.T) {
	commands := &startupCommands{}
	manager := New(t.TempDir(), startupCollector{commands})
	report, err := manager.MeasureStartup(context.Background(), StartupConfig{Package: "com.example.app", Mode: "warm", Runs: 2, BaselineP95Millis: 130})
	if err != nil {
		t.Fatal(err)
	}
	if report.Verdict != "not_tested" || !strings.Contains(report.Reason, "at least 5 samples") {
		t.Fatalf("report = %+v", report)
	}
	homeCalls := 0
	for _, call := range commands.calls {
		if call == "input keyevent KEYCODE_HOME" {
			homeCalls++
		}
	}
	if homeCalls != 2 {
		t.Fatalf("calls = %v", commands.calls)
	}
}

func TestParseStartupAllowsAndroid16MissingThisTime(t *testing.T) {
	sample, err := parseStartupOutput("Status: ok\nLaunchState: WARM\nActivity: com.example/.Main\nTotalTime: 20\nWaitTime: 21\n", 1, "warm")
	if err != nil || sample.ThisTime != nil {
		t.Fatalf("sample=%+v err=%v", sample, err)
	}
}

func TestParseWarmTaskResumeUsesWaitTimeWhenAndroidOmitsTotalTime(t *testing.T) {
	sample, err := parseStartupOutput("Status: ok\nLaunchState: UNKNOWN (0)\nActivity: com.example/.Main\nWaitTime: 25\n", 1, "warm")
	if err != nil || sample.TotalTime != 25 || sample.DurationSource != "wait_time" {
		t.Fatalf("sample=%+v err=%v", sample, err)
	}
	if _, err = parseStartupOutput("Status: ok\nActivity: com.example/.Main\nWaitTime: 25\n", 1, "cold"); err == nil {
		t.Fatal("cold startup accepted missing TotalTime")
	}
}
