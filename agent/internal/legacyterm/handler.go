package legacyterm

import (
	"errors"
	"net/http"

	"github.com/coder/websocket"
)

const maxMessageBytes = 64 << 10

type Handler struct {
	slots chan struct{}
}

func New(limit int) *Handler {
	if limit < 1 {
		limit = 1
	}
	return &Handler{slots: make(chan struct{}, limit)}
}

func (h *Handler) ServeWebSocket(w http.ResponseWriter, r *http.Request) {
	select {
	case h.slots <- struct{}{}:
		defer func() { <-h.slots }()
	default:
		http.Error(w, "terminal session limit reached", http.StatusTooManyRequests)
		return
	}

	connection, err := websocket.Accept(w, r, &websocket.AcceptOptions{CompressionMode: websocket.CompressionDisabled})
	if err != nil {
		return
	}
	connection.SetReadLimit(maxMessageBytes)
	defer func() { _ = connection.Close(websocket.StatusNormalClosure, "terminal session closed") }()
	if err = serveSession(r.Context(), connection); err != nil && !errors.Is(err, r.Context().Err()) {
		status := websocket.CloseStatus(err)
		if status == websocket.StatusNormalClosure || status == websocket.StatusGoingAway || status == websocket.StatusNoStatusRcvd {
			return
		}
		_ = connection.Close(websocket.StatusInternalError, "terminal session failed")
	}
}

func (h *Handler) Running() int { return len(h.slots) }
