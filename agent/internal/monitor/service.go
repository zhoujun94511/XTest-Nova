package monitor

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	novasystem "github.com/zhoujun94511/xtest-nova/agent/internal/system"
)

type Collector interface {
	Performance(context.Context, string) (novasystem.Performance, error)
}
type Service struct {
	mu          sync.Mutex
	collector   Collector
	address     string
	listener    net.Listener
	listenerErr error
	connections map[net.Conn]struct{}
	external    bool
}

func New(collector Collector, address string) *Service {
	return &Service{collector: collector, address: address, connections: map[net.Conn]struct{}{}}
}
func (s *Service) Ready() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return (s.listener != nil || s.external) && s.listenerErr == nil
}
func (s *Service) UseExternal() { s.mu.Lock(); s.external = true; s.listenerErr = nil; s.mu.Unlock() }
func (s *Service) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener != nil {
		return nil
	}
	listener, err := net.Listen("tcp", s.address)
	if err != nil {
		return err
	}
	s.listener = listener
	s.external = false
	s.listenerErr = nil
	go s.accept(listener)
	return nil
}
func (s *Service) Close() error {
	s.mu.Lock()
	listener := s.listener
	s.listener = nil
	s.external = false
	s.listenerErr = nil
	s.mu.Unlock()
	var listenerErr error
	if listener != nil {
		listenerErr = listener.Close()
	}
	s.mu.Lock()
	connections := make([]net.Conn, 0, len(s.connections))
	for connection := range s.connections {
		connections = append(connections, connection)
	}
	s.mu.Unlock()
	for _, connection := range connections {
		_ = connection.Close()
	}
	return listenerErr
}
func (s *Service) accept(listener net.Listener) {
	var retryDelay time.Duration
	for {
		connection, err := listener.Accept()
		if err != nil {
			s.mu.Lock()
			active := s.listener == listener
			s.mu.Unlock()
			if !active {
				return
			}
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Temporary() {
				if retryDelay == 0 {
					retryDelay = 5 * time.Millisecond
				} else {
					retryDelay *= 2
				}
				if retryDelay > time.Second {
					retryDelay = time.Second
				}
				time.Sleep(retryDelay)
				continue
			}
			s.mu.Lock()
			if s.listener == listener {
				s.listener = nil
				s.listenerErr = err
			}
			s.mu.Unlock()
			_ = listener.Close()
			return
		}
		retryDelay = 0
		s.mu.Lock()
		if s.listener != listener {
			s.mu.Unlock()
			_ = connection.Close()
			return
		}
		s.connections[connection] = struct{}{}
		s.mu.Unlock()
		go s.serve(connection)
	}
}

type request struct {
	PackageName string `json:"packageName"`
	Package     string `json:"package"`
	PkgName     string `json:"pkgname"`
}

func packageName(payload string) string {
	var r request
	if json.Unmarshal([]byte(payload), &r) == nil {
		for _, value := range []string{r.PackageName, r.Package, r.PkgName} {
			if value = strings.TrimSpace(value); value != "" {
				return value
			}
		}
		return ""
	}
	value := strings.TrimSpace(payload)
	if strings.ContainsAny(value, "{}[]\"") {
		return ""
	}
	return value
}
func (s *Service) serve(connection net.Conn) {
	defer func() {
		s.mu.Lock()
		delete(s.connections, connection)
		s.mu.Unlock()
		_ = connection.Close()
	}()
	scanner := bufio.NewScanner(connection)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	encoder := json.NewEncoder(connection)
	for scanner.Scan() {
		payload := strings.TrimSpace(scanner.Text())
		if payload == "" {
			continue
		}
		if strings.EqualFold(payload, "ping") {
			_ = encoder.Encode(map[string]any{"success": true, "service": "xtest-nova-monitor", "version": ServiceVersion})
			continue
		}
		pkg := packageName(payload)
		if pkg == "" {
			_ = encoder.Encode(map[string]any{"error": "packageName is required"})
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		performance, err := s.collector.Performance(ctx, pkg)
		cancel()
		if err != nil {
			_ = encoder.Encode(map[string]any{"error": err.Error(), "packageName": pkg})
			continue
		}
		_ = encoder.Encode(performance)
	}
}
func (s *Service) ServeWebSocket(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	listener := s.listener
	address := ""
	if listener != nil {
		address = listener.Addr().String()
	} else if s.external {
		address = s.address
	}
	s.mu.Unlock()
	if address == "" {
		http.Error(w, "Nova monitor source unavailable", http.StatusServiceUnavailable)
		return
	}
	upgrade := r
	if r.Method == http.MethodPut {
		upgrade = r.Clone(r.Context())
		upgrade.Method = http.MethodGet
	}
	ws, err := websocket.Accept(w, upgrade, nil)
	if err != nil {
		return
	}
	defer func() { _ = ws.CloseNow() }()
	ws.SetReadLimit(1 << 20)
	connection, err := net.DialTimeout("tcp", address, 2*time.Second)
	if err != nil {
		_ = ws.Close(websocket.StatusInternalError, err.Error())
		return
	}
	defer func() { _ = connection.Close() }()
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	go func() {
		defer cancel()
		for {
			_, payload, readErr := ws.Read(ctx)
			if readErr != nil {
				_ = connection.Close()
				return
			}
			_ = connection.SetWriteDeadline(time.Now().Add(2 * time.Second))
			if _, writeErr := fmt.Fprintf(connection, "%s\n", payload); writeErr != nil {
				_ = connection.Close()
				return
			}
		}
	}()
	scanner := bufio.NewScanner(connection)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		if err = ws.Write(ctx, websocket.MessageText, append([]byte(nil), scanner.Bytes()...)); err != nil {
			return
		}
	}
	if err = scanner.Err(); err != nil && !errors.Is(err, net.ErrClosed) {
		_ = ws.Close(websocket.StatusInternalError, "monitor stream failed")
	}
}

const ServiceVersion = "1"

func Probe(address string, timeout time.Duration) (resultErr error) {
	connection, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := connection.Close(); resultErr == nil {
			resultErr = closeErr
		}
	}()
	_ = connection.SetDeadline(time.Now().Add(timeout))
	if _, err = fmt.Fprintln(connection, "ping"); err != nil {
		return err
	}
	var response struct {
		Success bool   `json:"success"`
		Service string `json:"service"`
		Version string `json:"version"`
	}
	if err = json.NewDecoder(connection).Decode(&response); err != nil {
		return err
	}
	if !response.Success || response.Service != "xtest-nova-monitor" || response.Version != ServiceVersion {
		return errors.New("monitor compatibility handshake failed")
	}
	return nil
}
