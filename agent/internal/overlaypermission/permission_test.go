package overlaypermission

import (
	"context"
	"strings"
	"testing"
)

type recordingExecutor struct{ calls []string }

func (e *recordingExecutor) Run(_ context.Context, name string, args ...string) (string, error) {
	e.calls = append(e.calls, name+" "+strings.Join(args, " "))
	return "", nil
}
func (*recordingExecutor) RunBytes(context.Context, string, ...string) ([]byte, error) {
	return nil, nil
}

func TestGrantSetsPackageAndUIDModes(t *testing.T) {
	executor := &recordingExecutor{}
	if err := Grant(context.Background(), executor); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(executor.calls, "\n")
	for _, expected := range []string{
		"appops set " + Package + " SYSTEM_ALERT_WINDOW allow",
		"appops set --uid " + Package + " SYSTEM_ALERT_WINDOW allow",
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("missing %q in %s", expected, joined)
		}
	}
}
