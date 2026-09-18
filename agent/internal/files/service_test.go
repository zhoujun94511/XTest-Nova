package files

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveAllowedAndTraversal(t *testing.T) {
	service := New("/data/local/tmp", "/sdcard")
	path, err := service.Resolve("/data/local/tmp/result.txt")
	if err != nil || !strings.HasSuffix(strings.ReplaceAll(path, "\\", "/"), "/data/local/tmp/result.txt") {
		t.Fatalf("path=%q err=%v", path, err)
	}
	if _, err := service.Resolve("/data/local/tmp/../../system/build.prop"); err == nil {
		t.Fatal("traversal accepted")
	}
	if _, err := service.Resolve("/etc/passwd"); err == nil {
		t.Fatal("outside path accepted")
	}
}

func TestOpenRejectsSymlinkOutsideRoot(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	target := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(target, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "escape.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, _, err := New(root).Open(link); err == nil {
		t.Fatal("opened symlink outside allowed root")
	}
}

func TestSaveRejectsRatherThanTruncates(t *testing.T) {
	root := t.TempDir()
	service := New(root)
	path := filepath.Join(root, "oversized.bin")
	if _, err := service.save(path, bytes.NewReader([]byte("12345")), 0644, 4); err == nil {
		t.Fatal("oversized upload was accepted")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("oversized destination exists: %v", err)
	}
}

func TestSaveDoesNotCreateDirectoriesThroughOutsideSymlink(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	link := filepath.Join(root, "outside-link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	service := New(root)
	_, err := service.Save(filepath.Join(link, "must-not-exist", "payload.txt"), strings.NewReader("blocked"), 0644)
	if err == nil {
		t.Fatal("write through outside symlink was accepted")
	}
	if _, statErr := os.Stat(filepath.Join(outside, "must-not-exist")); !os.IsNotExist(statErr) {
		t.Fatalf("outside directory was created before rejection: %v", statErr)
	}
}
