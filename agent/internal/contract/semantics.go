package contract

import (
	"fmt"
	"sort"
	"strings"
)

type SemanticSpec struct {
	ID              int      `json:"id"`
	Method          string   `json:"method"`
	Path            string   `json:"path"`
	RequestEncoding []string `json:"requestEncoding"`
	RequestHeaders  []string `json:"requestHeaders,omitempty"`
	PathFields      []string `json:"pathFields,omitempty"`
	QueryFields     []string `json:"queryFields,omitempty"`
	BodyFields      []string `json:"bodyFields,omitempty"`
	SuccessStatuses []int    `json:"successStatuses"`
	ErrorStatuses   []int    `json:"errorStatuses"`
	ResponseKind    string   `json:"responseKind"`
	ResponseFields  []string `json:"responseFields,omitempty"`
	SideEffect      string   `json:"sideEffect"`
	RuntimeGate     string   `json:"runtimeGate"`
}

func Semantics() ([]SemanticSpec, error) {
	routes := Implemented()
	result := make([]SemanticSpec, 0, len(routes))
	for index, route := range routes {
		spec, ok := semanticFor(route)
		if !ok {
			return nil, fmt.Errorf("missing semantic contract for %s %s", route.Method, route.Path)
		}
		spec.ID, spec.Method, spec.Path = index+1, route.Method, route.Path
		if spec.RequestEncoding == nil {
			spec.RequestEncoding = []string{"none"}
		}
		if spec.SuccessStatuses == nil {
			spec.SuccessStatuses = []int{200}
		}
		if spec.ErrorStatuses == nil {
			spec.ErrorStatuses = []int{400, 500}
		}
		if spec.RuntimeGate == "" {
			spec.RuntimeGate = "always"
		}
		result = append(result, spec)
	}
	return result, ValidateSemantics(result, routes)
}

func ValidateSemantics(specs []SemanticSpec, routes []Route) error {
	if len(specs) != len(routes) {
		return fmt.Errorf("semantic count %d does not match route count %d", len(specs), len(routes))
	}
	seen := map[string]bool{}
	for _, spec := range specs {
		key := spec.Method + " " + canonical(spec.Path)
		if seen[key] {
			return fmt.Errorf("duplicate semantic contract %s", key)
		}
		seen[key] = true
		if len(spec.RequestEncoding) == 0 || len(spec.SuccessStatuses) == 0 || len(spec.ErrorStatuses) == 0 || spec.ResponseKind == "" || spec.SideEffect == "" || spec.RuntimeGate == "" {
			return fmt.Errorf("incomplete semantic contract %s", key)
		}
	}
	for _, route := range routes {
		if !seen[route.Method+" "+canonical(route.Path)] {
			return fmt.Errorf("semantic contract missing route %s %s", route.Method, route.Path)
		}
	}
	return nil
}

func semanticFor(route Route) (SemanticSpec, bool) {
	spec := SemanticSpec{ResponseKind: "json", ResponseFields: []string{"dynamic"}, SideEffect: "read"}
	path := canonical(route.Path)
	pathFields := func(names ...string) { spec.PathFields = names }
	switch path {
	case "/version":
		spec.ResponseKind, spec.ResponseFields, spec.ErrorStatuses = "text", nil, []int{500}
	case "/ping":
		spec.ResponseFields, spec.ErrorStatuses = []string{"success", "pong"}, []int{500}
	case "/dump/hierarchy":
		spec.ResponseFields, spec.RuntimeGate = []string{"jsonrpc", "id", "result"}, "uiautomator"
	case "/dump/hierarchyWithScreenshot":
		spec.ResponseFields, spec.RuntimeGate = []string{"windowHierarchy", "screenshot"}, "uiautomator"
	case "/info":
		spec.ResponseFields = []string{"serial", "model", "brand", "sdk", "agentVersion", "display", "density", "memory", "cpu", "storage"}
	case "/foregroundPkg":
		spec.ResponseFields = []string{"package"}
	case "/wakeupScreen":
		spec.ResponseFields, spec.SideEffect = []string{"success"}, "device-input"
	case "/shell":
		spec.RequestEncoding, spec.QueryFields, spec.BodyFields = []string{"query", "form", "json"}, []string{"command|c", "timeout?"}, []string{"command|c", "timeout?"}
		spec.ResponseFields, spec.SideEffect, spec.RuntimeGate = []string{"output", "exitCode", "error"}, "arbitrary-command", "legacy-unsafe-api"
	case "/shell/background":
		spec.RequestEncoding, spec.QueryFields, spec.BodyFields = []string{"query", "form", "json"}, []string{"command|c"}, []string{"command|c"}
		spec.ResponseFields, spec.SideEffect, spec.RuntimeGate = []string{"success", "pid", "description"}, "background-process", "legacy-unsafe-api"
		spec.ErrorStatuses = []int{400, 403, 405, 413, 429, 500}
	case "/term":
		spec.ResponseKind, spec.ResponseFields, spec.SideEffect, spec.RuntimeGate = "html-or-websocket", []string{"pty-frame"}, "interactive-shell", "legacy-unsafe-api"
		spec.SuccessStatuses = []int{200, 101}
		spec.ErrorStatuses = []int{403, 429, 500}
	case "/proc/list":
		spec.ResponseKind, spec.ResponseFields = "json-array", []string{"pid", "name"}
	case "/pidof/{}":
		pathFields("pkg")
		spec.ResponseKind, spec.ResponseFields, spec.ErrorStatuses = "text", nil, []int{410, 500}
	case "/proc/{}/meminfo":
		pathFields("pkg")
		spec.ResponseFields = []string{"memory-metrics"}
	case "/proc/{}/meminfo/all":
		pathFields("pkg")
		spec.ResponseFields = []string{"process-to-memory-map"}
	case "/proc/{}/cpuinfo":
		pathFields("pkg")
		spec.ResponseFields = []string{"pid", "cores", "processPercent"}
	case "/proc/{}/perf":
		pathFields("pkg")
		spec.ResponseFields = []string{"cpu", "memory", "network"}
	case "/device/memory":
		spec.ResponseFields = []string{"total", "free", "available"}
	case "/network/info":
		spec.ResponseFields = []string{"connected", "mobileConnected", "connectedType"}
	case "/disk/info":
		spec.ResponseFields = []string{"total", "available"}
	case "/services/{}":
		pathFields("name")
		spec.ResponseFields, spec.RuntimeGate = []string{"success", "running|description"}, "uiautomator"
		if route.Method != "GET" {
			spec.SideEffect = "service-lifecycle"
		}
	case "/uiautomator":
		spec.RuntimeGate = "uiautomator"
		spec.ResponseFields = []string{"running|text"}
		if route.Method != "GET" {
			spec.ResponseKind, spec.ResponseFields, spec.SideEffect = "text", nil, "service-lifecycle"
		}
	case "/raw/{}":
		pathFields("path")
		spec.ResponseKind, spec.ResponseFields, spec.RuntimeGate = "binary", nil, "allowed-file-root"
		spec.ErrorStatuses = []int{403, 404, 500}
	case "/finfo/{}":
		pathFields("path")
		spec.ResponseFields, spec.RuntimeGate = []string{"name", "path", "size", "mode", "isDir"}, "allowed-file-root"
	case "/upload/{}":
		pathFields("path")
		spec.RequestEncoding, spec.BodyFields = []string{"multipart"}, []string{"file", "mode?"}
		spec.ResponseFields, spec.SideEffect, spec.RuntimeGate = []string{"target", "isDir", "mode"}, "filesystem-write", "allowed-file-root"
	case "/installApk":
		spec.RequestEncoding, spec.BodyFields = []string{"multipart"}, []string{"file"}
		spec.ResponseFields, spec.SideEffect, spec.RuntimeGate = []string{"success", "output", "totalBytes"}, "package-install", "android-package-policy"
	case "/installLocalApk/{}":
		pathFields("apk")
		spec.ResponseFields, spec.SideEffect, spec.RuntimeGate = []string{"success", "output"}, "package-install-and-cleanup", "staged-apk"
	case "/installAgentApk":
		spec.ResponseFields, spec.SideEffect, spec.RuntimeGate = []string{"success", "installed", "versionCode|output"}, "companion-install", "staged-companion-apk"
	case "/download":
		spec.RequestEncoding, spec.BodyFields, spec.ResponseKind, spec.ResponseFields, spec.SideEffect, spec.RuntimeGate = []string{"form"}, []string{"url", "filepath", "mode?"}, "text", nil, "download-task", "public-http-url"
	case "/download/{}":
		pathFields("id")
		spec.ResponseFields = []string{"id", "status", "totalSize", "copiedSize", "error"}
	case "/packages":
		if route.Method == "POST" {
			spec.RequestEncoding, spec.BodyFields, spec.ResponseFields, spec.SideEffect, spec.RuntimeGate = []string{"form"}, []string{"url"}, []string{"success", "data.id"}, "package-install-task", "public-http-url"
		} else {
			spec.RequestEncoding, spec.QueryFields, spec.ResponseKind, spec.ResponseFields = []string{"query"}, []string{"system?"}, "json-array", []string{"packageName", "apkPath", "versionName", "versionCode", "system"}
		}
	case "/packages/{}":
		pathFields("id")
		spec.ResponseFields = []string{"success", "data.status", "data.description"}
		spec.ErrorStatuses = []int{404, 500}
	case "/packages/{}/info":
		pathFields("pkg")
		spec.ResponseFields = []string{"success", "data.packageName", "data.apkPath", "data.versionName", "data.versionCode", "data.system"}
	case "/packages/{}/icon":
		pathFields("pkg")
		spec.ResponseKind, spec.ResponseFields, spec.ErrorStatuses = "image/jpeg", nil, []int{404, 500}
	case "/install":
		spec.RequestEncoding, spec.BodyFields, spec.ResponseKind, spec.ResponseFields, spec.SideEffect, spec.RuntimeGate = []string{"form"}, []string{"url"}, "text", nil, "package-install-task", "public-http-url"
	case "/install/{}":
		pathFields("id")
		if route.Method == "DELETE" {
			spec.ResponseKind, spec.ResponseFields, spec.SideEffect = "text", nil, "task-cancel"
		} else {
			spec.ResponseFields = []string{"id", "status", "totalSize", "copiedSize", "error"}
		}
	case "/session/{}":
		pathFields("pkg")
		spec.ResponseFields, spec.SideEffect = []string{"success", "output", "mainActivity"}, "app-launch"
	case "/webviews":
		spec.ResponseKind, spec.ResponseFields = "json-array", []string{"socket", "pid", "package"}
	case "/webviews/{}":
		pathFields("pkg")
		spec.ResponseKind, spec.ResponseFields = "json-array", []string{"socket", "pid", "package"}
	case "/wlan/ip":
		spec.ResponseFields = []string{"ip"}
	case "/screenshot", "/screenshot/0":
		spec.ResponseKind, spec.ResponseFields, spec.RuntimeGate = "image/png", nil, "screenshot"
	case "/stop":
		spec.ResponseKind, spec.ResponseFields, spec.SideEffect = "text", nil, "agent-shutdown"
		spec.RequestHeaders = []string{"X-XTest-Control: true"}
		spec.ErrorStatuses = []int{403, 405, 500, 503}
	case "/minitouch":
		spec.RuntimeGate = "minitouch-binary"
		if route.Method == "GET" {
			spec.ResponseKind, spec.ResponseFields, spec.SuccessStatuses = "websocket", []string{"minitouch-protocol"}, []int{101}
		} else {
			spec.ResponseFields, spec.SideEffect = []string{"running", "pid", "lastError"}, "service-lifecycle"
		}
	case "/newCommandTimeout":
		spec.RequestEncoding, spec.BodyFields, spec.ResponseFields, spec.SideEffect = []string{"json-scalar"}, []string{"seconds"}, []string{"success", "description"}, "configuration"
	case "/popupBoxAssistant":
		spec.ResponseFields, spec.SideEffect, spec.RuntimeGate = []string{"success", "running", "stopping"}, "popup-assistant-lifecycle", "uiautomator"
	case "/appevent/info":
		spec.RequestEncoding, spec.BodyFields, spec.ResponseFields, spec.SideEffect = []string{"raw"}, []string{"event?"}, []string{"success"}, "event-publish"
	case "/appeventmonitor":
		spec.ResponseKind, spec.ResponseFields, spec.SuccessStatuses = "websocket", []string{"event-text"}, []int{101}
	case "/monitor":
		spec.ResponseKind, spec.ResponseFields, spec.SuccessStatuses, spec.RuntimeGate = "websocket", []string{"performance-json-line"}, []int{101}, "monitor-7890"
	case "/minicap", "/minicap/broadcast":
		spec.ResponseKind, spec.ResponseFields, spec.SuccessStatuses, spec.RuntimeGate = "websocket", []string{"rotation", "png-frame"}, []int{101}, "screenshot"
	case "/scrcpy/{}/{}":
		pathFields("type", "definition")
		spec.ResponseKind, spec.ResponseFields, spec.SuccessStatuses, spec.RuntimeGate = "websocket", []string{"h264-or-control-frame"}, []int{101}, "scrcpy-server"
	case "/screenrecord":
		spec.SideEffect, spec.RuntimeGate = "screenrecord-lifecycle", "screenrecord"
		if route.Method == "POST" {
			spec.ResponseKind, spec.ResponseFields = "text", nil
		} else {
			spec.ResponseFields = []string{"videos"}
		}
	case "/touchreader":
		spec.ResponseKind, spec.ResponseFields, spec.SuccessStatuses, spec.RuntimeGate = "websocket", []string{"protocol-b-event"}, []int{101}, "protocol-b-input"
	case "/jsonrpc/0":
		spec.RequestEncoding, spec.BodyFields, spec.ResponseFields, spec.SideEffect, spec.RuntimeGate = []string{"json"}, []string{"jsonrpc", "method", "params?", "id?"}, []string{"proxied-jsonrpc-response"}, "uiautomator-command", "uiautomator-jsonrpc"
		spec.ErrorStatuses = []int{400, 502}
	case "/imeStatus":
		spec.ResponseFields = []string{"current", "enabled"}
	case "/setIme":
		spec.RequestEncoding, spec.QueryFields, spec.BodyFields, spec.ResponseFields, spec.SideEffect, spec.RuntimeGate = []string{"form", "json"}, []string{"ime|id|inputMethod"}, []string{"ime|id|inputMethod"}, []string{"success", "ime", "output"}, "ime-change", "installed-ime"
	case "/u2packages":
		spec.ResponseFields = []string{"packages", "running"}
	case "/pushConfig":
		spec.RequestEncoding, spec.BodyFields, spec.ResponseFields, spec.SideEffect = []string{"json"}, []string{"configuration-object"}, []string{"success", "config"}, "configuration-write"
		spec.RequestHeaders = []string{"X-XTest-Control: true"}
		spec.ErrorStatuses = []int{400, 403, 413, 500}
	case "/pullConfig":
		spec.ResponseFields = []string{"configuration-object"}
	case "/assets/{}", "/static/js/{}", "/static/css/{}", "/static/media/{}":
		pathFields("path")
		spec.ResponseKind, spec.ResponseFields, spec.ErrorStatuses = "static-asset", nil, []int{404, 500}
	case "/", "/{}":
		if path == "/{}" {
			pathFields("path")
		}
		spec.ResponseKind, spec.ResponseFields, spec.ErrorStatuses = "text/html", nil, []int{404, 500}
	default:
		return SemanticSpec{}, false
	}
	return spec, true
}

func SemanticKeys(specs []SemanticSpec) []string {
	keys := make([]string, 0, len(specs))
	for _, spec := range specs {
		keys = append(keys, strings.Join([]string{spec.Method, canonical(spec.Path)}, " "))
	}
	sort.Strings(keys)
	return keys
}
