package httpapi

import (
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	maxMultipartFileBytes    int64 = 512 << 20
	maxMultipartRequestBytes       = maxMultipartFileBytes + (1 << 20)
	multipartMemoryBytes           = 32 << 20
)

func (a *API) registerAppFileAutomationRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /packages", a.listPackages)
	m.HandleFunc("/packages/{pkg}/info", a.packageInfo)
	m.HandleFunc("POST /v1/apps/{pkg}/launch", a.launchPackage)
	m.HandleFunc("DELETE /v1/apps/{pkg}", a.uninstallPackage)
	m.HandleFunc("/installApk", a.installAPK)
	m.HandleFunc("/finfo/{path...}", a.fileInfo)
	m.HandleFunc("/raw/{path...}", a.rawFile)
	m.HandleFunc("GET /archive/{path...}", a.archiveDirectory)
	m.HandleFunc("/upload/{path...}", a.uploadFile)
	m.HandleFunc("/uiautomator", a.uiautomator)
	m.HandleFunc("/dump/hierarchy", a.hierarchy)
	m.HandleFunc("GET /v1/hierarchy/raw", a.rawHierarchy)
	m.HandleFunc("/dump/hierarchyWithScreenshot", a.hierarchyWithScreenshot)
	m.HandleFunc("/screenshot", a.screenshot)
}

func parseMultipartUpload(w http.ResponseWriter, r *http.Request) error {
	return parseMultipartUploadLimit(w, r, maxMultipartRequestBytes)
}

func parseMultipartUploadLimit(w http.ResponseWriter, r *http.Request, limit int64) error {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	return r.ParseMultipartForm(multipartMemoryBytes)
}

func multipartErrorStatus(err error) int {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return http.StatusRequestEntityTooLarge
	}
	return http.StatusBadRequest
}

type multipartUpload struct {
	file   multipart.File
	header *multipart.FileHeader
	form   *multipart.Form
}

func (u *multipartUpload) Close() error {
	return errors.Join(u.file.Close(), u.form.RemoveAll())
}

func openMultipartUpload(w http.ResponseWriter, r *http.Request) (*multipartUpload, error) {
	if err := parseMultipartUpload(w, r); err != nil {
		return nil, err
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		_ = r.MultipartForm.RemoveAll()
		return nil, err
	}
	return &multipartUpload{file: file, header: header, form: r.MultipartForm}, nil
}

func (a *API) listPackages(w http.ResponseWriter, r *http.Request) {
	scope := r.URL.Query().Get("scope")
	if scope == "" {
		if r.URL.Query().Get("system") == "true" {
			scope = "all"
		} else {
			scope = "third-party"
		}
	}
	if scope != "third-party" && scope != "system" && scope != "all" {
		fail(w, http.StatusBadRequest, errors.New("scope must be third-party, system, or all"))
		return
	}
	includeSystem := scope != "third-party"
	values, err := a.apps.List(r.Context(), includeSystem)
	if err != nil {
		fail(w, 500, err)
		return
	}
	if scope == "system" {
		filtered := values[:0]
		for _, value := range values {
			if value.System {
				filtered = append(filtered, value)
			}
		}
		values = filtered
	}
	writeJSON(w, 200, values)
}
func (a *API) packageInfo(w http.ResponseWriter, r *http.Request) {
	value, err := a.apps.Info(r.Context(), r.PathValue("pkg"))
	if err != nil {
		fail(w, 404, err)
		return
	}
	writeJSON(w, 200, map[string]any{"success": true, "data": value})
}
func (a *API) launchPackage(w http.ResponseWriter, r *http.Request) {
	output, err := a.apps.Launch(r.Context(), r.PathValue("pkg"))
	if err != nil {
		fail(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]any{"success": true, "output": output})
}
func (a *API) uninstallPackage(w http.ResponseWriter, r *http.Request) {
	output, err := a.apps.Uninstall(r.Context(), r.PathValue("pkg"), r.URL.Query().Get("keepData") == "true")
	if err != nil {
		fail(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]any{"success": true, "output": output})
}
func (a *API) installAPK(w http.ResponseWriter, r *http.Request) {
	upload, err := openMultipartUpload(w, r)
	if err != nil {
		fail(w, multipartErrorStatus(err), err)
		return
	}
	defer func() { _ = upload.Close() }()
	name := filepath.Base(upload.header.Filename)
	if !strings.HasSuffix(strings.ToLower(name), ".apk") {
		fail(w, 400, errors.New("file must be an APK"))
		return
	}
	target := "/data/local/tmp/apk/" + name
	entry, err := a.files.Save(target, upload.file, 0644)
	if err != nil {
		fail(w, 500, err)
		return
	}
	defer func() { _ = os.Remove(entry.Path) }()
	output, err := a.apps.Install(r.Context(), entry.Path, true)
	if err != nil {
		fail(w, 500, fmt.Errorf("%s: %w", output, err))
		return
	}
	writeJSON(w, 200, map[string]any{"success": true, "output": output, "totalBytes": entry.Size})
}
func (a *API) fileInfo(w http.ResponseWriter, r *http.Request) {
	entry, err := a.files.Info("/" + r.PathValue("path"))
	if err != nil {
		fail(w, httpStatusForFile(err), err)
		return
	}
	writeJSON(w, 200, entry)
}
func (a *API) rawFile(w http.ResponseWriter, r *http.Request) {
	file, info, err := a.files.Open("/" + r.PathValue("path"))
	if err != nil {
		fail(w, httpStatusForFile(err), err)
		return
	}
	defer func() { _ = file.Close() }()
	http.ServeContent(w, r, info.Name(), info.ModTime(), file)
}
func (a *API) uploadFile(w http.ResponseWriter, r *http.Request) {
	upload, err := openMultipartUpload(w, r)
	if err != nil {
		fail(w, multipartErrorStatus(err), err)
		return
	}
	defer func() { _ = upload.Close() }()
	target := "/" + r.PathValue("path")
	if strings.HasSuffix(target, "/") {
		target += filepath.Base(upload.header.Filename)
	}
	mode := uint64(0644)
	if value := r.FormValue("mode"); value != "" {
		if parsed, e := strconv.ParseUint(value, 8, 32); e == nil {
			mode = parsed
		} else {
			fail(w, 400, e)
			return
		}
	}
	// Save owns the exact per-file limit and reads one byte beyond it. Wrapping
	// file in another LimitReader here would hide an oversized upload and turn
	// truncation into a false success.
	entry, err := a.files.Save(target, upload.file, os.FileMode(mode))
	if err != nil {
		fail(w, httpStatusForFile(err), err)
		return
	}
	writeJSON(w, 200, map[string]any{"target": entry.Path, "isDir": false, "mode": fmt.Sprintf("0%o", mode)})
}
func httpStatusForFile(err error) int {
	if errors.Is(err, os.ErrNotExist) {
		return 404
	}
	if strings.Contains(err.Error(), "outside allowed roots") {
		return 403
	}
	if strings.Contains(err.Error(), "upload exceeds") {
		return http.StatusRequestEntityTooLarge
	}
	return 500
}
func (a *API) screenshot(w http.ResponseWriter, r *http.Request) {
	data, err := a.automation.Screenshot(r.Context())
	if err != nil {
		fail(w, 500, err)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(200)
	_, _ = w.Write(data)
}
func (a *API) hierarchy(w http.ResponseWriter, r *http.Request) {
	xml, err := a.automation.Hierarchy(r.Context())
	if err != nil {
		fail(w, 500, err)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/dump/") {
		writeJSON(w, 200, map[string]any{"jsonrpc": "2.0", "id": 1, "result": xml})
	} else {
		writeJSON(w, 200, map[string]any{"success": true, "hierarchy": xml})
	}
}
func (a *API) hierarchyWithScreenshot(w http.ResponseWriter, r *http.Request) {
	value, err := a.automation.HierarchyWithScreenshot(r.Context())
	if err != nil {
		fail(w, 500, err)
		return
	}
	writeJSON(w, 200, value)
}
func (a *API) uiautomator(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		writeJSON(w, 200, map[string]any{"running": a.automation.Running()})
	case "POST":
		if err := a.automation.Start(); err != nil {
			fail(w, 500, err)
			return
		}
		_, _ = w.Write([]byte("Successfully started"))
	case "DELETE":
		if err := a.automation.Stop(r.Context()); err != nil {
			fail(w, 500, err)
			return
		}
		_, _ = w.Write([]byte("Successfully stopped"))
	default:
		w.Header().Set("Allow", "GET, POST, DELETE")
		fail(w, 405, errors.New("method not allowed"))
	}
}
func (a *API) uiautomatorJSON(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "POST", "PUT":
		if err := a.automation.Start(); err != nil {
			fail(w, 500, err)
			return
		}
	case "DELETE":
		if err := a.automation.Stop(r.Context()); err != nil {
			fail(w, 500, err)
			return
		}
	case "GET":
	default:
		fail(w, 405, errors.New("method not allowed"))
		return
	}
	writeJSON(w, 200, map[string]any{"success": true, "running": a.automation.Running()})
}
