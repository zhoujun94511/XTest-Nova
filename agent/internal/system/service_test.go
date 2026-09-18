package system

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

type fixedExecutor struct{ outputs []string }

func (f *fixedExecutor) Run(context.Context, string, ...string) (string, error) {
	if len(f.outputs) == 0 {
		return "", errors.New("no output")
	}
	value := f.outputs[0]
	f.outputs = f.outputs[1:]
	return value, nil
}
func (*fixedExecutor) RunBytes(context.Context, string, ...string) ([]byte, error) { return nil, nil }

func TestParseAppMemory(t *testing.T) {
	value, err := ParseAppMemory("header\nApp Summary\n Java Heap: 123\n Native Heap: 45\n TOTAL: 168")
	if err != nil {
		t.Fatal(err)
	}
	if value["java heap"] != 123 || value["total"] != 168 {
		t.Fatalf("%v", value)
	}
}
func TestParseStatus(t *testing.T) {
	value := parseStatus("Name:\ttest\nPPid:\t42\nThreads:\t7\n")
	if value["PPid"] != "42" || value["Threads"] != "7" {
		t.Fatalf("%v", value)
	}
}

func TestCPUInfoBetweenDistinguishesWarmupIdleAndMeasured(t *testing.T) {
	warmup := cpuInfoBetween(packageCPUSnapshot{}, packageCPUSnapshot{pids: []int{42}, total: 100, idle: 70, user: 5, system: 2})
	if warmup.State != "warming_up" {
		t.Fatalf("warmup=%#v", warmup)
	}
	idle := cpuInfoBetween(packageCPUSnapshot{pids: []int{42}, total: 100, idle: 70, user: 5, system: 2}, packageCPUSnapshot{pids: []int{42}, total: 200, idle: 160, user: 5, system: 2})
	if idle.State != "idle" || idle.Percent != 0 || idle.SystemPercent != 10 {
		t.Fatalf("idle=%#v", idle)
	}
	measured := cpuInfoBetween(packageCPUSnapshot{pids: []int{42}, total: 100, idle: 70, user: 5, system: 2}, packageCPUSnapshot{pids: []int{42}, total: 200, idle: 150, user: 8, system: 4})
	if measured.State != "measured" || measured.Percent != 5 || measured.SystemPercent != 20 {
		t.Fatalf("measured=%#v", measured)
	}
}

func TestFrameRateUsesRenderedFrameDelta(t *testing.T) {
	executor := &fixedExecutor{outputs: []string{"Total frames rendered: 100", "", "", "Total frames rendered: 220", "SurfaceFlinger unavailable"}}
	service := New(executor, time.Second)
	times := []time.Time{time.Unix(10, 0), time.Unix(12, 0)}
	service.now = func() time.Time { value := times[0]; times = times[1:]; return value }
	first, err := service.FrameRate(context.Background(), "com.example.app")
	if err != nil || first != nil {
		t.Fatalf("first=%v err=%v", first, err)
	}
	second, err := service.FrameRate(context.Background(), "com.example.app")
	if err != nil || second == nil || *second != 60 {
		t.Fatalf("second=%v err=%v", second, err)
	}
}

func TestFrameRateSessionsDoNotShareDeltaBaselines(t *testing.T) {
	executor := &fixedExecutor{outputs: []string{"Total frames rendered: 100", "", "", "Total frames rendered: 500", "SurfaceFlinger unavailable", "Total frames rendered: 160", "SurfaceFlinger unavailable"}}
	service := New(executor, time.Second)
	times := []time.Time{time.Unix(10, 0), time.Unix(10, 0), time.Unix(11, 0)}
	service.now = func() time.Time { value := times[0]; times = times[1:]; return value }
	first, second := newFrameRateState(), newFrameRateState()
	if value, _, _, err := service.frameRate(context.Background(), "com.example.app", first); err != nil || value != nil {
		t.Fatalf("first baseline value=%v err=%v", value, err)
	}
	if value, _, _, err := service.frameRate(context.Background(), "com.example.app", second); err != nil || value != nil {
		t.Fatalf("second baseline value=%v err=%v", value, err)
	}
	value, _, source, err := service.frameRate(context.Background(), "com.example.app", first)
	if err != nil || value == nil || *value != 60 || source != "gfxinfo-render-rate" {
		t.Fatalf("isolated value=%v source=%q err=%v", value, source, err)
	}
}

func TestPackageUIDAndNetstatsParsing(t *testing.T) {
	uid, err := ParsePackageUID("package:com.other uid:10001\npackage:com.example.app uid:10473\n", "com.example.app")
	if err != nil || uid != 10473 {
		t.Fatalf("uid=%d err=%v", uid, err)
	}
	output := `UID stats:
  ident=[{type=1}] uid=10473 set=DEFAULT tag=0x0
    NetworkStatsHistory: bucketDuration=7200
      st=1 rb=100 rp=1 tb=40 tp=1 op=0
      st=2 rb=50 rp=1 tb=10 tp=1 op=0
  ident=[{type=1}] uid=10473 set=DEFAULT tag=0xf00
      st=2 rb=999 rp=1 tb=999 tp=1 op=0
  ident=[{type=1}] uid=1000 set=DEFAULT tag=0x0
      st=2 rb=777 rp=1 tb=777 tp=1 op=0`
	value, err := ParseUIDNetstats(output, uid)
	if err != nil || value.Rx != 150 || value.Tx != 50 || value.Total != 200 || value.Scope != "package_uid" || value.Source != "dumpsys-netstats" {
		t.Fatalf("network=%#v err=%v", value, err)
	}
}

func TestParsePackagePIDsIncludesOnlyPackageProcesses(t *testing.T) {
	output := "PID NAME\n101 com.example.app\n102 com.example.app:worker\n103 com.example.application\n104 other.app\n"
	if got := ParsePackagePIDs(output, "com.example.app"); !reflect.DeepEqual(got, []int{101, 102}) {
		t.Fatalf("ParsePackagePIDs() = %v", got)
	}
}

func TestParsePerformanceProbes(t *testing.T) {
	frames, err := ParseRenderedFrames("Total frames rendered: 425\nJanky frames: 21")
	if err != nil || frames != 425 {
		t.Fatalf("frames=%d err=%v", frames, err)
	}
	if value, err := parsePercentValue(" 9 %\n"); err != nil || value != 9 {
		t.Fatalf("gpu=%v err=%v", value, err)
	}
	if janky, err := ParseJankyFrames("Janky frames: 21 (4.94%)\n"); err != nil || janky != 21 {
		t.Fatalf("janky=%d err=%v", janky, err)
	}
	battery := ParseBattery("status: 2\nlevel: 83\ntemperature: 306\n")
	if battery.Status != "charging" || battery.Level != 83 || battery.Temperature != 30.6 {
		t.Fatalf("battery=%#v", battery)
	}
	if normalizeCurrentMA(-528000) != -528 {
		t.Fatal("microamp current was not normalized")
	}
}

func TestFrameRateReturnsSessionJankDelta(t *testing.T) {
	executor := &fixedExecutor{outputs: []string{
		"Total frames rendered: 100\nJanky frames: 10 (10%)", "", "",
		"Total frames rendered: 160\nJanky frames: 13 (8%)", "SurfaceFlinger unavailable",
	}}
	service := New(executor, time.Second)
	times := []time.Time{time.Unix(10, 0), time.Unix(11, 0)}
	service.now = func() time.Time { value := times[0]; times = times[1:]; return value }
	if _, frame, _, err := service.frameRate(context.Background(), "com.example.app", service.defaultFrames); err != nil || frame != nil {
		t.Fatalf("warmup frame=%#v err=%v", frame, err)
	}
	value, frame, source, err := service.frameRate(context.Background(), "com.example.app", service.defaultFrames)
	if err != nil || value == nil || *value != 60 || frame == nil || frame.JankCount != 3 || frame.JankRate != 5 || source != "gfxinfo-render-rate" {
		t.Fatalf("value=%v frame=%#v source=%q err=%v", value, frame, source, err)
	}
}

func TestFrameRateMarksStationaryJankAsIdle(t *testing.T) {
	executor := &fixedExecutor{outputs: []string{
		"Total frames rendered: 100\nJanky frames: 10 (10%)", "", "",
		"Total frames rendered: 100\nJanky frames: 10 (10%)", "SurfaceFlinger unavailable",
	}}
	service := New(executor, time.Second)
	times := []time.Time{time.Unix(10, 0), time.Unix(11, 0)}
	service.now = func() time.Time { value := times[0]; times = times[1:]; return value }
	if _, _, _, err := service.frameRate(context.Background(), "com.example.app", service.defaultFrames); err != nil {
		t.Fatal(err)
	}
	value, frame, _, err := service.frameRate(context.Background(), "com.example.app", service.defaultFrames)
	if err != nil || value == nil || *value != 0 || frame == nil || frame.State != "idle" || frame.RenderedFrames != 0 {
		t.Fatalf("value=%v frame=%#v err=%v", value, frame, err)
	}
}

func TestParseSurfaceFramesSelectsNewestTargetLayer(t *testing.T) {
	output := `layerName = SurfaceView[com.example.app/Main]#10
totalFrames = 800
layerName = other.package/Main#11
totalFrames = 900
layerName = com.example.app/Main$_42#12
totalFrames = 120
`
	frames, err := ParseSurfaceFrames(output, "com.example.app")
	if err != nil || frames != 800 {
		t.Fatalf("frames=%d err=%v", frames, err)
	}
}

func TestParseSurfaceFramesPrefersSurfaceViewOverGenericLayer(t *testing.T) {
	output := `layerName = com.example.app/Main$_42#12
totalFrames = 900
layerName = SurfaceView[com.example.app/Main]#13
totalFrames = 120
`
	frames, err := ParseSurfaceFrames(output, "com.example.app")
	if err != nil || frames != 120 {
		t.Fatalf("frames=%d err=%v", frames, err)
	}
}

func TestParseSurfaceFramesDoesNotPinToDestroyedLayer(t *testing.T) {
	output := `layerName = SurfaceView[com.example.app/Main]#1045
totalFrames = 1788
layerName = SurfaceView[com.example.app/Main]#1072
totalFrames = 187
`
	frames, err := ParseSurfaceFrames(output, "com.example.app")
	if err != nil || frames != 187 {
		t.Fatalf("frames=%d err=%v", frames, err)
	}
}

func TestFrameRateFallsBackToSurfaceFlinger(t *testing.T) {
	executor := &fixedExecutor{outputs: []string{
		"Total frames rendered: 10", "", "",
		"Total frames rendered: 10", "layerName = SurfaceView[com.example.app/Main]#1\ntotalFrames = 120\n",
		"Total frames rendered: 10", "layerName = SurfaceView[com.example.app/Main]#1\ntotalFrames = 240\n",
	}}
	service := New(executor, time.Second)
	times := []time.Time{time.Unix(10, 0), time.Unix(11, 0), time.Unix(11, 0), time.Unix(12, 0), time.Unix(12, 0)}
	service.now = func() time.Time { value := times[0]; times = times[1:]; return value }
	for index := 0; index < 2; index++ {
		value, err := service.FrameRate(context.Background(), "com.example.app")
		if err != nil || value != nil {
			t.Fatalf("sample %d value=%v err=%v", index, value, err)
		}
	}
	value, err := service.FrameRate(context.Background(), "com.example.app")
	if err != nil || value == nil || *value != 120 {
		t.Fatalf("surface value=%v err=%v", value, err)
	}
}

func TestFrameRatePrefersPresentedFramesOverAggregatedRenderRate(t *testing.T) {
	executor := &fixedExecutor{outputs: []string{
		"Total frames rendered: 100", "", "",
		"Total frames rendered: 256", "layerName = SurfaceView[com.example.app/Main]#1\ntotalFrames = 100\n",
		"Total frames rendered: 412", "layerName = SurfaceView[com.example.app/Main]#1\ntotalFrames = 220\n",
	}}
	service := New(executor, time.Second)
	times := []time.Time{time.Unix(10, 0), time.Unix(11, 0), time.Unix(11, 0), time.Unix(12, 0), time.Unix(12, 0)}
	service.now = func() time.Time { value := times[0]; times = times[1:]; return value }
	for index := 0; index < 2; index++ {
		if value, _, _, err := service.frameRate(context.Background(), "com.example.app", service.defaultFrames); err != nil || value != nil {
			t.Fatalf("warmup sample %d value=%v err=%v", index, value, err)
		}
	}
	value, _, source, err := service.frameRate(context.Background(), "com.example.app", service.defaultFrames)
	if err != nil || value == nil || *value != 120 || source != "surfaceflinger-timestats" {
		t.Fatalf("presented value=%v source=%q err=%v", value, source, err)
	}
}
