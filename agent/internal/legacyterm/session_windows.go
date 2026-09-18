//go:build windows

package legacyterm

import (
	"context"
	"errors"

	"github.com/coder/websocket"
)

func serveSession(_ context.Context, _ *websocket.Conn) error {
	return errors.New("PTY terminal is only available on the Android/Linux agent")
}
