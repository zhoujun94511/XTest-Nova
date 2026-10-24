package command

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseDeviceCommands(t *testing.T) {
	invocation, err := Parse([]string{"monkey", "start", "--package", "com.example.app", "--duration", "30", "--json"})
	if err != nil {
		t.Fatal(err)
	}
	if invocation.Kind != Monkey || invocation.MonkeyAction != "start" || invocation.PackageName != "com.example.app" || invocation.DurationSeconds != 30 || !invocation.JSON {
		t.Fatalf("unexpected monkey start: %#v", invocation)
	}
	if _, err := Parse([]string{"explore", "start", "--package", "com.example.app"}); err != nil {
		t.Fatal(err)
	}
	preview, err := Parse([]string{"explore", "start", "--package", "com.example.app", "--execute"})
	if err != nil || !preview.Execute {
		t.Fatalf("execute flag not parsed: %#v %v", preview, err)
	}
	if _, err := Parse([]string{"replay", "start", "--package", "com.example.app", "--name", "open"}); err != nil {
		t.Fatal(err)
	}
	if _, err := Parse([]string{"shell"}); err == nil {
		t.Fatal("shell must stay outside the device CLI")
	}
	if _, err := Parse([]string{"screenshot", "/tmp/shot.png"}); err == nil {
		t.Fatal("screenshot path outside allowed roots was accepted")
	}
}

func TestStopRedactsOwnerToken(t *testing.T) {
	var sawToken bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = io.WriteString(w, `{"running":true,"package":"com.example.app","identity":{"sessionId":"session-1","ownerToken":"owner-secret"}}`)
			return
		}
		if r.Header.Get("X-XTest-Session-Id") != "session-1" || r.Header.Get("X-XTest-Owner-Token") != "owner-secret" {
			t.Fatalf("missing ownership headers")
		}
		sawToken = r.Header.Get("X-XTest-Owner-Token") == "owner-secret"
		_, _ = io.WriteString(w, `{"running":false,"identity":{"sessionId":"session-1","ownerToken":"owner-secret"}}`)
	}))
	defer server.Close()
	var output strings.Builder
	invocation := Invocation{Kind: Explore, Action: "stop", JSON: true}
	if err := Run(context.Background(), invocation, server.URL, server.Client(), &output); err != nil {
		t.Fatal(err)
	}
	if !sawToken {
		t.Fatal("stop did not send the owner token")
	}
	if strings.Contains(output.String(), "owner-secret") {
		t.Fatalf("owner token leaked into output: %s", output.String())
	}
}

func TestReplayWithoutExecuteDoesNotStart(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("replay without execute contacted the agent")
	}))
	defer server.Close()
	var output strings.Builder
	invocation := Invocation{Kind: Replay, Action: "start", PackageName: "com.example.app", Name: "open"}
	if err := Run(context.Background(), invocation, server.URL, server.Client(), &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "not started") {
		t.Fatalf("unexpected output: %s", output.String())
	}
}

func TestExploreStartWithoutExecutePreviewsOnly(t *testing.T) {
	var path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_, _ = io.WriteString(w, `{"success":true,"analysis":{"package":"com.example.app","activity":".Main","nodeCount":3,"actions":[{}]}}`)
	}))
	defer server.Close()
	var output strings.Builder
	invocation := Invocation{Kind: Explore, Action: "start", PackageName: "com.example.app"}
	if err := Run(context.Background(), invocation, server.URL, server.Client(), &output); err != nil {
		t.Fatal(err)
	}
	if path != "/v1/exploration/preview" || !strings.Contains(output.String(), "preview only") {
		t.Fatalf("path=%s output=%s", path, output.String())
	}
}

func TestExploreStartWithoutExecuteJSONMarksPreviewOnly(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/exploration/preview" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"success":true,"analysis":{"package":"com.example.app","nodeCount":2,"actions":[]}}`)
	}))
	defer server.Close()
	var output strings.Builder
	invocation := Invocation{Kind: Explore, Action: "start", PackageName: "com.example.app", JSON: true}
	if err := Run(context.Background(), invocation, server.URL, server.Client(), &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"previewOnly": true`) || strings.Contains(output.String(), `"running"`) {
		t.Fatalf("json preview was ambiguous: %s", output.String())
	}
}

func TestMonkeyStartPostsDuration(t *testing.T) {
	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, _ := io.ReadAll(r.Body)
		body = string(payload)
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, `{"running":true,"package":"com.example.app","requestId":"run-1","identity":{"ownerToken":"hidden"}}`)
	}))
	defer server.Close()
	var output strings.Builder
	invocation := Invocation{Kind: Monkey, MonkeyAction: "start", PackageName: "com.example.app", DurationSeconds: 30}
	if err := Run(context.Background(), invocation, server.URL, server.Client(), &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, `"durationSeconds":30`) || !strings.Contains(body, `"throttleMillis":500`) || !strings.Contains(body, `"renderFallbackMode":"bounded"`) {
		t.Fatalf("unexpected body: %s", body)
	}
	if strings.Contains(output.String(), "hidden") {
		t.Fatalf("token leaked: %s", output.String())
	}
}

func TestTraversalStartFlagsMatchOverlayDefaults(t *testing.T) {
	invocation, err := Parse([]string{
		"monkey", "start", "--package", "com.example.app", "--duration", "30",
		"--throttle", "800", "--render", "continuous", "--activity-mode", "allowlist",
		"--activities", ".FeedActivity,.MineActivity", "--control-blacklist", "ad_button",
		"--target-cases", ".FeedActivity=smoke/open；.MineActivity=smoke/open",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !invocation.LowBatteryExit || invocation.MinBattery != 15 || invocation.ThrottleMillis != 800 || invocation.RenderFallbackMode != "continuous" {
		t.Fatalf("defaults/flags = %#v", invocation)
	}
	if len(invocation.Activities) != 2 || len(invocation.ControlBlacklist) != 1 || len(invocation.TargetCases) != 2 || invocation.TargetCases[1].Task != "smoke" {
		t.Fatalf("lists = %#v", invocation)
	}
	body, err := json.Marshal(monkeyStartBody(invocation))
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{`"lowBatteryExit":true`, `"minBatteryPercent":15`, `"activityMode":"allowlist"`, `"throttleMillis":800`, `"renderFallbackMode":"continuous"`} {
		if !strings.Contains(string(body), fragment) {
			t.Fatalf("body %s missing %s", body, fragment)
		}
	}
	disabled, err := Parse([]string{"explore", "start", "--package", "com.example.app", "--min-battery", "0", "--render", "off"})
	if err != nil {
		t.Fatal(err)
	}
	if disabled.LowBatteryExit || disabled.RenderFallbackMode != "off" || disabled.ThrottleMillis != 500 {
		t.Fatalf("explore flags = %#v", disabled)
	}
}

func TestScreenshotRejectsPathOutsideRoots(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("rejected path still requested a screenshot")
	}))
	defer server.Close()
	invocation := Invocation{Kind: ScreenshotCmd, TargetPath: "/etc/passwd"}
	if err := Run(context.Background(), invocation, server.URL, server.Client(), io.Discard); err == nil {
		t.Fatal("expected path rejection")
	}
}
