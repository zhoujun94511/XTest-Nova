package automation

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeHierarchyProvider struct {
	name   string
	value  Snapshot
	err    error
	values []Snapshot
	errors []error
	calls  int
}

func (p *fakeHierarchyProvider) Name() string                { return p.name }
func (p *fakeHierarchyProvider) Ready(context.Context) error { return nil }
func (p *fakeHierarchyProvider) Hierarchy(context.Context) (Snapshot, error) {
	p.calls++
	if index := p.calls - 1; index < len(p.values) || index < len(p.errors) {
		var value Snapshot
		var err error
		if index < len(p.values) {
			value = p.values[index]
		}
		if index < len(p.errors) {
			err = p.errors[index]
		}
		return value, err
	}
	return p.value, p.err
}

func TestNovaEmptyFirstFrameRetriesBeforeFallback(t *testing.T) {
	nova := &fakeHierarchyProvider{name: "nova-provider", values: []Snapshot{{}, {Source: "nova-provider", XML: `<hierarchy><node text="ready" /></hierarchy>`}}, errors: []error{errors.New("nova-provider returned a hierarchy without nodes"), nil}}
	fallback := &fakeHierarchyProvider{name: "system-dump", value: Snapshot{Source: "system-dump", XML: `<hierarchy><node text="fallback" /></hierarchy>`}}
	manager := NewWithProviders(nil, time.Second, nova, fallback)
	document, err := manager.Hierarchy(context.Background())
	if err != nil || document != nova.values[1].XML {
		t.Fatalf("Hierarchy() document=%q err=%v", document, err)
	}
	if nova.calls != 2 || fallback.calls != 0 {
		t.Fatalf("nova calls=%d fallback calls=%d", nova.calls, fallback.calls)
	}
}

func TestHierarchyFallsBackAndRecordsMetrics(t *testing.T) {
	primary := &fakeHierarchyProvider{name: "primary", err: errors.New("dump failed")}
	fallback := &fakeHierarchyProvider{name: "fallback", value: Snapshot{
		Source: "fallback", CapturedAt: time.Now().UTC(), XML: `<hierarchy><node text="ok" /></hierarchy>`, Bytes: 42, Nodes: 1,
	}}
	manager := NewWithProviders(nil, time.Second, primary, fallback)
	document, err := manager.Hierarchy(context.Background())
	if err != nil || document != fallback.value.XML {
		t.Fatalf("Hierarchy() document=%q err=%v", document, err)
	}
	diagnostics := manager.hierarchyDiagnostics()
	if diagnostics.LastSource != "fallback" || diagnostics.Fallbacks != 1 || primary.calls != 1 || fallback.calls != 1 {
		t.Fatalf("unexpected fallback diagnostics: %+v calls=%d/%d", diagnostics, primary.calls, fallback.calls)
	}
	if diagnostics.Providers["primary"].Failures != 1 || diagnostics.Providers["fallback"].Successes != 1 {
		t.Fatalf("unexpected provider metrics: %+v", diagnostics.Providers)
	}
}

func TestNovaProviderUsesBearerTokenAndParsesHierarchy(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/health" {
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		_, _ = w.Write([]byte(`{"hierarchy":"<hierarchy><node class=\"android.view.View\" /></hierarchy>"}`))
	}))
	defer server.Close()
	provider := NewNovaProvider(server.URL+"/v1/hierarchy", server.URL+"/health", "secret", server.Client())
	if err := provider.Ready(context.Background()); err != nil {
		t.Fatal(err)
	}
	value, err := provider.Hierarchy(context.Background())
	if err != nil || value.Source != "nova-provider" || value.Nodes != 1 {
		t.Fatalf("unexpected snapshot: %+v err=%v", value, err)
	}
}

func TestSnapshotRejectsEmptyAndMalformedTrees(t *testing.T) {
	for _, document := range []string{"", `<hierarchy />`, `<hierarchy><node></hierarchy>`} {
		if _, err := snapshot("test", []byte(document)); err == nil {
			t.Fatalf("snapshot accepted %q", document)
		}
	}
}

func TestProviderSnapshotUsesLightweightValidation(t *testing.T) {
	value, err := providerSnapshot("test", []byte(`<?xml version="1.0"?><hierarchy><node class="View"><node></node></node></hierarchy>`))
	if err != nil {
		t.Fatal(err)
	}
	if value.Nodes != 2 || value.Fingerprint == "" || value.StructureFingerprint != "" || value.AttributeFingerprints != nil {
		t.Fatalf("unexpected lightweight snapshot: %+v", value)
	}
	for _, document := range []string{"", `<hierarchy></hierarchy>`, `<hierarchy><node class="View">`} {
		if _, snapshotErr := providerSnapshot("test", []byte(document)); snapshotErr == nil {
			t.Fatalf("providerSnapshot accepted %q", document)
		}
	}
}

func TestHierarchyFingerprintsNormalizeFormattingAndSeparateDifferences(t *testing.T) {
	first, err := snapshot("first", []byte(`<hierarchy><node text="A" class="Button"><node bounds="[0,0][1,1]" /></node></hierarchy>`))
	if err != nil {
		t.Fatal(err)
	}
	formatted, err := snapshot("formatted", []byte(`<hierarchy>
  <node class="Button" text="A"><node bounds="[0,0][1,1]"/></node>
</hierarchy>`))
	if err != nil {
		t.Fatal(err)
	}
	if first.Fingerprint != formatted.Fingerprint {
		t.Fatal("formatting or XML attribute order changed the hierarchy fingerprint")
	}
	changedText, err := snapshot("text", []byte(`<hierarchy><node text="B" class="Button"><node bounds="[0,0][1,1]" /></node></hierarchy>`))
	if err != nil {
		t.Fatal(err)
	}
	if first.StructureFingerprint != changedText.StructureFingerprint || first.AttributeFingerprints["class"] != changedText.AttributeFingerprints["class"] || first.AttributeFingerprints["text"] == changedText.AttributeFingerprints["text"] {
		t.Fatal("attribute-only change was not isolated from structure and unchanged attributes")
	}
	changedStructure, err := snapshot("structure", []byte(`<hierarchy><node text="A" class="Button" /><node bounds="[0,0][1,1]" /></hierarchy>`))
	if err != nil {
		t.Fatal(err)
	}
	if first.StructureFingerprint == changedStructure.StructureFingerprint {
		t.Fatal("tree topology change did not change the structure fingerprint")
	}
}

func TestHierarchyDifferenceMatchesStableNodesAndExplainsExtras(t *testing.T) {
	primary := `<hierarchy><node resource-id="plant:id/continue" class="android.widget.TextView" package="plant.care.identifier.app" text="Continue" bounds="[1,2][3,4]" /></hierarchy>`
	shadow := `<hierarchy><node class="android.widget.TextView" resource-id="plant:id/continue" package="plant.care.identifier.app" text="Next" bounds="[1,2][3,4]" /><node class="android.widget.FrameLayout" package="com.android.systemui" bounds="[0,0][10,10]" /></hierarchy>`
	difference, err := compareHierarchyDocuments(primary, shadow)
	if err != nil {
		t.Fatal(err)
	}
	if difference.MatchedNodes != 1 || difference.UnmatchedPrimaryNodes != 0 || difference.UnmatchedShadowNodes != 1 {
		t.Fatalf("unexpected node matching: %+v", difference)
	}
	if difference.AttributeMismatchCounts["text"] != 1 || difference.AttributeMismatchCounts["class"] != 0 {
		t.Fatalf("unexpected attribute mismatches: %+v", difference.AttributeMismatchCounts)
	}
	if difference.PackageNodeDeltas["com.android.systemui"] != 1 {
		t.Fatalf("unexpected package deltas: %+v", difference.PackageNodeDeltas)
	}
	if difference.UnmatchedShadowPackageCounts["com.android.systemui"] != 1 || len(difference.UnmatchedPrimaryPackageCounts) != 0 {
		t.Fatalf("unexpected unmatched package counts: primary=%+v shadow=%+v", difference.UnmatchedPrimaryPackageCounts, difference.UnmatchedShadowPackageCounts)
	}
}

func TestHierarchyDifferenceUsesSemanticIdentityBeforeBounds(t *testing.T) {
	primary := `<hierarchy><node class="android.view.View" package="plant.care.identifier.app" text="Privacy Policy" bounds="[0,10][100,20]" /></hierarchy>`
	shadow := `<hierarchy><node class="android.view.View" package="plant.care.identifier.app" text="Privacy Policy" bounds="[0,20][100,30]" /></hierarchy>`
	difference, err := compareHierarchyDocuments(primary, shadow)
	if err != nil {
		t.Fatal(err)
	}
	if difference.MatchedNodes != 1 || difference.UnmatchedPrimaryNodes != 0 || difference.UnmatchedShadowNodes != 0 {
		t.Fatalf("semantic node was not matched across bounds: %+v", difference)
	}
	if difference.AttributeMismatchCounts["bounds"] != 1 {
		t.Fatalf("bounds difference was not recorded: %+v", difference.AttributeMismatchCounts)
	}
}

func TestShadowProviderDoesNotChangeSelectedHierarchy(t *testing.T) {
	primary := &fakeHierarchyProvider{name: "system-dump", value: Snapshot{
		Source: "system-dump", CapturedAt: time.Now().UTC(), XML: `<hierarchy><node /></hierarchy>`, Bytes: 31, Nodes: 1,
	}}
	shadow := &fakeHierarchyProvider{name: "nova-provider", value: Snapshot{
		Source: "nova-provider", CapturedAt: time.Now().UTC(), XML: `<hierarchy><node /><node /></hierarchy>`, Bytes: 39, Nodes: 2,
	}}
	manager := NewWithProviders(nil, time.Second, primary)
	manager.shadowProvider = shadow
	document, err := manager.Hierarchy(context.Background())
	if err != nil || document != primary.value.XML {
		t.Fatalf("shadow changed selected hierarchy: %q err=%v", document, err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && manager.hierarchyDiagnostics().ShadowComparisons == 0 {
		time.Sleep(time.Millisecond)
	}
	diagnostics := manager.hierarchyDiagnostics()
	if diagnostics.LastSource != "system-dump" || diagnostics.ShadowComparisons != 1 || diagnostics.LastShadowNodeDelta != 1 || diagnostics.LastShadowStructureMatch {
		t.Fatalf("unexpected shadow diagnostics: %+v", diagnostics)
	}
	if len(diagnostics.LastShadowAttributeMatches) != len(hierarchyAttributes) || diagnostics.LastShadowAttributeMatches["text"] || diagnostics.LastShadowFingerprintMatch {
		t.Fatalf("unexpected shadow attribute comparison: %+v", diagnostics.LastShadowAttributeMatches)
	}
}

func TestProviderMetricsRecordFingerprintChanges(t *testing.T) {
	provider := &fakeHierarchyProvider{name: "system-dump", value: Snapshot{Source: "system-dump", XML: `<hierarchy><node text="one" /></hierarchy>`}}
	manager := NewWithProviders(nil, time.Second, provider)
	if _, err := manager.Hierarchy(context.Background()); err != nil {
		t.Fatal(err)
	}
	provider.value.XML = `<hierarchy><node text="two" /></hierarchy>`
	if _, err := manager.Hierarchy(context.Background()); err != nil {
		t.Fatal(err)
	}
	metric := manager.hierarchyDiagnostics().Providers["system-dump"]
	if metric.FingerprintSamples != 2 || metric.FingerprintChanges != 1 || metric.LastFingerprint == "" {
		t.Fatalf("unexpected fingerprint metrics: %+v", metric)
	}
}

func TestConfigureNovaRequiresLoopback(t *testing.T) {
	manager := NewWithProviders(nil, time.Second)
	for _, test := range []struct {
		mode    string
		address string
		ok      bool
	}{{"system", "127.0.0.1:9009", true}, {"shadow", "[::1]:9009", true}, {"nova", "0.0.0.0:9009", false}, {"other", "127.0.0.1:9009", false}} {
		err := manager.ConfigureNova(test.mode, test.address)
		if (err == nil) != test.ok {
			t.Fatalf("ConfigureNova(%q, %q) err=%v", test.mode, test.address, err)
		}
	}
}

type novaExecutor struct {
	started   chan struct{}
	startOnce sync.Once
	forceStop string
}

func (e *novaExecutor) Run(ctx context.Context, name string, args ...string) (string, error) {
	if name == "pm" {
		return "package:/data/app/base.apk", nil
	}
	if name == "am" && len(args) > 0 && args[0] == "instrument" {
		e.startOnce.Do(func() { close(e.started) })
		<-ctx.Done()
		return "", ctx.Err()
	}
	if name == "am" && len(args) > 1 && args[0] == "force-stop" {
		e.forceStop = args[1]
		return "", nil
	}
	return "", nil
}

func (*novaExecutor) RunBytes(context.Context, string, ...string) ([]byte, error) { return nil, nil }

func TestNovaLifecycleSelectsProviderAndStopsOnlyOwnedPackage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(r.Header.Get("Authorization")) != len("Bearer ")+64 {
			http.Error(w, "missing random token", http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/health" {
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		_, _ = w.Write([]byte(`<hierarchy><node text="nova" /></hierarchy>`))
	}))
	defer server.Close()
	executor := &novaExecutor{started: make(chan struct{})}
	manager := NewWithProviders(executor, time.Second, &fakeHierarchyProvider{name: "system", err: errors.New("must not be selected")})
	if err := manager.ConfigureNova("nova", server.Listener.Addr().String()); err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(); err != nil {
		t.Fatal(err)
	}
	<-executor.started
	document, err := manager.Hierarchy(context.Background())
	if err != nil || document != `<hierarchy><node text="nova" /></hierarchy>` {
		t.Fatalf("Nova hierarchy=%q err=%v", document, err)
	}
	if err := manager.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if executor.forceStop != "com.openatx.xtest.nova.uiautomator" {
		t.Fatalf("force-stopped %q", executor.forceStop)
	}
}

func TestFailedNovaReleasesUiAutomationBeforeSystemFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		_, _ = w.Write([]byte(`<hierarchy/>`))
	}))
	defer server.Close()
	executor := &novaExecutor{started: make(chan struct{})}
	fallback := &fakeHierarchyProvider{name: "system-dump", value: Snapshot{Source: "system-dump", XML: `<hierarchy><node text="fallback" /></hierarchy>`}}
	manager := NewWithProviders(executor, time.Second, fallback)
	if err := manager.ConfigureNova("nova", server.Listener.Addr().String()); err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(); err != nil {
		t.Fatal(err)
	}
	<-executor.started
	document, err := manager.Hierarchy(context.Background())
	if err != nil || document != fallback.value.XML {
		t.Fatalf("fallback hierarchy=%q err=%v", document, err)
	}
	if executor.forceStop != novaHostPackage || fallback.calls != 1 || manager.Running() {
		t.Fatalf("forceStop=%q fallbackCalls=%d running=%v", executor.forceStop, fallback.calls, manager.Running())
	}
	if diagnostics := manager.hierarchyDiagnostics(); diagnostics.Fallbacks != 1 {
		t.Fatalf("diagnostics=%+v", diagnostics)
	}
}

func TestShadowModeUsesTransientNovaAfterPrimaryCapture(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		_, _ = w.Write([]byte(`<hierarchy><node text="shadow" /></hierarchy>`))
	}))
	defer server.Close()
	executor := &novaExecutor{started: make(chan struct{})}
	primary := &fakeHierarchyProvider{name: "system-dump", value: Snapshot{Source: "system-dump", XML: `<hierarchy><node text="primary" /></hierarchy>`}}
	manager := NewWithProviders(executor, time.Second, primary)
	if err := manager.ConfigureNova("shadow", server.Listener.Addr().String()); err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-executor.started:
		t.Fatal("shadow mode started persistent instrumentation before a primary capture")
	default:
	}
	document, err := manager.Hierarchy(context.Background())
	if err != nil || document != primary.value.XML {
		t.Fatalf("shadow changed primary result: document=%q err=%v", document, err)
	}
	select {
	case <-executor.started:
	case <-time.After(time.Second):
		t.Fatal("transient shadow instrumentation did not start")
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && manager.hierarchyDiagnostics().ShadowComparisons == 0 {
		time.Sleep(time.Millisecond)
	}
	if diagnostics := manager.hierarchyDiagnostics(); diagnostics.ShadowComparisons != 1 || diagnostics.LastSource != "system-dump" {
		t.Fatalf("unexpected transient shadow diagnostics: %+v", diagnostics)
	}
	if executor.forceStop != novaHostPackage {
		t.Fatalf("transient shadow force-stopped %q", executor.forceStop)
	}
	if err := manager.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

type restartExecutor struct {
	mu            sync.Mutex
	instruments   int
	firstStarted  chan struct{}
	secondStarted chan struct{}
	releaseFirst  chan struct{}
}

func (e *restartExecutor) Run(ctx context.Context, name string, args ...string) (string, error) {
	if name == "pm" {
		return "package:/data/app/uiautomator.apk", nil
	}
	if name == "am" && len(args) > 0 && args[0] == "force-stop" {
		return "", nil
	}
	if name == "am" && len(args) > 0 && args[0] == "instrument" {
		e.mu.Lock()
		e.instruments++
		call := e.instruments
		e.mu.Unlock()
		if call == 1 {
			close(e.firstStarted)
			<-ctx.Done()
			<-e.releaseFirst
			return "", ctx.Err()
		}
		close(e.secondStarted)
		<-ctx.Done()
		return "", ctx.Err()
	}
	return "", nil
}

func (*restartExecutor) RunBytes(context.Context, string, ...string) ([]byte, error) {
	return nil, nil
}

func TestOldInstrumentationExitDoesNotClearRestartedState(t *testing.T) {
	executor := &restartExecutor{
		firstStarted:  make(chan struct{}),
		secondStarted: make(chan struct{}),
		releaseFirst:  make(chan struct{}),
	}
	manager := New(executor, 100*time.Millisecond, true)
	if err := manager.Start(); err != nil {
		t.Fatal(err)
	}
	<-executor.firstStarted
	if err := manager.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(); err != nil {
		t.Fatal(err)
	}
	<-executor.secondStarted
	close(executor.releaseFirst)
	time.Sleep(20 * time.Millisecond)

	manager.mu.Lock()
	running, owned := manager.running, manager.owned
	manager.mu.Unlock()
	if !running || !owned {
		t.Fatalf("old exit cleared restarted state: running=%v owned=%v", running, owned)
	}
	if err := manager.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyInstrumentationIsDisabledByDefault(t *testing.T) {
	manager := New(nil, 100*time.Millisecond)
	if err := manager.Start(); err == nil || !strings.Contains(err.Error(), "--legacy-uiautomator") {
		t.Fatalf("Start error = %v, want explicit legacy gate", err)
	}
	if len(manager.baseProviders) != 1 || manager.baseProviders[0].Name() != "system-dump" {
		t.Fatalf("default providers = %#v, want only system-dump", manager.baseProviders)
	}
}
