package exploration

import "testing"

const termsHierarchy = `<hierarchy><node package="com.example" class="android.widget.FrameLayout" bounds="[0,0][1080,1920]" enabled="true" visible-to-user="true"><node package="com.example" class="android.widget.TextView" text="By tapping Continue, you agree to our Terms of Service and Privacy Policy" bounds="[0,0][800,100]" enabled="true" visible-to-user="true"/><node package="com.example" class="android.view.View" bounds="[100,1600][980,1740]" clickable="true" enabled="true" visible-to-user="true"><node package="com.example" class="android.widget.TextView" text="Continue" bounds="[400,1630][680,1710]" clickable="false" enabled="true" visible-to-user="true"/></node></node></hierarchy>`

func TestConsentDefaultsToRecordedAcceptAction(t *testing.T) {
	policy := SpecialHandling{}
	if err := policy.normalize(); err != nil {
		t.Fatal(err)
	}
	event, found, err := inspectSpecial(termsHierarchy, "com.example", "com.example", policy)
	if err != nil || !found || event.Kind != "consent" || event.Policy != "accept" || event.Outcome != "action" || event.Action == nil || event.Action.SpecialKind != "consent" {
		t.Fatalf("event = %#v, found=%v, err=%v", event, found, err)
	}
}

func TestConsentPauseReturnsPolicyRequired(t *testing.T) {
	policy := SpecialHandling{ConsentPolicy: "pause"}
	if err := policy.normalize(); err != nil {
		t.Fatal(err)
	}
	event, found, err := inspectSpecial(termsHierarchy, "com.example", "com.example", policy)
	if err != nil || !found || event.Outcome != "policy_required" || event.Action != nil {
		t.Fatalf("event = %#v, found=%v, err=%v", event, found, err)
	}
}

func TestPermissionDefaultsToAllow(t *testing.T) {
	hierarchy := `<hierarchy><node package="com.android.permissioncontroller" class="android.widget.Button" text="While using the app" resource-id="com.android.permissioncontroller:id/permission_allow_foreground_only_button" bounds="[100,1200][980,1340]" clickable="true" enabled="true" visible-to-user="true"/></hierarchy>`
	policy := SpecialHandling{}
	if err := policy.normalize(); err != nil {
		t.Fatal(err)
	}
	event, found, err := inspectSpecial(hierarchy, "com.example", "com.android.permissioncontroller", policy)
	if err != nil || !found || event.Kind != "permission" || event.Policy != "allow" || event.Action == nil {
		t.Fatalf("event = %#v, found=%v, err=%v", event, found, err)
	}
}

func TestPaywallDefaultsToSafeExploration(t *testing.T) {
	hierarchy := `<hierarchy><node package="com.example" class="android.widget.TextView" text="Start your free trial subscription" bounds="[0,0][800,100]" enabled="true" visible-to-user="true"/><node package="com.example" class="android.widget.Button" text="Start trial" bounds="[0,1200][800,1320]" clickable="true" enabled="true" visible-to-user="true"/></hierarchy>`
	policy := SpecialHandling{}
	if err := policy.normalize(); err != nil {
		t.Fatal(err)
	}
	event, found, err := inspectSpecial(hierarchy, "com.example", "com.example", policy)
	if err != nil || !found || event.Kind != "paywall" || event.Outcome != "explore" {
		t.Fatalf("event = %#v, found=%v, err=%v", event, found, err)
	}
	analysis, err := Analyze(hierarchy, "com.example", Rules{})
	if err != nil {
		t.Fatal(err)
	}
	if len(analysis.Actions) != 1 || analysis.Actions[0].Text != "Start trial" {
		t.Fatalf("financial entry should remain explorable: %#v", analysis.Actions)
	}
}

func TestTargetOwnedPurchaseConfirmationIsDismissed(t *testing.T) {
	hierarchy := `<hierarchy><node package="com.example" class="android.widget.TextView" text="Order Info One-Time Purchase" bounds="[0,0][800,100]" enabled="true" visible-to-user="true"/><node package="com.example" class="android.widget.Button" text="Close" bounds="[0,120][200,220]" clickable="true" enabled="true" visible-to-user="true"/><node package="com.example" class="android.widget.Button" text="Pay Now" bounds="[0,300][800,420]" clickable="true" enabled="true" visible-to-user="true"/></hierarchy>`
	policy := SpecialHandling{}
	if err := policy.normalize(); err != nil {
		t.Fatal(err)
	}
	event, found, err := inspectSpecial(hierarchy, "com.example", "com.example", policy)
	if err != nil || !found || event.Kind != "purchase_confirmation" || event.Action == nil || event.Action.Text != "Close" {
		t.Fatalf("event=%#v found=%v err=%v", event, found, err)
	}
}

func TestPaywallTermsLinksAreNotMisclassifiedAsConsent(t *testing.T) {
	hierarchy := `<hierarchy><node package="com.example" class="android.widget.TextView" text="Unlock Ultimate Experience Free Trial Terms of Use Privacy Policy Subscription Terms" bounds="[0,0][900,300]" enabled="true" visible-to-user="true"/><node package="com.example" class="android.widget.Button" text="Continue" bounds="[0,1200][800,1320]" clickable="true" enabled="true" visible-to-user="true"/></hierarchy>`
	policy := SpecialHandling{}
	if err := policy.normalize(); err != nil {
		t.Fatal(err)
	}
	event, found, err := inspectSpecial(hierarchy, "com.example", "com.example", policy)
	if err != nil || !found || event.Kind != "paywall" || event.Outcome != "explore" {
		t.Fatalf("event = %#v, found=%v, err=%v", event, found, err)
	}
	analysis, err := Analyze(hierarchy, "com.example", Rules{DenyText: paywallContextDeniedText})
	if err != nil {
		t.Fatal(err)
	}
	if len(analysis.Actions) != 0 {
		t.Fatalf("paywall continuation was not filtered: %#v", analysis.Actions)
	}
}

func TestDocumentMentioningAdvertisementIsNotAnAdWithoutDismissControl(t *testing.T) {
	hierarchy := `<hierarchy><node package="com.example" class="android.webkit.WebView" text="Terms of Service and advertising disclosures" bounds="[0,0][1080,2200]" clickable="true" enabled="true" visible-to-user="true"/></hierarchy>`
	policy := SpecialHandling{}
	if err := policy.normalize(); err != nil {
		t.Fatal(err)
	}
	event, found, err := inspectSpecial(hierarchy, "com.example", "com.example", policy)
	if err != nil || found {
		t.Fatalf("document was classified as actionable special scene: %#v, found=%v, err=%v", event, found, err)
	}
}

func TestKnownAdActivityUsesSmallTopCornerControlWithoutLabel(t *testing.T) {
	hierarchy := `<hierarchy><node package="com.example" class="android.widget.FrameLayout" bounds="[0,0][1080,2400]" enabled="true" visible-to-user="true"><node package="com.example" class="android.widget.ImageView" bounds="[972,48][1056,132]" clickable="true" enabled="true" visible-to-user="true"/></node></hierarchy>`
	policy := SpecialHandling{}
	if err := policy.normalize(); err != nil {
		t.Fatal(err)
	}
	event, found, err := inspectSpecialActivity(hierarchy, "com.example", "com.example", "com.example/com.google.android.gms.ads.AdActivity", policy)
	if err != nil || !found || event.Kind != "ad" || event.Outcome != "action" || event.Action == nil || event.Action.Type != "tap" {
		t.Fatalf("event = %#v, found=%v, err=%v", event, found, err)
	}
	if event.Action.X < 900 || event.Action.Y > 200 {
		t.Fatalf("unexpected corner action: %#v", event.Action)
	}
}

func TestKnownAdActivityWaitsForCountdownBeforeBackFallback(t *testing.T) {
	hierarchy := `<hierarchy><node package="com.ad.sdk" class="android.widget.FrameLayout" bounds="[0,0][1080,2400]" enabled="true" visible-to-user="true"><node package="com.ad.sdk" class="android.widget.TextView" text="5" bounds="[990,60][1040,110]" enabled="true" visible-to-user="true"/></node></hierarchy>`
	policy := SpecialHandling{}
	if err := policy.normalize(); err != nil {
		t.Fatal(err)
	}
	event, found, err := inspectSpecialActivity(hierarchy, "com.example", "com.ad.sdk", "com.ad.sdk/com.mbridge.msdk.reward.player.MBRewardVideoActivity", policy)
	if err != nil || !found || event.Kind != "ad" || event.Outcome != "wait" || event.Action != nil || event.FallbackAction == nil || event.FallbackAction.Type != "back" {
		t.Fatalf("event = %#v, found=%v, err=%v", event, found, err)
	}
}

func TestOrdinaryPageDoesNotUseUnlabelledCornerControl(t *testing.T) {
	hierarchy := `<hierarchy><node package="com.example" class="android.widget.FrameLayout" bounds="[0,0][1080,2400]" enabled="true" visible-to-user="true"><node package="com.example" class="android.widget.ImageButton" bounds="[972,48][1056,132]" clickable="true" enabled="true" visible-to-user="true"/></node></hierarchy>`
	policy := SpecialHandling{}
	if err := policy.normalize(); err != nil {
		t.Fatal(err)
	}
	event, found, err := inspectSpecialActivity(hierarchy, "com.example", "com.example", "com.example/.MainActivity", policy)
	if err != nil || found {
		t.Fatalf("ordinary corner control was classified as an ad close: %#v, found=%v, err=%v", event, found, err)
	}
}

func TestAdWaitValidation(t *testing.T) {
	policy := SpecialHandling{AdMaxWaitSeconds: 181}
	if err := policy.normalize(); err == nil {
		t.Fatal("expected ad wait range validation")
	}
}

func TestPermissionControllerWaitsForDelayedControls(t *testing.T) {
	policy := SpecialHandling{Mode: specialModeSafe, PermissionPolicy: policyAllow}
	if err := policy.normalize(); err != nil {
		t.Fatal(err)
	}
	event, found, err := inspectSpecialActivity(`<hierarchy><node package="com.google.android.permissioncontroller" class="android.widget.FrameLayout" bounds="[0,0][1080,2400]"/></hierarchy>`, "com.example", "com.google.android.permissioncontroller", "com.google.android.permissioncontroller/.GrantPermissionsActivity", policy)
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if event.Kind != "permission" || event.Outcome != "wait" || event.WaitLimitSeconds != 5 {
		t.Fatalf("event=%+v", event)
	}
}

func TestShortsWaveWebAdUsesExplicitNonClickableCloseBar(t *testing.T) {
	hierarchy := `<hierarchy><node package="com.shorts.wave.drama" class="android.widget.FrameLayout" bounds="[0,0][1080,2340]" enabled="true" visible-to-user="true"><node package="com.shorts.wave.drama" class="android.webkit.WebView" bounds="[0,0][1080,2340]" enabled="true" visible-to-user="true"><node package="com.shorts.wave.drama" class="android.view.View" resource-id="close-button" bounds="[188,95][1080,196]" enabled="true" visible-to-user="true"><node package="com.shorts.wave.drama" class="android.widget.TextView" text="关闭广告并继续打开应用" bounds="[585,126][956,165]" enabled="true" visible-to-user="true"/></node><node package="com.shorts.wave.drama" class="android.view.View" resource-id="advertisement-card-wrapper" bounds="[0,81][1080,2306]" enabled="true" visible-to-user="true"><node package="com.shorts.wave.drama" class="android.widget.TextView" text="立即游玩" bounds="[820,1900][1020,1990]" enabled="true" visible-to-user="true"/></node></node></node></hierarchy>`
	policy := SpecialHandling{}
	if err := policy.normalize(); err != nil {
		t.Fatal(err)
	}
	event, found, err := inspectSpecialActivity(hierarchy, "com.shorts.wave.drama", "com.shorts.wave.drama", "com.shorts.wave.drama/com.google.android.gms.ads.AdActivity", policy)
	if err != nil || !found || event.AdType != adTypeAppOpen || event.AdPhase != adPhaseClose || event.Action == nil || event.Action.Type != "tap" {
		t.Fatalf("event = %#v, found=%v, err=%v", event, found, err)
	}
	if event.Action.X < 950 || event.Action.Y > 220 {
		t.Fatalf("close bar did not target its right-side X: %#v", event.Action)
	}
}

func TestRewardExitConfirmationSelectsGiveUpNotContinueWatching(t *testing.T) {
	hierarchy := `<hierarchy><node package="com.example" class="android.widget.FrameLayout" bounds="[0,0][1080,2400]" enabled="true" visible-to-user="true"><node package="com.example" class="android.widget.TextView" text="关闭将失去奖励，是否继续观看？" bounds="[100,800][980,1000]" enabled="true" visible-to-user="true"/><node package="com.example" class="android.widget.Button" text="继续观看" bounds="[100,1100][500,1250]" clickable="true" enabled="true" visible-to-user="true"/><node package="com.example" class="android.widget.Button" text="放弃奖励" bounds="[580,1100][980,1250]" clickable="true" enabled="true" visible-to-user="true"/></node></hierarchy>`
	policy := SpecialHandling{}
	if err := policy.normalize(); err != nil {
		t.Fatal(err)
	}
	event, found, err := inspectSpecialActivity(hierarchy, "com.example", "com.example", "com.example/com.mbridge.msdk.reward.player.MBRewardVideoActivity", policy)
	if err != nil || !found || event.AdType != adTypeRewarded || event.AdPhase != adPhaseConfirm || event.Action == nil || event.Action.Text != "放弃奖励" {
		t.Fatalf("event = %#v, found=%v, err=%v", event, found, err)
	}
}

func TestAdTypeWaitBudgets(t *testing.T) {
	if got := adWaitLimitSeconds(adTypeAppOpen, 75); got != 15 {
		t.Fatalf("app-open wait = %d", got)
	}
	if got := adWaitLimitSeconds(adTypeInterstitial, 75); got != 20 {
		t.Fatalf("interstitial wait = %d", got)
	}
	if got := adWaitLimitSeconds(adTypePlayable, 75); got != 45 {
		t.Fatalf("playable wait = %d", got)
	}
	if got := adWaitLimitSeconds(adTypeRewarded, 75); got != 75 {
		t.Fatalf("rewarded wait = %d", got)
	}
}

func TestEmbeddedBannerActionsAreMaskedWithoutBlockingPage(t *testing.T) {
	hierarchy := `<hierarchy><node package="com.example" class="android.widget.FrameLayout" bounds="[0,0][1080,2400]" enabled="true" visible-to-user="true"><node package="com.example" class="android.widget.Button" text="打开详情" resource-id="com.example:id/content" bounds="[40,200][500,340]" clickable="true" enabled="true" visible-to-user="true"/><node package="com.example" class="android.widget.FrameLayout" resource-id="com.example:id/banner_ad_container" bounds="[0,2100][1080,2400]" enabled="true" visible-to-user="true"><node package="com.example" class="android.widget.Button" text="立即安装" bounds="[700,2180][1040,2320]" clickable="true" enabled="true" visible-to-user="true"/></node></node></hierarchy>`
	analysis, err := Analyze(hierarchy, "com.example", Rules{})
	if err != nil {
		t.Fatal(err)
	}
	if len(analysis.Actions) != 1 || analysis.Actions[0].Text != "打开详情" {
		t.Fatalf("embedded ad action was not masked independently: %#v", analysis.Actions)
	}
}

func TestGooglePlayPurchaseConfirmationUsesBack(t *testing.T) {
	hierarchy := `<hierarchy><node package="com.android.vending" class="android.widget.TextView" text="Premium subscription $29.99" bounds="[0,0][900,300]" enabled="true" visible-to-user="true"/><node package="com.android.vending" class="android.widget.Button" text="Subscribe" bounds="[0,1200][800,1320]" clickable="true" enabled="true" visible-to-user="true"/></hierarchy>`
	policy := SpecialHandling{}
	if err := policy.normalize(); err != nil {
		t.Fatal(err)
	}
	event, found, err := inspectSpecial(hierarchy, "com.example", "com.android.vending", policy)
	if err != nil || !found || event.Kind != "purchase_confirmation" || event.Action == nil || event.Action.Type != "back" {
		t.Fatalf("event = %#v, found=%v, err=%v", event, found, err)
	}
}

func TestSystemPhotoPickerUsesBackWithoutSelectingUserMedia(t *testing.T) {
	hierarchy := `<hierarchy><node package="com.google.android.photopicker" class="android.widget.TextView" text="Select photos" bounds="[0,0][900,300]" enabled="true" visible-to-user="true"/><node package="com.google.android.photopicker" class="android.widget.ImageView" text="Vacation" bounds="[0,300][300,600]" clickable="true" enabled="true" visible-to-user="true"/></hierarchy>`
	policy := SpecialHandling{}
	if err := policy.normalize(); err != nil {
		t.Fatal(err)
	}
	event, found, err := inspectSpecial(hierarchy, "com.example", "com.google.android.photopicker", policy)
	if err != nil || !found || event.Kind != "system_picker" || event.Policy != "dismiss" || event.Action == nil || event.Action.Type != "back" {
		t.Fatalf("event = %#v, found=%v, err=%v", event, found, err)
	}
}

func TestExternalSystemSettingsUsesBackButRemainsExplorableAsTarget(t *testing.T) {
	hierarchy := `<hierarchy><node package="com.android.settings" class="android.widget.TextView" text="App notifications" bounds="[0,0][900,300]" enabled="true" visible-to-user="true"/><node package="com.android.settings" class="android.widget.Switch" text="Allow notifications" bounds="[0,300][900,500]" clickable="true" enabled="true" visible-to-user="true"/></hierarchy>`
	policy := SpecialHandling{}
	if err := policy.normalize(); err != nil {
		t.Fatal(err)
	}
	event, found, err := inspectSpecial(hierarchy, "com.example", "com.android.settings", policy)
	if err != nil || !found || event.Kind != "system_settings" || event.Action == nil || event.Action.Type != "back" {
		t.Fatalf("external event = %#v, found=%v, err=%v", event, found, err)
	}
	if event, found, err = inspectSpecial(hierarchy, "com.android.settings", "com.android.settings", policy); err != nil || found {
		t.Fatalf("target settings was treated as an external special scene: %#v, found=%v, err=%v", event, found, err)
	}
}

func TestAndroidResolverUsesBackOnlyWithResolverEvidence(t *testing.T) {
	hierarchy := `<hierarchy><node package="android" class="android.widget.TextView" text="打开方式" bounds="[0,0][900,300]" enabled="true" visible-to-user="true"/><node package="android" class="android.widget.Button" text="仅此一次" bounds="[0,800][400,1000]" clickable="true" enabled="true" visible-to-user="true"/></hierarchy>`
	policy := SpecialHandling{}
	if err := policy.normalize(); err != nil {
		t.Fatal(err)
	}
	event, found, err := inspectSpecial(hierarchy, "com.example", "android", policy)
	if err != nil || !found || event.Kind != "system_resolver" || event.Action == nil || event.Action.Type != "back" {
		t.Fatalf("resolver event = %#v, found=%v, err=%v", event, found, err)
	}
	ordinary := `<hierarchy><node package="android" class="android.widget.TextView" text="System information" bounds="[0,0][900,300]" enabled="true" visible-to-user="true"/></hierarchy>`
	if event, found, err = inspectSpecial(ordinary, "com.example", "android", policy); err != nil || found {
		t.Fatalf("ordinary android page was trusted: %#v, found=%v, err=%v", event, found, err)
	}
}

func TestGooglePlayBillingErrorIsDismissed(t *testing.T) {
	hierarchy := `<hierarchy><node package="com.android.vending" class="android.widget.TextView" text="此版本的应用未配置为通过 Google Play 结算" bounds="[0,0][900,300]" enabled="true" visible-to-user="true"/><node package="com.android.vending" class="android.widget.Button" text="知道了" bounds="[0,1200][800,1320]" clickable="true" enabled="true" visible-to-user="true"/></hierarchy>`
	policy := SpecialHandling{}
	if err := policy.normalize(); err != nil {
		t.Fatal(err)
	}
	event, found, err := inspectSpecial(hierarchy, "com.example", "com.android.vending", policy)
	if err != nil || !found || event.Kind != "billing_error" || event.Action == nil || event.Action.Type != "tap" {
		t.Fatalf("event = %#v, found=%v, err=%v", event, found, err)
	}
}

func TestGooglePlayReviewDialogIsDismissedButOtherStorePagesAreNotTrusted(t *testing.T) {
	hierarchy := `<hierarchy><node package="com.android.vending" class="android.widget.TextView" text="Reviews are only visible to developers" bounds="[0,0][800,100]" enabled="true" visible-to-user="true"/><node package="com.android.vending" class="android.widget.Button" text="Not now" bounds="[10,200][210,300]" clickable="true" enabled="true" visible-to-user="true"/></hierarchy>`
	policy := SpecialHandling{}
	if err := policy.normalize(); err != nil {
		t.Fatal(err)
	}
	event, found, err := inspectSpecial(hierarchy, "com.example", "com.android.vending", policy)
	if err != nil || !found || event.Kind != "review" || event.Policy != "dismiss" || event.Action == nil {
		t.Fatalf("event = %#v, found=%v, err=%v", event, found, err)
	}
	storePage := `<hierarchy><node package="com.android.vending" class="android.widget.TextView" text="Google Play" bounds="[0,0][800,100]" enabled="true" visible-to-user="true"/></hierarchy>`
	if event, found, err = inspectSpecial(storePage, "com.example", "com.android.vending", policy); err != nil || found {
		t.Fatalf("ordinary store page was trusted: %#v, found=%v, err=%v", event, found, err)
	}
}

func TestOnboardingFallsBackToHorizontalSwipe(t *testing.T) {
	hierarchy := `<hierarchy><node package="com.example" class="android.view.View" bounds="[0,0][1080,2400]" enabled="true" visible-to-user="true"><node package="com.example" class="android.widget.TextView" text="Skip" bounds="[900,40][1040,140]" enabled="true" visible-to-user="true"/><node package="com.example" class="android.widget.TextView" text="Welcome to the app" bounds="[100,500][900,700]" enabled="true" visible-to-user="true"/><node package="com.example" class="android.widget.TextView" text="Continue" bounds="[400,2100][680,2200]" enabled="true" visible-to-user="true"/></node></hierarchy>`
	policy := SpecialHandling{}
	if err := policy.normalize(); err != nil {
		t.Fatal(err)
	}
	event, found, err := inspectSpecial(hierarchy, "com.example", "com.example", policy)
	if err != nil || !found || event.Kind != "onboarding" || event.Action == nil || event.Action.Type != "swipe" || event.Action.Direction != "left" || event.Action.X <= event.Action.EndX {
		t.Fatalf("event = %#v, found=%v, err=%v", event, found, err)
	}
}

func TestForeignTransientWindowDoesNotChangeTargetFingerprint(t *testing.T) {
	withSystemUI := stringReplace(sampleHierarchy, `</hierarchy>`, `<node package="com.android.systemui" class="android.widget.TextView" text="Temporary notification 123" bounds="[0,0][200,50]" enabled="true" visible-to-user="true"/></hierarchy>`)
	withoutSystemUI := stringReplace(sampleHierarchy, `<node package="com.android.systemui" class="android.widget.Button" text="允许" bounds="[10,260][210,360]" clickable="true" enabled="true" visible-to-user="true"/>`, ``)
	withSystemUI = stringReplace(withSystemUI, `<node package="com.android.systemui" class="android.widget.Button" text="允许" bounds="[10,260][210,360]" clickable="true" enabled="true" visible-to-user="true"/>`, ``)
	a, err := Analyze(withSystemUI, "com.example", Rules{})
	if err != nil {
		t.Fatal(err)
	}
	b, err := Analyze(withoutSystemUI, "com.example", Rules{})
	if err != nil {
		t.Fatal(err)
	}
	if a.Fingerprint != b.Fingerprint {
		t.Fatalf("transient foreign window changed target fingerprint: %s != %s", a.Fingerprint, b.Fingerprint)
	}
}
