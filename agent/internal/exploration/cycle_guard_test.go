package exploration

import "testing"

func TestCycleGuardDetectsRepeatedPeriodsAcrossPhysicalStateChurn(t *testing.T) {
	guard := &cycleGuard{}
	period := 0
	for iteration := 0; iteration < 3; iteration++ {
		period = guard.observe("semantic-a", "to-b", "semantic-b", "semantic-a")
		period = guard.observe("semantic-b", "to-a", "semantic-a", "semantic-b")
	}
	if period != 2 {
		t.Fatalf("period = %d, want 2", period)
	}
}

func TestCycleGuardDetectsLongerCycleAndIgnoresIncompleteRepeat(t *testing.T) {
	guard := &cycleGuard{}
	sequence := []transitionSample{
		{from: "a", to: "b"}, {from: "b", to: "c"}, {from: "c", to: "a"},
		{from: "a", to: "b"}, {from: "b", to: "c"}, {from: "c", to: "a"},
	}
	for _, sample := range sequence {
		if period := guard.observe(sample.from, sample.action, sample.to, sample.from); period != 0 {
			t.Fatalf("detected cycle before third repetition: %d", period)
		}
	}
	for _, sample := range sequence[:3] {
		period := guard.observe(sample.from, sample.action, sample.to, sample.from)
		if sample.to == "a" && period != 3 {
			t.Fatalf("period = %d, want 3", period)
		}
	}
}

func TestCycleGuardDetectsRepeatedTransitionAcrossInterleavedDetours(t *testing.T) {
	guard := &cycleGuard{}
	for iteration := 0; iteration < repeatedTransitionLimit; iteration++ {
		period := guard.observe("a", "to-b", "b", "a")
		if iteration < repeatedTransitionLimit-1 && period != 0 {
			t.Fatalf("detected repeated transition too early at %d", iteration)
		}
		if iteration == repeatedTransitionLimit-1 && period != cycleMaxPeriod+1 {
			t.Fatalf("detection = %d, want non-consecutive sentinel", period)
		}
		if iteration < repeatedTransitionLimit-1 {
			guard.observe("detour-"+string(rune('a'+iteration)), "next", "other-"+string(rune('a'+iteration)), "detour")
		}
	}
}

func TestSemanticStateIgnoresCoordinatesAndDynamicDigits(t *testing.T) {
	first := Analysis{Actions: []Action{{Type: "tap", ResourceID: "id/tab", Class: "Button", Text: "Item 12", X: 10, Y: 20}}}
	second := Analysis{Actions: []Action{{Type: "tap", ResourceID: "id/tab", Class: "Button", Text: "Item 99", X: 500, Y: 800}}}
	if semanticStateKey(first, "pkg/.Main") != semanticStateKey(second, "pkg/.Main") {
		t.Fatal("semantic state changed with coordinates or digits")
	}
}
