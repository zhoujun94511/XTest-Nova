package overlaypermission

import (
	"context"
	"fmt"
	"strings"
)

const Package = "com.openatx.xtest.popup"

type Executor interface {
	Run(context.Context, string, ...string) (string, error)
}

// Grant updates both package and UID modes. Some Android 16 vendor builds retain
// an ignored UID mode across reinstall, which overrides an allowed package mode.
func Grant(ctx context.Context, executor Executor) error {
	commands := [][]string{
		{"set", Package, "SYSTEM_ALERT_WINDOW", "allow"},
		{"set", "--uid", Package, "SYSTEM_ALERT_WINDOW", "allow"},
	}
	for _, args := range commands {
		output, err := executor.Run(ctx, "appops", args...)
		if err == nil {
			continue
		}
		if vendorBlocksAppOps(output, err) {
			// ColorOS and some Android 15 shells cannot MANAGE_APP_OPS_MODES.
			// Keep going so bootstrap can finish; the user grants overlay in Settings.
			continue
		}
		return fmt.Errorf("appops %s: %s: %w", strings.Join(args, " "), strings.TrimSpace(output), err)
	}
	return nil
}

// Allowed reports whether SYSTEM_ALERT_WINDOW is currently allow.
func Allowed(ctx context.Context, executor Executor) bool {
	output, err := executor.Run(ctx, "appops", "get", Package, "SYSTEM_ALERT_WINDOW")
	if err != nil {
		return false
	}
	return ParseAllowed(output)
}

// ParseAllowed reads `appops get` output.
func ParseAllowed(output string) bool {
	for _, line := range strings.Split(output, "\n") {
		line = strings.ToLower(strings.TrimSpace(line))
		if strings.HasPrefix(line, "system_alert_window:") {
			return strings.Contains(line, "allow")
		}
	}
	return false
}

// OpenSettings opens the AOSP overlay page. Android 11+ ignores package
// data and shows the app list; ROM-specific shortcuts stay out of this path.
func OpenSettings(ctx context.Context, executor Executor) error {
	intents := [][]string{
		{"start", "-a", "android.settings.action.MANAGE_OVERLAY_PERMISSION", "-d", "package:" + Package},
		{"start", "-a", "android.settings.action.MANAGE_OVERLAY_PERMISSION"},
		{"start", "-n", "com.android.settings/.Settings$OverlaySettingsActivity"},
		{"start", "-a", "android.settings.APPLICATION_DETAILS_SETTINGS", "-d", "package:" + Package},
	}
	var last error
	for _, args := range intents {
		output, err := executor.Run(ctx, "am", args...)
		if err == nil && !settingsStartFailed(output) {
			return nil
		}
		if err != nil {
			last = fmt.Errorf("am %s: %s: %w", strings.Join(args, " "), strings.TrimSpace(output), err)
			continue
		}
		last = fmt.Errorf("am %s: %s", strings.Join(args, " "), strings.TrimSpace(output))
	}
	if last == nil {
		return fmt.Errorf("no overlay settings activity available")
	}
	return last
}

func settingsStartFailed(output string) bool {
	text := strings.ToLower(output)
	return strings.Contains(text, "error type") || strings.Contains(text, "unable to resolve") || strings.Contains(text, "does not exist") || (strings.Contains(text, "activity class") && strings.Contains(text, "does not exist"))
}

// StartOverlayService starts Companion's overlay service after the user
// has granted SYSTEM_ALERT_WINDOW. Used when the launcher activity is
// covered by Settings.
func StartOverlayService(ctx context.Context, executor Executor) error {
	commands := [][]string{
		{"start-foreground-service", "-n", Package + "/.OverlayService"},
		{"startservice", "-n", Package + "/.OverlayService"},
	}
	var last error
	for _, args := range commands {
		output, err := executor.Run(ctx, "am", args...)
		if err == nil && !settingsStartFailed(output) {
			return nil
		}
		if err != nil {
			last = fmt.Errorf("am %s: %s: %w", strings.Join(args, " "), strings.TrimSpace(output), err)
			continue
		}
		last = fmt.Errorf("am %s: %s", strings.Join(args, " "), strings.TrimSpace(output))
	}
	if last == nil {
		return fmt.Errorf("unable to start overlay service")
	}
	return last
}

func vendorBlocksAppOps(output string, err error) bool {
	text := strings.ToLower(strings.TrimSpace(output))
	if err != nil {
		text += " " + strings.ToLower(err.Error())
	}
	return strings.Contains(text, "manage_app_ops_modes") || strings.Contains(text, "securityexception")
}
