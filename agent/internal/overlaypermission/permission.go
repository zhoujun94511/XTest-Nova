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
		if err != nil {
			return fmt.Errorf("appops %s: %s: %w", strings.Join(args, " "), strings.TrimSpace(output), err)
		}
	}
	return nil
}
