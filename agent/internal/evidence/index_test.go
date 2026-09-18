package evidence

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zhoujun94511/xtest-nova/agent/internal/execution"
)

func TestBuildRejectsOutsideAndHashesInsideEvidence(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "finish.png")
	if err := os.WriteFile(inside, []byte("image"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.log")
	if err := os.WriteFile(outside, []byte("log"), 0o644); err != nil {
		t.Fatal(err)
	}
	index := Build(root, "com.example", "sha256:case", execution.NewIdentity("runner", "run-1", 1), []string{inside, outside})
	if len(index.Items) != 1 || index.Items[0].SHA256 == "" || index.Items[0].Type != "screenshot" {
		t.Fatalf("items = %+v", index.Items)
	}
	if len(index.Errors) != 1 {
		t.Fatalf("errors = %+v", index.Errors)
	}
	if err := Validate(index); err == nil {
		t.Fatal("Validate accepted an evidence index with build errors")
	}
	indexPath := filepath.Join(root, "evidence.json")
	if err := Write(indexPath, index); err == nil {
		t.Fatal("Write accepted an evidence index with build errors")
	}
	if _, err := os.Stat(indexPath); !os.IsNotExist(err) {
		t.Fatalf("invalid evidence index was published: %v", err)
	}
}

func TestBuildForPublicationUsesFinalPaths(t *testing.T) {
	parent := t.TempDir()
	staging := filepath.Join(parent, ".run.staging")
	final := filepath.Join(parent, "run")
	if err := os.Mkdir(staging, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(staging, "result.json")
	if err := os.WriteFile(file, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	index := BuildForPublication(staging, final, "com.example", "", execution.Identity{}, []string{file})
	if err := Validate(index); err != nil {
		t.Fatal(err)
	}
	if len(index.Items) != 1 || index.Items[0].Path != filepath.ToSlash(filepath.Join(final, "result.json")) {
		t.Fatalf("items = %+v", index.Items)
	}
}
