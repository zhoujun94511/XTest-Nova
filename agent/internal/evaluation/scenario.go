package evaluation

import (
	"errors"
	"strings"

	"github.com/zhoujun94511/xtest-nova/agent/internal/exploration"
)

type Scenario struct {
	SchemaVersion            string             `json:"schemaVersion"`
	Name                     string             `json:"name"`
	Description              string             `json:"description,omitempty"`
	CoverageTolerancePercent float64            `json:"coverageTolerancePercent,omitempty"`
	WallClockSeconds         int                `json:"wallClockSeconds,omitempty"`
	Evidence                 EvidencePlan       `json:"evidence,omitempty"`
	SafetyAudit              SafetyAuditPlan    `json:"safetyAudit,omitempty"`
	Config                   exploration.Config `json:"config"`
}

type EvidencePlan struct {
	Screenshots          []string `json:"screenshots,omitempty"`
	TrackExternalEscapes bool     `json:"trackExternalEscapes,omitempty"`
	TrackPerformance     bool     `json:"trackPerformance,omitempty"`
}

type SafetyAuditPlan struct {
	RequirePaymentProtection bool `json:"requirePaymentProtection,omitempty"`
	MaximumDangerousActions  int  `json:"maximumDangerousActions"`
}

func (s Scenario) Validate() error {
	if s.SchemaVersion != SchemaVersion {
		return errors.New("unsupported scenario schemaVersion")
	}
	if strings.TrimSpace(s.Name) == "" {
		return errors.New("scenario name is required")
	}
	if s.CoverageTolerancePercent < 0 || s.CoverageTolerancePercent > 100 {
		return errors.New("coverageTolerancePercent must be between 0 and 100")
	}
	if s.WallClockSeconds < 0 || s.WallClockSeconds > 86400 {
		return errors.New("wallClockSeconds must be between 0 and 86400")
	}
	if len(s.Evidence.Screenshots) > 10 {
		return errors.New("evidence screenshots are limited to 10 checkpoints")
	}
	for _, checkpoint := range s.Evidence.Screenshots {
		if checkpoint != "before" && checkpoint != "middle" && checkpoint != "after" {
			return errors.New("evidence screenshots must use before, middle, or after checkpoints")
		}
	}
	if s.SafetyAudit.MaximumDangerousActions < 0 {
		return errors.New("maximumDangerousActions must not be negative")
	}
	if !s.Config.Execute {
		return errors.New("scenario config must explicitly set execute=true")
	}
	return s.Config.Validate()
}
