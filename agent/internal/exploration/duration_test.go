package exploration

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestTimedSessionHasNoStepCeiling(t *testing.T) {
	timed := Config{Package: "com.example.app", DurationSeconds: 30}
	if err := timed.normalize(); err != nil || timed.MaxSteps != 0 {
		t.Fatalf("maxSteps=%d err=%v", timed.MaxSteps, err)
	}
	if stepBudgetReached(timed, 10000) {
		t.Fatal("a timed session stopped at the step count")
	}
	stepped := Config{Package: "com.example.app"}
	if err := stepped.normalize(); err != nil || stepped.MaxSteps != 100 {
		t.Fatalf("maxSteps=%d err=%v", stepped.MaxSteps, err)
	}
}

func TestTimedRoundReopensTriedActions(t *testing.T) {
	manager := New(nil, nil, nil)
	now := time.Now().UTC()
	manager.state.Running = true
	manager.state.StartedAt = &now
	action := Action{ID: "swipe-1", Type: "swipe"}
	node := &graphNode{
		semantic: "page", analysis: Analysis{Fingerprint: "fp", Actions: []Action{action}},
		tried: map[string]bool{"swipe-1": true}, actionAttempts: map[string]int{"swipe-1": 3},
	}
	manager.nodes["fp"] = node
	loop := &runLoop{exhaustedFingerprint: "fp", exhaustedCaptures: minimumExhaustedCapturesBeforeStop}
	manager.mu.Lock()
	choice := manager.chooseActionLocked(Config{DurationSeconds: 60, Seed: 1, EnableScroll: true, MaxBacktracks: 20}, node, node.analysis, "com.example.app/.Main", loop)
	manager.mu.Unlock()
	if choice.signal != loopNext || choice.action.ID != "swipe-1" || manager.state.StopReason != "" {
		t.Fatalf("signal=%v action=%s reason=%s", choice.signal, choice.action.ID, manager.state.StopReason)
	}
}

func TestTimedRoundRestartsWhenNothingRemainsSelectable(t *testing.T) {
	manager := New(nil, nil, nil)
	now := time.Now().UTC()
	manager.state.Running = true
	manager.state.StartedAt = &now
	node := &graphNode{semantic: "page", analysis: Analysis{Fingerprint: "fp"}, tried: map[string]bool{}}
	manager.nodes["fp"] = node
	loop := &runLoop{exhaustedFingerprint: "fp", exhaustedCaptures: minimumExhaustedCapturesBeforeStop}
	manager.mu.Lock()
	choice := manager.chooseActionLocked(Config{DurationSeconds: 60, MaxBacktracks: 20}, node, node.analysis, "com.example.app/.Main", loop)
	manager.mu.Unlock()
	if choice.signal != loopRestart || manager.state.StopReason != "" {
		t.Fatalf("signal=%v reason=%s", choice.signal, manager.state.StopReason)
	}
}

func TestExpiredDurationStopsInsteadOfStartingAnotherRound(t *testing.T) {
	manager := New(nil, nil, nil)
	started := time.Now().Add(-2 * time.Minute).UTC()
	manager.state.Running = true
	manager.state.StartedAt = &started
	action := Action{ID: "swipe-1", Type: "swipe"}
	node := &graphNode{
		semantic: "page", analysis: Analysis{Fingerprint: "fp", Actions: []Action{action}},
		tried: map[string]bool{"swipe-1": true}, actionAttempts: map[string]int{},
	}
	manager.nodes["fp"] = node
	loop := &runLoop{exhaustedFingerprint: "fp", exhaustedCaptures: minimumExhaustedCapturesBeforeStop}
	manager.mu.Lock()
	choice := manager.chooseActionLocked(Config{DurationSeconds: 30, EnableScroll: true, MaxBacktracks: 20}, node, node.analysis, "com.example.app/.Main", loop)
	manager.mu.Unlock()
	if choice.signal != loopStop || manager.state.StopReason != "duration_elapsed" {
		t.Fatalf("signal=%v reason=%s", choice.signal, manager.state.StopReason)
	}
}

func TestExhaustedActivityBacksOutBeforeColdStart(t *testing.T) {
	device := &scriptedDevice{}
	manager := New(device, device, device)
	config := Config{Package: "com.example"}
	loop := &runLoop{}
	if _, err := manager.restartExhaustedTarget(context.Background(), config, loop, "com.example/.RuleActivity"); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.restartExhaustedTarget(context.Background(), config, loop, "com.example/.RuleActivity"); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.restartExhaustedTarget(context.Background(), config, loop, "com.example/.RuleActivity"); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.restartExhaustedTarget(context.Background(), config, loop, "com.example/.RuleActivity"); err != nil {
		t.Fatal(err)
	}
	cold, err := manager.restartExhaustedTarget(context.Background(), config, loop, "com.example/.RuleActivity")
	if err != nil || !cold {
		t.Fatal(err)
	}
	device.mu.Lock()
	defer device.mu.Unlock()
	joined := make([]string, 0, len(device.commands))
	for _, command := range device.commands {
		joined = append(joined, strings.Join(command, " "))
	}
	commands := strings.Join(joined, "\n")
	if commands != "input keyevent BACK\nwm size\ninput swipe 8 1200 360 1200 350\ninput swipe 1072 1200 720 1200 350\ninput swipe 540 360 540 1320 350\nam force-stop com.example\nsh /system/bin/monkey -p com.example -c android.intent.category.LAUNCHER 1" {
		t.Fatalf("commands =\n%s", commands)
	}
	if manager.State().Recoveries != 5 || loop.restartedActivity != "" || loop.escapeAttempts != 0 {
		t.Fatalf("recoveries=%d activity=%s attempts=%d", manager.State().Recoveries, loop.restartedActivity, loop.escapeAttempts)
	}
}

const unhandledConsentHierarchy = `<hierarchy><node package="com.example" class="android.widget.FrameLayout" bounds="[0,0][400,400]" enabled="true" visible-to-user="true"><node package="com.example" class="android.widget.TextView" text="Privacy Policy" bounds="[0,0][400,80]" enabled="true" visible-to-user="true"/><node package="com.example" class="android.widget.TextView" text="By tapping below" bounds="[0,100][400,180]" enabled="true" visible-to-user="true"/><node package="com.example" class="android.widget.Button" text="Decline" bounds="[0,200][200,300]" clickable="true" enabled="true" visible-to-user="true"/></node></hierarchy>`

func TestTimedUnhandledSpecialSceneRestarts(t *testing.T) {
	manager := New(nil, nil, nil)
	now := time.Now().UTC()
	manager.state.Running = true
	manager.state.StartedAt = &now
	loop := &runLoop{}
	signal := manager.handleSpecialScene(context.Background(), Config{Package: "com.example", DurationSeconds: 60}, loop, "com.example", "com.example/.Main", "", unhandledConsentHierarchy)
	if signal != loopRestart || manager.state.StopReason != "" || !manager.state.Running {
		t.Fatalf("signal=%v state=%#v", signal, manager.state)
	}
}

func TestPausedSpecialPolicyStillStopsTimedSession(t *testing.T) {
	manager := New(nil, nil, nil)
	now := time.Now().UTC()
	manager.state.Running = true
	manager.state.StartedAt = &now
	loop := &runLoop{}
	signal := manager.handleSpecialScene(context.Background(), Config{Package: "com.example", DurationSeconds: 60, SpecialHandling: SpecialHandling{ConsentPolicy: "pause"}}, loop, "com.example", "com.example/.Main", "", unhandledConsentHierarchy)
	if signal != loopStop || manager.state.StopReason != "policy_required" {
		t.Fatalf("signal=%v reason=%s", signal, manager.state.StopReason)
	}
}

func TestTimedExternalRecoveryColdStartsAfterBoundedAttempts(t *testing.T) {
	device := &scriptedDevice{foreground: "com.other"}
	manager := New(device, device, device)
	now := time.Now().UTC()
	manager.state.Running = true
	manager.state.StartedAt = &now
	loop := &runLoop{pending: -1, externalAttempts: map[string]int{"com.other": 3}, startupReleased: true, exhaustedCaptures: 2}
	signal := manager.recoverOffTarget(context.Background(), Config{Package: "com.example", DurationSeconds: 60, IntervalMillis: 100}, loop, "com.other", "com.other/.Main", "")
	if signal != loopContinue || manager.State().StopReason != "" || len(loop.externalAttempts) != 0 || loop.startupReleased || loop.exhaustedCaptures != 0 || !loop.initialSettleUntil.After(now) {
		t.Fatalf("signal=%v reason=%s attempts=%v released=%v until=%s", signal, manager.State().StopReason, loop.externalAttempts, loop.startupReleased, loop.initialSettleUntil)
	}
	device.mu.Lock()
	defer device.mu.Unlock()
	joined := make([]string, 0, len(device.commands))
	for _, command := range device.commands {
		joined = append(joined, strings.Join(command, " "))
	}
	commands := strings.Join(joined, "\n")
	if commands != "am force-stop com.example\nsh /system/bin/monkey -p com.example -c android.intent.category.LAUNCHER 1" {
		t.Fatalf("commands =\n%s", commands)
	}
}

func TestUntimedExternalRecoveryStillStops(t *testing.T) {
	device := &scriptedDevice{foreground: "com.other"}
	manager := New(device, device, device)
	manager.state.Running = true
	loop := &runLoop{pending: -1, externalAttempts: map[string]int{"com.other": 3}}
	signal := manager.recoverOffTarget(context.Background(), Config{Package: "com.example", IntervalMillis: 100}, loop, "com.other", "com.other/.Main", "")
	if signal != loopStop || manager.State().StopReason != "safety_stop" {
		t.Fatalf("signal=%v reason=%s err=%s", signal, manager.State().StopReason, manager.State().Error)
	}
}
