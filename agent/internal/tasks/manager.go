package tasks

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const maxDownloadSize int64 = 1 << 30
const maxRetainedTasks = 256
const maxConcurrentTasks = 8

var ErrTaskLimit = errors.New("download task concurrency limit reached")

type State struct {
	ID          string     `json:"id"`
	Status      string     `json:"status"`
	Message     string     `json:"message,omitempty"`
	Error       string     `json:"error,omitempty"`
	Destination string     `json:"destination,omitempty"`
	TotalSize   int64      `json:"totalSize,omitempty"`
	CopiedSize  int64      `json:"copiedSize,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	EndedAt     *time.Time `json:"endedAt,omitempty"`
}
type task struct {
	state  State
	cancel context.CancelFunc
	done   chan struct{}
}
type Manager struct {
	mu     sync.RWMutex
	tasks  map[string]*task
	client *http.Client
	slots  chan struct{}
}

func New() *Manager {
	return &Manager{tasks: map[string]*task{}, client: &http.Client{Timeout: 2 * time.Hour, Transport: safeTransport()}, slots: make(chan struct{}, maxConcurrentTasks)}
}
func safeTransport() *http.Transport {
	dialer := net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}
	return &http.Transport{DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		addresses, err := lookupHostIPs(ctx, host)
		if err != nil {
			return nil, err
		}
		for _, ip := range addresses {
			if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
				continue
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		}
		return nil, fmt.Errorf("download host resolves only to private or local addresses")
	}, TLSClientConfig: &tls.Config{RootCAs: androidRootCAs(), MinVersion: tls.VersionTLS12}, TLSHandshakeTimeout: 15 * time.Second, ResponseHeaderTimeout: 30 * time.Second}
}

func androidRootCAs() *x509.CertPool {
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	for _, directory := range []string{"/apex/com.android.conscrypt/cacerts", "/system/etc/security/cacerts"} {
		entries, err := os.ReadDir(directory)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			data, err := os.ReadFile(filepath.Join(directory, entry.Name()))
			if err != nil || pool.AppendCertsFromPEM(data) {
				continue
			}
			if certificate, err := x509.ParseCertificate(data); err == nil {
				pool.AddCert(certificate)
			}
		}
	}
	return pool
}

func lookupHostIPs(ctx context.Context, host string) ([]net.IP, error) {
	resolvers := []*net.Resolver{net.DefaultResolver}
	for _, server := range []string{"1.1.1.1:53", "8.8.8.8:53"} {
		server := server
		resolvers = append(resolvers, &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
			dialer := net.Dialer{Timeout: 5 * time.Second}
			return dialer.DialContext(ctx, "udp", server)
		}})
	}
	var lookupErrors []error
	for _, resolver := range resolvers {
		addresses, err := resolver.LookupIP(ctx, "ip", host)
		if err == nil && len(addresses) > 0 {
			return addresses, nil
		}
		if err != nil {
			lookupErrors = append(lookupErrors, err)
		}
	}
	return nil, fmt.Errorf("resolve %q: %w", host, errors.Join(lookupErrors...))
}
func newID() string {
	data := make([]byte, 12)
	if _, err := rand.Read(data); err == nil {
		return hex.EncodeToString(data)
	}
	return strconv.FormatInt(time.Now().UnixNano(), 36)
}
func validateURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("only http and https downloads are supported")
	}
	if parsed.Hostname() == "" || parsed.User != nil {
		return fmt.Errorf("invalid download URL")
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return fmt.Errorf("local download hosts are not allowed")
	}
	if ip := net.ParseIP(host); ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified()) {
		return fmt.Errorf("private download hosts are not allowed")
	}
	return nil
}
func validateMode(mode os.FileMode) error {
	if mode&^os.FileMode(0777) != 0 {
		return fmt.Errorf("download mode must contain permission bits only")
	}
	return nil
}
func (m *Manager) Start(rawURL, destination string, mode os.FileMode, after func(context.Context, string) error, cleanup bool) (string, error) {
	if err := validateURL(rawURL); err != nil {
		return "", err
	}
	if err := validateMode(mode); err != nil {
		return "", err
	}
	if !filepath.IsAbs(destination) {
		return "", fmt.Errorf("destination must be absolute")
	}
	select {
	case m.slots <- struct{}{}:
	default:
		return "", ErrTaskLimit
	}
	id := newID()
	ctx, cancel := context.WithCancel(context.Background())
	item := &task{state: State{ID: id, Status: "pending", Destination: destination, CreatedAt: time.Now().UTC()}, cancel: cancel, done: make(chan struct{})}
	m.mu.Lock()
	m.pruneLocked()
	m.tasks[id] = item
	m.mu.Unlock()
	go m.run(ctx, item, rawURL, mode, after, cleanup)
	return id, nil
}
func (m *Manager) run(ctx context.Context, item *task, rawURL string, mode os.FileMode, after func(context.Context, string) error, cleanup bool) {
	defer func() { <-m.slots }()
	defer close(item.done)
	m.update(item.funcID(), func(s *State) { s.Status = "downloading" })
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err == nil {
		var response *http.Response
		response, err = m.client.Do(request)
		if err == nil {
			defer func() { _ = response.Body.Close() }()
			if response.StatusCode/100 != 2 {
				err = fmt.Errorf("download returned %s", response.Status)
			} else if response.ContentLength > maxDownloadSize {
				err = fmt.Errorf("download exceeds 1 GiB limit")
			} else {
				m.update(item.funcID(), func(s *State) { s.TotalSize = response.ContentLength })
				err = m.write(item, response.Body, mode)
			}
		}
	}
	if err == nil && after != nil {
		m.update(item.funcID(), func(s *State) { s.Status = "processing" })
		err = after(ctx, item.state.Destination)
	}
	if cleanup {
		_ = os.Remove(item.state.Destination)
	}
	m.finish(item.funcID(), err)
}
func (m *Manager) CancelAll(ctx context.Context) error {
	m.mu.RLock()
	items := make([]*task, 0, len(m.tasks))
	for _, item := range m.tasks {
		if item.state.EndedAt == nil {
			items = append(items, item)
		}
	}
	m.mu.RUnlock()
	for _, item := range items {
		item.cancel()
	}
	for _, item := range items {
		select {
		case <-item.done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}
func (t *task) funcID() string { return t.state.ID }
func (m *Manager) write(item *task, source io.Reader, mode os.FileMode) error {
	destination := item.state.Destination
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(destination), ".nova-download-*")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer func() { _ = os.Remove(temporary) }()
	writer := &progressWriter{writer: file, update: func(count int64) { m.update(item.funcID(), func(s *State) { s.CopiedSize = count }) }}
	written, err := io.Copy(writer, io.LimitReader(source, maxDownloadSize+1))
	if err == nil && written > maxDownloadSize {
		err = fmt.Errorf("download exceeds 1 GiB limit")
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Chmod(temporary, mode); err != nil {
		return err
	}
	return os.Rename(temporary, destination)
}

type progressWriter struct {
	writer io.Writer
	copied int64
	update func(int64)
}

func (w *progressWriter) Write(data []byte) (int, error) {
	n, err := w.writer.Write(data)
	w.copied += int64(n)
	w.update(w.copied)
	return n, err
}
func (m *Manager) update(id string, change func(*State)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if item := m.tasks[id]; item != nil {
		change(&item.state)
	}
}
func (m *Manager) finish(id string, err error) {
	now := time.Now().UTC()
	m.mu.Lock()
	defer m.mu.Unlock()
	if item := m.tasks[id]; item != nil {
		s := &item.state
		s.EndedAt = &now
		if err != nil {
			if strings.Contains(err.Error(), "canceled") {
				s.Status = "canceled"
			} else {
				s.Status = "failure"
			}
			s.Error = err.Error()
		} else {
			s.Status = "success"
		}
	}
	m.pruneLocked()
}

func (m *Manager) pruneLocked() {
	excess := len(m.tasks) - maxRetainedTasks
	if excess <= 0 {
		return
	}
	type completedTask struct {
		id        string
		createdAt time.Time
	}
	completed := make([]completedTask, 0, len(m.tasks))
	for id, item := range m.tasks {
		if item.state.EndedAt != nil {
			completed = append(completed, completedTask{id: id, createdAt: item.state.CreatedAt})
		}
	}
	sort.Slice(completed, func(i, j int) bool { return completed[i].createdAt.Before(completed[j].createdAt) })
	for _, item := range completed {
		if excess == 0 {
			break
		}
		delete(m.tasks, item.id)
		excess--
	}
}
func (m *Manager) Get(id string) (State, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	item, ok := m.tasks[id]
	if !ok {
		return State{}, false
	}
	return item.state, true
}
func (m *Manager) Cancel(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	item := m.tasks[id]
	if item == nil || item.state.EndedAt != nil {
		return false
	}
	item.cancel()
	return true
}
