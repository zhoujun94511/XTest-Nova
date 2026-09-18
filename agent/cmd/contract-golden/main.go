package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/zhoujun94511/xtest-nova/agent/internal/contract"
)

const maxBody = 4 << 20

type observation struct {
	Method      string   `json:"method"`
	Path        string   `json:"path"`
	Status      int      `json:"status"`
	ContentType string   `json:"contentType,omitempty"`
	BodyKind    string   `json:"bodyKind"`
	JSONFields  []string `json:"jsonFields,omitempty"`
	Error       string   `json:"error,omitempty"`
}

type difference struct {
	Method    string `json:"method"`
	Path      string `json:"path"`
	Baseline  any    `json:"baseline"`
	Candidate any    `json:"candidate"`
}

type report struct {
	SchemaVersion string        `json:"schemaVersion"`
	BaselineURL   string        `json:"baselineUrl"`
	CandidateURL  string        `json:"candidateUrl"`
	Fixtures      int           `json:"fixtures"`
	Baseline      []observation `json:"baseline"`
	Candidate     []observation `json:"candidate"`
	Differences   []difference  `json:"differences"`
}

func main() {
	baselineURL := flag.String("baseline-url", "", "Nexus reference base URL")
	candidateURL := flag.String("candidate-url", "", "Nova candidate base URL")
	out := flag.String("out", "", "output JSON path; stdout when empty")
	flag.Parse()
	if *baselineURL == "" || *candidateURL == "" {
		fatal(errors.New("-baseline-url and -candidate-url are required"))
	}
	fixtures, err := contract.SafeGoldenFixtures()
	if err != nil {
		fatal(err)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	result := report{SchemaVersion: "xtest-http-golden/v1", BaselineURL: *baselineURL, CandidateURL: *candidateURL, Fixtures: len(fixtures)}
	for _, fixture := range fixtures {
		result.Baseline = append(result.Baseline, observe(client, *baselineURL, fixture))
		result.Candidate = append(result.Candidate, observe(client, *candidateURL, fixture))
	}
	for index := range fixtures {
		left, right := result.Baseline[index], result.Candidate[index]
		if left.Status != right.Status || left.ContentType != right.ContentType || left.BodyKind != right.BodyKind || !equalStrings(left.JSONFields, right.JSONFields) || (left.Error == "") != (right.Error == "") {
			result.Differences = append(result.Differences, difference{left.Method, left.Path, left, right})
		}
	}
	content, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fatal(err)
	}
	content = append(content, '\n')
	if *out == "" {
		_, err = os.Stdout.Write(content)
	} else {
		err = os.WriteFile(*out, content, 0o644)
	}
	if err != nil {
		fatal(err)
	}
}

func observe(client *http.Client, baseURL string, fixture contract.GoldenFixture) observation {
	value := observation{Method: fixture.Method, Path: fixture.Path}
	request, err := http.NewRequest(fixture.Method, strings.TrimRight(baseURL, "/")+fixture.Path, nil)
	if err != nil {
		value.Error = err.Error()
		return value
	}
	response, err := client.Do(request)
	if err != nil {
		value.Error = err.Error()
		return value
	}
	value.Status = response.StatusCode
	value.ContentType = strings.TrimSpace(strings.Split(response.Header.Get("Content-Type"), ";")[0])
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBody+1))
	closeErr := response.Body.Close()
	if err != nil {
		value.Error = err.Error()
		return value
	}
	if closeErr != nil {
		value.Error = closeErr.Error()
		return value
	}
	if len(body) > maxBody {
		value.Error = "response exceeds 4 MiB"
		return value
	}
	value.BodyKind, value.JSONFields = classify(body)
	return value
}

func classify(body []byte) (string, []string) {
	if len(body) >= 8 && bytes.Equal(body[:8], []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}) {
		return "image/png", nil
	}
	if len(body) >= 3 && bytes.Equal(body[:3], []byte{0xff, 0xd8, 0xff}) {
		return "image/jpeg", nil
	}
	var value any
	if json.Unmarshal(body, &value) == nil {
		fields := make([]string, 0)
		collectFields("$", value, &fields)
		sort.Strings(fields)
		return "json", uniqueStrings(fields)
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return "empty", nil
	}
	return "text", nil
}

func uniqueStrings(values []string) []string {
	result := values[:0]
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}

func collectFields(path string, value any, fields *[]string) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			childPath := path + "." + key
			*fields = append(*fields, childPath)
			collectFields(childPath, child, fields)
		}
	case []any:
		*fields = append(*fields, path+"[]")
		for _, child := range typed {
			collectFields(path+"[]", child, fields)
		}
	}
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func fatal(err error) {
	_, _ = fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
