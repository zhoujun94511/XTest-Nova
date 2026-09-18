package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const pidPath = "/data/local/tmp/xtest-nova-agent.pid"

func claimPID() (func(), error) {
	if pid, err := readPID(); err == nil {
		if processAlive(pid) && ownsProcess(pid) {
			return nil, fmt.Errorf("xtest-nova-agent server is already running with pid %d", pid)
		}
		_ = os.Remove(pidPath)
	}
	file, err := os.OpenFile(pidPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return nil, fmt.Errorf("write pid file: %w", err)
	}
	if _, err = file.WriteString(strconv.Itoa(os.Getpid())); err != nil {
		_ = file.Close()
		_ = os.Remove(pidPath)
		return nil, fmt.Errorf("write pid file: %w", err)
	}
	if err = file.Close(); err != nil {
		_ = os.Remove(pidPath)
		return nil, err
	}
	return func() {
		if pid, err := readPID(); err == nil && pid == os.Getpid() {
			_ = os.Remove(pidPath)
		}
	}, nil
}

func stopDaemon() error {
	pid, err := readPID()
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("xtest-nova-agent server is not running")
			return nil
		}
		return err
	}
	if !processAlive(pid) {
		_ = os.Remove(pidPath)
		fmt.Println("removed stale xtest-nova-agent pid file")
		return nil
	}
	if !ownsProcess(pid) {
		return fmt.Errorf("pid file points to non-xtest-nova-agent process %d; refusing to signal it", pid)
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if err = process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("stop pid %d: %w", pid, err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for processAlive(pid) && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	if processAlive(pid) {
		return fmt.Errorf("xtest-nova-agent pid %d did not stop", pid)
	}
	_ = os.Remove(pidPath)
	fmt.Printf("stopped xtest-nova-agent pid %d\n", pid)
	return nil
}

func readPID() (int, error) {
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return 0, fmt.Errorf("invalid pid file %s", pidPath)
	}
	return pid, nil
}

func processAlive(pid int) bool {
	process, err := os.FindProcess(pid)
	return err == nil && process.Signal(syscall.Signal(0)) == nil
}

func ownsProcess(pid int) bool {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		return false
	}
	fields := strings.Split(string(data), "\x00")
	return len(fields) > 0 && strings.Contains(fields[0], "xtest-nova-agent")
}
