package screenrecord

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/zhoujun94511/xtest-nova/agent/internal/artifacts"
)

var packagePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*(?:\.[A-Za-z][A-Za-z0-9_]*)+$`)

type Manager struct {
	mu            sync.Mutex
	root, session string
	command       *exec.Cmd
	cancel        context.CancelFunc
	done          chan struct{}
	running       bool
}

func New(root string) *Manager  { return &Manager{root: root} }
func (m *Manager) Start() error { return m.StartForPackage("") }
func (m *Manager) StartForPackage(packageName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running {
		return errors.New("screenrecord not closed")
	}
	if packageName == "" {
		packageName = "device.unknown"
	}
	if !packagePattern.MatchString(packageName) {
		return errors.New("invalid screenrecord package")
	}
	session, err := artifacts.NewDeviceSession(m.root, packageName, "ScreenRecord")
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, "screenrecord", filepath.Join(session, "0.mp4"))
	if err := cmd.Start(); err != nil {
		cancel()
		_ = os.Remove(session)
		removeEmptyParents(filepath.Dir(session), m.root)
		return err
	}
	m.command, m.cancel, m.done, m.running, m.session = cmd, cancel, make(chan struct{}), true, session
	done := m.done
	go func() {
		_ = cmd.Wait()
		m.mu.Lock()
		if m.command == cmd {
			m.command = nil
			m.cancel = nil
			m.running = false
		}
		m.mu.Unlock()
		close(done)
	}()
	return nil
}
func (m *Manager) Stop() ([]string, error) {
	m.mu.Lock()
	cmd, cancel, done, session := m.command, m.cancel, m.done, m.session
	m.mu.Unlock()
	if cmd != nil {
		if cmd.Process != nil {
			_ = cmd.Process.Signal(os.Interrupt)
		}
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			if cancel != nil {
				cancel()
			}
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			select {
			case <-done:
			case <-time.After(time.Second):
				return nil, fmt.Errorf("screenrecord stop timed out")
			}
		}
	}
	if session == "" {
		return make([]string, 0), nil
	}
	entries, err := os.ReadDir(session)
	if err != nil {
		return nil, err
	}
	files := make([]string, 0)
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".mp4" {
			files = append(files, filepath.Join(session, entry.Name()))
		}
	}
	return files, nil
}
func (m *Manager) Running() bool { m.mu.Lock(); defer m.mu.Unlock(); return m.running }

func removeEmptyParents(directory, root string) {
	root = filepath.Clean(root)
	for current := filepath.Clean(directory); current != root && current != filepath.Dir(current); current = filepath.Dir(current) {
		if err := os.Remove(current); err != nil {
			return
		}
	}
}
