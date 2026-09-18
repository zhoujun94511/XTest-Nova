package runtimebootstrap

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/zhoujun94511/xtest-nova/agent/internal/runtimebundle"
)

type memorySource struct {
	components []runtimebundle.Component
	payloads   map[string][]byte
}

func (s memorySource) Components() ([]runtimebundle.Component, error) {
	return append([]runtimebundle.Component(nil), s.components...), nil
}
func (s memorySource) Data(name string) ([]byte, error) {
	data, ok := s.payloads[name]
	if !ok {
		return nil, errors.New("missing")
	}
	return append([]byte(nil), data...), nil
}

type fakeExecutor struct {
	packages        map[string]string
	packageForFile  map[string]string
	failInstall     string
	calls           []string
	atomic          bool
	failAtomicWrite bool
	pathFailures    map[string]int
	nextSession     int
	sessionFiles    map[int]string
}

func (e *fakeExecutor) Run(_ context.Context, name string, args ...string) (string, error) {
	e.calls = append(e.calls, name+" "+strings.Join(args, " "))
	if name == "pm" && len(args) >= 2 && args[0] == "path" {
		if e.pathFailures[args[1]] > 0 {
			e.pathFailures[args[1]]--
			return "", errors.New("package manager temporarily unavailable")
		}
		path, ok := e.packages[args[1]]
		if !ok {
			return "", errors.New("not installed")
		}
		return "package:" + path, nil
	}
	if name == "pm" && len(args) >= 1 && args[0] == "install-create" {
		if !e.atomic {
			return "Error: multi-package sessions unsupported", errors.New("unsupported")
		}
		e.nextSession++
		if e.sessionFiles == nil {
			e.sessionFiles = map[int]string{}
		}
		return fmt.Sprintf("Success: created install session [%d]", e.nextSession), nil
	}
	if name == "pm" && len(args) >= 6 && args[0] == "install-write" {
		if e.failAtomicWrite {
			e.failAtomicWrite = false
			return "Failure [INSTALL_FAILED_SESSION_INVALID]", errors.New("write failed")
		}
		session, err := strconv.Atoi(args[3])
		if err != nil {
			return "Failure", err
		}
		e.sessionFiles[session] = args[len(args)-1]
		return "Success: streamed", nil
	}
	if name == "pm" && len(args) >= 3 && args[0] == "install-add-session" {
		return "Success", nil
	}
	if name == "pm" && len(args) >= 2 && args[0] == "install-commit" {
		for _, target := range e.sessionFiles {
			pkg := e.packageForFile[filepath.Base(target)]
			e.packages[pkg] = target
		}
		return "Success", nil
	}
	if name == "pm" && len(args) >= 2 && args[0] == "install-abandon" {
		session, _ := strconv.Atoi(args[1])
		delete(e.sessionFiles, session)
		return "Success", nil
	}
	if name == "pm" && len(args) >= 3 && args[0] == "install" {
		target := args[len(args)-1]
		if target == e.failInstall {
			return "Failure", errors.New("install failed")
		}
		pkg := e.packageForFile[filepath.Base(target)]
		e.packages[pkg] = target
		return "Success", nil
	}
	if name == "pm" && len(args) >= 2 && args[0] == "uninstall" {
		delete(e.packages, args[1])
		return "Success", nil
	}
	if name == "sha256sum" && len(args) == 1 {
		data, err := os.ReadFile(args[0])
		if err != nil {
			return "", err
		}
		sum := sha256.Sum256(data)
		return hex.EncodeToString(sum[:]) + "  " + args[0], nil
	}
	if name == "cp" && len(args) == 3 {
		return "", copyFile(args[1], args[2])
	}
	if name == "rm" && len(args) >= 2 {
		err := os.Remove(args[len(args)-1])
		if os.IsNotExist(err) {
			err = nil
		}
		return "", err
	}
	if name == "appops" {
		return "", nil
	}
	return "", fmt.Errorf("unexpected command %s %v", name, args)
}
func copyFile(source, target string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	return os.WriteFile(target, data, 0o644)
}

func testBundle(root string) (memorySource, Targets) {
	definitions := []struct{ name, file, kind, pkg string }{{"runner", "runner.jar", "jar", ""}, {"companion", "companion.apk", "apk", "popup.pkg"}, {"uiautomatorHost", "host.apk", "apk", "host.pkg"}, {"uiautomatorTest", "test.apk", "apk", "test.pkg"}}
	components := make([]runtimebundle.Component, 0, 4)
	payloads := map[string][]byte{}
	targets := Targets{Runner: filepath.Join(root, "runner.jar"), Companion: filepath.Join(root, "companion.apk"), UIAutomatorHost: filepath.Join(root, "host.apk"), UIAutomatorTest: filepath.Join(root, "test.apk")}
	for _, definition := range definitions {
		payload := []byte("payload-" + definition.name)
		sum := sha256.Sum256(payload)
		payloads[definition.file] = payload
		components = append(components, runtimebundle.Component{Name: definition.name, File: definition.file, Kind: definition.kind, Package: definition.pkg, Size: int64(len(payload)), SHA256: hex.EncodeToString(sum[:])})
	}
	return memorySource{components: components, payloads: payloads}, targets
}

func TestEnsureInstallsAPKsInOneAtomicTransactionAndThenReusesThem(t *testing.T) {
	root := t.TempDir()
	source, targets := testBundle(root)
	executor := &fakeExecutor{
		packages:       map[string]string{},
		packageForFile: map[string]string{"companion.apk": "popup.pkg", "host.apk": "host.pkg", "test.apk": "test.pkg"},
		atomic:         true,
	}
	manager := New(executor, source, targets)
	state, err := manager.Ensure(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !state.Ready || len(executor.packages) != 3 {
		t.Fatalf("state=%+v packages=%+v", state, executor.packages)
	}
	if countCall(executor.calls, "pm install-create -r --multi-package") != 1 || countCall(executor.calls, "pm install-commit ") != 1 || countCall(executor.calls, "pm install -r ") != 0 {
		t.Fatalf("expected one atomic transaction, calls=%v", executor.calls)
	}
	for _, component := range state.Components {
		if component.Kind == "apk" && component.Action != "extracted,installed-atomic" {
			t.Fatalf("unexpected atomic action for %s: %s", component.Name, component.Action)
		}
	}

	executor.calls = nil
	state, err = manager.Ensure(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if countCall(executor.calls, "pm install-create") != 0 {
		t.Fatalf("unchanged packages must not create an install session: %v", executor.calls)
	}
	for _, component := range state.Components {
		if component.Kind == "apk" && component.Action != "reused,installed-reused" {
			t.Fatalf("unexpected reuse action for %s: %s", component.Name, component.Action)
		}
	}
}

func TestEnsureFallsBackAfterAtomicPreparationFailure(t *testing.T) {
	root := t.TempDir()
	source, targets := testBundle(root)
	executor := &fakeExecutor{
		packages:        map[string]string{},
		packageForFile:  map[string]string{"companion.apk": "popup.pkg", "host.apk": "host.pkg", "test.apk": "test.pkg"},
		atomic:          true,
		failAtomicWrite: true,
	}
	state, err := New(executor, source, targets).Ensure(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !state.Ready || len(executor.packages) != 3 || len(executor.sessionFiles) != 0 {
		t.Fatalf("state=%+v packages=%+v sessions=%+v", state, executor.packages, executor.sessionFiles)
	}
	if countCall(executor.calls, "pm install-abandon ") < 2 || countCall(executor.calls, "pm install -r ") != 3 {
		t.Fatalf("expected child/parent cleanup and sequential fallback, calls=%v", executor.calls)
	}
	for _, component := range state.Components {
		if component.Kind == "apk" && component.Action != "extracted,installed" {
			t.Fatalf("unexpected fallback action for %s: %s", component.Name, component.Action)
		}
	}
}

func TestEnsureRetriesTransientPackageVisibilityWithoutReinstalling(t *testing.T) {
	root := t.TempDir()
	source, targets := testBundle(root)
	executor := &fakeExecutor{
		packages:       map[string]string{},
		packageForFile: map[string]string{"companion.apk": "popup.pkg", "host.apk": "host.pkg", "test.apk": "test.pkg"},
		atomic:         true,
		pathFailures:   map[string]int{},
	}
	manager := New(executor, source, targets)
	if _, err := manager.Ensure(context.Background()); err != nil {
		t.Fatal(err)
	}
	executor.calls = nil
	for _, pkg := range []string{"popup.pkg", "host.pkg", "test.pkg"} {
		executor.pathFailures[pkg] = 2
	}
	state, err := manager.Ensure(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if countCall(executor.calls, "pm install-create") != 0 || countCall(executor.calls, "pm install -r ") != 0 {
		t.Fatalf("transient package visibility must not reinstall: %v", executor.calls)
	}
	for _, component := range state.Components {
		if component.Kind == "apk" && component.Action != "reused,installed-reused" {
			t.Fatalf("unexpected retry action for %s: %s", component.Name, component.Action)
		}
	}
}

func countCall(calls []string, prefix string) int {
	count := 0
	for _, call := range calls {
		if strings.HasPrefix(call, prefix) {
			count++
		}
	}
	return count
}

func TestEnsureExtractsAndInstallsCompleteBundle(t *testing.T) {
	root := t.TempDir()
	source, targets := testBundle(root)
	executor := &fakeExecutor{packages: map[string]string{}, packageForFile: map[string]string{"companion.apk": "popup.pkg", "host.apk": "host.pkg", "test.apk": "test.pkg"}}
	state, err := New(executor, source, targets).Ensure(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !state.Ready || len(state.Components) != 4 || len(executor.packages) != 3 {
		t.Fatalf("state=%+v packages=%+v", state, executor.packages)
	}
	if data, err := os.ReadFile(targets.Runner); err != nil || string(data) != "payload-runner" {
		t.Fatalf("runner=%q err=%v", data, err)
	}
}

func TestEnsureRollsBackFilesAndInstalledPackages(t *testing.T) {
	root := t.TempDir()
	source, targets := testBundle(root)
	if err := os.WriteFile(targets.Runner, []byte("previous"), 0o644); err != nil {
		t.Fatal(err)
	}
	executor := &fakeExecutor{packages: map[string]string{}, packageForFile: map[string]string{"companion.apk": "popup.pkg", "host.apk": "host.pkg", "test.apk": "test.pkg"}, failInstall: targets.UIAutomatorHost}
	manager := New(executor, source, targets)
	_, err := manager.Ensure(context.Background())
	if err == nil {
		t.Fatal("expected install failure")
	}
	state := manager.State()
	if state.Ready {
		t.Fatalf("state unexpectedly ready: %+v", state)
	}
	data, readErr := os.ReadFile(targets.Runner)
	if readErr != nil || string(data) != "previous" {
		t.Fatalf("runner rollback=%q err=%v", data, readErr)
	}
	if len(executor.packages) != 0 {
		t.Fatalf("packages not rolled back: %+v", executor.packages)
	}
}
