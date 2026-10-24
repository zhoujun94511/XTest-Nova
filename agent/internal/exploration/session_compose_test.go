package exploration

import (
	"context"
	"errors"
	"testing"
	"time"
)

type commandExecutor struct {
	commands []string
	output   string
}

func (e *commandExecutor) Run(_ context.Context, name string, _ ...string) (string, error) {
	e.commands = append(e.commands, name)
	return e.output, nil
}

func (e *commandExecutor) RunBytes(context.Context, string, ...string) ([]byte, error) {
	return nil, nil
}

func TestActivityGuardMatchesClassOrComponent(t *testing.T) {
	config := Config{Package: "com.shorts.wave.drama", ActivityMode: "allowlist", Activities: []string{".FeedActivity"}}
	if !activityAllowed(config, "com.shorts.wave.drama/com.shorts.wave.drama.FeedActivity") {
		t.Fatal("picker class should match the canonical foreground component")
	}
	if activityAllowed(config, "com.shorts.wave.drama/com.shorts.wave.drama.PayActivity") {
		t.Fatal("activity outside the allowlist was accepted")
	}
	config.Activities = []string{"FeedActivity"}
	if !activityAllowed(config, "com.shorts.wave.drama/com.shorts.wave.drama.FeedActivity") {
		t.Fatal("class name should match the resumed component")
	}
	config.ActivityMode = "blocklist"
	config.Activities = []string{".FeedActivity"}
	if activityAllowed(config, "com.shorts.wave.drama/com.shorts.wave.drama.FeedActivity") {
		t.Fatal("blocklist activity was accepted")
	}
}

func TestBlacklistRemovesOnlyTheNamedControl(t *testing.T) {
	analysis := Analysis{Actions: []Action{{Text: "播放"}, {ResourceID: "com.shorts.wave.drama:id/ad_button_close", Text: "关闭"}}}
	filterBlockedControls(&analysis, []string{"ad_button"})
	if len(analysis.Actions) != 1 || analysis.Actions[0].Text != "播放" {
		t.Fatalf("blacklist filter = %+v", analysis.Actions)
	}
}

func TestCoordinateGestureStaysInsideTheSafeBand(t *testing.T) {
	gesture := coordinateGesture(7, 0, 1080, 2400)
	if gesture.Type != "tap" || gesture.X < 108 || gesture.X > 972 || gesture.Y < 400 || gesture.Y > 2160 {
		t.Fatalf("gesture = %+v", gesture)
	}
	if _, _, ok := screenSize("Physical size: 1440x3200\nOverride size: 1080x2400\n"); !ok {
		t.Fatal("override size was not selected")
	}
	level, ok := batteryLevel("AC powered: false\n  level: 12\n")
	if !ok || level != 12 {
		t.Fatalf("battery = %d %v", level, ok)
	}
}

func TestFailedTargetReplayStaysRetryable(t *testing.T) {
	manager := New(nil, nil, &commandExecutor{})
	manager.playedTargets = map[string]bool{}
	manager.targetReplayAfter = map[string]time.Time{}
	calls := 0
	manager.targetReplay = func(context.Context, string, string, string) error {
		calls++
		if calls == 1 {
			return errors.New("device control is owned by another execution session")
		}
		return nil
	}
	config := Config{Package: "com.example", TargetCases: []TargetCase{{Activity: ".Feed", Case: "open"}}}
	activity := "com.example/com.example.Feed"
	if signal := manager.replayTargetPage(context.Background(), config, &runLoop{}, activity); signal != loopNext || calls != 1 || len(manager.playedTargets) != 0 {
		t.Fatalf("failed replay signal=%v calls=%d played=%v", signal, calls, manager.playedTargets)
	}
	if signal := manager.replayTargetPage(context.Background(), config, &runLoop{}, activity); signal != loopNext || calls != 1 {
		t.Fatal("failed replay was retried before the cooldown")
	}
	manager.targetReplayAfter[".Feed\x00\x00open"] = time.Now().Add(-time.Second)
	manager.state.Running = true
	if signal := manager.replayTargetPage(context.Background(), config, &runLoop{}, activity); signal != loopContinue || calls != 2 || !manager.playedTargets[".Feed\x00\x00open"] {
		t.Fatalf("retry signal=%v calls=%d played=%v", signal, calls, manager.playedTargets)
	}
	steps := manager.Steps()
	if len(steps) != 1 || steps[0].Action.Type != "replay" || steps[0].Action.Text != "open" {
		t.Fatalf("replay step = %+v", steps)
	}
}

func TestCancelledTargetReplayStopsTheLoop(t *testing.T) {
	manager := New(nil, nil, &commandExecutor{})
	manager.playedTargets = map[string]bool{}
	manager.targetReplayAfter = map[string]time.Time{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	manager.targetReplay = func(context.Context, string, string, string) error {
		return errors.New("target-page case did not finish")
	}
	config := Config{Package: "com.example", TargetCases: []TargetCase{{Activity: ".Feed", Case: "open"}}}
	signal := manager.replayTargetPage(ctx, config, &runLoop{}, "com.example/com.example.Feed")
	if signal != loopStop || len(manager.playedTargets) != 0 {
		t.Fatalf("cancelled replay signal=%v played=%v", signal, manager.playedTargets)
	}
}

func TestBoundedRenderStopsAfterItsBudget(t *testing.T) {
	if !renderFallbackEnabled("bounded") || renderFallbackEnabled("off") || renderFallbackEnabled("") {
		t.Fatal("render mode gate changed")
	}
	if !renderSurface(`<node class="android.view.SurfaceView" />`) {
		t.Fatal("surface page was not recognized")
	}
	loop := &runLoop{renderActivity: "com.example/.Game", renderAttempts: boundedRenderAttempts}
	executor := &commandExecutor{output: "Physical size: 1080x2400"}
	manager := New(nil, nil, executor)
	signal := manager.coordinateWhenUnlabeled(context.Background(), Config{RenderFallbackMode: "bounded"}, loop, "com.example/.Game", `<node class="android.view.SurfaceView" />`, &Analysis{})
	if signal != loopNext || len(executor.commands) != 0 {
		t.Fatal("bounded render continued after its budget")
	}
}

func TestCoordinateGestureIsRecorded(t *testing.T) {
	loop := &runLoop{}
	executor := &commandExecutor{output: "Physical size: 1080x2400"}
	manager := New(nil, nil, executor)
	manager.state.Running = true
	signal := manager.coordinateWhenUnlabeled(context.Background(), Config{RenderFallbackMode: "bounded", Seed: 7}, loop, "com.example/.Game", `<node class="android.view.SurfaceView" />`, &Analysis{})
	steps := manager.Steps()
	if signal != loopContinue || len(steps) != 1 || steps[0].Action.Type == "" || steps[0].From != "com.example/.Game" {
		t.Fatalf("signal=%v steps=%+v", signal, steps)
	}
}
