package companioncontrol

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const metadataURI = "content://com.openatx.xtest.popup.metadata/control"

type Executor interface {
	Run(context.Context, string, ...string) (string, error)
}

// SuppressOverlay tucks a Companion overlay that is already on screen into
// the edge handle and reports whether the caller must restore the menu after
// the automation ends. An overlay that is absent or already minimized is left
// alone, so an API-started session does not launch the floating entry.
func SuppressOverlay(ctx context.Context, executor Executor) (bool, error) {
	state, err := readOverlayState(ctx, executor)
	if err != nil {
		return false, err
	}
	if state == "absent" || state == "suppressed" || state == "minimized" {
		return false, nil
	}
	if err := setOverlayState(ctx, executor, "overlay-suppress"); err != nil {
		return false, err
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		state, err = readOverlayState(ctx, executor)
		if err != nil {
			return false, err
		}
		if state == "suppressed" || state == "absent" || state == "minimized" {
			return true, nil
		}
		select {
		case <-ctx.Done():
			return false, fmt.Errorf("overlay-suppress: window still %s: %w", state, ctx.Err())
		case <-ticker.C:
		}
	}
}

func RestoreOverlay(ctx context.Context, executor Executor) error {
	return setOverlayState(ctx, executor, "overlay-restore")
}

func readOverlayState(ctx context.Context, executor Executor) (string, error) {
	output, err := callOverlay(ctx, executor, "overlay-status")
	if err != nil {
		return "", err
	}
	return parseOverlayState(output)
}

func setOverlayState(ctx context.Context, executor Executor, method string) error {
	_, err := callOverlay(ctx, executor, method)
	return err
}

func callOverlay(ctx context.Context, executor Executor, method string) (string, error) {
	if executor == nil {
		return "", fmt.Errorf("companion overlay executor is unavailable")
	}
	output, err := executor.Run(ctx, "content", "call", "--uri", metadataURI, "--method", method)
	if err != nil {
		return "", fmt.Errorf("%s: %w", method, err)
	}
	if strings.Contains(strings.ToLower(output), "error") {
		return "", fmt.Errorf("%s: %s", method, strings.TrimSpace(output))
	}
	return output, nil
}

func parseOverlayState(output string) (string, error) {
	lower := strings.ToLower(output)
	index := strings.Index(lower, "state=")
	if index < 0 {
		return "", fmt.Errorf("overlay-status: %s", strings.TrimSpace(output))
	}
	state := lower[index+len("state="):]
	if end := strings.IndexAny(state, "}]\n\r "); end >= 0 {
		state = state[:end]
	}
	state = strings.Trim(state, "\"'")
	if state == "" {
		return "", fmt.Errorf("overlay-status: %s", strings.TrimSpace(output))
	}
	return state, nil
}
