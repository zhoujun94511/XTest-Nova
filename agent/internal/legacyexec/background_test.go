package legacyexec

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"
)

func TestBackgroundReapsCompletedCommand(t *testing.T) {
	background := NewBackground(1)
	pid, err := background.Start("echo nova-background")
	if err != nil || pid <= 0 {
		t.Fatalf("Start() pid=%d err=%v", pid, err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for background.Running() != 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if background.Running() != 0 {
		t.Fatal("completed background command was not reaped")
	}
}

func TestBackgroundLimit(t *testing.T) {
	background := NewBackground(1)
	background.slots <- struct{}{}
	if _, err := background.Start("echo should-not-run"); !errors.Is(err, ErrBackgroundLimit) {
		t.Fatalf("Start() err=%v, want ErrBackgroundLimit", err)
	}
	<-background.slots
}

func TestBackgroundStopAll(t *testing.T) {
	background := NewBackground(1)
	command := "sleep 30"
	if runtime.GOOS == "windows" {
		command = "ping -n 30 127.0.0.1 >NUL"
	}
	if _, err := background.Start(command); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := background.StopAll(ctx); err != nil {
		t.Fatal(err)
	}
	if running := background.Running(); running != 0 {
		t.Fatalf("running commands=%d after StopAll", running)
	}
	if err := background.StopAll(ctx); err != nil {
		t.Fatalf("idempotent StopAll: %v", err)
	}
}
