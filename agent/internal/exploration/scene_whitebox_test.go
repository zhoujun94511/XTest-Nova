package exploration

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestSceneWhitebox is the pre-build gate for dialogs that previously stuck a session.
func TestSceneWhitebox(t *testing.T) {
	t.Run("payment failure taps close and not feedback", func(t *testing.T) {
		event := mustPurchaseFailure(t, purchaseFailureHierarchy("90001"))
		if event.Action.ResourceID != "live.shorttv.apps:id/close_iv" || event.Action.Y > 1200 {
			t.Fatalf("action=%+v", event.Action)
		}
	})

	t.Run("changing failure text keeps one budget then leaves", func(t *testing.T) {
		device := &stableActivityDevice{activity: purchaseActivity}
		manager := New(device, device, device)
		now := time.Now().UTC()
		manager.state.Running = true
		manager.state.StartedAt = &now
		loop := &runLoop{pending: -1, specialAttempts: map[string]int{}}
		config := Config{Package: "live.shorttv.apps", DurationSeconds: 1800, IntervalMillis: 100, SpecialHandling: SpecialHandling{MaxAttempts: 2}}
		for _, code := range []string{"90001", "90002"} {
			signal := manager.handleSpecialScene(context.Background(), config, loop, "live.shorttv.apps", purchaseActivity, "", purchaseFailureHierarchy(code))
			if signal != loopContinue || !loop.purchaseBlocked || loop.allowColdStart {
				t.Fatalf("code=%s signal=%v blocked=%v cold=%v", code, signal, loop.purchaseBlocked, loop.allowColdStart)
			}
		}
		signal := manager.handleSpecialScene(context.Background(), config, loop, "live.shorttv.apps", purchaseActivity, "", purchaseFailureHierarchy("90003"))
		if signal != loopRestart || loop.allowColdStart {
			t.Fatalf("signal=%v cold=%v", signal, loop.allowColdStart)
		}
		joined := strings.Join(flattenCommands(device.commands), "\n")
		if strings.Count(joined, "input tap 940 933") != 2 || strings.Contains(joined, "input swipe") {
			t.Fatalf("commands=\n%s", joined)
		}
		cold, err := manager.restartExhaustedTarget(context.Background(), config, loop, purchaseActivity)
		if err != nil || cold {
			t.Fatalf("cold=%v err=%v", cold, err)
		}
		joined = strings.Join(flattenCommands(device.commands), "\n")
		if strings.Count(joined, "input keyevent BACK") != 1 || strings.Contains(joined, "input swipe") {
			t.Fatalf("commands=\n%s", joined)
		}
	})

	t.Run("dismissed payment failure does not tap the next purchase control", func(t *testing.T) {
		manager := New(nil, nil, nil)
		now := time.Now().UTC()
		manager.state.Running = true
		manager.state.StartedAt = &now
		analysis := Analysis{Fingerprint: "cashier", Actions: []Action{
			{ID: "weekly", Type: "tap", Description: "Weekly", ResourceID: "live.shorttv.apps:id/sku_view", Class: "android.view.ViewGroup", X: 300, Y: 1300},
			{ID: "inbox", Type: "tap", Description: "Inbox", ResourceID: "app:id/inbox", Class: "android.view.ViewGroup", X: 100, Y: 200},
		}}
		node := &graphNode{semantic: "cashier", analysis: analysis, tried: map[string]bool{}, actionAttempts: map[string]int{}}
		choice := manager.chooseActionLocked(Config{DurationSeconds: 1800, Seed: 1, EnableScroll: true}, node, analysis, purchaseActivity, &runLoop{purchaseBlocked: true})
		if choice.signal != loopNext || choice.action.ID != "inbox" {
			t.Fatalf("choice=%+v signal=%v", choice.action, choice.signal)
		}
	})

	t.Run("purchase-only page leaves without another buy", func(t *testing.T) {
		manager := New(nil, nil, nil)
		now := time.Now().UTC()
		manager.state.Running = true
		manager.state.StartedAt = &now
		analysis := Analysis{Fingerprint: "cashier", Actions: []Action{
			{ID: "weekly", Type: "tap", Description: "Weekly", ResourceID: "live.shorttv.apps:id/sku_view"},
			{ID: "coins", Type: "tap", Description: "5000", ResourceID: "live.shorttv.apps:id/coin_sku_view"},
		}}
		node := &graphNode{semantic: "cashier", analysis: analysis, tried: map[string]bool{}, actionAttempts: map[string]int{}}
		loop := &runLoop{purchaseBlocked: true, exhaustedFingerprint: "cashier", exhaustedCaptures: minimumExhaustedCapturesBeforeStop}
		manager.mu.Lock()
		choice := manager.chooseActionLocked(Config{DurationSeconds: 1800, Seed: 1, EnableScroll: true, MaxBacktracks: 20}, node, analysis, purchaseActivity, loop)
		manager.mu.Unlock()
		if choice.signal != loopRestart || loop.allowColdStart || choice.action.ResourceID != "" {
			t.Fatalf("signal=%v cold=%v action=%+v", choice.signal, loop.allowColdStart, choice.action)
		}
	})

	t.Run("executed tap is not reopened when the page fingerprint changes", func(t *testing.T) {
		manager := New(nil, nil, nil)
		now := time.Now().UTC()
		manager.state.Running = true
		manager.state.StartedAt = &now
		refresh := Action{ID: "refresh", Type: "tap", ResourceID: "com.example:id/refresh", Class: "android.widget.ImageView", X: 1000, Y: 400}
		back := Action{ID: "back", Type: "tap", ResourceID: "com.example:id/back", Class: "android.widget.ImageView", Description: "Back", X: 72, Y: 216}
		activity := "com.example/.ListActivity"
		first := Analysis{Fingerprint: "suggestions-a", Actions: []Action{refresh}}
		choice := manager.chooseActionLocked(Config{DurationSeconds: 1800, Seed: 1, EnableScroll: true, MaxBacktracks: 20}, &graphNode{semantic: "a", analysis: first, tried: map[string]bool{}, actionAttempts: map[string]int{}}, first, activity, &runLoop{})
		if choice.signal != loopNext || choice.action.ID != "refresh" {
			t.Fatalf("first=%+v", choice.action)
		}
		second := Analysis{Fingerprint: "suggestions-b", Actions: []Action{refresh, back}}
		choice = manager.chooseActionLocked(Config{DurationSeconds: 1800, Seed: 1, EnableScroll: true, MaxBacktracks: 20}, &graphNode{semantic: "b", analysis: second, tried: map[string]bool{}, actionAttempts: map[string]int{}}, second, activity, &runLoop{})
		if choice.signal != loopNext || choice.action.ID != "back" {
			t.Fatalf("repeat refresh was selected: %+v signal=%v", choice.action, choice.signal)
		}
	})

	t.Run("language switching stays out of the candidates", func(t *testing.T) {
		manager := New(nil, nil, nil)
		now := time.Now().UTC()
		manager.state.Running = true
		manager.state.StartedAt = &now
		activity := "com.example/.SettingsActivity"
		language := Action{ID: "language", Type: "tap", Text: "Language", ResourceID: "com.example:id/item"}
		notify := Action{ID: "notify", Type: "tap", Text: "Notifications", ResourceID: "com.example:id/notify"}
		analysis := Analysis{Fingerprint: "settings", Actions: []Action{language, notify}}
		choice := manager.chooseActionLocked(Config{DurationSeconds: 1800, Seed: 1, EnableScroll: true}, &graphNode{semantic: "settings", analysis: analysis, tried: map[string]bool{}, actionAttempts: map[string]int{}}, analysis, activity, &runLoop{})
		if choice.signal != loopNext || choice.action.ID != "notify" {
			t.Fatalf("language=%+v signal=%v", choice.action, choice.signal)
		}
		only := Analysis{Fingerprint: "settings", Actions: []Action{language}}
		loop := &runLoop{exhaustedFingerprint: "settings", exhaustedCaptures: minimumExhaustedCapturesBeforeStop}
		manager.mu.Lock()
		choice = manager.chooseActionLocked(Config{DurationSeconds: 1800, Seed: 1, EnableScroll: true, MaxBacktracks: 20}, &graphNode{semantic: "settings", analysis: only, tried: map[string]bool{}, actionAttempts: map[string]int{}}, only, activity, loop)
		manager.mu.Unlock()
		if choice.action.ID == "language" {
			t.Fatalf("language was selected with nothing else left: %+v", choice.action)
		}
		title := Action{ID: "title", Type: "tap", Text: "中文短剧", ResourceID: "com.example:id/title"}
		lesson := Action{ID: "lesson", Type: "tap", Text: "语言学习", ResourceID: "com.example:id/course"}
		idiom := Action{ID: "idiom", Type: "tap", Text: "body language", ResourceID: "com.example:id/idiom"}
		chip := Action{ID: "chip", Type: "tap", Text: "中文", ResourceID: "com.example.language:id/play"}
		show := Analysis{Fingerprint: "feed", Actions: []Action{title, lesson, idiom, chip}}
		for _, action := range show.Actions {
			if isLanguageSwitch(action) || isLanguageChoice("com.example.language/.MainActivity", action) {
				t.Fatalf("ordinary control blocked: %+v", action)
			}
		}
		choice = manager.chooseActionLocked(Config{DurationSeconds: 1800, Seed: 1, EnableScroll: true}, &graphNode{semantic: "feed", analysis: show, tried: map[string]bool{}, actionAttempts: map[string]int{}}, show, "com.example.language/.MainActivity", &runLoop{})
		if choice.signal != loopNext || choice.action.ID == "" {
			t.Fatalf("ordinary page had no candidate: %+v signal=%v", choice.action, choice.signal)
		}
		settingChip := Action{ID: "setting-chip", Type: "tap", Text: "中文", ResourceID: "com.example:id/value"}
		privacy := Action{ID: "privacy", Type: "tap", Text: "Privacy", ResourceID: "com.example:id/privacy"}
		settingPage := Analysis{Fingerprint: "settings-value", Actions: []Action{settingChip, privacy}}
		choice = manager.chooseActionLocked(Config{DurationSeconds: 1800, Seed: 1, EnableScroll: true}, &graphNode{semantic: "settings-value", analysis: settingPage, tried: map[string]bool{}, actionAttempts: map[string]int{}}, settingPage, "com.example/.SettingsActivity", &runLoop{})
		if choice.signal != loopNext || choice.action.ID != "privacy" {
			t.Fatalf("settings language value stayed explorable: %+v signal=%v", choice.action, choice.signal)
		}
		named := Action{ID: "named", Type: "tap", Text: "Open", ResourceID: "com.example:id/app_language"}
		if !isLanguageSwitch(named) || isLanguageSettingsActivity("com.example.language/.MainActivity") || !isLanguageSettingsActivity("com.example/.LanguageActivity") {
			t.Fatalf("scope named=%v package=%v class=%v", isLanguageSwitch(named), isLanguageSettingsActivity("com.example.language/.MainActivity"), isLanguageSettingsActivity("com.example/.LanguageActivity"))
		}
		option := Action{ID: "japanese", Type: "tap", Text: "日本語", ResourceID: "com.example:id/label"}
		picker := Analysis{Fingerprint: "picker", Actions: []Action{option, notify}}
		choice = manager.chooseActionLocked(Config{DurationSeconds: 1800, Seed: 1, EnableScroll: true}, &graphNode{semantic: "picker", analysis: picker, tried: map[string]bool{}, actionAttempts: map[string]int{}}, picker, "com.example/.LanguageActivity", &runLoop{})
		if choice.action.ID == "japanese" || choice.action.ID == "notify" {
			t.Fatalf("language page remained explorable: %+v", choice.action)
		}
		launchActivity := "com.example/.MainActivity"
		chinese := Action{ID: "zh", Type: "tap", Text: "中文", ResourceID: "com.example:id/zh"}
		english := Action{ID: "en", Type: "tap", Text: "English", ResourceID: "com.example:id/en"}
		confirm := Action{ID: "go", Type: "tap", Text: "继续", ResourceID: "com.example:id/next"}
		launch := Analysis{Fingerprint: "launch-language", Actions: []Action{chinese, english, confirm}}
		launchLoop := &runLoop{}
		choice = manager.chooseActionLocked(Config{DurationSeconds: 1800, Seed: 1, EnableScroll: true}, &graphNode{semantic: "launch-language", analysis: launch, tried: map[string]bool{}, actionAttempts: map[string]int{}}, launch, launchActivity, launchLoop)
		if choice.signal != loopNext || choice.action.ID != "en" {
			t.Fatalf("first-launch language page did not select English: %+v signal=%v", choice.action, choice.signal)
		}
		choice = manager.chooseActionLocked(Config{DurationSeconds: 1800, Seed: 1, EnableScroll: true}, &graphNode{semantic: "launch-language", analysis: launch, tried: map[string]bool{}, actionAttempts: map[string]int{}}, launch, launchActivity, launchLoop)
		if choice.action.ID == "go" {
			t.Fatalf("failed language tap was confirmed: %+v", choice.action)
		}
		launchLoop.startupLanguageAccepted = map[string]string{launchActivity: semanticActionKey(english)}
		choice = manager.chooseActionLocked(Config{DurationSeconds: 1800, Seed: 1, EnableScroll: true}, &graphNode{semantic: "launch-language", analysis: launch, tried: map[string]bool{}, actionAttempts: map[string]int{}}, launch, launchActivity, launchLoop)
		if choice.signal != loopNext || choice.action.ID != "go" {
			t.Fatalf("first-launch language page did not confirm the selected language: %+v signal=%v", choice.action, choice.signal)
		}
		logo := Action{ID: "logo", Type: "tap", ResourceID: "com.example:id/logo"}
		heading := Action{ID: "heading", Type: "tap", Text: "选择语言", ResourceID: "com.example:id/heading"}
		decorated := Analysis{Fingerprint: "launch-decorated", Actions: []Action{logo, heading, chinese, english, confirm}}
		choice = manager.chooseActionLocked(Config{DurationSeconds: 1800, Seed: 1, EnableScroll: true}, &graphNode{semantic: "launch-decorated", analysis: decorated, tried: map[string]bool{}, actionAttempts: map[string]int{}}, decorated, "com.example/.WelcomeActivity", &runLoop{})
		if choice.signal != loopNext || choice.action.ID != "en" {
			t.Fatalf("language page with a blank control did not select English: %+v signal=%v", choice.action, choice.signal)
		}
		simplified := Action{ID: "hans", Type: "tap", Text: "简体中文", ResourceID: "com.example:id/hans"}
		spanish := Action{ID: "es", Type: "tap", Text: "español", ResourceID: "com.example:id/es"}
		chineseOnly := Analysis{Fingerprint: "launch-chinese", Actions: []Action{simplified, spanish, confirm}}
		choice = manager.chooseActionLocked(Config{DurationSeconds: 1800, Seed: 1, EnableScroll: true}, &graphNode{semantic: "launch-chinese", analysis: chineseOnly, tried: map[string]bool{}, actionAttempts: map[string]int{}}, chineseOnly, launchActivity, &runLoop{})
		if choice.signal != loopNext || choice.action.ID != "hans" {
			t.Fatalf("first-launch language page did not select 中文: %+v signal=%v", choice.action, choice.signal)
		}
		settingsLaunch := Analysis{Fingerprint: "settings-language", Actions: []Action{chinese, english, confirm}}
		choice = manager.chooseActionLocked(Config{DurationSeconds: 1800, Seed: 1, EnableScroll: true}, &graphNode{semantic: "settings-language", analysis: settingsLaunch, tried: map[string]bool{}, actionAttempts: map[string]int{}}, settingsLaunch, "com.example/.SettingsActivity", &runLoop{})
		if choice.action.ID == "zh" || choice.action.ID == "en" || choice.action.ID == "go" {
			t.Fatalf("settings language page selected a language: %+v", choice.action)
		}
		play := Action{ID: "play", Type: "tap", Text: "Play", ResourceID: "com.example:id/play"}
		mixed := Analysis{Fingerprint: "filters", Actions: []Action{chinese, english, play}}
		choice = manager.chooseActionLocked(Config{DurationSeconds: 1800, Seed: 1, EnableScroll: true}, &graphNode{semantic: "filters", analysis: mixed, tried: map[string]bool{}, actionAttempts: map[string]int{}}, mixed, launchActivity, &runLoop{})
		if choice.signal != loopNext || choice.action.ID != "play" {
			t.Fatalf("content page blocked an ordinary control: %+v signal=%v", choice.action, choice.signal)
		}
		single := Analysis{Fingerprint: "one-name", Actions: []Action{chip, play}}
		if blocksLanguagePickerTap(launchActivity, single.Actions, chip) {
			t.Fatal("one language name blocked an ordinary page")
		}
		namedValue := Action{ID: "named-value", Type: "tap", Text: "中文", ResourceID: "com.example:id/app_language"}
		resourcePage := Analysis{Fingerprint: "resource-language", Actions: []Action{namedValue, play}}
		if !blocksNamedLanguageResource(launchActivity, resourcePage.Actions, namedValue) {
			t.Fatal("language resource stayed tappable outside a startup language page")
		}
		choice = manager.chooseActionLocked(Config{DurationSeconds: 1800, Seed: 1, EnableScroll: true}, &graphNode{semantic: "resource-language", analysis: resourcePage, tried: map[string]bool{}, actionAttempts: map[string]int{}}, resourcePage, "com.example/.FeedActivity", &runLoop{})
		if choice.signal != loopNext || choice.action.ID != "play" {
			t.Fatalf("language resource was selected: %+v signal=%v", choice.action, choice.signal)
		}
	})

	t.Run("external account action remains after other controls are used", func(t *testing.T) {
		manager := New(nil, nil, nil)
		now := time.Now().UTC()
		manager.state.Running = true
		manager.state.StartedAt = &now
		activity := "com.example/.MainActivity"
		ordinary := Action{ID: "alpha", Type: "tap", Description: "Alpha", Class: "android.view.ViewGroup"}
		login := Action{ID: "login", Type: "tap", Text: "Sign in", Class: "android.widget.Button"}
		first := Analysis{Fingerprint: "page-a", Actions: []Action{ordinary}}
		choice := manager.chooseActionLocked(Config{DurationSeconds: 1800, Seed: 1, EnableScroll: true}, &graphNode{semantic: "a", analysis: first, tried: map[string]bool{}, actionAttempts: map[string]int{}}, first, activity, &runLoop{})
		if choice.action.ID != "alpha" {
			t.Fatalf("first=%+v", choice.action)
		}
		second := Analysis{Fingerprint: "page-b", Actions: []Action{ordinary, login}}
		choice = manager.chooseActionLocked(Config{DurationSeconds: 1800, Seed: 1, EnableScroll: true}, &graphNode{semantic: "b", analysis: second, tried: map[string]bool{}, actionAttempts: map[string]int{}}, second, activity, &runLoop{})
		if choice.signal != loopNext || choice.action.ID != "login" {
			t.Fatalf("login was not deferred correctly: %+v signal=%v", choice.action, choice.signal)
		}
	})

	t.Run("a page that was just left is not backed into a loop", func(t *testing.T) {
		now := time.Unix(1_000, 0)
		action, until := nextSurfaceRecovery(now, time.Time{})
		if action != "back" || !until.Equal(now.Add(surfaceCooldown)) {
			t.Fatalf("first=%s until=%s", action, until)
		}
		action, until = nextSurfaceRecovery(now.Add(5*time.Second), until)
		if action != "wait" || !until.Equal(now.Add(surfaceCooldown)) {
			t.Fatalf("reentry=%s until=%s", action, until)
		}
		action, _ = nextSurfaceRecovery(now.Add(surfaceCooldown+time.Second), until)
		if action != "back" {
			t.Fatalf("after cooldown=%s", action)
		}
	})

	t.Run("handoff surfaces leave without exploring them", func(t *testing.T) {
		if !isHandoffSurface("com.android.intentresolver", "com.android.intentresolver/.ChooserActivity") || !isHandoffSurface("com.example", "com.example/.ShareSheetActivity") {
			t.Fatal("share and chooser surfaces were not recognized")
		}
		if isHandoffSurface("com.example", "com.example/.MainActivity") || isHandoffSurface("com.example", "com.example/.ListActivity") || isHandoffSurface("com.example.share", "com.example.share/.MainActivity") {
			t.Fatal("ordinary pages were treated as handoff surfaces")
		}
	})

	t.Run("shell dialog backs once then force-stops once", func(t *testing.T) {
		device := &scriptedDevice{}
		manager := New(device, device, device)
		loop := &runLoop{}
		cold, err := manager.restartExhaustedTarget(context.Background(), Config{Package: "live.shorttv.apps"}, loop, purchaseActivity)
		if err != nil || cold || loop.escapeAttempts != 1 {
			t.Fatalf("cold=%v err=%v attempts=%d", cold, err, loop.escapeAttempts)
		}
		cold, err = manager.restartExhaustedTarget(context.Background(), Config{Package: "live.shorttv.apps"}, loop, purchaseActivity)
		if err != nil || !cold {
			t.Fatalf("second cold=%v err=%v", cold, err)
		}
		cold, err = manager.restartExhaustedTarget(context.Background(), Config{Package: "live.shorttv.apps"}, loop, purchaseActivity)
		if err != nil || cold {
			t.Fatalf("third cold=%v err=%v", cold, err)
		}
		joined := strings.Join(flattenCommands(device.commands), "\n")
		if strings.Count(joined, "input keyevent BACK") != 1 || strings.Count(joined, "am force-stop live.shorttv.apps") != 1 || strings.Contains(joined, "input swipe") {
			t.Fatalf("commands=\n%s", joined)
		}
	})
}

func TestStallSceneCatalog(t *testing.T) {
	now := time.Now()
	until := now.Add(initialAsyncSettleDuration)
	var baseline pageGeneration
	released := false
	routing := pageGeneration{activity: "com.example/.RoutingActivity", windowVersion: "1"}
	if !observeStartup(now, until, routing.activity, &baseline, &released, routing) || released {
		t.Fatal("empty routing page was not held")
	}
	if isAsyncStartupActivity("com.android.launcher/.Launcher") || isAsyncStartupActivity("com.example/.MainActivity") || isAsyncStartupActivity("com.example/.LauncherActivity") || isAsyncStartupActivity("com.example.launch/.MainActivity") || isAsyncStartupActivity("com.example/.startup.ui.VipPaywallActivity") {
		t.Fatal("launcher, ordinary page, or inner class path was held as startup")
	}
	if !isAsyncStartupActivity("com.example/.SplashActivity") || !isAsyncStartupActivity("com.example/.LaunchActivity") {
		t.Fatal("splash or launch page was not a startup page")
	}
	for pkg := range systemLauncherPackages {
		if !isHomePackage(pkg) {
			t.Fatalf("system launcher %s was not treated as home", pkg)
		}
	}
	for _, surface := range []string{
		"com.android.launcher3/com.android.launcher3.uioverrides.QuickstepLauncher",
		"com.sec.android.app.launcher/com.sec.android.app.launcher.activities.LauncherActivity",
		"com.miui.home/com.miui.home.launcher.Launcher",
		"com.huawei.android.launcher/com.huawei.android.launcher.unihome.UniHomeLauncher",
		"com.bbk.launcher2/com.bbk.launcher2.Launcher",
		"com.transsion.XOSLauncher/com.android.launcher3.Launcher",
		"com.sonymobile.home/com.sonymobile.home.HomeActivity",
		"com.example/.LauncherActivity",
		"com.example/.QuickstepLauncher",
		"com.example/.HomeActivity",
	} {
		pkg, _, _ := strings.Cut(surface, "/")
		if !isHomeSurface(pkg, surface) {
			t.Fatalf("system launcher %s was not treated as home", surface)
		}
	}
	if isHomePackage("com.example") || isHomePackage("com.example.launcher.settings") || isHomePackage("com.example.launcherhelper") || isHomePackage("com.example.share") {
		t.Fatal("ordinary package was treated as home")
	}
	if isHomeSurface("com.example", "com.example/.MainActivity") || isHomeSurface("com.example", "com.example/.MyLauncherActivity") || isHomeSurface("com.example", "com.example/.ChooserActivityLauncher") {
		t.Fatal("launcher class name was classified from a package or a longer class name")
	}
	if coldStartAllowed("com.example/.MainActivity") || coldStartAllowed("com.example/.RoutingActivity") {
		t.Fatal("shell class was allowed to use the gesture ladder")
	}
	if !coldStartAllowed("com.example.routingactivity/.ListActivity") || !coldStartAllowed("com.example/.ListActivity") || !coldStartAllowed("com.example/.startup.ui.VipPaywallActivity") {
		t.Fatal("ordinary page was kept on the shell escape")
	}
	ad := "com.example/com.google.android.gms.ads.AdActivity"
	if isHandoffSurface("com.example", ad) || !isKnownAdActivity(ad) || isAsyncStartupActivity(ad) {
		t.Fatal("ad playback was classified as a stall surface")
	}
	if !isHandoffSurface("com.example", "com.example/.ShareActivity") || isAsyncStartupActivity("com.example/.ShareActivity") {
		t.Fatal("share handoff was not recognized")
	}
	action, quietUntil := nextSurfaceRecovery(now, time.Time{})
	if action != "back" {
		t.Fatalf("first handoff recovery=%s", action)
	}
	if action, _ = nextSurfaceRecovery(now.Add(time.Second), quietUntil); action != "wait" {
		t.Fatalf("handoff reentry=%s", action)
	}
	guard := &cycleGuard{}
	period := 0
	for iteration := 0; iteration < 3; iteration++ {
		period = guard.observe("page-a", "open", "page-b", "page-a")
		period = guard.observe("page-b", "back", "page-a", "page-b")
	}
	if period != 2 {
		t.Fatalf("two-page ping-pong period=%d", period)
	}
}

const purchaseActivity = "live.shorttv.apps/com.startshorts.androidplayer.ui.activity.MainActivity"

type stableActivityDevice struct {
	scriptedDevice
	activity string
}

func (d *stableActivityDevice) ForegroundActivity(context.Context) (string, error) {
	return d.activity, nil
}

func purchaseFailureHierarchy(code string) string {
	return `<hierarchy><node package="live.shorttv.apps" class="android.widget.FrameLayout" bounds="[144,861][1012,1745]" enabled="true" visible-to-user="true"><node package="live.shorttv.apps" class="android.widget.ImageView" resource-id="live.shorttv.apps:id/close_iv" content-desc="Close" bounds="[868,861][1012,1005]" clickable="true" enabled="true" visible-to-user="true"/><node package="live.shorttv.apps" class="android.widget.TextView" text="支付失败 ` + code + `" bounds="[216,1278][940,1357]" enabled="true" visible-to-user="true"/><node package="live.shorttv.apps" class="android.widget.TextView" text="反馈" resource-id="live.shorttv.apps:id/feedback_button" bounds="[216,1565][940,1673]" clickable="true" enabled="true" visible-to-user="true"/></node></hierarchy>`
}

func mustPurchaseFailure(t *testing.T, hierarchy string) SpecialEvent {
	t.Helper()
	policy := SpecialHandling{}
	if err := policy.normalize(); err != nil {
		t.Fatal(err)
	}
	event, found, err := inspectSpecialActivity(hierarchy, "live.shorttv.apps", "live.shorttv.apps", purchaseActivity, policy)
	if err != nil || !found || event.Kind != "purchase_failure" || event.Outcome != "action" || event.Action == nil {
		t.Fatalf("event=%#v found=%v err=%v", event, found, err)
	}
	return event
}
