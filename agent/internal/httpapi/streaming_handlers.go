package httpapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
)

func (a *API) registerStreamingRoutes(m *http.ServeMux) {
	m.HandleFunc("/appevent/info", a.appEventInfo)
	m.HandleFunc("/appeventmonitor", a.appEventMonitor)
	m.HandleFunc("PUT /monitor", a.monitorWebSocket)
	m.HandleFunc("GET /minicap", a.minicapFallback)
	m.HandleFunc("GET /minicap/broadcast", a.minicapFallback)
	m.HandleFunc("POST /screenrecord", a.startScreenRecord)
	m.HandleFunc("PUT /screenrecord", a.stopScreenRecord)
}
func (a *API) appEventInfo(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, (1<<20)+1))
	if err != nil || len(body) > 1<<20 {
		fail(w, http.StatusRequestEntityTooLarge, errors.New("event payload exceeds 1 MiB"))
		return
	}
	a.events.Publish(string(body))
	writeJSON(w, 200, map[string]any{"success": true})
}
func (a *API) appEventMonitor(w http.ResponseWriter, r *http.Request) {
	upgrade := r
	if r.Method != http.MethodGet {
		upgrade = r.Clone(r.Context())
		upgrade.Method = http.MethodGet
	}
	connection, err := websocket.Accept(w, upgrade, nil)
	if err != nil {
		return
	}
	defer func() { _ = connection.CloseNow() }()
	connection.SetReadLimit(4096)
	events, unsubscribe := a.events.Subscribe()
	defer unsubscribe()
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	cancelOnWebSocketRead(ctx, connection, cancel)
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			writeCtx, done := context.WithTimeout(ctx, 10*time.Second)
			err = connection.Write(writeCtx, websocket.MessageText, []byte(event))
			done()
			if err != nil {
				return
			}
		}
	}
}
func (a *API) monitorWebSocket(w http.ResponseWriter, r *http.Request) {
	a.monitor.ServeWebSocket(w, r)
}
func (a *API) minicapFallback(w http.ResponseWriter, r *http.Request) {
	connection, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = connection.CloseNow() }()
	connection.SetReadLimit(4096)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	cancelOnWebSocketRead(ctx, connection, cancel)
	rotation, rotationErr := a.system.Rotation(ctx)
	if rotationErr != nil {
		_ = connection.Close(websocket.StatusInternalError, "display rotation unavailable")
		return
	}
	if err = connection.Write(ctx, websocket.MessageText, []byte(fmt.Sprintf("rotation %d", rotation))); err != nil {
		return
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		frame, frameErr := a.automation.Screenshot(ctx)
		if frameErr != nil {
			_ = connection.Close(websocket.StatusInternalError, "screenshot unavailable")
			return
		}
		writeCtx, done := context.WithTimeout(ctx, 10*time.Second)
		err = connection.Write(writeCtx, websocket.MessageBinary, frame)
		done()
		if err != nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func cancelOnWebSocketRead(ctx context.Context, connection *websocket.Conn, cancel context.CancelFunc) {
	go func() {
		for {
			if _, _, readErr := connection.Read(ctx); readErr != nil {
				cancel()
				return
			}
		}
	}()
}

func (a *API) startScreenRecord(w http.ResponseWriter, r *http.Request) {
	a.executionMu.Lock()
	defer a.executionMu.Unlock()
	if a.shuttingDown {
		fail(w, http.StatusServiceUnavailable, errors.New("agent is shutting down"))
		return
	}
	packageName := strings.TrimSpace(r.URL.Query().Get("package"))
	if packageName == "" {
		if foreground, err := a.device.ForegroundPackage(r.Context()); err == nil {
			packageName = foreground
		}
	}
	if err := a.recorder.StartForPackage(packageName); err != nil {
		fail(w, 400, err)
		return
	}
	_, _ = w.Write([]byte("screenrecord started"))
}
func (a *API) stopScreenRecord(w http.ResponseWriter, _ *http.Request) {
	a.executionMu.Lock()
	defer a.executionMu.Unlock()
	videos, err := a.recorder.Stop()
	if err != nil {
		fail(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]any{"videos": videos})
}
