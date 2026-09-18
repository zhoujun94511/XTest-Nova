package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	diagnosticLogHeadBytes = 512 << 10
	diagnosticLogTailBytes = 15 << 19 // 7.5 MiB
)

var (
	sensitiveJSONPattern = regexp.MustCompile(`(?i)("\$?(?:access_token|advertising_id|anonymous_id|authorization|device_id|email|gaid|id_token|refresh_token|s_user_id|token|user_id)"\s*:\s*")[^"]*(")`)
	sensitiveKVPattern   = regexp.MustCompile(`(?i)(^|[^A-Za-z0-9_])(\$?(?:access_token|advertising_id|anonymous_id|authorization|device_id|email|gaid|id_token|refresh_token|s_user_id|token|user_id))=([^\s,&}\]]+)`)
	bearerPattern        = regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/=-]+`)
)

type boundedLineLog struct {
	head      []string
	tail      []string
	headBytes int
	tailBytes int
	tailStart int
	truncated bool
}

func (b *boundedLineLog) add(line string) {
	line = trimDiagnosticLine(line)
	size := len(line) + 1
	if b.headBytes+size <= diagnosticLogHeadBytes {
		b.head = append(b.head, line)
		b.headBytes += size
		return
	}
	b.tail = append(b.tail, line)
	b.tailBytes += size
	for b.tailBytes > diagnosticLogTailBytes && b.tailStart < len(b.tail) {
		b.tailBytes -= len(b.tail[b.tailStart]) + 1
		b.tailStart++
		b.truncated = true
	}
	if b.tailStart > 4096 && b.tailStart*2 > len(b.tail) {
		b.tail = append([]string(nil), b.tail[b.tailStart:]...)
		b.tailStart = 0
	}
}

func (b *boundedLineLog) value() (string, bool) {
	lines := append([]string(nil), b.head...)
	if b.truncated {
		lines = append(lines, "--- earlier scoped log lines omitted by bounded tail retention ---")
	}
	lines = append(lines, b.tail[b.tailStart:]...)
	return strings.Join(lines, "\n"), b.truncated
}

type scopedLogWriter struct {
	mu          sync.Mutex
	packageName string
	pids        map[int]bool
	death       []*regexp.Regexp
	pending     []byte
	log         boundedLineLog
}

func newScopedLogWriter(packageName string, pids []int) *scopedLogWriter {
	escaped := regexp.QuoteMeta(packageName)
	writer := &scopedLogWriter{packageName: packageName, pids: map[int]bool{}, death: []*regexp.Regexp{
		regexp.MustCompile(`(?i)Process\s+` + escaped + `(?:\S*)?\s+\(pid\s+(\d+)\)\s+has died`),
		regexp.MustCompile(`(?i)(?:Process|Killing)\s+(\d+):` + escaped + `(?:[/:\s]|$)`),
	}}
	for _, pid := range pids {
		if pid > 0 {
			writer.pids[pid] = true
		}
	}
	return writer
}

func (w *scopedLogWriter) Write(value []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pending = append(w.pending, value...)
	for {
		index := bytes.IndexByte(w.pending, '\n')
		if index < 0 {
			break
		}
		w.addLineLocked(string(w.pending[:index]))
		w.pending = w.pending[index+1:]
	}
	if len(w.pending) > 64<<10 {
		w.addLineLocked(string(w.pending))
		w.pending = nil
	}
	return len(value), nil
}

func (w *scopedLogWriter) addLineLocked(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	pid, _ := logcatHeader(line)
	containsPackage := strings.Contains(line, w.packageName)
	for _, pattern := range w.death {
		if match := pattern.FindStringSubmatch(line); len(match) == 2 {
			if dead, err := strconv.Atoi(match[1]); err == nil {
				delete(w.pids, dead)
			}
		}
	}
	if containsPackage {
		for _, pattern := range []*regexp.Regexp{processPIDPattern, startPIDPattern, nativePIDPattern} {
			if match := pattern.FindStringSubmatch(line); len(match) == 2 {
				if discovered, err := strconv.Atoi(match[1]); err == nil && discovered > 0 {
					w.pids[discovered] = true
				}
			}
		}
		if discovered := diagnosticEventPID(line); discovered > 0 {
			w.pids[discovered] = true
		}
	}
	if !containsPackage && !w.pids[pid] {
		return
	}
	w.log.add(redactDiagnosticLine(line))
}

func (w *scopedLogWriter) snapshot() (string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.pending) > 0 {
		w.addLineLocked(string(w.pending))
		w.pending = nil
	}
	return w.log.value()
}

func redactDiagnosticLine(line string) string {
	line = sensitiveJSONPattern.ReplaceAllString(line, `${1}[REDACTED]${2}`)
	line = sensitiveKVPattern.ReplaceAllString(line, `${1}${2}=[REDACTED]`)
	return bearerPattern.ReplaceAllString(line, "Bearer [REDACTED]")
}

type sessionLogCapture struct {
	cancel context.CancelFunc
	done   chan error
	writer *scopedLogWriter
	stderr *cappedBuffer
}

func currentPackagePIDs(ctx context.Context, packageName string) []int {
	pidContext, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	output, err := exec.CommandContext(pidContext, "pidof", packageName).Output()
	if err != nil {
		return nil
	}
	var result []int
	for _, field := range strings.Fields(string(output)) {
		if pid, convertErr := strconv.Atoi(field); convertErr == nil && pid > 0 {
			result = append(result, pid)
		}
	}
	return result
}

func startSessionLogCapture(ctx context.Context, packageName string) (*sessionLogCapture, error) {
	if _, err := exec.LookPath("logcat"); err != nil {
		return nil, err
	}
	streamContext, cancel := context.WithCancel(context.Background())
	writer := newScopedLogWriter(packageName, currentPackagePIDs(ctx, packageName))
	stderr := &cappedBuffer{limit: 64 << 10}
	command := exec.CommandContext(streamContext, "logcat", "-b", "all", "-v", "epoch", "-T", "1")
	command.Stdout, command.Stderr = writer, stderr
	if err := command.Start(); err != nil {
		cancel()
		return nil, err
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	return &sessionLogCapture{cancel: cancel, done: done, writer: writer, stderr: stderr}, nil
}

func (s *sessionLogCapture) stop() (string, bool, error) {
	if s == nil {
		return "", false, errors.New("session log capture unavailable")
	}
	var commandErr error
	endedEarly := false
	select {
	case commandErr = <-s.done:
		endedEarly = true
	default:
		s.cancel()
		commandErr = <-s.done
		commandErr = nil // cancellation is the expected session boundary
	}
	value, truncated := s.writer.snapshot()
	if endedEarly && commandErr == nil {
		return value, truncated, errors.New("session logcat exited before the session ended")
	}
	if commandErr != nil {
		message := strings.TrimSpace(s.stderr.buffer.String())
		if message != "" {
			return value, truncated, fmt.Errorf("session logcat exited: %w: %s", commandErr, message)
		}
		return value, truncated, fmt.Errorf("session logcat exited: %w", commandErr)
	}
	return value, truncated, nil
}
