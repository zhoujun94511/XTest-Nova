package exploration

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"strings"
)

const (
	maxNonScrollSelectionsBeforeScroll = 2
	maxStableScrollAttempts            = 3
)

var externalRiskMarkers = []string{
	"cloud", "sync", "account", "sign in", "login", "share", "backup", "drive",
	"云", "同步", "账号", "账户", "登录", "分享", "备份", "网盘",
}

func isPurchaseControl(action Action) bool {
	resource := strings.ToLower(action.ResourceID)
	for _, fragment := range []string{"sku", "billing", "purchase", "subscribe", "recharge", "cashier"} {
		if strings.Contains(resource, fragment) {
			return true
		}
	}
	label := strings.ToLower(strings.Join([]string{action.Text, action.Description}, " "))
	return containsAny(label, purchaseMarkers) || containsAny(label, []string{"充值", "recharge", "top up", "top-up"})
}

func isExternalRisk(action Action) bool {
	label := strings.ToLower(strings.Join([]string{action.Description, action.ResourceID, action.Text}, " "))
	for _, marker := range externalRiskMarkers {
		if marker != "" && strings.Contains(label, marker) {
			return true
		}
	}
	return false
}

// languageSettingLabels match a control whose own text is the language entry.
// A longer title that merely contains these words stays eligible.
var languageSettingLabels = map[string]struct{}{}

// languageOptionLabels match a control that is only a language name. They apply
// on settings pages, where that control changes the display language.
var languageOptionLabels = map[string]struct{}{}

func init() {
	for _, label := range []string{
		"language", "languages", "locale",
		"app language", "display language", "change language", "switch language", "system language", "select language",
		"语言", "語言", "语言设置", "語言設置", "语言设定", "語言設定", "切换语言", "切換語言",
		"应用语言", "應用語言", "显示语言", "顯示語言", "多语言", "多語言", "选择语言", "選擇語言",
	} {
		languageSettingLabels[stableText(label)] = struct{}{}
	}
	for _, label := range []string{
		"english", "chinese", "spanish", "french", "german", "korean", "russian",
		"portuguese", "arabic", "hindi", "vietnamese", "indonesian", "malay",
		"turkish", "italian", "polish", "dutch", "thai", "ukrainian", "czech",
		"swedish", "danish", "finnish", "norwegian", "romanian", "hungarian",
		"greek", "hebrew", "japanese",
		"中文", "简体中文", "简体", "繁体中文", "繁体", "繁體中文", "繁體", "简體中文", "简體",
		"英语", "英文", "汉语", "漢語", "日语", "日語", "日本語", "韩语", "韓語", "한국어",
		"español", "français", "deutsch", "русский", "português", "العربية", "हिन्दी",
		"tiếng việt", "bahasa indonesia", "bahasa melayu", "türkçe", "italiano",
		"polski", "nederlands", "ไทย", "ภาษาไทย", "українська", "čeština", "svenska",
		"dansk", "suomi", "norsk", "română", "magyar", "ελληνικά", "עברית",
	} {
		languageOptionLabels[stableText(label)] = struct{}{}
	}
}

func activitySimpleName(activity string) string {
	name := strings.ToLower(strings.TrimSpace(activity))
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	name = strings.TrimPrefix(name, ".")
	if i := strings.LastIndex(name, "."); i >= 0 {
		name = name[i+1:]
	}
	return name
}

func resourceEntry(resourceID string) string {
	value := strings.ToLower(strings.TrimSpace(resourceID))
	if i := strings.LastIndex(value, "/"); i >= 0 {
		return value[i+1:]
	}
	if i := strings.LastIndex(value, ":"); i >= 0 {
		return value[i+1:]
	}
	return value
}

func isLanguageSettingsActivity(activity string) bool {
	name := activitySimpleName(activity)
	if strings.Contains(name, "launcher") {
		return false
	}
	return strings.Contains(name, "language") || strings.Contains(name, "locale")
}

func isLanguageSettingLabel(value string) bool {
	normalized := stableText(value)
	if normalized == "" {
		return false
	}
	_, ok := languageSettingLabels[normalized]
	return ok
}

func isLanguageOptionLabel(value string) bool {
	normalized := stableText(value)
	if normalized == "" {
		return false
	}
	_, ok := languageOptionLabels[normalized]
	return ok
}

func isLanguageSwitch(action Action) bool {
	if action.Type == "swipe" || action.Type == "back" || isVisibleBackControl(action) {
		return false
	}
	if isLanguageOptionLabel(action.Text) || isLanguageOptionLabel(action.Description) {
		return false
	}
	entry := resourceEntry(action.ResourceID)
	for _, fragment := range []string{"language", "locale", "i18n", "lang_", "_lang"} {
		if strings.Contains(entry, fragment) {
			return true
		}
	}
	if entry == "lang" {
		return true
	}
	return isLanguageSettingLabel(action.Text) || isLanguageSettingLabel(action.Description)
}

func isLanguageChoice(activity string, action Action) bool {
	if action.Type == "swipe" || action.Type == "back" || isVisibleBackControl(action) {
		return false
	}
	name := activitySimpleName(activity)
	if !strings.Contains(name, "setting") && !strings.Contains(name, "language") && !strings.Contains(name, "locale") {
		return false
	}
	return isLanguageOptionLabel(action.Text) || isLanguageOptionLabel(action.Description)
}

func isLanguagePickerNeutral(action Action) bool {
	if isVisibleBackControl(action) {
		return true
	}
	if strings.TrimSpace(action.Text) == "" && strings.TrimSpace(action.Description) == "" {
		return true
	}
	return isLanguageSettingLabel(action.Text) || isLanguageSettingLabel(action.Description)
}

func hasLanguageResource(action Action) bool {
	entry := resourceEntry(action.ResourceID)
	for _, fragment := range []string{"language", "locale", "i18n", "lang_", "_lang"} {
		if strings.Contains(entry, fragment) {
			return true
		}
	}
	return entry == "lang"
}

func blocksNamedLanguageResource(activity string, actions []Action, action Action) bool {
	if action.Type == "swipe" || action.Type == "back" || isVisibleBackControl(action) {
		return false
	}
	if !isLanguageOptionLabel(action.Text) && !isLanguageOptionLabel(action.Description) {
		return false
	}
	if !hasLanguageResource(action) {
		return false
	}
	if isStartupLanguageSurface(activity) {
		if preferred, ok := preferredStartupLanguage(actions); ok && semanticActionKey(action) == semanticActionKey(preferred) {
			return false
		}
	}
	return true
}

func isLanguagePickerConfirm(value string) bool {
	switch stableText(value) {
	case "continue", "next", "skip", "get started", "start exploring",
		"ok", "okay", "confirm", "done", "apply",
		"继续", "下一步", "跳过", "确定", "确认", "完成":
		return true
	default:
		return false
	}
}

// languagePageCensus separates a first-launch language list from a page that
// merely shows one language name. Two exact language names, with every other
// tap being a confirm or skip, is the picker. A content control keeps the page ordinary.
func languagePageCensus(actions []Action) (options, other int) {
	for _, action := range actions {
		if action.Type == "swipe" || action.Type == "back" || action.Type == "scroll" || isVisibleBackControl(action) {
			continue
		}
		if isLanguageOptionLabel(action.Text) || isLanguageOptionLabel(action.Description) {
			options++
			continue
		}
		if isLanguagePickerNeutral(action) || isLanguagePickerConfirm(action.Text) || isLanguagePickerConfirm(action.Description) {
			continue
		}
		other++
	}
	return options, other
}

func isLanguagePickerPage(actions []Action) bool {
	options, other := languagePageCensus(actions)
	return options >= 2 && other == 0
}

func isStartupLanguageSurface(activity string) bool {
	name := activitySimpleName(activity)
	if strings.Contains(name, "launcher") {
		return false
	}
	return !strings.Contains(name, "setting") && !strings.Contains(name, "language") && !strings.Contains(name, "locale")
}

func startupLanguageRank(value string) int {
	switch stableText(value) {
	case "english":
		return 2
	case "中文", "简体中文", "简體中文", "简体", "简體":
		return 1
	default:
		return 0
	}
}

func preferredStartupLanguage(actions []Action) (Action, bool) {
	if !isLanguagePickerPage(actions) {
		return Action{}, false
	}
	var selected Action
	rank := 0
	for _, action := range actions {
		if action.Type != "tap" || isVisibleBackControl(action) {
			continue
		}
		next := startupLanguageRank(action.Text)
		if descriptionRank := startupLanguageRank(action.Description); descriptionRank > next {
			next = descriptionRank
		}
		if next > rank {
			rank = next
			selected = action
		}
	}
	if rank == 0 {
		return Action{}, false
	}
	return selected, true
}

func startupLanguageConfirm(actions []Action) (Action, bool) {
	var selected Action
	rank := 0
	for _, action := range actions {
		if action.Type != "tap" || isVisibleBackControl(action) {
			continue
		}
		next := 0
		for _, value := range []string{action.Text, action.Description} {
			switch stableText(value) {
			case "continue", "next", "get started", "start exploring", "继续", "下一步":
				if next < 2 {
					next = 2
				}
			case "ok", "okay", "confirm", "done", "apply", "确定", "确认", "完成":
				if next < 1 {
					next = 1
				}
			}
		}
		if next > rank {
			rank = next
			selected = action
		}
	}
	if rank == 0 {
		return Action{}, false
	}
	return selected, true
}

func blocksLanguagePickerTap(activity string, actions []Action, action Action) bool {
	if action.Type == "swipe" || action.Type == "back" || isVisibleBackControl(action) {
		return false
	}
	options, other := languagePageCensus(actions)
	if options < 2 {
		return false
	}
	if other == 0 {
		if !isStartupLanguageSurface(activity) {
			return true
		}
		preferred, ok := preferredStartupLanguage(actions)
		return !ok || semanticActionKey(action) != semanticActionKey(preferred)
	}
	if isStartupLanguageSurface(activity) && (isLanguagePickerConfirm(action.Text) || isLanguagePickerConfirm(action.Description)) {
		return true
	}
	return isLanguageOptionLabel(action.Text) || isLanguageOptionLabel(action.Description)
}

func selectAction(node *graphNode, seed int64, enableScroll, recoverPopups bool) (Action, bool) {
	return selectActionBlocked(node, seed, enableScroll, recoverPopups, nil, nil, nil)
}

func selectActionBlocked(node *graphNode, seed int64, enableScroll, recoverPopups bool, blocked func(Action) bool, memory *activityMemory, visited map[string]struct{}) (Action, bool) {
	candidates := make([]Action, 0, len(node.analysis.Actions))
	for _, action := range node.analysis.Actions {
		if !node.tried[action.ID] && (action.Type != "swipe" || enableScroll) && (blocked == nil || !blocked(action)) {
			candidates = append(candidates, action)
		}
	}
	if recoverPopups {
		recovery := make([]Action, 0, len(candidates))
		for _, action := range candidates {
			if action.Recovery {
				recovery = append(recovery, action)
			}
		}
		if len(recovery) > 0 {
			candidates = recovery
		}
	}
	if node.nonScrollSelections >= maxNonScrollSelectionsBeforeScroll {
		scrolls := make([]Action, 0, len(candidates))
		for _, action := range candidates {
			if action.Type == "swipe" {
				scrolls = append(scrolls, action)
			}
		}
		if len(scrolls) > 0 {
			candidates = scrolls
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		leftWeight := actionWeight(node, candidates[i], recoverPopups, memory, visited)
		rightWeight := actionWeight(node, candidates[j], recoverPopups, memory, visited)
		leftRank := weightedActionRank(seed+int64(len(node.tried)), candidates[i].ID, leftWeight)
		rightRank := weightedActionRank(seed+int64(len(node.tried)), candidates[j].ID, rightWeight)
		return leftRank < rightRank
	})
	if len(candidates) == 0 {
		return Action{}, false
	}
	return candidates[0], true
}

func actionWeightForNode(node *graphNode, action Action, recoverPopups bool) float64 {
	return actionWeight(node, action, recoverPopups, nil, nil)
}

func actionWeight(node *graphNode, action Action, recoverPopups bool, memory *activityMemory, visited map[string]struct{}) float64 {
	if recoverPopups && action.Recovery {
		return 1000
	}
	weight := 100.0
	switch action.Type {
	case "swipe":
		weight = 80
		if node.nonScrollSelections >= maxNonScrollSelectionsBeforeScroll {
			weight = 220
		}
	case "input":
		weight = 45
	case "back":
		weight = 25
	}
	if isExternalRisk(action) {
		weight *= 0.25
	}
	weight /= math.Sqrt(float64(node.actionAttempts[action.ID] + 1))
	return weight * rememberedActionFactor(memory, visited, action)
}

func rememberedActionFactor(memory *activityMemory, visited map[string]struct{}, action Action) float64 {
	if memory == nil || action.Backtrack || action.Type == "back" {
		return 1
	}
	known, unseen := memory.unseenFraction(semanticActionKey(action), visited)
	if !known {
		return 2.1
	}
	if unseen > 0 {
		return 1 + unseen
	}
	return 0.55
}

func weightedActionRank(seed int64, id string, weight float64) float64 {
	value := actionRank(seed, id)
	uniform := (float64(value>>11) + 1) / (float64(uint64(1)<<53) + 1)
	return -math.Log(uniform) / math.Max(weight, 0.001)
}

func actionRank(seed int64, id string) uint64 {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d:%s", seed, id)))
	return binary.BigEndian.Uint64(sum[:8])
}
