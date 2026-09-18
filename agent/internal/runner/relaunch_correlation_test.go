package runner

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestApplyRelaunchExitCorrelationMarksForceStopWithinWindow(t *testing.T) {
	started := time.Unix(100, 0).UTC()
	windowStart := started.Add(5 * time.Second)
	windows := []relaunchWindow{{Start: windowStart, End: windowStart.Add(3 * time.Second), Reason: "exploration"}}
	result := diagnosticResult{
		ExitRecords: []processExitRecord{{
			ReasonCode: 10,
			Reason:     "USER REQUESTED",
			Subreason:  "FORCE STOP",
			Status:     0,
			Abnormal:   true,
			parsedTime: windowStart.Add(1 * time.Second),
		}},
		AbnormalExits: 1,
	}
	applyRelaunchExitCorrelation(&result, windows)
	if result.ExitRecords[0].Abnormal || !result.ExitRecords[0].ExpectedToolExit {
		t.Fatalf("record=%#v", result.ExitRecords[0])
	}
	if result.AbnormalExits != 0 {
		t.Fatalf("abnormal exits=%d", result.AbnormalExits)
	}
}

func TestApplyRelaunchExitCorrelationKeepsUnknownSigkillAbnormal(t *testing.T) {
	windows := []relaunchWindow{{Start: time.Unix(200, 0).UTC(), End: time.Unix(205, 0).UTC()}}
	result := diagnosticResult{
		ExitRecords: []processExitRecord{{
			ReasonCode: 2,
			Reason:     "SIGNALED",
			Status:     9,
			Abnormal:   true,
			parsedTime: time.Unix(100, 0).UTC(),
		}},
		AbnormalExits: 1,
	}
	applyRelaunchExitCorrelation(&result, windows)
	if !result.ExitRecords[0].Abnormal || result.ExitRecords[0].ExpectedToolExit {
		t.Fatalf("record=%#v", result.ExitRecords[0])
	}
}

func TestParseRelaunchWindowsPairsRequestedAndCompleted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	content := "" +
		`{"state":"relaunch_requested","reason":"exploration","time":100000}` + "\n" +
		`{"state":"relaunch_completed","reason":"exploration","time":102500}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	windows, err := parseRelaunchWindows(path)
	if err != nil || len(windows) != 1 {
		t.Fatalf("windows=%#v err=%v", windows, err)
	}
	if windows[0].End.Sub(windows[0].Start) < 2*time.Second {
		t.Fatalf("window=%#v", windows[0])
	}
}
