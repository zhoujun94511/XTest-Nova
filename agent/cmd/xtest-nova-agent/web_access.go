package main

import (
	"fmt"
	"io"
	"net"

	"github.com/zhoujun94511/xtest-nova/agent/internal/config"
)

func writeWebAccessInstructions(writer io.Writer, cfg config.Config) error {
	_, port, err := net.SplitHostPort(cfg.ListenAddress)
	if err != nil || port == "" {
		port = "7912"
	}
	if cfg.AllowLAN {
		if _, err = fmt.Fprintln(writer, "Web 控制台（LAN 模式）："); err != nil {
			return fmt.Errorf("write Web console heading: %w", err)
		}
		if _, err = fmt.Fprintf(writer, "  浏览器打开 http://<设备WLAN-IP>:%s/\n", port); err != nil {
			return fmt.Errorf("write LAN Web console URL: %w", err)
		}
		if _, err = fmt.Fprintln(writer, "  首次访问需使用 API 令牌认证；令牌不会出现在 URL 或启动参数中。"); err != nil {
			return fmt.Errorf("write LAN authentication instructions: %w", err)
		}
		if _, err = fmt.Fprintf(writer, "  恢复通道仍可使用：adb -s <设备序列号> forward tcp:%s tcp:%s\n", port, port); err != nil {
			return fmt.Errorf("write ADB recovery instructions: %w", err)
		}
		return nil
	}
	if _, err = fmt.Fprintln(writer, "Web 控制台（需先建立 ADB 端口转发）："); err != nil {
		return fmt.Errorf("write Web console heading: %w", err)
	}
	if _, err = fmt.Fprintf(writer, "  adb -s <设备序列号> forward tcp:%s tcp:%s\n", port, port); err != nil {
		return fmt.Errorf("write ADB forwarding instructions: %w", err)
	}
	if _, err = fmt.Fprintf(writer, "  浏览器打开 http://127.0.0.1:%s/\n", port); err != nil {
		return fmt.Errorf("write Web console URL: %w", err)
	}
	if _, err = fmt.Fprintln(writer, "多设备同时使用时，请为每台设备指定不同的本机端口。"); err != nil {
		return fmt.Errorf("write multi-device instructions: %w", err)
	}
	return nil
}

func localProbeAddress(address string) string {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return address
	}
	ip := net.ParseIP(host)
	if host == "" || ip != nil && ip.IsUnspecified() {
		return net.JoinHostPort("127.0.0.1", port)
	}
	return address
}
