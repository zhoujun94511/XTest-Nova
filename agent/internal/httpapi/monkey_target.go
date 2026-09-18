package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/zhoujun94511/xtest-nova/agent/internal/recordreplay"
	"github.com/zhoujun94511/xtest-nova/agent/internal/runner"
)

type monkeyTargetPlan struct {
	token       string
	requestID   string
	fingerprint string
	cases       map[string]recordreplay.Case
	used        map[string]bool
}

func targetCaseKey(activity, task, caseName string) string {
	return activity + "\x00" + task + "\x00" + caseName
}

func monkeyTargetFingerprint(config runner.RunConfig) string {
	encoded, _ := json.Marshal(struct {
		Package string              `json:"package"`
		Cases   []runner.TargetCase `json:"cases"`
	}{Package: config.Package, Cases: config.TargetCases})
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func (a *API) prepareMonkeyTargetPlan(config *runner.RunConfig) (*monkeyTargetPlan, error) {
	if len(config.TargetCases) == 0 {
		return nil, nil
	}
	if a.recordReplay == nil {
		return nil, errors.New("record/replay engine unavailable")
	}
	for _, target := range config.TargetCases {
		found := false
		for _, activity := range config.TargetActivities {
			found = found || activity == target.Activity
		}
		if !found {
			config.TargetActivities = append(config.TargetActivities, target.Activity)
		}
	}
	fingerprint := monkeyTargetFingerprint(*config)
	a.monkeyTargetMu.Lock()
	existing := a.monkeyTarget
	if existing != nil && config.RequestID != "" && existing.requestID == config.RequestID && existing.fingerprint == fingerprint {
		config.TargetToken = existing.token
		a.monkeyTargetMu.Unlock()
		return existing, nil
	}
	a.monkeyTargetMu.Unlock()
	summaries, err := a.recordReplay.Cases(config.Package)
	if err != nil {
		return nil, err
	}
	buffer := make([]byte, 16)
	if _, err = rand.Read(buffer); err != nil {
		return nil, err
	}
	token := hex.EncodeToString(buffer)
	if config.RequestID == "" {
		config.RequestID = "monkey-target-" + token
	}
	config.TargetToken = token
	plan := &monkeyTargetPlan{token: token, requestID: config.RequestID, fingerprint: fingerprint, cases: map[string]recordreplay.Case{}, used: map[string]bool{}}
	for _, target := range config.TargetCases {
		var selected string
		for _, summary := range summaries {
			if summary.Task == target.Task && summary.Name == target.Case {
				selected = summary.ID
				break // Cases is newest-first; use the latest matching signed case.
			}
		}
		if selected == "" {
			return nil, fmt.Errorf("target-page case not found: %s/%s", target.Task, target.Case)
		}
		value, loadErr := a.recordReplay.LoadCase(selected)
		if loadErr != nil || value.Package != config.Package {
			if loadErr != nil {
				return nil, loadErr
			}
			return nil, errors.New("target-page case package mismatch")
		}
		key := targetCaseKey(target.Activity, target.Task, target.Case)
		if _, duplicate := plan.cases[key]; duplicate {
			return nil, errors.New("duplicate target-page case mapping")
		}
		plan.cases[key] = value
	}
	return plan, nil
}

func (a *API) setMonkeyTargetPlan(plan *monkeyTargetPlan) {
	a.monkeyTargetMu.Lock()
	a.monkeyTarget = plan
	a.monkeyTargetMu.Unlock()
}

func (a *API) clearMonkeyTargetPlan() {
	a.monkeyTargetMu.Lock()
	a.monkeyTarget = nil
	a.monkeyTargetMu.Unlock()
}

func (a *API) runMonkeyTargetCase(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	var request struct {
		Token    string `json:"token"`
		Activity string `json:"activity"`
		Task     string `json:"task,omitempty"`
		Case     string `json:"case"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	key := targetCaseKey(strings.TrimSpace(request.Activity), strings.TrimSpace(request.Task), strings.TrimSpace(request.Case))
	a.executionMu.Lock()
	a.monkeyTargetMu.Lock()
	plan := a.monkeyTarget
	if plan == nil || request.Token == "" || request.Token != plan.token || plan.used[key] {
		a.monkeyTargetMu.Unlock()
		a.executionMu.Unlock()
		fail(w, http.StatusForbidden, errors.New("invalid or already-used target-page case token"))
		return
	}
	value, ok := plan.cases[key]
	runnerState := a.runner.State()
	if !ok || !runnerState.Running || runnerState.RequestID != plan.requestID {
		a.monkeyTargetMu.Unlock()
		a.executionMu.Unlock()
		fail(w, http.StatusConflict, errors.New("target-page case is not active for this Monkey run"))
		return
	}
	plan.used[key] = true
	a.monkeyTargetMu.Unlock()

	state, err := a.recordReplay.StartReplay(r.Context(), recordreplay.ReplayConfig{Execute: true, Speed: 1, Case: value})
	if err != nil {
		a.monkeyTargetMu.Lock()
		if a.monkeyTarget == plan {
			plan.used[key] = false
		}
		a.monkeyTargetMu.Unlock()
	}
	a.executionMu.Unlock()
	if err != nil {
		fail(w, http.StatusConflict, err)
		return
	}
	deadline := time.NewTimer(5 * time.Minute)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer deadline.Stop()
	defer ticker.Stop()
	for state.Running || state.Stopping || state.Finalizing {
		select {
		case <-r.Context().Done():
			stopContext, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			_, _ = a.recordReplay.StopReplay(stopContext)
			cancel()
			return
		case <-deadline.C:
			stopContext, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			_, _ = a.recordReplay.StopReplay(stopContext)
			cancel()
			fail(w, http.StatusGatewayTimeout, errors.New("target-page case exceeded five minutes"))
			return
		case <-ticker.C:
			state = a.recordReplay.ReplayState()
		}
	}
	if state.StopReason != "completed" {
		fail(w, http.StatusConflict, fmt.Errorf("target-page case stopped: %s: %s", state.StopReason, state.Error))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "activity": request.Activity, "task": request.Task, "case": request.Case, "completedActions": state.Completed})
}
