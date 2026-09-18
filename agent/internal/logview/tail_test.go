package logview

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTailIsAliasBoundedAndFilterable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.log")
	if err := os.WriteFile(path, []byte("info\nerror one\nerror two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reader := New(map[string]string{"agent": path})
	result, err := reader.Tail("agent", 1, "ERROR")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Lines) != 1 || result.Lines[0] != "error two" || !result.Truncated {
		t.Fatalf("unexpected result: %+v", result)
	}
	if _, err = reader.Tail("../../secret", 10, ""); err == nil {
		t.Fatal("expected alias validation error")
	}
}
