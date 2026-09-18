package main

import (
	"context"
	"io"
	"strings"
	"testing"
)

type startupPopupExecutor struct{ calls []string }

func (e *startupPopupExecutor) Run(_ context.Context, name string, args ...string) (string, error) {
	e.calls = append(e.calls, name+" "+strings.Join(args, " "))
	switch {
	case name == "pm" && len(args) > 0 && args[0] == "path":
		return "package:/data/app/popup.apk", nil
	case name == "dumpsys" && len(args) > 0 && args[0] == "package":
		return "versionCode=30728", nil
	case name == "dumpsys" && len(args) > 1 && args[0] == "activity" && args[1] == "services":
		return "com.openatx.xtest.popup.OverlayService", nil
	case name == "getprop":
		return "36", nil
	default:
		return "", nil
	}
}

func (e *startupPopupExecutor) RunBytes(context.Context, string, ...string) ([]byte, error) {
	return nil, nil
}

func TestStartPopupAtServerBootDefaultsToLaunchAndSupportsExplicitOptOut(t *testing.T) {
	executor := &startupPopupExecutor{}
	attempted, err := startPopupAtServerBoot(context.Background(), false, executor, io.Discard)
	if err != nil || !attempted {
		t.Fatalf("attempted=%t err=%v", attempted, err)
	}
	joined := strings.Join(executor.calls, "\n")
	if !strings.Contains(joined, "appops set com.openatx.xtest.popup SYSTEM_ALERT_WINDOW allow") || !strings.Contains(joined, "appops set --uid com.openatx.xtest.popup SYSTEM_ALERT_WINDOW allow") || !strings.Contains(joined, "am start -n com.openatx.xtest.popup/.PopupLauncherActivity") {
		t.Fatalf("default server startup did not launch popup: %s", joined)
	}

	executor.calls = nil
	attempted, err = startPopupAtServerBoot(context.Background(), true, executor, io.Discard)
	if err != nil || attempted || len(executor.calls) != 0 {
		t.Fatalf("--no-popup must skip every popup side effect: attempted=%t err=%v calls=%v", attempted, err, executor.calls)
	}
}
