package exploration

import "testing"

func TestCoverageSchedulerPrefersNavigationBeforeInput(t *testing.T) {
	node := &graphNode{analysis: Analysis{Actions: []Action{
		{ID: "input", Type: "input", Description: "search"},
		{ID: "tap", Type: "tap", Description: "albums"},
	}}, tried: map[string]bool{}}
	if actionWeightForNode(node, node.analysis.Actions[0], true) >= actionWeightForNode(node, node.analysis.Actions[1], true) {
		t.Fatal("input should have a lower first-pass probability than a navigation tap")
	}
}

func TestCoverageSchedulerDefersExternalRisk(t *testing.T) {
	node := &graphNode{analysis: Analysis{Actions: []Action{
		{ID: "cloud", Type: "tap", Description: "云同步"},
		{ID: "local", Type: "tap", Description: "本地相册"},
	}}, tried: map[string]bool{}}
	if actionWeightForNode(node, node.analysis.Actions[0], true) >= actionWeightForNode(node, node.analysis.Actions[1], true) {
		t.Fatal("external-risk action should have lower probability, not a hard exclusion")
	}
}

func TestCoverageSchedulerForcesPeriodicScroll(t *testing.T) {
	node := &graphNode{analysis: Analysis{Actions: []Action{
		{ID: "tap", Type: "tap"},
		{ID: "scroll", Type: "swipe"},
	}}, tried: map[string]bool{}, nonScrollSelections: maxNonScrollSelectionsBeforeScroll}
	action, ok := selectAction(node, 1, true, true)
	if !ok || action.ID != "scroll" {
		t.Fatalf("selected %#v, want periodic scroll", action)
	}
}

func TestCoverageSchedulerDoesNotHardCodeBusinessEntryOrder(t *testing.T) {
	node := &graphNode{analysis: Analysis{Actions: []Action{
		{ID: "points", Type: "tap", ResourceID: "com.example:id/enter_pointswall_ic"},
		{ID: "search", Type: "tap", ResourceID: "com.example:id/search"},
	}}, tried: map[string]bool{}}
	pointsWeight := actionWeightForNode(node, node.analysis.Actions[0], true)
	searchWeight := actionWeightForNode(node, node.analysis.Actions[1], true)
	if pointsWeight != searchWeight {
		t.Fatalf("business labels created a fixed priority: points=%v search=%v", pointsWeight, searchWeight)
	}
}
