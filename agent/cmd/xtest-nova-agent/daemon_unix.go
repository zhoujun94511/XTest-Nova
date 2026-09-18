//go:build linux || android

package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/zhoujun94511/xtest-nova/agent/internal/buildinfo"
	"github.com/zhoujun94511/xtest-nova/agent/internal/config"
	"github.com/zhoujun94511/xtest-nova/agent/internal/logrotation"
)

func startDaemon(serverArgs []string) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	const logPath = "/data/local/tmp/xtest-nova-agent.log"
	if err = logrotation.Rotate(logPath, logrotation.Options{MaxBytes: 24 << 20, Backups: 3, MaxAge: 72 * time.Hour, Compress: true}, time.Now()); err != nil {
		return fmt.Errorf("rotate daemon log: %w", err)
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer logFile.Close()
	child := exec.Command(executable, append([]string{"server"}, serverArgs...)...)
	child.Env = append(os.Environ(), "XTEST_NOVA_DAEMON_LOG="+logPath)
	child.Stdout, child.Stderr = logFile, logFile
	child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err = child.Start(); err != nil {
		return err
	}
	cfg, err := config.Parse(serverArgs, io.Discard)
	if err != nil {
		_ = child.Process.Kill()
		_, _ = child.Process.Wait()
		return err
	}
	defer cfg.ClearAPIToken()
	apiToken := cfg.APIToken()
	defer clear(apiToken)
	if err = waitForDaemon(child.Process.Pid, cfg.ListenAddress, apiToken, 2*time.Minute); err != nil {
		_ = child.Process.Kill()
		_, _ = child.Process.Wait()
		return fmt.Errorf("daemon did not become healthy; inspect /data/local/tmp/xtest-nova-agent.log: %w", err)
	}
	fmt.Printf("xtest-nova-agent server started in background with pid %d\n", child.Process.Pid)
	return writeWebAccessInstructions(os.Stdout, cfg)
}

func configureDaemonLog() (func(), error) {
	path := os.Getenv("XTEST_NOVA_DAEMON_LOG")
	if path == "" {
		return func() {}, nil
	}
	writer, err := logrotation.NewWriter(path, logrotation.Options{MaxBytes: 24 << 20, Backups: 3, MaxAge: 72 * time.Hour, Compress: true})
	if err != nil {
		return func() {}, fmt.Errorf("open rotating daemon log: %w", err)
	}
	log.SetOutput(writer)
	return func() { _ = writer.Close() }, nil
}

func waitForDaemon(pid int, address string, apiToken []byte, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 300 * time.Millisecond}
	probeAddress := localProbeAddress(address)
	for time.Now().Before(deadline) {
		claimed, pidErr := readPID()
		if pidErr == nil && claimed == pid {
			request, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://"+probeAddress+"/v1/health", nil)
			if len(apiToken) != 0 {
				request.Header.Set("Authorization", "Bearer "+string(apiToken))
			}
			response, requestErr := client.Do(request)
			if requestErr == nil {
				_ = response.Body.Close()
				if response.StatusCode == http.StatusOK && response.Header.Get("X-XTest-Version") == buildinfo.Version {
					return nil
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("version %s was not ready on %s", buildinfo.Version, address)
}
