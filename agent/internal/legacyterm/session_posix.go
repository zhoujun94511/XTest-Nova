//go:build !windows

package legacyterm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/coder/websocket"
	"github.com/creack/pty"
)

type windowSize struct {
	Rows uint16 `json:"rows"`
	Cols uint16 `json:"cols"`
}

func serveSession(parent context.Context, connection *websocket.Conn) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	shell, err := exec.LookPath("sh")
	if err != nil {
		shell = "/system/bin/sh"
	}
	cmd := exec.Command(shell, "-l")
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	tty, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 24, Cols: 80})
	if err != nil {
		return err
	}
	defer func() {
		_ = tty.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}()

	outputErr := make(chan error, 1)
	go func() {
		buffer := make([]byte, 4096)
		for {
			n, readErr := tty.Read(buffer)
			if n > 0 {
				writeCtx, writeCancel := context.WithTimeout(ctx, 10*time.Second)
				writeErr := connection.Write(writeCtx, websocket.MessageBinary, append([]byte(nil), buffer[:n]...))
				writeCancel()
				if writeErr != nil {
					outputErr <- writeErr
					cancel()
					return
				}
			}
			if readErr != nil {
				if errors.Is(readErr, io.EOF) {
					outputErr <- nil
				} else {
					outputErr <- readErr
				}
				cancel()
				return
			}
		}
	}()

	for {
		messageType, payload, readErr := connection.Read(ctx)
		if readErr != nil {
			select {
			case output := <-outputErr:
				return output
			default:
				return readErr
			}
		}
		if messageType != websocket.MessageBinary || len(payload) < 1 {
			continue
		}
		switch payload[0] {
		case 0:
			if _, err = tty.Write(payload[1:]); err != nil {
				return err
			}
		case 1:
			var size windowSize
			if err = json.Unmarshal(payload[1:], &size); err != nil {
				continue
			}
			if size.Rows == 0 || size.Cols == 0 {
				continue
			}
			if err = pty.Setsize(tty, &pty.Winsize{Rows: size.Rows, Cols: size.Cols}); err != nil {
				return err
			}
		}
	}
}
