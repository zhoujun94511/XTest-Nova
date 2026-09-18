package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/zhoujun94511/xtest-nova/agent/internal/runner"
	"github.com/zhoujun94511/xtest-nova/agent/internal/tasks"
)

func (a *API) registerTaskSessionRoutes(m *http.ServeMux) {
	m.HandleFunc("POST /download", a.startDownload)
	m.HandleFunc("/download/{id}", a.downloadState)
	m.HandleFunc("POST /packages", a.startPackageInstall)
	m.HandleFunc("/packages/{id}", a.packageTask)
	m.HandleFunc("POST /install", a.startLegacyInstall)
	m.HandleFunc("GET /install/{id}", a.installState)
	m.HandleFunc("DELETE /install/{id}", a.cancelInstall)
	m.HandleFunc("POST /session/{pkg}", a.session)
	m.HandleFunc("/webviews", a.webViews)
	m.HandleFunc("/webviews/{pkg}", a.packageWebViews)
	m.HandleFunc("/wlan/ip", a.wlanIP)
	m.HandleFunc("/screenshot/0", a.screenshot)
	m.HandleFunc("/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			fail(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
			return
		}
		a.stopServices(w, r)
	})
	m.HandleFunc("/minitouch", a.minitouchLifecycle)
}
func (a *API) startDownload(w http.ResponseWriter, r *http.Request) {
	a.executionMu.Lock()
	defer a.executionMu.Unlock()
	if a.shuttingDown {
		fail(w, http.StatusServiceUnavailable, errors.New("agent is shutting down"))
		return
	}
	destination, err := a.files.ResolveForWrite(r.FormValue("filepath"))
	if err != nil {
		fail(w, 400, err)
		return
	}
	mode := os.FileMode(0644)
	if raw := r.FormValue("mode"); raw != "" {
		value, parseErr := strconv.ParseUint(raw, 8, 32)
		if parseErr != nil {
			fail(w, 400, parseErr)
			return
		}
		mode = os.FileMode(value)
	}
	id, err := a.tasks.Start(r.FormValue("url"), destination, mode, nil, false)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, tasks.ErrTaskLimit) {
			status = http.StatusTooManyRequests
		}
		fail(w, status, err)
		return
	}
	_, _ = w.Write([]byte(id))
}
func (a *API) downloadState(w http.ResponseWriter, r *http.Request) {
	state, ok := a.tasks.Get(r.PathValue("id"))
	if !ok {
		fail(w, 404, errors.New("download task not found"))
		return
	}
	writeJSON(w, 200, state)
}
func (a *API) newInstallTask(r *http.Request) (string, error) {
	destination := fmt.Sprintf("/data/local/tmp/xtest-nova-downloads/%d.apk", time.Now().UnixNano())
	return a.tasks.Start(r.FormValue("url"), destination, 0644, func(ctx context.Context, path string) error {
		output, err := a.apps.Install(ctx, path, true)
		if err != nil {
			return fmt.Errorf("%s: %w", output, err)
		}
		return nil
	}, true)
}
func (a *API) startPackageInstall(w http.ResponseWriter, r *http.Request) {
	a.startInstallTask(w, r, func(id string) {
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": map[string]string{"id": id}})
	})
}
func (a *API) startLegacyInstall(w http.ResponseWriter, r *http.Request) {
	a.startInstallTask(w, r, func(id string) { _, _ = w.Write([]byte(id)) })
}
func (a *API) startInstallTask(w http.ResponseWriter, r *http.Request, respond func(string)) {
	a.executionMu.Lock()
	defer a.executionMu.Unlock()
	if a.shuttingDown {
		fail(w, http.StatusServiceUnavailable, errors.New("agent is shutting down"))
		return
	}
	id, err := a.newInstallTask(r)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, tasks.ErrTaskLimit) {
			status = http.StatusTooManyRequests
		}
		fail(w, status, err)
		return
	}
	respond(id)
}
func (a *API) packageTask(w http.ResponseWriter, r *http.Request) {
	state, ok := a.tasks.Get(r.PathValue("id"))
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"success": false, "description": "package task not found"})
		return
	}
	description, _ := json.Marshal(map[string]any{"totalSize": state.TotalSize, "copiedSize": state.CopiedSize, "error": state.Error})
	writeJSON(w, 200, map[string]any{"success": true, "data": map[string]string{"status": state.Status, "description": string(description)}})
}
func (a *API) installState(w http.ResponseWriter, r *http.Request) {
	state, ok := a.tasks.Get(r.PathValue("id"))
	if !ok {
		fail(w, 404, errors.New("install task not found"))
		return
	}
	writeJSON(w, 200, state)
}
func (a *API) cancelInstall(w http.ResponseWriter, r *http.Request) {
	if a.tasks.Cancel(r.PathValue("id")) {
		_, _ = w.Write([]byte("Cancelled"))
		return
	}
	_, _ = w.Write([]byte("Unable to cancel"))
}
func (a *API) session(w http.ResponseWriter, r *http.Request) {
	info, output, err := a.apps.Session(r.Context(), r.PathValue("pkg"))
	if err != nil {
		writeJSON(w, 500, map[string]any{"success": false, "error": err.Error(), "output": output})
		return
	}
	writeJSON(w, 200, map[string]any{"success": true, "output": output, "mainActivity": info.MainActivity})
}
func (a *API) webViews(w http.ResponseWriter, _ *http.Request) {
	values, err := a.system.WebViews("")
	if err != nil {
		fail(w, 500, err)
		return
	}
	writeJSON(w, 200, values)
}
func (a *API) packageWebViews(w http.ResponseWriter, r *http.Request) {
	values, err := a.system.WebViews(r.PathValue("pkg"))
	if err != nil {
		fail(w, 500, err)
		return
	}
	writeJSON(w, 200, values)
}
func (a *API) wlanIP(w http.ResponseWriter, r *http.Request) {
	value, err := a.system.WLANIP(r.Context())
	if err != nil {
		fail(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]string{"ip": value})
}
func (a *API) stopServices(w http.ResponseWriter, r *http.Request) {
	if !requireControlHeader(w, r) {
		return
	}
	a.executionMu.Lock()
	defer a.executionMu.Unlock()
	if a.shuttingDown {
		fail(w, http.StatusServiceUnavailable, errors.New("agent is already shutting down"))
		return
	}
	a.shuttingDown = true
	var explorerErr error
	if a.explorer != nil {
		_, explorerErr = a.explorer.Stop(r.Context())
	}
	runnerErr, runnerFinalizeErr := a.stopRunnerForShutdown()
	automationErr := a.automation.Stop(r.Context())
	_, touchErr := a.minitouch.Stop()
	_, popupErr := a.popup.Stop()
	_, recorderErr := a.recorder.Stop()
	_, performanceErr := a.performance.Stop(r.Context())
	backgroundErr := a.background.StopAll(r.Context())
	if runnerErr != nil || runnerFinalizeErr != nil || automationErr != nil || touchErr != nil || popupErr != nil || recorderErr != nil || explorerErr != nil || performanceErr != nil || backgroundErr != nil {
		a.shuttingDown = false
		fail(w, 500, fmt.Errorf("runner=%v runnerFinalization=%v automation=%v minitouch=%v popup=%v screenrecord=%v exploration=%v performance=%v background=%v", runnerErr, runnerFinalizeErr, automationErr, touchErr, popupErr, recorderErr, explorerErr, performanceErr, backgroundErr))
		return
	}
	_, _ = w.Write([]byte("Finished!"))
	a.requestShutdown()
}

func (a *API) stopRunnerForShutdown() (error, error) {
	if _, err := a.runner.Stop(); err != nil {
		return err, nil
	}
	finalizeContext, cancelFinalize := context.WithTimeout(context.Background(), runner.FinalizationTimeout)
	defer cancelFinalize()
	_, err := a.runner.WaitFinalized(finalizeContext)
	return nil, err
}
func (a *API) minitouchLifecycle(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "PUT":
		a.executionMu.Lock()
		defer a.executionMu.Unlock()
		if a.shuttingDown {
			fail(w, http.StatusServiceUnavailable, errors.New("agent is shutting down"))
			return
		}
		state, err := a.minitouch.Start()
		if err != nil {
			fail(w, 503, err)
			return
		}
		writeJSON(w, 200, state)
	case "DELETE":
		state, err := a.minitouch.Stop()
		if err != nil {
			fail(w, 500, err)
			return
		}
		writeJSON(w, 200, state)
	case "GET":
		a.minitouch.ServeWebSocket(w, r)
	default:
		fail(w, 405, errors.New("method not allowed"))
	}
}
