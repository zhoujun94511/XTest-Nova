package exploration

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"strings"
)

const (
	specialModeSafe     = "safe"
	specialModeDisabled = "disabled"
	policyPause         = "pause"
	policyAccept        = "accept"
	policyDecline       = "decline"
	policyAllow         = "allow"
	policyDeny          = "deny"
	policyDismiss       = "dismiss"
	policyExplore       = "explore"
	policyAdvance       = "advance"
)

// SpecialHandling controls the deterministic pre-processing performed before
// ordinary graph exploration. Empty values select the conservative defaults.
type SpecialHandling struct {
	Mode             string `json:"mode,omitempty"`
	ConsentPolicy    string `json:"consentPolicy,omitempty"`
	PermissionPolicy string `json:"permissionPolicy,omitempty"`
	PaywallPolicy    string `json:"paywallPolicy,omitempty"`
	AdPolicy         string `json:"adPolicy,omitempty"`
	ReviewPolicy     string `json:"reviewPolicy,omitempty"`
	OnboardingPolicy string `json:"onboardingPolicy,omitempty"`
	MaxAttempts      int    `json:"maxAttempts,omitempty"`
	AdMaxWaitSeconds int    `json:"adMaxWaitSeconds,omitempty"`
}

func (p *SpecialHandling) normalize() error {
	if p.Mode == "" {
		p.Mode = specialModeSafe
	}
	if p.ConsentPolicy == "" {
		p.ConsentPolicy = policyAccept
	}
	if p.PermissionPolicy == "" {
		p.PermissionPolicy = policyAllow
	}
	if p.PaywallPolicy == "" {
		p.PaywallPolicy = policyExplore
	}
	if p.AdPolicy == "" {
		p.AdPolicy = policyDismiss
	}
	if p.ReviewPolicy == "" {
		p.ReviewPolicy = policyDismiss
	}
	if p.OnboardingPolicy == "" {
		p.OnboardingPolicy = policyAdvance
	}
	if p.MaxAttempts == 0 {
		p.MaxAttempts = 3
	}
	if p.AdMaxWaitSeconds == 0 {
		p.AdMaxWaitSeconds = 75
	}
	if !oneOf(p.Mode, specialModeSafe, specialModeDisabled) {
		return errors.New("specialHandling.mode must be safe or disabled")
	}
	if !oneOf(p.ConsentPolicy, policyPause, policyAccept, policyDecline) {
		return errors.New("specialHandling.consentPolicy must be pause, accept, or decline")
	}
	if !oneOf(p.PermissionPolicy, policyPause, policyAllow, policyDeny) {
		return errors.New("specialHandling.permissionPolicy must be pause, allow, or deny")
	}
	if !oneOf(p.PaywallPolicy, policyPause, policyDismiss, policyExplore) {
		return errors.New("specialHandling.paywallPolicy must be pause, dismiss, or explore")
	}
	if !oneOf(p.AdPolicy, policyPause, policyDismiss) {
		return errors.New("specialHandling.adPolicy must be pause or dismiss")
	}
	if !oneOf(p.ReviewPolicy, policyPause, policyDismiss) {
		return errors.New("specialHandling.reviewPolicy must be pause or dismiss")
	}
	if !oneOf(p.OnboardingPolicy, policyPause, policyAdvance) {
		return errors.New("specialHandling.onboardingPolicy must be pause or advance")
	}
	if p.MaxAttempts < 1 || p.MaxAttempts > 20 {
		return errors.New("specialHandling.maxAttempts must be between 1 and 20")
	}
	if p.AdMaxWaitSeconds < 1 || p.AdMaxWaitSeconds > 180 {
		return errors.New("specialHandling.adMaxWaitSeconds must be between 1 and 180")
	}
	return nil
}

func oneOf(value string, values ...string) bool {
	for _, candidate := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

type SpecialEvent struct {
	Kind             string  `json:"kind"`
	Policy           string  `json:"policy"`
	Outcome          string  `json:"outcome"`
	Fingerprint      string  `json:"fingerprint,omitempty"`
	Foreground       string  `json:"foreground,omitempty"`
	AdType           string  `json:"adType,omitempty"`
	AdPhase          string  `json:"adPhase,omitempty"`
	WaitLimitSeconds int     `json:"waitLimitSeconds,omitempty"`
	Reason           string  `json:"reason,omitempty"`
	Action           *Action `json:"action,omitempty"`
	FallbackAction   *Action `json:"fallbackAction,omitempty"`
}

type specialNode struct {
	Text        string        `xml:"text,attr"`
	Description string        `xml:"content-desc,attr"`
	ResourceID  string        `xml:"resource-id,attr"`
	Class       string        `xml:"class,attr"`
	Package     string        `xml:"package,attr"`
	Bounds      string        `xml:"bounds,attr"`
	Clickable   string        `xml:"clickable,attr"`
	Enabled     string        `xml:"enabled,attr"`
	Visible     string        `xml:"visible-to-user,attr"`
	Password    string        `xml:"password,attr"`
	Children    []specialNode `xml:"node"`
}

type specialDocument struct {
	Nodes []specialNode `xml:"node"`
}

var permissionControllerPackages = map[string]struct{}{
	"com.android.permissioncontroller":         {},
	"com.google.android.permissioncontroller":  {},
	"com.android.packageinstaller":             {},
	"com.google.android.packageinstaller":      {},
	"com.miui.packageinstaller":                {},
	"com.miui.securitycenter":                  {},
	"com.samsung.android.permissioncontroller": {},
}
var reviewControllerPackages = map[string]struct{}{
	"com.android.vending": {},
}
var systemPickerPackages = map[string]struct{}{
	"com.android.documentsui":            {},
	"com.google.android.documentsui":     {},
	"com.google.android.photopicker":     {},
	"com.android.providers.media.module": {},
}
var systemSettingsPackages = map[string]struct{}{
	"com.android.settings":         {},
	"com.google.android.settings":  {},
	"com.samsung.android.settings": {},
}

var consentMarkers = []string{
	"terms of service", "terms and conditions", "privacy policy", "user agreement", "agree to the terms",
	"用户协议", "服务条款", "使用条款", "隐私政策", "隐私条款", "同意条款",
}
var consentIntentMarkers = []string{
	"by tapping", "by clicking", "agree to", "i agree", "accept the terms", "accept terms",
	"我已阅读并同意", "同意并继续", "点击继续即表示", "点击即表示同意",
}
var paywallMarkers = []string{
	"subscription", "subscribe", "free trial", "start trial", "restore purchase", "premium", "pro plan",
	"订阅", "免费试用", "开始试用", "恢复购买", "高级版", "会员",
}
var paywallContextDeniedText = []string{
	"continue", "next", "unlock", "upgrade", "get premium", "free trial", "start trial", "restore",
	"继续", "下一步", "解锁", "升级", "试用", "恢复",
}
var reviewMarkers = []string{"rate this app", "reviews are only visible", "first star", "评价此应用", "评分", "撰写评价"}
var purchaseMarkers = []string{"subscribe", "confirm purchase", "payment method", "订阅", "确认购买", "付款方式"}
var targetPurchaseConfirmationMarkers = []string{"pay now", "order info", "one-time purchase", "confirm payment", "place order", "立即支付", "订单信息", "确认支付", "提交订单"}
var billingErrorMarkers = []string{"not configured for google play billing", "未配置为通过 google play 结算"}
var billingErrorTargets = []string{"ok", "got it", "知道了", "确定"}
var resolverMarkers = []string{"open with", "complete action using", "just once", "打开方式", "仅此一次", "始终使用选择的应用程序"}

var consentAcceptTargets = []string{"agree", "accept", "continue", "i agree", "同意", "接受", "继续"}
var consentDeclineTargets = []string{"decline", "disagree", "exit", "不同意", "拒绝", "退出"}
var permissionAllowTargets = []string{"allow", "while using the app", "only this time", "允许", "使用应用时", "仅此一次"}
var permissionDenyTargets = []string{"don’t allow", "don't allow", "deny", "not now", "拒绝", "不允许", "暂不"}
var dismissTargets = []string{"close", "dismiss", "not now", "no thanks", "maybe later", "later", "skip", "cancel", "x", "关闭", "稍后", "以后再说", "不了", "跳过", "取消", "暂不"}
var dismissResourceFragments = []string{"close", "dismiss", "cancel", "skip", "not_now", "later"}

func inspectSpecial(hierarchy, targetPackage, foreground string, policy SpecialHandling) (SpecialEvent, bool, error) {
	return inspectSpecialActivity(hierarchy, targetPackage, foreground, "", policy)
}

func inspectSpecialActivity(hierarchy, targetPackage, foreground, activity string, policy SpecialHandling) (SpecialEvent, bool, error) {
	if policy.Mode == specialModeDisabled {
		return SpecialEvent{}, false, nil
	}
	var document specialDocument
	if err := xml.Unmarshal([]byte(hierarchy), &document); err != nil {
		return SpecialEvent{}, false, fmt.Errorf("parse special-scene hierarchy: %w", err)
	}
	nodes := make([]specialNode, 0, 64)
	for i := range document.Nodes {
		flattenSpecialNodes(&document.Nodes[i], &nodes)
	}
	pageText := specialPageText(nodes, foreground)
	kind, selectedPolicy := "", ""
	var targets, resourceFragments []string
	if isPermissionController(foreground) {
		kind, selectedPolicy = "permission", policy.PermissionPolicy
		if selectedPolicy == policyAllow {
			targets = permissionAllowTargets
		} else if selectedPolicy == policyDeny {
			targets = permissionDenyTargets
		}
	} else if foreground != targetPackage && isSystemPicker(foreground) {
		kind, selectedPolicy = "system_picker", policyDismiss
	} else if foreground != targetPackage && isSystemSettings(foreground) {
		kind, selectedPolicy = "system_settings", policyDismiss
	} else if foreground != targetPackage && foreground == "android" && containsAny(pageText, resolverMarkers) {
		kind, selectedPolicy = "system_resolver", policyDismiss
	} else if isReviewController(foreground) && containsAny(pageText, reviewMarkers) {
		kind, selectedPolicy, targets, resourceFragments = "review", policy.ReviewPolicy, dismissTargets, dismissResourceFragments
	} else if isReviewController(foreground) && containsAny(pageText, billingErrorMarkers) {
		kind, selectedPolicy, targets = "billing_error", policyDismiss, billingErrorTargets
	} else if isReviewController(foreground) && containsAny(pageText, purchaseMarkers) {
		kind, selectedPolicy = "purchase_confirmation", policyDismiss
	} else if foreground == targetPackage && containsAny(pageText, targetPurchaseConfirmationMarkers) {
		kind, selectedPolicy, targets, resourceFragments = "purchase_confirmation", policyDismiss, dismissTargets, dismissResourceFragments
	} else if foreground == targetPackage && isConsentPage(pageText) && hasConsentDecision(nodes, foreground) {
		kind, selectedPolicy = "consent", policy.ConsentPolicy
		if selectedPolicy == policyAccept {
			targets = consentAcceptTargets
		} else if selectedPolicy == policyDecline {
			targets = consentDeclineTargets
		}
	} else if foreground == targetPackage && containsAllGroups(pageText, []string{"skip", "跳过"}, []string{"continue", "继续"}) {
		kind, selectedPolicy, targets = "onboarding", policy.OnboardingPolicy, []string{"continue", "next", "继续", "下一步"}
	} else if isKnownAdActivity(activity) {
		kind, selectedPolicy, targets, resourceFragments = "ad", policy.AdPolicy, dismissTargets, dismissResourceFragments
	} else if foreground == targetPackage && containsAny(pageText, paywallMarkers) {
		kind, selectedPolicy, targets, resourceFragments = "paywall", policy.PaywallPolicy, dismissTargets, dismissResourceFragments
	} else if foreground == targetPackage && containsAny(pageText, adMarkers) && hasSpecialTarget(nodes, foreground, dismissTargets, dismissResourceFragments) {
		kind, selectedPolicy, targets, resourceFragments = "ad", policy.AdPolicy, dismissTargets, dismissResourceFragments
	} else {
		return SpecialEvent{}, false, nil
	}
	event := SpecialEvent{Kind: kind, Policy: selectedPolicy, Foreground: foreground}
	if kind == "ad" {
		scene := classifyAdScene(activity, pageText)
		event.AdType, event.AdPhase = scene.Type, scene.Phase
		event.WaitLimitSeconds = adWaitLimitSeconds(scene.Type, policy.AdMaxWaitSeconds)
	}
	event.Fingerprint = specialFingerprint(kind, foreground, pageText)
	if selectedPolicy == policyExplore {
		event.Outcome = "explore"
		event.Reason = "paywall is retained for safe graph exploration"
		return event, true, nil
	}
	if selectedPolicy == policyPause {
		event.Outcome = "policy_required"
		event.Reason = kind + " requires an explicit policy"
		return event, true, nil
	}
	if kind == "purchase_confirmation" && len(targets) > 0 {
		if action, ok := findSpecialAction(nodes, foreground, targets, resourceFragments); ok {
			action.SpecialKind, action.Policy, action.Recovery = kind, selectedPolicy, true
			event.Outcome, event.Action = "action", &action
			return event, true, nil
		}
	}
	if kind == "purchase_confirmation" || kind == "system_picker" || kind == "system_settings" || kind == "system_resolver" {
		sum := sha256.Sum256([]byte(kind + "-back|" + foreground + "|" + pageText))
		action := Action{ID: hex.EncodeToString(sum[:8]), Type: "back", Description: "dismiss " + strings.ReplaceAll(kind, "_", " ")}
		action.SpecialKind = kind
		action.Policy = selectedPolicy
		action.Recovery = true
		event.Outcome = "action"
		event.Action = &action
		return event, true, nil
	}
	action, ok := findSpecialAction(nodes, foreground, targets, resourceFragments)
	if kind == "ad" && isKnownAdActivity(activity) {
		if candidate, candidateOK := findExplicitAdDismissAction(nodes, foreground, event.AdPhase); candidateOK {
			action, ok = candidate, true
		}
		if candidate, candidateOK := findAdCornerAction(nodes, foreground); candidateOK && !ok {
			action, ok = candidate, true
		}
		if !ok {
			event.Outcome = "wait"
			event.Reason = "known full-screen ad activity has not exposed a safe dismiss control"
			fallback := specialBackAction("ad", foreground, pageText)
			event.FallbackAction = &fallback
			return event, true, nil
		}
	}
	if kind == "onboarding" {
		if fallback, fallbackOK := onboardingSwipe(nodes, foreground); fallbackOK {
			fallback.SpecialKind = kind
			fallback.Policy = selectedPolicy
			fallback.Recovery = true
			event.FallbackAction = &fallback
			if !ok {
				action, ok = fallback, true
			}
		}
	}
	if !ok {
		if kind == "permission" {
			event.Outcome = "wait"
			event.WaitLimitSeconds = 5
			event.Reason = "permission controller has not exposed a safe decision control"
			return event, true, nil
		}
		event.Outcome = "unhandled"
		event.Reason = "no safe action matched the selected policy"
		return event, true, nil
	}
	action.SpecialKind = kind
	action.Policy = selectedPolicy
	action.Recovery = true
	event.Outcome = "action"
	event.Action = &action
	return event, true, nil
}

func isPermissionController(packageName string) bool {
	_, ok := permissionControllerPackages[packageName]
	return ok
}

func isReviewController(packageName string) bool {
	_, ok := reviewControllerPackages[packageName]
	return ok
}

func isSystemPicker(packageName string) bool {
	_, ok := systemPickerPackages[packageName]
	return ok
}

func isSystemSettings(packageName string) bool {
	_, ok := systemSettingsPackages[packageName]
	return ok
}

func isTransientController(packageName string) bool {
	return isPermissionController(packageName) || isReviewController(packageName) || isSystemPicker(packageName) || isSystemSettings(packageName)
}

func isConsentPage(pageText string) bool {
	return containsAny(pageText, consentMarkers) && containsAny(pageText, consentIntentMarkers)
}

func hasConsentDecision(nodes []specialNode, foreground string) bool {
	if _, ok := findSpecialAction(nodes, foreground, consentAcceptTargets, nil); ok {
		return true
	}
	_, ok := findSpecialAction(nodes, foreground, consentDeclineTargets, nil)
	return ok
}

func hasSpecialTarget(nodes []specialNode, foreground string, targets, resourceFragments []string) bool {
	_, ok := findSpecialAction(nodes, foreground, targets, resourceFragments)
	return ok
}

func flattenSpecialNodes(node *specialNode, result *[]specialNode) string {
	labels := make([]string, 0, len(node.Children)+2)
	if value := firstNonEmpty(node.Text, node.Description); value != "" {
		labels = append(labels, value)
	}
	for i := range node.Children {
		if label := flattenSpecialNodes(&node.Children[i], result); label != "" {
			labels = append(labels, label)
		}
	}
	label := strings.Join(labels, " ")
	flattened := *node
	if flattened.Text == "" && flattened.Description == "" {
		flattened.Description = label
	}
	*result = append(*result, flattened)
	return label
}

func specialPageText(nodes []specialNode, foreground string) string {
	parts := make([]string, 0, len(nodes)*2)
	for _, node := range nodes {
		if node.Package != "" && foreground != "" && node.Package != foreground {
			continue
		}
		parts = append(parts, node.Text, node.Description, node.ResourceID)
	}
	return normalizeSpecial(strings.Join(parts, " "))
}

func findSpecialAction(nodes []specialNode, foreground string, targets, resourceFragments []string) (Action, bool) {
	for _, node := range nodes {
		if node.Package != "" && foreground != "" && node.Package != foreground {
			continue
		}
		if node.Enabled == "false" || node.Visible == "false" || node.Password == "true" || node.Clickable != "true" {
			continue
		}
		if strings.EqualFold(node.Class, "android.webkit.WebView") {
			continue
		}
		bounds, ok := parseBounds(node.Bounds)
		if !ok {
			continue
		}
		label := normalizeSpecial(firstNonEmpty(node.Text, node.Description))
		resource := normalizeSpecial(node.ResourceID)
		if !matchesSpecialTarget(label, targets) && !containsAny(resource, resourceFragments) {
			continue
		}
		x, y := bounds.Center()
		identity := strings.Join([]string{"special", foreground, node.ResourceID, node.Class, node.Bounds, label}, "|")
		sum := sha256.Sum256([]byte(identity))
		return Action{ID: hex.EncodeToString(sum[:8]), Type: "tap", X: x, Y: y, Text: node.Text, Description: node.Description, ResourceID: node.ResourceID, Class: node.Class, Bounds: bounds}, true
	}
	return Action{}, false
}

func matchesSpecialTarget(label string, targets []string) bool {
	for _, target := range targets {
		target = normalizeSpecial(target)
		if label == target || (len(target) > 2 && strings.Contains(label, target)) {
			return true
		}
	}
	return false
}

func containsAny(value string, candidates []string) bool {
	for _, candidate := range candidates {
		if candidate = normalizeSpecial(candidate); candidate != "" && strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}

func containsAllGroups(value string, groups ...[]string) bool {
	for _, group := range groups {
		if !containsAny(value, group) {
			return false
		}
	}
	return true
}

func onboardingSwipe(nodes []specialNode, foreground string) (Action, bool) {
	var selected Bounds
	bestArea := 0
	for _, node := range nodes {
		if node.Package != "" && foreground != "" && node.Package != foreground {
			continue
		}
		bounds, ok := parseBounds(node.Bounds)
		if !ok {
			continue
		}
		area := (bounds.Right - bounds.Left) * (bounds.Bottom - bounds.Top)
		if area > bestArea {
			selected, bestArea = bounds, area
		}
	}
	if bestArea == 0 || selected.Right-selected.Left < 200 {
		return Action{}, false
	}
	y := selected.Top + (selected.Bottom-selected.Top)/2
	startX := selected.Left + (selected.Right-selected.Left)*4/5
	endX := selected.Left + (selected.Right-selected.Left)/5
	sum := sha256.Sum256([]byte(fmt.Sprintf("onboarding-swipe|%s|%d|%d|%d", foreground, startX, endX, y)))
	return Action{ID: hex.EncodeToString(sum[:8]), Type: "swipe", Direction: "left", X: startX, Y: y, EndX: endX, EndY: y, Bounds: selected, Description: "onboarding swipe left"}, true
}

func normalizeSpecial(value string) string {
	value = strings.ToLower(strings.Join(strings.Fields(value), " "))
	return strings.ReplaceAll(value, "’", "'")
}

func specialFingerprint(kind, foreground, pageText string) string {
	sum := sha256.Sum256([]byte(kind + "|" + foreground + "|" + stableText(pageText)))
	return hex.EncodeToString(sum[:12])
}
