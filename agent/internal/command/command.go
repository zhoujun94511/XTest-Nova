package command

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/zhoujun94511/xtest-nova/agent/internal/device"
	"github.com/zhoujun94511/xtest-nova/agent/internal/overlaypermission"
	"github.com/zhoujun94511/xtest-nova/agent/internal/platform"
)

type Kind int

const (
	Server Kind = iota
	Popup
	Monkey
	Version
)

type Invocation struct {
	Kind         Kind
	Daemon       bool
	Stop         bool
	PopupAction  string
	MonkeyAction string
	ServerArgs   []string
}

func Parse(args []string) (Invocation, error) {
	if len(args) == 0 {
		return Invocation{Kind: Server}, nil
	}
	switch args[0] {
	case "version":
		if len(args) != 1 {
			return Invocation{}, fmt.Errorf("version does not accept arguments")
		}
		return Invocation{Kind: Version}, nil
	case "popup":
		if len(args) != 2 {
			return Invocation{}, fmt.Errorf("usage: xtest-nova-agent popup start|status|uninstall")
		}
		action := strings.ToLower(args[1])
		if action != "start" && action != "status" && action != "uninstall" {
			return Invocation{}, fmt.Errorf("unsupported popup action %q", args[1])
		}
		return Invocation{Kind: Popup, PopupAction: action}, nil
	case "monkey":
		if len(args) != 2 {
			return Invocation{}, fmt.Errorf("usage: xtest-nova-agent monkey stop|status")
		}
		action := strings.ToLower(args[1])
		if action != "stop" && action != "status" {
			return Invocation{}, fmt.Errorf("unsupported monkey action %q", args[1])
		}
		return Invocation{Kind: Monkey, MonkeyAction: action}, nil
	case "server":
		result := Invocation{Kind: Server}
		for _, arg := range args[1:] {
			switch arg {
			case "-d", "--daemon":
				result.Daemon = true
			case "--stop":
				result.Stop = true
			default:
				result.ServerArgs = append(result.ServerArgs, arg)
			}
		}
		return result, nil
	default:
		if strings.HasPrefix(args[0], "-") {
			return Invocation{Kind: Server, ServerArgs: args}, nil
		}
		return Invocation{}, fmt.Errorf("unknown command %q", args[0])
	}
}

const DefaultAgentURL = "http://127.0.0.1:7912"

type monkeyCommandState struct {
	Running    bool   `json:"running"`
	Finalizing bool   `json:"finalizing"`
	RequestID  string `json:"requestId"`
	StopReason string `json:"stopReason"`
	Identity   struct {
		SessionID  string `json:"sessionId"`
		OwnerToken string `json:"ownerToken"`
	} `json:"identity"`
}

// ManageMonkey provides a device-local task control command. It intentionally
// goes through the HTTP ownership contract instead of touching the stop marker
// or killing the Runner process, so stale clients cannot stop a newer run and
// the normal artifact/overlay finalization path remains intact.
func ManageMonkey(ctx context.Context, action, baseURL string, client *http.Client, output io.Writer) (err error) {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/v1/monkey/runs/current"
	state, err := readMonkeyCommandState(ctx, client, endpoint)
	if err != nil {
		return fmt.Errorf("query Monkey state: %w", err)
	}
	if action == "status" {
		_, err = fmt.Fprintf(output, "running=%t finalizing=%t requestId=%s stopReason=%s\n", state.Running, state.Finalizing, state.RequestID, state.StopReason)
		return err
	}
	if action != "stop" {
		return fmt.Errorf("unsupported monkey action %q", action)
	}
	if !state.Running {
		_, err = fmt.Fprintf(output, "Monkey is not running (finalizing=%t)\n", state.Finalizing)
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("X-XTest-Session-Id", state.Identity.SessionID)
	request.Header.Set("X-XTest-Owner-Token", state.Identity.OwnerToken)
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("request Monkey stop: %w", err)
	}
	defer func() {
		if closeErr := response.Body.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close Monkey stop response: %w", closeErr)
		}
	}()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 8<<10))
		return fmt.Errorf("request Monkey stop: HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	_, err = fmt.Fprintln(output, "Monkey stop requested; artifacts will finish and the overlay will restore automatically")
	return err
}

func readMonkeyCommandState(ctx context.Context, client *http.Client, endpoint string) (state monkeyCommandState, err error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return monkeyCommandState{}, err
	}
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return monkeyCommandState{}, err
	}
	defer func() {
		if closeErr := response.Body.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close Monkey state response: %w", closeErr)
		}
	}()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 8<<10))
		return monkeyCommandState{}, fmt.Errorf("HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 128<<10)).Decode(&state); err != nil {
		return monkeyCommandState{}, fmt.Errorf("decode response: %w", err)
	}
	return state, nil
}

const (
	PopupPackage     = overlaypermission.Package
	PopupActivity    = PopupPackage + "/.PopupLauncherActivity"
	PopupAPK         = "/data/local/tmp/xtest-nova-companion.apk"
	PopupVersionCode = 30718
)

func ManagePopup(ctx context.Context, action string, executor platform.Executor, output io.Writer) error {
	path, installed, versionCode, running := popupStatus(ctx, executor)
	sdkText, sdkErr := executor.Run(ctx, "getprop", "ro.build.version.sdk")
	sdk, _ := strconv.Atoi(strings.TrimSpace(sdkText))
	switch action {
	case "status":
		_, err := fmt.Fprintf(output, "package=%s installed=%t running=%t versionCode=%d expectedVersionCode=%d sdk=%d sdkError=%v payload=%s installedPath=%s\n", PopupPackage, installed, running, versionCode, PopupVersionCode, sdk, sdkErr, PopupAPK, path)
		return err
	case "start":
		if sdk < 28 {
			return fmt.Errorf("popup requires Android SDK 28 or newer (device SDK %d)", sdk)
		}
		if !installed || versionCode != PopupVersionCode {
			value, err := executor.Run(ctx, "pm", "install", "-r", PopupAPK)
			if err != nil {
				return fmt.Errorf("install popup: %s: %w", value, err)
			}
		}
		if err := overlaypermission.Grant(ctx, executor); err != nil {
			return fmt.Errorf("grant popup overlay permission: %w", err)
		}
		startArgs := []string{"start", "-n", PopupActivity}
		if target, targetErr := device.New(executor, 2*time.Second).ForegroundPackage(ctx); targetErr == nil && target != "" && target != PopupPackage {
			startArgs = append(startArgs, "--es", "target_package", target)
		}
		value, err := executor.Run(ctx, "am", startArgs...)
		if err != nil {
			return fmt.Errorf("start popup: %s: %w", value, err)
		}
		deadline := time.Now().Add(3 * time.Second)
		for {
			_, _, _, running = popupStatus(ctx, executor)
			if running {
				_, err = fmt.Fprintln(output, "popup overlay started")
				return err
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("popup launcher completed but overlay service is not running")
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(100 * time.Millisecond):
			}
		}
	case "uninstall":
		if !installed {
			_, err := fmt.Fprintln(output, "popup is not installed")
			return err
		}
		value, err := executor.Run(ctx, "pm", "uninstall", PopupPackage)
		if err != nil {
			return fmt.Errorf("uninstall popup: %s: %w", value, err)
		}
		_, err = fmt.Fprintln(output, strings.TrimSpace(value))
		return err
	default:
		return fmt.Errorf("unsupported popup action %q", action)
	}
}

var versionCodePattern = regexp.MustCompile(`versionCode=(\d+)`)

func popupStatus(ctx context.Context, executor platform.Executor) (string, bool, int, bool) {
	value, err := executor.Run(ctx, "pm", "path", PopupPackage)
	if err != nil {
		return "", false, 0, false
	}
	value = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(value), "package:"))
	if value == "" {
		return "", false, 0, false
	}
	dump, _ := executor.Run(ctx, "dumpsys", "package", PopupPackage)
	match := versionCodePattern.FindStringSubmatch(dump)
	versionCode := 0
	if len(match) == 2 {
		versionCode, _ = strconv.Atoi(match[1])
	}
	services, _ := executor.Run(ctx, "dumpsys", "activity", "services", PopupPackage)
	running := strings.Contains(services, PopupPackage+".OverlayService") || strings.Contains(services, "OverlayService")
	return value, true, versionCode, running
}
