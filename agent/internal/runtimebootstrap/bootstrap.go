package runtimebootstrap

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zhoujun94511/xtest-nova/agent/internal/overlaypermission"
	"github.com/zhoujun94511/xtest-nova/agent/internal/runtimebundle"
)

type Executor interface {
	Run(context.Context, string, ...string) (string, error)
}
type Source interface {
	Components() ([]runtimebundle.Component, error)
	Data(string) ([]byte, error)
}

type Result struct {
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	Package string `json:"package,omitempty"`
	Target  string `json:"target"`
	Action  string `json:"action"`
	Ready   bool   `json:"ready"`
	Error   string `json:"error,omitempty"`
}
type State struct {
	Running    bool       `json:"running"`
	Ready      bool       `json:"ready"`
	StartedAt  time.Time  `json:"startedAt"`
	EndedAt    *time.Time `json:"endedAt,omitempty"`
	Components []Result   `json:"components"`
	Error      string     `json:"error,omitempty"`
}
type Targets struct{ Runner, Companion, UIAutomatorHost, UIAutomatorTest string }

type Manager struct {
	operation sync.Mutex
	mu        sync.RWMutex
	executor  Executor
	source    Source
	targets   Targets
	state     State
}

func New(executor Executor, source Source, targets Targets) *Manager {
	return &Manager{executor: executor, source: source, targets: targets, state: State{Components: []Result{}}}
}
func (m *Manager) State() State {
	m.mu.RLock()
	defer m.mu.RUnlock()
	state := m.state
	state.Components = append([]Result(nil), state.Components...)
	return state
}

func (m *Manager) Ensure(ctx context.Context) (State, error) {
	m.operation.Lock()
	defer m.operation.Unlock()
	started := time.Now().UTC()
	m.setState(State{Running: true, StartedAt: started, Components: []Result{}})
	components, err := m.source.Components()
	if err != nil {
		return m.fail(err)
	}
	targets := map[string]string{"runner": m.targets.Runner, "companion": m.targets.Companion, "uiautomatorHost": m.targets.UIAutomatorHost, "uiautomatorTest": m.targets.UIAutomatorTest}
	files := make([]fileChange, 0, len(components))
	results := make([]Result, 0, len(components))
	for _, component := range components {
		payload, readErr := m.source.Data(component.File)
		if readErr != nil {
			m.rollbackFiles(files)
			return m.fail(readErr)
		}
		target := targets[component.Name]
		change, action, stageErr := stageFile(target, payload)
		if stageErr != nil {
			m.rollbackFiles(files)
			return m.fail(fmt.Errorf("stage %s: %w", component.Name, stageErr))
		}
		files = append(files, change)
		results = append(results, Result{Name: component.Name, Kind: component.Kind, Package: component.Package, Target: target, Action: action})
	}
	installed, installActions, installErr := m.ensurePackages(ctx, components, targets)
	if installErr != nil {
		m.rollbackPackages(ctx, installed)
		m.rollbackFiles(files)
		return m.fail(installErr)
	}
	for index, component := range components {
		if component.Kind == "apk" {
			results[index].Action = joinAction(results[index].Action, installActions[index])
		}
		results[index].Ready = true
	}
	if permissionErr := overlaypermission.Grant(ctx, m.executor); permissionErr != nil {
		m.rollbackPackages(ctx, installed)
		m.rollbackFiles(files)
		return m.fail(fmt.Errorf("grant Companion overlay permission: %w", permissionErr))
	}
	commitPackages(installed)
	commitFiles(files)
	ended := time.Now().UTC()
	state := State{Ready: true, StartedAt: started, EndedAt: &ended, Components: results}
	m.setState(state)
	return state, nil
}

func (m *Manager) setState(state State) { m.mu.Lock(); m.state = state; m.mu.Unlock() }
func (m *Manager) fail(err error) (State, error) {
	ended := time.Now().UTC()
	state := m.State()
	state.Running = false
	state.Ready = false
	state.EndedAt = &ended
	state.Error = err.Error()
	m.setState(state)
	return state, err
}

type fileChange struct {
	target, backup   string
	changed, existed bool
}

func stageFile(target string, payload []byte) (fileChange, string, error) {
	if target == "" || !filepath.IsAbs(target) {
		return fileChange{}, "", errors.New("runtime target must be absolute")
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fileChange{}, "", err
	}
	digest := sha256.Sum256(payload)
	if current, err := fileDigest(target); err == nil && current == hex.EncodeToString(digest[:]) {
		return fileChange{target: target}, "reused", nil
	}
	temporary := target + ".bundle.staged"
	backup := target + ".bundle.previous"
	_ = os.Remove(temporary)
	if err := os.WriteFile(temporary, payload, 0o644); err != nil {
		return fileChange{}, "", err
	}
	file, err := os.OpenFile(temporary, os.O_RDWR, 0)
	if err == nil {
		err = file.Sync()
		_ = file.Close()
	}
	if err != nil {
		_ = os.Remove(temporary)
		return fileChange{}, "", err
	}
	_, statErr := os.Stat(target)
	existed := statErr == nil
	if existed {
		_ = os.Remove(backup)
		if err = os.Rename(target, backup); err != nil {
			_ = os.Remove(temporary)
			return fileChange{}, "", err
		}
	}
	if err = os.Rename(temporary, target); err != nil {
		if existed {
			_ = os.Rename(backup, target)
		}
		return fileChange{}, "", err
	}
	return fileChange{target: target, backup: backup, changed: true, existed: existed}, "extracted", nil
}
func fileDigest(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
func commitFiles(changes []fileChange) {
	for _, change := range changes {
		if change.changed && change.existed {
			_ = os.Remove(change.backup)
		}
	}
}
func (m *Manager) rollbackFiles(changes []fileChange) {
	for index := len(changes) - 1; index >= 0; index-- {
		change := changes[index]
		if !change.changed {
			continue
		}
		_ = os.Remove(change.target)
		if change.existed {
			_ = os.Rename(change.backup, change.target)
		}
	}
}

type packageChange struct {
	pkg, backup      string
	changed, existed bool
}

type packageInstallPlan struct {
	index     int
	component runtimebundle.Component
	target    string
	installed bool
}

var installSessionPattern = regexp.MustCompile(`\[(\d+)]`)

const (
	packageMatchAttempts = 4
	packageMatchDelay    = 200 * time.Millisecond
)

func (m *Manager) ensurePackages(ctx context.Context, components []runtimebundle.Component, targets map[string]string) ([]packageChange, map[int]string, error) {
	actions := make(map[int]string)
	plans := make([]packageInstallPlan, 0, 3)
	for index, component := range components {
		if component.Kind != "apk" {
			continue
		}
		_, installed, matches := m.packageMatch(ctx, component)
		if matches {
			actions[index] = "installed-reused"
			continue
		}
		plans = append(plans, packageInstallPlan{index: index, component: component, target: targets[component.Name], installed: installed})
	}
	if len(plans) == 0 {
		return nil, actions, nil
	}
	if len(plans) > 1 {
		parent, err := m.createInstallSession(ctx, true)
		if err == nil {
			changes, mayHaveChanged, atomicErr := m.installPackagesAtomic(ctx, parent, plans)
			if atomicErr != nil {
				if mayHaveChanged {
					return changes, actions, fmt.Errorf("atomic APK install: %w", atomicErr)
				}
			} else {
				for _, plan := range plans {
					actions[plan.index] = "installed-atomic"
				}
				return changes, actions, nil
			}
		}
	}
	changes := make([]packageChange, 0, len(plans))
	for _, plan := range plans {
		change, action, err := m.ensurePackage(ctx, plan.component, plan.target)
		if change != nil && change.changed {
			changes = append(changes, *change)
		}
		if err != nil {
			return changes, actions, fmt.Errorf("install %s: %w", plan.component.Name, err)
		}
		actions[plan.index] = action
	}
	return changes, actions, nil
}

func (m *Manager) createInstallSession(ctx context.Context, multi bool) (int, error) {
	arguments := []string{"install-create", "-r"}
	if multi {
		arguments = append(arguments, "--multi-package")
	}
	output, err := m.executor.Run(ctx, "pm", arguments...)
	if err != nil {
		return 0, fmt.Errorf("pm %s: %s: %w", strings.Join(arguments, " "), strings.TrimSpace(output), err)
	}
	match := installSessionPattern.FindStringSubmatch(output)
	if len(match) != 2 {
		return 0, fmt.Errorf("unable to parse install session: %s", strings.TrimSpace(output))
	}
	return strconv.Atoi(match[1])
}

func (m *Manager) installPackagesAtomic(ctx context.Context, parent int, plans []packageInstallPlan) ([]packageChange, bool, error) {
	committed := false
	defer func() {
		if !committed {
			_, _ = m.executor.Run(context.Background(), "pm", "install-abandon", strconv.Itoa(parent))
		}
	}()
	changes := make([]packageChange, 0, len(plans))
	for _, plan := range plans {
		change := packageChange{pkg: plan.component.Package, backup: plan.target + ".installed.previous", changed: true, existed: plan.installed}
		if plan.installed {
			installedPath, ok := m.packagePath(ctx, plan.component.Package)
			if !ok {
				m.discardPackageBackups(ctx, changes)
				return nil, false, errors.New("installed package disappeared during atomic preparation")
			}
			if output, err := m.executor.Run(ctx, "cp", "-f", installedPath, change.backup); err != nil {
				m.discardPackageBackups(ctx, changes)
				return nil, false, commandError("backup "+plan.component.Name, output, err)
			}
		}
		changes = append(changes, change)
		child, err := m.createInstallSession(ctx, false)
		if err != nil {
			m.discardPackageBackups(ctx, changes)
			return nil, false, err
		}
		output, err := m.executor.Run(ctx, "pm", "install-write", "-S", strconv.FormatInt(plan.component.Size, 10), strconv.Itoa(child), "base.apk", plan.target)
		if err != nil || !strings.Contains(output, "Success") {
			m.abandonInstallSession(child)
			m.discardPackageBackups(ctx, changes)
			return nil, false, commandError("write "+plan.component.Name+" session", output, err)
		}
		output, err = m.executor.Run(ctx, "pm", "install-add-session", strconv.Itoa(parent), strconv.Itoa(child))
		if err != nil || !strings.Contains(output, "Success") {
			m.abandonInstallSession(child)
			m.discardPackageBackups(ctx, changes)
			return nil, false, commandError("attach "+plan.component.Name+" session", output, err)
		}
	}
	output, err := m.executor.Run(ctx, "pm", "install-commit", strconv.Itoa(parent))
	if err != nil || !strings.Contains(output, "Success") {
		return changes, true, commandError("commit multi-package session", output, err)
	}
	committed = true
	for _, plan := range plans {
		installedPath, ok := m.packagePath(ctx, plan.component.Package)
		if !ok {
			return changes, true, fmt.Errorf("package missing after atomic install: %s", plan.component.Package)
		}
		digest, digestErr := m.remoteDigest(ctx, installedPath)
		if digestErr != nil || !strings.EqualFold(digest, plan.component.SHA256) {
			return changes, true, fmt.Errorf("installed APK digest mismatch: %s", plan.component.Package)
		}
	}
	return changes, true, nil
}

func (m *Manager) abandonInstallSession(session int) {
	_, _ = m.executor.Run(context.Background(), "pm", "install-abandon", strconv.Itoa(session))
}

func (m *Manager) discardPackageBackups(ctx context.Context, changes []packageChange) {
	for _, change := range changes {
		if change.existed {
			_, _ = m.executor.Run(ctx, "rm", "-f", change.backup)
		}
	}
}

func commandError(operation, output string, err error) error {
	detail := strings.TrimSpace(output)
	if err != nil {
		return fmt.Errorf("%s: %s: %w", operation, detail, err)
	}
	return fmt.Errorf("%s: %s", operation, detail)
}

func (m *Manager) ensurePackage(ctx context.Context, component runtimebundle.Component, target string) (*packageChange, string, error) {
	installedPath, installed, matches := m.packageMatch(ctx, component)
	if matches {
		return nil, "installed-reused", nil
	}
	change := packageChange{pkg: component.Package, backup: target + ".installed.previous", changed: true, existed: installed}
	if installed {
		if output, err := m.executor.Run(ctx, "cp", "-f", installedPath, change.backup); err != nil {
			return nil, "", fmt.Errorf("backup installed APK: %s: %w", strings.TrimSpace(output), err)
		}
	}
	output, err := m.executor.Run(ctx, "pm", "install", "-r", target)
	if err != nil {
		return &change, "", fmt.Errorf("pm install: %s: %w", strings.TrimSpace(output), err)
	}
	installedPath, installed = m.packagePath(ctx, component.Package)
	if !installed {
		return &change, "", errors.New("package missing after install")
	}
	digest, err := m.remoteDigest(ctx, installedPath)
	if err != nil || !strings.EqualFold(digest, component.SHA256) {
		return &change, "", errors.New("installed APK digest mismatch")
	}
	return &change, "installed", nil
}

func (m *Manager) packageMatch(ctx context.Context, component runtimebundle.Component) (string, bool, bool) {
	var lastPath string
	var installed bool
	for attempt := 0; attempt < packageMatchAttempts; attempt++ {
		lastPath, installed = m.packagePath(ctx, component.Package)
		if installed {
			if digest, err := m.remoteDigest(ctx, lastPath); err == nil && strings.EqualFold(digest, component.SHA256) {
				return lastPath, true, true
			}
		}
		if attempt+1 < packageMatchAttempts {
			timer := time.NewTimer(packageMatchDelay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return lastPath, installed, false
			case <-timer.C:
			}
		}
	}
	return lastPath, installed, false
}
func (m *Manager) packagePath(ctx context.Context, pkg string) (string, bool) {
	output, err := m.executor.Run(ctx, "pm", "path", pkg)
	if err != nil {
		return "", false
	}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "package:") {
			return strings.TrimPrefix(line, "package:"), true
		}
	}
	return "", false
}
func (m *Manager) remoteDigest(ctx context.Context, path string) (string, error) {
	output, err := m.executor.Run(ctx, "sha256sum", path)
	if err != nil {
		return "", err
	}
	fields := strings.Fields(output)
	if len(fields) < 1 || len(fields[0]) != 64 {
		return "", errors.New("invalid sha256sum output")
	}
	return fields[0], nil
}
func (m *Manager) rollbackPackages(ctx context.Context, changes []packageChange) {
	for index := len(changes) - 1; index >= 0; index-- {
		change := changes[index]
		if change.existed {
			_, _ = m.executor.Run(ctx, "pm", "install", "-r", "-d", change.backup)
			_, _ = m.executor.Run(ctx, "rm", "-f", change.backup)
		} else {
			_, _ = m.executor.Run(ctx, "pm", "uninstall", change.pkg)
		}
	}
}
func commitPackages(changes []packageChange) {
	for _, change := range changes {
		if change.existed {
			_ = os.Remove(change.backup)
		}
	}
}
func joinAction(left, right string) string {
	if left == "" {
		return right
	}
	if right == "" {
		return left
	}
	return left + "," + right
}
