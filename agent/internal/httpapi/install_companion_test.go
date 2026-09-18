package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/zhoujun94511/xtest-nova/agent/internal/apps"
	"github.com/zhoujun94511/xtest-nova/agent/internal/command"
)

type companionApplications struct {
	version  int64
	apkPath  string
	installs int
	packages []apps.Package
	listAll  bool
	iconErr  error
}

func (a *companionApplications) List(_ context.Context, includeSystem bool) ([]apps.Package, error) {
	a.listAll = includeSystem
	return a.packages, nil
}
func (a *companionApplications) Info(_ context.Context, name string) (apps.Package, error) {
	if name != command.PopupPackage || a.version == 0 {
		return apps.Package{}, errors.New("not installed")
	}
	return apps.Package{PackageName: name, VersionCode: a.version, APKPath: a.apkPath}, nil
}
func (a *companionApplications) Icon(context.Context, string) ([]byte, error) {
	if a.iconErr != nil {
		return nil, a.iconErr
	}
	return []byte{0xff, 0xd8, 0xff}, nil
}
func (a *companionApplications) Session(context.Context, string) (apps.Package, string, error) {
	return apps.Package{}, "", nil
}
func (a *companionApplications) Launch(context.Context, string) (string, error) { return "", nil }
func (a *companionApplications) Stop(context.Context, string) error             { return nil }
func (a *companionApplications) Uninstall(context.Context, string, bool) (string, error) {
	return "", nil
}
func (a *companionApplications) Install(context.Context, string, bool) (string, error) {
	a.installs++
	return "Success", nil
}

func TestInstallCompanionOnlySkipsExactVersion(t *testing.T) {
	payload := t.TempDir() + "/companion.apk"
	if err := os.WriteFile(payload, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name     string
		version  int64
		installs int
	}{
		{name: "exact", version: command.PopupVersionCode, installs: 0},
		{name: "older Nova", version: command.PopupVersionCode - 1, installs: 1},
		{name: "legacy threshold", version: 20001, installs: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			applications := &companionApplications{version: test.version, apkPath: payload}
			api := &API{apps: applications, companionAPK: payload}
			response := httptest.NewRecorder()
			api.installCompanion(response, httptest.NewRequest(http.MethodPost, "/installAgentApk", nil))
			if response.Code != http.StatusOK || applications.installs != test.installs {
				t.Fatalf("status=%d installs=%d body=%s", response.Code, applications.installs, response.Body.String())
			}
		})
	}
	different := t.TempDir() + "/installed.apk"
	if err := os.WriteFile(different, []byte("different"), 0600); err != nil {
		t.Fatal(err)
	}
	applications := &companionApplications{version: command.PopupVersionCode, apkPath: different}
	api := &API{apps: applications, companionAPK: payload}
	response := httptest.NewRecorder()
	api.installCompanion(response, httptest.NewRequest(http.MethodPost, "/installAgentApk", nil))
	if response.Code != http.StatusOK || applications.installs != 1 {
		t.Fatalf("same version with different bytes was not replaced: status=%d installs=%d", response.Code, applications.installs)
	}
}
