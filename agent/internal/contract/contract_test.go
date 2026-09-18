package contract

import (
	"strings"
	"testing"
)

func TestParseAndCompare(t *testing.T) {
	input := "| # | Method | Route | Handler | Site |\n|---|---|---|---|---|\n| 1 | `GET, POST` | `/items/{id}` | x | x |"
	routes, err := ParseNexusMarkdown(strings.NewReader(input))
	if err != nil || len(routes) != 2 {
		t.Fatalf("routes=%v err=%v", routes, err)
	}
	report := Compare(routes, []Route{{"GET", "/items/{value}"}})
	if len(report.Covered) != 1 || len(report.Missing) != 1 {
		t.Fatalf("%+v", report)
	}
}

func TestCompareANYUsesImplementedMethodForPath(t *testing.T) {
	report := Compare(
		[]Route{{"ANY", "/stop"}},
		[]Route{{"POST", "/stop"}},
	)
	if len(report.Covered) != 1 || len(report.Missing) != 0 {
		t.Fatalf("%+v", report)
	}
}
