//go:build !windows

package legacyexec

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

func shellCommand(command string) *exec.Cmd {
	path, err := exec.LookPath("sh")
	if err != nil {
		path = "/system/bin/sh"
	}
	return exec.Command(path, "-c", command)
}

func prepareBackgroundCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func stopBackgroundCommand(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return os.ErrProcessDone
	}
	return err
}
