package command

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestParseCompatibilityCommands(t *testing.T) {
	tests := []struct {
		args []string
		want Invocation
	}{
		{[]string{"version"}, Invocation{Kind: Version}},
		{[]string{"popup", "start"}, Invocation{Kind: Popup, PopupAction: "start"}},
		{[]string{"monkey", "status"}, Invocation{Kind: Monkey, MonkeyAction: "status"}},
		{[]string{"monkey", "stop"}, Invocation{Kind: Monkey, MonkeyAction: "stop"}},
		{[]string{"server", "-d"}, Invocation{Kind: Server, Daemon: true}},
		{[]string{"server", "-d", "--stop"}, Invocation{Kind: Server, Daemon: true, Stop: true}},
		{[]string{"server", "--legacy-unsafe-api"}, Invocation{Kind: Server, ServerArgs: []string{"--legacy-unsafe-api"}}},
	}
	for _, test := range tests {
		got, err := Parse(test.args)
		if err != nil {
			t.Fatalf("Parse(%v): %v", test.args, err)
		}
		if !reflect.DeepEqual(got, test.want) {
			t.Fatalf("Parse(%v) = %#v, want %#v", test.args, got, test.want)
		}
	}
}

func TestManageMonkeyStopUsesCurrentOwner(t *testing.T) {
	var deleted bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = fmt.Fprint(w, `{"running":true,"requestId":"run-1","identity":{"sessionId":"session-1","ownerToken":"owner-1"}}`)
			return
		}
		if r.Header.Get("X-XTest-Session-Id") != "session-1" || r.Header.Get("X-XTest-Owner-Token") != "owner-1" {
			t.Fatalf("missing execution ownership headers")
		}
		deleted = true
		_, _ = fmt.Fprint(w, `{"running":false,"finalizing":true}`)
	}))
	defer server.Close()
	var output strings.Builder
	if err := ManageMonkey(context.Background(), "stop", server.URL, server.Client(), &output); err != nil {
		t.Fatal(err)
	}
	if !deleted || !strings.Contains(output.String(), "overlay will restore") {
		t.Fatalf("unexpected result: deleted=%t output=%q", deleted, output.String())
	}
}

func TestManageMonkeyStopIsIdempotentWhenIdle(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		_, _ = fmt.Fprint(w, `{"running":false,"finalizing":false}`)
	}))
	defer server.Close()
	if err := ManageMonkey(context.Background(), "stop", server.URL, server.Client(), io.Discard); err != nil {
		t.Fatal(err)
	}
	if requests != 1 {
		t.Fatalf("idle stop made %d requests, want one state query", requests)
	}
}

type fakeExecutor struct {
	calls      [][]string
	installed  bool
	running    bool
	foreground string
}

func (f *fakeExecutor) Run(_ context.Context, name string, args ...string) (string, error) {
	f.calls = append(f.calls, append([]string{name}, args...))
	if name == "pm" && len(args) > 0 && args[0] == "path" {
		if f.installed {
			return "package:/data/app/popup.apk", nil
		}
		return "", context.Canceled
	}
	if name == "getprop" {
		return "33", nil
	}
	if name == "pm" && len(args) > 0 && args[0] == "install" {
		f.installed = true
		return "Success", nil
	}
	if name == "am" && len(args) > 0 && args[0] == "start" {
		f.running = true
		return "Starting", nil
	}
	if name == "dumpsys" && len(args) > 0 && args[0] == "package" {
		return "versionCode=30728 minSdk=28", nil
	}
	if name == "dumpsys" && len(args) > 0 && args[0] == "window" && f.foreground != "" {
		return "mCurrentFocus=Window{123 u0 " + f.foreground + "/.MainActivity}", nil
	}
	if name == "dumpsys" && len(args) > 0 && args[0] == "activity" && f.running {
		return PopupPackage + ".OverlayService", nil
	}
	return "Success", nil
}

func TestPopupStatusReportsRunningState(t *testing.T) {
	executor := &fakeExecutor{installed: true, running: true}
	var output strings.Builder
	if err := ManagePopup(context.Background(), "status", executor, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "installed=true running=true versionCode=30728") {
		t.Fatalf("unexpected status: %s", output.String())
	}
}
func (*fakeExecutor) RunBytes(context.Context, string, ...string) ([]byte, error) { return nil, nil }

func TestPopupStartInstallsAndLaunches(t *testing.T) {
	executor := &fakeExecutor{foreground: "com.example.app"}
	if err := ManagePopup(context.Background(), "start", executor, io.Discard); err != nil {
		t.Fatal(err)
	}
	var calls []string
	for _, call := range executor.calls {
		calls = append(calls, strings.Join(call, " "))
	}
	all := strings.Join(calls, "\n")
	for _, expected := range []string{"pm install -r " + PopupAPK, "appops set " + PopupPackage, "appops set --uid " + PopupPackage, "am start -n " + PopupActivity + " --es target_package com.example.app"} {
		if !strings.Contains(all, expected) {
			t.Fatalf("missing %q in %s", expected, all)
		}
	}
}
