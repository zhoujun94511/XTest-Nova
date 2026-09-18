package automation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/zhoujun94511/xtest-nova/agent/internal/platform"
)

const maxHierarchyBytes = 16 << 20

// Snapshot is the provider-neutral hierarchy result used inside Nova. XML stays
// available during the first migration phase so existing HTTP and Runner
// contracts do not change.
type Snapshot struct {
	Source                string            `json:"source"`
	CapturedAt            time.Time         `json:"capturedAt"`
	XML                   string            `json:"-"`
	Bytes                 int               `json:"bytes"`
	Nodes                 int               `json:"nodes"`
	Fingerprint           string            `json:"-"`
	StructureFingerprint  string            `json:"-"`
	AttributeFingerprints map[string]string `json:"-"`
}

type hierarchyDifference struct {
	MatchedNodes                  int
	UnmatchedPrimaryNodes         int
	UnmatchedShadowNodes          int
	AttributeMismatchCounts       map[string]int
	PackageNodeDeltas             map[string]int
	UnmatchedPrimaryPackageCounts map[string]int
	UnmatchedShadowPackageCounts  map[string]int
}

var hierarchyAttributes = [...]string{
	"index", "text", "resource-id", "class", "package", "content-desc",
	"checkable", "checked", "clickable", "enabled", "focusable", "focused",
	"scrollable", "long-clickable", "password", "selected", "bounds",
}

var hierarchyAttributeIndexes = func() map[string]int {
	indexes := make(map[string]int, len(hierarchyAttributes))
	for index, name := range hierarchyAttributes {
		indexes[name] = index
	}
	return indexes
}()

type HierarchyProvider interface {
	Name() string
	Ready(context.Context) error
	Hierarchy(context.Context) (Snapshot, error)
}

type systemDumpProvider struct {
	executor platform.Executor
	path     string
}

func newSystemDumpProvider(executor platform.Executor) HierarchyProvider {
	return &systemDumpProvider{executor: executor, path: "/data/local/tmp/xtest-nova-window.xml"}
}

func (p *systemDumpProvider) Name() string                { return "system-dump" }
func (p *systemDumpProvider) Ready(context.Context) error { return nil }
func (p *systemDumpProvider) Hierarchy(ctx context.Context) (Snapshot, error) {
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		path := strings.TrimSuffix(p.path, ".xml") + "-" + strconv.Itoa(os.Getpid()) + "-" + strconv.FormatInt(time.Now().UnixNano(), 10) + ".xml"
		if _, err := p.executor.Run(ctx, "uiautomator", "dump", "--compressed", path); err != nil {
			// The platform command may create its output before its process or
			// transport reports a timeout. Always remove that partial attempt.
			_ = os.Remove(path)
			lastErr = err
			continue
		}
		data, err := os.ReadFile(path)
		_ = os.Remove(path)
		if err == nil {
			return providerSnapshot(p.Name(), data)
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return Snapshot{}, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	return Snapshot{}, fmt.Errorf("uiautomator dump did not create readable output: %w", lastErr)
}

type rpcProvider struct {
	name      string
	endpoint  string
	healthURL string
	token     string
	client    *http.Client
	legacy    bool
}

type transientNovaProvider struct {
	executor    platform.Executor
	timeout     time.Duration
	address     string
	idleTimeout time.Duration
}

func (p *transientNovaProvider) Name() string { return "nova-provider" }
func (p *transientNovaProvider) Ready(context.Context) error {
	return nil
}
func (p *transientNovaProvider) Hierarchy(ctx context.Context) (result Snapshot, resultErr error) {
	token, err := generateNovaToken()
	if err != nil {
		return Snapshot{}, err
	}
	host, port, err := net.SplitHostPort(p.address)
	if err != nil {
		return Snapshot{}, fmt.Errorf("split Nova UiAutomator address: %w", err)
	}
	baseURL := "http://" + net.JoinHostPort(host, port)
	provider := NewNovaProvider(baseURL+"/v1/hierarchy", baseURL+"/health", token, &http.Client{Timeout: p.timeout})
	instrumentContext, cancel := context.WithCancel(ctx)
	instrumentDone := make(chan struct{})
	go func() {
		defer close(instrumentDone)
		_, _ = p.executor.Run(instrumentContext, "am", "instrument", "-w", "-r", "-e", "token", token, "-e", "port", port, "-e", "idleTimeoutMillis", fmt.Sprint(p.idleTimeout.Milliseconds()), novaInstrumentation)
	}()
	defer func() {
		cancel()
		cleanupTimeout := p.timeout
		if cleanupTimeout > 5*time.Second {
			cleanupTimeout = 5 * time.Second
		}
		cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), cleanupTimeout)
		defer cleanupCancel()
		if _, stopErr := p.executor.Run(cleanupContext, "am", "force-stop", novaHostPackage); resultErr == nil && stopErr != nil {
			resultErr = fmt.Errorf("stop transient Nova UiAutomator: %w", stopErr)
		}
		select {
		case <-instrumentDone:
		case <-cleanupContext.Done():
			if resultErr == nil {
				resultErr = fmt.Errorf("wait for transient Nova UiAutomator exit: %w", cleanupContext.Err())
			}
		}
	}()
	for {
		readyContext, readyCancel := context.WithTimeout(ctx, 500*time.Millisecond)
		readyErr := provider.Ready(readyContext)
		readyCancel()
		if readyErr == nil {
			return provider.Hierarchy(ctx)
		}
		select {
		case <-ctx.Done():
			return Snapshot{}, fmt.Errorf("transient Nova UiAutomator did not become ready: %w", ctx.Err())
		case <-instrumentDone:
			return Snapshot{}, fmt.Errorf("transient Nova UiAutomator exited before becoming ready")
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func newLegacyRPCProvider(client *http.Client) HierarchyProvider {
	return &rpcProvider{
		name: "legacy-9008", endpoint: "http://127.0.0.1:9008/jsonrpc/0",
		healthURL: "http://127.0.0.1:9008/ping", client: client, legacy: true,
	}
}

// NewNovaProvider creates the client for Nova's dedicated loopback-only
// Instrumentation service. It is intentionally opt-in until shadow qualification
// is complete.
func NewNovaProvider(endpoint, healthURL, token string, client *http.Client) HierarchyProvider {
	return &rpcProvider{name: "nova-provider", endpoint: endpoint, healthURL: healthURL, token: token, client: client}
}

func (p *rpcProvider) Name() string { return p.name }
func (p *rpcProvider) Ready(ctx context.Context) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, p.healthURL, nil)
	if err != nil {
		return err
	}
	p.authorize(request)
	response, err := p.httpClient().Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode/100 != 2 {
		return fmt.Errorf("%s health returned %s", p.name, response.Status)
	}
	return nil
}

func (p *rpcProvider) Hierarchy(ctx context.Context) (Snapshot, error) {
	method := http.MethodGet
	var body io.Reader
	if p.legacy {
		method = http.MethodPost
		body = strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"dumpWindowHierarchy","params":[false]}`)
	}
	request, err := http.NewRequestWithContext(ctx, method, p.endpoint, body)
	if err != nil {
		return Snapshot{}, err
	}
	request.Header.Set("Accept", "application/json, application/xml")
	if p.legacy {
		request.Header.Set("Content-Type", "application/json")
	}
	p.authorize(request)
	response, err := p.httpClient().Do(request)
	if err != nil {
		return Snapshot{}, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode/100 != 2 {
		return Snapshot{}, fmt.Errorf("%s returned %s", p.name, response.Status)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxHierarchyBytes+1))
	if err != nil {
		return Snapshot{}, err
	}
	if len(data) > maxHierarchyBytes {
		return Snapshot{}, fmt.Errorf("%s hierarchy exceeds %d bytes", p.name, maxHierarchyBytes)
	}
	if bytes.HasPrefix(bytes.TrimSpace(data), []byte("<")) {
		return providerSnapshot(p.name, data)
	}
	var result struct {
		Result    string `json:"result"`
		Hierarchy string `json:"hierarchy"`
	}
	if err = json.Unmarshal(data, &result); err != nil {
		return Snapshot{}, err
	}
	document := result.Hierarchy
	if document == "" {
		document = result.Result
	}
	return providerSnapshot(p.name, []byte(document))
}

func (p *rpcProvider) authorize(request *http.Request) {
	if p.token != "" {
		request.Header.Set("Authorization", "Bearer "+p.token)
	}
}

func (p *rpcProvider) httpClient() *http.Client {
	if p.client != nil {
		return p.client
	}
	return &http.Client{Timeout: 15 * time.Second}
}

func snapshot(source string, data []byte) (Snapshot, error) {
	document := strings.TrimSpace(string(data))
	if document == "" {
		return Snapshot{}, fmt.Errorf("%s returned an empty hierarchy", source)
	}
	nodes, structureFingerprint, attributeFingerprints, fingerprint, err := analyzeHierarchy(document)
	if err != nil {
		return Snapshot{}, fmt.Errorf("%s returned invalid XML: %w", source, err)
	}
	if nodes == 0 {
		return Snapshot{}, fmt.Errorf("%s returned a hierarchy without nodes", source)
	}
	return Snapshot{
		Source: source, CapturedAt: time.Now().UTC(), XML: document, Bytes: len(document), Nodes: nodes,
		Fingerprint: fingerprint, StructureFingerprint: structureFingerprint, AttributeFingerprints: attributeFingerprints,
	}, nil
}

// providerSnapshot keeps the request path lightweight. Canonical structure and
// per-attribute fingerprints are only needed when comparing shadow providers;
// computing all of them synchronously for every production capture creates
// substantial allocation and GC pressure on large WebView hierarchies.
func providerSnapshot(source string, data []byte) (Snapshot, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return Snapshot{}, fmt.Errorf("%s returned an empty hierarchy", source)
	}
	document := string(trimmed)
	nodes := quickHierarchyNodeCount(trimmed)
	if nodes == 0 {
		return Snapshot{}, fmt.Errorf("%s returned a hierarchy without nodes", source)
	}
	if !bytes.Contains(trimmed, []byte("<hierarchy")) || !bytes.HasSuffix(trimmed, []byte("</hierarchy>")) {
		return Snapshot{}, fmt.Errorf("%s returned an incomplete hierarchy", source)
	}
	sum := sha256.Sum256(trimmed)
	return Snapshot{
		Source: source, CapturedAt: time.Now().UTC(), XML: document, Bytes: len(document), Nodes: nodes,
		Fingerprint: fmt.Sprintf("%x", sum),
	}, nil
}

func quickHierarchyNodeCount(document []byte) int {
	return bytes.Count(document, []byte("<node ")) + bytes.Count(document, []byte("<node>"))
}

func analyzeHierarchy(document string) (int, string, map[string]string, string, error) {
	decoder := xml.NewDecoder(strings.NewReader(document))
	var structureBuffer bytes.Buffer
	structureBuffer.Grow(max(256, len(document)/16))
	var attributeBuffers [len(hierarchyAttributes)]bytes.Buffer
	for index := range attributeBuffers {
		attributeBuffers[index].Grow(max(256, len(document)/(len(hierarchyAttributes)*4)))
	}
	nodes, depth := 0, 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, "", nil, "", err
		}
		switch value := token.(type) {
		case xml.StartElement:
			if value.Name.Local != "node" {
				continue
			}
			nodes++
			writeCanonicalPart(&structureBuffer, "(", strconv.Itoa(depth))
			var attributeValues [len(hierarchyAttributes)]string
			var attributePresent [len(hierarchyAttributes)]bool
			for _, attribute := range value.Attr {
				if index, ok := hierarchyAttributeIndexes[attribute.Name.Local]; ok {
					attributeValues[index] = attribute.Value
					attributePresent[index] = true
				}
			}
			for index := range hierarchyAttributes {
				present := "0"
				if attributePresent[index] {
					present = "1"
				}
				writeCanonicalPart(&attributeBuffers[index], present, attributeValues[index])
			}
			depth++
		case xml.EndElement:
			if value.Name.Local == "node" {
				depth--
				writeCanonicalPart(&structureBuffer, ")")
			}
		}
	}
	structureFingerprint := fingerprintBuffer(&structureBuffer)
	attributeFingerprints := make(map[string]string, len(hierarchyAttributes))
	var combinedBuffer bytes.Buffer
	combinedBuffer.Grow(len(hierarchyAttributes) * (sha256.Size*2 + 32))
	writeCanonicalPart(&combinedBuffer, structureFingerprint)
	for index, name := range hierarchyAttributes {
		value := fingerprintBuffer(&attributeBuffers[index])
		attributeFingerprints[name] = value
		writeCanonicalPart(&combinedBuffer, name, value)
	}
	return nodes, structureFingerprint, attributeFingerprints, fingerprintBuffer(&combinedBuffer), nil
}

func compareHierarchyDocuments(primary, shadow string) (hierarchyDifference, error) {
	primaryNodes, err := hierarchyNodeAttributes(primary)
	if err != nil {
		return hierarchyDifference{}, err
	}
	shadowNodes, err := hierarchyNodeAttributes(shadow)
	if err != nil {
		return hierarchyDifference{}, err
	}
	shadowIndexes := make(map[string][]int, len(shadowNodes))
	for index, attributes := range shadowNodes {
		identity := hierarchyNodeIdentity(attributes)
		shadowIndexes[identity] = append(shadowIndexes[identity], index)
	}
	usedByIdentity := make(map[string]int, len(shadowIndexes))
	matchedShadow := make([]bool, len(shadowNodes))
	mismatches := make(map[string]int, len(hierarchyAttributes))
	unmatchedPrimaryPackages := make(map[string]int)
	matched := 0
	for _, attributes := range primaryNodes {
		identity := hierarchyNodeIdentity(attributes)
		used := usedByIdentity[identity]
		indexes := shadowIndexes[identity]
		if used >= len(indexes) {
			incrementPackageCount(unmatchedPrimaryPackages, attributes)
			continue
		}
		shadowIndex := indexes[used]
		other := shadowNodes[shadowIndex]
		matchedShadow[shadowIndex] = true
		usedByIdentity[identity] = used + 1
		matched++
		for _, name := range hierarchyAttributes {
			if attributes[name] != other[name] {
				mismatches[name]++
			}
		}
	}
	packageDeltas := hierarchyPackageCounts(shadowNodes)
	for name, count := range hierarchyPackageCounts(primaryNodes) {
		packageDeltas[name] -= count
	}
	for name, delta := range packageDeltas {
		if delta == 0 {
			delete(packageDeltas, name)
		}
	}
	unmatchedShadowPackages := make(map[string]int)
	for index, attributes := range shadowNodes {
		if !matchedShadow[index] {
			incrementPackageCount(unmatchedShadowPackages, attributes)
		}
	}
	return hierarchyDifference{
		MatchedNodes: matched, UnmatchedPrimaryNodes: len(primaryNodes) - matched, UnmatchedShadowNodes: len(shadowNodes) - matched,
		AttributeMismatchCounts: mismatches, PackageNodeDeltas: packageDeltas,
		UnmatchedPrimaryPackageCounts: unmatchedPrimaryPackages, UnmatchedShadowPackageCounts: unmatchedShadowPackages,
	}, nil
}

func hierarchyNodeAttributes(document string) ([]map[string]string, error) {
	decoder := xml.NewDecoder(strings.NewReader(document))
	var nodes []map[string]string
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return nodes, nil
		}
		if err != nil {
			return nil, err
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "node" {
			continue
		}
		attributes := make(map[string]string, len(start.Attr))
		for _, attribute := range start.Attr {
			attributes[attribute.Name.Local] = attribute.Value
		}
		nodes = append(nodes, attributes)
	}
}

func hierarchyNodeIdentity(attributes map[string]string) string {
	var identity bytes.Buffer
	writeCanonicalPart(&identity, attributes["class"])
	if resourceID := attributes["resource-id"]; resourceID != "" {
		writeCanonicalPart(&identity, "resource-id", resourceID)
	} else if text, description := attributes["text"], attributes["content-desc"]; text != "" || description != "" {
		writeCanonicalPart(&identity, "semantic", text, description)
	} else {
		writeCanonicalPart(&identity, "bounds", attributes["bounds"])
	}
	return fingerprintBuffer(&identity)
}

func hierarchyPackageCounts(nodes []map[string]string) map[string]int {
	counts := make(map[string]int)
	for _, attributes := range nodes {
		incrementPackageCount(counts, attributes)
	}
	return counts
}

func incrementPackageCount(counts map[string]int, attributes map[string]string) {
	name := attributes["package"]
	if name == "" {
		name = "<empty>"
	}
	counts[name]++
}

func writeCanonicalPart(target *bytes.Buffer, values ...string) {
	var scratch [20]byte
	for _, value := range values {
		target.Write(strconv.AppendInt(scratch[:0], int64(len(value)), 10))
		target.WriteByte(':')
		target.WriteString(value)
	}
}

func fingerprintBuffer(value *bytes.Buffer) string {
	sum := sha256.Sum256(value.Bytes())
	return fmt.Sprintf("%x", sum)
}
