package httpapi

import (
	"errors"
	"net/http"
	"strings"
)

func (a *API) term(w http.ResponseWriter, r *http.Request) {
	if !a.unsafe {
		fail(w, http.StatusForbidden, errors.New("legacy terminal API disabled"))
		return
	}
	if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		a.terminal.ServeWebSocket(w, r)
		return
	}
	data, err := consoleFiles.ReadFile("web/terminal.html")
	if err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	setConsoleResponseHeaders(w)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'; connect-src 'self' ws: wss:")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(data)
}
