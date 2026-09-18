package automation

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type partialDumpExecutor struct {
	paths []string
}

func (e *partialDumpExecutor) Run(_ context.Context, _ string, args ...string) (string, error) {
	path := args[len(args)-1]
	e.paths = append(e.paths, path)
	if err := os.WriteFile(path, []byte("partial"), 0o600); err != nil {
		return "", err
	}
	return "", errors.New("dump timed out after creating output")
}

func (*partialDumpExecutor) RunBytes(context.Context, string, ...string) ([]byte, error) {
	return nil, errors.New("not used")
}

func TestSystemDumpRemovesPartialFilesAfterCommandFailure(t *testing.T) {
	executor := &partialDumpExecutor{}
	provider := &systemDumpProvider{executor: executor, path: filepath.Join(t.TempDir(), "window.xml")}
	if _, err := provider.Hierarchy(context.Background()); err == nil {
		t.Fatal("partial dump unexpectedly succeeded")
	}
	if len(executor.paths) != 2 {
		t.Fatalf("attempts = %d, want 2", len(executor.paths))
	}
	for _, path := range executor.paths {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("partial hierarchy remains at %s: %v", path, err)
		}
	}
}
