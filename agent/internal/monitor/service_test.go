package monitor

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
)

type failedListener struct{ err error }

func (f failedListener) Accept() (net.Conn, error) { return nil, f.err }
func (failedListener) Close() error                { return nil }
func (failedListener) Addr() net.Addr              { return failedAddr("failed") }

type failedAddr string

func (a failedAddr) Network() string { return string(a) }
func (a failedAddr) String() string  { return string(a) }

func TestPermanentAcceptFailureClearsReadiness(t *testing.T) {
	listener := failedListener{err: errors.New("listener failed")}
	service := New(nil, "unused")
	service.listener = listener
	service.accept(listener)
	if service.Ready() {
		t.Fatal("service remained ready after the accept loop exited")
	}
}

func TestPackageName(t *testing.T) {
	for input, want := range map[string]string{`{"packageName":"com.example.app"}`: "com.example.app", "com.example.other": "com.example.other", `{"bad":1}`: ""} {
		if got := packageName(input); got != want {
			t.Fatalf("packageName(%q)=%q want %q", input, got, want)
		}
	}
}

func TestWebSocketDisconnectUnblocksBackendScanner(t *testing.T) {
	service := New(nil, "127.0.0.1:0")
	if err := service.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if closeErr := service.Close(); closeErr != nil {
			t.Errorf("close service: %v", closeErr)
		}
	})
	handlerDone := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		service.ServeWebSocket(w, r)
		close(handlerDone)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	connection, _, err := websocket.Dial(ctx, "ws"+server.URL[len("http"):], nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = connection.CloseNow()
	select {
	case <-handlerDone:
	case <-ctx.Done():
		t.Fatal("websocket disconnect left the backend scanner blocked")
	}
}

func TestCloseTerminatesActiveConnections(t *testing.T) {
	service := New(nil, "127.0.0.1:0")
	if err := service.Start(); err != nil {
		t.Fatal(err)
	}
	service.mu.Lock()
	address := service.listener.Addr().String()
	service.mu.Unlock()
	connection, err := net.Dial("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if closeErr := connection.Close(); closeErr != nil && !errors.Is(closeErr, net.ErrClosed) {
			t.Errorf("close connection: %v", closeErr)
		}
	})

	deadline := time.Now().Add(time.Second)
	for {
		service.mu.Lock()
		registered := len(service.connections) == 1
		service.mu.Unlock()
		if registered {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("connection was not registered")
		}
		time.Sleep(time.Millisecond)
	}
	if err := service.Close(); err != nil {
		t.Fatal(err)
	}
	if err := connection.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	buffer := make([]byte, 1)
	if _, err := connection.Read(buffer); err == nil {
		t.Fatal("active connection remained open after service close")
	} else {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			t.Fatalf("connection close did not unblock reader: %v", err)
		}
	}
}
