package exploration

import (
	"strings"
	"time"
)

const (
	minimumEmptyCapturesBeforeExhaustion = 3
	transitionSettleDuration             = 1500 * time.Millisecond
	initialAsyncSettleDuration           = 8 * time.Second
)

// armStartupHold starts another startup wait after a cold relaunch. The first
// eight seconds belong to the original session and do not cover the splash that
// comes back from force-stop.
func armStartupHold(loop *runLoop, now time.Time) {
	loop.initialSettleUntil = now.Add(initialAsyncSettleDuration)
	loop.startupGeneration = pageGeneration{}
	loop.startupReleased = false
	loop.emptyCaptures = 0
	loop.exhaustedFingerprint = ""
	loop.exhaustedCaptures = 0
	loop.escapeActivity = ""
	loop.escapeAttempts = 0
	loop.restartedActivity = ""
}

func isAsyncStartupActivity(activity string) bool {
	value := strings.ToLower(activity)
	return strings.Contains(value, "splash") || strings.Contains(value, "startup") || strings.Contains(value, "launch") || strings.Contains(value, "loading")
}

// pageGeneration is the activity and the focused window's relayout sequence.
type pageGeneration struct {
	activity      string
	windowVersion string
}

// settleReady reports whether ordinary graph construction may proceed. The
// deadline remains the upper bound. A new activity proceeds immediately. A new
// relayout sequence proceeds only after the same sequence is seen twice, so a
// single in-progress layout does not enter the graph.
func settleReady(now, until time.Time, baseline, previous, current pageGeneration) bool {
	if until.IsZero() || !now.Before(until) {
		return true
	}
	if baseline.activity != "" && current.activity != "" && baseline.activity != current.activity {
		return true
	}
	if baseline.windowVersion == "" || previous.windowVersion == "" || current.windowVersion == "" {
		return false
	}
	if previous.activity != current.activity || previous.windowVersion != current.windowVersion {
		return false
	}
	return baseline.windowVersion != current.windowVersion
}

// observeStartup keeps an empty startup page out of the graph until its
// activity or window relayout sequence changes, or the startup deadline passes.
func observeStartup(now, until time.Time, activity string, baseline *pageGeneration, released *bool, current pageGeneration) bool {
	if released == nil || *released || !now.Before(until) || !isAsyncStartupActivity(activity) {
		return false
	}
	if baseline.activity == "" && baseline.windowVersion == "" {
		*baseline = current
		return true
	}
	if baseline.activity != "" && current.activity != "" && baseline.activity != current.activity {
		*released = true
		return false
	}
	return true
}

// deferEmptyPage is true while an empty hierarchy must not be stored. A released
// settle or startup hold still requires the usual run of empty captures.
func deferEmptyPage(holdStartup bool, now, settleUntil time.Time, baseline, previous, current pageGeneration, emptyCaptures int) bool {
	return holdStartup || !settleReady(now, settleUntil, baseline, previous, current) || emptyCaptures < minimumEmptyCapturesBeforeExhaustion
}
