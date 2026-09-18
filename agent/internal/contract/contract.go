package contract

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
)

type Route struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}
type Report struct {
	TargetCount      int     `json:"targetCount"`
	ImplementedCount int     `json:"implementedCount"`
	Covered          []Route `json:"covered"`
	Missing          []Route `json:"missing"`
}

var parameter = regexp.MustCompile(`\{[^}]+}`)

func canonical(path string) string { return parameter.ReplaceAllString(path, "{}") }
func Implemented() []Route {
	return []Route{
		{"ANY", "/version"}, {"ANY", "/dump/hierarchy"}, {"ANY", "/dump/hierarchyWithScreenshot"},
		{"ANY", "/info"}, {"GET", "/foregroundPkg"}, {"GET", "/wakeupScreen"},
		{"GET", "/shell"}, {"POST", "/shell"}, {"GET", "/shell/background"}, {"POST", "/shell/background"}, {"ANY", "/term"}, {"POST", "/uiautomator"}, {"DELETE", "/uiautomator"}, {"GET", "/uiautomator"},
		{"ANY", "/raw/{path...}"}, {"ANY", "/finfo/{path...}"}, {"ANY", "/upload/{path...}"},
		{"ANY", "/installApk"}, {"GET", "/packages"}, {"ANY", "/packages/{pkg}/info"}, {"ANY", "/screenshot"},
		{"ANY", "/ping"}, {"GET", "/pullConfig"}, {"POST", "/pushConfig"},
		{"ANY", "/proc/list"}, {"ANY", "/pidof/{pkg}"}, {"ANY", "/proc/{pkg}/meminfo"},
		{"ANY", "/proc/{pkg}/meminfo/all"}, {"ANY", "/proc/{pkg}/cpuinfo"}, {"ANY", "/proc/{pkg}/perf"},
		{"ANY", "/device/memory"}, {"ANY", "/network/info"}, {"ANY", "/disk/info"},
		{"ANY", "/imeStatus"}, {"ANY", "/setIme"}, {"ANY", "/u2packages"},
		{"ANY", "/installAgentApk"}, {"ANY", "/installLocalApk/{apk}"},
		{"GET", "/services/{name}"}, {"POST", "/services/{name}"}, {"DELETE", "/services/{name}"},
		{"POST", "/download"}, {"ANY", "/download/{id}"},
		{"POST", "/packages"}, {"ANY", "/packages/{id}"},
		{"POST", "/install"}, {"GET", "/install/{id}"}, {"DELETE", "/install/{id}"},
		{"POST", "/session/{pkg}"}, {"ANY", "/webviews"}, {"ANY", "/webviews/{pkg}"},
		{"ANY", "/wlan/ip"}, {"ANY", "/screenshot/0"}, {"POST", "/stop"},
		{"PUT", "/minitouch"}, {"DELETE", "/minitouch"},
		{"GET", "/minitouch"}, {"POST", "/newCommandTimeout"}, {"ANY", "/packages/{pkg}/icon"},
		{"POST", "/popupBoxAssistant"}, {"DELETE", "/popupBoxAssistant"},
		{"ANY", "/appevent/info"}, {"ANY", "/appeventmonitor"}, {"PUT", "/monitor"},
		{"GET", "/minicap"}, {"GET", "/minicap/broadcast"},
		{"POST", "/screenrecord"}, {"PUT", "/screenrecord"},
		{"ANY", "/touchreader"}, {"ANY", "/jsonrpc/0"},
		{"ANY", "/assets/{path}"}, {"ANY", "/static/js/{path}"}, {"ANY", "/static/css/{path}"}, {"ANY", "/static/media/{path}"},
		{"ANY", "/"}, {"ANY", "/{path}"},
		{"ANY", "/scrcpy/{type}/{definition}"},
	}
}
func ParseNexusMarkdown(reader io.Reader) ([]Route, error) {
	scanner := bufio.NewScanner(reader)
	routes := make([]Route, 0, 72)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "|") || strings.Contains(line, "---") {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) < 6 {
			continue
		}
		method := strings.Trim(strings.TrimSpace(parts[2]), "`")
		path := strings.Trim(strings.TrimSpace(parts[3]), "`")
		if !strings.HasPrefix(path, "/") || method == "Method" {
			continue
		}
		for _, item := range strings.Split(method, ",") {
			routes = append(routes, Route{Method: strings.TrimSpace(item), Path: path})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(routes) == 0 {
		return nil, fmt.Errorf("no routes found")
	}
	return routes, nil
}
func Compare(target, implemented []Route) Report {
	available := map[string]bool{}
	availablePaths := map[string]bool{}
	for _, route := range implemented {
		path := canonical(route.Path)
		available[route.Method+" "+path] = true
		availablePaths[path] = true
	}
	report := Report{TargetCount: len(target), ImplementedCount: len(implemented), Covered: []Route{}, Missing: []Route{}}
	for _, route := range target {
		key := route.Method + " " + canonical(route.Path)
		path := canonical(route.Path)
		if available[key] || available["ANY "+path] || route.Method == "ANY" && availablePaths[path] {
			report.Covered = append(report.Covered, route)
		} else {
			report.Missing = append(report.Missing, route)
		}
	}
	sort.Slice(report.Missing, func(i, j int) bool {
		if report.Missing[i].Path == report.Missing[j].Path {
			return report.Missing[i].Method < report.Missing[j].Method
		}
		return report.Missing[i].Path < report.Missing[j].Path
	})
	return report
}
func (r Report) JSON() ([]byte, error) { return json.MarshalIndent(r, "", "  ") }
