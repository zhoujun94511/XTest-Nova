package main

import "testing"

func TestClassifyNormalizesJSONByFieldShape(t *testing.T) {
	firstKind, firstFields := classify([]byte(`{"name":"one","nested":{"count":1}}`))
	secondKind, secondFields := classify([]byte(`{"name":"two","nested":{"count":99}}`))
	if firstKind != "json" || secondKind != "json" || !equalStrings(firstFields, secondFields) {
		t.Fatalf("first=%s %#v second=%s %#v", firstKind, firstFields, secondKind, secondFields)
	}
}

func TestClassifyIgnoresJSONArrayLength(t *testing.T) {
	_, first := classify([]byte(`[{"id":1}]`))
	_, second := classify([]byte(`[{"id":1},{"id":2}]`))
	if !equalStrings(first, second) {
		t.Fatalf("first=%#v second=%#v", first, second)
	}
}
