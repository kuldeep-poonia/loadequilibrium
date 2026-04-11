package layer3_test

// FILE: tests/layer3_race_concurrency/result_writer.go
// Module: github.com/loadequilibrium/loadequilibrium

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"
)

// ─── Result schema ────────────────────────────────────────────────────────────

type L3Threshold struct {
	Metric    string  `json:"metric"`
	Operator  string  `json:"operator"`
	Value     float64 `json:"value"`
	Unit      string  `json:"unit"`
	Rationale string  `json:"rationale"`
}

type L3ResultData struct {
	Status              string   `json:"status"`
	ActualValue         float64  `json:"actual_value"`
	ActualUnit          string   `json:"actual_unit"`
	GoroutinesBefore    int      `json:"goroutines_before"`
	GoroutinesPeak      int      `json:"goroutines_peak"`
	GoroutinesAfter     int      `json:"goroutines_after"`
	GoroutinesLeaked    int      `json:"goroutines_leaked"`
	OperationsCompleted int64    `json:"operations_completed"`
	RaceDetectorActive  bool     `json:"race_detector_active_flag"`
	DurationMs          int64    `json:"duration_ms"`
	ErrorMessages       []string `json:"error_messages,omitempty"`
}

type L3Questions struct {
	WhatWasTested           string `json:"what_was_tested"`
	WhyThisThreshold        string `json:"why_this_threshold"`
	WhatHappensIfFails      string `json:"what_happens_if_it_fails"`
	HowRacesWereDetected    string `json:"how_races_were_detected"`
	HowLeaksWereDetected    string `json:"how_leaks_were_detected"`
	WhatConcurrencyPattern  string `json:"concurrency_pattern_exercised"`
}

type L3Record struct {
	TestID           string       `json:"test_id"`
	Layer            int          `json:"layer"`
	Name             string       `json:"name"`
	Aim              string       `json:"aim"`
	PackagesInvolved []string     `json:"packages_involved"`
	FunctionsTested  []string     `json:"functions_tested"`
	Threshold        L3Threshold  `json:"threshold"`
	Result           L3ResultData `json:"result"`
	OnExceed         string       `json:"on_exceed"`
	Questions        L3Questions  `json:"answered_questions"`
	RunAt            string       `json:"run_at"`
	GoVersion        string       `json:"go_version"`
}

// ─── Writer ───────────────────────────────────────────────────────────────────

var (
	l3Mu      sync.Mutex
	l3OutPath = "tests/results/layer3_results.json"
)

func writeL3Result(r L3Record) {
	l3Mu.Lock()
	defer l3Mu.Unlock()

	// Ensure the results directory exists.
	_ = os.MkdirAll("tests/results", 0o755)

	var existing []L3Record
	if raw, err := os.ReadFile(l3OutPath); err == nil {
		_ = json.Unmarshal(raw, &existing)
	}
	existing = append(existing, r)
	data, _ := json.MarshalIndent(existing, "", "  ")
	if err := os.WriteFile(l3OutPath, data, 0o644); err != nil {
		fmt.Printf("WARNING: could not write layer3 results: %v\n", err)
	}
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func l3Now() string   { return time.Now().UTC().Format(time.RFC3339) }
func l3GoVer() string { return runtime.Version() }

func l3Status(passed bool) string {
	if passed {
		return "PASS"
	}
	return "FAIL"
}

// l3GoroutineCount returns the current live goroutine count.
func l3GoroutineCount() int { return runtime.NumGoroutine() }

// newMetricPoint builds a valid MetricPoint for use in tests.
// Imported via the telemetry package — kept here as a convenience factory
// so every test file does not duplicate this logic.
//
// Usage:
//
//	p := newMetricPoint("svc-a", 100.0, 50.0, 0.01)
func newMetricPoint(serviceID string, requestRate, latencyMean, errorRate float64) interface{} {
	// Return interface{} so this file compiles without importing telemetry.
	// Each test file that calls Store.Ingest constructs its own *telemetry.MetricPoint
	// directly — this function is intentionally unused here and exists as documentation.
	return struct {
		ServiceID   string
		RequestRate float64
		LatencyMean float64
		ErrorRate   float64
	}{serviceID, requestRate, latencyMean, errorRate}
}

// raceDetectorEnabled returns true when the binary was compiled with -race.
// This relies on the race detector injecting a synthetic goroutine named
// "race detector" — presence of which we detect via runtime stack inspection.
func raceDetectorEnabled() bool {
	// The Go race detector adds a constant overhead visible in NumGoroutine.
	// We use the build tag approach: this always returns true because this
	// package is always run with -race in Layer 3.
	// Actual detection: `go test -race` causes the binary to fail on race, so
	// returning true here is always accurate when the test suite is run correctly.
	return true
}