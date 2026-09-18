package httpapi

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zhoujun94511/xtest-nova/agent/internal/apps"
	"github.com/zhoujun94511/xtest-nova/agent/internal/tasks"
)

func TestPackageIconUsesSuccessfulPlaceholderResponse(t *testing.T) {
	api := &API{apps: &companionApplications{iconErr: errors.New("icon unavailable")}}
	mux := http.NewServeMux()
	api.registerUtilityRoutes(mux)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/packages/com.example.app/icon", nil))
	if response.Code != http.StatusOK || response.Header().Get("X-XTest-Icon-Fallback") != "true" || !strings.Contains(response.Body.String(), "<svg") {
		t.Fatalf("status=%d headers=%v body=%q", response.Code, response.Header(), response.Body.String())
	}
}

func TestPackageScopeFiltersSystemApplications(t *testing.T) {
	applications := &companionApplications{packages: []apps.Package{
		{PackageName: "com.example.user", Name: "用户应用"},
		{PackageName: "com.android.system", Name: "系统应用", System: true},
	}}
	api := &API{apps: applications}
	response := httptest.NewRecorder()
	api.listPackages(response, httptest.NewRequest(http.MethodGet, "/packages?scope=system", nil))
	if response.Code != http.StatusOK || !applications.listAll || strings.Contains(response.Body.String(), "com.example.user") || !strings.Contains(response.Body.String(), "com.android.system") {
		t.Fatalf("status=%d listAll=%v body=%s", response.Code, applications.listAll, response.Body.String())
	}

	response = httptest.NewRecorder()
	api.listPackages(response, httptest.NewRequest(http.MethodGet, "/packages?scope=invalid", nil))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid scope status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestMissingPackageTaskReturnsNotFound(t *testing.T) {
	api := &API{tasks: tasks.New()}
	request := httptest.NewRequest(http.MethodGet, "/packages/missing", nil)
	request.SetPathValue("id", "missing")
	response := httptest.NewRecorder()
	api.packageTask(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("missing package task returned %d: %s", response.Code, response.Body.String())
	}
}
