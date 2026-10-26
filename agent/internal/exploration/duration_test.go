package exploration

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestDurationFloorIsOneMinute(t *testing.T) {
	missing := Config{Package: "com.example.app", MaxSteps: 10}
	if err := missing.normalize(); err != nil || missing.DurationSeconds != 60 || missing.MaxSteps != 0 {
		t.Fatalf("duration=%d maxSteps=%d err=%v", missing.DurationSeconds, missing.MaxSteps, err)
	}
	short := Config{Package: "com.example.app", DurationSeconds: 30}
	if err := short.normalize(); err != nil || short.DurationSeconds != 60 {
		t.Fatalf("duration=%d err=%v", short.DurationSeconds, err)
	}
	long := Config{Package: "com.example.app", DurationSeconds: 604801}
	if err := long.normalize(); err == nil {
		t.Fatal("duration above 604800 was accepted")
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

func TestEmptyForegroundWaitsWithoutLeaving(t *testing.T) {
	device := &scriptedDevice{}
	manager := New(device, device, device)
	loop := &runLoop{pending: -1, externalAttempts: map[string]int{}}
	signal := manager.recoverOffTarget(context.Background(), Config{Package: "com.example", IntervalMillis: 100}, loop, "", "", "")
	if signal != loopContinue || len(loop.externalAttempts) != 0 {
		t.Fatalf("signal=%v attempts=%v", signal, loop.externalAttempts)
	}
	device.mu.Lock()
	defer device.mu.Unlock()
	if len(device.commands) != 0 {
		t.Fatalf("commands = %#v", device.commands)
	}
}

func TestMainShellEscapeDoesNotColdStart(t *testing.T) {
	device := &scriptedDevice{}
	manager := New(device, device, device)
	loop := &runLoop{}
	activity := "live.shorttv.apps/com.startshorts.androidplayer.ui.activity.MainActivity"
	cold, err := manager.restartExhaustedTarget(context.Background(), Config{Package: "live.shorttv.apps"}, loop, activity)
	if err != nil || cold || loop.escapeAttempts != 1 {
		t.Fatalf("cold=%v err=%v attempts=%d", cold, err, loop.escapeAttempts)
	}
	cold, err = manager.restartExhaustedTarget(context.Background(), Config{Package: "live.shorttv.apps"}, loop, activity)
	if err != nil || !cold {
		t.Fatalf("second cold=%v err=%v", cold, err)
	}
	cold, err = manager.restartExhaustedTarget(context.Background(), Config{Package: "live.shorttv.apps"}, loop, activity)
	if err != nil || cold {
		t.Fatalf("third cold=%v err=%v", cold, err)
	}
	device.mu.Lock()
	defer device.mu.Unlock()
	joined := strings.Join(flattenCommands(device.commands), "\n")
	if strings.Count(joined, "input keyevent BACK") != 1 || strings.Count(joined, "am force-stop live.shorttv.apps") != 1 || strings.Contains(joined, "input swipe") {
		t.Fatalf("commands =\n%s", joined)
	}
}

func TestUnseenTapsRotateBeforeARepeatedControl(t *testing.T) {
	manager := New(nil, nil, nil)
	now := time.Now().UTC()
	manager.state.Running = true
	manager.state.StartedAt = &now
	actions := []Action{
		{ID: "alpha", Type: "tap", Description: "Alpha", Class: "android.view.ViewGroup", X: 100, Y: 200, Bounds: Bounds{Left: 0, Top: 100, Right: 200, Bottom: 300}},
		{ID: "beta", Type: "tap", Description: "Beta", Class: "android.view.ViewGroup", X: 400, Y: 200, Bounds: Bounds{Left: 300, Top: 100, Right: 500, Bottom: 300}},
		{ID: "login", Type: "tap", Text: "Sign in", Class: "android.widget.Button", X: 300, Y: 800, Bounds: Bounds{Left: 100, Top: 700, Right: 500, Bottom: 900}},
	}
	seen := map[string]bool{}
	for index := 0; index < 2; index++ {
		analysis := Analysis{Fingerprint: string(rune('a' + index)), Actions: actions}
		node := &graphNode{semantic: "page", analysis: analysis, tried: map[string]bool{}, actionAttempts: map[string]int{}}
		choice := manager.chooseActionLocked(Config{DurationSeconds: 1800, Seed: 1, EnableScroll: true}, node, analysis, "com.example/.MainActivity", &runLoop{})
		if choice.signal != loopNext || choice.action.Text == "Sign in" || seen[choice.action.Description] {
			t.Fatalf("choice=%+v seen=%v", choice.action, seen)
		}
		seen[choice.action.Description] = true
	}
	if len(seen) != 2 {
		t.Fatalf("seen=%v", seen)
	}
}

func TestLadderCooldownWaitsOnTheClock(t *testing.T) {
	manager := New(nil, nil, nil)
	now := time.Now().UTC()
	manager.state.Running = true
	manager.state.StartedAt = &now
	manager.state.Steps = 254
	node := &graphNode{semantic: "rewards", analysis: Analysis{Fingerprint: "fp"}, tried: map[string]bool{}, actionAttempts: map[string]int{}}
	loop := &runLoop{
		exhaustedFingerprint: "fp", exhaustedCaptures: minimumExhaustedCapturesBeforeStop,
		ladderCooldownUntil: time.Now().Add(time.Minute),
	}
	manager.mu.Lock()
	choice := manager.chooseActionLocked(Config{DurationSeconds: 1800, Seed: 1, EnableScroll: true, MaxBacktracks: 20}, node, node.analysis, "live.shorttv.apps/.MainActivity", loop)
	manager.mu.Unlock()
	if choice.signal != loopContinue || manager.state.StopReason != "" {
		t.Fatalf("signal=%v reason=%s", choice.signal, manager.state.StopReason)
	}
	loop.ladderCooldownUntil = time.Now().Add(-time.Second)
	loop.exhaustedCaptures = minimumExhaustedCapturesBeforeStop
	manager.mu.Lock()
	choice = manager.chooseActionLocked(Config{DurationSeconds: 1800, Seed: 1, EnableScroll: true, MaxBacktracks: 20}, node, node.analysis, "live.shorttv.apps/.MainActivity", loop)
	manager.mu.Unlock()
	if choice.signal != loopRestart {
		t.Fatalf("signal=%v", choice.signal)
	}
}

func TestExhaustedPageTapsVisibleBackBeforeRestart(t *testing.T) {
	manager := New(nil, nil, nil)
	now := time.Now().UTC()
	manager.state.Running = true
	manager.state.StartedAt = &now
	manager.blockedEdges = map[string]bool{}
	analysis := Analysis{Fingerprint: "page", Actions: []Action{
		{ID: "task", Type: "tap", ResourceID: "live.shorttv.apps:id/task_card_view"},
		{ID: "back", Type: "tap", ResourceID: "live.shorttv.apps:id/navigation_iv", Text: "Back"},
	}}
	node := &graphNode{semantic: "page", analysis: analysis, tried: map[string]bool{}, actionAttempts: map[string]int{}}
	for _, action := range analysis.Actions {
		manager.blockedEdges[explorationEdgeKey(node.semantic, semanticActionKey(action))] = true
	}
	choice := manager.chooseActionLocked(Config{DurationSeconds: 30, EnableScroll: true, Seed: 1}, node, analysis, "live.shorttv.apps/.MainActivity", &runLoop{exhaustedCaptures: 2, exhaustedFingerprint: "page"})
	if choice.signal != loopNext || choice.action.ResourceID != "live.shorttv.apps:id/navigation_iv" {
		t.Fatalf("choice=%+v", choice)
	}
}

func TestHomeScreenResumeDoesNotForceStop(t *testing.T) {
	device := &scriptedDevice{foreground: "com.miui.home"}
	manager := New(device, device, device)
	manager.state.Running = true
	loop := &runLoop{pending: -1, externalAttempts: map[string]int{"com.miui.home": 3}}
	signal := manager.recoverOffTarget(context.Background(), Config{Package: "live.shorttv.apps", DurationSeconds: 60, IntervalMillis: 100}, loop, "com.miui.home", "com.miui.home/.Launcher", "")
	if signal != loopContinue || len(loop.externalAttempts) != 1 {
		t.Fatalf("signal=%v attempts=%v", signal, loop.externalAttempts)
	}
	device.mu.Lock()
	joined := strings.Join(flattenCommands(device.commands), "\n")
	device.mu.Unlock()
	if strings.Contains(joined, "force-stop") || strings.Contains(joined, "keyevent BACK") || strings.Count(joined, "monkey -p") != 1 {
		t.Fatalf("commands =\n%s", joined)
	}
	again := manager.recoverOffTarget(context.Background(), Config{Package: "live.shorttv.apps", DurationSeconds: 60, IntervalMillis: 100}, loop, "com.miui.home", "com.miui.home/.Launcher", "")
	if again != loopContinue {
		t.Fatalf("signal=%v", again)
	}
	device.mu.Lock()
	joined = strings.Join(flattenCommands(device.commands), "\n")
	device.mu.Unlock()
	if strings.Count(joined, "monkey -p") != 1 {
		t.Fatalf("home resume repeated:\n%s", joined)
	}
}

func flattenCommands(commands [][]string) []string {
	joined := make([]string, 0, len(commands))
	for _, command := range commands {
		joined = append(joined, strings.Join(command, " "))
	}
	return joined
}

func TestExternalRecoveryRelaunchesAfterBoundedAttempts(t *testing.T) {
	device := &scriptedDevice{foreground: "com.other"}
	manager := New(device, device, device)
	manager.state.Running = true
	loop := &runLoop{pending: -1, externalAttempts: map[string]int{"com.other": 3}}
	signal := manager.recoverOffTarget(context.Background(), Config{Package: "com.example", IntervalMillis: 100}, loop, "com.other", "com.other/.Main", "")
	if signal != loopContinue || manager.State().StopReason == "safety_stop" {
		t.Fatalf("signal=%v reason=%s err=%s", signal, manager.State().StopReason, manager.State().Error)
	}
	device.mu.Lock()
	defer device.mu.Unlock()
	joined := strings.Join(flattenCommands(device.commands), "\n")
	if !strings.Contains(joined, "force-stop com.example") || !strings.Contains(joined, "monkey -p com.example") {
		t.Fatalf("commands =\n%s", joined)
	}
}
