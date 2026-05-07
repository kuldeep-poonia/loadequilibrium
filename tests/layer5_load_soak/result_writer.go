package layer5

// FILE: tests/layer5_load_soak/result_writer.go
// Module: github.com/loadequilibrium/loadequilibrium

import (
	"context"
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
	Status        string         `json:"status"`
	ActualValue   float64        `json:"actual_value"`
	ActualUnit    string         `json:"actual_unit"`
	SampleCount   int            `json:"sample_count"`
	Percentiles   *L5Percentiles `json:"percentiles,omitempty"`
	ThroughputRps float64        `json:"throughput_rps,omitempty"`
	ErrorCount    int64          `json:"error_count,omitempty"`
	ErrorRate     float64        `json:"error_rate,omitempty"`
	DurationMs    int64          `json:"duration_ms"`
	ErrorMessages []string       `json:"error_messages,omitempty"`
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

func testContextWithTimeout(d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), d)
}

func waitForL5Workers(wg *sync.WaitGroup, timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

type l5LatencyRecorder struct {
	mu     sync.Mutex
	values []float64
	next   int
	full   bool
}

func newL5LatencyRecorder(capacity int) *l5LatencyRecorder {
	if capacity <= 0 {
		capacity = 1
	}
	return &l5LatencyRecorder{
		values: make([]float64, 0, capacity),
	}
}

func (r *l5LatencyRecorder) Record(ms float64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.values) < cap(r.values) {
		r.values = append(r.values, ms)
		return
	}
	r.values[r.next] = ms
	r.next = (r.next + 1) % len(r.values)
	r.full = true
}

func (r *l5LatencyRecorder) Snapshot() []float64 {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.values) == 0 {
		return nil
	}
	out := make([]float64, len(r.values))
	if !r.full {
		copy(out, r.values)
		return out
	}
	copy(out, r.values[r.next:])
	copy(out[len(r.values)-r.next:], r.values[:r.next])
	return out
}
