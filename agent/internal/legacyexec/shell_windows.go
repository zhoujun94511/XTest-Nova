//go:build windows

package legacyexec

import (
	"os"
	"os/exec"
)

func shellCommand(command string) *exec.Cmd {
	path := os.Getenv("COMSPEC")
	if path == "" {
		path = "cmd.exe"
	}
	return exec.Command(path, "/d", "/s", "/c", command)
}

func prepareBackgroundCommand(_ *exec.Cmd) {}

func stopBackgroundCommand(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}
