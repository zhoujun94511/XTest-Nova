package device

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

type fakeExecutor struct {
	values map[string]string
}

func (f fakeExecutor) Run(_ context.Context, name string, args ...string) (string, error) {
	key := strings.Join(append([]string{name}, args...), " ")
	value, ok := f.values[key]
	if !ok {
		return "", fmt.Errorf("unexpected command: %s", key)
	}
	return value, nil
}

func (fakeExecutor) RunBytes(context.Context, string, ...string) ([]byte, error) { return nil, nil }

func TestForegroundPackageUsesWindow(t *testing.T) {
	service := New(fakeExecutor{values: map[string]string{
		"dumpsys window windows": "mCurrentFocus=Window{123 u0 com.example/.MainActivity}",
	}}, time.Second)
	value, err := service.ForegroundPackage(context.Background())
	if err != nil || value != "com.example" {
		t.Fatalf("foreground = %q, %v", value, err)
	}
}

func TestForegroundFocusReadsRelayoutSequence(t *testing.T) {
	service := New(fakeExecutor{values: map[string]string{
		"dumpsys window windows": "mCurrentFocus=Window{a1b2 u0 com.example/.MainActivity}\n" +
			"  Window #4 Window{bbbb u0 com.android.systemui}:\n    mSeq=3\n" +
			"  Window #3 Window{a1b2 u0 com.example/com.example.MainActivity}:\n    mSeq=17\n",
	}}, time.Second)
	component, version, err := service.ForegroundFocus(context.Background())
	if err != nil || component != "com.example/com.example.MainActivity" || version != "a1b2" {
		t.Fatalf("focus = %q %q, %v", component, version, err)
	}
}

func TestForegroundFocusDoesNotDumpEveryWindowService(t *testing.T) {
	service := New(fakeExecutor{values: map[string]string{
		"dumpsys window windows": "mCurrentFocus=Window{a1b2 u0 com.example/.MainActivity}",
	}}, time.Second)
	component, version, err := service.ForegroundFocus(context.Background())
	if err != nil || component != "com.example/com.example.MainActivity" || version != "a1b2" {
		t.Fatalf("focus = %q %q, %v", component, version, err)
	}
}

func TestForegroundFocusReadsSequenceForResumedActivity(t *testing.T) {
	service := New(fakeExecutor{values: map[string]string{
		"dumpsys window windows": "mCurrentFocus=null\n" +
			"  Window #2 Window{abc u0 com.example.target/com.example.Main}:\n    mSeq=9\n",
		"dumpsys activity activities": "topResumedActivity=ActivityRecord{231359804 u0 com.example.target/com.example.Main t3657}",
	}}, time.Second)
	component, version, err := service.ForegroundFocus(context.Background())
	if err != nil || component != "com.example.target/com.example.Main" || version != "abc" {
		t.Fatalf("focus = %q %q, %v", component, version, err)
	}
}

func TestForegroundFocusPrefersResumedActivity(t *testing.T) {
	service := New(fakeExecutor{values: map[string]string{
		"dumpsys window windows": "mResumedActivity: ActivityRecord{11 u0 com.example/.MainActivity t1}\n" +
			"mCurrentFocus=Window{ime u0 com.android.inputmethod/.Keyboard}\n" +
			"  Window #1 Window{ime u0 com.android.inputmethod/com.android.inputmethod.Keyboard}:\n    mSeq=3\n" +
			"  Window #2 Window{app u0 com.example/com.example.MainActivity}:\n    mSeq=17\n",
	}}, time.Second)
	component, version, err := service.ForegroundFocus(context.Background())
	if err != nil || component != "com.example/com.example.MainActivity" || version != "app" {
		t.Fatalf("focus = %q %q, %v", component, version, err)
	}
}

func TestForegroundFocusLeavesVersionEmptyWhenWindowTokenIsMissing(t *testing.T) {
	service := New(fakeExecutor{values: map[string]string{
		"dumpsys window windows":      "mCurrentFocus=null\nmFocusedApp=null",
		"dumpsys activity activities": "topResumedActivity=ActivityRecord{231359804 u0 com.example.target/com.example.Main t3657}",
	}}, time.Second)
	component, version, err := service.ForegroundFocus(context.Background())
	if err != nil || component != "com.example.target/com.example.Main" || version != "" {
		t.Fatalf("focus = %q %q, %v", component, version, err)
	}
}

func TestForegroundActivityCanonicalizesRelativeClass(t *testing.T) {
	service := New(fakeExecutor{values: map[string]string{
		"dumpsys window windows": "mCurrentFocus=Window{123 u0 com.example/.MainActivity}",
	}}, time.Second)
	value, err := service.ForegroundActivity(context.Background())
	if err != nil || value != "com.example/com.example.MainActivity" {
		t.Fatalf("foreground activity = %q, %v", value, err)
	}
}

func TestForegroundPackageFallsBackToActivityOnAndroid16(t *testing.T) {
	service := New(fakeExecutor{values: map[string]string{
		"dumpsys window windows":      "mCurrentFocus=null\nmFocusedApp=null",
		"dumpsys activity activities": "topResumedActivity=ActivityRecord{231359804 u0 com.example.target/com.example.Main t3657}",
	}}, time.Second)
	value, err := service.ForegroundPackage(context.Background())
	if err != nil || value != "com.example.target" {
		t.Fatalf("foreground = %q, %v", value, err)
	}
}

func TestForegroundPackageSupportsXiaomiAndroid16ActivityFormat(t *testing.T) {
	service := New(fakeExecutor{values: map[string]string{
		"dumpsys window windows":      "mCurrentFocus=null\nmFocusedApp=null",
		"dumpsys activity activities": "  ResumedActivity: ActivityRecord{86603467 u0 com.mi.android.globallauncher/com.miui.home.launcher.Launcher t2}",
	}}, time.Second)
	value, err := service.ForegroundPackage(context.Background())
	if err != nil || value != "com.mi.android.globallauncher" {
		t.Fatalf("foreground = %q, %v", value, err)
	}
}

func TestResourcesToleratesEmptyStorageOutput(t *testing.T) {
	service := New(fakeExecutor{values: map[string]string{
		"cat /proc/meminfo": "MemTotal: 1024 kB",
		"cat /proc/cpuinfo": "processor : 0",
		"df -k /data":       "",
	}}, time.Second)
	_, _, storage := service.resources(context.Background())
	if storage != nil {
		t.Fatalf("storage = %#v, want nil", storage)
	}
}

func TestDisplayPrefersOverrideSize(t *testing.T) {
	service := New(fakeExecutor{values: map[string]string{
		"wm size": "Physical size: 1440x3200\nOverride size: 1080x2400",
	}}, time.Second)
	display := service.display(context.Background())
	if display == nil || display.Width != 1080 || display.Height != 2400 {
		t.Fatalf("display = %#v", display)
	}
}

func TestDensityPrefersOverrideValue(t *testing.T) {
	service := New(fakeExecutor{values: map[string]string{
		"wm density": "Physical density: 450\nOverride density: 420",
	}}, time.Second)
	density := service.density(context.Background())
	if density == nil || density.Density != 420 {
		t.Fatalf("density = %#v", density)
	}
}
