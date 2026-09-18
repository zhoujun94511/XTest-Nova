package artifactcatalog

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListReturnsOnlyValidatedSessions(t *testing.T) {
	root := t.TempDir()
	valid := filepath.Join(root, "com.example.app", "Monkey", "20260911_120000")
	if err := os.MkdirAll(valid, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(valid, "run.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "bad", "Monkey", "not-a-session"), 0o755); err != nil {
		t.Fatal(err)
	}
	sessions, err := New(root).List("", "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || len(sessions[0].Files) != 1 || sessions[0].Files[0].Name != "run.json" {
		t.Fatalf("unexpected catalog: %+v", sessions)
	}
}

func TestListRejectsTraversalFilters(t *testing.T) {
	catalog := New(t.TempDir())
	if _, err := catalog.List("../tmp", "", 20); err == nil {
		t.Fatal("expected package validation error")
	}
	if _, err := catalog.List("", "../Monkey", 20); err == nil {
		t.Fatal("expected kind validation error")
	}
}
