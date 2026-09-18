package evaluation

import "strings"

type VerdictStatus string

const (
	VerdictPassed    VerdictStatus = "passed"
	VerdictFailed    VerdictStatus = "failed"
	VerdictNotTested VerdictStatus = "not_tested"
)

type Checkpoint struct {
	ID           string   `json:"id"`
	Reached      bool     `json:"reached"`
	Passed       bool     `json:"passed"`
	EvidenceRefs []string `json:"evidenceRefs,omitempty"`
	Error        string   `json:"error,omitempty"`
}

type Verdict struct {
	Status      VerdictStatus `json:"status"`
	Reason      string        `json:"reason"`
	Checkpoints []Checkpoint  `json:"checkpoints,omitempty"`
}

// Judge is intentionally deterministic and has no access to a device or an
// Agent decision. A missing checkpoint or missing evidence can never pass.
func Judge(completed bool, executionError string, crashed bool, checkpoints []Checkpoint) Verdict {
	if crashed {
		return Verdict{Status: VerdictFailed, Reason: "target application or execution engine crashed", Checkpoints: checkpoints}
	}
	if strings.TrimSpace(executionError) != "" {
		return Verdict{Status: VerdictNotTested, Reason: "execution did not produce a reliable test result", Checkpoints: checkpoints}
	}
	if !completed {
		return Verdict{Status: VerdictNotTested, Reason: "execution did not complete", Checkpoints: checkpoints}
	}
	if len(checkpoints) == 0 {
		return Verdict{Status: VerdictNotTested, Reason: "no acceptance checkpoints were defined"}
	}
	for _, checkpoint := range checkpoints {
		if !checkpoint.Reached {
			return Verdict{Status: VerdictNotTested, Reason: "required checkpoint was not reached: " + checkpoint.ID, Checkpoints: checkpoints}
		}
		if len(checkpoint.EvidenceRefs) == 0 {
			return Verdict{Status: VerdictNotTested, Reason: "required checkpoint has no evidence: " + checkpoint.ID, Checkpoints: checkpoints}
		}
		if !checkpoint.Passed {
			return Verdict{Status: VerdictFailed, Reason: "required checkpoint failed: " + checkpoint.ID, Checkpoints: checkpoints}
		}
	}
	return Verdict{Status: VerdictPassed, Reason: "all required checkpoints passed with evidence", Checkpoints: checkpoints}
}
