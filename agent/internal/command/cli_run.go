package command

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"
)

// Run executes a device-local CLI command against the Agent on loopback.
// Owner tokens are read only to authorize stop requests and are removed from output.
func Run(ctx context.Context, invocation Invocation, baseURL string, client *http.Client, output io.Writer) error {
	if client == nil {
		client = &http.Client{}
	}
	runner := &cliRunner{baseURL: strings.TrimRight(baseURL, "/"), client: client, output: output, json: invocation.JSON}
	switch invocation.Kind {
	case Doctor:
		return runner.doctor(ctx)
	case DeviceInfo:
		return runner.showJSON(ctx, http.MethodGet, "/v1/device", nil, func(raw []byte) string {
			var info struct {
				Brand string `json:"brand"`
				Model string `json:"model"`
				SDK   int    `json:"sdk"`
			}
			_ = json.Unmarshal(raw, &info)
			return fmt.Sprintf("brand=%s model=%s sdk=%d\n", info.Brand, info.Model, info.SDK)
		})
	case Foreground:
		return runner.showJSON(ctx, http.MethodGet, "/foregroundPkg", nil, func(raw []byte) string {
			var body struct {
				Package string `json:"package"`
			}
			_ = json.Unmarshal(raw, &body)
			return fmt.Sprintf("package=%s\n", body.Package)
		})
	case Apps:
		return runner.apps(ctx, invocation)
	case ScreenshotCmd:
		return runner.saveBinary(ctx, http.MethodGet, "/screenshot", nil, invocation.TargetPath)
	case Monkey:
		return runner.monkey(ctx, invocation)
	case Explore:
		return runner.explore(ctx, invocation)
	case Record:
		return runner.record(ctx, invocation)
	case Replay:
		return runner.replay(ctx, invocation)
	case Perf:
		return runner.perf(ctx, invocation)
	case Artifacts:
		endpoint := "/v1/diagnostics/artifacts"
		if invocation.PackageName != "" {
			endpoint += "?package=" + url.QueryEscape(invocation.PackageName)
		}
		return runner.showJSON(ctx, http.MethodGet, endpoint, nil, formatArtifacts)
	case Pull:
		return runner.pull(ctx, invocation.TargetPath, invocation.DestPath)
	default:
		return fmt.Errorf("unsupported command")
	}
}

type cliRunner struct {
	baseURL string
	client  *http.Client
	output  io.Writer
	json    bool
}

type executionIdentity struct {
	SessionID  string `json:"sessionId"`
	OwnerToken string `json:"ownerToken"`
}

func (r *cliRunner) doctor(ctx context.Context) error {
	raw, _, err := r.request(ctx, http.MethodGet, "/v1/health", nil, nil)
	if err != nil {
		return err
	}
	if r.json {
		return r.writeRaw(raw)
	}
	var body struct {
		Status  string `json:"status"`
		Version string `json:"version"`
	}
	_ = json.Unmarshal(raw, &body)
	_, err = fmt.Fprintf(r.output, "status=%s version=%s\n", body.Status, body.Version)
	return err
}

func (r *cliRunner) apps(ctx context.Context, invocation Invocation) error {
	switch invocation.Action {
	case "list":
		return r.showJSON(ctx, http.MethodGet, "/packages", nil, formatPackages)
	case "info":
		return r.showJSON(ctx, http.MethodGet, "/packages/"+url.PathEscape(invocation.PackageName)+"/info", nil, formatPackageInfo)
	case "launch":
		return r.showJSON(ctx, http.MethodPost, "/v1/apps/"+url.PathEscape(invocation.PackageName)+"/launch", nil, func(raw []byte) string {
			return "launched " + invocation.PackageName + "\n"
		})
	default:
		return fmt.Errorf("unsupported apps action %q", invocation.Action)
	}
}

func (r *cliRunner) monkey(ctx context.Context, invocation Invocation) error {
	switch invocation.MonkeyAction {
	case "status":
		return r.showJSON(ctx, http.MethodGet, monkeyEndpoint(invocation.Engine), nil, formatSession)
	case "stop":
		return r.stopOwned(ctx, monkeyEndpoint(invocation.Engine))
	case "report":
		return r.showJSON(ctx, http.MethodGet, "/v1/monkey/report", nil, formatVerdict)
	case "start":
		body, err := json.Marshal(monkeyStartBody(invocation))
		if err != nil {
			return err
		}
		return r.showJSON(ctx, http.MethodPost, "/v1/monkey/runs/current", body, formatSession)
	default:
		return fmt.Errorf("unsupported monkey action %q", invocation.MonkeyAction)
	}
}

func (r *cliRunner) explore(ctx context.Context, invocation Invocation) error {
	switch invocation.Action {
	case "preview":
		return r.preview(ctx, invocation.PackageName)
	case "start":
		if !invocation.Execute {
			if r.json {
				return r.previewOnlyJSON(ctx, invocation.PackageName)
			}
			if err := r.preview(ctx, invocation.PackageName); err != nil {
				return err
			}
			_, err := fmt.Fprintln(r.output, "execute is not set; preview only")
			return err
		}
		body, err := json.Marshal(exploreStartBody(invocation))
		if err != nil {
			return err
		}
		return r.showJSON(ctx, http.MethodPost, "/v1/exploration/sessions", body, formatSession)
	case "status":
		return r.showJSON(ctx, http.MethodGet, "/v1/exploration/sessions/current", nil, formatSession)
	case "stop":
		return r.stopOwned(ctx, "/v1/exploration/sessions/current")
	default:
		return fmt.Errorf("unsupported explore action %q", invocation.Action)
	}
}

func monkeyStartBody(invocation Invocation) map[string]any {
	body := traversalGuardBody(invocation)
	body["package"] = invocation.PackageName
	body["durationSeconds"] = floorDurationSeconds(invocation.DurationSeconds)
	body["throttleMillis"] = throttleOrDefault(invocation.ThrottleMillis)
	if invocation.Engine != "" {
		body["engine"] = invocation.Engine
	}
	return body
}

func monkeyEndpoint(engine string) string {
	if engine == "" {
		return "/v1/monkey/runs/current"
	}
	return "/v1/monkey/runs/current?engine=" + url.QueryEscape(engine)
}

func exploreStartBody(invocation Invocation) map[string]any {
	body := traversalGuardBody(invocation)
	body["package"] = invocation.PackageName
	body["execute"] = true
	body["enableScroll"] = true
	body["enableBacktrack"] = true
	body["recoverPopups"] = true
	body["durationSeconds"] = floorDurationSeconds(invocation.DurationSeconds)
	body["intervalMillis"] = throttleOrDefault(invocation.ThrottleMillis)
	return body
}

func floorDurationSeconds(seconds int) int {
	if seconds < 60 {
		return 60
	}
	return seconds
}

func throttleOrDefault(millis int) int {
	if millis == 0 {
		return 500
	}
	return millis
}

func traversalGuardBody(invocation Invocation) map[string]any {
	mode := invocation.ActivityMode
	if mode == "" {
		mode = "none"
	}
	render := invocation.RenderFallbackMode
	if render == "" {
		render = "bounded"
	}
	activities := invocation.Activities
	if activities == nil {
		activities = []string{}
	}
	blocked := invocation.ControlBlacklist
	if blocked == nil {
		blocked = []string{}
	}
	cases := invocation.TargetCases
	if cases == nil {
		cases = []TargetCase{}
	}
	return map[string]any{
		"renderFallbackMode": render,
		"lowBatteryExit":     invocation.LowBatteryExit,
		"minBatteryPercent":  invocation.MinBattery,
		"activityMode":       mode,
		"activities":         activities,
		"controlBlacklist":   blocked,
		"targetCases":        cases,
	}
}

func (r *cliRunner) previewOnlyJSON(ctx context.Context, packageName string) error {
	body, err := json.Marshal(map[string]string{"package": packageName})
	if err != nil {
		return err
	}
	raw, _, err := r.request(ctx, http.MethodPost, "/v1/exploration/preview", body, nil)
	if err != nil {
		return err
	}
	var response any
	if err := json.Unmarshal(raw, &response); err != nil {
		return r.writeRaw(raw)
	}
	encoded, err := json.Marshal(map[string]any{"previewOnly": true, "response": response})
	if err != nil {
		return err
	}
	return r.writeRaw(encoded)
}

func (r *cliRunner) preview(ctx context.Context, packageName string) error {
	body, err := json.Marshal(map[string]string{"package": packageName})
	if err != nil {
		return err
	}
	return r.showJSON(ctx, http.MethodPost, "/v1/exploration/preview", body, func(raw []byte) string {
		var body struct {
			Analysis struct {
				Package   string `json:"package"`
				Activity  string `json:"activity"`
				NodeCount int    `json:"nodeCount"`
				Actions   []any  `json:"actions"`
			} `json:"analysis"`
		}
		_ = json.Unmarshal(raw, &body)
		return fmt.Sprintf("preview package=%s activity=%s nodes=%d actions=%d\n", body.Analysis.Package, body.Analysis.Activity, body.Analysis.NodeCount, len(body.Analysis.Actions))
	})
}

func (r *cliRunner) record(ctx context.Context, invocation Invocation) error {
	switch invocation.Action {
	case "start":
		payload := map[string]string{"package": invocation.PackageName, "name": invocation.Name}
		if invocation.Task != "" {
			payload["task"] = invocation.Task
		}
		body, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		return r.showJSON(ctx, http.MethodPost, "/v1/recordings", body, formatSession)
	case "status":
		return r.showJSON(ctx, http.MethodGet, "/v1/recordings/current", nil, formatSession)
	case "stop":
		return r.stopOwned(ctx, "/v1/recordings/current")
	case "cases":
		endpoint := "/v1/recordings"
		if invocation.PackageName != "" {
			endpoint += "?package=" + url.QueryEscape(invocation.PackageName)
		}
		return r.showJSON(ctx, http.MethodGet, endpoint, nil, formatCases)
	default:
		return fmt.Errorf("unsupported record action %q", invocation.Action)
	}
}

func (r *cliRunner) replay(ctx context.Context, invocation Invocation) error {
	switch invocation.Action {
	case "start":
		if !invocation.Execute {
			_, err := fmt.Fprintln(r.output, "execute is not set; replay was not started")
			return err
		}
		return r.startReplay(ctx, invocation.PackageName, invocation.Name)
	case "status":
		return r.showJSON(ctx, http.MethodGet, "/v1/replays/current", nil, formatSession)
	case "stop":
		return r.stopOwned(ctx, "/v1/replays/current")
	default:
		return fmt.Errorf("unsupported replay action %q", invocation.Action)
	}
}

func (r *cliRunner) startReplay(ctx context.Context, packageName, name string) error {
	raw, _, err := r.request(ctx, http.MethodGet, "/v1/recordings?package="+url.QueryEscape(packageName), nil, nil)
	if err != nil {
		return err
	}
	var listed struct {
		Cases []struct {
			ID         string    `json:"id"`
			Name       string    `json:"name"`
			Package    string    `json:"package"`
			RecordedAt time.Time `json:"recordedAt"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(raw, &listed); err != nil {
		return fmt.Errorf("decode recording cases: %w", err)
	}
	var selected string
	var selectedAt time.Time
	for _, item := range listed.Cases {
		if item.Package == packageName && item.Name == name && (selected == "" || item.RecordedAt.After(selectedAt)) {
			selected = item.ID
			selectedAt = item.RecordedAt
		}
	}
	if selected == "" {
		return fmt.Errorf("recording %s for %s was not found", name, packageName)
	}
	caseRaw, _, err := r.request(ctx, http.MethodGet, "/v1/recordings/cases/"+url.PathEscape(selected), nil, nil)
	if err != nil {
		return err
	}
	var saved json.RawMessage
	if err := json.Unmarshal(caseRaw, &saved); err != nil {
		return fmt.Errorf("decode recording case: %w", err)
	}
	body, err := json.Marshal(map[string]any{"execute": true, "case": saved})
	if err != nil {
		return err
	}
	return r.showJSON(ctx, http.MethodPost, "/v1/replays", body, formatSession)
}

func (r *cliRunner) perf(ctx context.Context, invocation Invocation) error {
	switch invocation.Action {
	case "start":
		body, err := json.Marshal(map[string]any{"package": invocation.PackageName})
		if err != nil {
			return err
		}
		return r.showJSON(ctx, http.MethodPost, "/v1/performance/sessions", body, formatPerf)
	case "status":
		return r.showJSON(ctx, http.MethodGet, "/v1/performance/sessions/current", nil, formatPerf)
	case "stop":
		return r.stopOwned(ctx, "/v1/performance/sessions/current")
	default:
		return fmt.Errorf("unsupported perf action %q", invocation.Action)
	}
}

func (r *cliRunner) stopOwned(ctx context.Context, endpoint string) error {
	raw, _, err := r.request(ctx, http.MethodGet, endpoint, nil, nil)
	if err != nil {
		return err
	}
	var state struct {
		Running    bool              `json:"running"`
		Finalizing bool              `json:"finalizing"`
		Identity   executionIdentity `json:"identity"`
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		return fmt.Errorf("decode session: %w", err)
	}
	if !state.Running {
		if r.json {
			return r.writeRaw(raw)
		}
		_, err = fmt.Fprintf(r.output, "running=false finalizing=%t\n", state.Finalizing)
		return err
	}
	if state.Identity.SessionID == "" || state.Identity.OwnerToken == "" {
		return fmt.Errorf("current session has no ownership credentials")
	}
	headers := map[string]string{
		"X-XTest-Session-Id":  state.Identity.SessionID,
		"X-XTest-Owner-Token": state.Identity.OwnerToken,
	}
	stopped, _, err := r.request(ctx, http.MethodDelete, endpoint, nil, headers)
	if err != nil {
		return err
	}
	if r.json {
		return r.writeRaw(stopped)
	}
	_, err = fmt.Fprintln(r.output, "stop requested")
	return err
}

func (r *cliRunner) pull(ctx context.Context, directory, destination string) error {
	source, err := allowedDevicePath(directory)
	if err != nil {
		return err
	}
	target, err := allowedDevicePath(destination)
	if err != nil {
		return err
	}
	endpoint := archiveEndpoint(source)
	raw, _, err := r.requestBounded(ctx, http.MethodGet, endpoint, nil, nil, maxCLIArchiveBytes)
	if err != nil {
		return err
	}
	if err := writeDeviceFile(target, raw); err != nil {
		return err
	}
	if r.json {
		encoded, err := json.Marshal(map[string]any{"path": target, "bytes": len(raw)})
		if err != nil {
			return err
		}
		return r.writeRaw(encoded)
	}
	_, err = fmt.Fprintf(r.output, "wrote %s bytes=%d\n", target, len(raw))
	return err
}

func (r *cliRunner) saveBinary(ctx context.Context, method, endpoint string, body []byte, destination string) error {
	target, err := allowedDevicePath(destination)
	if err != nil {
		return err
	}
	raw, _, err := r.request(ctx, method, endpoint, body, nil)
	if err != nil {
		return err
	}
	if err := writeDeviceFile(target, raw); err != nil {
		return err
	}
	if r.json {
		encoded, err := json.Marshal(map[string]any{"path": target, "bytes": len(raw)})
		if err != nil {
			return err
		}
		return r.writeRaw(encoded)
	}
	_, err = fmt.Fprintf(r.output, "wrote %s bytes=%d\n", target, len(raw))
	return err
}

func (r *cliRunner) showJSON(ctx context.Context, method, endpoint string, body []byte, text func([]byte) string) error {
	raw, _, err := r.request(ctx, method, endpoint, body, nil)
	if err != nil {
		return err
	}
	if r.json {
		return r.writeRaw(raw)
	}
	_, err = io.WriteString(r.output, text(raw))
	return err
}

func (r *cliRunner) request(ctx context.Context, method, endpoint string, body []byte, headers map[string]string) ([]byte, int, error) {
	return r.requestBounded(ctx, method, endpoint, body, headers, maxCLIJSONBytes)
}

func (r *cliRunner) writeRaw(raw []byte) error {
	redacted := redactJSON(raw)
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, redacted, "", "  "); err != nil {
		_, err = r.output.Write(append(redacted, '\n'))
		return err
	}
	if pretty.Len() == 0 || pretty.Bytes()[pretty.Len()-1] != '\n' {
		pretty.WriteByte('\n')
	}
	_, err := r.output.Write(pretty.Bytes())
	return err
}

const (
	maxCLIJSONBytes    int64 = 32 << 20
	maxCLIArchiveBytes       = int64(2 << 30)
)

func (r *cliRunner) requestBounded(ctx context.Context, method, endpoint string, body []byte, headers map[string]string, limit int64) ([]byte, int, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequestWithContext(ctx, method, r.baseURL+endpoint, reader)
	if err != nil {
		return nil, 0, err
	}
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := r.client.Do(request)
	if err != nil {
		return nil, 0, fmt.Errorf("agent is not reachable at %s: %w", r.baseURL, err)
	}
	defer response.Body.Close()
	payload, err := readBounded(response.Body, limit)
	if err != nil {
		return nil, response.StatusCode, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, response.StatusCode, fmt.Errorf("HTTP %d: %s", response.StatusCode, errorText(payload))
	}
	return payload, response.StatusCode, nil
}

func readBounded(reader io.Reader, limit int64) ([]byte, error) {
	payload, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if int64(len(payload)) > limit {
		return nil, fmt.Errorf("response exceeds %d bytes", limit)
	}
	return payload, nil
}

func archiveEndpoint(source string) string {
	parts := strings.Split(strings.TrimPrefix(source, "/"), "/")
	escaped := make([]string, len(parts))
	for index, part := range parts {
		escaped[index] = url.PathEscape(part)
	}
	return "/archive/" + strings.Join(escaped, "/")
}

func redactJSON(raw []byte) []byte {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return raw
	}
	scrub(value)
	encoded, err := json.Marshal(value)
	if err != nil {
		return raw
	}
	return encoded
}

func scrub(value any) {
	switch typed := value.(type) {
	case map[string]any:
		delete(typed, "ownerToken")
		for _, child := range typed {
			scrub(child)
		}
	case []any:
		for _, child := range typed {
			scrub(child)
		}
	}
}

func errorText(raw []byte) string {
	redacted := redactJSON(raw)
	var body struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(redacted, &body) == nil && strings.TrimSpace(body.Error) != "" {
		return body.Error
	}
	text := strings.TrimSpace(string(redacted))
	if len(text) > 512 {
		return text[:512]
	}
	return text
}

func formatSession(raw []byte) string {
	var state struct {
		Package    string `json:"package"`
		Running    bool   `json:"running"`
		Finalizing bool   `json:"finalizing"`
		RequestID  string `json:"requestId"`
		StopReason string `json:"stopReason"`
	}
	_ = json.Unmarshal(raw, &state)
	return fmt.Sprintf("running=%t finalizing=%t package=%s requestId=%s stopReason=%s\n", state.Running, state.Finalizing, state.Package, state.RequestID, state.StopReason)
}

func formatPerf(raw []byte) string {
	var state struct {
		Package string `json:"package"`
		Running bool   `json:"running"`
		Rows    int    `json:"rows"`
		Path    string `json:"path"`
	}
	_ = json.Unmarshal(raw, &state)
	return fmt.Sprintf("running=%t package=%s rows=%d path=%s\n", state.Running, state.Package, state.Rows, state.Path)
}

func formatVerdict(raw []byte) string {
	var report struct {
		Package string `json:"package"`
		Verdict struct {
			Status string `json:"status"`
			Reason string `json:"reason"`
		} `json:"verdict"`
	}
	_ = json.Unmarshal(raw, &report)
	return fmt.Sprintf("package=%s verdict=%s reason=%s\n", report.Package, report.Verdict.Status, report.Verdict.Reason)
}

func formatPackages(raw []byte) string {
	var packages []struct {
		PackageName string `json:"packageName"`
		Name        string `json:"name"`
	}
	if err := json.Unmarshal(raw, &packages); err != nil {
		return string(redactJSON(raw)) + "\n"
	}
	var builder strings.Builder
	for _, item := range packages {
		fmt.Fprintf(&builder, "%s %s\n", item.PackageName, item.Name)
	}
	return builder.String()
}

func formatPackageInfo(raw []byte) string {
	var body struct {
		Data struct {
			PackageName string `json:"packageName"`
			VersionName string `json:"versionName"`
			VersionCode int64  `json:"versionCode"`
		} `json:"data"`
	}
	_ = json.Unmarshal(raw, &body)
	return fmt.Sprintf("package=%s versionName=%s versionCode=%d\n", body.Data.PackageName, body.Data.VersionName, body.Data.VersionCode)
}

func formatCases(raw []byte) string {
	var body struct {
		Cases []struct {
			Name    string `json:"name"`
			Package string `json:"package"`
			Actions int    `json:"actions"`
		} `json:"cases"`
	}
	_ = json.Unmarshal(raw, &body)
	var builder strings.Builder
	for _, item := range body.Cases {
		fmt.Fprintf(&builder, "%s %s actions=%d\n", item.Package, item.Name, item.Actions)
	}
	return builder.String()
}

func formatArtifacts(raw []byte) string {
	var body struct {
		Sessions []struct {
			Package string `json:"package"`
			Kind    string `json:"kind"`
			Name    string `json:"name"`
			Path    string `json:"path"`
		} `json:"sessions"`
	}
	_ = json.Unmarshal(raw, &body)
	var builder strings.Builder
	for _, item := range body.Sessions {
		fmt.Fprintf(&builder, "%s %s %s %s\n", item.Package, item.Kind, item.Name, item.Path)
	}
	return builder.String()
}

func allowedDevicePath(value string) (string, error) {
	if value == "" || !strings.HasPrefix(value, "/") {
		return "", fmt.Errorf("path must be absolute")
	}
	clean := path.Clean(value)
	roots := []string{"/data/local/tmp", "/sdcard", "/storage/emulated/0"}
	for _, root := range roots {
		if clean == root || strings.HasPrefix(clean, root+"/") {
			return clean, nil
		}
	}
	return "", fmt.Errorf("path is outside allowed roots")
}

func writeDeviceFile(target string, payload []byte) error {
	if err := os.MkdirAll(path.Dir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, payload, 0o644)
}
