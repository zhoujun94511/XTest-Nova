package command

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseCoversEveryDeviceCommand(t *testing.T) {
	cases := []struct {
		args    []string
		kind    Kind
		action  string
		pkg     string
		execute bool
		json    bool
	}{
		{[]string{"--json", "doctor"}, Doctor, "", "", false, true},
		{[]string{"device", "--json"}, DeviceInfo, "", "", false, true},
		{[]string{"foreground"}, Foreground, "", "", false, false},
		{[]string{"apps", "list"}, Apps, "list", "", false, false},
		{[]string{"apps", "info", "com.example.app"}, Apps, "info", "com.example.app", false, false},
		{[]string{"apps", "launch", "com.example.app"}, Apps, "launch", "com.example.app", false, false},
		{[]string{"screenshot", "/sdcard/shot.png"}, ScreenshotCmd, "", "", false, false},
		{[]string{"explore", "preview", "--package", "com.example.app"}, Explore, "preview", "com.example.app", false, false},
		{[]string{"explore", "status"}, Explore, "status", "", false, false},
		{[]string{"explore", "stop"}, Explore, "stop", "", false, false},
		{[]string{"record", "start", "--package", "com.example.app", "--name", "open", "--task", "checkout"}, Record, "start", "com.example.app", false, false},
		{[]string{"record", "cases", "--package", "com.example.app"}, Record, "cases", "com.example.app", false, false},
		{[]string{"replay", "stop"}, Replay, "stop", "", false, false},
		{[]string{"perf", "start", "--package", "com.example.app"}, Perf, "start", "com.example.app", false, false},
		{[]string{"perf", "status"}, Perf, "status", "", false, false},
		{[]string{"artifacts", "--package", "com.example.app"}, Artifacts, "", "com.example.app", false, false},
		{[]string{"pull", "/sdcard/xtest-nova/com.example.app", "/data/local/tmp/out.zip"}, Pull, "", "", false, false},
	}
	for _, test := range cases {
		got, err := Parse(test.args)
		if err != nil {
			t.Fatalf("Parse(%v): %v", test.args, err)
		}
		if got.Kind != test.kind || got.Action != test.action || got.PackageName != test.pkg || got.Execute != test.execute || got.JSON != test.json {
			t.Fatalf("Parse(%v) = kind %d action %q package %q execute %t json %t", test.args, got.Kind, got.Action, got.PackageName, got.Execute, got.JSON)
		}
	}
}

func TestParseRejectsUnsafeOrIncompleteCommands(t *testing.T) {
	rejected := [][]string{
		{"--json"},
		{"stop"},
		{"shell"},
		{"term"},
		{"monkey", "start", "--package", "com.example.app"},
		{"monkey", "start", "--duration", "10"},
		{"explore", "start"},
		{"explore", "preview", "--execute"},
		{"record", "start", "--package", "com.example.app"},
		{"replay", "start", "--package", "com.example.app", "--execute"},
		{"screenshot", "/sdcard/../etc/passwd"},
		{"pull", "/sdcard/a b", "/tmp/out.zip"},
		{"apps", "info", "not a package"},
		{"artifacts", "--package", ".."},
	}
	for _, args := range rejected {
		if _, err := Parse(args); err == nil {
			t.Fatalf("Parse(%v) succeeded", args)
		}
	}
	escaped, err := Parse([]string{"pull", "/sdcard/a b", "/data/local/tmp/out.zip"})
	if err != nil {
		t.Fatal(err)
	}
	if escaped.TargetPath != "/sdcard/a b" || escaped.DestPath != "/data/local/tmp/out.zip" {
		t.Fatalf("path with space was rewritten: %#v", escaped)
	}
}

func TestAllowedDevicePathCanonicalizesTraversal(t *testing.T) {
	got, err := allowedDevicePath("/sdcard/xtest-nova/../shot.png")
	if err != nil || got != "/sdcard/shot.png" {
		t.Fatalf("canonical path = %q, %v", got, err)
	}
	if _, err := allowedDevicePath("/sdcard-extra/shot.png"); err == nil {
		t.Fatal("prefix lookalike was accepted")
	}
	if archiveEndpoint("/sdcard/a b") != "/archive/sdcard/a%20b" {
		t.Fatalf("archive path was not escaped: %s", archiveEndpoint("/sdcard/a b"))
	}
}

func TestReadBoundedRejectsOverflow(t *testing.T) {
	payload, err := readBounded(strings.NewReader("abcd"), 4)
	if err != nil || string(payload) != "abcd" {
		t.Fatalf("exact limit failed: %q %v", payload, err)
	}
	if _, err := readBounded(strings.NewReader("abcde"), 4); err == nil {
		t.Fatal("overflow was accepted")
	}
}

func TestWhiteBoxCommandRoutes(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		calls = append(calls, r.Method+" "+r.URL.RequestURI()+" "+string(body))
		switch {
		case r.URL.Path == "/v1/health":
			_, _ = io.WriteString(w, `{"status":"ok","version":"test"}`)
		case r.URL.Path == "/v1/device":
			_, _ = io.WriteString(w, `{"brand":"Google","model":"Pixel","sdk":34}`)
		case r.URL.Path == "/foregroundPkg":
			_, _ = io.WriteString(w, `{"package":"com.example.app"}`)
		case r.URL.Path == "/packages":
			_, _ = io.WriteString(w, `[{"packageName":"com.example.app","name":"Example"}]`)
		case r.URL.Path == "/packages/com.example.app/info":
			_, _ = io.WriteString(w, `{"data":{"packageName":"com.example.app","versionName":"1.2","versionCode":3}}`)
		case r.URL.Path == "/v1/apps/com.example.app/launch":
			if r.Method != http.MethodPost {
				t.Fatalf("launch method %s", r.Method)
			}
			_, _ = io.WriteString(w, `{"success":true}`)
		case r.URL.Path == "/v1/monkey/report":
			_, _ = io.WriteString(w, `{"package":"com.example.app","verdict":{"status":"passed","reason":"ok"}}`)
		case r.URL.Path == "/v1/exploration/sessions" && r.Method == http.MethodPost:
			if !strings.Contains(string(body), `"execute":true`) {
				t.Fatalf("explore start body: %s", body)
			}
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, `{"running":true,"package":"com.example.app","requestId":"explore-1"}`)
		case r.URL.Path == "/v1/recordings" && r.Method == http.MethodPost:
			if !strings.Contains(string(body), `"task":"checkout"`) || !strings.Contains(string(body), `"name":"open"`) {
				t.Fatalf("record body: %s", body)
			}
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, `{"running":true,"package":"com.example.app","requestId":"record-1"}`)
		case r.URL.Path == "/v1/recordings" && r.Method == http.MethodGet:
			_, _ = io.WriteString(w, `{"cases":[{"id":"older","name":"open","package":"com.example.app","actions":1,"recordedAt":"2026-10-01T00:00:00Z"},{"id":"newer","name":"open","package":"com.example.app","actions":2,"recordedAt":"2026-10-02T00:00:00Z"},{"id":"other","name":"close","package":"com.example.app","actions":1,"recordedAt":"2026-10-03T00:00:00Z"}]}`)
		case r.URL.Path == "/v1/recordings/cases/newer":
			_, _ = io.WriteString(w, `{"schemaVersion":"xtest-recording/v1","name":"open","package":"com.example.app","actions":[]}`)
		case r.URL.Path == "/v1/replays" && r.Method == http.MethodPost:
			if !strings.Contains(string(body), `"execute":true`) || !strings.Contains(string(body), `"name":"open"`) || strings.Contains(string(body), `"id":"older"`) {
				t.Fatalf("replay body: %s", body)
			}
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, `{"running":true,"package":"com.example.app","requestId":"replay-1"}`)
		case r.URL.Path == "/v1/performance/sessions" && r.Method == http.MethodPost:
			if string(body) != `{"package":"com.example.app"}` {
				t.Fatalf("perf body: %s", body)
			}
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, `{"running":true,"package":"com.example.app","rows":0,"path":"/sdcard/xtest-nova/com.example.app/Perf/1"}`)
		case r.URL.Path == "/v1/diagnostics/artifacts":
			if r.URL.Query().Get("package") != "com.example.app" {
				t.Fatalf("artifact query: %s", r.URL.RawQuery)
			}
			_, _ = io.WriteString(w, `{"sessions":[{"package":"com.example.app","kind":"Monkey","name":"20261023","path":"/sdcard/xtest-nova/com.example.app/Monkey/20261023"}]}`)
		case r.URL.Path == "/archive/sdcard/a%20b":
			_, _ = io.WriteString(w, "zip-bytes")
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.RequestURI())
		}
	}))
	defer server.Close()
	client := server.Client()
	commands := []Invocation{
		{Kind: Doctor},
		{Kind: DeviceInfo},
		{Kind: Foreground},
		{Kind: Apps, Action: "list"},
		{Kind: Apps, Action: "info", PackageName: "com.example.app"},
		{Kind: Apps, Action: "launch", PackageName: "com.example.app"},
		{Kind: Monkey, MonkeyAction: "report"},
		{Kind: Explore, Action: "start", PackageName: "com.example.app", Execute: true},
		{Kind: Record, Action: "start", PackageName: "com.example.app", Name: "open", Task: "checkout"},
		{Kind: Replay, Action: "start", PackageName: "com.example.app", Name: "open", Execute: true},
		{Kind: Perf, Action: "start", PackageName: "com.example.app"},
		{Kind: Artifacts, PackageName: "com.example.app"},
	}
	var output strings.Builder
	for _, invocation := range commands {
		output.Reset()
		if err := Run(context.Background(), invocation, server.URL, client, &output); err != nil {
			t.Fatalf("%#v: %v", invocation, err)
		}
	}
	joined := strings.Join(calls, "\n")
	for _, fragment := range []string{
		"GET /v1/health",
		"GET /v1/device",
		"GET /foregroundPkg",
		"GET /packages ",
		"GET /packages/com.example.app/info",
		"POST /v1/apps/com.example.app/launch",
		"GET /v1/monkey/report",
		"POST /v1/exploration/sessions",
		"POST /v1/recordings",
		"GET /v1/recordings/cases/newer",
		"POST /v1/replays",
		"POST /v1/performance/sessions",
		"GET /v1/diagnostics/artifacts?package=com.example.app",
	} {
		if !strings.Contains(joined, fragment) {
			t.Fatalf("missing %s in\n%s", fragment, joined)
		}
	}
	if strings.Contains(joined, "/v1/recordings/cases/older") {
		t.Fatal("replay loaded an older case with the same name")
	}
}

func TestStopBranches(t *testing.T) {
	t.Run("idle does not delete", func(t *testing.T) {
		requests := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests++
			if r.Method != http.MethodGet {
				t.Fatalf("idle stop used %s", r.Method)
			}
			_, _ = io.WriteString(w, `{"running":false,"finalizing":true}`)
		}))
		defer server.Close()
		for _, invocation := range []Invocation{
			{Kind: Monkey, MonkeyAction: "stop"},
			{Kind: Record, Action: "stop"},
			{Kind: Replay, Action: "stop"},
			{Kind: Perf, Action: "stop"},
		} {
			if err := Run(context.Background(), invocation, server.URL, server.Client(), io.Discard); err != nil {
				t.Fatal(err)
			}
		}
		if requests != 4 {
			t.Fatalf("idle stops made %d requests", requests)
		}
	})
	t.Run("running without credentials does not delete", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Fatalf("delete without credentials: %s", r.Method)
			}
			_, _ = io.WriteString(w, `{"running":true,"identity":{}}`)
		}))
		defer server.Close()
		err := Run(context.Background(), Invocation{Kind: Perf, Action: "stop"}, server.URL, server.Client(), io.Discard)
		if err == nil || !strings.Contains(err.Error(), "ownership") {
			t.Fatalf("expected ownership error, got %v", err)
		}
	})
	t.Run("conflict response redacts token", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusConflict)
			_, _ = io.WriteString(w, `{"running":true,"identity":{"ownerToken":"owner-secret"},"error":"already active"}`)
		}))
		defer server.Close()
		err := Run(context.Background(), Invocation{Kind: Monkey, MonkeyAction: "start", PackageName: "com.example.app", DurationSeconds: 5}, server.URL, server.Client(), io.Discard)
		if err == nil || strings.Contains(err.Error(), "owner-secret") || !strings.Contains(err.Error(), "already active") {
			t.Fatalf("conflict error = %v", err)
		}
	})
}

func TestReplayMissingCase(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/recordings" {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"cases":[]}`)
	}))
	defer server.Close()
	err := Run(context.Background(), Invocation{Kind: Replay, Action: "start", PackageName: "com.example.app", Name: "missing", Execute: true}, server.URL, server.Client(), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected missing case, got %v", err)
	}
}

func TestPullWritesEscapedArchive(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != "/archive/sdcard/a%20b" {
			t.Fatalf("archive path %s", r.URL.EscapedPath())
		}
		_, _ = io.WriteString(w, "PK")
	}))
	defer server.Close()
	destination := "/data/local/tmp/xtest-nova-cli-whitebox.zip"
	native := filepath.FromSlash(destination)
	t.Cleanup(func() { _ = os.Remove(native) })
	var output strings.Builder
	err := Run(context.Background(), Invocation{Kind: Pull, TargetPath: "/sdcard/a b", DestPath: destination, JSON: true}, server.URL, server.Client(), &output)
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(native)
	if err != nil || string(content) != "PK" {
		t.Fatalf("archive file %q %v", content, err)
	}
	if !strings.Contains(output.String(), destination) || strings.Contains(output.String(), "ownerToken") {
		t.Fatalf("output %s", output.String())
	}
}

func TestUnreachableAgent(t *testing.T) {
	err := Run(context.Background(), Invocation{Kind: Doctor}, "http://127.0.0.1:1", &http.Client{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "not reachable") {
		t.Fatalf("expected unreachable agent, got %v", err)
	}
}
