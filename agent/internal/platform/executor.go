package platform

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

type Executor interface {
	Run(context.Context, string, ...string) (string, error)
	RunBytes(context.Context, string, ...string) ([]byte, error)
}

func (OSExecutor) RunBytes(ctx context.Context, name string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, args...)
	output, err := command.Output()
	var exit *exec.ExitError
	if errors.As(err, &exit) && len(exit.Stderr) > 0 {
		return output, fmt.Errorf("%s: %w", strings.TrimSpace(string(exit.Stderr)), err)
	}
	return output, err
}

type OSExecutor struct{}

func (OSExecutor) Run(ctx context.Context, name string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	if err != nil && strings.TrimSpace(stderr.String()) != "" {
		return strings.TrimSpace(stdout.String()), fmt.Errorf("%s: %w", strings.TrimSpace(stderr.String()), err)
	}
	return strings.TrimSpace(stdout.String()), err
}
