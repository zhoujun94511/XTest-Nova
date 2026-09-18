package httpapi

import (
	"errors"
	"net/http"
)

func (a *API) registerScrcpyRoutes(m *http.ServeMux) {
	m.HandleFunc("/scrcpy/{type}/{definition}", a.scrcpyWebSocket)
}

func (a *API) scrcpyWebSocket(w http.ResponseWriter, r *http.Request) {
	if a.scrcpy == nil {
		fail(w, http.StatusServiceUnavailable, errors.New("scrcpy bridge unavailable"))
		return
	}
	a.scrcpy.ServeWebSocket(w, r, r.PathValue("type"), r.PathValue("definition"))
}
