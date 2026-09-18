package exploration

import "testing"

const sampleHierarchy = `<hierarchy rotation="0"><node package="com.example" class="android.widget.FrameLayout" bounds="[0,0][1080,1920]" clickable="false" enabled="true"><node package="com.example" class="android.widget.Button" resource-id="com.example:id/safe" text="继续" bounds="[10,20][210,120]" clickable="true" enabled="true" visible-to-user="true"/><node package="com.example" class="android.widget.Button" resource-id="com.example:id/delete" text="删除账户" bounds="[10,140][210,240]" clickable="true" enabled="true" visible-to-user="true"/><node package="com.android.systemui" class="android.widget.Button" text="允许" bounds="[10,260][210,360]" clickable="true" enabled="true" visible-to-user="true"/></node></hierarchy>`

func TestAnalyzeFiltersDangerousAndForeignNodes(t *testing.T) {
	analysis, err := Analyze(sampleHierarchy, "com.example", Rules{})
	if err != nil {
		t.Fatal(err)
	}
	if analysis.NodeCount != 4 || analysis.Filtered != 2 || len(analysis.Actions) != 1 {
		t.Fatalf("analysis = %#v", analysis)
	}
	if action := analysis.Actions[0]; action.Text != "继续" || action.X != 110 || action.Y != 70 {
		t.Fatalf("action = %#v", action)
	}
}

func TestAnalyzeKeepsRechargeAndPaymentProviderEntrypointsExplorable(t *testing.T) {
	hierarchy := `<hierarchy><node package="com.example" class="android.widget.Button" resource-id="com.example:id/top_up" text="Top Up" bounds="[0,0][200,100]" clickable="true" enabled="true" visible-to-user="true"/><node package="com.example" class="android.view.ViewGroup" resource-id="com.example:id/cl_stripe" bounds="[0,120][200,220]" clickable="true" enabled="true" visible-to-user="true"/><node package="com.example" class="android.widget.Button" text="继续浏览" bounds="[0,240][200,340]" clickable="true" enabled="true" visible-to-user="true"/></hierarchy>`
	analysis, err := Analyze(hierarchy, "com.example", Rules{})
	if err != nil {
		t.Fatal(err)
	}
	if len(analysis.Actions) != 3 || analysis.Filtered != 0 {
		t.Fatalf("analysis=%#v", analysis)
	}
}

func TestAnalyzeBlocksInstallAndDownloadCallsToAction(t *testing.T) {
	hierarchy := `<hierarchy><node package="com.example" class="android.widget.Button" text="安装" bounds="[0,0][200,100]" clickable="true" enabled="true" visible-to-user="true"/><node package="com.example" class="android.widget.Button" resource-id="com.example:id/download_app" text="Continue" bounds="[0,120][200,220]" clickable="true" enabled="true" visible-to-user="true"/><node package="com.example" class="android.widget.Button" text="开始游戏" bounds="[0,240][200,340]" clickable="true" enabled="true" visible-to-user="true"/></hierarchy>`
	analysis, err := Analyze(hierarchy, "com.example", Rules{})
	if err != nil {
		t.Fatal(err)
	}
	if len(analysis.Actions) != 1 || analysis.Actions[0].Text != "开始游戏" || analysis.Filtered != 2 {
		t.Fatalf("analysis=%#v", analysis)
	}
}

func TestFingerprintScrubsDynamicDigits(t *testing.T) {
	first, err := Analyze(sampleHierarchy, "com.example", Rules{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := Analyze(stringReplace(sampleHierarchy, "继续", "继续 12345"), "com.example", Rules{})
	if err != nil {
		t.Fatal(err)
	}
	third, err := Analyze(stringReplace(sampleHierarchy, "继续", "继续 67890"), "com.example", Rules{})
	if err != nil {
		t.Fatal(err)
	}
	if second.Fingerprint != third.Fingerprint || first.Fingerprint == "" {
		t.Fatalf("unstable fingerprints: %s %s %s", first.Fingerprint, second.Fingerprint, third.Fingerprint)
	}
}

func TestAllowList(t *testing.T) {
	analysis, err := Analyze(sampleHierarchy, "com.example", Rules{AllowResourcePrefixes: []string{"com.example:id/other"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(analysis.Actions) != 0 {
		t.Fatalf("actions = %#v", analysis.Actions)
	}
}

func TestScrollableNodeCreatesBoundedForwardSwipe(t *testing.T) {
	hierarchy := `<hierarchy><node package="com.example" class="android.widget.ScrollView" resource-id="com.example:id/list" bounds="[0,100][1080,1800]" scrollable="true" enabled="true" visible-to-user="true"/></hierarchy>`
	analysis, err := Analyze(hierarchy, "com.example", Rules{})
	if err != nil {
		t.Fatal(err)
	}
	if len(analysis.Actions) != 1 {
		t.Fatalf("actions = %#v", analysis.Actions)
	}
	action := analysis.Actions[0]
	if action.Type != "swipe" || action.Direction != "forward" || action.Y <= action.EndY || action.X != action.EndX {
		t.Fatalf("swipe = %#v", action)
	}
}

func TestWebViewContainerIsNotClickedButAccessibleChildrenRemain(t *testing.T) {
	hierarchy := `<hierarchy><node package="com.example" class="android.webkit.WebView" bounds="[0,0][1080,1800]" clickable="true" enabled="true" visible-to-user="true"><node package="com.example" class="android.widget.Button" text="Learn more" bounds="[20,100][300,200]" clickable="true" enabled="true" visible-to-user="true"/></node></hierarchy>`
	analysis, err := Analyze(hierarchy, "com.example", Rules{})
	if err != nil {
		t.Fatal(err)
	}
	if len(analysis.Actions) != 1 || analysis.Actions[0].Text != "Learn more" {
		t.Fatalf("actions = %#v", analysis.Actions)
	}
}

func TestSafePopupActionIsMarkedForRecovery(t *testing.T) {
	hierarchy := `<hierarchy><node package="com.example" class="android.widget.Button" text="稍后" bounds="[0,0][200,100]" clickable="true" enabled="true" visible-to-user="true"/></hierarchy>`
	analysis, err := Analyze(hierarchy, "com.example", Rules{})
	if err != nil {
		t.Fatal(err)
	}
	if len(analysis.Actions) != 1 || !analysis.Actions[0].Recovery {
		t.Fatalf("recovery action = %#v", analysis.Actions)
	}
}

func TestEditableFieldCreatesBoundedInputCorpusAndNormalizesProbeFingerprint(t *testing.T) {
	empty := `<hierarchy><node package="com.example" class="android.widget.EditText" resource-id="com.example:id/search" text="" bounds="[20,100][900,220]" clickable="true" focusable="true" enabled="true" visible-to-user="true" password="false"/></hierarchy>`
	strategy := InputStrategy{CasesPerField: maxInputCasesPerField, MaxLength: defaultInputMaxLength}
	analysis, err := AnalyzeWithInputStrategy(empty, "com.example", Rules{}, 0, strategy)
	if err != nil {
		t.Fatal(err)
	}
	if analysis.InputFields != 1 || len(analysis.Actions) != maxInputCasesPerField {
		t.Fatalf("analysis = %#v", analysis)
	}
	expectedKinds := map[string]bool{"empty": true, "ascii_min": true, "ascii_boundary_minus_1": true, "ascii_boundary": true, "ascii_boundary_plus_1": true, "cjk": true, "symbols": true, "emoji": true, "mixed": true, "numeric_decimal": true, "email_valid": true, "email_invalid": true}
	seen := map[string]bool{}
	for index, action := range analysis.Actions {
		if action.Type != "input" || action.InputKind == "" || seen[action.InputKind] {
			t.Fatalf("input action %d = %#v", index, action)
		}
		seen[action.InputKind] = true
	}
	for kind := range expectedKinds {
		if !seen[kind] {
			t.Fatalf("missing equivalence class %s", kind)
		}
	}
	firstProbe := stringReplace(empty, `text=""`, `text="Nova123"`)
	secondProbe := stringReplace(empty, `text=""`, `text="测试"`)
	first, _ := AnalyzeWithInputStrategy(firstProbe, "com.example", Rules{}, 0, strategy)
	second, _ := AnalyzeWithInputStrategy(secondProbe, "com.example", Rules{}, 0, strategy)
	if first.Fingerprint != second.Fingerprint {
		t.Fatalf("probe values created distinct fingerprints: %s != %s", first.Fingerprint, second.Fingerprint)
	}
	if len(first.Actions) != len(second.Actions) {
		t.Fatalf("probe value changed corpus size: %d != %d", len(first.Actions), len(second.Actions))
	}
	for index := range first.Actions {
		if first.Actions[index].ID != second.Actions[index].ID || first.Actions[index].Text != second.Actions[index].Text {
			t.Fatalf("probe value changed generated corpus at %d", index)
		}
	}
	shrunk, _ := AnalyzeWithInputStrategy(stringReplace(firstProbe, `[20,100][900,220]`, `[20,100][805,220]`), "com.example", Rules{}, 0, strategy)
	ids := func(value Analysis) map[string]bool {
		result := map[string]bool{}
		for _, action := range value.Actions {
			result[action.ID] = true
		}
		return result
	}
	if len(shrunk.Actions) != len(first.Actions) || len(ids(shrunk)) != len(ids(first)) {
		t.Fatalf("field resize changed input corpus: %#v %#v", first.Actions, shrunk.Actions)
	}
	for id := range ids(first) {
		if !ids(shrunk)[id] {
			t.Fatalf("field resize changed input action id %s", id)
		}
	}
	shifted, _ := AnalyzeWithInputStrategy(stringReplace(firstProbe, `[20,100][900,220]`, `[20,500][900,620]`), "com.example", Rules{}, 0, strategy)
	for id := range ids(first) {
		if !ids(shifted)[id] {
			t.Fatalf("viewport movement changed anchored input action id %s", id)
		}
	}
}

func TestGeneratedInputCorpusIsSeededBoundedAndReproducible(t *testing.T) {
	strategy := InputStrategy{CasesPerField: maxInputCasesPerField, MaxLength: 64}
	first := generateInputProbes(42, strategy, "search-field")
	repeated := generateInputProbes(42, strategy, "search-field")
	otherSeed := generateInputProbes(43, strategy, "search-field")
	if len(first) != maxInputCasesPerField || len(repeated) != len(first) || len(otherSeed) != len(first) {
		t.Fatalf("unexpected corpus sizes: %d %d %d", len(first), len(repeated), len(otherSeed))
	}
	different := false
	lengths := map[string]int{}
	for index := range first {
		if first[index] != repeated[index] {
			t.Fatalf("same seed was not reproducible at %d", index)
		}
		if first[index].Text != otherSeed[index].Text {
			different = true
		}
		if len([]rune(first[index].Text)) > strategy.MaxLength {
			t.Fatalf("probe exceeded max length: %#v", first[index])
		}
		lengths[first[index].Kind] = len([]rune(first[index].Text))
	}
	if !different {
		t.Fatal("different seeds generated the same corpus")
	}
	if lengths["empty"] != 0 || lengths["ascii_min"] != 1 || lengths["ascii_boundary_minus_1"] != 31 || lengths["ascii_boundary"] != 32 || lengths["ascii_boundary_plus_1"] != 33 || lengths["ascii_max"] != 64 {
		t.Fatalf("boundary lengths = %#v", lengths)
	}
	for _, probe := range generateInputProbes(42, InputStrategy{CasesPerField: maxInputCasesPerField, MaxLength: 4}, "small-field") {
		if len([]rune(probe.Text)) > 4 {
			t.Fatalf("small max length was ignored: %#v", probe)
		}
	}
}

func TestSensitiveEditableFieldsAreExcluded(t *testing.T) {
	hierarchy := `<hierarchy><node package="com.example" class="android.widget.EditText" resource-id="com.example:id/password" bounds="[0,0][500,100]" clickable="true" enabled="true" visible-to-user="true" password="true"/><node package="com.example" class="android.widget.EditText" resource-id="com.example:id/otp_code" bounds="[0,120][500,220]" clickable="true" enabled="true" visible-to-user="true" password="false"/></hierarchy>`
	analysis, err := Analyze(hierarchy, "com.example", Rules{})
	if err != nil {
		t.Fatal(err)
	}
	if analysis.InputFields != 0 || len(analysis.Actions) != 0 {
		t.Fatalf("sensitive inputs escaped filtering: %#v", analysis)
	}
}

func TestMinorScrollableRegionIsFiltered(t *testing.T) {
	hierarchy := `<hierarchy><node package="com.example" class="android.widget.ScrollView" bounds="[0,0][1080,2000]" scrollable="true" enabled="true" visible-to-user="true"><node package="com.example" class="android.view.View" bounds="[0,1700][1080,2000]" scrollable="true" enabled="true" visible-to-user="true"/></node></hierarchy>`
	analysis, err := Analyze(hierarchy, "com.example", Rules{})
	if err != nil {
		t.Fatal(err)
	}
	if len(analysis.Actions) != 1 || analysis.Actions[0].Bounds.Top != 0 {
		t.Fatalf("scroll targets = %#v", analysis.Actions)
	}
}

func stringReplace(value, old, replacement string) string {
	for i := 0; i+len(old) <= len(value); i++ {
		if value[i:i+len(old)] == old {
			return value[:i] + replacement + value[i+len(old):]
		}
	}
	return value
}
