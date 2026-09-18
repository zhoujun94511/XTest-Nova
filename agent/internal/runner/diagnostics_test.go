package runner

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func epochLine(timestamp time.Time, uid, pid int, priority, tag, message string) string {
	_ = uid // retained at call sites to document the emitting Android UID
	return fmt.Sprintf("%.3f %d %d %s %s: %s", float64(timestamp.UnixNano())/float64(time.Second), pid, pid+7, priority, tag, message)
}

func TestAnalyzeDiagnosticsScopesCrashANRAndNativeCrash(t *testing.T) {
	startedAt := time.Unix(1_789_000_000, 0).UTC()
	packageName := "com.example.app"
	lines := []string{
		epochLine(startedAt.Add(-time.Minute), 10001, 111, "E", "AndroidRuntime", "Process: "+packageName+", PID: 111"),
		epochLine(startedAt.Add(time.Second), 10002, 222, "E", "AndroidRuntime", "FATAL EXCEPTION: main"),
		epochLine(startedAt.Add(1100*time.Millisecond), 10002, 222, "E", "AndroidRuntime", "Process: "+packageName+", PID: 222"),
		epochLine(startedAt.Add(1200*time.Millisecond), 1000, 900, "I", "am_crash", "[0,222,"+packageName+",java.lang.IllegalStateException]"),
		epochLine(startedAt.Add(2*time.Second), 1000, 900, "I", "am_anr", "[0,333,"+packageName+",Input dispatching timed out]"),
		epochLine(startedAt.Add(3*time.Second), 10003, 444, "F", "libc", "Fatal signal 11 (SIGSEGV)"),
		epochLine(startedAt.Add(3100*time.Millisecond), 1000, 777, "F", "DEBUG", "pid: 444, tid: 444, name: worker  >>> "+packageName+" <<<"),
		epochLine(startedAt.Add(4*time.Second), 10004, 555, "E", "AndroidRuntime", "FATAL EXCEPTION: unrelated"),
		epochLine(startedAt.Add(4100*time.Millisecond), 10004, 555, "E", "AndroidRuntime", "Process: com.other.app, PID: 555"),
	}
	capture := diagnosticCapture{Logcat: strings.Join(lines, "\n"), LogcatAvailable: true}
	result := analyzeDiagnostics(capture, packageName, startedAt)
	if len(result.Crashes) != 1 || len(result.ANRs) != 1 || len(result.NativeCrashes) != 1 {
		t.Fatalf("diagnostic counts crash=%d anr=%d native=%d", len(result.Crashes), len(result.ANRs), len(result.NativeCrashes))
	}
	if result.Crashes[0].PID != 222 || result.ANRs[0].PID != 333 || result.NativeCrashes[0].PID != 444 {
		t.Fatalf("incident PIDs were not extracted from epoch/event records: crash=%d anr=%d native=%d", result.Crashes[0].PID, result.ANRs[0].PID, result.NativeCrashes[0].PID)
	}
	if strings.Contains(result.ScopedLogcat, "com.other.app") || strings.Contains(result.ScopedLogcat, "PID: 111") {
		t.Fatalf("scoped logcat retained unrelated or pre-session data:\n%s", result.ScopedLogcat)
	}
	if !strings.Contains(result.ScopedLogcat, "IllegalStateException") || !strings.Contains(result.NativeLog, "Fatal signal 11") {
		t.Fatalf("scoped evidence is incomplete:\n%s\n-- native --\n%s", result.ScopedLogcat, result.NativeLog)
	}
}

func TestAnalyzeDiagnosticsDoesNotDoubleClassifyNativeAMCrashAsJava(t *testing.T) {
	startedAt := time.Unix(1_789_000_000, 0).UTC()
	packageName := "com.example.app"
	nativeLine := epochLine(startedAt.Add(time.Second), 1000, 900, "I", "am_crash",
		"[900,0,"+packageName+",1,Native crash,Segmentation fault,unknown,0]")
	capture := diagnosticCapture{
		LogcatAvailable: true,
		Logcat:          nativeLine,
		IncidentLog:     nativeLine,
		ExitInfo: fmt.Sprintf("timestamp=%s pid=444 realUid=10001 packageUid=10001 reason=5 (APP CRASH(NATIVE)) subreason=0 (UNKNOWN) status=11\nprocess=%s",
			startedAt.Add(1100*time.Millisecond).Format("2006-01-02 15:04:05.000"), packageName),
		ExitTimezone: "+0000",
	}
	result := analyzeDiagnostics(capture, packageName, startedAt)
	if len(result.Crashes) != 0 || len(result.NativeCrashes) != 1 || result.NativeCrashes[0].Occurrences != 1 || result.NativeCrashes[0].PID != 444 {
		t.Fatalf("native am_crash was double classified: java=%#v native=%#v", result.Crashes, result.NativeCrashes)
	}
	if strings.Count(result.NativeLog, "am_crash:") != 1 {
		t.Fatalf("native text retained duplicate cross-source lines: %q", result.NativeLog)
	}
}

func TestAnalyzeDiagnosticsRetainsSeparateCrashesFromSameProcess(t *testing.T) {
	startedAt := time.Unix(1_789_000_000, 0).UTC()
	packageName := "com.example.app"
	capture := diagnosticCapture{LogcatAvailable: true, Logcat: strings.Join([]string{
		epochLine(startedAt.Add(time.Second), 1000, 900, "I", "am_crash", "[0,222,"+packageName+",first]"),
		epochLine(startedAt.Add(1200*time.Millisecond), 1000, 900, "I", "am_crash", "[0,222,"+packageName+",second]"),
	}, "\n")}
	result := analyzeDiagnostics(capture, packageName, startedAt)
	if len(result.Crashes) != 2 {
		t.Fatalf("separate same-process crashes were collapsed: %#v", result.Crashes)
	}
}

func TestAnalyzeDiagnosticsReportsUnavailableCollection(t *testing.T) {
	result := analyzeDiagnostics(diagnosticCapture{Errors: []string{"logcat unavailable"}}, "com.example.app", time.Now())
	if result.LogcatAvailable || len(result.Crashes) != 0 || len(result.ANRs) != 0 || len(result.Errors) != 1 {
		t.Fatalf("unexpected unavailable result: %+v", result)
	}
}

func TestAnalyzeDiagnosticsUsesDedicatedIncidentLogWhenGeneralLogIsTruncated(t *testing.T) {
	startedAt := time.Unix(1_789_000_000, 0).UTC()
	packageName := "com.example.app"
	capture := diagnosticCapture{
		LogcatAvailable: true,
		Truncated:       true,
		Logcat:          epochLine(startedAt.Add(time.Second), 10001, 111, "I", "Example", packageName+" ordinary output"),
		IncidentLog: strings.Join([]string{
			epochLine(startedAt.Add(9*time.Minute), 10002, 222, "E", "AndroidRuntime", "Process: "+packageName+", PID: 222"),
			epochLine(startedAt.Add(9*time.Minute+time.Second), 1000, 900, "I", "am_crash", "[0,222,"+packageName+",java.lang.IllegalStateException]"),
			epochLine(startedAt.Add(9*time.Minute+2*time.Second), 1000, 900, "I", "am_anr", "[0,222,"+packageName+",Input dispatching timed out]"),
		}, "\n"),
	}
	result := analyzeDiagnostics(capture, packageName, startedAt)
	if !result.Truncated || len(result.Crashes) != 1 || len(result.ANRs) != 1 {
		t.Fatalf("dedicated incident log was not classified: %+v", result)
	}
}

func TestAnalyzeDiagnosticsDoesNotClassifyStaleLastANR(t *testing.T) {
	startedAt := time.Unix(1_789_000_000, 0).UTC()
	packageName := "com.example.app"
	capture := diagnosticCapture{
		LogcatAvailable: true,
		LastANR: "ACTIVITY MANAGER LAST ANR\n  ANR time: Sep 11, 2026 10:55:53 AM\n" +
			"  ANR in " + packageName + "\n  Reason: Input dispatching timed out\n",
	}
	result := analyzeDiagnostics(capture, packageName, startedAt)
	if len(result.ANRs) != 0 {
		t.Fatalf("stale lastanr contaminated the current session: %+v", result.ANRs)
	}
	if !strings.Contains(result.Diagnostics, "[lastanr]") || !strings.Contains(result.Diagnostics, packageName) {
		t.Fatalf("lastanr supporting evidence was not preserved: %s", result.Diagnostics)
	}
}

type crashDiagnosticSource struct{}

func (crashDiagnosticSource) Collect(_ context.Context, packageName string, startedAt time.Time) diagnosticCapture {
	return diagnosticCapture{LogcatAvailable: true, Logcat: strings.Join([]string{
		epochLine(startedAt.Add(time.Second), 10002, 222, "E", "AndroidRuntime", "FATAL EXCEPTION: main"),
		epochLine(startedAt.Add(1100*time.Millisecond), 10002, 222, "E", "AndroidRuntime", "Process: "+packageName+", PID: 222"),
	}, "\n")}
}
