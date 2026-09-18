package companioncontrol

import (
	"context"
	"fmt"
	"strings"
)

const metadataURI = "content://com.openatx.xtest.popup.metadata/control"

type Executor interface {
	Run(context.Context, string, ...string) (string, error)
}

func SuppressOverlay(ctx context.Context, executor Executor) error {
	return setOverlayState(ctx, executor, "overlay-suppress")
}

func RestoreOverlay(ctx context.Context, executor Executor) error {
	return setOverlayState(ctx, executor, "overlay-restore")
}

func setOverlayState(ctx context.Context, executor Executor, method string) error {
	if executor == nil {
		return fmt.Errorf("companion overlay executor is unavailable")
	}
	output, err := executor.Run(ctx, "content", "call", "--uri", metadataURI, "--method", method)
	if err != nil {
		return fmt.Errorf("%s: %w", method, err)
	}
	if strings.Contains(strings.ToLower(output), "error") {
		return fmt.Errorf("%s: %s", method, strings.TrimSpace(output))
	}
	return nil
}
