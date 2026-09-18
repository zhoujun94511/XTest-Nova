package httpapi

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/zhoujun94511/xtest-nova/agent/internal/touchreader"
)

//go:embed web
var consoleFiles embed.FS

func setConsoleResponseHeaders(w http.ResponseWriter) {
	w.Header().Set("Referrer-Policy", "no-referrer")
}

func (a *API) registerWebConsoleRoutes(m *http.ServeMux) {
	m.HandleFunc("/jsonrpc/0", a.jsonRPCProxy)
	m.HandleFunc("/touchreader", touchreader.ServeWebSocket)
	m.HandleFunc("GET /assets/{path...}", a.webAsset)
	m.HandleFunc("GET /favicon.ico", a.webFavicon)
	m.HandleFunc("GET /static/js/{path...}", a.webAsset)
	m.HandleFunc("GET /static/css/{path...}", a.webAsset)
	m.HandleFunc("GET /static/media/{path...}", a.webAsset)
	m.HandleFunc("GET /app.css", a.webAsset)
	m.HandleFunc("GET /app.js", a.webAsset)
	m.HandleFunc("GET /terminal.css", a.webAsset)
	m.HandleFunc("GET /terminal.js", a.webAsset)
	m.HandleFunc("GET /placeholder.svg", a.webAsset)
	m.HandleFunc("/", a.webConsole)
}
func (a *API) webFavicon(w http.ResponseWriter, _ *http.Request) {
	setConsoleResponseHeaders(w)
	data, err := consoleFiles.ReadFile("web/assets/media/favicon.svg")
	if err != nil {
		http.Error(w, "favicon unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(data)
}
func (a *API) jsonRPCProxy(w http.ResponseWriter, r *http.Request) {
	if !a.legacyUiAutomator {
		http.NotFound(w, r)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, (4<<20)+1))
	if err != nil {
		fail(w, 400, err)
		return
	}
	if len(body) > 4<<20 {
		fail(w, http.StatusRequestEntityTooLarge, errors.New("JSON-RPC request exceeds 4 MiB"))
		return
	}
	request, err := http.NewRequestWithContext(r.Context(), r.Method, "http://127.0.0.1:9008/jsonrpc/0", bytes.NewReader(body))
	if err != nil {
		fail(w, 400, err)
		return
	}
	if contentType := r.Header.Get("Content-Type"); contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	a.automation.ResetCommandTimeout()
	client := http.Client{Timeout: 45 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		fail(w, http.StatusBadGateway, err)
		return
	}
	defer func() { _ = response.Body.Close() }()
	writeProxiedResponse(w, response, 16<<20)
}

func writeProxiedResponse(w http.ResponseWriter, response *http.Response, limit int64) {
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		fail(w, http.StatusBadGateway, fmt.Errorf("read JSON-RPC response: %w", err))
		return
	}
	if int64(len(body)) > limit {
		fail(w, http.StatusBadGateway, errors.New("JSON-RPC response exceeds 16 MiB"))
		return
	}
	if contentType := response.Header.Get("Content-Type"); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.WriteHeader(response.StatusCode)
	_, _ = w.Write(body)
}
func (a *API) webAsset(w http.ResponseWriter, r *http.Request) {
	setConsoleResponseHeaders(w)
	requestPath := strings.TrimPrefix(path.Clean("/"+r.PathValue("path")), "/")
	var files []string
	switch {
	case strings.HasPrefix(r.URL.Path, "/assets/") && requestPath != ".":
		files = []string{"web/assets/" + requestPath}
	case path.Base(r.URL.Path) == "app.css":
		files = []string{"web/assets/css/foundation.css", "web/assets/css/shell.css", "web/assets/css/components.css"}
	case path.Base(r.URL.Path) == "app.js":
		files = []string{"web/assets/js/core.js", "web/assets/js/renderers.js", "web/assets/js/package-selector.js", "web/assets/js/app.js"}
	case path.Base(r.URL.Path) == "terminal.css":
		files = []string{"web/assets/css/terminal.css"}
	case path.Base(r.URL.Path) == "terminal.js":
		files = []string{"web/assets/js/terminal.js"}
	case path.Base(r.URL.Path) == "placeholder.svg":
		files = []string{"web/assets/media/placeholder.svg"}
	}
	if len(files) == 0 || strings.Contains(requestPath, "..") {
		http.NotFound(w, r)
		return
	}
	var data []byte
	for _, name := range files {
		content, err := consoleFiles.ReadFile(name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		data = append(data, content...)
		data = append(data, '\n')
	}
	if contentType := mime.TypeByExtension(path.Ext(r.URL.Path)); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	// The console is embedded in the Agent binary. Revalidate assets after an
	// in-place,Agent upgrade so a browser never mixes an old script with new HTML.
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(data)
}
func (a *API) webConsole(w http.ResponseWriter, r *http.Request) {
	setConsoleResponseHeaders(w)
	if r.URL.Path != "/index.html" && strings.Contains(path.Base(r.URL.Path), ".") {
		http.NotFound(w, r)
		return
	}
	data, err := consoleFiles.ReadFile("web/index.html")
	if err != nil {
		fail(w, 500, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' blob:; style-src 'self'; script-src 'self'; connect-src 'self'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(data)
}
