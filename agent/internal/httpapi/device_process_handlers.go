package httpapi

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/zhoujun94511/xtest-nova/agent/internal/command"
	"github.com/zhoujun94511/xtest-nova/agent/internal/legacyexec"
)

func (a *API) registerDeviceProcessRoutes(m *http.ServeMux) {
	m.HandleFunc("/proc/list", a.processList)
	m.HandleFunc("/pidof/{pkg}", a.pidOf)
	m.HandleFunc("/proc/{pkg}/meminfo", a.appMemory)
	m.HandleFunc("/proc/{pkg}/meminfo/all", a.appMemoryAll)
	m.HandleFunc("/proc/{pkg}/cpuinfo", a.appCPU)
	m.HandleFunc("/proc/{pkg}/perf", a.appPerformance)
	m.HandleFunc("/device/memory", a.deviceMemory)
	m.HandleFunc("/network/info", a.networkInfo)
	m.HandleFunc("/disk/info", a.diskInfo)
	m.HandleFunc("/services/{name}", a.service)
	m.HandleFunc("/shell/background", a.backgroundShell)
	m.HandleFunc("/imeStatus", a.imeStatus)
	m.HandleFunc("/setIme", a.setIME)
	m.HandleFunc("/u2packages", a.u2Packages)
	m.HandleFunc("/installAgentApk", a.installCompanion)
	m.HandleFunc("/installLocalApk/{apk}", a.installLocalAPK)
}
func (a *API) registerCompanionDeviceRoutes(m *http.ServeMux) {
	m.HandleFunc("/network/info", a.networkInfo)
	m.HandleFunc("/disk/info", a.diskInfo)
	m.HandleFunc("/imeStatus", a.imeStatus)
	m.HandleFunc("/setIme", a.setIME)
	m.HandleFunc("/u2packages", a.u2Packages)
}
func (a *API) processList(w http.ResponseWriter, _ *http.Request) {
	values, err := a.system.Processes()
	if err != nil {
		fail(w, 500, err)
		return
	}
	writeJSON(w, 200, values)
}
func (a *API) pidOf(w http.ResponseWriter, r *http.Request) {
	pid, err := a.system.PID(r.Context(), r.PathValue("pkg"))
	if err != nil {
		fail(w, http.StatusGone, err)
		return
	}
	_, _ = fmt.Fprint(w, pid)
}
func (a *API) appMemory(w http.ResponseWriter, r *http.Request) {
	value, err := a.system.AppMemory(r.Context(), r.PathValue("pkg"))
	if err != nil {
		fail(w, 500, err)
		return
	}
	writeJSON(w, 200, value)
}
func (a *API) appMemoryAll(w http.ResponseWriter, r *http.Request) {
	prefix := r.PathValue("pkg")
	processes, err := a.system.Processes()
	if err != nil {
		fail(w, 500, err)
		return
	}
	result := map[string]map[string]int{}
	for _, process := range processes {
		if process.Name == prefix || strings.HasPrefix(process.Name, prefix+":") {
			if value, memoryErr := a.system.AppMemory(r.Context(), process.Name); memoryErr == nil {
				result[process.Name] = value
			}
		}
	}
	writeJSON(w, 200, result)
}
func (a *API) appCPU(w http.ResponseWriter, r *http.Request) {
	value, err := a.system.CPU(r.Context(), r.PathValue("pkg"))
	if err != nil {
		fail(w, 500, err)
		return
	}
	writeJSON(w, 200, value)
}
func (a *API) appPerformance(w http.ResponseWriter, r *http.Request) {
	value, err := a.system.Performance(r.Context(), r.PathValue("pkg"))
	if err != nil {
		fail(w, 500, err)
		return
	}
	writeJSON(w, 200, value)
}
func (a *API) deviceMemory(w http.ResponseWriter, _ *http.Request) {
	value, err := a.system.DeviceMemory()
	if err != nil {
		fail(w, 500, err)
		return
	}
	writeJSON(w, 200, value)
}
func (a *API) networkInfo(w http.ResponseWriter, r *http.Request) {
	value, err := a.system.NetworkStatus(r.Context())
	if err != nil {
		fail(w, 500, err)
		return
	}
	writeJSON(w, 200, value)
}
func (a *API) diskInfo(w http.ResponseWriter, r *http.Request) {
	value, err := a.system.Storage(r.Context())
	if err != nil {
		fail(w, 500, err)
		return
	}
	writeJSON(w, 200, value)
}
func (a *API) service(w http.ResponseWriter, r *http.Request) {
	if r.PathValue("name") != "uiautomator" {
		fail(w, 400, fmt.Errorf("service %q does not exist", r.PathValue("name")))
		return
	}
	switch r.Method {
	case "GET":
		writeJSON(w, 200, map[string]any{"success": true, "running": a.automation.Running()})
	case "POST":
		a.executionMu.Lock()
		defer a.executionMu.Unlock()
		if a.shuttingDown {
			fail(w, http.StatusServiceUnavailable, errors.New("agent is shutting down"))
			return
		}
		if err := a.automation.Start(); err != nil {
			fail(w, 500, err)
			return
		}
		writeJSON(w, 200, map[string]any{"success": true, "description": "successfully started"})
	case "DELETE":
		if err := a.automation.Stop(r.Context()); err != nil {
			fail(w, 500, err)
			return
		}
		writeJSON(w, 200, map[string]any{"success": true, "description": "successfully stopped"})
	default:
		fail(w, 405, errors.New("method not allowed"))
	}
}
func (a *API) backgroundShell(w http.ResponseWriter, r *http.Request) {
	if !a.unsafe {
		fail(w, http.StatusForbidden, errors.New("legacy background shell API disabled"))
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		w.Header().Set("Allow", "GET, POST")
		fail(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	if err := r.ParseForm(); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	shellCommand := r.FormValue("command")
	if shellCommand == "" {
		shellCommand = r.FormValue("c")
	}
	if shellCommand == "" && strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		var body struct {
			Command string `json:"command"`
			C       string `json:"c"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			fail(w, http.StatusBadRequest, err)
			return
		}
		shellCommand = body.Command
		if shellCommand == "" {
			shellCommand = body.C
		}
	}
	if strings.TrimSpace(shellCommand) == "" {
		fail(w, http.StatusBadRequest, errors.New("command required"))
		return
	}
	if len(shellCommand) > 16<<10 {
		fail(w, http.StatusRequestEntityTooLarge, errors.New("command exceeds 16 KiB"))
		return
	}
	a.executionMu.Lock()
	if a.shuttingDown {
		a.executionMu.Unlock()
		fail(w, http.StatusServiceUnavailable, errors.New("agent is shutting down"))
		return
	}
	pid, err := a.background.Start(shellCommand)
	a.executionMu.Unlock()
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, legacyexec.ErrBackgroundLimit) {
			status = http.StatusTooManyRequests
		}
		writeJSON(w, status, map[string]any{"success": false, "description": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success":     true,
		"pid":         pid,
		"description": fmt.Sprintf("Successfully started program: %v", shellCommand),
	})
}
func (a *API) imeStatus(w http.ResponseWriter, r *http.Request) {
	value, err := a.system.IME(r.Context())
	if err != nil {
		fail(w, 500, err)
		return
	}
	writeJSON(w, 200, value)
}
func requestedIME(r *http.Request) string {
	for _, key := range []string{"ime", "id", "inputMethod"} {
		if value := strings.TrimSpace(r.FormValue(key)); value != "" {
			return value
		}
	}
	var body map[string]any
	if json.NewDecoder(r.Body).Decode(&body) == nil {
		for _, key := range []string{"ime", "id", "inputMethod"} {
			if value, ok := body[key].(string); ok {
				return strings.TrimSpace(value)
			}
		}
	}
	return ""
}
func (a *API) setIME(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" && r.Method != "PUT" {
		fail(w, 405, errors.New("method not allowed"))
		return
	}
	value := requestedIME(r)
	output, err := a.system.SetIME(r.Context(), value)
	if err != nil {
		fail(w, 400, err)
		return
	}
	status, _ := a.system.IME(r.Context())
	writeJSON(w, 200, map[string]any{"success": true, "ime": status.Current, "output": output})
}
func (a *API) u2Packages(w http.ResponseWriter, r *http.Request) {
	values := make([]any, 0, 3)
	for _, name := range []string{"com.github.uiautomator", "com.github.uiautomator.test", "com.utest.agent"} {
		if value, err := a.apps.Info(r.Context(), name); err == nil {
			values = append(values, map[string]any{"packageName": value.PackageName, "versionName": value.VersionName, "versionCode": value.VersionCode})
		}
	}
	writeJSON(w, 200, map[string]any{"packages": values, "running": a.automation.Running()})
}
func (a *API) installCompanion(w http.ResponseWriter, r *http.Request) {
	if value, err := a.apps.Info(r.Context(), command.PopupPackage); err == nil && value.VersionCode == command.PopupVersionCode && sameFileContent(value.APKPath, a.companionAPK) {
		writeJSON(w, 200, map[string]any{"success": true, "installed": true, "versionCode": value.VersionCode})
		return
	}
	if _, err := os.Stat(a.companionAPK); err != nil {
		fail(w, 503, fmt.Errorf("companion APK is not staged: %w", err))
		return
	}
	output, err := a.apps.Install(r.Context(), a.companionAPK, true)
	if err != nil {
		fail(w, 500, fmt.Errorf("%s: %w", output, err))
		return
	}
	writeJSON(w, 200, map[string]any{"success": true, "installed": true, "output": output})
}

func sameFileContent(first, second string) bool {
	left, leftErr := os.Open(first)
	if leftErr != nil {
		return false
	}
	defer func() { _ = left.Close() }()
	right, rightErr := os.Open(second)
	if rightErr != nil {
		return false
	}
	defer func() { _ = right.Close() }()
	leftHash, rightHash := sha256.New(), sha256.New()
	if _, err := io.Copy(leftHash, left); err != nil {
		return false
	}
	if _, err := io.Copy(rightHash, right); err != nil {
		return false
	}
	return bytes.Equal(leftHash.Sum(nil), rightHash.Sum(nil))
}
func (a *API) installLocalAPK(w http.ResponseWriter, r *http.Request) {
	name := filepath.Base(r.PathValue("apk"))
	if name != r.PathValue("apk") || !strings.HasSuffix(strings.ToLower(name), ".apk") {
		fail(w, 400, errors.New("invalid APK name"))
		return
	}
	path := "/data/local/tmp/apk/" + name
	if _, err := os.Stat(path); err != nil {
		fail(w, 404, err)
		return
	}
	defer func() { _ = os.Remove(path) }()
	output, err := a.apps.Install(r.Context(), path, true)
	if err != nil {
		fail(w, 500, fmt.Errorf("%s: %w", output, err))
		return
	}
	writeJSON(w, 200, map[string]any{"success": true, "output": output})
}
