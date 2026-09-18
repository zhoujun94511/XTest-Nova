package config

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"runtime"
	"strings"
	"time"
)

type Config struct {
	ListenAddress      string
	CompanionAddress   string
	MonitorAddress     string
	AllowLAN           bool
	APITokenFile       string
	RunnerClasspath    string
	CompanionAPK       string
	ConfigPath         string
	StopFile           string
	LogFile            string
	UnsafeLegacyAPI    bool
	LegacyUiAutomator  bool
	NoPopup            bool
	CommandTimeout     time.Duration
	HierarchyProvider  string
	UiAutomatorAddress string
	apiToken           []byte
}

func Default() Config {
	return Config{
		ListenAddress: "127.0.0.1:7912", CompanionAddress: "127.0.0.1:8912", MonitorAddress: "127.0.0.1:7890",
		RunnerClasspath: "/data/local/tmp/xtest-nova-runner.jar",
		CompanionAPK:    "/data/local/tmp/xtest-nova-companion.apk",
		ConfigPath:      "/data/local/tmp/xtest-nova-config.json",
		StopFile:        "/data/local/tmp/.xtest-nova-monkey.stop",
		LogFile:         "/data/local/tmp/xtest-nova-monkey.log", CommandTimeout: 15 * time.Second,
		HierarchyProvider: "system", UiAutomatorAddress: "127.0.0.1:9009",
	}
}

func Parse(args []string, output io.Writer) (Config, error) {
	cfg := Default()
	set := flag.NewFlagSet("xtest-nova-agent", flag.ContinueOnError)
	set.SetOutput(output)
	set.StringVar(&cfg.ListenAddress, "listen", cfg.ListenAddress, "Agent listen address")
	set.StringVar(&cfg.CompanionAddress, "companion-listen", cfg.CompanionAddress, "Companion listen address")
	set.StringVar(&cfg.MonitorAddress, "monitor-listen", cfg.MonitorAddress, "Monitor compatibility listen address")
	set.BoolVar(&cfg.AllowLAN, "allow-lan", false, "Allow the primary Agent listener on a non-loopback address")
	set.StringVar(&cfg.APITokenFile, "api-token-file", "", "File containing the LAN API authentication token")
	set.StringVar(&cfg.RunnerClasspath, "runner-classpath", cfg.RunnerClasspath, "Nova Runner jar")
	set.StringVar(&cfg.CompanionAPK, "companion-apk", cfg.CompanionAPK, "Nova Companion APK staged on device")
	set.StringVar(&cfg.ConfigPath, "config-path", cfg.ConfigPath, "Companion configuration")
	set.StringVar(&cfg.StopFile, "runner-stop-file", cfg.StopFile, "Runner stop marker")
	set.StringVar(&cfg.LogFile, "runner-log-file", cfg.LogFile, "Runner log")
	set.BoolVar(&cfg.UnsafeLegacyAPI, "legacy-unsafe-api", false, "Enable arbitrary shell compatibility API")
	set.BoolVar(&cfg.LegacyUiAutomator, "legacy-uiautomator", false, "Enable legacy port 9008 and com.github.uiautomator compatibility")
	set.BoolVar(&cfg.NoPopup, "no-popup", false, "Install the complete runtime without starting the Companion overlay")
	set.DurationVar(&cfg.CommandTimeout, "command-timeout", cfg.CommandTimeout, "Device command timeout")
	set.StringVar(&cfg.HierarchyProvider, "hierarchy-provider", cfg.HierarchyProvider, "Hierarchy provider: system, shadow, or nova")
	set.StringVar(&cfg.UiAutomatorAddress, "uiautomator-address", cfg.UiAutomatorAddress, "Nova UiAutomator loopback address")
	if err := set.Parse(args); err != nil {
		return Config{}, err
	}
	if cfg.ListenAddress == "" || cfg.CompanionAddress == "" || cfg.MonitorAddress == "" {
		return Config{}, fmt.Errorf("listen addresses cannot be empty")
	}
	for name, address := range map[string]string{
		"companion-listen": cfg.CompanionAddress,
		"monitor-listen":   cfg.MonitorAddress,
	} {
		if err := requireLoopback(address); err != nil {
			return Config{}, fmt.Errorf("%s: %w", name, err)
		}
	}
	if err := requireLoopback(cfg.ListenAddress); err != nil && !cfg.AllowLAN {
		return Config{}, fmt.Errorf("listen: %w; use --allow-lan with --api-token-file to allow a non-loopback listener", err)
	}
	if cfg.CommandTimeout <= 0 {
		return Config{}, fmt.Errorf("command timeout must be positive")
	}
	if cfg.HierarchyProvider != "system" && cfg.HierarchyProvider != "shadow" && cfg.HierarchyProvider != "nova" {
		return Config{}, fmt.Errorf("hierarchy provider must be system, shadow, or nova")
	}
	if cfg.AllowLAN {
		if strings.TrimSpace(cfg.APITokenFile) == "" {
			return Config{}, fmt.Errorf("--api-token-file is required with --allow-lan")
		}
		token, err := ReadAPIToken(cfg.APITokenFile)
		if err != nil {
			return Config{}, fmt.Errorf("read API token file: %w", err)
		}
		cfg.apiToken = token
	}
	return cfg, nil
}

// APIToken returns a copy of the configured LAN API token.
func (c *Config) APIToken() []byte {
	if c == nil {
		return nil
	}
	return append([]byte(nil), c.apiToken...)
}

// ClearAPIToken removes the retained plaintext LAN token.
func (c *Config) ClearAPIToken() {
	clear(c.apiToken)
	c.apiToken = nil
}

// ReadAPIToken securely reads and validates a LAN API token file.
func ReadAPIToken(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("token path is not a regular file")
	}
	if info.Size() > 4<<10 {
		return nil, fmt.Errorf("token file exceeds 4 KiB")
	}
	if (runtime.GOOS == "android" || runtime.GOOS == "linux") && info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("token file must not grant group or world permissions")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	data = bytes.TrimSpace(data)
	if len(data) < 32 {
		return nil, fmt.Errorf("token must contain at least 32 bytes")
	}
	if len(data) > 4<<10 {
		return nil, fmt.Errorf("token exceeds 4 KiB")
	}
	return data, nil
}

func requireLoopback(address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("invalid listen address %q", address)
	}
	host = strings.TrimSpace(strings.Trim(host, "[]"))
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("listen address %q is not loopback", address)
	}
	return nil
}
