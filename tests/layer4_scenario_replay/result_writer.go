package layer4

// FILE: tests/layer4_scenario_replay/result_writer.go
// Module: github.com/loadequilibrium/loadequilibrium

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"runtime"
	"sync"
	"time"
)

// ─── Result schema ─────────────────────────────────────────────────────────

type L4Threshold struct {
	Metric    string  `json:"metric"`
	Operator  string  `json:"operator"`
	Value     float64 `json:"value"`
	Unit      string  `json:"unit"`
	Rationale string  `json:"rationale"`
}

type L4FieldDiff struct {
	FieldName      string  `json:"field"`
	GoldenValue    float64 `json:"golden_value"`
	ActualValue    float64 `json:"actual_value"`
	AbsDelta       float64 `json:"abs_delta"`
	RelDeltaPct    float64 `json:"rel_delta_pct"`
	WithinTolerance bool   `json:"within_tolerance"`
}

type L4ResultData struct {
	Status             string        `json:"status"`
	ActualValue        float64       `json:"actual_value"`
	ActualUnit         string        `json:"actual_unit"`
	RunsAttempted      int           `json:"runs_attempted"`
	RunsIdentical      int           `json:"runs_identical"`
	FieldsChecked      int           `json:"fields_checked"`
	FieldsInTolerance  int           `json:"fields_in_tolerance"`
	FieldsOutside      int           `json:"fields_outside_tolerance"`
	WorstDeltaPct      float64       `json:"worst_relative_delta_pct"`
	OutputChecksum     string        `json:"output_sha256"`
	Diffs              []L4FieldDiff `json:"field_diffs,omitempty"`
	DurationMs         int64         `json:"duration_ms"`
	ErrorMessages      []string      `json:"error_messages,omitempty"`
}

type L4Questions struct {
	WhatWasTested          string `json:"what_was_tested"`
	WhyThisThreshold       string `json:"why_this_threshold"`
	WhatHappensIfFails     string `json:"what_happens_if_it_fails"`
	HowDeterminismVerified string `json:"how_determinism_was_verified"`
	IsGoldenFileFrozen     string `json:"is_golden_file_frozen"`
	HowToUpdateGolden      string `json:"how_to_update_golden_file"`
}

type L4Record struct {
	TestID           string       `json:"test_id"`
	Layer            int          `json:"layer"`
	Name             string       `json:"name"`
	Aim              string       `json:"aim"`
	PackagesInvolved []string     `json:"packages_involved"`
	FunctionsTested  []string     `json:"functions_tested"`
	GoldenFile       string       `json:"golden_file"`
	Threshold        L4Threshold  `json:"threshold"`
	Result           L4ResultData `json:"result"`
	OnExceed         string       `json:"on_exceed"`
	Questions        L4Questions  `json:"answered_questions"`
	RunAt            string       `json:"run_at"`
	GoVersion        string       `json:"go_version"`
}

// ─── Writer ────────────────────────────────────────────────────────────────

var (
	l4Mu      sync.Mutex
	l4OutPath = "tests/results/layer4_results.json"
)

func writeL4Result(r L4Record) {
	l4Mu.Lock()
	defer l4Mu.Unlock()
	_ = os.MkdirAll("tests/results", 0o755)
	var existing []L4Record
	if raw, err := os.ReadFile(l4OutPath); err == nil {
		_ = json.Unmarshal(raw, &existing)
	}
	existing = append(existing, r)
	data, _ := json.MarshalIndent(existing, "", "  ")
	if err := os.WriteFile(l4OutPath, data, 0o644); err != nil {
		fmt.Printf("WARNING: could not write layer4 results: %v\n", err)
	}
}

// ─── Golden file helpers ───────────────────────────────────────────────────

// goldenDir is the directory where golden files are stored.
const goldenDir = "tests/layer4_scenario_replay/golden"

func ensureGoldenDir() {
	_ = os.MkdirAll(goldenDir, 0o755)
}

// writeGoldenFile writes data as the golden reference for a test.
// Only called when the golden file does not yet exist.
func writeGoldenFile(name string, data interface{}) error {
	ensureGoldenDir()
	path := goldenDir + "/" + name + ".json"
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal golden: %w", err)
	}
	return os.WriteFile(path, raw, 0o644)
}

// readGoldenFile reads a golden file into a map[string]float64.
// Returns (nil, false) when the file does not yet exist.
func readGoldenFile(name string) (map[string]float64, bool) {
	path := goldenDir + "/" + name + ".json"
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var m map[string]float64
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, false
	}
	return m, true
}

// compareToGolden compares actual against golden with tolerancePct.
// Returns (diffs, worstDeltaPct).
func compareToGolden(goldenName string, actual map[string]float64, tolerancePct float64) ([]L4FieldDiff, float64) {
	golden, ok := readGoldenFile(goldenName)
	if !ok {
		return []L4FieldDiff{{FieldName: "golden_file_missing", WithinTolerance: false}}, 100.0
	}
	var diffs []L4FieldDiff
	var worstPct float64
	for k, gv := range golden {
		av, exists := actual[k]
		if !exists {
			diffs = append(diffs, L4FieldDiff{
				FieldName: k, GoldenValue: gv, ActualValue: 0,
				AbsDelta: gv, RelDeltaPct: 100, WithinTolerance: false,
			})
			if 100 > worstPct {
				worstPct = 100
			}
			continue
		}
		absDelta := math.Abs(av - gv)
		relPct := 0.0
		if gv != 0 {
			relPct = absDelta / math.Abs(gv) * 100
		} else if av != 0 {
			relPct = 100
		}
		within := relPct <= tolerancePct
		if relPct > worstPct {
			worstPct = relPct
		}
		if !within {
			diffs = append(diffs, L4FieldDiff{
				FieldName: k, GoldenValue: gv, ActualValue: av,
				AbsDelta: absDelta, RelDeltaPct: relPct, WithinTolerance: within,
			})
		}
	}
	return diffs, worstPct
}

// ─── Checksum ─────────────────────────────────────────────────────────────

func checksumOf(v interface{}) string {
	raw, _ := json.Marshal(v)
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}

// ─── Helpers ──────────────────────────────────────────────────────────────

func l4Now() string   { return time.Now().UTC().Format(time.RFC3339) }
func l4GoVer() string { return runtime.Version() }
func l4Status(passed bool) string {
	if passed {
		return "PASS"
	}
	return "FAIL"
}

func goldenExists(name string) bool {
	_, err := os.Stat(goldenDir + "/" + name + ".json")
	return err == nil
}

// absFloat64 returns |x|.
func absFloat64(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
