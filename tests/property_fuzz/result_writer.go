package layer1

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// L1Threshold documents the acceptance criterion for a property test.
type L1Threshold struct {
	Metric    string  `json:"metric"`
	Operator  string  `json:"operator"`
	Value     float64 `json:"value"`
	Unit      string  `json:"unit"`
	Rationale string  `json:"rationale"`
}

// L1ResultData holds measured proof from the execution.
type L1ResultData struct {
	Status          string      `json:"status"`
	ActualValue     float64     `json:"actual_value"`
	ActualUnit      string      `json:"actual_unit"`
	IterationsRun   int         `json:"iterations_run"`
	Seed            int64       `json:"seed"`
	WorstCaseInput  interface{} `json:"worst_case_input"`
	WorstCaseOutput float64     `json:"worst_case_output"`
	DurationMs      int64       `json:"duration_ms"`
}

// L1Questions answers the master spec mandatory interrogation fields.
type L1Questions struct {
	WhatWasTested        string `json:"what_was_tested"`
	WhyThisThreshold     string `json:"why_this_threshold"`
	WhatHappensIfFails   string `json:"what_happens_if_it_fails"`
	IsDeterministic      string `json:"is_the_test_deterministic"`
	HasEverFailed        string `json:"has_this_ever_failed"`
	WorstCaseDescription string `json:"worst_case_description"`
}

// L1Record is a single test result conforming to the master Layer 1 spec schema.
type L1Record struct {
	TestID            string       `json:"test_id"`
	Layer             int          `json:"layer"`
	Name              string       `json:"name"`
	Aim               string       `json:"aim"`
	Package           string       `json:"package"`
	File              string       `json:"file"`
	FunctionUnderTest string       `json:"function_under_test"`
	Threshold         L1Threshold  `json:"threshold"`
	Result            L1ResultData `json:"result"`
	OnExceed          string       `json:"on_exceed"`
	Questions         L1Questions  `json:"answered_questions"`
	RunAt             string       `json:"run_at"`
	GoVersion         string       `json:"go_version"`
}

var (
	resultsMu sync.Mutex
)

// resolveResultsPath returns the absolute path to the results JSON file,
// walking up from the test binary directory to find the project root.
func resolveResultsPath() string {
	// Try relative path from the working directory (go test runs in package dir).
	// Walk upward to locate "tests/results/".
	candidates := []string{
		filepath.Join("..", "..", "tests", "results", "property_fuzz_results.json"),
		filepath.Join("tests", "results", "property_fuzz_results.json"),
	}
	for _, c := range candidates {
		dir := filepath.Dir(c)
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return c
		}
	}
	// Fallback: create the results directory relative to project root.
	p := filepath.Join("..", "..", "tests", "results", "property_fuzz_results.json")
	_ = os.MkdirAll(filepath.Dir(p), 0755)
	return p
}

func writeL1Result(r L1Record) {
	resultsMu.Lock()
	defer resultsMu.Unlock()

	path := resolveResultsPath()
	_ = os.MkdirAll(filepath.Dir(path), 0755)

	var existing []L1Record
	if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
		_ = json.Unmarshal(data, &existing)
	}

	// Deduplicate: replace existing record with same TestID.
	found := false
	for i, e := range existing {
		if e.TestID == r.TestID {
			existing[i] = r
			found = true
			break
		}
	}
	if !found {
		existing = append(existing, r)
	}

	data, _ := json.MarshalIndent(existing, "", "  ")
	if err := os.WriteFile(path, data, 0644); err != nil {
		fmt.Printf("WARNING: could not write results to %s: %v\n", path, err)
	}
}

func l1Now() string   { return time.Now().UTC().Format(time.RFC3339) }
func l1GoVer() string { return runtime.Version() }
func l1Pass(ok bool) string {
	if ok {
		return "PASS"
	}
	return "FAIL"
}