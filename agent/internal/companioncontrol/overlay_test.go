package companioncontrol

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

type scriptExecutor struct {
	calls     [][]string
	responses []string
	errs      []error
}

func (f *scriptExecutor) Run(_ context.Context, name string, args ...string) (string, error) {
	f.calls = append(f.calls, append([]string{name}, args...))
	index := len(f.calls) - 1
	if index < len(f.errs) && f.errs[index] != nil {
		return "", f.errs[index]
	}
	if index < len(f.responses) {
		return f.responses[index], nil
	}
	return "", errors.New("unexpected overlay call")
}

func TestSuppressLeavesAbsentOverlayUntouched(t *testing.T) {
	executor := &scriptExecutor{responses: []string{"Result: Bundle[{state=absent}]"}}
	restore, err := SuppressOverlay(context.Background(), executor)
	if err != nil || restore {
		t.Fatalf("restore=%t err=%v", restore, err)
	}
	want := []string{"content", "call", "--uri", metadataURI, "--method", "overlay-status"}
	if len(executor.calls) != 1 || !reflect.DeepEqual(executor.calls[0], want) {
		t.Fatalf("calls=%v", executor.calls)
	}
}

func TestSuppressWaitsUntilVisibleOverlayIsGone(t *testing.T) {
	executor := &scriptExecutor{responses: []string{
		"Result: Bundle[{state=visible}]",
		"Result: Bundle[{state=requested-suppress}]",
		"Result: Bundle[{state=visible}]",
		"Result: Bundle[{state=suppressed}]",
	}}
	restore, err := SuppressOverlay(context.Background(), executor)
	if err != nil || !restore {
		t.Fatalf("restore=%t err=%v", restore, err)
	}
	if len(executor.calls) != 4 || executor.calls[1][len(executor.calls[1])-1] != "overlay-suppress" {
		t.Fatalf("calls=%v", executor.calls)
	}
}

func TestSuppressAcceptsTheEdgeHandle(t *testing.T) {
	executor := &scriptExecutor{responses: []string{
		"Result: Bundle[{state=visible}]",
		"Result: Bundle[{state=requested-suppress}]",
		"Result: Bundle[{state=minimized}]",
	}}
	restore, err := SuppressOverlay(context.Background(), executor)
	if err != nil || !restore {
		t.Fatalf("restore=%t err=%v", restore, err)
	}
}

func TestSuppressReportsWindowThatStaysVisible(t *testing.T) {
	executor := &scriptExecutor{responses: []string{
		"Result: Bundle[{state=visible}]",
		"Result: Bundle[{state=requested-suppress}]",
		"Result: Bundle[{state=visible}]",
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	restore, err := SuppressOverlay(ctx, executor)
	if err == nil || restore {
		t.Fatalf("restore=%t err=%v", restore, err)
	}
}

func TestRestoreOverlayReportsCommandFailure(t *testing.T) {
	executor := &scriptExecutor{errs: []error{errors.New("unavailable")}}
	if err := RestoreOverlay(context.Background(), executor); err == nil {
		t.Fatal("expected restore failure")
	}
}
