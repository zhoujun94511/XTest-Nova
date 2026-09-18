package httpapi

import (
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/zhoujun94511/xtest-nova/agent/internal/componenthealth"
	"github.com/zhoujun94511/xtest-nova/agent/internal/scrcpy"
)

func (a *API) registerDiagnosticsRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /v1/diagnostics/components", a.componentDiagnostics)
	m.HandleFunc("GET /v1/diagnostics/artifacts", a.artifactDiagnostics)
	m.HandleFunc("GET /v1/diagnostics/logs/{name}", a.logDiagnostics)
}

func (a *API) componentDiagnostics(w http.ResponseWriter, _ *http.Request) {
	components := []componenthealth.Status{
		componenthealth.New("agent", true, true, true, Version, ""),
		componenthealth.New("uiautomator", a.automation != nil, a.automation != nil && a.automation.Running(), a.automation != nil && a.automation.Running(), "", "provider starts on demand"),
		componenthealth.New("scrcpy", a.scrcpy != nil, a.scrcpy != nil, false, scrcpy.ServerVersion, "embedded payload verified when a session starts"),
	}
	if a.monitor != nil {
		ready := a.monitor.Ready()
		status := componenthealth.New("monitor", true, ready, ready, "1", "")
		if !ready {
			status = componenthealth.Degraded("monitor", true, "1", "listener unavailable")
		}
		components = append(components, status)
	}
	if a.minitouch != nil {
		state := a.minitouch.State()
		components = append(components, componenthealth.New("minitouch", true, state.Running, state.Running, "", state.Error))
	}
	if a.popup != nil {
		state := a.popup.State()
		components = append(components, componenthealth.New("autoPopup", true, state.Running && !state.Stopping, state.Running, "", state.LastError))
	}
	if a.runner != nil {
		state := a.runner.State()
		components = append(components, componenthealth.New("runnerSession", true, !state.Finalizing, state.Running || state.Finalizing, "", state.Error))
	}
	if a.explorer != nil {
		state := a.explorer.State()
		busy := state.Running || state.Stopping || state.Finalizing
		components = append(components, componenthealth.New("explorationSession", true, !state.Finalizing, busy, "", state.Error))
	}
	if a.recorder != nil {
		running := a.recorder.Running()
		components = append(components, componenthealth.New("screenRecord", true, true, running, "", ""))
	}
	if a.components != nil {
		components = mergeComponentStatuses(components, a.components.Snapshot())
	}
	sort.Slice(components, func(i, j int) bool { return components[i].Name < components[j].Name })
	degraded := false
	for _, component := range components {
		if component.Supported && !component.Ready && component.State == "degraded" {
			degraded = true
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"schemaVersion": "xtest-nova-components/v1", "degraded": degraded, "components": components})
}

func (a *API) artifactDiagnostics(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			fail(w, http.StatusBadRequest, errors.New("limit must be an integer"))
			return
		}
		limit = value
	}
	sessions, err := a.artifactIndex.List(strings.TrimSpace(r.URL.Query().Get("package")), strings.TrimSpace(r.URL.Query().Get("kind")), limit)
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"schemaVersion": "xtest-nova-artifacts/v1", "sessions": sessions})
}

func (a *API) logDiagnostics(w http.ResponseWriter, r *http.Request) {
	limit := 200
	if raw := r.URL.Query().Get("lines"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			fail(w, http.StatusBadRequest, errors.New("lines must be an integer"))
			return
		}
		limit = value
	}
	result, err := a.logs.Tail(r.PathValue("name"), limit, r.URL.Query().Get("contains"))
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func mergeComponentStatuses(base, overrides []componenthealth.Status) []componenthealth.Status {
	index := map[string]int{}
	for position, status := range base {
		index[status.Name] = position
	}
	for _, status := range overrides {
		if position, ok := index[status.Name]; ok {
			base[position] = status
		} else {
			index[status.Name] = len(base)
			base = append(base, status)
		}
	}
	return base
}
