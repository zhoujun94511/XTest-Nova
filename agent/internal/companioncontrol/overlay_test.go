package companioncontrol

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type fakeExecutor struct {
	name string
	args []string
	out  string
	err  error
}

func (f *fakeExecutor) Run(_ context.Context, name string, args ...string) (string, error) {
	f.name, f.args = name, append([]string(nil), args...)
	return f.out, f.err
}

func TestSuppressOverlayUsesProtectedCompanionControl(t *testing.T) {
	executor := &fakeExecutor{out: "Result: Bundle[{state=suppressed}]"}
	if err := SuppressOverlay(context.Background(), executor); err != nil {
		t.Fatal(err)
	}
	want := []string{"call", "--uri", metadataURI, "--method", "overlay-suppress"}
	if executor.name != "content" || !reflect.DeepEqual(executor.args, want) {
		t.Fatalf("unexpected command %q %v", executor.name, executor.args)
	}
}

func TestRestoreOverlayReportsCommandFailure(t *testing.T) {
	executor := &fakeExecutor{err: errors.New("unavailable")}
	if err := RestoreOverlay(context.Background(), executor); err == nil {
		t.Fatal("expected restore failure")
	}
}
