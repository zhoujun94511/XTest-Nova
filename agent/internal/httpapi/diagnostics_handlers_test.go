package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zhoujun94511/xtest-nova/agent/internal/artifactcatalog"
	"github.com/zhoujun94511/xtest-nova/agent/internal/componenthealth"
	"github.com/zhoujun94511/xtest-nova/agent/internal/configstore"
	"github.com/zhoujun94511/xtest-nova/agent/internal/logview"
	"github.com/zhoujun94511/xtest-nova/agent/internal/monitor"
)

func newDiagnosticsTestAPI(t *testing.T) *API {
	t.Helper()
	a := New(fakeDevice{}, &fakeRunner{}, configstore.NewEmpty(filepath.Join(t.TempDir(), "config.json")), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "", false)
	return a
}

func TestComponentDiagnosticsIncludesAuxiliaryStatus(t *testing.T) {
	a := newDiagnosticsTestAPI(t)
	a.SetAuxiliaryStatus(componenthealth.Degraded("companion", true, Version, "port conflict"))
	response := httptest.NewRecorder()
	a.Primary().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/diagnostics/components", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"schemaVersion":"xtest-nova-components/v1"`) || !strings.Contains(response.Body.String(), `"name":"companion"`) || !strings.Contains(response.Body.String(), `"state":"degraded"`) {
		t.Fatalf("unexpected response %d: %s", response.Code, response.Body.String())
	}
}

func TestHealthRemainsLiveWhenAuxiliaryMonitorIsDegraded(t *testing.T) {
	a := newDiagnosticsTestAPI(t)
	a.monitor = monitor.New(nil, "127.0.0.1:0")
	response := httptest.NewRecorder()
	a.Primary().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/health", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"degraded"`) {
		t.Fatalf("health returned %d: %s", response.Code, response.Body.String())
	}
}

func TestArtifactDiagnosticsUsesCatalogValidation(t *testing.T) {
	a := newDiagnosticsTestAPI(t)
	root := t.TempDir()
	session := filepath.Join(root, "com.example.app", "Monkey", "20260911_120000")
	if err := os.MkdirAll(session, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(session, "run.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	a.artifactIndex = artifactcatalog.New(root)
	response := httptest.NewRecorder()
	a.Primary().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/diagnostics/artifacts?package=com.example.app", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "run.json") {
		t.Fatalf("unexpected response %d: %s", response.Code, response.Body.String())
	}
	rejected := httptest.NewRecorder()
	a.Primary().ServeHTTP(rejected, httptest.NewRequest(http.MethodGet, "/v1/diagnostics/artifacts?package=../tmp", nil))
	if rejected.Code != http.StatusBadRequest {
		t.Fatalf("traversal returned %d", rejected.Code)
	}
}

func TestLogDiagnosticsRejectsUnknownAlias(t *testing.T) {
	a := newDiagnosticsTestAPI(t)
	a.logs = logview.New(map[string]string{"agent": filepath.Join(t.TempDir(), "agent.log")})
	response := httptest.NewRecorder()
	a.Primary().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/diagnostics/logs/secret", nil))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("unknown alias returned %d", response.Code)
	}
}

func TestSoakAPIRejectsUnboundedDuration(t *testing.T) {
	a := newDiagnosticsTestAPI(t)
	response := httptest.NewRecorder()
	a.Primary().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/diagnostics/soak", strings.NewReader(`{"durationSeconds":59,"intervalSeconds":5}`)))
	if response.Code != http.StatusBadRequest {
		var body any
		_ = json.Unmarshal(response.Body.Bytes(), &body)
		t.Fatalf("unbounded soak returned %d: %#v", response.Code, body)
	}
}
