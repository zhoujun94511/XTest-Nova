package contract

import (
	"strings"
	"testing"
)

func TestSafeGoldenFixturesAreConcreteReadOnlyRequests(t *testing.T) {
	fixtures, err := SafeGoldenFixtures()
	if err != nil {
		t.Fatal(err)
	}
	if len(fixtures) < 10 {
		t.Fatalf("safe fixture count=%d", len(fixtures))
	}
	for _, fixture := range fixtures {
		if fixture.Method != "GET" || strings.Contains(fixture.Path, "{") {
			t.Fatalf("unsafe fixture: %#v", fixture)
		}
	}
}
