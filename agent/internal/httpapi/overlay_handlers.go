package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/zhoujun94511/xtest-nova/agent/internal/command"
)

func (a *API) registerOverlayRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /v1/overlay", a.overlay)
	m.HandleFunc("POST /v1/overlay", a.overlay)
	m.HandleFunc("DELETE /v1/overlay", a.overlay)
}

func (a *API) overlay(w http.ResponseWriter, r *http.Request) {
	if !requireControlHeader(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	switch r.Method {
	case http.MethodGet:
		running, err := a.overlayRunning(ctx)
		if err != nil {
			fail(w, http.StatusBadGateway, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "running": running})
	case http.MethodPost:
		a.executionMu.Lock()
		down := a.shuttingDown
		a.executionMu.Unlock()
		if down {
			fail(w, http.StatusServiceUnavailable, errors.New("服务正在退出"))
			return
		}
		if err := a.startOverlay(ctx); err != nil {
			fail(w, http.StatusBadGateway, err)
			return
		}
		running, err := a.waitOverlay(ctx, true)
		if err != nil {
			fail(w, http.StatusBadGateway, err)
			return
		}
		if !running {
			fail(w, http.StatusBadGateway, errors.New("悬浮窗没有保持运行，请确认已允许显示在其他应用上层"))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "running": true})
	case http.MethodDelete:
		if err := a.stopOverlay(ctx); err != nil {
			fail(w, http.StatusBadGateway, err)
			return
		}
		running, err := a.waitOverlay(ctx, false)
		if err != nil {
			fail(w, http.StatusBadGateway, err)
			return
		}
		if running {
			fail(w, http.StatusBadGateway, errors.New("悬浮窗仍在运行"))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "running": false})
	default:
		w.Header().Set("Allow", "GET, POST, DELETE")
		fail(w, http.StatusMethodNotAllowed, errors.New("不支持的请求方法"))
	}
}

func (a *API) startOverlay(ctx context.Context) error {
	_, _ = a.device.Shell(ctx, "appops set "+command.PopupPackage+" SYSTEM_ALERT_WINDOW allow", 3*time.Second)
	_, _ = a.device.Shell(ctx, "appops set --uid "+command.PopupPackage+" SYSTEM_ALERT_WINDOW allow", 3*time.Second)
	launch := "am start -n " + command.PopupActivity
	if target, err := a.device.ForegroundPackage(ctx); err == nil && overlayTargetPackage(target) {
		launch += " --es target_package " + target
	}
	output, err := a.device.Shell(ctx, launch, 5*time.Second)
	if err != nil || overlayStartFailed(output) {
		if err != nil {
			return err
		}
		return errors.New("悬浮窗没有保持运行，请确认已允许显示在其他应用上层")
	}
	return nil
}

func overlayTargetPackage(value string) bool {
	if value == "" || value == command.PopupPackage {
		return false
	}
	for _, part := range strings.Split(value, ".") {
		if part == "" || !overlayPackagePart(part) {
			return false
		}
	}
	return strings.Contains(value, ".")
}

func overlayPackagePart(value string) bool {
	for i, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '_':
		case i > 0 && r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return value != ""
}

func overlayStartFailed(output string) bool {
	text := strings.ToLower(output)
	return strings.Contains(text, "error") || strings.Contains(text, "unable to resolve") || strings.Contains(text, "does not exist")
}

func (a *API) stopOverlay(ctx context.Context) error {
	output, err := a.device.Shell(ctx, "am stopservice -n "+command.PopupPackage+"/.OverlayService", 5*time.Second)
	if err == nil && !overlayStartFailed(output) {
		return nil
	}
	_, err = a.device.Shell(ctx, "am force-stop "+command.PopupPackage, 5*time.Second)
	return err
}

func (a *API) overlayRunning(ctx context.Context) (bool, error) {
	output, err := a.device.Shell(ctx, "dumpsys activity services "+command.PopupPackage, 4*time.Second)
	if err != nil {
		return false, err
	}
	return strings.Contains(output, command.PopupPackage+".OverlayService") || strings.Contains(output, "OverlayService"), nil
}

func (a *API) waitOverlay(ctx context.Context, want bool) (bool, error) {
	deadline := time.Now().Add(2 * time.Second)
	var running bool
	var err error
	for {
		running, err = a.overlayRunning(ctx)
		if err != nil || running == want || time.Now().After(deadline) {
			return running, err
		}
		timer := time.NewTimer(150 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return running, ctx.Err()
		case <-timer.C:
		}
	}
}
