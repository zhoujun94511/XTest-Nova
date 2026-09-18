package contract

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func repositorySource(t *testing.T, path ...string) string {
	t.Helper()
	parts := append([]string{"..", "..", ".."}, path...)
	content, err := os.ReadFile(filepath.Join(parts...))
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func requireSourceMarkers(t *testing.T, name, source string, markers ...string) {
	t.Helper()
	for _, marker := range markers {
		if !strings.Contains(source, marker) {
			t.Fatalf("%s is missing contract marker %q", name, marker)
		}
	}
}

func TestCompanionLauncherAndProviderManifestBoundary(t *testing.T) {
	manifest := repositorySource(t, "companion", "app", "src", "main", "AndroidManifest.xml")
	if err := xml.Unmarshal([]byte(manifest), new(struct{})); err != nil {
		t.Fatalf("Companion manifest is not valid XML: %v", err)
	}
	requireSourceMarkers(t, "Companion manifest", manifest,
		`android:name="com.openatx.xtest.popup.PopupLauncherActivity"`,
		`android:exported="true"`,
		`android:noHistory="true"`,
		`android:theme="@android:style/Theme.NoDisplay"`,
		`android:name="com.openatx.xtest.popup.OverlayService" android:exported="false"`,
		`android:readPermission="android.permission.DUMP"`,
	)
	launcher := repositorySource(t, "companion", "app", "src", "main", "java", "com", "openatx", "xtest", "popup", "PopupLauncherActivity.java")
	requireSourceMarkers(t, "Companion launcher", launcher,
		"startForegroundService(service)",
		"new Intent(this, OverlayService.class)",
		"finish();",
	)
}

func TestValidationFixtureKeepsExplicitExternalEntryPoints(t *testing.T) {
	manifest := repositorySource(t, "fixtures", "validation-app", "AndroidManifest.xml")
	if err := xml.Unmarshal([]byte(manifest), new(struct{})); err != nil {
		t.Fatalf("fixture manifest is not valid XML: %v", err)
	}
	requireSourceMarkers(t, "fixture manifest", manifest,
		`package="com.xtest.nova.fixture"`,
		`android:name=".ValidationActivity" android:exported="true"`,
		`android:name="android.intent.action.MAIN"`,
		`android:name="android.intent.category.LAUNCHER"`,
		`android:name=".FaultScenarioActivity" android:exported="true"`,
		`android:name=".LifecycleScenarioActivity" android:exported="true"`,
	)
}

func TestAccessibilityWindowRecyclingSourceBoundary(t *testing.T) {
	instrumentation := repositorySource(t, "uiautomator", "test", "src", "main", "java", "com", "openatx", "xtest", "nova", "uiautomator", "test", "NovaInstrumentation.java")
	requireSourceMarkers(t, "NovaInstrumentation", instrumentation,
		"finally {",
		"recycleWindowIfRequired(window);",
		"AccessibilityRecyclingPolicy.shouldRecycleWindow(Build.VERSION.SDK_INT)",
	)
	policy := repositorySource(t, "uiautomator", "test", "src", "main", "java", "com", "openatx", "xtest", "nova", "uiautomator", "test", "AccessibilityRecyclingPolicy.java")
	requireSourceMarkers(t, "accessibility recycling policy", policy,
		"return sdkInt >= 28 && sdkInt < 33;",
		"shouldRecycleWindow(33)",
	)
}
