package evaluation

import "testing"

func TestJudgeNeverPassesMissingEvidence(t *testing.T) {
	verdict := Judge(true, "", false, []Checkpoint{{ID: "home", Reached: true, Passed: true}})
	if verdict.Status != VerdictNotTested {
		t.Fatalf("verdict = %+v", verdict)
	}
}

func TestJudgeFailsObservedFailureAndPassesSupportedCheckpoint(t *testing.T) {
	failed := Judge(true, "", true, nil)
	if failed.Status != VerdictFailed {
		t.Fatalf("failed = %+v", failed)
	}
	passed := Judge(true, "", false, []Checkpoint{{ID: "home", Reached: true, Passed: true, EvidenceRefs: []string{"finish.png"}}})
	if passed.Status != VerdictPassed {
		t.Fatalf("passed = %+v", passed)
	}
}
