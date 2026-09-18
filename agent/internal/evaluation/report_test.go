package evaluation

import (
	"reflect"
	"testing"
	"time"

	"github.com/zhoujun94511/xtest-nova/agent/internal/exploration"
)

func TestParseNexusLogKeepsCoverageAndDiscoverySemanticsSeparate(t *testing.T) {
	report := ParseNexusLog("random hit motion 1\nscroll activity=A\nactivity coverage exact=29.411764% covered=5/17\nscene states discovered=59 (discovery count, not a denominator-based percentage)\n// Monkey finished", NexusOptions{Scenario: "historical", Package: "com.example"})
	if !report.Completed || report.Metrics.ActivityCovered == nil || *report.Metrics.ActivityCovered != 5 || report.Metrics.ActivityTotal == nil || *report.Metrics.ActivityTotal != 17 || report.Metrics.StatesDiscovered == nil || *report.Metrics.StatesDiscovered != 59 {
		t.Fatalf("report = %#v", report)
	}
}

func TestParseNexusLogDetectsStartupSecurityException(t *testing.T) {
	report := ParseNexusLog("java.lang.SecurityException: calling package mismatch\n\tat android.os.Parcel.readException", NexusOptions{Package: "com.example"})
	if !report.Crashed || report.Completed || report.StopReason != "engine_crash" || report.Error == "" {
		t.Fatalf("report=%#v", report)
	}
}

func TestNormalizeForGoldenExcludesRuntimeOnlyFields(t *testing.T) {
	firstTime := time.Unix(1, 0).UTC()
	secondTime := time.Unix(2, 0).UTC()
	first := RunReport{SchemaVersion: SchemaVersion, Engine: "nova", Scenario: "same", Package: "com.example", Seed: 7, StartedAt: &firstTime, Evidence: []string{"C:/temp/a"}, Completed: true}
	second := first
	second.StartedAt = &secondTime
	second.Evidence = []string{"D:/other/b"}
	if !reflect.DeepEqual(NormalizeForGolden(first), NormalizeForGolden(second)) {
		t.Fatal("runtime timestamps or evidence paths changed the golden report")
	}
}

func TestCompareRejectsMismatchedRunIdentity(t *testing.T) {
	result := Compare(
		RunReport{Scenario: "one", Package: "com.example", Seed: 1},
		RunReport{Scenario: "two", Package: "com.example", Seed: 1, Completed: true}, 0,
	)
	if result.Verdict != "fail" {
		t.Fatalf("comparison = %#v", result)
	}
}

func TestCompareMarksCoverageRegressionForReview(t *testing.T) {
	baseCoverage, candidateCoverage := 30.0, 25.0
	result := Compare(RunReport{Metrics: Metrics{ActivityCoverage: &baseCoverage}}, RunReport{Completed: true, Metrics: Metrics{ActivityCoverage: &candidateCoverage}}, 1)
	if result.Verdict != "needs-review" {
		t.Fatalf("comparison = %#v", result)
	}
}

func TestCompareBlocksWhenBaselineEngineCrashes(t *testing.T) {
	result := Compare(
		RunReport{Scenario: "same", Package: "com.example", Seed: 7, Crashed: true, Error: "startup failed"},
		RunReport{Scenario: "same", Package: "com.example", Seed: 7, Completed: true}, 0,
	)
	if result.Verdict != "blocked" {
		t.Fatalf("comparison = %#v", result)
	}
	for _, metric := range result.Metrics {
		if metric.Comparable || metric.Delta != nil {
			t.Fatalf("failed baseline metric must not be comparable: %#v", metric)
		}
	}
}

func TestNovaSequenceDigestIgnoresTimestamps(t *testing.T) {
	steps := []exploration.Step{{From: "a", Destination: "b", Action: exploration.Action{ID: "tap-one", Type: "tap"}}}
	first := FromNova("stable", "1", exploration.State{}, exploration.Graph{}, steps)
	second := FromNova("stable", "1", exploration.State{}, exploration.Graph{}, steps)
	result := CheckDeterminism([]RunReport{first, second})
	if !result.Deterministic {
		t.Fatalf("determinism = %#v", result)
	}
}
