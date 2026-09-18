package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/coder/websocket"
)

type result struct {
	AppEventBytes int    `json:"appEventBytes"`
	Rotation      string `json:"rotation"`
	MinicapBytes  int    `json:"minicapBytes"`
	ScrcpyBytes   int    `json:"scrcpyBytes"`
	ScrcpyControl string `json:"scrcpyControl"`
	Monitor       string `json:"monitor"`
}

func websocketURL(base, path string) (string, error) {
	parsed, err := url.Parse(base)
	if err != nil || parsed.Host == "" {
		return "", fmt.Errorf("invalid base URL")
	}
	if parsed.Scheme == "http" {
		parsed.Scheme = "ws"
	} else if parsed.Scheme == "https" {
		parsed.Scheme = "wss"
	} else {
		return "", fmt.Errorf("unsupported base URL scheme")
	}
	parsed.Path, parsed.RawQuery = path, ""
	return parsed.String(), nil
}

func main() {
	base := flag.String("base", "", "forwarded Nova Agent base URL")
	monitor := flag.String("monitor", "", "forwarded Nova monitor address")
	flag.Parse()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	value, err := check(ctx, *base, *monitor)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err = json.NewEncoder(os.Stdout).Encode(value); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func dialWebSocket(ctx context.Context, address string) (*websocket.Conn, error) {
	connection, response, err := websocket.Dial(ctx, address, nil)
	if response != nil && response.Body != nil {
		closeErr := response.Body.Close()
		if err == nil && closeErr != nil {
			_ = connection.CloseNow()
			return nil, fmt.Errorf("close websocket handshake response: %w", closeErr)
		}
	}
	return connection, err
}

func check(ctx context.Context, base, monitorAddress string) (result, error) {
	var output result
	eventURL, err := websocketURL(base, "/appeventmonitor")
	if err != nil {
		return output, err
	}
	eventSocket, err := dialWebSocket(ctx, eventURL)
	if err != nil {
		return output, fmt.Errorf("app event websocket: %w", err)
	}
	defer func() { _ = eventSocket.CloseNow() }()
	payload := []byte(`{"source":"m5.6","event":"stream-check"}`)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(base, "/")+"/appevent/info", bytes.NewReader(payload))
	if err != nil {
		return output, err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return output, err
	}
	statusCode := response.StatusCode
	if err = response.Body.Close(); err != nil {
		return output, fmt.Errorf("close app event response: %w", err)
	}
	if statusCode < 200 || statusCode >= 300 {
		return output, fmt.Errorf("app event publish returned HTTP %d", statusCode)
	}
	messageType, received, err := eventSocket.Read(ctx)
	if err != nil || messageType != websocket.MessageText || !bytes.Equal(received, payload) {
		return output, fmt.Errorf("app event stream mismatch")
	}
	output.AppEventBytes = len(received)

	minicapURL, err := websocketURL(base, "/minicap")
	if err != nil {
		return output, err
	}
	minicapSocket, err := dialWebSocket(ctx, minicapURL)
	if err != nil {
		return output, fmt.Errorf("minicap websocket: %w", err)
	}
	defer func() { _ = minicapSocket.CloseNow() }()
	minicapSocket.SetReadLimit(16 << 20)
	messageType, rotation, err := minicapSocket.Read(ctx)
	if err != nil || messageType != websocket.MessageText || !strings.HasPrefix(string(rotation), "rotation ") {
		return output, fmt.Errorf("invalid minicap rotation frame")
	}
	messageType, frame, err := minicapSocket.Read(ctx)
	if err != nil || messageType != websocket.MessageBinary || len(frame) < 8 || !bytes.Equal(frame[:8], []byte{137, 80, 78, 71, 13, 10, 26, 10}) {
		prefix := frame
		if len(prefix) > 12 {
			prefix = prefix[:12]
		}
		return output, fmt.Errorf("invalid minicap PNG frame: type=%d bytes=%d prefix=%x error=%v", messageType, len(frame), prefix, err)
	}
	output.Rotation, output.MinicapBytes = string(rotation), len(frame)

	scrcpyScreenURL, err := websocketURL(base, "/scrcpy/screen/low")
	if err != nil {
		return output, err
	}
	scrcpyScreen, err := dialWebSocket(ctx, scrcpyScreenURL)
	if err != nil {
		return output, fmt.Errorf("scrcpy screen websocket: %w", err)
	}
	defer func() { _ = scrcpyScreen.CloseNow() }()
	scrcpyScreen.SetReadLimit(16 << 20)
	for output.ScrcpyBytes < 1024 {
		messageType, frame, err = scrcpyScreen.Read(ctx)
		if err != nil || messageType != websocket.MessageBinary || len(frame) == 0 {
			return output, fmt.Errorf("invalid scrcpy H.264 frame: type=%d bytes=%d error=%v", messageType, len(frame), err)
		}
		output.ScrcpyBytes += len(frame)
	}
	scrcpyControlURL, err := websocketURL(base, "/scrcpy/control/low")
	if err != nil {
		return output, err
	}
	scrcpyControl, err := dialWebSocket(ctx, scrcpyControlURL)
	if err != nil {
		return output, fmt.Errorf("scrcpy control websocket: %w", err)
	}
	defer func() { _ = scrcpyControl.CloseNow() }()
	if err = scrcpyControl.Write(ctx, websocket.MessageText, []byte(`{"type":1,"keycode":3}`)); err != nil {
		return output, fmt.Errorf("scrcpy control write: %w", err)
	}
	messageType, received, err = scrcpyControl.Read(ctx)
	if err != nil || messageType != websocket.MessageText || !bytes.Contains(received, []byte(`"success":true`)) {
		return output, fmt.Errorf("invalid scrcpy control response: %s (%v)", received, err)
	}
	output.ScrcpyControl = "accepted"

	connection, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", monitorAddress)
	if err != nil {
		return output, fmt.Errorf("monitor TCP: %w", err)
	}
	defer func() { _ = connection.Close() }()
	if err = connection.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return output, err
	}
	if _, err = connection.Write([]byte("ping\n")); err != nil {
		return output, err
	}
	line, err := bufio.NewReader(connection).ReadString('\n')
	if err != nil {
		return output, err
	}
	var monitorResult map[string]any
	if err = json.Unmarshal([]byte(line), &monitorResult); err != nil {
		return output, fmt.Errorf("decode monitor response: %w", err)
	}
	if monitorResult["service"] != "xtest-nova-monitor" {
		return output, fmt.Errorf("invalid monitor response")
	}
	output.Monitor = "xtest-nova-monitor"
	return output, nil
}
