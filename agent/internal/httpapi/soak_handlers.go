package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"runtime"

	"github.com/zhoujun94511/xtest-nova/agent/internal/diagnosticsoak"
)

func (a *API) registerSoakRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /v1/diagnostics/soak/current", a.soakState)
	m.HandleFunc("POST /v1/diagnostics/soak", a.startSoak)
	m.HandleFunc("DELETE /v1/diagnostics/soak/current", a.stopSoak)
}

func (a *API) soakSample() any {
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	return map[string]any{"goroutines": runtime.NumGoroutine(), "heapAllocBytes": memory.HeapAlloc, "heapObjects": memory.HeapObjects, "systemBytes": memory.Sys, "gcCycles": memory.NumGC, "runner": a.runner != nil && a.runner.State().Running, "monitorReady": a.monitor != nil && a.monitor.Ready()}
}
func (a *API) soakState(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, a.soak.State())
}
func (a *API) startSoak(w http.ResponseWriter, r *http.Request) {
	var config diagnosticsoak.Config
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	state, err := a.soak.Start(config)
	if errors.Is(err, diagnosticsoak.ErrRunning) {
		writeJSON(w, http.StatusConflict, state)
		return
	}
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusAccepted, state)
}
func (a *API) stopSoak(w http.ResponseWriter, r *http.Request) {
	state, err := a.soak.Stop(r.Context())
	if err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}
func (a *API) StopSoak(ctx context.Context) error { _, err := a.soak.Stop(ctx); return err }
