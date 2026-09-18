package runner

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestScopedLogWriterFiltersBeforeBoundingAndLearnsTargetPID(t *testing.T) {
	writer := newScopedLogWriter("com.example.app", []int{222})
	lines := []string{
		"1789119668.469 111 112 I Other: unrelated",
		`1789119668.470 222 223 I App: {"device_id":"device-secret","message":"kept"}`,
		"1789119668.471 900 901 I ActivityManager: Start proc 333:com.example.app/u0a1 for activity",
		"1789119668.472 333 334 I App: access_token=token-secret $device_id=device-secret-2 Bearer abc.def",
	}
	if _, err := writer.Write([]byte(strings.Join(lines, "\n") + "\n")); err != nil {
		t.Fatal(err)
	}
	value, truncated := writer.snapshot()
	if truncated || strings.Contains(value, "unrelated") || strings.Contains(value, "device-secret") || !strings.Contains(value, `"device_id":"[REDACTED]"`) || !strings.Contains(value, "access_token=[REDACTED]") || !strings.Contains(value, "$device_id=[REDACTED]") || !strings.Contains(value, "Bearer [REDACTED]") {
		t.Fatalf("unexpected scoped log:\n%s", value)
	}
}

func TestScopedLogWriterExpiresPIDOnProcessDeath(t *testing.T) {
	writer := newScopedLogWriter("com.example.app", []int{222})
	lines := []string{
		"1789119668.470 100 101 I ActivityManager: Process com.example.app (pid 222) has died: fg TOP",
		"1789119668.471 222 223 I ReusedByOtherApp: must-not-be-retained",
	}
	_, _ = writer.Write([]byte(strings.Join(lines, "\n") + "\n"))
	value, _ := writer.snapshot()
	if !strings.Contains(value, "has died") || strings.Contains(value, "must-not-be-retained") {
		t.Fatalf("dead PID remained in scope: %s", value)
	}
}

func TestBoundedLineLogPreservesHeadAndLatestTail(t *testing.T) {
	var log boundedLineLog
	for index := 0; index < 2200; index++ {
		log.add(fmt.Sprintf("line-%04d-%s", index, strings.Repeat("x", 4080)))
	}
	value, truncated := log.value()
	if !truncated || !strings.Contains(value, "line-0000-") || !strings.Contains(value, "line-2199-") || !strings.Contains(value, "earlier scoped log lines omitted") {
		t.Fatalf("bounded head/tail retention failed: truncated=%t bytes=%d", truncated, len(value))
	}
	if len(value) > diagnosticLogHeadBytes+diagnosticLogTailBytes+8192 {
		t.Fatalf("bounded log exceeded its envelope: %d", len(value))
	}
}

func TestDiagnosticCompletenessIsIndependentFromGeneralLogTruncation(t *testing.T) {
	capture := diagnosticCapture{
		LogcatAvailable: true,
		Truncated:       true,
		Sources: map[string]DiagnosticSourceState{
			"general_log":  {Available: true, Truncated: true},
			"crash_buffer": {Available: true},
			"event_buffer": {Available: true},
			"exit_info":    {Available: true},
		},
	}
	result := analyzeDiagnostics(capture, "com.example.app", time.Now().Add(-time.Minute))
	if !result.Truncated || !result.GeneralTruncated || result.IncidentTruncated || !result.Complete {
		t.Fatalf("source completeness was conflated: %+v", result)
	}
}

func TestParseExitInfoScopesSessionAndClassifiesReasons(t *testing.T) {
	zone := time.FixedZone("test", 8*60*60)
	startedAt := time.Now().UTC().Add(-time.Minute).Truncate(time.Millisecond)
	crashAt := startedAt.Add(time.Second).In(zone)
	stopAt := startedAt.Add(2 * time.Second).In(zone)
	staleAt := startedAt.Add(-time.Hour).In(zone)
	value := strings.Join([]string{
		"ApplicationExitInfo #0:",
		fmt.Sprintf("timestamp=%s pid=111 realUid=10001 packageUid=10001 definingUid=10001 user=0", crashAt.Format("2006-01-02 15:04:05.000")),
		"process=com.example.app reason=4 (APP CRASH(EXCEPTION)) subreason=0 (UNKNOWN) status=0",
		"importance=100 description=uncaught access_token=secret state=empty",
		"ApplicationExitInfo #1:",
		fmt.Sprintf("timestamp=%s pid=222 realUid=10001 packageUid=10001 reason=10 (USER REQUESTED) subreason=21 (FORCE STOP) status=0", stopAt.Format("2006-01-02 15:04:05.000")),
		"process=com.example.app",
		"ApplicationExitInfo #2:",
		fmt.Sprintf("timestamp=%s pid=333 realUid=10001 packageUid=10001 reason=6 (ANR) subreason=0 (UNKNOWN) status=0", staleAt.Format("2006-01-02 15:04:05.000")),
		"process=com.example.app",
	}, "\n")
	records := parseExitInfo(value, "com.example.app", startedAt, "+0800")
	if len(records) != 2 || !records[0].Abnormal || records[0].ReasonCode != 4 || records[1].Abnormal || records[1].ReasonCode != 10 {
		t.Fatalf("unexpected exit records: %#v", records)
	}
	if strings.Contains(records[0].Description, "secret") || !strings.Contains(records[0].Description, "[REDACTED]") {
		t.Fatalf("exit description was not redacted: %s", records[0].Description)
	}
}

func TestDiagnosticEventPIDSupportsAndroidFieldLayouts(t *testing.T) {
	for _, test := range []struct {
		line string
		want int
	}{
		{"am_crash: [15527,0,com.example.app,1,java.lang.IllegalStateException]", 15527},
		{"am_crash: [0,222,com.example.app,1,java.lang.IllegalStateException]", 222},
		{"am_anr: [0,333,com.example.app,1,Input dispatching timed out]", 333},
	} {
		if got := diagnosticEventPID(test.line); got != test.want {
			t.Fatalf("diagnosticEventPID(%q)=%d want %d", test.line, got, test.want)
		}
	}
}

func TestExitInfoParseFailureMakesDiagnosticsIncomplete(t *testing.T) {
	startedAt := time.Now().UTC().Add(-time.Minute)
	capture := diagnosticCapture{
		ExitInfo: "ApplicationExitInfo #0:\ntimestamp=malformed pid=123\nprocess=com.example.app reason=6 (ANR)",
		Sources: map[string]DiagnosticSourceState{
			"crash_buffer": {Available: true}, "event_buffer": {Available: true}, "exit_info": {Available: true},
		},
	}
	result := analyzeDiagnostics(capture, "com.example.app", startedAt)
	if result.Complete || result.Sources["exit_info"].Error == "" || len(result.Errors) == 0 {
		t.Fatalf("malformed exit info was treated as complete: %+v", result)
	}
}

func TestExitInfoIncidentsAugmentCrashANRAndAbnormalExitCounts(t *testing.T) {
	startedAt := time.Now().UTC().Add(-time.Minute).Truncate(time.Millisecond)
	stamp := startedAt.Add(time.Second).Format("2006-01-02 15:04:05.000")
	capture := diagnosticCapture{
		ExitInfo:     fmt.Sprintf("timestamp=%s pid=123 realUid=10001 packageUid=10001 reason=6 (ANR) subreason=0 (UNKNOWN) status=0\nprocess=com.example.app", stamp),
		ExitTimezone: "+0000",
		Sources: map[string]DiagnosticSourceState{
			"general_log":  {Available: true},
			"crash_buffer": {Available: true},
			"event_buffer": {Available: true},
			"exit_info":    {Available: true},
		},
		LogcatAvailable: true,
	}
	result := analyzeDiagnostics(capture, "com.example.app", startedAt)
	if result.AbnormalExits != 1 || len(result.ANRs) != 1 || result.ANRs[0].PID != 123 || !result.Complete {
		t.Fatalf("exit-info incident was not classified: %+v", result)
	}
}

func TestNativeExitInfoProvidesTextArtifactFallback(t *testing.T) {
	startedAt := time.Now().UTC().Add(-time.Minute).Truncate(time.Millisecond)
	stamp := startedAt.Add(time.Second).Format("2006-01-02 15:04:05.000")
	capture := diagnosticCapture{
		ExitInfo:     fmt.Sprintf("timestamp=%s pid=456 realUid=10001 packageUid=10001 reason=5 (APP CRASH(NATIVE)) subreason=0 (UNKNOWN) status=11\nprocess=com.example.app", stamp),
		ExitTimezone: "+0000",
		Sources: map[string]DiagnosticSourceState{
			"general_log": {Available: true}, "crash_buffer": {Available: true},
			"event_buffer": {Available: true}, "exit_info": {Available: true},
		},
		LogcatAvailable: true,
	}
	result := analyzeDiagnostics(capture, "com.example.app", startedAt)
	if len(result.NativeCrashes) != 1 || result.NativeCrashes[0].PID != 456 {
		t.Fatalf("native exit-info incident was not classified: %+v", result)
	}
	if !strings.Contains(result.NativeLog, "[structured-native-incidents]") ||
		!strings.Contains(result.NativeLog, "APP CRASH(NATIVE)") ||
		!strings.Contains(result.NativeLog, "fingerprint=") {
		t.Fatalf("native text fallback was incomplete: %q", result.NativeLog)
	}
}

func TestAggregateIncidentsKeepsFirstDetailAndCountsRepeatedSignature(t *testing.T) {
	started := time.Now().UTC().Add(-time.Minute)
	first := "1789119668.470 222 223 E ActivityManager: am_crash: [0,222,com.example.app,1,java.lang.IllegalStateException,boom]"
	second := "1789119678.470 333 334 E ActivityManager: am_crash: [0,333,com.example.app,1,java.lang.IllegalStateException,boom]"
	var values []diagnosticIncident
	values = appendIncident(values, "java_crash", started, 222, first, "event_buffer")
	values = appendIncident(values, "java_crash", started.Add(300*time.Millisecond), 222, "exit-info reason=CRASH process=com.example.app", "exit_info")
	values = appendIncident(values, "java_crash", started.Add(10*time.Second), 333, second, "event_buffer")
	aggregated := aggregateIncidents(values)
	if len(aggregated) != 1 || aggregated[0].Occurrences != 2 || aggregated[0].Summary != first || aggregated[0].RepeatSummary == "" {
		t.Fatalf("unexpected aggregation: %#v", aggregated)
	}
	if totalIncidentOccurrences(aggregated) != 2 || len(aggregated[0].Sources) != 2 {
		t.Fatalf("unexpected occurrence/source counts: %#v", aggregated[0])
	}
}

func TestAggregateExitRecordsCountsRepeatedNonCrashExit(t *testing.T) {
	records := []processExitRecord{
		{Timestamp: "2026-09-11T10:00:00Z", Process: "com.example.app", ReasonCode: 3, Reason: "LOW MEMORY", Description: "pressure score 12000", Abnormal: true},
		{Timestamp: "2026-09-11T10:01:00Z", Process: "com.example.app", ReasonCode: 3, Reason: "LOW MEMORY", Description: "pressure score 13000", Abnormal: true},
	}
	aggregated := aggregateExitRecords(records)
	if len(aggregated) != 1 || aggregated[0].Occurrences != 2 || aggregated[0].RepeatSummary == "" {
		t.Fatalf("unexpected process-exit aggregation: %#v", aggregated)
	}
}
