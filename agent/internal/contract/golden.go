package contract

import "strings"

type GoldenFixture struct {
	Method         string   `json:"method"`
	Path           string   `json:"path"`
	ResponseKind   string   `json:"responseKind"`
	ResponseFields []string `json:"responseFields,omitempty"`
	RuntimeGate    string   `json:"runtimeGate"`
}

// SafeGoldenFixtures returns only concrete, read-only, non-streaming requests.
// Stateful and destructive routes require an isolated-device fixture instead.
func SafeGoldenFixtures() ([]GoldenFixture, error) {
	specs, err := Semantics()
	if err != nil {
		return nil, err
	}
	result := make([]GoldenFixture, 0)
	for _, spec := range specs {
		if (spec.Method != "GET" && spec.Method != "ANY") || spec.SideEffect != "read" || strings.Contains(spec.Path, "{") {
			continue
		}
		switch spec.ResponseKind {
		case "json", "json-array", "text", "image/png", "image/jpeg":
			result = append(result, GoldenFixture{"GET", spec.Path, spec.ResponseKind, spec.ResponseFields, spec.RuntimeGate})
		}
	}
	return result, nil
}
