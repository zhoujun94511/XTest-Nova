package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/coder/websocket"
)

func main() {
	base := flag.String("base", "", "Agent HTTP base URL")
	flag.Parse()
	if *base == "" {
		fatal(fmt.Errorf("-base is required"))
	}
	client := &http.Client{Timeout: 10 * time.Second}
	for _, path := range []string{"/", "/term"} {
		response, err := client.Get(strings.TrimRight(*base, "/") + path)
		if err != nil {
			fatal(err)
		}
		body, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		closeErr := response.Body.Close()
		if closeErr != nil {
			fatal(fmt.Errorf("close %s response: %w", path, closeErr))
		}
		productName := strings.ToLower(string(body))
		if readErr != nil || response.StatusCode != http.StatusOK || !strings.Contains(productName, "xtest nova") {
			fatal(fmt.Errorf("%s UI failed: status=%d read=%v", path, response.StatusCode, readErr))
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	websocketURL := strings.Replace(strings.TrimRight(*base, "/"), "http://", "ws://", 1) + "/term"
	connection, response, err := websocket.Dial(ctx, websocketURL, nil)
	if response != nil && response.Body != nil {
		closeErr := response.Body.Close()
		if err == nil && closeErr != nil {
			_ = connection.CloseNow()
			fatal(fmt.Errorf("close terminal handshake response: %w", closeErr))
		}
	}
	if err != nil {
		fatal(err)
	}
	defer func() { _ = connection.CloseNow() }()
	resize := append([]byte{1}, []byte(`{"cols":100,"rows":30}`)...)
	if err = connection.Write(ctx, websocket.MessageBinary, resize); err != nil {
		fatal(err)
	}
	marker := "__XTEST_NOVA_TERMINAL_OK__"
	input := append([]byte{0}, []byte("printf "+marker+"\\n\nexit\n")...)
	if err = connection.Write(ctx, websocket.MessageBinary, input); err != nil {
		fatal(err)
	}
	var transcript strings.Builder
	for !strings.Contains(transcript.String(), marker) {
		_, payload, readErr := connection.Read(ctx)
		if readErr != nil {
			fatal(fmt.Errorf("terminal closed before marker: %w", readErr))
		}
		transcript.Write(payload)
		if transcript.Len() > 1<<20 {
			fatal(fmt.Errorf("terminal transcript exceeded 1 MiB"))
		}
	}
	result := map[string]any{"passed": true, "console": true, "terminalPage": true, "websocket": true, "resize": true, "marker": marker, "transcriptBytes": transcript.Len()}
	data, err := json.Marshal(result)
	if err != nil {
		fatal(err)
	}
	if _, err = os.Stdout.Write(append(data, '\n')); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	_, _ = fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
