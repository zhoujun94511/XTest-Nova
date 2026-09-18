package minitouch

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

type State struct {
	Running bool   `json:"running"`
	PID     int    `json:"pid,omitempty"`
	Path    string `json:"path"`
	Error   string `json:"error,omitempty"`
}
type Manager struct {
	mu                        sync.Mutex
	path, logPath, socketPath string
	command                   *exec.Cmd
	cancel                    context.CancelFunc
	state                     State
	bridging                  bool
}

func New(path, logPath, socketPath string) *Manager {
	return &Manager{path: path, logPath: logPath, socketPath: socketPath, state: State{Path: path}}
}

type TouchRequest struct {
	Operation    string  `json:"operation"`
	Index        int     `json:"index"`
	PercentX     float64 `json:"xP"`
	PercentY     float64 `json:"yP"`
	Milliseconds int     `json:"milliseconds"`
	Pressure     float64 `json:"pressure"`
}

func (m *Manager) ServeWebSocket(w http.ResponseWriter, r *http.Request) {
	connection, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = connection.CloseNow() }()
	connection.SetReadLimit(4096)
	m.mu.Lock()
	if m.bridging {
		m.mu.Unlock()
		_ = connection.Close(websocket.StatusPolicyViolation, "minitouch bridge already active")
		return
	}
	m.bridging = true
	m.mu.Unlock()
	defer func() { m.mu.Lock(); m.bridging = false; m.mu.Unlock() }()
	ctx := r.Context()
	if _, err = m.Start(); err != nil {
		_ = wsjson.Write(ctx, connection, map[string]any{"success": false, "error": err.Error()})
		return
	}
	var socket net.Conn
	for attempt := 0; attempt < 10; attempt++ {
		socket, err = net.DialTimeout("unix", m.socketPath, 500*time.Millisecond)
		if err == nil {
			break
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(250 * time.Millisecond):
		}
	}
	if err != nil {
		_ = wsjson.Write(ctx, connection, map[string]any{"success": false, "error": "minitouch socket unavailable"})
		return
	}
	defer func() { _ = socket.Close() }()
	defer func() { _, _ = io.WriteString(socket, "r\n") }()
	_ = socket.SetReadDeadline(time.Now().Add(5 * time.Second))
	reader := bufio.NewReader(socket)
	contacts, maxX, maxY, maxPressure, err := readBanner(reader)
	if err != nil {
		_ = wsjson.Write(ctx, connection, map[string]any{"success": false, "error": err.Error()})
		return
	}
	_ = socket.SetReadDeadline(time.Time{})
	go func() { _, _ = io.Copy(io.Discard, reader) }()
	if err = wsjson.Write(ctx, connection, map[string]any{"success": true, "maxContacts": contacts, "maxX": maxX, "maxY": maxY, "maxPressure": maxPressure}); err != nil {
		return
	}
	for {
		var request TouchRequest
		if err = wsjson.Read(ctx, connection, &request); err != nil {
			return
		}
		command, validationErr := touchCommand(request, contacts, maxX, maxY, maxPressure)
		if validationErr != nil {
			_ = wsjson.Write(ctx, connection, map[string]any{"success": false, "error": validationErr.Error()})
			continue
		}
		_ = socket.SetWriteDeadline(time.Now().Add(2 * time.Second))
		if _, err = io.WriteString(socket, command); err != nil {
			return
		}
	}
}

func readBanner(reader *bufio.Reader) (int, int, int, int, error) {
	lines := make([]string, 3)
	for i := range lines {
		line, err := reader.ReadString('\n')
		if err != nil {
			return 0, 0, 0, 0, err
		}
		lines[i] = strings.TrimSpace(line)
	}
	var flag string
	var version, contacts, maxX, maxY, maxPressure, pid int
	if _, err := fmt.Sscanf(lines[0], "%s %d", &flag, &version); err != nil || flag != "v" {
		return 0, 0, 0, 0, errors.New("invalid minitouch version banner")
	}
	if _, err := fmt.Sscanf(lines[1], "%s %d %d %d %d", &flag, &contacts, &maxX, &maxY, &maxPressure); err != nil || flag != "^" {
		return 0, 0, 0, 0, errors.New("invalid minitouch capability banner")
	}
	if contacts < 1 || maxX < 1 || maxY < 1 || maxPressure < 1 {
		return 0, 0, 0, 0, errors.New("invalid minitouch capability values")
	}
	if _, err := fmt.Sscanf(lines[2], "%s %d", &flag, &pid); err != nil || flag != "$" {
		return 0, 0, 0, 0, errors.New("invalid minitouch pid banner")
	}
	return contacts, maxX, maxY, maxPressure, nil
}
func touchCommand(r TouchRequest, contacts, maxX, maxY, maxPressure int) (string, error) {
	switch r.Operation {
	case "r", "c":
		return r.Operation + "\n", nil
	case "u":
		if r.Index < 0 || r.Index >= contacts {
			return "", errors.New("touch index out of range")
		}
		return fmt.Sprintf("u %d\n", r.Index), nil
	case "w":
		if r.Milliseconds < 0 || r.Milliseconds > 60000 {
			return "", errors.New("wait out of range")
		}
		return fmt.Sprintf("w %d\n", r.Milliseconds), nil
	case "d", "m":
		if r.Index < 0 || r.Index >= contacts {
			return "", errors.New("touch index out of range")
		}
		if r.PercentX < 0 || r.PercentX > 1 || r.PercentY < 0 || r.PercentY > 1 || r.Pressure < 0 || r.Pressure > 1 {
			return "", errors.New("touch percentage out of range")
		}
		pressure := int(r.Pressure * float64(maxPressure))
		if pressure == 0 {
			pressure = maxPressure - 1
		}
		return fmt.Sprintf("%s %d %d %d %d\n", r.Operation, r.Index, int(r.PercentX*float64(maxX)), int(r.PercentY*float64(maxY)), pressure), nil
	default:
		return "", errors.New("unsupported touch operation")
	}
}
func (m *Manager) State() State { m.mu.Lock(); defer m.mu.Unlock(); return m.state }
func (m *Manager) Start() (State, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state.Running {
		return m.state, nil
	}
	info, err := os.Stat(m.path)
	if err != nil {
		return m.state, fmt.Errorf("minitouch unavailable: %w", err)
	}
	if info.Mode()&0111 == 0 {
		return m.state, fmt.Errorf("minitouch is not executable")
	}
	log, err := os.OpenFile(m.logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return m.state, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	command := exec.CommandContext(ctx, m.path)
	command.Stdout = log
	command.Stderr = log
	if err = command.Start(); err != nil {
		cancel()
		_ = log.Close()
		return m.state, err
	}
	m.command = command
	m.cancel = cancel
	m.state = State{Running: true, PID: command.Process.Pid, Path: m.path}
	go m.wait(command, log)
	return m.state, nil
}
func (m *Manager) wait(command *exec.Cmd, log *os.File) {
	err := command.Wait()
	_ = log.Close()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.command != command {
		return
	}
	m.state.Running = false
	m.state.PID = 0
	if err != nil {
		m.state.Error = err.Error()
	}
	m.command = nil
	m.cancel = nil
}
func (m *Manager) Stop() (State, error) {
	m.mu.Lock()
	command, cancel := m.command, m.cancel
	if command == nil {
		state := m.state
		m.mu.Unlock()
		return state, nil
	}
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if command.Process != nil {
		_ = command.Process.Kill()
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		state := m.State()
		if !state.Running {
			return state, nil
		}
		time.Sleep(25 * time.Millisecond)
	}
	return m.State(), fmt.Errorf("minitouch stop timed out")
}
