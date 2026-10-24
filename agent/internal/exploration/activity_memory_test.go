package exploration

import (
	"os"
	"path/filepath"
	"testing"
)

func TestActivityMemoryRoundTripIsolatesVersion(t *testing.T) {
	root := t.TempDir()
	first := loadActivityMemory(root, "com.example.app", 7)
	first.note("tap|com.example:id/open|android.widget.TextView|album", "com.example.app/.Detail")
	if err := first.save(); err != nil {
		t.Fatal(err)
	}
	loaded := loadActivityMemory(root, "com.example.app", 7)
	known, unseen := loaded.unseenFraction("tap|com.example:id/open|android.widget.TextView|album", map[string]struct{}{})
	if !known || unseen != 1 {
		t.Fatalf("known=%v unseen=%v", known, unseen)
	}
	other := loadActivityMemory(root, "com.example.app", 8)
	if known, _ := other.unseenFraction("tap|com.example:id/open|android.widget.TextView|album", nil); known {
		t.Fatal("another version reused the saved counts")
	}
	if _, err := os.Stat(filepath.Join(root, "com.example.app", "ActivityMemory", "7.json")); err != nil {
		t.Fatal(err)
	}
}

func TestActivityMemoryKeepsCorruptFile(t *testing.T) {
	root := t.TempDir()
	path := activityMemoryPath(root, "com.example.app", 3)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	memory := loadActivityMemory(root, "com.example.app", 3)
	memory.note("tap|id|class|label", "com.example.app/.Next")
	if err := memory.save(); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "{" {
		t.Fatalf("corrupt memory was replaced: %s", content)
	}
}

func TestRememberedActionsPreferUnseenDestinations(t *testing.T) {
	memory := &activityMemory{actions: map[string]map[string]int{
		"tap|com.example:id/seen|android.widget.TextView|seen": {"com.example.app/.Seen": 4},
		"tap|com.example:id/next|android.widget.TextView|next": {"com.example.app/.Next": 2},
	}}
	node := &graphNode{analysis: Analysis{Actions: []Action{
		{ID: "seen", Type: "tap", ResourceID: "com.example:id/seen", Class: "android.widget.TextView", Text: "seen"},
		{ID: "next", Type: "tap", ResourceID: "com.example:id/next", Class: "android.widget.TextView", Text: "next"},
		{ID: "fresh", Type: "tap", ResourceID: "com.example:id/fresh", Class: "android.widget.TextView", Text: "fresh"},
	}}, tried: map[string]bool{}, actionAttempts: map[string]int{}}
	visited := map[string]struct{}{"com.example.app/.Seen": {}}
	seen := actionWeight(node, node.analysis.Actions[0], false, memory, visited)
	next := actionWeight(node, node.analysis.Actions[1], false, memory, visited)
	fresh := actionWeight(node, node.analysis.Actions[2], false, memory, visited)
	if !(fresh > next && next > seen) {
		t.Fatalf("weights fresh=%v next=%v seen=%v", fresh, next, seen)
	}
	if actionWeight(node, node.analysis.Actions[0], false, nil, nil) == 0 {
		t.Fatal("missing memory removed the action")
	}
}

func TestRememberedBiasStaysBehindExternalRisk(t *testing.T) {
	memory := &activityMemory{actions: map[string]map[string]int{
		"tap|com.example:id/seen|android.widget.TextView|seen": {"com.example.app/.Seen": 1},
	}}
	node := &graphNode{analysis: Analysis{Actions: []Action{
		{ID: "seen", Type: "tap", ResourceID: "com.example:id/seen", Class: "android.widget.TextView", Text: "seen"},
		{ID: "login", Type: "tap", ResourceID: "com.example:id/login", Class: "android.widget.TextView", Text: "login"},
	}}, tried: map[string]bool{}, actionAttempts: map[string]int{}}
	visited := map[string]struct{}{"com.example.app/.Seen": {}}
	seen := actionWeight(node, node.analysis.Actions[0], false, memory, visited)
	login := actionWeight(node, node.analysis.Actions[1], false, memory, visited)
	if !(seen > login && login > 0) {
		t.Fatalf("seen=%v login=%v", seen, login)
	}
}
