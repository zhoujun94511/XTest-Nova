package automation

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/zhoujun94511/xtest-nova/agent/internal/platform"
)

type Manager struct {
	executor       platform.Executor
	timeout        time.Duration
	providers      []HierarchyProvider
	baseProviders  []HierarchyProvider
	shadowProvider HierarchyProvider
	shadowSlot     chan struct{}
	novaMode       string
	novaAddress    string
	legacyEnabled  bool
	ownedPackage   string
	metricsMu      sync.RWMutex
	metrics        HierarchyDiagnostics
	mu             sync.Mutex
	captureMu      sync.Mutex
	hierarchyMu    sync.Mutex
	commandContext context.CancelFunc
	running        bool
	owned          bool
	idleTimeout    time.Duration
	idleTimer      *time.Timer
	generation     uint64
}

const (
	novaHostPackage     = "com.openatx.xtest.nova.uiautomator"
	novaTestPackage     = "com.openatx.xtest.nova.uiautomator.test"
	novaInstrumentation = novaTestPackage + "/.NovaInstrumentation"
)

func New(executor platform.Executor, timeout time.Duration, legacy ...bool) *Manager {
	client := &http.Client{Timeout: timeout}
	providers := []HierarchyProvider{newSystemDumpProvider(executor)}
	enabled := len(legacy) > 0 && legacy[0]
	if enabled {
		providers = append(providers, newLegacyRPCProvider(client))
	}
	manager := NewWithProviders(executor, timeout, providers...)
	manager.legacyEnabled = enabled
	return manager
}

func NewWithProviders(executor platform.Executor, timeout time.Duration, providers ...HierarchyProvider) *Manager {
	base := append([]HierarchyProvider(nil), providers...)
	return &Manager{executor: executor, timeout: timeout, providers: append([]HierarchyProvider(nil), base...), baseProviders: base, shadowSlot: make(chan struct{}, 1), novaMode: "system", novaAddress: "127.0.0.1:9009", idleTimeout: 3 * time.Hour, metrics: HierarchyDiagnostics{Providers: map[string]ProviderMetrics{}}}
}

type ProviderMetrics struct {
	Attempts            uint64          `json:"attempts"`
	Successes           uint64          `json:"successes"`
	Failures            uint64          `json:"failures"`
	EmptyTrees          uint64          `json:"emptyTrees"`
	ConsecutiveFailures uint64          `json:"consecutiveFailures"`
	LastDurationMillis  int64           `json:"lastDurationMillis"`
	TotalDurationMillis int64           `json:"totalDurationMillis"`
	LastBytes           int             `json:"lastBytes"`
	LastNodes           int             `json:"lastNodes"`
	LastError           string          `json:"lastError,omitempty"`
	LastCapturedAt      time.Time       `json:"lastCapturedAt,omitempty"`
	FingerprintSamples  uint64          `json:"fingerprintSamples"`
	FingerprintChanges  uint64          `json:"fingerprintChanges"`
	LastFingerprint     string          `json:"lastFingerprint,omitempty"`
	DurationBuckets     DurationBuckets `json:"durationBuckets"`
}

type DurationBuckets struct {
	UpTo100Millis  uint64 `json:"le100ms"`
	UpTo250Millis  uint64 `json:"le250ms"`
	UpTo500Millis  uint64 `json:"le500ms"`
	UpTo1000Millis uint64 `json:"le1000ms"`
	UpTo3000Millis uint64 `json:"le3000ms"`
	Over3000Millis uint64 `json:"gt3000ms"`
}

type HierarchyDiagnostics struct {
	LastSource                              string                     `json:"lastSource,omitempty"`
	Fallbacks                               uint64                     `json:"fallbacks"`
	ShadowComparisons                       uint64                     `json:"shadowComparisons"`
	ShadowExactMatches                      uint64                     `json:"shadowExactMatches"`
	ShadowStructureMatches                  uint64                     `json:"shadowStructureMatches"`
	ShadowFingerprintMatches                uint64                     `json:"shadowFingerprintMatches"`
	ShadowSkippedBusy                       uint64                     `json:"shadowSkippedBusy"`
	LastShadowNodeDelta                     int                        `json:"lastShadowNodeDelta"`
	LastShadowByteDelta                     int                        `json:"lastShadowByteDelta"`
	LastShadowStructureMatch                bool                       `json:"lastShadowStructureMatch"`
	LastShadowFingerprintMatch              bool                       `json:"lastShadowFingerprintMatch"`
	LastShadowAttributeMatches              map[string]bool            `json:"lastShadowAttributeMatches,omitempty"`
	LastShadowMatchedNodes                  int                        `json:"lastShadowMatchedNodes"`
	LastShadowUnmatchedPrimaryNodes         int                        `json:"lastShadowUnmatchedPrimaryNodes"`
	LastShadowUnmatchedShadowNodes          int                        `json:"lastShadowUnmatchedShadowNodes"`
	LastShadowAttributeMismatches           map[string]int             `json:"lastShadowAttributeMismatches,omitempty"`
	LastShadowPackageNodeDeltas             map[string]int             `json:"lastShadowPackageNodeDeltas,omitempty"`
	LastShadowUnmatchedPrimaryPackageCounts map[string]int             `json:"lastShadowUnmatchedPrimaryPackageCounts,omitempty"`
	LastShadowUnmatchedShadowPackageCounts  map[string]int             `json:"lastShadowUnmatchedShadowPackageCounts,omitempty"`
	Providers                               map[string]ProviderMetrics `json:"providers"`
}

func (m *Manager) ConfigureNova(mode, address string) error {
	if mode != "system" && mode != "shadow" && mode != "nova" {
		return fmt.Errorf("hierarchy provider must be system, shadow, or nova")
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil || port == "" {
		return fmt.Errorf("invalid nova UiAutomator address %q", address)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("nova UiAutomator address must use a loopback IP")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running {
		return fmt.Errorf("cannot change hierarchy provider while UiAutomator is running")
	}
	m.novaMode = mode
	m.novaAddress = address
	return nil
}

func (m *Manager) hierarchyDiagnostics() HierarchyDiagnostics {
	m.metricsMu.RLock()
	defer m.metricsMu.RUnlock()
	result := m.metrics
	result.Providers = make(map[string]ProviderMetrics, len(m.metrics.Providers))
	for name, value := range m.metrics.Providers {
		result.Providers[name] = value
	}
	result.LastShadowAttributeMatches = cloneBoolMap(m.metrics.LastShadowAttributeMatches)
	result.LastShadowAttributeMismatches = cloneIntMap(m.metrics.LastShadowAttributeMismatches)
	result.LastShadowPackageNodeDeltas = cloneIntMap(m.metrics.LastShadowPackageNodeDeltas)
	result.LastShadowUnmatchedPrimaryPackageCounts = cloneIntMap(m.metrics.LastShadowUnmatchedPrimaryPackageCounts)
	result.LastShadowUnmatchedShadowPackageCounts = cloneIntMap(m.metrics.LastShadowUnmatchedShadowPackageCounts)
	return result
}

// Diagnostics is intentionally returned as any so the HTTP layer can expose
// observability without importing automation's internal metric types.
func (m *Manager) Diagnostics() any { return m.hierarchyDiagnostics() }
func (m *Manager) Screenshot(ctx context.Context) ([]byte, error) {
	c, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()
	data, err := m.executor.RunBytes(c, "screencap", "-p")
	if err != nil {
		return nil, err
	}
	if len(data) < 8 {
		return nil, fmt.Errorf("empty screenshot")
	}
	return data, nil
}
func (m *Manager) Hierarchy(ctx context.Context) (string, error) {
	m.captureMu.Lock()
	defer m.captureMu.Unlock()
	if err := m.waitForShadow(ctx); err != nil {
		return "", err
	}
	m.resetIdleTimer()
	m.hierarchyMu.Lock()
	providers := append([]HierarchyProvider(nil), m.providers...)
	m.hierarchyMu.Unlock()
	var failures []error
	for index, provider := range providers {
		started := time.Now()
		c, cancel := context.WithTimeout(ctx, m.timeout)
		readyErr := provider.Ready(c)
		var value Snapshot
		var err error
		if readyErr == nil {
			value, err = provider.Hierarchy(c)
		} else {
			err = readyErr
		}
		cancel()
		duration := time.Since(started)
		m.observe(provider.Name(), value, duration, err, true)
		if err != nil && provider.Name() == "nova-provider" && strings.Contains(err.Error(), "without nodes") {
			// On some Android 15/16 builds the first accessibility query after
			// instrumentation startup only primes the active-window cache. Retry on
			// the same owner before tearing it down; a separate system dump cannot
			// run until this instrumentation is released.
			select {
			case <-ctx.Done():
				err = ctx.Err()
			case <-time.After(100 * time.Millisecond):
				retryStarted := time.Now()
				retryContext, retryCancel := context.WithTimeout(ctx, m.timeout)
				if readyErr = provider.Ready(retryContext); readyErr == nil {
					value, err = provider.Hierarchy(retryContext)
				} else {
					err = readyErr
				}
				retryCancel()
				m.observe(provider.Name(), value, time.Since(retryStarted), err, true)
			}
		}
		if err == nil {
			if index > 0 {
				m.metricsMu.Lock()
				m.metrics.Fallbacks++
				m.metricsMu.Unlock()
			}
			m.startShadow(value)
			return value.XML, nil
		}
		failures = append(failures, fmt.Errorf("%s: %w", provider.Name(), err))
		// A live UiAutomation instrumentation and the platform `uiautomator dump`
		// command cannot own the accessibility bridge at the same time. Release a
		// failed Nova primary before attempting the system provider, otherwise the
		// fallback is predictably killed and is not a real fallback at all.
		if provider.Name() == "nova-provider" && index+1 < len(providers) {
			fallbackContext, fallbackCancel := context.WithTimeout(ctx, m.timeout)
			stopErr := m.stopNovaForFallback(fallbackContext)
			fallbackCancel()
			if stopErr != nil {
				failures = append(failures, fmt.Errorf("release nova provider for fallback: %w", stopErr))
				break
			}
		}
	}
	if len(failures) == 0 {
		return "", errors.New("no hierarchy providers configured")
	}
	return "", fmt.Errorf("hierarchy collection failed: %w", errors.Join(failures...))
}

func (m *Manager) stopNovaForFallback(ctx context.Context) error {
	m.mu.Lock()
	active := m.novaMode == "nova" && m.running && m.owned && m.ownedPackage == novaHostPackage
	m.mu.Unlock()
	if !active {
		return nil
	}
	return m.Stop(ctx)
}

func (m *Manager) waitForShadow(ctx context.Context) error {
	m.mu.Lock()
	shadowMode := m.novaMode == "shadow" && m.running
	m.mu.Unlock()
	if !shadowMode {
		return nil
	}
	select {
	case m.shadowSlot <- struct{}{}:
		<-m.shadowSlot
		return nil
	case <-ctx.Done():
		return fmt.Errorf("wait for previous hierarchy shadow: %w", ctx.Err())
	}
}

func (m *Manager) observe(name string, value Snapshot, duration time.Duration, err error, selected bool) {
	m.metricsMu.Lock()
	defer m.metricsMu.Unlock()
	metric := m.metrics.Providers[name]
	metric.Attempts++
	metric.LastDurationMillis = duration.Milliseconds()
	metric.TotalDurationMillis += duration.Milliseconds()
	switch millis := duration.Milliseconds(); {
	case millis <= 100:
		metric.DurationBuckets.UpTo100Millis++
	case millis <= 250:
		metric.DurationBuckets.UpTo250Millis++
	case millis <= 500:
		metric.DurationBuckets.UpTo500Millis++
	case millis <= 1000:
		metric.DurationBuckets.UpTo1000Millis++
	case millis <= 3000:
		metric.DurationBuckets.UpTo3000Millis++
	default:
		metric.DurationBuckets.Over3000Millis++
	}
	if err != nil {
		metric.Failures++
		metric.ConsecutiveFailures++
		metric.LastError = err.Error()
		if strings.Contains(err.Error(), "empty hierarchy") || strings.Contains(err.Error(), "without nodes") {
			metric.EmptyTrees++
		}
	} else {
		if value.Fingerprint == "" {
			value = ensureSnapshotFingerprints(value)
		}
		metric.Successes++
		metric.ConsecutiveFailures = 0
		metric.LastError = ""
		metric.LastBytes = value.Bytes
		metric.LastNodes = value.Nodes
		metric.LastCapturedAt = value.CapturedAt
		metric.FingerprintSamples++
		if metric.LastFingerprint != "" && metric.LastFingerprint != value.Fingerprint {
			metric.FingerprintChanges++
		}
		metric.LastFingerprint = value.Fingerprint
		if selected {
			m.metrics.LastSource = value.Source
		}
	}
	m.metrics.Providers[name] = metric
}

func (m *Manager) startShadow(primary Snapshot) {
	provider := m.shadowProvider
	if provider == nil {
		return
	}
	select {
	case m.shadowSlot <- struct{}{}:
	default:
		m.metricsMu.Lock()
		m.metrics.ShadowSkippedBusy++
		m.metricsMu.Unlock()
		return
	}
	go func() {
		defer func() { <-m.shadowSlot }()
		ctx, cancel := context.WithTimeout(context.Background(), m.timeout)
		defer cancel()
		started := time.Now()
		err := provider.Ready(ctx)
		var value Snapshot
		if err == nil {
			value, err = provider.Hierarchy(ctx)
		}
		m.observe(provider.Name(), value, time.Since(started), err, false)
		if err != nil {
			return
		}
		primary = ensureSnapshotFingerprints(primary)
		value = ensureSnapshotFingerprints(value)
		m.metricsMu.Lock()
		m.metrics.ShadowComparisons++
		if primary.XML == value.XML {
			m.metrics.ShadowExactMatches++
		}
		m.metrics.LastShadowNodeDelta = value.Nodes - primary.Nodes
		m.metrics.LastShadowByteDelta = value.Bytes - primary.Bytes
		m.metrics.LastShadowStructureMatch = primary.StructureFingerprint == value.StructureFingerprint
		m.metrics.LastShadowFingerprintMatch = primary.Fingerprint == value.Fingerprint
		if m.metrics.LastShadowStructureMatch {
			m.metrics.ShadowStructureMatches++
		}
		if m.metrics.LastShadowFingerprintMatch {
			m.metrics.ShadowFingerprintMatches++
		}
		matches := make(map[string]bool, len(hierarchyAttributes))
		for _, name := range hierarchyAttributes {
			matches[name] = primary.AttributeFingerprints[name] == value.AttributeFingerprints[name]
		}
		m.metrics.LastShadowAttributeMatches = matches
		if difference, differenceErr := compareHierarchyDocuments(primary.XML, value.XML); differenceErr == nil {
			m.metrics.LastShadowMatchedNodes = difference.MatchedNodes
			m.metrics.LastShadowUnmatchedPrimaryNodes = difference.UnmatchedPrimaryNodes
			m.metrics.LastShadowUnmatchedShadowNodes = difference.UnmatchedShadowNodes
			m.metrics.LastShadowAttributeMismatches = difference.AttributeMismatchCounts
			m.metrics.LastShadowPackageNodeDeltas = difference.PackageNodeDeltas
			m.metrics.LastShadowUnmatchedPrimaryPackageCounts = difference.UnmatchedPrimaryPackageCounts
			m.metrics.LastShadowUnmatchedShadowPackageCounts = difference.UnmatchedShadowPackageCounts
		}
		m.metricsMu.Unlock()
	}()
}

func ensureSnapshotFingerprints(value Snapshot) Snapshot {
	if value.StructureFingerprint != "" || value.XML == "" {
		return value
	}
	nodes, structure, attributes, fingerprint, err := analyzeHierarchy(value.XML)
	if err != nil {
		return value
	}
	value.Nodes = nodes
	value.Bytes = len(value.XML)
	value.StructureFingerprint = structure
	value.AttributeFingerprints = attributes
	value.Fingerprint = fingerprint
	return value
}

func cloneBoolMap(source map[string]bool) map[string]bool {
	if source == nil {
		return nil
	}
	result := make(map[string]bool, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func cloneIntMap(source map[string]int) map[string]int {
	if source == nil {
		return nil
	}
	result := make(map[string]int, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}
func (m *Manager) HierarchyWithScreenshot(ctx context.Context) (map[string]any, error) {
	xml, err := m.Hierarchy(ctx)
	if err != nil {
		return nil, err
	}
	png, err := m.Screenshot(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]any{"hierarchy": xml, "screenshot": "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)}, nil
}
func (m *Manager) Running() bool {
	m.mu.Lock()
	legacyEnabled := m.legacyEnabled
	running := m.running
	m.mu.Unlock()
	if legacyEnabled {
		client := http.Client{Timeout: 500 * time.Millisecond}
		if response, err := client.Get("http://127.0.0.1:9008/ping"); err == nil {
			_ = response.Body.Close()
			if response.StatusCode/100 == 2 {
				return true
			}
		}
	}
	return running
}
func (m *Manager) Start() error {
	m.mu.Lock()
	mode := m.novaMode
	legacyEnabled := m.legacyEnabled
	m.mu.Unlock()
	if mode == "shadow" {
		return m.startNovaShadow()
	}
	if mode == "nova" {
		return m.startNova()
	}
	if !legacyEnabled {
		return errors.New("legacy UiAutomator is disabled; select the nova provider or pass --legacy-uiautomator")
	}
	if m.Running() {
		return nil
	}
	checkContext, checkCancel := context.WithTimeout(context.Background(), m.timeout)
	defer checkCancel()
	packagePath, err := m.executor.Run(checkContext, "pm", "path", "com.github.uiautomator.test")
	if err != nil || !strings.Contains(packagePath, "package:") {
		return fmt.Errorf("uiautomator server package is not installed")
	}
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.generation++
	generation := m.generation
	m.commandContext = cancel
	m.running = true
	m.owned = true
	m.ownedPackage = "com.github.uiautomator"
	m.mu.Unlock()
	m.resetIdleTimer()
	go func() {
		_, _ = m.executor.Run(ctx, "am", "instrument", "-w", "-r", "-e", "debug", "false", "-e", "class", "com.github.uiautomator.stub.Stub", "com.github.uiautomator.test/androidx.test.runner.AndroidJUnitRunner")
		m.instrumentationExited(generation)
	}()
	return nil
}

func (m *Manager) startNovaShadow() error {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return nil
	}
	address, idleTimeout := m.novaAddress, m.idleTimeout
	m.mu.Unlock()
	checkContext, checkCancel := context.WithTimeout(context.Background(), m.timeout)
	defer checkCancel()
	if err := checkNovaPackages(checkContext, m.executor); err != nil {
		return err
	}
	provider := &transientNovaProvider{executor: m.executor, timeout: m.timeout, address: address, idleTimeout: idleTimeout}
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return nil
	}
	m.generation++
	m.running = true
	m.owned = true
	m.ownedPackage = novaHostPackage
	m.mu.Unlock()
	m.hierarchyMu.Lock()
	m.shadowProvider = provider
	m.hierarchyMu.Unlock()
	m.resetIdleTimer()
	return nil
}

func (m *Manager) startNova() error {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return nil
	}
	mode, address, idleTimeout := m.novaMode, m.novaAddress, m.idleTimeout
	m.mu.Unlock()
	checkContext, checkCancel := context.WithTimeout(context.Background(), m.timeout)
	defer checkCancel()
	if err := checkNovaPackages(checkContext, m.executor); err != nil {
		return err
	}
	token, err := generateNovaToken()
	if err != nil {
		return err
	}
	host, port, _ := net.SplitHostPort(address)
	baseURL := "http://" + net.JoinHostPort(host, port)
	provider := NewNovaProvider(baseURL+"/v1/hierarchy", baseURL+"/health", token, &http.Client{Timeout: m.timeout})
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.generation++
	generation := m.generation
	m.commandContext = cancel
	m.running = true
	m.owned = true
	// Instrumentation code is supplied by the test APK but executes inside the
	// target host process, so lifecycle cleanup must stop the host package.
	m.ownedPackage = novaHostPackage
	m.mu.Unlock()
	m.resetIdleTimer()
	go func() {
		_, _ = m.executor.Run(ctx, "am", "instrument", "-w", "-r", "-e", "token", token, "-e", "port", port, "-e", "idleTimeoutMillis", fmt.Sprint(idleTimeout.Milliseconds()), novaInstrumentation)
		m.instrumentationExited(generation)
	}()
	deadline := time.Now().Add(m.timeout)
	var readyErr error
	for time.Now().Before(deadline) {
		readyContext, readyCancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		readyErr = provider.Ready(readyContext)
		readyCancel()
		if readyErr == nil {
			m.hierarchyMu.Lock()
			if mode == "nova" {
				m.providers = append([]HierarchyProvider{provider}, m.baseProviders...)
			} else {
				m.shadowProvider = provider
			}
			m.hierarchyMu.Unlock()
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	_ = m.Stop(context.Background())
	return fmt.Errorf("nova UiAutomator did not become ready: %w", readyErr)
}

func checkNovaPackages(ctx context.Context, executor platform.Executor) error {
	for _, packageName := range []string{novaHostPackage, novaTestPackage} {
		packagePath, err := executor.Run(ctx, "pm", "path", packageName)
		if err != nil || !strings.Contains(packagePath, "package:") {
			return fmt.Errorf("nova UiAutomator package %s is not installed", packageName)
		}
	}
	return nil
}

func generateNovaToken() (string, error) {
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generate UiAutomator token: %w", err)
	}
	return hex.EncodeToString(random), nil
}

func (m *Manager) instrumentationExited(generation uint64) {
	m.mu.Lock()
	if m.generation != generation {
		m.mu.Unlock()
		return
	}
	m.running = false
	m.owned = false
	m.ownedPackage = ""
	m.commandContext = nil
	if m.idleTimer != nil {
		m.idleTimer.Stop()
		m.idleTimer = nil
	}
	m.mu.Unlock()
	m.resetProviderSelection()
}

func (m *Manager) Stop(ctx context.Context) error {
	m.mu.Lock()
	cancel := m.commandContext
	owned := m.owned
	ownedPackage := m.ownedPackage
	if owned {
		m.generation++
	}
	m.commandContext = nil
	m.running = false
	m.owned = false
	m.ownedPackage = ""
	if m.idleTimer != nil {
		m.idleTimer.Stop()
		m.idleTimer = nil
	}
	m.mu.Unlock()
	if !owned {
		return nil
	}
	if cancel != nil {
		cancel()
	}
	m.resetProviderSelection()
	c, done := context.WithTimeout(ctx, m.timeout)
	defer done()
	_, err := m.executor.Run(c, "am", "force-stop", ownedPackage)
	return err
}

func (m *Manager) resetProviderSelection() {
	m.hierarchyMu.Lock()
	m.providers = append([]HierarchyProvider(nil), m.baseProviders...)
	m.shadowProvider = nil
	m.hierarchyMu.Unlock()
}

func (m *Manager) SetCommandTimeout(value time.Duration) error {
	if value < time.Second || value > 24*time.Hour {
		return fmt.Errorf("command timeout must be between 1 second and 24 hours")
	}
	m.mu.Lock()
	m.idleTimeout = value
	running := m.running && m.owned
	m.mu.Unlock()
	if running {
		m.resetIdleTimer()
	}
	return nil
}

func (m *Manager) resetIdleTimer() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.running || !m.owned {
		return
	}
	if m.idleTimer != nil {
		m.idleTimer.Stop()
	}
	m.idleTimer = time.AfterFunc(m.idleTimeout, func() { _ = m.Stop(context.Background()) })
}
func (m *Manager) ResetCommandTimeout() { m.resetIdleTimer() }
