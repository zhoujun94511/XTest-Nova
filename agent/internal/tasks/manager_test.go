package tasks

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidateURL(t *testing.T) {
	for _, value := range []string{"file:///etc/passwd", "http://127.0.0.1/a", "http://localhost/a", "ftp://example.com/a"} {
		if validateURL(value) == nil {
			t.Fatalf("accepted %s", value)
		}
	}
	if err := validateURL("https://example.com/app.apk"); err != nil {
		t.Fatal(err)
	}
}

func TestCompletedTaskHistoryIsBounded(t *testing.T) {
	manager := New()
	manager.mu.Lock()
	for index := 0; index < maxRetainedTasks+20; index++ {
		ended := time.Unix(int64(index), 0)
		id := newID()
		manager.tasks[id] = &task{state: State{ID: id, CreatedAt: ended, EndedAt: &ended}}
	}
	manager.pruneLocked()
	count := len(manager.tasks)
	manager.mu.Unlock()
	if count != maxRetainedTasks {
		t.Fatalf("retained %d tasks, want %d", count, maxRetainedTasks)
	}
}

func TestValidateMode(t *testing.T) {
	if err := validateMode(0644); err != nil {
		t.Fatal(err)
	}
	if err := validateMode(os.ModeSetuid | 0755); err == nil {
		t.Fatal("accepted non-permission mode bits")
	}
}

func TestConcurrentTaskLimitIsBounded(t *testing.T) {
	manager := New()
	for index := 0; index < maxConcurrentTasks; index++ {
		manager.slots <- struct{}{}
	}
	if _, err := manager.Start("https://example.com/app.apk", filepath.Join(t.TempDir(), "app.apk"), 0644, nil, false); !errors.Is(err, ErrTaskLimit) {
		t.Fatalf("limit error = %v", err)
	}
}

func TestSafeTransportRejectsResolvedLocalAddress(t *testing.T) {
	transport := safeTransport()
	_, err := transport.DialContext(context.Background(), "tcp", "localhost:80")
	if err == nil || !strings.Contains(err.Error(), "private or local") {
		t.Fatalf("expected local-address rejection, got %v", err)
	}
}
