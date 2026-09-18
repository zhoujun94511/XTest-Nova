package runtimebundle

import "testing"

func TestEmbeddedBundleIsCompleteAndExcludesFixtures(t *testing.T) {
	components, err := (Embedded{}).Components()
	if err != nil {
		t.Fatal(err)
	}
	if len(components) != 4 {
		t.Fatalf("component count=%d", len(components))
	}
	for _, component := range components {
		if component.Package == "com.xtest.nova.fixture" || component.Name == "fixture" {
			t.Fatalf("fixture leaked into runtime bundle: %+v", component)
		}
		if _, err = (Embedded{}).Data(component.File); err != nil {
			t.Fatal(err)
		}
	}
}
