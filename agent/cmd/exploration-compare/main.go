package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/zhoujun94511/xtest-nova/agent/internal/evaluation"
	"github.com/zhoujun94511/xtest-nova/agent/internal/runner"
)

func main() {
	baselinePath := flag.String("baseline", "", "baseline JSON report")
	candidatePath := flag.String("candidate", "", "candidate JSON report")
	nexusLog := flag.String("nexus-log", "", "legacy Nexus text log to import")
	novaRun := flag.String("nova-run", "", "Nova persisted run manifest to import")
	scenarioFile := flag.String("scenario-file", "", "scenario JSON file to validate")
	determinism := flag.String("determinism", "", "comma-separated Nova reports to check")
	normalize := flag.String("normalize", "", "run report to convert to its reproducible golden form")
	outPath := flag.String("out", "", "output JSON path; stdout when empty")
	scenario := flag.String("scenario", "", "scenario name for Nexus import")
	packageName := flag.String("package", "", "target package for Nexus import")
	engineVersion := flag.String("engine-version", "", "engine version recorded on import")
	seed := flag.Int64("seed", 0, "seed recorded on Nexus import")
	durationMillis := flag.Int64("duration-millis", 0, "wall-clock budget recorded on Nexus import")
	tolerance := flag.Float64("coverage-tolerance", 0, "allowed activity coverage regression in percentage points")
	flag.Parse()
	var result any
	if *normalize != "" {
		var report evaluation.RunReport
		readJSON(*normalize, &report)
		result = evaluation.NormalizeForGolden(report)
	} else if *scenarioFile != "" {
		var scenario evaluation.Scenario
		readJSON(*scenarioFile, &scenario)
		check(scenario.Validate())
		result = scenario
	} else if *determinism != "" {
		paths := strings.Split(*determinism, ",")
		reports := make([]evaluation.RunReport, 0, len(paths))
		for _, path := range paths {
			var report evaluation.RunReport
			readJSON(strings.TrimSpace(path), &report)
			reports = append(reports, report)
		}
		result = evaluation.CheckDeterminism(reports)
	} else if *nexusLog != "" {
		content, err := os.ReadFile(*nexusLog)
		check(err)
		result = evaluation.ParseNexusLog(string(content), evaluation.NexusOptions{Scenario: *scenario, Package: *packageName, EngineVersion: *engineVersion, Seed: *seed, DurationMillis: *durationMillis})
	} else if *novaRun != "" {
		var manifest struct {
			Config runner.RunConfig `json:"config"`
			State  runner.State     `json:"state"`
		}
		readJSON(*novaRun, &manifest)
		result = evaluation.FromRunner(*scenario, *engineVersion, manifest.Config, manifest.State)
	} else {
		if *baselinePath == "" || *candidatePath == "" {
			check(fmt.Errorf("provide -normalize, -scenario-file, -determinism, -nexus-log, -nova-run, or both -baseline and -candidate"))
		}
		var baseline, candidate evaluation.RunReport
		readJSON(*baselinePath, &baseline)
		readJSON(*candidatePath, &candidate)
		result = evaluation.Compare(baseline, candidate, *tolerance)
	}
	content, err := evaluation.Marshal(result)
	check(err)
	content = append(content, '\n')
	if *outPath == "" {
		_, err = os.Stdout.Write(content)
	} else {
		err = os.WriteFile(*outPath, content, 0o644)
	}
	check(err)
}

func readJSON(path string, value any) {
	content, err := os.ReadFile(path)
	check(err)
	check(json.Unmarshal(content, value))
}

func check(err error) {
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
