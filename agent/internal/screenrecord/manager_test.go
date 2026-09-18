package screenrecord

import (
	"os"
	"testing"
)

func TestStartFailureRemovesSessionDirectory(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PATH", t.TempDir())
	manager := New(root)
	if err := manager.Start(); err == nil {
		t.Fatal("expected screenrecord start failure")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 || manager.Running() {
		t.Fatalf("failed start left state behind: entries=%d running=%v", len(entries), manager.Running())
	}
}

func TestIdleStopIsIdempotent(t *testing.T) {
	files, err := New(t.TempDir()).Stop()
	if err != nil || len(files) != 0 {
		t.Fatalf("idle stop returned files=%v err=%v", files, err)
	}
}
