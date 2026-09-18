package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/zhoujun94511/xtest-nova/agent/internal/evaluation"
	"github.com/zhoujun94511/xtest-nova/agent/internal/exploration"
	"github.com/zhoujun94511/xtest-nova/agent/internal/runner"
)

func (a *API) registerExplorationRoutes(m *http.ServeMux) {
	m.HandleFunc("POST /v1/exploration/preview", a.explorationPreview)
	m.HandleFunc("POST /v1/exploration/sessions", a.startExploration)
	m.HandleFunc("GET /v1/exploration/sessions/current", a.explorationState)
	m.HandleFunc("DELETE /v1/exploration/sessions/current", a.stopExploration)
	m.HandleFunc("GET /v1/exploration/graph", a.explorationGraph)
	m.HandleFunc("GET /v1/exploration/steps", a.explorationSteps)
	m.HandleFunc("GET /v1/exploration/receipts", a.explorationReceipts)
	m.HandleFunc("GET /v1/exploration/report", a.explorationReport)
	m.HandleFunc("GET /v1/monkey/report", a.monkeyReport)
}

func (a *API) monkeyReport(w http.ResponseWriter, r *http.Request) {
	state := a.runner.State()
	scenario := r.URL.Query().Get("scenario")
	if scenario == "" {
		scenario = "unspecified"
	}
	report := evaluation.FromRunner(scenario, Version, runner.RunConfig{RequestID: state.RequestID, Package: state.Package}, state)
	writeJSON(w, http.StatusOK, report)
}

func (a *API) explorationReport(w http.ResponseWriter, r *http.Request) {
	if !a.requireExplorer(w) {
		return
	}
	scenario := r.URL.Query().Get("scenario")
	if scenario == "" {
		scenario = "unspecified"
	}
	report := evaluation.FromNova(scenario, Version, a.explorer.State(), a.explorer.Graph(), a.explorer.Steps())
	writeJSON(w, http.StatusOK, report)
}

func (a *API) requireExplorer(w http.ResponseWriter) bool {
	if a.explorer == nil {
		fail(w, http.StatusServiceUnavailable, errors.New("exploration engine unavailable"))
		return false
	}
	return true
}

func decodeExplorationJSON(w http.ResponseWriter, r *http.Request, value any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 128<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		fail(w, http.StatusBadRequest, err)
		return false
	}
	return true
}

func (a *API) explorationPreview(w http.ResponseWriter, r *http.Request) {
	if !a.requireExplorer(w) {
		return
	}
	var request struct {
		Package string            `json:"package"`
		Rules   exploration.Rules `json:"rules,omitempty"`
	}
	if !decodeExplorationJSON(w, r, &request) {
		return
	}
	analysis, err := a.explorer.Preview(r.Context(), request.Package, request.Rules)
	if err != nil {
		fail(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "analysis": analysis})
}

func (a *API) startExploration(w http.ResponseWriter, r *http.Request) {
	if !a.requireExplorer(w) {
		return
	}
	var config exploration.Config
	if !decodeExplorationJSON(w, r, &config) {
		return
	}
	a.executionMu.Lock()
	defer a.executionMu.Unlock()
	if a.shuttingDown {
		fail(w, http.StatusServiceUnavailable, errors.New("agent is shutting down"))
		return
	}
	runnerState := a.runner.State()
	if runnerState.Running || runnerState.Finalizing {
		fail(w, http.StatusConflict, errors.New("random runner session already active"))
		return
	}
	if a.recordReplay != nil {
		recording, replay := a.recordReplay.RecordingState(), a.recordReplay.ReplayState()
		if recording.Running || recording.Stopping || recording.Finalizing || replay.Running || replay.Stopping || replay.Finalizing {
			fail(w, http.StatusConflict, errors.New("record or replay session already active"))
			return
		}
	}
	state, err := a.explorer.Start(config)
	if err != nil {
		fail(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusAccepted, state)
}

func (a *API) explorationState(w http.ResponseWriter, _ *http.Request) {
	if a.requireExplorer(w) {
		writeJSON(w, http.StatusOK, a.explorer.State())
	}
}

func (a *API) stopExploration(w http.ResponseWriter, r *http.Request) {
	if !a.requireExplorer(w) {
		return
	}
	sessionID, ownerToken := executionCredentials(r)
	state, err := a.explorer.StopOwned(r.Context(), sessionID, ownerToken)
	if err != nil {
		fail(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (a *API) explorationGraph(w http.ResponseWriter, _ *http.Request) {
	if a.requireExplorer(w) {
		writeJSON(w, http.StatusOK, a.explorer.Graph())
	}
}

func (a *API) explorationSteps(w http.ResponseWriter, _ *http.Request) {
	if a.requireExplorer(w) {
		writeJSON(w, http.StatusOK, a.explorer.Steps())
	}
}

func (a *API) explorationReceipts(w http.ResponseWriter, _ *http.Request) {
	if a.requireExplorer(w) {
		writeJSON(w, http.StatusOK, map[string]any{"receipts": a.explorer.Receipts()})
	}
}
