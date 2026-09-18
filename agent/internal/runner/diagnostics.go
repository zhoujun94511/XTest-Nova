package runner

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const maxDiagnosticArtifactBytes = 1 << 20

type diagnosticSource interface {
	Collect(context.Context, string, time.Time) diagnosticCapture
}

type diagnosticCapture struct {
	Logcat          string
	IncidentLog     string
	LastANR         string
	Processes       string
	ExitInfo        string
	ExitTimezone    string
	LogcatAvailable bool
	Truncated       bool
	Errors          []string
	Sources         map[string]DiagnosticSourceState
}

type DiagnosticSourceState struct {
	Available bool   `json:"available"`
	Truncated bool   `json:"truncated,omitempty"`
	Error     string `json:"error,omitempty"`
}

type osDiagnosticSource struct{}

func (source osDiagnosticSource) Collect(ctx context.Context, packageName string, startedAt time.Time) diagnosticCapture {
	return source.collect(ctx, packageName, startedAt, true)
}

func (source osDiagnosticSource) CollectIncidents(ctx context.Context, packageName string, startedAt time.Time) diagnosticCapture {
	return source.collect(ctx, packageName, startedAt, false)
}

type diagnosticSourceResult struct {
	name      string
	value     string
	truncated bool
	err       error
}

func (osDiagnosticSource) collect(ctx context.Context, packageName string, startedAt time.Time, includeGeneral bool) diagnosticCapture {
	capture := diagnosticCapture{Sources: map[string]DiagnosticSourceState{}}
	since := strconv.FormatFloat(float64(startedAt.UnixNano())/float64(time.Second), 'f', 3, 64)
	type sourceJob struct {
		name string
		run  func(context.Context) (string, bool, error)
	}
	jobs := []sourceJob{
		{"crash_buffer", func(commandContext context.Context) (string, bool, error) {
			return boundedCommandLimit(commandContext, 2<<20, "logcat", "-b", "crash", "-d", "-v", "epoch", "-t", since)
		}},
		{"event_buffer", func(commandContext context.Context) (string, bool, error) {
			return boundedCommandLimit(commandContext, 1<<20, "logcat", "-b", "events", "-d", "-v", "epoch", "-t", since, "am_crash:I", "am_anr:I", "*:S")
		}},
		{"last_anr", func(commandContext context.Context) (string, bool, error) {
			return boundedCommandLimit(commandContext, 2<<20, "dumpsys", "activity", "lastanr")
		}},
		{"processes", func(commandContext context.Context) (string, bool, error) {
			return boundedCommandLimit(commandContext, 2<<20, "dumpsys", "activity", "processes")
		}},
		{"exit_info", func(commandContext context.Context) (string, bool, error) {
			return boundedCommandLimit(commandContext, 2<<20, "dumpsys", "activity", "exit-info", packageName)
		}},
		{"device_timezone", func(commandContext context.Context) (string, bool, error) {
			return boundedCommandLimit(commandContext, 64<<10, "date", "+%z")
		}},
	}
	if includeGeneral {
		jobs = append(jobs, sourceJob{"general_log", func(commandContext context.Context) (string, bool, error) {
			return scopedLogcatSnapshot(commandContext, packageName, since)
		}})
	}
	results := make(chan diagnosticSourceResult, len(jobs))
	for _, job := range jobs {
		job := job
		go func() {
			commandContext, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			value, truncated, err := job.run(commandContext)
			results <- diagnosticSourceResult{name: job.name, value: value, truncated: truncated, err: err}
		}()
	}
	var crashLog, eventLog string
	for range jobs {
		result := <-results
		status := DiagnosticSourceState{Available: result.err == nil, Truncated: result.truncated}
		if result.err != nil {
			status.Error = result.err.Error()
			capture.Errors = append(capture.Errors, result.name+": "+result.err.Error())
		}
		capture.Sources[result.name] = status
		capture.Truncated = capture.Truncated || result.truncated
		switch result.name {
		case "general_log":
			capture.Logcat, capture.LogcatAvailable = result.value, result.err == nil
		case "crash_buffer":
			crashLog = result.value
		case "event_buffer":
			eventLog = result.value
		case "last_anr":
			capture.LastANR = result.value
		case "processes":
			capture.Processes = filterPackageLines(result.value, packageName)
		case "exit_info":
			capture.ExitInfo = result.value
		case "device_timezone":
			capture.ExitTimezone = strings.TrimSpace(result.value)
		}
	}
	capture.IncidentLog = strings.Join([]string{crashLog, eventLog}, "\n")
	return capture
}

type cappedBuffer struct {
	buffer    bytes.Buffer
	limit     int
	truncated bool
}

func (b *cappedBuffer) Write(value []byte) (int, error) {
	remaining := b.limit - b.buffer.Len()
	if remaining > 0 {
		if len(value) > remaining {
			_, _ = b.buffer.Write(value[:remaining])
			b.truncated = true
		} else {
			_, _ = b.buffer.Write(value)
		}
	} else if len(value) > 0 {
		b.truncated = true
	}
	return len(value), nil
}

func boundedCommandLimit(ctx context.Context, limit int, name string, args ...string) (string, bool, error) {
	output := &cappedBuffer{limit: limit}
	command := exec.CommandContext(ctx, name, args...)
	command.Stdout, command.Stderr = output, output
	err := command.Run()
	return output.buffer.String(), output.truncated, err
}

func scopedLogcatSnapshot(ctx context.Context, packageName, since string) (string, bool, error) {
	writer := newScopedLogWriter(packageName, currentPackagePIDs(ctx, packageName))
	errorOutput := &cappedBuffer{limit: 64 << 10}
	command := exec.CommandContext(ctx, "logcat", "-b", "all", "-d", "-v", "epoch", "-t", since)
	command.Stdout, command.Stderr = writer, errorOutput
	err := command.Run()
	value, truncated := writer.snapshot()
	if err != nil && errorOutput.buffer.Len() > 0 {
		err = fmt.Errorf("%w: %s", err, strings.TrimSpace(errorOutput.buffer.String()))
	}
	return value, truncated, err
}

type diagnosticIncident struct {
	Kind          string   `json:"kind"`
	Time          string   `json:"time,omitempty"`
	PID           int      `json:"pid,omitempty"`
	Summary       string   `json:"summary"`
	Fingerprint   string   `json:"fingerprint"`
	Occurrences   int      `json:"occurrences"`
	FirstSeenAt   string   `json:"firstSeenAt,omitempty"`
	LastSeenAt    string   `json:"lastSeenAt,omitempty"`
	RepeatSummary string   `json:"repeatSummary,omitempty"`
	Sources       []string `json:"sources,omitempty"`
	source        string
	parsedTime    time.Time
}

type diagnosticReport struct {
	SchemaVersion string                           `json:"schemaVersion"`
	Package       string                           `json:"package"`
	CapturedAt    string                           `json:"capturedAt"`
	Available     bool                             `json:"available"`
	Complete      bool                             `json:"complete"`
	Truncated     bool                             `json:"truncated,omitempty"`
	Redacted      bool                             `json:"redacted"`
	Sources       map[string]DiagnosticSourceState `json:"sources,omitempty"`
	Incidents     []diagnosticIncident             `json:"incidents"`
	Errors        []string                         `json:"collectionErrors,omitempty"`
}

type diagnosticResult struct {
	LogcatAvailable   bool
	Truncated         bool
	GeneralTruncated  bool
	IncidentTruncated bool
	Complete          bool
	ScopedLogcat      string
	Diagnostics       string
	NativeLog         string
	Crashes           []diagnosticIncident
	ANRs              []diagnosticIncident
	NativeCrashes     []diagnosticIncident
	ExitRecords       []processExitRecord
	AbnormalExits     int
	Sources           map[string]DiagnosticSourceState
	Errors            []string
}

var (
	processPIDPattern  = regexp.MustCompile(`(?i)Process:\s*[^,]+,\s*PID:\s*(\d+)`)
	startPIDPattern    = regexp.MustCompile(`(?i)Start proc\s+(\d+):`)
	nativePIDPattern   = regexp.MustCompile(`(?i)pid:\s*(\d+)`)
	eventFieldsPattern = regexp.MustCompile(`(?i)am_(?:crash|anr):\s*\[([^]]+)]`)
)

func analyzeDiagnostics(capture diagnosticCapture, packageName string, startedAt time.Time) diagnosticResult {
	sources := capture.Sources
	if sources == nil {
		sources = map[string]DiagnosticSourceState{"general_log": {Available: capture.LogcatAvailable, Truncated: capture.Truncated}}
	}
	general := sources["general_log"]
	crashSource, eventSource, exitSource := sources["crash_buffer"], sources["event_buffer"], sources["exit_info"]
	result := diagnosticResult{
		LogcatAvailable: capture.LogcatAvailable, Truncated: capture.Truncated,
		GeneralTruncated:  general.Truncated,
		IncidentTruncated: crashSource.Truncated || eventSource.Truncated || exitSource.Truncated,
		Complete:          crashSource.Available && eventSource.Available && exitSource.Available && !crashSource.Truncated && !eventSource.Truncated && !exitSource.Truncated,
		Sources:           sources, Errors: capture.Errors,
	}
	// Full logcat is intentionally bounded. Merge the much lower-volume crash
	// and lifecycle event buffers so incidents near the end of a long run are
	// still classified when the ordinary buffer has already reached its cap.
	type sourcedLine struct{ line, source string }
	lines := make([]sourcedLine, 0)
	for _, item := range []struct{ value, source string }{{capture.Logcat, "general_log"}, {capture.IncidentLog, "incident_buffers"}} {
		for _, line := range sessionLogLines(item.value, startedAt) {
			lines = append(lines, sourcedLine{line: line, source: item.source})
		}
	}
	pids := map[int]bool{}
	for _, item := range lines {
		line := item.line
		if !strings.Contains(line, packageName) {
			continue
		}
		for _, pattern := range []*regexp.Regexp{processPIDPattern, startPIDPattern, nativePIDPattern} {
			if match := pattern.FindStringSubmatch(line); len(match) == 2 {
				if pid, err := strconv.Atoi(match[1]); err == nil {
					pids[pid] = true
				}
			}
		}
		if pid := diagnosticEventPID(line); pid > 0 {
			pids[pid] = true
		}
	}
	var scoped, nativeLines []string
	for _, item := range lines {
		line := item.line
		pid, timestamp := logcatHeader(line)
		relevant := strings.Contains(line, packageName) || pids[pid]
		if !relevant {
			continue
		}
		// General logs are already filtered and bounded before analysis; keep
		// their retained head and tail intact instead of applying a second
		// head-only cap that would discard the session ending again.
		scoped = append(scoped, trimDiagnosticLine(line))
		lower := strings.ToLower(line)
		incidentPID := diagnosticPID(line, pid)
		nativeAMCrash := strings.Contains(lower, "am_crash:") && (strings.Contains(lower, ",native crash,") || strings.Contains(lower, "segmentation fault"))
		// Samsung has emitted native am_crash payloads whose first field repeats
		// the system_server log PID instead of the crashed app PID. Treat that
		// value as unknown and let the nearby ApplicationExitInfo record supply
		// the authoritative process PID.
		if nativeAMCrash && incidentPID == pid {
			incidentPID = 0
		}
		switch {
		case strings.Contains(lower, "libc:") && strings.Contains(lower, "fatal signal") ||
			(strings.Contains(lower, "debug:") && strings.Contains(lower, ">>> "+strings.ToLower(packageName)+" <<<") && strings.Contains(lower, "pid:")) ||
			nativeAMCrash:
			result.NativeCrashes = appendIncident(result.NativeCrashes, "native_crash", timestamp, incidentPID, line, item.source)
			nativeLines = appendBoundedLine(nativeLines, line)
		case strings.Contains(lower, "anr in "+strings.ToLower(packageName)) || strings.Contains(lower, "am_anr:") || (strings.Contains(lower, "inputdispatcher:") && strings.Contains(lower, "input dispatching timed out")):
			result.ANRs = appendIncident(result.ANRs, "anr", timestamp, incidentPID, line, item.source)
		case strings.Contains(lower, "androidruntime:") && strings.Contains(lower, "fatal exception") || strings.Contains(lower, "am_crash:"):
			result.Crashes = appendIncident(result.Crashes, "java_crash", timestamp, incidentPID, line, item.source)
		}
	}
	lastANR := filterPackageContext(capture.LastANR, packageName)
	exitParse := parseExitInfoDetailed(capture.ExitInfo, packageName, startedAt, capture.ExitTimezone)
	result.ExitRecords = aggregateExitRecords(exitParse.Records)
	if exitParse.Candidates != exitParse.Parsed {
		exitSource := result.Sources["exit_info"]
		exitSource.Error = fmt.Sprintf("parsed %d of %d ApplicationExitInfo records", exitParse.Parsed, exitParse.Candidates)
		result.Sources["exit_info"] = exitSource
		result.Errors = append(result.Errors, "exit_info: "+exitSource.Error)
		result.Complete = false
	}
	for _, record := range exitParse.Records {
		if record.Abnormal {
			result.AbnormalExits++
		}
		summary := fmt.Sprintf("exit-info reason=%s subreason=%s status=%d process=%s", record.Reason, record.Subreason, record.Status, record.Process)
		switch record.ReasonCode {
		case 4:
			result.Crashes = appendIncident(result.Crashes, "java_crash", record.parsedTime, record.PID, summary, "exit_info")
		case 5:
			result.NativeCrashes = appendIncident(result.NativeCrashes, "native_crash", record.parsedTime, record.PID, summary, "exit_info")
		case 6:
			result.ANRs = appendIncident(result.ANRs, "anr", record.parsedTime, record.PID, summary, "exit_info")
		}
	}
	result.Crashes = aggregateIncidents(result.Crashes)
	result.NativeCrashes = aggregateIncidents(result.NativeCrashes)
	result.ANRs = aggregateIncidents(result.ANRs)
	// Some vendor builds expose a native crash only through
	// ApplicationExitInfo and do not retain readable debuggerd/tombstone lines
	// for the shell user. Keep native_crash.txt useful in that case instead of
	// emitting a header-only file; the structured crash report remains the
	// canonical representation.
	if len(nativeLines) == 0 && len(result.NativeCrashes) > 0 {
		nativeLines = append(nativeLines, "[structured-native-incidents]")
		for _, incident := range result.NativeCrashes {
			nativeLines = append(nativeLines, fmt.Sprintf(
				"time=%s pid=%d summary=%s fingerprint=%s occurrences=%d sources=%s",
				incident.Time, incident.PID, incident.Summary, incident.Fingerprint,
				incident.Occurrences, strings.Join(incident.Sources, ",")))
		}
	}
	// `dumpsys activity lastanr` is global, persistent system state. It may
	// describe a previous Runner session (or even a previous installation of
	// the package), and vendor timestamp formats are not reliable enough to
	// scope it to startedAt. Preserve it in diagnostics.txt as supporting
	// context, but classify incidents only from timestamp-scoped logcat lines.
	result.ScopedLogcat = strings.Join(scoped, "\n")
	result.NativeLog = strings.Join(nativeLines, "\n")
	result.Diagnostics = diagnosticMetadata(capture, packageName, startedAt, lastANR)
	return result
}

func sessionLogLines(value string, startedAt time.Time) []string {
	lines := strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n")
	result := make([]string, 0, len(lines))
	threshold := startedAt.Add(-2 * time.Second)
	for _, line := range lines {
		line = redactDiagnosticLine(strings.TrimSpace(line))
		if line == "" {
			continue
		}
		_, timestamp := logcatHeader(line)
		if !timestamp.IsZero() && timestamp.Before(threshold) {
			continue
		}
		result = append(result, line)
	}
	return result
}

type processExitRecord struct {
	Timestamp        string `json:"timestamp"`
	PID              int    `json:"pid,omitempty"`
	Process          string `json:"process,omitempty"`
	ReasonCode       int    `json:"reasonCode"`
	Reason           string `json:"reason"`
	SubreasonCode    int    `json:"subreasonCode,omitempty"`
	Subreason        string `json:"subreason,omitempty"`
	Status           int    `json:"status"`
	Description      string `json:"description,omitempty"`
	Abnormal         bool   `json:"abnormal"`
	ExpectedToolExit bool   `json:"expectedToolExit,omitempty"`
	Fingerprint      string `json:"fingerprint"`
	Occurrences      int    `json:"occurrences"`
	FirstSeenAt      string `json:"firstSeenAt,omitempty"`
	LastSeenAt       string `json:"lastSeenAt,omitempty"`
	RepeatSummary    string `json:"repeatSummary,omitempty"`
	parsedTime       time.Time
}

type exitInfoReport struct {
	SchemaVersion string                           `json:"schemaVersion"`
	Package       string                           `json:"package"`
	CapturedAt    string                           `json:"capturedAt"`
	Available     bool                             `json:"available"`
	Complete      bool                             `json:"complete"`
	Truncated     bool                             `json:"truncated,omitempty"`
	Redacted      bool                             `json:"redacted"`
	Records       []processExitRecord              `json:"records"`
	Sources       map[string]DiagnosticSourceState `json:"sources,omitempty"`
	Errors        []string                         `json:"collectionErrors,omitempty"`
}

var (
	exitTimestampPattern = regexp.MustCompile(`timestamp=([0-9]{4}-[0-9]{2}-[0-9]{2} [0-9:.]+) pid=(\d+)`)
	exitReasonPattern    = regexp.MustCompile(`reason=(\d+) \((.*)\) subreason=(\d+) \((.*)\) status=(-?\d+)`)
	exitProcessPattern   = regexp.MustCompile(`(?:^|\s)process=(\S+)`)
)

type exitInfoParseResult struct {
	Records    []processExitRecord
	Candidates int
	Parsed     int
}

func parseExitInfo(value, packageName string, startedAt time.Time, timezone string) []processExitRecord {
	return parseExitInfoDetailed(value, packageName, startedAt, timezone).Records
}

func parseExitInfoDetailed(value, packageName string, startedAt time.Time, timezone string) exitInfoParseResult {
	_ = packageName // dumpsys scopes the records to the requested package/UID.
	location := time.UTC
	if match := regexp.MustCompile(`^([+-])(\d{2})(\d{2})$`).FindStringSubmatch(strings.TrimSpace(timezone)); len(match) == 4 {
		hours, _ := strconv.Atoi(match[2])
		minutes, _ := strconv.Atoi(match[3])
		offset := (hours*60 + minutes) * 60
		if match[1] == "-" {
			offset = -offset
		}
		location = time.FixedZone(timezone, offset)
	}
	lines := strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n")
	result := exitInfoParseResult{Records: make([]processExitRecord, 0)}
	var current *processExitRecord
	inWindow := false
	flush := func() {
		if current == nil {
			return
		}
		if current.ReasonCode >= 0 {
			result.Parsed++
			if inWindow {
				result.Records = append(result.Records, *current)
			}
		}
		current = nil
	}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "timestamp=") {
			flush()
			result.Candidates++
			match := exitTimestampPattern.FindStringSubmatch(line)
			if len(match) != 3 {
				continue
			}
			parsed, err := time.ParseInLocation("2006-01-02 15:04:05.000", match[1], location)
			if err != nil {
				continue
			}
			pid, _ := strconv.Atoi(match[2])
			current = &processExitRecord{Timestamp: parsed.UTC().Format(time.RFC3339Nano), PID: pid, ReasonCode: -1, parsedTime: parsed.UTC()}
			inWindow = !parsed.Before(startedAt) && !parsed.After(time.Now().Add(2*time.Second))
		}
		if current == nil {
			continue
		}
		if match := exitProcessPattern.FindStringSubmatch(line); len(match) == 2 {
			current.Process = match[1]
		}
		if match := exitReasonPattern.FindStringSubmatch(line); len(match) == 6 {
			current.ReasonCode, _ = strconv.Atoi(match[1])
			current.Reason = match[2]
			current.SubreasonCode, _ = strconv.Atoi(match[3])
			current.Subreason = match[4]
			current.Status, _ = strconv.Atoi(match[5])
			current.Abnormal = current.ReasonCode >= 2 && current.ReasonCode <= 9 || current.ReasonCode == 12
		}
		if index := strings.Index(line, "description="); index >= 0 {
			description := strings.TrimPrefix(line[index:], "description=")
			if state := strings.Index(description, " state="); state >= 0 {
				description = description[:state]
			}
			current.Description = redactDiagnosticLine(strings.TrimSpace(description))
		}
	}
	flush()
	return result
}

func aggregateExitRecords(records []processExitRecord) []processExitRecord {
	aggregated := make([]processExitRecord, 0, len(records))
	for _, record := range records {
		normalizedDescription := diagnosticHexPattern.ReplaceAllString(strings.ToLower(record.Description), "<hex>")
		normalizedDescription = diagnosticNumberPattern.ReplaceAllString(normalizedDescription, "<n>")
		sum := sha256.Sum256([]byte(fmt.Sprintf("%d\n%d\n%s\n%d\n%s", record.ReasonCode, record.SubreasonCode, record.Process, record.Status, normalizedDescription)))
		record.Fingerprint = hex.EncodeToString(sum[:12])
		record.Occurrences = 1
		record.FirstSeenAt, record.LastSeenAt = record.Timestamp, record.Timestamp
		matched := -1
		for index := range aggregated {
			if aggregated[index].Fingerprint == record.Fingerprint {
				matched = index
				break
			}
		}
		if matched < 0 {
			aggregated = append(aggregated, record)
			continue
		}
		item := &aggregated[matched]
		item.Occurrences++
		item.LastSeenAt = record.Timestamp
		item.RepeatSummary = fmt.Sprintf("same process exit observed %d times (%d repeats after first detail)", item.Occurrences, item.Occurrences-1)
	}
	return aggregated
}

func logcatHeader(line string) (int, time.Time) {
	fields := strings.Fields(line)
	if len(fields) < 3 {
		return 0, time.Time{}
	}
	seconds, _ := strconv.ParseFloat(fields[0], 64)
	// Android's plain `logcat -v epoch` prefix is: epoch PID TID priority.
	// fields[2] is the thread ID and must not be used for process correlation.
	pid, _ := strconv.Atoi(fields[1])
	if seconds <= 0 {
		return pid, time.Time{}
	}
	nanos := int64(seconds * float64(time.Second))
	return pid, time.Unix(0, nanos).UTC()
}

func diagnosticPID(line string, fallback int) int {
	for _, pattern := range []*regexp.Regexp{processPIDPattern, nativePIDPattern, startPIDPattern} {
		if match := pattern.FindStringSubmatch(line); len(match) == 2 {
			if pid, err := strconv.Atoi(match[1]); err == nil {
				return pid
			}
		}
	}
	if pid := diagnosticEventPID(line); pid > 0 {
		return pid
	}
	return fallback
}

func diagnosticEventPID(line string) int {
	match := eventFieldsPattern.FindStringSubmatch(line)
	if len(match) != 2 {
		return 0
	}
	pid := 0
	for _, field := range strings.Split(match[1], ",") {
		field = strings.TrimSpace(field)
		if strings.Contains(field, ".") {
			break
		}
		if candidate, err := strconv.Atoi(field); err == nil && candidate > 0 {
			pid = candidate
		}
	}
	return pid
}

func appendIncident(values []diagnosticIncident, kind string, timestamp time.Time, pid int, summary, source string) []diagnosticIncident {
	incident := diagnosticIncident{Kind: kind, PID: pid, Summary: trimDiagnosticLine(summary), Occurrences: 1, source: source, parsedTime: timestamp}
	if !timestamp.IsZero() {
		incident.Time = timestamp.Format(time.RFC3339Nano)
		incident.FirstSeenAt, incident.LastSeenAt = incident.Time, incident.Time
	}
	return append(values, incident)
}

var (
	diagnosticLogPrefixPattern = regexp.MustCompile(`^\d+(?:\.\d+)?\s+\d+\s+\d+\s+[A-Z]\s+`)
	diagnosticHexPattern       = regexp.MustCompile(`(?i)0x[0-9a-f]+`)
	diagnosticNumberPattern    = regexp.MustCompile(`\b\d+\b`)
)

func aggregateIncidents(observations []diagnosticIncident) []diagnosticIncident {
	if len(observations) == 0 {
		return observations
	}
	sort.SliceStable(observations, func(i, j int) bool {
		if observations[i].parsedTime.IsZero() {
			return false
		}
		if observations[j].parsedTime.IsZero() {
			return true
		}
		return observations[i].parsedTime.Before(observations[j].parsedTime)
	})
	occurrences := make([]diagnosticIncident, 0, len(observations))
	for _, observation := range observations {
		merged := false
		for index := len(occurrences) - 1; index >= 0; index-- {
			candidate := &occurrences[index]
			if candidate.Kind != observation.Kind || (candidate.PID != observation.PID && candidate.PID != 0 && observation.PID != 0) {
				continue
			}
			delta := observation.parsedTime.Sub(candidate.parsedTime)
			sameOccurrence := !candidate.parsedTime.IsZero() && !observation.parsedTime.IsZero() && delta >= 0 && sameIncidentOccurrence(*candidate, observation, delta)
			if !sameOccurrence && (candidate.parsedTime.IsZero() || observation.parsedTime.IsZero()) {
				sameOccurrence = candidate.Summary == observation.Summary
			}
			if !sameOccurrence {
				break
			}
			candidate.Sources = appendUnique(candidate.Sources, candidate.source, observation.source)
			if incidentEvidenceRank(observation.Summary) > incidentEvidenceRank(candidate.Summary) {
				candidate.Summary = observation.Summary
				candidate.source = observation.source
			}
			if candidate.PID == 0 && observation.PID != 0 {
				candidate.PID = observation.PID
			}
			if observation.parsedTime.After(candidate.parsedTime) {
				candidate.parsedTime = observation.parsedTime
				candidate.LastSeenAt = observation.parsedTime.Format(time.RFC3339Nano)
			}
			merged = true
			break
		}
		if !merged {
			observation.Sources = appendUnique(nil, observation.source)
			occurrences = append(occurrences, observation)
		}
	}
	aggregated := make([]diagnosticIncident, 0, len(occurrences))
	for _, occurrence := range occurrences {
		occurrence.Fingerprint = incidentFingerprint(occurrence.Kind, occurrence.Summary)
		matched := -1
		for index := range aggregated {
			if aggregated[index].Fingerprint == occurrence.Fingerprint {
				matched = index
				break
			}
		}
		if matched < 0 {
			occurrence.Occurrences = 1
			aggregated = append(aggregated, occurrence)
			continue
		}
		item := &aggregated[matched]
		item.Occurrences++
		item.Sources = appendUnique(item.Sources, occurrence.Sources...)
		if occurrence.LastSeenAt != "" {
			item.LastSeenAt = occurrence.LastSeenAt
		}
		item.RepeatSummary = fmt.Sprintf("same %s observed %d times (%d repeats after first detail)", incidentDisplayName(item.Kind), item.Occurrences, item.Occurrences-1)
	}
	for index := range aggregated {
		sort.Strings(aggregated[index].Sources)
		aggregated[index].source = ""
	}
	return aggregated
}

func sameIncidentOccurrence(left, right diagnosticIncident, delta time.Duration) bool {
	if left.Summary == right.Summary {
		return delta <= 2*time.Second
	}
	if left.source == "exit_info" || right.source == "exit_info" {
		return delta <= 2*time.Second
	}
	return delta <= 500*time.Millisecond && duplicateIncidentRepresentation(left.Kind, left.Summary, right.Summary)
}

func duplicateIncidentRepresentation(kind, left, right string) bool {
	left, right = strings.ToLower(trimDiagnosticLine(left)), strings.ToLower(trimDiagnosticLine(right))
	pair := func(first, second string) bool {
		return strings.Contains(left, first) && strings.Contains(right, second) || strings.Contains(left, second) && strings.Contains(right, first)
	}
	switch kind {
	case "java_crash":
		return pair("fatal exception", "am_crash:")
	case "native_crash":
		return pair("fatal signal", ">>>")
	case "anr":
		return pair("am_anr:", "input dispatching timed out") || pair("am_anr:", "anr in ")
	default:
		return false
	}
}

func incidentEvidenceRank(summary string) int {
	lower := strings.ToLower(summary)
	switch {
	case strings.Contains(lower, "am_crash:") || strings.Contains(lower, "am_anr:") || strings.Contains(lower, "fatal signal"):
		return 3
	case strings.Contains(lower, "fatal exception") || strings.Contains(lower, "anr in ") || strings.Contains(lower, ">>>"):
		return 2
	case strings.Contains(lower, "exit-info"):
		return 1
	default:
		return 0
	}
}

func incidentFingerprint(kind, summary string) string {
	normalized := strings.ToLower(strings.TrimSpace(diagnosticLogPrefixPattern.ReplaceAllString(summary, "")))
	normalized = diagnosticHexPattern.ReplaceAllString(normalized, "<hex>")
	normalized = diagnosticNumberPattern.ReplaceAllString(normalized, "<n>")
	sum := sha256.Sum256([]byte(kind + "\n" + normalized))
	return hex.EncodeToString(sum[:12])
}

func incidentDisplayName(kind string) string {
	switch kind {
	case "java_crash":
		return "Java Crash"
	case "native_crash":
		return "Native Crash"
	case "anr":
		return "ANR"
	default:
		return kind
	}
}

func appendUnique(values []string, additions ...string) []string {
	for _, addition := range additions {
		if addition == "" {
			continue
		}
		found := false
		for _, value := range values {
			if value == addition {
				found = true
				break
			}
		}
		if !found {
			values = append(values, addition)
		}
	}
	return values
}

func totalIncidentOccurrences(values []diagnosticIncident) int {
	total := 0
	for _, value := range values {
		if value.Occurrences > 0 {
			total += value.Occurrences
		} else {
			total++
		}
	}
	return total
}

func appendBoundedLine(lines []string, line string) []string {
	line = trimDiagnosticLine(line)
	current := 0
	for _, value := range lines {
		if value == line {
			return lines
		}
		current += len(value) + 1
	}
	if current+len(line)+1 > maxDiagnosticArtifactBytes {
		return lines
	}
	return append(lines, line)
}

func trimDiagnosticLine(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 4096 {
		return value[:4096] + " [truncated]"
	}
	return value
}

func filterPackageLines(value, packageName string) string {
	var result []string
	for _, line := range strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n") {
		if strings.Contains(line, packageName) {
			result = appendBoundedLine(result, line)
		}
	}
	return strings.Join(result, "\n")
}

func filterPackageContext(value, packageName string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	if !strings.Contains(value, packageName) {
		return ""
	}
	var result []string
	for _, line := range strings.Split(value, "\n") {
		result = appendBoundedLine(result, line)
	}
	return strings.Join(result, "\n")
}

func diagnosticMetadata(capture diagnosticCapture, packageName string, startedAt time.Time, lastANR string) string {
	var value strings.Builder
	value.WriteString(fmt.Sprintf("package=%s\nstartedAt=%s\nlogcatAvailable=%t\ntruncated=%t\nredacted=true\n", packageName, startedAt.UTC().Format(time.RFC3339Nano), capture.LogcatAvailable, capture.Truncated))
	keys := make([]string, 0, len(capture.Sources))
	for name := range capture.Sources {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	for _, name := range keys {
		status := capture.Sources[name]
		value.WriteString(fmt.Sprintf("source.%s.available=%t\nsource.%s.truncated=%t\n", name, status.Available, name, status.Truncated))
		if status.Error != "" {
			value.WriteString(fmt.Sprintf("source.%s.error=%s\n", name, status.Error))
		}
	}
	for _, err := range capture.Errors {
		value.WriteString(fmt.Sprintf("collectionError=%s\n", err))
	}
	if lastANR != "" {
		value.WriteString("\n[lastanr]\n")
		value.WriteString(lastANR)
		value.WriteByte('\n')
	}
	if capture.Processes != "" {
		value.WriteString("\n[activity-processes]\n")
		value.WriteString(capture.Processes)
		value.WriteByte('\n')
	}
	return value.String()
}

func writeDiagnosticReport(path, schemaVersion, packageName string, available, complete, truncated bool, incidents []diagnosticIncident, sources map[string]DiagnosticSourceState, collectionErrors []string) error {
	if incidents == nil {
		incidents = []diagnosticIncident{}
	}
	report := diagnosticReport{SchemaVersion: schemaVersion, Package: packageName, CapturedAt: time.Now().UTC().Format(time.RFC3339Nano), Available: available, Complete: complete, Truncated: truncated, Redacted: true, Sources: sources, Incidents: incidents, Errors: collectionErrors}
	content, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(path, append(content, '\n'), 0o644)
}

func writeExitInfoReport(path, packageName string, result diagnosticResult) error {
	exitSource := result.Sources["exit_info"]
	records := result.ExitRecords
	if records == nil {
		records = []processExitRecord{}
	}
	report := exitInfoReport{
		SchemaVersion: "xtest-exit-info/v1", Package: packageName,
		CapturedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Available:  exitSource.Available, Complete: exitSource.Available && !exitSource.Truncated,
		Truncated: exitSource.Truncated, Redacted: true, Records: records,
		Sources: map[string]DiagnosticSourceState{"exit_info": exitSource, "device_timezone": result.Sources["device_timezone"]},
		Errors:  result.Errors,
	}
	content, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(path, append(content, '\n'), 0o644)
}
