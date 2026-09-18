package auxiliary

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"
)

type HTTPProbe struct{ Path, Service, Version string }

func ListenOrReuse(ctx context.Context, address string, probe HTTPProbe) (net.Listener, bool, error) {
	listener, err := net.Listen("tcp", address)
	if err == nil {
		return listener, false, nil
	}
	request, requestErr := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+address+probe.Path, nil)
	if requestErr != nil {
		return nil, false, err
	}
	client := &http.Client{Timeout: time.Second}
	response, requestErr := client.Do(request)
	if requestErr != nil {
		return nil, false, fmt.Errorf("bind %s: %w; compatibility probe: %v", address, err, requestErr)
	}
	var payload struct{ Service, Version string }
	decodeErr := json.NewDecoder(response.Body).Decode(&payload)
	closeErr := response.Body.Close()
	if closeErr != nil {
		return nil, false, fmt.Errorf("bind %s: %w; close compatibility response: %v", address, err, closeErr)
	}
	if response.StatusCode != http.StatusOK || decodeErr != nil || payload.Service != probe.Service || payload.Version != probe.Version {
		return nil, false, fmt.Errorf("bind %s: %w; occupied service failed compatibility handshake", address, err)
	}
	return nil, true, nil
}
