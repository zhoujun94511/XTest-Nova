package httpapi

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/zhoujun94511/xtest-nova/agent/internal/execution"
	"github.com/zhoujun94511/xtest-nova/agent/internal/recordreplay"
)

func decodeRecordReplayJSON(w http.ResponseWriter, r *http.Request, value any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		fail(w, http.StatusBadRequest, err)
		return false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			err = errors.New("request must contain one JSON value")
		}
		fail(w, http.StatusBadRequest, err)
		return false
	}
	return true
}

func (a *API) registerRecordReplayRoutes(m *http.ServeMux) {
	m.HandleFunc("POST /v1/recordings", a.startRecording)
	m.HandleFunc("GET /v1/recordings", a.recordingCases)
	m.HandleFunc("GET /v1/recordings/cases/{id}", a.savedRecordingCase)
	m.HandleFunc("DELETE /v1/recordings/cases/{id}", a.deleteSavedRecordingCase)
	m.HandleFunc("GET /v1/recordings/drafts", a.recordingDrafts)
	m.HandleFunc("POST /v1/recordings/drafts/{id}/finalize", a.finalizeRecordingDraft)
	m.HandleFunc("DELETE /v1/recordings/drafts/{id}", a.deleteRecordingDraft)
	m.HandleFunc("GET /v1/recordings/current", a.recordingState)
	m.HandleFunc("DELETE /v1/recordings/current", a.stopRecording)
	m.HandleFunc("GET /v1/recordings/current/case", a.recordingCase)
	m.HandleFunc("POST /v1/recordings/current/text", a.appendRecordingText)
	m.HandleFunc("POST /v1/recordings/current/focused-text", a.appendFocusedRecordingText)
	m.HandleFunc("POST /v1/recordings/current/key", a.appendRecordingKey)
	m.HandleFunc("POST /v1/recordings/current/assertions/screenshot", a.appendScreenshotAssertion)
	m.HandleFunc("PUT /v1/recordings/current/excluded-bounds", a.updateRecordingExcludedBounds)
	m.HandleFunc("POST /v1/replays", a.startReplay)
	m.HandleFunc("POST /v1/replays/validate", a.validateReplay)
	m.HandleFunc("GET /v1/replays/current", a.replayState)
	m.HandleFunc("DELETE /v1/replays/current", a.stopReplay)
}

type hierarchyNode struct {
	Text            string          `xml:"text,attr"`
	Class           string          `xml:"class,attr"`
	ResourceID      string          `xml:"resource-id,attr"`
	Description     string          `xml:"content-desc,attr"`
	InputType       string          `xml:"input-type,attr"`
	LegacyInputType string          `xml:"inputType,attr"`
	Focused         bool            `xml:"focused,attr"`
	Password        bool            `xml:"password,attr"`
	Bounds          string          `xml:"bounds,attr"`
	Children        []hierarchyNode `xml:"node"`
}

var hierarchyBoundsPattern = regexp.MustCompile(`^\[(\d+),(\d+)]\[(\d+),(\d+)]$`)
var sensitiveFocusedFieldTerms = []string{
	"password", "passwd", "passcode", "pin", "otp", "verification code", "security code", "cvv", "card number", "bank account", "payment", "secret", "token",
	"密码", "口令", "验证码", "校验码", "动态码", "支付", "银行卡", "卡号", "账户", "账号",
}

func sensitiveFocusedField(node hierarchyNode) bool {
	if node.Password {
		return true
	}
	candidate := strings.ToLower(strings.Join([]string{node.ResourceID, node.Description, node.InputType, node.LegacyInputType}, " "))
	for _, term := range sensitiveFocusedFieldTerms {
		if strings.Contains(candidate, term) {
			return true
		}
	}
	return node.Text != "" && strings.Trim(node.Text, "*•●· ") == ""
}

func focusedText(xmlDocument string, width, height int) (string, recordreplay.Point, error) {
	var root hierarchyNode
	if err := xml.Unmarshal([]byte(xmlDocument), &root); err != nil {
		return "", recordreplay.Point{}, err
	}
	var visit func(hierarchyNode) (string, recordreplay.Point, bool, error)
	visit = func(node hierarchyNode) (string, recordreplay.Point, bool, error) {
		if node.Focused && strings.Contains(node.Class, "EditText") {
			if sensitiveFocusedField(node) {
				return "", recordreplay.Point{}, false, errors.New("password fields cannot be recorded")
			}
			match := hierarchyBoundsPattern.FindStringSubmatch(node.Bounds)
			if len(match) != 5 || width < 1 || height < 1 {
				return "", recordreplay.Point{}, false, errors.New("focused input bounds unavailable")
			}
			values := make([]int, 4)
			for index := range values {
				values[index], _ = strconv.Atoi(match[index+1])
			}
			point := recordreplay.Point{X: float64(values[0]+values[2]) / 2 / float64(width), Y: float64(values[1]+values[3]) / 2 / float64(height)}
			return node.Text, point, true, nil
		}
		for _, child := range node.Children {
			if text, point, found, err := visit(child); found || err != nil {
				return text, point, found, err
			}
		}
		return "", recordreplay.Point{}, false, nil
	}
	text, point, found, err := visit(root)
	if err != nil {
		return "", recordreplay.Point{}, err
	}
	if !found {
		return "", recordreplay.Point{}, errors.New("no focused non-password input field")
	}
	if point.X < 0 || point.X > 1 || point.Y < 0 || point.Y > 1 {
		return "", recordreplay.Point{}, fmt.Errorf("focused input is outside the recorded display")
	}
	return text, point, nil
}

func (a *API) appendFocusedRecordingText(w http.ResponseWriter, r *http.Request) {
	if !a.requireRecordReplay(w) {
		return
	}
	sessionID, ownerToken := executionCredentials(r)
	if err := a.recordReplay.ValidateRecordingOwner(sessionID, ownerToken); err != nil {
		fail(w, executionStopErrorStatus(err), err)
		return
	}
	current := a.recordReplay.CurrentCase()
	document, err := a.automation.Hierarchy(r.Context())
	if err != nil {
		fail(w, http.StatusConflict, err)
		return
	}
	text, focus, err := focusedText(document, current.RecordedWidth, current.RecordedHeight)
	if err != nil {
		fail(w, http.StatusConflict, err)
		return
	}
	value, err := a.recordReplay.AppendTextOwned(r.Context(), sessionID, ownerToken, text, &focus)
	if err != nil {
		fail(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (a *API) recordingCases(w http.ResponseWriter, r *http.Request) {
	if !a.requireRecordReplay(w) {
		return
	}
	values, err := a.recordReplay.Cases(r.URL.Query().Get("package"))
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cases": values})
}

func (a *API) savedRecordingCase(w http.ResponseWriter, r *http.Request) {
	if !a.requireRecordReplay(w) {
		return
	}
	value, err := a.recordReplay.LoadCase(r.PathValue("id"))
	if err != nil {
		fail(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (a *API) deleteSavedRecordingCase(w http.ResponseWriter, r *http.Request) {
	a.deleteRecordingArtifact(w, r, a.recordReplay.DeleteCase)
}

func (a *API) recordingDrafts(w http.ResponseWriter, r *http.Request) {
	if !a.requireRecordReplay(w) {
		return
	}
	values, err := a.recordReplay.Drafts(r.URL.Query().Get("package"))
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"drafts": values})
}

func (a *API) finalizeRecordingDraft(w http.ResponseWriter, r *http.Request) {
	if !a.requireRecordReplay(w) {
		return
	}
	if !requireControlHeader(w, r) {
		return
	}
	a.startExclusiveInput(w, func() (any, error) {
		return a.recordReplay.FinalizeDraft(r.Context(), r.PathValue("id"))
	})
}

func (a *API) deleteRecordingDraft(w http.ResponseWriter, r *http.Request) {
	a.deleteRecordingArtifact(w, r, a.recordReplay.DeleteDraft)
}

func (a *API) updateRecordingExcludedBounds(w http.ResponseWriter, r *http.Request) {
	if !a.requireRecordReplay(w) {
		return
	}
	var request recordreplay.Bounds
	if !decodeRecordReplayJSON(w, r, &request) {
		return
	}
	sessionID, ownerToken := executionCredentials(r)
	if err := a.recordReplay.SetExcludedBoundsOwned(sessionID, ownerToken, &request); err != nil {
		fail(w, executionStopErrorStatus(err), err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"updated": true})
}

func (a *API) appendRecordingText(w http.ResponseWriter, r *http.Request) {
	if !a.requireRecordReplay(w) {
		return
	}
	var request struct {
		Text  string  `json:"text"`
		Focus bool    `json:"focus,omitempty"`
		X     float64 `json:"x,omitempty"`
		Y     float64 `json:"y,omitempty"`
	}
	if !decodeRecordReplayJSON(w, r, &request) {
		return
	}
	var focus *recordreplay.Point
	if request.Focus {
		focus = &recordreplay.Point{X: request.X, Y: request.Y}
	}
	sessionID, ownerToken := executionCredentials(r)
	value, err := a.recordReplay.AppendTextOwned(r.Context(), sessionID, ownerToken, request.Text, focus)
	if err != nil {
		fail(w, executionStopErrorStatus(err), err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (a *API) appendRecordingKey(w http.ResponseWriter, r *http.Request) {
	if !a.requireRecordReplay(w) {
		return
	}
	var request struct {
		KeyCode int `json:"keyCode"`
	}
	if !decodeRecordReplayJSON(w, r, &request) {
		return
	}
	sessionID, ownerToken := executionCredentials(r)
	value, err := a.recordReplay.AppendKeyOwned(r.Context(), sessionID, ownerToken, request.KeyCode)
	if err != nil {
		fail(w, executionStopErrorStatus(err), err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (a *API) appendScreenshotAssertion(w http.ResponseWriter, r *http.Request) {
	if !a.requireRecordReplay(w) {
		return
	}
	var request struct {
		MaxHashDistance int `json:"maxHashDistance"`
	}
	if !decodeRecordReplayJSON(w, r, &request) {
		return
	}
	sessionID, ownerToken := executionCredentials(r)
	value, err := a.recordReplay.AppendScreenshotAssertionOwned(r.Context(), sessionID, ownerToken, request.MaxHashDistance)
	if err != nil {
		fail(w, executionStopErrorStatus(err), err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (a *API) validateReplay(w http.ResponseWriter, r *http.Request) {
	var value recordreplay.Case
	if !decodeRecordReplayJSON(w, r, &value) {
		return
	}
	if err := value.Verify(); err != nil {
		fail(w, http.StatusUnprocessableEntity, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"valid": true, "schemaVersion": value.SchemaVersion, "package": value.Package, "actions": len(value.Actions), "caseFingerprint": value.Integrity})
}

func (a *API) requireRecordReplay(w http.ResponseWriter) bool {
	if a.recordReplay == nil {
		fail(w, http.StatusServiceUnavailable, errors.New("record/replay engine unavailable"))
		return false
	}
	return true
}

func (a *API) deleteRecordingArtifact(w http.ResponseWriter, r *http.Request, remove func(string) error) {
	if !a.requireRecordReplay(w) {
		return
	}
	if !requireControlHeader(w, r) {
		return
	}
	a.executionMu.Lock()
	defer a.executionMu.Unlock()
	if err := remove(r.PathValue("id")); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

func (a *API) startRecording(w http.ResponseWriter, r *http.Request) {
	if !a.requireRecordReplay(w) {
		return
	}
	var config recordreplay.RecordingConfig
	if !decodeRecordReplayJSON(w, r, &config) {
		return
	}
	a.startExclusiveInput(w, func() (any, error) {
		return a.recordReplay.StartRecording(r.Context(), config)
	})
}

func (a *API) recordingState(w http.ResponseWriter, _ *http.Request) {
	if a.requireRecordReplay(w) {
		writeJSON(w, http.StatusOK, a.recordReplay.RecordingState())
	}
}

func (a *API) stopRecording(w http.ResponseWriter, r *http.Request) {
	if !a.requireRecordReplay(w) {
		return
	}
	a.stopExclusiveInput(w, r, func(ctx context.Context, sessionID, ownerToken string) (any, error) {
		return a.recordReplay.StopRecordingOwned(ctx, sessionID, ownerToken)
	})
}

func (a *API) stopExclusiveInput(w http.ResponseWriter, r *http.Request, stop func(context.Context, string, string) (any, error)) {
	a.executionMu.Lock()
	defer a.executionMu.Unlock()
	sessionID, ownerToken := executionCredentials(r)
	state, err := stop(r.Context(), sessionID, ownerToken)
	if err != nil {
		fail(w, executionStopErrorStatus(err), err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (a *API) recordingCase(w http.ResponseWriter, _ *http.Request) {
	if a.requireRecordReplay(w) {
		writeJSON(w, http.StatusOK, a.recordReplay.CurrentCase())
	}
}

func (a *API) startReplay(w http.ResponseWriter, r *http.Request) {
	if !a.requireRecordReplay(w) {
		return
	}
	var config recordreplay.ReplayConfig
	if !decodeRecordReplayJSON(w, r, &config) {
		return
	}
	a.startExclusiveInput(w, func() (any, error) {
		return a.recordReplay.StartReplay(r.Context(), config)
	})
}

func (a *API) startExclusiveInput(w http.ResponseWriter, start func() (any, error)) {
	a.executionMu.Lock()
	defer a.executionMu.Unlock()
	if a.shuttingDown {
		fail(w, http.StatusServiceUnavailable, errors.New("agent is shutting down"))
		return
	}
	runnerState := a.runner.State()
	explorationBusy := false
	if a.explorer != nil {
		state := a.explorer.State()
		explorationBusy = state.Running || state.Stopping || state.Finalizing
	}
	if runnerState.Running || runnerState.Finalizing || explorationBusy {
		fail(w, http.StatusConflict, errors.New("another input session is active"))
		return
	}
	state, err := start()
	if err != nil {
		fail(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusAccepted, state)
}

func (a *API) replayState(w http.ResponseWriter, _ *http.Request) {
	if a.requireRecordReplay(w) {
		writeJSON(w, http.StatusOK, a.recordReplay.ReplayState())
	}
}

func (a *API) stopReplay(w http.ResponseWriter, r *http.Request) {
	if !a.requireRecordReplay(w) {
		return
	}
	a.stopExclusiveInput(w, r, func(ctx context.Context, sessionID, ownerToken string) (any, error) {
		return a.recordReplay.StopReplayOwned(ctx, sessionID, ownerToken)
	})
}

func executionStopErrorStatus(err error) int {
	switch {
	case errors.Is(err, execution.ErrOwnerMismatch):
		return http.StatusConflict
	case errors.Is(err, context.DeadlineExceeded):
		return http.StatusGatewayTimeout
	default:
		return http.StatusInternalServerError
	}
}
