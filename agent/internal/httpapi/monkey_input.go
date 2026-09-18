package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/zhoujun94511/xtest-nova/agent/internal/runner"
)

type monkeyControlAuthorizer interface {
	AuthorizeControlToken(string) (runner.State, bool)
}

func (a *API) runMonkeyInputText(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	var request struct {
		Token string `json:"token"`
		Text  string `json:"text"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	if !utf8.ValidString(request.Text) || len(request.Text) > 4096 || strings.ContainsRune(request.Text, '\x00') {
		fail(w, http.StatusBadRequest, errors.New("invalid UTF-8 text"))
		return
	}
	authorizer, ok := a.runner.(monkeyControlAuthorizer)
	if !ok {
		fail(w, http.StatusServiceUnavailable, errors.New("runner control authorization unavailable"))
		return
	}
	a.executionMu.Lock()
	defer a.executionMu.Unlock()
	inputContext, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	state, authorized := authorizer.AuthorizeControlToken(request.Token)
	if !authorized {
		fail(w, http.StatusForbidden, errors.New("invalid or inactive Runner control token"))
		return
	}
	foreground, err := a.device.ForegroundPackage(inputContext)
	if err != nil {
		fail(w, http.StatusConflict, err)
		return
	}
	if foreground != state.Package {
		fail(w, http.StatusConflict, errors.New("runner target package is not foreground"))
		return
	}
	if a.scrcpy == nil {
		fail(w, http.StatusServiceUnavailable, errors.New("scrcpy text injection unavailable"))
		return
	}
	if err := a.scrcpy.InjectText(inputContext, request.Text); err != nil {
		fail(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}
