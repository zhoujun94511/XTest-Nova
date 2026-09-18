package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/zhoujun94511/xtest-nova/agent/internal/execution"
	"github.com/zhoujun94511/xtest-nova/agent/internal/perflog"
)

func (a *API) registerPerformanceRoutes(m *http.ServeMux) {
	m.HandleFunc("POST /v1/performance/sessions", a.startPerformanceSession)
	m.HandleFunc("GET /v1/performance/sessions/current", a.performanceSession)
	m.HandleFunc("DELETE /v1/performance/sessions/current", a.stopPerformanceSession)
	m.HandleFunc("POST /v1/performance/startup", a.measureStartupPerformance)
}

func (a *API) measureStartupPerformance(w http.ResponseWriter, r *http.Request) {
	var request perflog.StartupConfig
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	a.executionMu.Lock()
	defer a.executionMu.Unlock()
	if a.shuttingDown {
		fail(w, http.StatusServiceUnavailable, errors.New("agent is shutting down"))
		return
	}
	if state := a.runner.State(); state.Running || state.Finalizing {
		fail(w, http.StatusConflict, errors.New("random runner session already active"))
		return
	}
	if a.explorer != nil {
		state := a.explorer.State()
		if state.Running || state.Stopping || state.Finalizing {
			fail(w, http.StatusConflict, errors.New("exploration session already active"))
			return
		}
	}
	if a.recordReplay != nil {
		recording, replay := a.recordReplay.RecordingState(), a.recordReplay.ReplayState()
		if recording.Running || recording.Stopping || recording.Finalizing || replay.Running || replay.Stopping || replay.Finalizing {
			fail(w, http.StatusConflict, errors.New("record or replay session already active"))
			return
		}
	}
	if a.performance.State().Running {
		fail(w, http.StatusConflict, errors.New("continuous performance session already active"))
		return
	}
	report, err := a.performance.MeasureStartup(r.Context(), request)
	if err != nil {
		if errors.Is(err, execution.ErrOwned) {
			fail(w, http.StatusConflict, err)
			return
		}
		fail(w, http.StatusUnprocessableEntity, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (a *API) startPerformanceSession(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Package         string `json:"package"`
		IntervalSeconds int    `json:"intervalSeconds,omitempty"`
		DurationSeconds int    `json:"durationSeconds,omitempty"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	a.executionMu.Lock()
	defer a.executionMu.Unlock()
	if a.shuttingDown {
		fail(w, http.StatusServiceUnavailable, errors.New("agent is shutting down"))
		return
	}
	if request.IntervalSeconds == 0 {
		request.IntervalSeconds = 1
	}
	if request.IntervalSeconds < 1 || request.IntervalSeconds > 60 || request.DurationSeconds < 0 || request.DurationSeconds > 604800 {
		fail(w, http.StatusBadRequest, errors.New("invalid performance interval or duration"))
		return
	}
	state, err := a.performance.StartConfig(r.Context(), perflog.Config{Package: request.Package, Interval: time.Duration(request.IntervalSeconds) * time.Second, Duration: time.Duration(request.DurationSeconds) * time.Second})
	if errors.Is(err, perflog.ErrRunning) {
		writeJSON(w, http.StatusConflict, state)
		return
	}
	if err != nil {
		if errors.Is(err, execution.ErrOwned) {
			fail(w, http.StatusConflict, err)
			return
		}
		fail(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusAccepted, state)
}

func (a *API) performanceSession(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, a.performance.State())
}

func (a *API) stopPerformanceSession(w http.ResponseWriter, r *http.Request) {
	a.executionMu.Lock()
	defer a.executionMu.Unlock()
	sessionID, ownerToken := executionCredentials(r)
	state, err := a.performance.StopOwned(r.Context(), sessionID, ownerToken)
	if err != nil {
		fail(w, executionStopErrorStatus(err), err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (a *API) StopPerformance(ctx context.Context) { _, _ = a.performance.Stop(ctx) }
