package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zhoujun94511/xtest-nova/agent/internal/configstore"
)

type overlayDevice struct {
	fakeDevice
	running  bool
	commands []string
}

func (d *overlayDevice) Shell(_ context.Context, command string, _ time.Duration) (string, error) {
	d.commands = append(d.commands, command)
	switch {
	case strings.Contains(command, "am start -n "):
		d.running = true
	case strings.Contains(command, "stopservice"), strings.Contains(command, "force-stop"):
		d.running = false
	}
	if strings.Contains(command, "dumpsys activity services") && d.running {
		return "com.openatx.xtest.popup.OverlayService", nil
	}
	return "", nil
}

func TestOverlayButtonsStartAndStopTheForegroundService(t *testing.T) {
	device := &overlayDevice{}
	api := New(device, &fakeRunner{}, configstore.NewEmpty(t.TempDir()+"/config.json"), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "", false)
	start := httptest.NewRequest(http.MethodPost, "/v1/overlay", nil)
	start.Header.Set("X-XTest-Control", "true")
	response := httptest.NewRecorder()
	api.Primary().ServeHTTP(response, start)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"running":true`) {
		t.Fatalf("start overlay = %d %s", response.Code, response.Body.String())
	}
	joined := strings.Join(device.commands, "\n")
	if !strings.Contains(joined, "am start -n com.openatx.xtest.popup/.PopupLauncherActivity --es target_package com.example.app") {
		t.Fatalf("overlay start commands = %v", device.commands)
	}
	stop := httptest.NewRequest(http.MethodDelete, "/v1/overlay", nil)
	stop.Header.Set("X-XTest-Control", "true")
	response = httptest.NewRecorder()
	api.Primary().ServeHTTP(response, stop)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"running":false`) {
		t.Fatalf("stop overlay = %d %s", response.Code, response.Body.String())
	}
	script, err := consoleFiles.ReadFile("web/assets/js/app.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(script), "api('/v1/overlay', {method: 'POST'})") || strings.Contains(string(script), "popup-start': () => api('/popupBoxAssistant'") {
		t.Fatal("overlay buttons still call the popup assistant")
	}
}
