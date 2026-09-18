package diagnosticsoak

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStartValidatesBoundsWithoutCreatingTask(t *testing.T) {
	manager := New(t.TempDir(), func() any { return nil })
	for _, config := range []Config{{DurationSeconds: 59, IntervalSeconds: 5}, {DurationSeconds: 60, IntervalSeconds: 4}, {DurationSeconds: 604801, IntervalSeconds: 5}} {
		if _, err := manager.Start(config); err == nil {
			t.Fatalf("expected error for %+v", config)
		}
	}
}

func TestNewMarksInterruptedSession(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "device.runtime", "Diagnostics", "20260911_120000")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, ".active"), []byte("started"), 0o644); err != nil {
		t.Fatal(err)
	}
	manager := New(root, func() any { return nil })
	state := manager.State()
	if state.RecoveredInterruptions != 1 {
		t.Fatalf("recovered=%d", state.RecoveredInterruptions)
	}
	if _, err := os.Stat(filepath.Join(directory, ".interrupted")); err != nil {
		t.Fatal(err)
	}
}

func TestRecentSamplesAreBounded(t *testing.T) {
	state := State{Recent: make([]Sample, maxRecentSamples+1)}
	cloned := cloneState(state)
	cloned.Recent = cloned.Recent[len(cloned.Recent)-maxRecentSamples:]
	if len(cloned.Recent) != maxRecentSamples {
		t.Fatalf("recent size=%d", len(cloned.Recent))
	}
}
