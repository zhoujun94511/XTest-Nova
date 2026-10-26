package exploration

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestSettleReleasesOnActivityOrWindowChange(t *testing.T) {
	now := time.Unix(100, 0)
	until := now.Add(transitionSettleDuration)
	baseline := pageGeneration{activity: "com.example/.Splash", windowVersion: "17"}
	same := pageGeneration{activity: "com.example/.Main", windowVersion: "17"}
	if settleReady(now, until, baseline, baseline, baseline) {
		t.Fatal("unchanged page ended the wait")
	}
	if !settleReady(now, until, baseline, baseline, same) {
		t.Fatal("activity change kept waiting")
	}
	changed := pageGeneration{activity: "com.example/.Splash", windowVersion: "18"}
	if settleReady(now, until, baseline, baseline, changed) {
		t.Fatal("first relayout ended the wait")
	}
	if !settleReady(now, until, baseline, changed, changed) {
		t.Fatal("repeated relayout kept waiting")
	}
	if !settleReady(until, until, baseline, baseline, baseline) {
		t.Fatal("deadline kept waiting")
	}
	if !settleReady(now, time.Time{}, baseline, baseline, baseline) {
		t.Fatal("inactive wait blocked the page")
	}
}

func TestStartupHoldIgnoresSingleRelayout(t *testing.T) {
	now := time.Unix(100, 0)
	until := now.Add(initialAsyncSettleDuration)
	var baseline pageGeneration
	released := false
	splash := pageGeneration{activity: "com.example/.SplashActivity", windowVersion: "4"}
	if !observeStartup(now, until, splash.activity, &baseline, &released, splash) || released {
		t.Fatal("first splash frame was not held")
	}
	if !observeStartup(now, until, splash.activity, &baseline, &released, splash) {
		t.Fatal("same splash window was released")
	}
	next := pageGeneration{activity: "com.example/.SplashActivity", windowVersion: "5"}
	if !observeStartup(now, until, next.activity, &baseline, &released, next) || released {
		t.Fatal("one splash relayout released the hold")
	}
	moved := pageGeneration{activity: "com.example/.OtherSplash", windowVersion: "5"}
	if observeStartup(now, until, moved.activity, &baseline, &released, moved) || !released {
		t.Fatal("a different startup activity stayed held")
	}
}

func TestColdRelaunchRearmsStartupHold(t *testing.T) {
	now := time.Unix(100, 0)
	loop := &runLoop{startupReleased: true, initialSettleUntil: now, startupGeneration: pageGeneration{activity: "com.example/.SplashActivity", windowVersion: "4"}, emptyCaptures: 3, exhaustedCaptures: 2, exhaustedFingerprint: "old"}
	armStartupHold(loop, now)
	var baseline pageGeneration
	if !observeStartup(now, loop.initialSettleUntil, "com.example/.SplashActivity", &baseline, &loop.startupReleased, pageGeneration{activity: "com.example/.SplashActivity", windowVersion: "9"}) || loop.startupReleased {
		t.Fatal("relaunch did not hold the new splash")
	}
	if loop.emptyCaptures != 0 || loop.exhaustedCaptures != 0 || loop.exhaustedFingerprint != "" {
		t.Fatal("relaunch kept the previous empty-page exhaustion")
	}
}

type relayoutFocusDevice struct {
	mu      sync.Mutex
	version int
}

func (d *relayoutFocusDevice) Hierarchy(context.Context) (string, error) { return pageA, nil }
func (*relayoutFocusDevice) ForegroundPackage(context.Context) (string, error) {
	return "com.example", nil
}
func (*relayoutFocusDevice) ForegroundActivity(context.Context) (string, error) {
	return "com.example/com.example.MainActivity", nil
}
func (d *relayoutFocusDevice) ForegroundFocus(context.Context) (string, string, error) {
	d.mu.Lock()
	d.version++
	version := d.version
	d.mu.Unlock()
	return "com.example/com.example.MainActivity", fmt.Sprintf("%d", version), nil
}
func (d *relayoutFocusDevice) Run(_ context.Context, name string, args ...string) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return "", nil
}
func (*relayoutFocusDevice) RunBytes(context.Context, string, ...string) ([]byte, error) {
	return nil, nil
}

func TestRelayoutSequenceDoesNotInvalidateCapture(t *testing.T) {
	device := &relayoutFocusDevice{}
	manager := New(device, device, device)
	if _, err := manager.Start(Config{RequestID: "relayout-capture", Package: "com.example", MaxSteps: 1, IntervalMillis: 100, Execute: true}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) && manager.State().Steps < 1 {
		time.Sleep(10 * time.Millisecond)
	}
	stopContext, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, _ = manager.Stop(stopContext)
	state := manager.State()
	if state.StopReason == "unstable_capture_limit" || state.Steps < 1 {
		t.Fatalf("state=%#v", state)
	}
}

func TestEmptyPageStillNeedsRepeatedCapturesAfterRelease(t *testing.T) {
	now := time.Unix(100, 0)
	current := pageGeneration{activity: "com.example/.Main", windowVersion: "8"}
	baseline := pageGeneration{activity: "com.example/.Splash", windowVersion: "4"}
	if !deferEmptyPage(false, now, now.Add(transitionSettleDuration), baseline, baseline, current, 1) {
		t.Fatal("a single empty capture was accepted")
	}
	if deferEmptyPage(false, now, now.Add(transitionSettleDuration), baseline, baseline, current, minimumEmptyCapturesBeforeExhaustion) {
		t.Fatal("confirmed empty page stayed deferred")
	}
}
