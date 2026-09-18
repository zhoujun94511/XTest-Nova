package auxiliary

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"testing"
)

func TestListenOrReuseRequiresMatchingIdentityAndVersion(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"service": "companion", "version": "1"})
	})}
	serveErrors := make(chan error, 1)
	go func() { serveErrors <- server.Serve(listener) }()
	t.Cleanup(func() {
		if closeErr := server.Close(); closeErr != nil {
			t.Errorf("close server: %v", closeErr)
		}
		if serveErr := <-serveErrors; serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			t.Errorf("serve: %v", serveErr)
		}
	})
	address := listener.Addr().String()
	owned, reused, err := ListenOrReuse(context.Background(), address, HTTPProbe{Path: "/health", Service: "companion", Version: "1"})
	if err != nil || owned != nil || !reused {
		t.Fatalf("owned=%v reused=%v err=%v", owned, reused, err)
	}
	if _, _, err = ListenOrReuse(context.Background(), address, HTTPProbe{Path: "/health", Service: "companion", Version: "2"}); err == nil {
		t.Fatal("expected incompatible version")
	}
}
