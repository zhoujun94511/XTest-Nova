package exploration

import (
	"context"
	"testing"
	"time"
)

func TestStuckStateLeavesByBackThenColdStart(t *testing.T) {
	loop := &runLoop{}
	noteSemanticProgress(loop, "same", "com.example/.MainActivity", false)
	if pageIsStuck(loop, "com.example/.MainActivity", "same") {
		t.Fatal("one unchanged action was already stuck")
	}
	noteSemanticProgress(loop, "same", "com.example/.MainActivity", false)
	if !pageIsStuck(loop, "com.example/.MainActivity", "same") {
		t.Fatal("two unchanged actions did not stick")
	}
	noteSemanticProgress(loop, "next", "com.example/.MainActivity", true)
	if pageIsStuck(loop, "com.example/.MainActivity", "same") || loop.stuckReady {
		t.Fatal("a new semantic state stayed stuck")
	}

	manager := New(nil, nil, nil)
	now := time.Now().UTC()
	manager.state.Running = true
	manager.state.StartedAt = &now
	activity := "com.example/.MainActivity"
	covered := &runLoop{stuckReady: true, stuckActivity: activity, stuckSemantic: "page"}
	play := Analysis{Fingerprint: "page", Actions: []Action{{ID: "play", Type: "tap", Text: "Play"}}}
	choice := manager.chooseActionLocked(Config{DurationSeconds: 1800, Seed: 1, EnableScroll: true}, &graphNode{semantic: "page", analysis: play, tried: map[string]bool{}, actionAttempts: map[string]int{}}, play, activity, covered)
	if choice.signal != loopNext || choice.action.ID != "play" {
		t.Fatalf("stuck page skipped an untried control: %+v signal=%v", choice.action, choice.signal)
	}

	stuck := &runLoop{stuckReady: true, stuckActivity: activity, stuckSemantic: "gate"}
	empty := Analysis{Fingerprint: "gate"}
	node := &graphNode{semantic: "gate", analysis: empty, tried: map[string]bool{}, actionAttempts: map[string]int{}}
	choice = manager.chooseActionLocked(Config{DurationSeconds: 1800, Seed: 1}, node, empty, activity, stuck)
	if choice.signal != loopNext || choice.action.Type != "back" {
		t.Fatalf("stuck page did not back out: %+v signal=%v", choice.action, choice.signal)
	}
	choice = manager.chooseActionLocked(Config{DurationSeconds: 1800, Seed: 1}, node, empty, activity, stuck)
	if choice.signal != loopRestart || !stuck.stuckRelaunch {
		t.Fatalf("stuck page did not cold-start: signal=%v relaunch=%v", choice.signal, stuck.stuckRelaunch)
	}
	choice = manager.chooseActionLocked(Config{DurationSeconds: 1800, Seed: 1}, node, empty, activity, stuck)
	if choice.signal != loopContinue || manager.state.StopReason != "" {
		t.Fatalf("stuck cold start ignored its cooldown: signal=%v reason=%s", choice.signal, manager.state.StopReason)
	}
}

func TestStuckOnboardingReturnsToOrdinaryExploration(t *testing.T) {
	manager := New(nil, nil, nil)
	now := time.Now().UTC()
	manager.state.Running = true
	manager.state.StartedAt = &now
	hierarchy := `<hierarchy><node package="com.example" class="android.widget.TextView" text="Skip" bounds="[900,40][1040,140]" clickable="true" enabled="true" visible-to-user="true"/><node package="com.example" class="android.widget.TextView" text="Welcome" bounds="[100,500][900,700]" enabled="true" visible-to-user="true"/><node package="com.example" class="android.widget.Button" text="Continue" bounds="[400,2100][680,2200]" clickable="true" enabled="true" visible-to-user="true"/></hierarchy>`
	policy := SpecialHandling{}
	if err := policy.normalize(); err != nil {
		t.Fatal(err)
	}
	loop := &runLoop{stuckReady: true, stuckActivity: "com.example/.MainActivity", specialAttempts: map[string]int{}}
	signal := manager.handleSpecialScene(context.Background(), Config{Package: "com.example", DurationSeconds: 60, SpecialHandling: policy}, loop, "com.example", "com.example/.MainActivity", "", hierarchy)
	if signal != loopNext || manager.state.StopReason != "" {
		t.Fatalf("signal=%v reason=%s", signal, manager.state.StopReason)
	}
}
