package componenthealth

import "testing"

func TestRegistryReplacesStatusAndSnapshotsIndependently(t *testing.T) {
	registry := NewRegistry()
	registry.Set(New("companion", true, true, true, "1", ""))
	registry.Set(Degraded("companion", true, "1", "conflict"))
	snapshot := registry.Snapshot()
	if len(snapshot) != 1 || snapshot[0].State != "degraded" {
		t.Fatalf("snapshot=%+v", snapshot)
	}
	snapshot[0].State = "changed"
	stored, _ := registry.Get("companion")
	if stored.State != "degraded" {
		t.Fatalf("registry leaked snapshot mutation: %+v", stored)
	}
}
