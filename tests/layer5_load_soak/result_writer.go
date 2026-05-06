package layer5

// FILE: tests/layer5_load_soak/result_writer.go
// Module: github.com/loadequilibrium/loadequilibrium

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"runtime"
	"sort"
	"sync"
	"time"
)

// ─── Result schema 

type L5Threshold struct {
	Metric    string  `json:"metric"`
	Operator  string  `json:"operator"`
	Value     float64 `json:"value"`
	Unit      string  `json:"unit"`
	Rationale string  `json:"rationale"`
}

type L5Percentiles struct {
	P50Ms  float64 `json:"p50_ms"`
	P95Ms  float64 `json:"p95_ms"`
	P99Ms  float64 `json:"p99_ms"`
	P100Ms float64 `json:"p100_ms"`
}

type L5ResultData struct {
	Status          string         `json:"status"`
	ActualValue     float64        `json:"actual_value"`
	ActualUnit      string         `json:"actual_unit"`
	SampleCount     int            `json:"sample_count"`
	Percentiles     *L5Percentiles `json:"percentiles,omitempty"`
	ThroughputRps   float64        `json:"throughput_rps,omitempty"`
	ErrorCount      int64          `json:"error_count,omitempty"`
	ErrorRate       float64        `json:"error_rate,omitempty"`
	DurationMs      int64          `json:"duration_ms"`
	ErrorMessages   []string       `json:"error_messages,omitempty"`
}

type L5Questions struct {
	WhatWasTested        string `json:"what_was_tested"`
	WhyThisThreshold     string `json:"why_this_threshold"`
	WhatHappensIfFails   string `json:"what_happens_if_it_fails"`
	HowLoadWasGenerated  string `json:"how_load_was_generated"`
	HowMetricsMeasured   string `json:"how_metrics_were_measured"`
	WorstCaseDescription string `json:"worst_case_description"`
}

type L5Record struct {
	TestID           string       `json:"test_id"`
	Layer            int          `json:"layer"`
	Name             string       `json:"name"`
	Aim              string       `json:"aim"`
	PackagesInvolved []string     `json:"packages_involved"`
	FunctionsTested  []string     `json:"functions_tested"`
	Threshold        L5Threshold  `json:"threshold"`
	Result           L5ResultData `json:"result"`
	OnExceed         string       `json:"on_exceed"`
	Questions        L5Questions  `json:"answered_questions"`
	RunAt            string       `json:"run_at"`
	GoVersion        string       `json:"go_version"`
}

// ─── Writer 

var (
	l5Mu      sync.Mutex
	l5OutPath = "tests/results/layer5_results.json"
)

func writeL5Result(r L5Record) {
	l5Mu.Lock()
	defer l5Mu.Unlock()
	_ = os.MkdirAll("tests/results", 0o755)
	var existing []L5Record
	if raw, err := os.ReadFile(l5OutPath); err == nil {
		_ = json.Unmarshal(raw, &existing)
	}
	existing = append(existing, r)
	data, _ := json.MarshalIndent(existing, "", "  ")
	if err := os.WriteFile(l5OutPath, data, 0o644); err != nil {
		fmt.Printf("WARNING: could not write layer5 results: %v\n", err)
	}
}

// ─── Helpers 

func l5Now() string   { return time.Now().UTC().Format(time.RFC3339) }
func l5GoVer() string { return runtime.Version() }
func l5Status(passed bool) string {
	if passed {
		return "PASS"
	}
	return "FAIL"
}

// computePercentiles returns p50/p95/p99/p100 from a slice of float64 durations in ms.
// Input need not be sorted — this function sorts a copy.
func computePercentiles(durationsMs []float64) L5Percentiles {
	if len(durationsMs) == 0 {
		return L5Percentiles{}
	}
	sorted := make([]float64, len(durationsMs))
	copy(sorted, durationsMs)
	sort.Float64s(sorted)
	return L5Percentiles{
		P50Ms:  pctile(sorted, 50),
		P95Ms:  pctile(sorted, 95),
		P99Ms:  pctile(sorted, 99),
		P100Ms: sorted[len(sorted)-1],
	}
}

func pctile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Ceil(float64(len(sorted))*p/100.0)) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// heapBytes returns current HeapInuse in bytes after forcing GC.
func heapBytes() uint64 {
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return ms.HeapInuse
}

type testCtx struct {
	done <-chan struct{}
}

func (t testCtx) Done() <-chan struct{} {
	return t.done
}

// testContextWithTimeout returns a context that expires after d.
// Defined here to avoid importing context in every file.
func testContextWithTimeout(d time.Duration) (testCtx, func()) {
	done := make(chan struct{})
	timer := time.AfterFunc(d, func() {
		close(done)
	})
	return testCtx{done: done}, func() {
		timer.Stop()
		select {
		case <-done:
		default:
			close(done)
		}
	}
}
