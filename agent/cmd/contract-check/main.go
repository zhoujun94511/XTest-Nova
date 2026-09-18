package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/zhoujun94511/xtest-nova/agent/internal/contract"
)

const expectedMethodRoutes = 77

func main() {
	reference := flag.String("reference", "", "path to the historical reference HTTP contract")
	document := flag.String("document", "", "Path to Nova HTTP contract documentation")
	jsonOutput := flag.Bool("json", false, "Print JSON report")
	semanticsJSON := flag.Bool("semantics-json", false, "Print all request/response semantic contracts as JSON")
	flag.Parse()
	if *reference == "" {
		fatal(fmt.Errorf("-reference is required"))
	}
	target, err := parseRoutes(*reference)
	if err != nil {
		fatal(err)
	}
	if len(target) != expectedMethodRoutes {
		fatal(fmt.Errorf("reference contract has %d method-routes, want %d", len(target), expectedMethodRoutes))
	}
	implemented := contract.Implemented()
	if len(implemented) != expectedMethodRoutes {
		fatal(fmt.Errorf("implemented contract has %d method-routes, want %d", len(implemented), expectedMethodRoutes))
	}
	report := contract.Compare(target, implemented)
	semantics, err := contract.Semantics()
	if err != nil {
		fatal(err)
	}
	if len(semantics) != expectedMethodRoutes {
		fatal(fmt.Errorf("semantic contract has %d method-routes, want %d", len(semantics), expectedMethodRoutes))
	}
	if *semanticsJSON {
		if err = json.NewEncoder(os.Stdout).Encode(semantics); err != nil {
			fatal(err)
		}
		return
	}
	if *document != "" {
		documented, parseErr := parseRoutes(*document)
		if parseErr != nil {
			fatal(parseErr)
		}
		missingFromDocument := contract.Compare(target, documented).Missing
		extraInDocument := contract.Compare(documented, target).Missing
		if len(missingFromDocument) != 0 || len(extraInDocument) != 0 {
			fatal(fmt.Errorf("contract document mismatch: missing=%v extra=%v", missingFromDocument, extraInDocument))
		}
	}
	if *jsonOutput {
		data, err := report.JSON()
		if err != nil {
			fatal(err)
		}
		if _, err = os.Stdout.Write(append(data, '\n')); err != nil {
			fatal(err)
		}
	} else {
		writef("target method-routes: %d\nimplemented: %d\ncovered: %d\nmissing: %d\nsemantic contracts: %d\n", report.TargetCount, report.ImplementedCount, len(report.Covered), len(report.Missing), len(semantics))
		if *document != "" {
			writef("documented: %d\ndocument mismatch: 0\n", report.TargetCount)
		}
		for _, route := range report.Missing {
			writef("MISSING %-7s %s\n", route.Method, route.Path)
		}
	}
	if len(report.Missing) != 0 {
		fatal(fmt.Errorf("contract coverage incomplete: %d route(s) missing", len(report.Missing)))
	}
}

func parseRoutes(path string) ([]contract.Route, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	routes, parseErr := contract.ParseNexusMarkdown(file)
	closeErr := file.Close()
	if parseErr != nil {
		return nil, parseErr
	}
	return routes, closeErr
}

func writef(format string, values ...any) {
	if _, err := fmt.Printf(format, values...); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	_, _ = fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
