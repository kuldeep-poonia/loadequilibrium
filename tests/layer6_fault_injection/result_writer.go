package layer6

// FILE: tests/layer6_fault_injection/result_writer.go
// Module: github.com/loadequilibrium/loadequilibrium

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"
)

// ─── Result schema 

type L6Threshold struct {
	Metric    string  `json:"metric"`
	Operator  string  `json:"operator"`
	Value     float64 `json:"value"`
	Unit      string  `json:"unit"`
	Rationale string  `json:"rationale"`
}

type L6ResultData struct {
	Status              string   `json:"status"`
	ActualValue         float64  `json:"actual_value"`
	ActualUnit          string   `json:"actual_unit"`
	FaultInjected       string   `json:"fault_injected"`
	TimeToDetectMs      int64    `json:"time_to_detect_fault_ms"`
	TimeToRecoverMs     int64    `json:"time_to_recover_ms"`
	CommandsSent        int64    `json:"commands_sent"`
	CommandsSucceeded   int64    `json:"commands_succeeded"`
	CommandsFailed      int64    `json:"commands_failed"`
	CommandsCoalesced   int64    `json:"commands_coalesced_not_sent"`
	Panics              int64    `json:"panics"`
	DurationMs          int64    `json:"duration_ms"`
	ErrorMessages       []string `json:"error_messages,omitempty"`
}

type L6Questions struct {
	WhatFaultWasInjected  string `json:"what_fault_was_injected"`
	WhyThisThreshold      string `json:"why_this_threshold"`
	WhatHappensIfFails    string `json:"what_happens_if_it_fails"`
	HowFaultWasInjected   string `json:"how_fault_was_injected"`
	HowRecoveryVerified   string `json:"how_recovery_was_verified"`
	WhatDegradedMeans     string `json:"what_degraded_mode_means"`
}

type L6Record struct {
	TestID           string       `json:"test_id"`
	Layer            int          `json:"layer"`
	Name             string       `json:"name"`
	Aim              string       `json:"aim"`
	PackagesInvolved []string     `json:"packages_involved"`
	FunctionsTested  []string     `json:"functions_tested"`
	Threshold        L6Threshold  `json:"threshold"`
	Result           L6ResultData `json:"result"`
	OnExceed         string       `json:"on_exceed"`
	Questions        L6Questions  `json:"answered_questions"`
	RunAt            string       `json:"run_at"`
	GoVersion        string       `json:"go_version"`
}

// ─── Writer 

var (
	l6Mu      sync.Mutex
	l6OutPath = "tests/results/layer6_results.json"
)

func writeL6Result(r L6Record) {
	l6Mu.Lock()
	defer l6Mu.Unlock()
	_ = os.MkdirAll("tests/results", 0o755)
	var existing []L6Record
	if raw, err := os.ReadFile(l6OutPath); err == nil {
		_ = json.Unmarshal(raw, &existing)
	}
	existing = append(existing, r)
	data, _ := json.MarshalIndent(existing, "", "  ")
	if err := os.WriteFile(l6OutPath, data, 0o644); err != nil {
		fmt.Printf("WARNING: could not write layer6 results: %v\n", err)
	}
}

// ─── Helpers 
func l6Now() string   { return time.Now().UTC().Format(time.RFC3339) }
func l6GoVer() string { return runtime.Version() }
func l6Status(passed bool) string {
	if passed {
		return "PASS"
	}
	return "FAIL"
}

// drainFeedback drains all available ActuationResults from the feedback channel
// without blocking. Returns (succeeded, failed) counts.
func drainFeedback(ch <-chan interface{ isActuationResult() }, timeout time.Duration) (int64, int64) {
	// Caller uses the real typed channel — this helper is not used directly.
	// See per-test drain functions that use the real actuator.ActuationResult type.
	return 0, 0
}