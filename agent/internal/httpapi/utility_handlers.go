package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

func (a *API) registerUtilityRoutes(m *http.ServeMux) {
	m.HandleFunc("POST /newCommandTimeout", a.newCommandTimeout)
	m.HandleFunc("/packages/{pkg}/icon", a.packageIcon)
	m.HandleFunc("POST /popupBoxAssistant", a.popupAssistant)
	m.HandleFunc("DELETE /popupBoxAssistant", a.popupAssistant)
	// Package icon is registered when its Android extraction backend is available.
}
func (a *API) packageIcon(w http.ResponseWriter, r *http.Request) {
	data, err := a.apps.Icon(r.Context(), r.PathValue("pkg"))
	if err != nil {
		data, _ = consoleFiles.ReadFile("web/assets/media/app-placeholder.svg")
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Header().Set("X-XTest-Icon-Fallback", "true")
	} else {
		w.Header().Set("Content-Type", "image/jpeg")
	}
	w.Header().Set("Cache-Control", "private, max-age=300")
	w.WriteHeader(200)
	_, _ = w.Write(data)
}
func (a *API) registerCompanionUtilityRoutes(m *http.ServeMux) {
	m.HandleFunc("/popupBoxAssistant", a.popupAssistant)
}

func (a *API) newCommandTimeout(w http.ResponseWriter, r *http.Request) {
	var seconds int
	if err := json.NewDecoder(r.Body).Decode(&seconds); err != nil {
		if requestErrorStatus(err) == http.StatusRequestEntityTooLarge {
			fail(w, http.StatusRequestEntityTooLarge, err)
		} else {
			fail(w, 400, errors.New("timeout must be a JSON integer in seconds"))
		}
		return
	}
	if seconds < 1 || seconds > 86400 {
		fail(w, 400, errors.New("command timeout must be between 1 and 86400 seconds"))
		return
	}
	duration := time.Duration(seconds) * time.Second
	if err := a.automation.SetCommandTimeout(duration); err != nil {
		fail(w, 400, err)
		return
	}
	writeJSON(w, 200, map[string]any{"success": true, "description": fmt.Sprintf("newCommandTimeout updated to %v", duration)})
}
func (a *API) popupAssistant(w http.ResponseWriter, r *http.Request) {
	a.executionMu.Lock()
	defer a.executionMu.Unlock()
	switch r.Method {
	case http.MethodGet:
		state := a.popup.State()
		writeJSON(w, 200, map[string]any{"success": true, "running": state.Running, "stopping": state.Stopping})
	case http.MethodPost:
		if a.shuttingDown {
			fail(w, http.StatusServiceUnavailable, errors.New("agent is shutting down"))
			return
		}
		state, err := a.popup.Start()
		if err != nil {
			fail(w, 500, err)
			return
		}
		writeJSON(w, 200, map[string]any{"success": true, "running": state.Running, "stopping": state.Stopping})
	case http.MethodDelete:
		state, err := a.popup.Stop()
		if err != nil {
			fail(w, http.StatusGatewayTimeout, err)
			return
		}
		writeJSON(w, 200, map[string]any{"success": true, "running": state.Running, "stopping": state.Stopping})
	default:
		w.Header().Set("Allow", "GET, POST, DELETE")
		fail(w, 405, errors.New("method not allowed"))
	}
}
