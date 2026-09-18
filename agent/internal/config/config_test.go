package config

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestParseOverridesAndRejectsUnsafeRuntimeValues(t *testing.T) {
	if value := Default(); value.HierarchyProvider != "system" {
		t.Fatalf("unqualified hierarchy provider became default: %q", value.HierarchyProvider)
	}
	defaults, err := Parse(nil, io.Discard)
	if err != nil || defaults.AllowLAN || defaults.APITokenFile != "" ||
		defaults.ListenAddress != "127.0.0.1:7912" ||
		defaults.CompanionAddress != "127.0.0.1:8912" ||
		defaults.MonitorAddress != "127.0.0.1:7890" {
		t.Fatalf("default ADB-forward configuration changed: %#v err=%v", defaults, err)
	}
	value, err := Parse([]string{"-listen", "127.0.0.1:17912", "-command-timeout", "2s", "-legacy-unsafe-api", "-legacy-uiautomator", "-no-popup", "-hierarchy-provider", "shadow"}, io.Discard)
	if err != nil || value.ListenAddress != "127.0.0.1:17912" || value.CommandTimeout != 2*time.Second || !value.UnsafeLegacyAPI || !value.LegacyUiAutomator || !value.NoPopup || value.HierarchyProvider != "shadow" {
		t.Fatalf("unexpected config: %#v err=%v", value, err)
	}
	if _, err = Parse([]string{"-listen", "", "-command-timeout", "0s"}, io.Discard); err == nil {
		t.Fatal("accepted empty listen address and non-positive timeout")
	}
	if _, err = Parse([]string{"-hierarchy-provider", "unknown"}, io.Discard); err == nil {
		t.Fatal("accepted unknown hierarchy provider")
	}
}

func TestParseListenerSafetyAndLANRequirements(t *testing.T) {
	for _, args := range [][]string{
		{"-listen", "0.0.0.0:7912"},
		{"-companion-listen", "192.168.1.10:8912"},
		{"-monitor-listen", "[::]:7890"},
	} {
		if _, err := Parse(args, io.Discard); err == nil {
			t.Fatalf("accepted non-loopback listener: %v", args)
		}
	}
	if _, err := Parse([]string{"-listen", "0.0.0.0:7912", "-legacy-unsafe-api"}, io.Discard); err == nil {
		t.Fatal("legacy unsafe API unexpectedly relaxed primary listener")
	}
	tokenFile := writeTokenFile(t, strings.Repeat("a", 32)+"\n", 0o600)
	value, err := Parse([]string{"-listen", "0.0.0.0:7912", "-allow-lan", "-api-token-file", tokenFile}, io.Discard)
	if err != nil {
		t.Fatalf("explicit LAN listener rejected: %v", err)
	}
	if value.ListenAddress != "0.0.0.0:7912" || !value.AllowLAN || string(value.APIToken()) != strings.Repeat("a", 32) {
		t.Fatalf("explicit LAN listener config: address=%q allowLAN=%t", value.ListenAddress, value.AllowLAN)
	}
	tokenCopy := value.APIToken()
	tokenCopy[0] = 'z'
	if value.APIToken()[0] != 'a' {
		t.Fatal("APIToken exposed mutable configuration storage")
	}
	value.ClearAPIToken()
	if len(value.APIToken()) != 0 {
		t.Fatal("ClearAPIToken retained plaintext token data")
	}
	if _, err = Parse([]string{"-allow-lan"}, io.Discard); err == nil {
		t.Fatal("accepted LAN mode without token file")
	}
	combined, err := Parse([]string{"-allow-lan", "-api-token-file", tokenFile, "-legacy-unsafe-api"}, io.Discard)
	if err != nil {
		t.Fatalf("LAN authentication and legacy unsafe API combination was rejected: %v", err)
	}
	if !combined.AllowLAN || !combined.UnsafeLegacyAPI {
		t.Fatalf("combined LAN and legacy unsafe configuration was not retained: %#v", combined)
	}
	for _, args := range [][]string{
		{"-allow-lan", "-api-token-file", tokenFile, "-companion-listen", "0.0.0.0:8912"},
		{"-allow-lan", "-api-token-file", tokenFile, "-monitor-listen", "0.0.0.0:7890"},
	} {
		if _, err = Parse(args, io.Discard); err == nil {
			t.Fatalf("LAN mode relaxed auxiliary listener: %v", args)
		}
	}
	if _, err = Parse([]string{"-listen", "localhost:7912"}, io.Discard); err != nil {
		t.Fatalf("localhost listener rejected: %v", err)
	}
}

func TestReadAPITokenValidation(t *testing.T) {
	for name, contents := range map[string]string{
		"minimum": " \n" + strings.Repeat("z", 32) + "\r\n",
		"maximum": strings.Repeat("z", 4<<10),
	} {
		path := writeTokenFile(t, contents, 0o600)
		token, err := ReadAPIToken(path)
		if err != nil || len(token) < 32 || len(token) > 4<<10 {
			t.Fatalf("%s valid token rejected: length=%d err=%v", name, len(token), err)
		}
	}
	for name, contents := range map[string]string{
		"short": strings.Repeat("x", 31),
		"large": strings.Repeat("x", (4<<10)+1),
	} {
		path := writeTokenFile(t, contents, 0o600)
		if _, err := ReadAPIToken(path); err == nil {
			t.Fatalf("%s token accepted", name)
		}
	}
	if _, err := ReadAPIToken(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing token file accepted")
	}
	if runtime.GOOS == "android" || runtime.GOOS == "linux" {
		path := writeTokenFile(t, strings.Repeat("x", 32), 0o640)
		if _, err := ReadAPIToken(path); err == nil {
			t.Fatal("group-readable token file accepted")
		}
	}
}

func writeTokenFile(t *testing.T, contents string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte(contents), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
	return path
}
