package runner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseCoordinateActionEvidence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	content := "" +
		`{"state":"coordinate_action","activity":"com.example/.Game","time":100}` + "\n" +
		`{"state":"coordinate_action_result","activity":"com.android.vending/.Main","outcome":"external","specialKind":"","time":120}` + "\n" +
		`{"state":"coordinate_action_recovery","activity":"com.example/.Game","recovered":true,"recoveryMillis":190,"time":290}` + "\n" +
		`{"state":"coordinate_action","activity":"com.example/.Game","time":300}` + "\n" +
		`{"state":"coordinate_action_result","activity":"com.example/.Paywall","outcome":"special","specialKind":"paywall","time":320}` + "\n" +
		`{"state":"coordinate_action","activity":"com.example/.Game","time":330}` + "\n" +
		`{"state":"coordinate_action_result","activity":"com.example/.Game","outcome":"target","time":335}` + "\n" +
		`{"state":"coordinate_action","activity":"com.example/.Game","time":340}` + "\n" +
		`{"state":"coordinate_action_result","activity":"com.example/.Game","outcome":"unobserved","reason":"superseded_by_next_action","time":350}` + "\n" +
		`{"state":"coordinate_zone_blocked","activity":"com.android.vending/.Main","zone":"r2c1","time":360}` + "\n" +
		`{"state":"render_fallback_downgraded","activity":"com.android.vending/.Main","time":370}` + "\n" +
		`{"state":"coordinate_fallback_suppressed","activity":"com.example/.Ad","time":380}` + "\n" +
		`{"state":"generation_pingpong_blocked","activity":"com.example/.Game","time":390}` + "\n" +
		`{"state":"generation_pingpong_wait","activity":"com.example/.Game","time":400}` + "\n" +
		`{"state":"onboarding_exhaustion_guard","activity":"com.example/.Onboarding","reason":"safe_advance_actions_exhausted","time":410}` + "\n" +
		`{"state":"onboarding_exhausted","requestId":"onboarding-run","package":"com.example","events":4,"time":420}` + "\n" +
		`{"state":"coordinate_action_recovery","activity":"com.example/.Game","recovered":false,"recoveryMillis":500,"time":800}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	summary, _, metrics, live, err := parseEventLog(path)
	if err != nil {
		t.Fatal(err)
	}
	if metrics.CoordinateActions != 4 || metrics.CoordinateResults != 4 || metrics.CoordinateTarget != 1 || metrics.CoordinateExternal != 1 || metrics.CoordinateSpecial != 1 || metrics.CoordinateUnobserved != 1 {
		t.Fatalf("coordinate metrics = %+v", metrics)
	}
	if metrics.CoordinateZoneBlocks != 1 || metrics.RenderFallbackDowngrades != 1 || metrics.CoordinateFallbackSuppressed != 1 {
		t.Fatalf("coordinate feedback metrics = %+v", metrics)
	}
	if metrics.GenerationPingPongBlocks != 1 || metrics.GenerationPingPongWaits != 1 {
		t.Fatalf("generation ping-pong metrics = %+v", metrics)
	}
	if metrics.OnboardingExhaustions != 1 {
		t.Fatalf("onboarding exhaustion metrics = %+v", metrics)
	}
	if summary.State != "onboarding_exhausted" || summary.RequestID != "onboarding-run" {
		t.Fatalf("onboarding terminal summary = %+v", summary)
	}
	if metrics.CoordinateRecoveries != 1 || metrics.CoordinateRecoveryMillis != 190 || metrics.CoordinateRecoveryMaxMillis != 190 {
		t.Fatalf("coordinate recovery metrics = %+v", metrics)
	}
	if live.LastAction != "coordinate" {
		t.Fatalf("last action = %q", live.LastAction)
	}
}
