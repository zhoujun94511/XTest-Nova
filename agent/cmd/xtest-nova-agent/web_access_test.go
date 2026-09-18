package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/zhoujun94511/xtest-nova/agent/internal/config"
)

func TestWriteWebAccessInstructionsUsesConfiguredPort(t *testing.T) {
	var output bytes.Buffer
	cfg := config.Default()
	cfg.ListenAddress = "127.0.0.1:17912"
	if err := writeWebAccessInstructions(&output, cfg); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, expected := range []string{"Web 控制台", "tcp:17912 tcp:17912", "http://127.0.0.1:17912/", "多设备"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("instructions %q do not contain %q", text, expected)
		}
	}
}

func TestWriteWebAccessInstructionsDescribesLANWithoutToken(t *testing.T) {
	var output bytes.Buffer
	cfg := config.Default()
	cfg.ListenAddress = "0.0.0.0:17912"
	cfg.AllowLAN = true
	cfg.APITokenFile = "/data/local/tmp/secret-token"
	if err := writeWebAccessInstructions(&output, cfg); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, expected := range []string{"LAN 模式", "http://<设备WLAN-IP>:17912/", "API 令牌认证", "forward tcp:17912 tcp:17912"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("instructions %q do not contain %q", text, expected)
		}
	}
	if strings.Contains(text, cfg.APITokenFile) {
		t.Fatalf("instructions exposed token path: %q", text)
	}
}

func TestLocalProbeAddressUsesLoopbackForWildcardListeners(t *testing.T) {
	for input, expected := range map[string]string{
		"0.0.0.0:7912": "127.0.0.1:7912",
		"[::]:7912":    "127.0.0.1:7912",
		":7912":        "127.0.0.1:7912",
		"127.0.0.1:8":  "127.0.0.1:8",
		"[::1]:9":      "[::1]:9",
		"invalid":      "invalid",
	} {
		if actual := localProbeAddress(input); actual != expected {
			t.Fatalf("localProbeAddress(%q) = %q, want %q", input, actual, expected)
		}
	}
}
