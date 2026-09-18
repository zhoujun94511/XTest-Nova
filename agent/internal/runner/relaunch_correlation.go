package runner

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
	"time"
)

type relaunchWindow struct {
	Start  time.Time
	End    time.Time
	Reason string
}

func parseRelaunchWindows(path string) ([]relaunchWindow, error) {
	if path == "" {
		return nil, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	var windows []relaunchWindow
	var pending *relaunchWindow
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64<<10), 1<<20)
	for scanner.Scan() {
		var event eventRecord
		if json.Unmarshal(scanner.Bytes(), &event) != nil {
			continue
		}
		when := time.UnixMilli(event.Time).UTC()
		switch event.State {
		case "relaunch_requested":
			pending = &relaunchWindow{Start: when, End: when.Add(10 * time.Second), Reason: event.Reason}
		case "relaunch_completed":
			if pending == nil {
				continue
			}
			pending.End = when.Add(2 * time.Second)
			windows = append(windows, *pending)
			pending = nil
		}
	}
	if pending != nil {
		windows = append(windows, *pending)
	}
	return windows, scanner.Err()
}

func applyRelaunchExitCorrelation(result *diagnosticResult, windows []relaunchWindow) {
	if result == nil || len(result.ExitRecords) == 0 || len(windows) == 0 {
		return
	}
	abnormal := 0
	for index := range result.ExitRecords {
		record := &result.ExitRecords[index]
		if matchesToolRelaunch(record, windows) {
			record.Abnormal = false
			record.ExpectedToolExit = true
		}
		if record.Abnormal {
			abnormal++
		}
	}
	result.AbnormalExits = abnormal
}

func matchesToolRelaunch(record *processExitRecord, windows []relaunchWindow) bool {
	if record == nil || record.parsedTime.IsZero() {
		return false
	}
	if record.ReasonCode != 10 && !(record.ReasonCode == 2 && strings.Contains(strings.ToUpper(record.Subreason), "FORCE")) {
		return false
	}
	subreason := strings.ToUpper(record.Subreason + " " + record.Reason)
	if !strings.Contains(subreason, "FORCE") && record.ReasonCode != 10 {
		return false
	}
	when := record.parsedTime
	for _, window := range windows {
		if !when.Before(window.Start) && !when.After(window.End) {
			return true
		}
	}
	return false
}
