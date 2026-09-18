package contract

import "testing"

func TestEveryImplementedRouteHasCompleteSemantics(t *testing.T) {
	specs, err := Semantics()
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 77 {
		t.Fatalf("semantic contracts=%d, want 77", len(specs))
	}
	if len(SemanticKeys(specs)) != 77 {
		t.Fatal("semantic key count mismatch")
	}
	for _, spec := range specs {
		if spec.ResponseKind == "json" && len(spec.ResponseFields) == 0 {
			t.Fatalf("%s %s has no response fields", spec.Method, spec.Path)
		}
	}
}

func TestControlAndNotFoundSemantics(t *testing.T) {
	specs, err := Semantics()
	if err != nil {
		t.Fatal(err)
	}
	for _, spec := range specs {
		switch spec.Path {
		case "/stop":
			if spec.Method != "POST" || len(spec.RequestHeaders) != 1 || spec.RequestHeaders[0] != "X-XTest-Control: true" {
				t.Fatalf("stop semantics=%#v", spec)
			}
		case "/pushConfig":
			if len(spec.RequestHeaders) != 1 || spec.RequestHeaders[0] != "X-XTest-Control: true" {
				t.Fatalf("pushConfig semantics=%#v", spec)
			}
		case "/packages/{id}":
			if !containsStatus(spec.ErrorStatuses, 404) {
				t.Fatalf("package task semantics=%#v", spec)
			}
		}
	}
}

func containsStatus(values []int, expected int) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
