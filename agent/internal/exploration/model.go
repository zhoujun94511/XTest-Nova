package exploration

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

var boundsPattern = regexp.MustCompile(`^\[(-?\d+),(-?\d+)]\[(-?\d+),(-?\d+)]$`)
var digitsPattern = regexp.MustCompile(`\d+`)

var defaultDeniedText = []string{
	"uninstall", "delete", "remove", "factory reset", "clear data", "format",
	"install", "download", "get app", "play store", "app store",
	"卸载", "删除", "移除", "恢复出厂", "清除数据", "格式化",
	"安装", "下载", "获取应用", "应用商店",
}
var defaultRecoveryText = []string{"取消", "稍后", "以后再说", "跳过", "继续等待", "cancel", "not now", "later", "skip", "wait"}
var sensitiveInputText = []string{
	"password", "passwd", "pin", "otp", "verification code", "security code", "cvv", "card number", "bank account", "payment",
	"密码", "口令", "验证码", "校验码", "动态码", "支付", "银行卡", "卡号", "账户", "账号",
}

type inputProbe struct{ Kind, Text string }

type InputStrategy struct {
	CasesPerField int `json:"casesPerField,omitempty"`
	MaxLength     int `json:"maxLength,omitempty"`
}

const defaultInputCasesPerField = 1
const maxInputCasesPerField = 18
const defaultInputMaxLength = 64

type Bounds struct {
	Left, Top, Right, Bottom int
}

func (b Bounds) Valid() bool {
	return b.Left >= 0 && b.Top >= 0 && b.Right > b.Left && b.Bottom > b.Top
}
func (b Bounds) Center() (int, int) { return b.Left + (b.Right-b.Left)/2, b.Top + (b.Bottom-b.Top)/2 }

type Rules struct {
	DenyText              []string `json:"denyText,omitempty"`
	AllowText             []string `json:"allowText,omitempty"`
	AllowResourcePrefixes []string `json:"allowResourcePrefixes,omitempty"`
	RecoveryText          []string `json:"recoveryText,omitempty"`
}

type Action struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	X           int    `json:"x"`
	Y           int    `json:"y"`
	EndX        int    `json:"endX,omitempty"`
	EndY        int    `json:"endY,omitempty"`
	Direction   string `json:"direction,omitempty"`
	Recovery    bool   `json:"recovery,omitempty"`
	Backtrack   bool   `json:"backtrack,omitempty"`
	SpecialKind string `json:"specialKind,omitempty"`
	Policy      string `json:"policy,omitempty"`
	InputKind   string `json:"inputKind,omitempty"`
	InputLength *int   `json:"inputLength,omitempty"`
	InputSeed   int64  `json:"inputSeed,omitempty"`
	Text        string `json:"text,omitempty"`
	Description string `json:"description,omitempty"`
	ResourceID  string `json:"resourceId,omitempty"`
	Class       string `json:"class,omitempty"`
	Bounds      Bounds `json:"bounds"`
}

type Analysis struct {
	ObservationID       string   `json:"observationId,omitempty"`
	Fingerprint         string   `json:"fingerprint"`
	SemanticFingerprint string   `json:"-"`
	Package             string   `json:"package"`
	Activity            string   `json:"activity,omitempty"`
	NodeCount           int      `json:"nodeCount"`
	Filtered            int      `json:"filtered"`
	InputFields         int      `json:"inputFields"`
	Actions             []Action `json:"actions"`
}

type xmlDocument struct {
	Nodes []xmlNode `xml:"node"`
}

type xmlNode struct {
	Text        string    `xml:"text,attr"`
	Description string    `xml:"content-desc,attr"`
	ResourceID  string    `xml:"resource-id,attr"`
	Class       string    `xml:"class,attr"`
	Package     string    `xml:"package,attr"`
	Bounds      string    `xml:"bounds,attr"`
	Clickable   string    `xml:"clickable,attr"`
	Enabled     string    `xml:"enabled,attr"`
	Visible     string    `xml:"visible-to-user,attr"`
	Password    string    `xml:"password,attr"`
	Focusable   string    `xml:"focusable,attr"`
	Focused     string    `xml:"focused,attr"`
	Scrollable  string    `xml:"scrollable,attr"`
	Children    []xmlNode `xml:"node"`
}

func Analyze(hierarchy, targetPackage string, rules Rules) (Analysis, error) {
	return AnalyzeWithInputStrategy(hierarchy, targetPackage, rules, 0, InputStrategy{CasesPerField: defaultInputCasesPerField, MaxLength: defaultInputMaxLength})
}

func AnalyzeWithInputStrategy(hierarchy, targetPackage string, rules Rules, seed int64, input InputStrategy) (Analysis, error) {
	if strings.TrimSpace(hierarchy) == "" {
		return Analysis{}, errors.New("hierarchy is empty")
	}
	var document xmlDocument
	if err := xml.Unmarshal([]byte(hierarchy), &document); err != nil {
		return Analysis{}, fmt.Errorf("parse hierarchy: %w", err)
	}
	if len(document.Nodes) == 0 {
		return Analysis{}, errors.New("hierarchy has no nodes")
	}
	analysis := Analysis{Package: targetPackage, Actions: []Action{}}
	signatures := make([]string, 0, 64)
	semanticSignatures := make([]string, 0, 64)
	for i := range document.Nodes {
		walkNode(&document.Nodes[i], targetPackage, rules, seed, input, &analysis, &signatures, &semanticSignatures)
	}
	analysis.Actions = filterMinorScrollTargets(analysis.Actions)
	sort.Strings(signatures)
	sort.Strings(semanticSignatures)
	sum := sha256.Sum256([]byte(strings.Join(signatures, "\n")))
	analysis.Fingerprint = hex.EncodeToString(sum[:16])
	semanticSum := sha256.Sum256([]byte(strings.Join(semanticSignatures, "\n")))
	analysis.SemanticFingerprint = hex.EncodeToString(semanticSum[:16])
	sort.Slice(analysis.Actions, func(i, j int) bool {
		left, right := analysis.Actions[i], analysis.Actions[j]
		if left.Bounds.Top != right.Bounds.Top {
			return left.Bounds.Top < right.Bounds.Top
		}
		if left.Bounds.Left != right.Bounds.Left {
			return left.Bounds.Left < right.Bounds.Left
		}
		return left.ID < right.ID
	})
	return analysis, nil
}

func walkNode(node *xmlNode, targetPackage string, rules Rules, seed int64, input InputStrategy, analysis *Analysis, signatures, semanticSignatures *[]string) string {
	return walkNodeContext(node, targetPackage, rules, seed, input, analysis, signatures, semanticSignatures, false)
}

func walkNodeContext(node *xmlNode, targetPackage string, rules Rules, seed int64, input InputStrategy, analysis *Analysis, signatures, semanticSignatures *[]string, parentAdContext bool) string {
	analysis.NodeCount++
	adContext := parentAdContext || isEmbeddedAdContainer(node.ResourceID, node.Description, node.Class)
	labels := make([]string, 0, 3)
	for i := range node.Children {
		if label := walkNodeContext(&node.Children[i], targetPackage, rules, seed, input, analysis, signatures, semanticSignatures, adContext); label != "" {
			labels = append(labels, label)
		}
	}
	label := firstNonEmpty(node.Text, node.Description, strings.Join(labels, " "))
	fingerprintLabel := label
	if strings.Contains(strings.ToLower(node.Class), "edittext") {
		fingerprintLabel = firstNonEmpty(node.Description, node.ResourceID, "<editable-value>")
	}
	bounds, boundsOK := parseBounds(node.Bounds)
	// System UI, launchers and transient notification windows are included by
	// the multi-window provider. They are not part of the target application's
	// state and must not create artificial graph nodes.
	if node.Package == "" || targetPackage == "" || node.Package == targetPackage {
		*signatures = append(*signatures, strings.Join([]string{
			node.Package, node.Class, node.ResourceID, stableText(fingerprintLabel), node.Bounds,
			node.Clickable, node.Scrollable,
		}, "|"))
		*semanticSignatures = append(*semanticSignatures, strings.Join([]string{
			node.Package, node.Class, node.ResourceID, stableText(fingerprintLabel), node.Clickable, node.Scrollable,
		}, "|"))
	}
	clickable := node.Clickable == "true" && !strings.EqualFold(node.Class, "android.webkit.WebView")
	editable := strings.Contains(strings.ToLower(node.Class), "edittext")
	actionable := clickable || editable || node.Scrollable == "true"
	if !actionable {
		return label
	}
	if node.Enabled == "false" || node.Visible == "false" || node.Password == "true" || !boundsOK {
		analysis.Filtered++
		return label
	}
	if node.Package != "" && targetPackage != "" && node.Package != targetPackage {
		analysis.Filtered++
		return label
	}
	if adContext && !isAdDismissControl(label, node.ResourceID) {
		analysis.Filtered++
		return label
	}
	if !allowed(label, node.Description, node.ResourceID, rules) {
		analysis.Filtered++
		return label
	}
	if editable && safeInputField(node, label) {
		analysis.InputFields++
		x, y := bounds.Center()
		fieldLabel := firstNonEmpty(node.Description, node.ResourceID, node.Class)
		for _, probe := range generateInputProbes(seed, input, inputFieldIdentity(node, fieldLabel, bounds)) {
			inputLength := utf8.RuneCountInString(probe.Text)
			analysis.Actions = append(analysis.Actions, Action{
				ID: inputActionID(probe.Kind, node, fieldLabel, bounds), Type: "input", InputKind: probe.Kind, InputLength: &inputLength, InputSeed: seed,
				X: x, Y: y, Text: probe.Text, Description: node.Description, ResourceID: node.ResourceID, Class: node.Class, Bounds: bounds,
			})
		}
	}
	if clickable && !editable {
		x, y := bounds.Center()
		analysis.Actions = append(analysis.Actions, Action{
			ID: actionID("tap", node, label), Type: "tap", X: x, Y: y, Text: node.Text,
			Description: node.Description, ResourceID: node.ResourceID, Class: node.Class, Bounds: bounds,
			Recovery: isRecoveryText(label, rules),
		})
	}
	if node.Scrollable == "true" && bounds.Bottom-bounds.Top >= 80 {
		x, _ := bounds.Center()
		analysis.Actions = append(analysis.Actions, Action{
			ID: actionID("swipe-forward", node, label), Type: "swipe", Direction: "forward",
			X: x, Y: bounds.Top + (bounds.Bottom-bounds.Top)*3/4,
			EndX: x, EndY: bounds.Top + (bounds.Bottom-bounds.Top)/4,
			Text: node.Text, Description: node.Description, ResourceID: node.ResourceID, Class: node.Class, Bounds: bounds,
		})
	}
	return label
}

func inputFieldIdentity(node *xmlNode, label string, bounds Bounds) string {
	return stableInputFieldIdentity(node.ResourceID, node.Description, node.Class, label, bounds)
}

func stableInputFieldIdentity(resourceID, description, className, label string, bounds Bounds) string {
	// Resource IDs and accessibility descriptions identify the logical field
	// across viewport movement. Coordinates are only a last resort for legacy
	// layouts that expose no stable accessibility anchor.
	anchor := firstNonEmpty(resourceID, description)
	if anchor != "" {
		return strings.Join([]string{resourceID, description, className, stableText(anchor)}, "|")
	}
	return strings.Join([]string{className, stableText(label), strconv.Itoa(bounds.Left), strconv.Itoa(bounds.Top)}, "|")
}

func generateInputProbes(seed int64, strategy InputStrategy, field string) []inputProbe {
	if strategy.CasesPerField <= 0 {
		strategy.CasesPerField = defaultInputCasesPerField
	}
	if strategy.MaxLength <= 0 {
		strategy.MaxLength = defaultInputMaxLength
	}
	ascii := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
	digits := []rune("0123456789")
	cjk := []rune("测试边界中文输入验证内容样例")
	symbols := []rune("!@#$%^&*()_+-=[]{}:,.?/\\|")
	emoji := []rune("😀🚀✨🧪✅🌟🌈🎉")
	value := func(kind string, alphabet []rune, length int) string {
		if length > strategy.MaxLength {
			length = strategy.MaxLength
		}
		return deterministicRunes(seed, field+"|"+kind, alphabet, length)
	}
	boundary := 32
	probes := []inputProbe{
		{Kind: "ascii_short", Text: value("ascii_short", ascii, 8)},
		{Kind: "empty", Text: ""},
		{Kind: "whitespace", Text: "   "},
		{Kind: "ascii_min", Text: value("ascii_min", ascii, 1)},
		{Kind: "ascii_boundary_minus_1", Text: value("ascii_boundary_minus_1", ascii, boundary-1)},
		{Kind: "ascii_boundary", Text: value("ascii_boundary", ascii, boundary)},
		{Kind: "ascii_boundary_plus_1", Text: value("ascii_boundary_plus_1", ascii, boundary+1)},
		{Kind: "ascii_max", Text: value("ascii_max", ascii, strategy.MaxLength)},
		{Kind: "cjk", Text: value("cjk", cjk, 8)},
		{Kind: "symbols", Text: value("symbols", symbols, 12)},
		{Kind: "emoji", Text: value("emoji", emoji, 4)},
		{Kind: "mixed", Text: value("mixed_ascii", ascii, 5) + value("mixed_cjk", cjk, 3) + value("mixed_symbol", symbols, 3) + value("mixed_emoji", emoji, 2)},
		{Kind: "numeric_zero", Text: "0"},
		{Kind: "numeric_negative", Text: "-" + value("numeric_negative", digits, 6)},
		{Kind: "numeric_decimal", Text: value("numeric_decimal_a", digits, 3) + "." + value("numeric_decimal_b", digits, 4)},
		{Kind: "email_valid", Text: strings.ToLower(value("email_valid", ascii, 8)) + "@example.test"},
		{Kind: "email_invalid", Text: strings.ToLower(value("email_invalid", ascii, 6)) + "..@"},
		{Kind: "leading_trailing_space", Text: "  " + value("trim", ascii, 8) + "  "},
	}
	for index := range probes {
		probes[index].Text = limitRunes(probes[index].Text, strategy.MaxLength)
	}
	if strategy.CasesPerField < len(probes) {
		probes = probes[:strategy.CasesPerField]
	}
	return probes
}

func limitRunes(value string, maximum int) string {
	runes := []rune(value)
	if len(runes) <= maximum {
		return value
	}
	return string(runes[:maximum])
}

func deterministicRunes(seed int64, key string, alphabet []rune, length int) string {
	if length <= 0 || len(alphabet) == 0 {
		return ""
	}
	result := make([]rune, length)
	var seedBytes [8]byte
	binary.LittleEndian.PutUint64(seedBytes[:], uint64(seed))
	for index := range result {
		sum := sha256.Sum256(append(append(append([]byte{}, seedBytes[:]...), key...), byte(index), byte(index>>8)))
		result[index] = alphabet[binary.LittleEndian.Uint32(sum[:4])%uint32(len(alphabet))]
	}
	return string(result)
}

func inputActionID(kind string, node *xmlNode, label string, bounds Bounds) string {
	identity := "input|" + kind + "|" + inputFieldIdentity(node, label, bounds)
	sum := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(sum[:8])
}

func safeInputField(node *xmlNode, label string) bool {
	if node.Password == "true" {
		return false
	}
	candidate := strings.ToLower(strings.Join([]string{label, node.Description, node.ResourceID}, " "))
	for _, denied := range sensitiveInputText {
		if strings.Contains(candidate, denied) {
			return false
		}
	}
	return true
}

func filterMinorScrollTargets(actions []Action) []Action {
	maxBottom := 0
	for _, action := range actions {
		if action.Bounds.Bottom > maxBottom {
			maxBottom = action.Bounds.Bottom
		}
	}
	if maxBottom == 0 {
		return actions
	}
	filtered := make([]Action, 0, len(actions))
	seen := map[string]bool{}
	for _, action := range actions {
		if action.Type == "swipe" && (action.Bounds.Bottom-action.Bounds.Top)*4 < maxBottom {
			continue
		}
		if seen[action.ID] {
			continue
		}
		seen[action.ID] = true
		filtered = append(filtered, action)
	}
	return filtered
}

func actionID(kind string, node *xmlNode, label string) string {
	identity := strings.Join([]string{kind, node.ResourceID, node.Class, node.Bounds, stableText(label)}, "|")
	sum := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(sum[:8])
}

func isRecoveryText(label string, rules Rules) bool {
	normalized := stableText(label)
	for _, value := range append(append([]string{}, defaultRecoveryText...), rules.RecoveryText...) {
		if normalized == stableText(value) && normalized != "" {
			return true
		}
	}
	return false
}

func parseBounds(value string) (Bounds, bool) {
	match := boundsPattern.FindStringSubmatch(strings.TrimSpace(value))
	if len(match) != 5 {
		return Bounds{}, false
	}
	values := [4]int{}
	for i := range values {
		parsed, err := strconv.Atoi(match[i+1])
		if err != nil {
			return Bounds{}, false
		}
		values[i] = parsed
	}
	bounds := Bounds{Left: values[0], Top: values[1], Right: values[2], Bottom: values[3]}
	return bounds, bounds.Valid()
}

func allowed(text, description, resourceID string, rules Rules) bool {
	haystack := strings.ToLower(strings.Join([]string{text, description, resourceID}, " "))
	for _, value := range append(append([]string{}, defaultDeniedText...), rules.DenyText...) {
		if normalized := strings.ToLower(strings.TrimSpace(value)); normalized != "" && strings.Contains(haystack, normalized) {
			return false
		}
	}
	if len(rules.AllowText) == 0 && len(rules.AllowResourcePrefixes) == 0 {
		return true
	}
	for _, value := range rules.AllowText {
		if normalized := strings.ToLower(strings.TrimSpace(value)); normalized != "" && strings.Contains(haystack, normalized) {
			return true
		}
	}
	for _, prefix := range rules.AllowResourcePrefixes {
		if prefix = strings.TrimSpace(prefix); prefix != "" && strings.HasPrefix(resourceID, prefix) {
			return true
		}
	}
	return false
}

func stableText(value string) string {
	value = strings.ToLower(strings.Join(strings.Fields(value), " "))
	return digitsPattern.ReplaceAllString(value, "#")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
