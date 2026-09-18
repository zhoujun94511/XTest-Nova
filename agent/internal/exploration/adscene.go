package exploration

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

const (
	adTypeAppOpen      = "app_open"
	adTypeInterstitial = "interstitial"
	adTypeRewarded     = "rewarded_video"
	adTypePlayable     = "playable_fullscreen"
	adPhaseWaiting     = "waiting_close"
	adPhaseClose       = "close_ready"
	adPhaseConfirm     = "confirm_exit"
	adPhaseEndCard     = "end_card"
)

type adScene struct{ Type, Phase string }

var adMarkers = []string{"advertisement", "sponsored", "skip ad", "close ad", "广告", "跳过广告"}
var adActivityMarkers = []string{
	"com.google.android.gms.ads.adactivity", "com.bytedance.sdk.openadsdk.activity.", "com.qq.e.ads.",
	"com.applovin.adview.", "com.unity3d.services.ads.adunit.", "com.ironsource.sdk.controller.",
	"com.inmobi.ads.rendering.", "com.mbridge.msdk.", "com.vungle.ads.", "com.chartboost.sdk.",
	"com.sigmob.sdk.", "sg.bigo.ads.", "com.huawei.openalliance.ad.", "com.miui.systemadsolution.",
	"com.kwad.sdk.", "com.baidu.mobads.", "com.anythink.", "com.tradplus.", "com.fyber.",
	"com.digitalturbine.", "com.smaato.", "com.moloco.sdk.", "com.yandex.mobile.ads.",
}
var adRewardMarkers = []string{"rewarded", "reward video", "earn reward", "watch video", "lose reward", "奖励视频", "观看视频", "获得奖励", "放弃奖励"}
var adPlayableMarkers = []string{"play now", "try now", "试玩", "立即试玩", "立即游玩"}
var adAppOpenMarkers = []string{"continue to app", "continue to application", "继续打开应用", "进入应用"}
var adConfirmMarkers = []string{"lose reward", "give up reward", "continue watching", "放弃奖励", "继续观看", "确认退出", "仍要退出"}
var adEndCardMarkers = []string{"install now", "download now", "get app", "open app", "立即安装", "立即下载", "打开应用"}
var adExitTargets = []string{"give up", "exit", "leave", "close anyway", "放弃", "退出", "仍要退出", "确认退出", "关闭广告"}
var adDismissText = append([]string{}, dismissTargets...)

func init() {
	adDismissText = append(adDismissText, "close and continue to app", "关闭广告并继续打开应用")
}

func classifyAdScene(activity, pageText string) adScene {
	activity, pageText = normalizeSpecial(activity), normalizeSpecial(pageText)
	scene := adScene{Type: adTypeInterstitial, Phase: adPhaseWaiting}
	switch {
	case containsAny(pageText, adAppOpenMarkers) || strings.Contains(activity, "appopen") || strings.Contains(activity, "splash"):
		scene.Type = adTypeAppOpen
	case containsAny(pageText, adRewardMarkers) || strings.Contains(activity, "reward"):
		scene.Type = adTypeRewarded
	case containsAny(pageText, adPlayableMarkers) || strings.Contains(activity, "playable"):
		scene.Type = adTypePlayable
	}
	switch {
	case containsAny(pageText, adConfirmMarkers):
		scene.Phase = adPhaseConfirm
	case containsAny(pageText, adEndCardMarkers) && !containsAny(pageText, adAppOpenMarkers):
		scene.Phase = adPhaseEndCard
	case containsAny(pageText, adDismissText) || containsAny(pageText, dismissResourceFragments):
		scene.Phase = adPhaseClose
	}
	return scene
}

func adWaitLimitSeconds(adType string, configuredMaximum int) int {
	limit := 20
	switch adType {
	case adTypeAppOpen:
		limit = 15
	case adTypePlayable:
		limit = 45
	case adTypeRewarded:
		limit = 75
	}
	if configuredMaximum > 0 && configuredMaximum < limit {
		return configuredMaximum
	}
	return limit
}

func isEmbeddedAdContainer(resourceID, description, className string) bool {
	value := normalizeSpecial(strings.Join([]string{resourceID, description, className}, " "))
	markers := []string{"advertisement-card", "ad_iframe", "native_ad", "nativead", "banner_ad", "bannerad", "ad_container", "adcontainer", "ad_view", "adview", ":id/ad_", "/id/ad_"}
	return containsAny(value, markers)
}

func isAdDismissControl(label, resourceID string) bool {
	value := normalizeSpecial(label + " " + resourceID)
	return matchesSpecialTarget(value, adDismissText) || containsAny(value, dismissResourceFragments)
}

func isKnownAdActivity(activity string) bool {
	return containsAny(normalizeSpecial(activity), adActivityMarkers)
}

func specialBackAction(kind, foreground, pageText string) Action {
	sum := sha256.Sum256([]byte(kind + "-back|" + foreground + "|" + pageText))
	return Action{ID: hex.EncodeToString(sum[:8]), Type: "back", Description: "bounded " + kind + " fallback", SpecialKind: kind, Policy: policyDismiss, Recovery: true}
}

func findExplicitAdDismissAction(nodes []specialNode, foreground, phase string) (Action, bool) {
	targets := adDismissText
	if phase == adPhaseConfirm {
		targets = adExitTargets
	}
	viewport, viewportOK := specialViewport(nodes, foreground)
	bestArea, bestScore := int(^uint(0)>>1), -1
	var best specialNode
	for _, node := range nodes {
		if node.Package != "" && foreground != "" && node.Package != foreground || node.Enabled == "false" || node.Visible == "false" || node.Password == "true" {
			continue
		}
		bounds, ok := parseBounds(node.Bounds)
		if !ok {
			continue
		}
		label := normalizeSpecial(strings.Join([]string{node.Text, node.Description, node.ResourceID}, " "))
		resourceMatch := containsAny(normalizeSpecial(node.ResourceID), dismissResourceFragments)
		textMatch := matchesSpecialTarget(label, targets)
		if !textMatch && !resourceMatch {
			continue
		}
		area := (bounds.Right - bounds.Left) * (bounds.Bottom - bounds.Top)
		if viewportOK && area > (viewport.Right-viewport.Left)*(viewport.Bottom-viewport.Top)/4 {
			continue
		}
		score := 0
		if resourceMatch {
			score += 200
		}
		if textMatch {
			score += 100
		}
		if node.Clickable == "true" {
			score += 20
		}
		if score > bestScore || score == bestScore && area < bestArea {
			best, bestArea, bestScore = node, area, score
		}
	}
	if bestScore < 0 {
		return Action{}, false
	}
	bounds, _ := parseBounds(best.Bounds)
	x, y := bounds.Center()
	// Web ads often expose the whole close bar as a non-clickable accessibility
	// node while the actual X is its rightmost square.
	if strings.Contains(normalizeSpecial(best.ResourceID), "close") && bounds.Right-bounds.Left > 2*(bounds.Bottom-bounds.Top) {
		x = bounds.Right - (bounds.Bottom-bounds.Top)/2
	}
	sum := sha256.Sum256([]byte(strings.Join([]string{"ad-explicit", foreground, best.ResourceID, best.Text, best.Bounds}, "|")))
	return Action{ID: hex.EncodeToString(sum[:8]), Type: "tap", X: x, Y: y, Text: best.Text, Description: firstNonEmpty(best.Description, "explicit ad dismiss control"), ResourceID: best.ResourceID, Class: best.Class, Bounds: bounds}, true
}

func findAdCornerAction(nodes []specialNode, foreground string) (Action, bool) {
	viewport, ok := specialViewport(nodes, foreground)
	if !ok || viewport.Right-viewport.Left < 400 || viewport.Bottom-viewport.Top < 600 {
		return Action{}, false
	}
	width, height := viewport.Right-viewport.Left, viewport.Bottom-viewport.Top
	maxArea := width * height / 30
	bestScore := -1
	var best specialNode
	for _, node := range nodes {
		if node.Package != "" && foreground != "" && node.Package != foreground {
			continue
		}
		if node.Enabled == "false" || node.Visible == "false" || node.Password == "true" || node.Clickable != "true" || strings.EqualFold(node.Class, "android.webkit.WebView") {
			continue
		}
		bounds, valid := parseBounds(node.Bounds)
		if !valid {
			continue
		}
		itemWidth, itemHeight := bounds.Right-bounds.Left, bounds.Bottom-bounds.Top
		area := itemWidth * itemHeight
		x, y := bounds.Center()
		if y > viewport.Top+height/4 || (x > viewport.Left+width/4 && x < viewport.Right-width/4) || area < 144 || area > maxArea || itemWidth > width/4 || itemHeight > height/5 {
			continue
		}
		label := normalizeSpecial(firstNonEmpty(node.Text, node.Description))
		resource := normalizeSpecial(node.ResourceID)
		score := 10
		if matchesSpecialTarget(label, adDismissText) || containsAny(resource, dismissResourceFragments) {
			score += 100
		}
		if label == "" {
			score += 20
		}
		if area < maxArea/3 {
			score += 10
		}
		if score > bestScore {
			bestScore, best = score, node
		}
	}
	if bestScore < 0 {
		return Action{}, false
	}
	bounds, _ := parseBounds(best.Bounds)
	x, y := bounds.Center()
	sum := sha256.Sum256([]byte(strings.Join([]string{"ad-corner", foreground, best.ResourceID, best.Class, best.Bounds}, "|")))
	return Action{ID: hex.EncodeToString(sum[:8]), Type: "tap", X: x, Y: y, Text: best.Text, Description: firstNonEmpty(best.Description, "top-corner ad dismiss control"), ResourceID: best.ResourceID, Class: best.Class, Bounds: bounds}, true
}

func specialViewport(nodes []specialNode, foreground string) (Bounds, bool) {
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
		if area := (bounds.Right - bounds.Left) * (bounds.Bottom - bounds.Top); area > bestArea {
			selected, bestArea = bounds, area
		}
	}
	return selected, bestArea > 0
}
