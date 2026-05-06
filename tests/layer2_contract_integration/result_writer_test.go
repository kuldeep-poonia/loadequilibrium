package layer2_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// ----- L2 result schema -----

type L2Threshold struct {
	Metric    string      `json:"metric"`
	Operator  string      `json:"operator"`
	Value     interface{} `json:"value"`
	Unit      string      `json:"unit"`
	Rationale string      `json:"rationale"`
}

type L2PercentileResult struct {
	P50Ms  float64 `json:"p50_ms"`
	P95Ms  float64 `json:"p95_ms"`
	P99Ms  float64 `json:"p99_ms"`
	P100Ms float64 `json:"p100_ms"`
}

type L2ResultData struct {
	Status        string              `json:"status"`
	ActualValue   float64             `json:"actual_value"`
	ActualUnit    string              `json:"actual_unit"`
	SampleCount   int                 `json:"sample_count"`
	Percentiles   *L2PercentileResult `json:"percentiles,omitempty"`
	WorstCaseInput interface{}        `json:"worst_case_input,omitempty"`
	ErrorMessages []string            `json:"error_messages,omitempty"`
	DurationMs    int64               `json:"duration_ms"`
}

type L2Questions struct {
	WhatWasTested        string `json:"what_was_tested"`
	WhyThisThreshold     string `json:"why_this_threshold"`
	WhatHappensIfFails   string `json:"what_happens_if_it_fails"`
	HowInterfaceVerified string `json:"how_interface_was_verified"`
	HasEverFailed        string `json:"has_this_ever_failed"`
	WorstCaseDescription string `json:"worst_case_description"`
}

type L2Record struct {
	TestID            string       `json:"test_id"`
	Layer             int          `json:"layer"`
	Name              string       `json:"name"`
	Aim               string       `json:"aim"`
	PackagesInvolved  []string     `json:"packages_involved"`
	FunctionUnderTest string       `json:"function_under_test"`
	Threshold         L2Threshold  `json:"threshold"`
	Result            L2ResultData `json:"result"`
	OnExceed          string       `json:"on_exceed"`
	Questions         L2Questions  `json:"answered_questions"`
	RunAt             string       `json:"run_at"`
	GoVersion         string       `json:"go_version"`
}

// ----- writer -----

var (
	l2Mu   sync.Mutex
	l2Once sync.Once
	l2Abs  string
)

func l2ResolvePath() string {
	l2Once.Do(func() {
		// Walk up from this file to find project root (contains go.mod).
		_, src, _, _ := runtime.Caller(0)
		dir := filepath.Dir(src)
		for i := 0; i < 5; i++ {
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				l2Abs = filepath.Join(dir, "tests", "results", "layer2_results.json")
				return
			}
			dir = filepath.Dir(dir)
		}
		// fallback
		l2Abs = "tests/results/layer2_results.json"
	})
	return l2Abs
}

func writeL2Result(r L2Record) {
	l2Mu.Lock()
	defer l2Mu.Unlock()

	path := l2ResolvePath()
	_ = os.MkdirAll(filepath.Dir(path), 0755)

	var existing []L2Record
	if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
		_ = json.Unmarshal(data, &existing)
	}

	// Deduplicate by TestID — overwrite matching record.
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
		fmt.Printf("WARNING: could not write L2 results: %v\n", err)
	}
}

func l2Now() string   { return time.Now().UTC().Format(time.RFC3339) }
func l2GoVer() string { return runtime.Version() }
func l2Pass(ok bool) string {
	if ok {
		return "PASS"
	}
	return "FAIL"
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(float64(len(sorted)) * p / 100)
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}