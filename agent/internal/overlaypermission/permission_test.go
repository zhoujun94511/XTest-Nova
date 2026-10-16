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

type failingExecutor struct {
	output string
	err    error
	calls  []string
}

func (e *failingExecutor) Run(_ context.Context, name string, args ...string) (string, error) {
	e.calls = append(e.calls, name+" "+strings.Join(args, " "))
	return e.output, e.err
}

func TestGrantIgnoresVendorLockedAppOps(t *testing.T) {
	executor := &failingExecutor{
		output: "uid 2000 does not have android.permission.MANAGE_APP_OPS_MODES.",
		err:    errVendorLocked,
	}
	if err := Grant(context.Background(), executor); err != nil {
		t.Fatal(err)
	}
	if len(executor.calls) != 2 {
		t.Fatalf("calls=%d want 2", len(executor.calls))
	}
}

func TestVendorBlocksAppOps(t *testing.T) {
	if !vendorBlocksAppOps("uid 2000 does not have android.permission.MANAGE_APP_OPS_MODES.", nil) {
		t.Fatal("expected MANAGE_APP_OPS_MODES to be vendor-locked")
	}
	if vendorBlocksAppOps("Unknown package: com.example", errNotFound) {
		t.Fatal("unknown package is not a vendor appops lock")
	}
}

type staticError string

func (e staticError) Error() string { return string(e) }

const errVendorLocked = staticError("SecurityException: uid 2000 does not have android.permission.MANAGE_APP_OPS_MODES")
const errNotFound = staticError("exit status 255")

func TestParseAllowedReadsAppOpsGet(t *testing.T) {
	if !ParseAllowed("SYSTEM_ALERT_WINDOW: allow; time=+17s ago (running)\n") {
		t.Fatal("allow")
	}
	if ParseAllowed("SYSTEM_ALERT_WINDOW: default; time=+1s ago\n") {
		t.Fatal("default is not allow")
	}
	if ParseAllowed("No operations.\nDefault mode: default\n") {
		t.Fatal("missing op is not allow")
	}
}

func TestOpenSettingsUsesPackageOverlayIntent(t *testing.T) {
	executor := &recordingExecutor{}
	if err := OpenSettings(context.Background(), executor); err != nil {
		t.Fatal(err)
	}
	want := "am start -a android.settings.action.MANAGE_OVERLAY_PERMISSION -d package:" + Package
	if len(executor.calls) == 0 || executor.calls[0] != want {
		t.Fatalf("calls=%v", executor.calls)
	}
}

func TestStartOverlayServiceUsesForegroundService(t *testing.T) {
	executor := &recordingExecutor{}
	if err := StartOverlayService(context.Background(), executor); err != nil {
		t.Fatal(err)
	}
	want := "am start-foreground-service -n " + Package + "/.OverlayService"
	if len(executor.calls) == 0 || executor.calls[0] != want {
		t.Fatalf("calls=%v", executor.calls)
	}
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
