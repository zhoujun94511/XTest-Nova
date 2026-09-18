package evaluation

import (
	"testing"

	"github.com/zhoujun94511/xtest-nova/agent/internal/exploration"
)

func TestScenarioRequiresExplicitExecution(t *testing.T) {
	scenario := Scenario{SchemaVersion: SchemaVersion, Name: "safe", Config: exploration.Config{Package: "com.example.app"}}
	if err := scenario.Validate(); err == nil {
		t.Fatal("scenario accepted without execute=true")
	}
	scenario.Config.Execute = true
	if err := scenario.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestGameScenarioAcceptsWallClockEvidenceAndPaymentAudit(t *testing.T) {
	scenario := Scenario{
		SchemaVersion: SchemaVersion, Name: "game", WallClockSeconds: 1200,
		Evidence:    EvidencePlan{Screenshots: []string{"before", "middle", "after"}, TrackExternalEscapes: true, TrackPerformance: true},
		SafetyAudit: SafetyAuditPlan{RequirePaymentProtection: true, MaximumDangerousActions: 0},
		Config:      exploration.Config{Package: "com.example.game", Execute: true},
	}
	if err := scenario.Validate(); err != nil {
		t.Fatal(err)
	}
	scenario.Evidence.Screenshots = []string{"eventually"}
	if err := scenario.Validate(); err == nil {
		t.Fatal("scenario accepted an unknown screenshot checkpoint")
	}
}
