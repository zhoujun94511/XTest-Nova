package autopopup

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zhoujun94511/xtest-nova/agent/internal/platform"
)

type HierarchyProvider interface {
	Hierarchy(context.Context) (string, error)
}
type ClickAction struct {
	MainContentContains string `json:"mainContentContains,omitempty"`
	ClickTarget         string `json:"clickTarget"`
}
type InputAction struct {
	MainContentContains string `json:"mainContentContains,omitempty"`
	InputTarget         string `json:"inputTarget"`
	InputValue          string `json:"inputValue,omitempty"`
}
type Config struct {
	AutoClickByText       []ClickAction `json:"autoClickByText"`
	AutoClickByResourceID []ClickAction `json:"autoClickByResourceId"`
	AutoInputByHint       []InputAction `json:"autoInputByHint"`
	AutoInputByResourceID []InputAction `json:"autoInputByResourceId"`
}
type State struct {
	Running   bool   `json:"running"`
	Stopping  bool   `json:"stopping"`
	LastError string `json:"lastError,omitempty"`
}
type xmlNode struct {
	Text       string    `xml:"text,attr"`
	ResourceID string    `xml:"resource-id,attr"`
	Hint       string    `xml:"hint,attr"`
	Content    string    `xml:"content-desc,attr"`
	Package    string    `xml:"package,attr"`
	Clickable  string    `xml:"clickable,attr"`
	Enabled    string    `xml:"enabled,attr"`
	Visible    string    `xml:"visible-to-user,attr"`
	Password   string    `xml:"password,attr"`
	Bounds     string    `xml:"bounds,attr"`
	Children   []xmlNode `xml:"node"`
}
type hierarchy struct {
	Nodes []xmlNode `xml:"node"`
}

type Manager struct {
	mu        sync.Mutex
	hierarchy HierarchyProvider
	executor  platform.Executor
	config    func() map[string]any
	interval  time.Duration
	cancel    context.CancelFunc
	done      chan struct{}
	state     State
}

func New(h HierarchyProvider, executor platform.Executor, config func() map[string]any) *Manager {
	return &Manager{hierarchy: h, executor: executor, config: config, interval: time.Second}
}
func (m *Manager) State() State { m.mu.Lock(); defer m.mu.Unlock(); return m.state }
func (m *Manager) Start() (State, error) {
	if _, err := m.loadConfig(); err != nil {
		return m.State(), err
	}
	m.mu.Lock()
	if m.state.Running {
		state := m.state
		m.mu.Unlock()
		return state, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel, m.done = cancel, make(chan struct{})
	m.state = State{Running: true}
	done := m.done
	m.mu.Unlock()
	go m.run(ctx, done)
	return m.State(), nil
}
func (m *Manager) Stop() (State, error) {
	m.mu.Lock()
	if !m.state.Running {
		state := m.state
		m.mu.Unlock()
		return state, nil
	}
	m.state.Stopping = true
	cancel, done := m.cancel, m.done
	m.mu.Unlock()
	cancel()
	select {
	case <-done:
		return m.State(), nil
	case <-time.After(2 * time.Second):
		return m.State(), fmt.Errorf("AutoPopup stop timed out")
	}
}
func (m *Manager) run(ctx context.Context, done chan struct{}) {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()
	defer func() {
		m.mu.Lock()
		m.state.Running, m.state.Stopping, m.cancel = false, false, nil
		m.mu.Unlock()
		close(done)
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			err := m.scan(ctx)
			m.mu.Lock()
			if err != nil {
				m.state.LastError = err.Error()
			} else {
				m.state.LastError = ""
			}
			m.mu.Unlock()
		}
	}
}
func (m *Manager) loadConfig() (Config, error) {
	data, err := json.Marshal(m.config())
	if err != nil {
		return Config{}, err
	}
	var config Config
	err = json.Unmarshal(data, &config)
	return config, err
}
func (m *Manager) scan(ctx context.Context) error {
	config, err := m.loadConfig()
	if err != nil {
		return err
	}
	document, err := m.hierarchy.Hierarchy(ctx)
	if err != nil {
		return err
	}
	var root hierarchy
	if err = xml.Unmarshal([]byte(document), &root); err != nil {
		return err
	}
	for i := range root.Nodes {
		if m.visit(ctx, &root.Nodes[i], document, config) {
			return nil
		}
	}
	return nil
}
func (m *Manager) visit(ctx context.Context, n *xmlNode, document string, config Config) bool {
	if n.Enabled == "false" {
		return false
	}
	for _, a := range config.AutoClickByText {
		if n.Text == a.ClickTarget && matches(document, a.MainContentContains) {
			return m.tap(ctx, n.Bounds)
		}
	}
	for _, a := range config.AutoClickByResourceID {
		if n.ResourceID == a.ClickTarget && matches(document, a.MainContentContains) {
			return m.tap(ctx, n.Bounds)
		}
	}
	for _, a := range config.AutoInputByHint {
		if (n.Hint == a.InputTarget || n.Text == a.InputTarget || n.Content == a.InputTarget) && matches(document, a.MainContentContains) {
			return m.input(ctx, n.Bounds, a.InputValue)
		}
	}
	for _, a := range config.AutoInputByResourceID {
		if n.ResourceID == a.InputTarget && matches(document, a.MainContentContains) {
			return m.input(ctx, n.Bounds, a.InputValue)
		}
	}
	for i := range n.Children {
		if m.visit(ctx, &n.Children[i], document, config) {
			return true
		}
	}
	return false
}

func matches(document, required string) bool {
	return required == "" || strings.Contains(document, required)
}

var boundsPattern = regexp.MustCompile(`\[(\d+),(\d+)]\[(\d+),(\d+)]`)

func center(bounds string) (int, int, bool) {
	match := boundsPattern.FindStringSubmatch(bounds)
	if len(match) != 5 {
		return 0, 0, false
	}
	var v [4]int
	for i := range v {
		v[i], _ = strconv.Atoi(match[i+1])
	}
	if v[2] <= v[0] || v[3] <= v[1] {
		return 0, 0, false
	}
	return (v[0] + v[2]) / 2, (v[1] + v[3]) / 2, true
}
func (m *Manager) tap(ctx context.Context, bounds string) bool {
	x, y, ok := center(bounds)
	if !ok {
		return false
	}
	_, err := m.executor.Run(ctx, "input", "tap", strconv.Itoa(x), strconv.Itoa(y))
	return err == nil
}
func (m *Manager) input(ctx context.Context, bounds, value string) bool {
	if !m.tap(ctx, bounds) || value == "" {
		return false
	}
	_, err := m.executor.Run(ctx, "input", "text", strings.ReplaceAll(value, " ", "%s"))
	return err == nil
}
