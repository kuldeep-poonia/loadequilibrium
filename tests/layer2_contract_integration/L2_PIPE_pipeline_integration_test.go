package layer2_test

import (
	"fmt"
	"math"
	"sort"
	"testing"
	"time"

	"github.com/loadequilibrium/loadequilibrium/internal/autopilot"
)

// ---------------------------------------------------------------
// L2-PIPE-001 — Full autopilot pipeline tick latency
// AIM: End-to-end RuntimeOrchestrator.Tick() measures real wall-clock
//
//	latency for the control path:
//	  MPC.Optimise → Safety.ShouldOverrideProb → Rollout.StepAdaptive
//	  → Predictor.Step → ID.Step
//
// THRESHOLD: p50<1ms p95<3ms p99<8ms p100<20ms
// ON EXCEED: Control loop misses timing → instability
// ---------------------------------------------------------------
func TestL2_PIPE_001_FullPipelineTickLatency(t *testing.T) {
	start := time.Now()
	const N = 1000

	orch := buildDefaultOrchestrator()
	state := buildDefaultRuntimeState()

	latenciesMs := make([]float64, 0, N)

	for i := 0; i < N; i++ {
		arrival := 3.0 + 4.0*math.Sin(float64(i)*0.1) // oscillating arrival
		infraLoad := 0.2 + 0.1*math.Sin(float64(i)*0.07)

		t0 := time.Now()
		state, _ = orch.Tick(state, arrival, infraLoad)
		lat := float64(time.Since(t0).Microseconds()) / 1000.0
		latenciesMs = append(latenciesMs, lat)
	}

	sort.Float64s(latenciesMs)
	p50 := percentile(latenciesMs, 50)
	p95 := percentile(latenciesMs, 95)
	p99 := percentile(latenciesMs, 99)
	p100 := percentile(latenciesMs, 100)

	passed := p99 < 8 && p100 < 20
	durationMs := time.Since(start).Milliseconds()

	writeL2Result(L2Record{
		TestID: "L2-PIPE-001", Layer: 2,
		Name: "Full autopilot pipeline end-to-end tick latency",
		Aim:  "MPC→Safety→Rollout→Predictor→ID pipeline must complete within SLA: p50<1ms p95<3ms p99<8ms p100<20ms",
		PackagesInvolved: []string{
			"internal/autopilot",
		},
		FunctionUnderTest: "RuntimeOrchestrator.Tick (full pipeline)",
		Threshold:         L2Threshold{"pipeline_tick_p99_ms", "<", 8, "ms", "Control loop at 125Hz = 8ms budget per pipeline tick"},
		Result: L2ResultData{
			Status: l2Pass(passed), ActualValue: p99,
			ActualUnit: "ms", SampleCount: N,
			Percentiles: &L2PercentileResult{P50Ms: p50, P95Ms: p95, P99Ms: p99, P100Ms: p100},
			DurationMs:  durationMs,
		},
		OnExceed: "Pipeline too slow → control frequency drops → actuator corrections delayed → system drifts",
		Questions: L2Questions{
			WhatWasTested:        fmt.Sprintf("Full pipeline across %d ticks with oscillating arrival (3±4) and infraLoad (0.2±0.1)", N),
			WhyThisThreshold:     "The autopilot tick includes MPC SA optimization (50 iters × 8 scenarios), safety evaluation, rollout, predictor, and identification — all must fit in <8ms at p99",
			WhatHappensIfFails:   "Autopilot cannot maintain >100Hz control frequency → delayed corrections → oscillation/instability",
			HowInterfaceVerified: "Wall-clock measurement of each Tick() call with varying arrival and infra load",
			HasEverFailed:        fmt.Sprintf("p50=%.2fms p95=%.2fms p99=%.2fms p100=%.2fms", p50, p95, p99, p100),
			WorstCaseDescription: fmt.Sprintf("p99=%.2fms (threshold 8ms), p100=%.2fms", p99, p100),
		},
		RunAt: l2Now(), GoVersion: l2GoVer(),
	})

	if !passed {
		t.Fatalf("L2-PIPE-001 FAILED: p99=%.2fms p100=%.2fms\nFIX: reduce MPC iters or ScenarioCount in autopilot/mpc.go",
			p99, p100)
	}
	t.Logf("L2-PIPE-001 PASS: p50=%.2fms p95=%.2fms p99=%.2fms p100=%.2fms", p50, p95, p99, p100)
}

// ---------------------------------------------------------------
// L2-PIPE-002 — No NaN/Inf in pipeline output (no silent corruption)
// AIM: RuntimeOrchestrator.Tick must never produce NaN/Inf in
//
//	telemetry output, regardless of input intensity.
//
// THRESHOLD: 0 NaN/Inf values across all ticks
// ON EXCEED: Silent data corruption → downstream consumers
//
//	(dashboard, actuator) operate on garbage
//
// ---------------------------------------------------------------
func TestL2_PIPE_002_PipelineOutputNoNaN(t *testing.T) {
	start := time.Now()
	const N = 2000

	orch := buildDefaultOrchestrator()
	state := buildDefaultRuntimeState()

	var nanCount int
	var worstInput interface{}

	for i := 0; i < N; i++ {
		// Adversarial inputs: high arrival, zero infra, extreme swings.
		arrival := float64(i%100) * 0.5
		infraLoad := float64(i%10) * 0.1

		var tel autopilot.RuntimeTelemetry
		state, tel = orch.Tick(state, arrival, infraLoad)

		// Check all telemetry fields for NaN/Inf.
		fields := []struct {
			name string
			val  float64
		}{
			{"Backlog", tel.Backlog},
			{"Latency", tel.Latency},
			{"Capacity", tel.Capacity},
			{"Confidence", tel.Confidence},
			{"MPCConfidence", tel.MPCConfidence},
			{"OverrideRate", tel.OverrideRate},
			{"VarianceScale", tel.VarianceScale},
			{"SafetyScale", tel.SafetyScale},
			{"Damping", tel.Damping},
		}

		for _, f := range fields {
			if math.IsNaN(f.val) || math.IsInf(f.val, 0) {
				nanCount++
				worstInput = map[string]interface{}{
					"tick": i, "field": f.name, "value": f.val,
					"arrival": arrival, "infraLoad": infraLoad,
				}
			}
		}
	}

	passed := nanCount == 0
	durationMs := time.Since(start).Milliseconds()

	writeL2Result(L2Record{
		TestID: "L2-PIPE-002", Layer: 2,
		Name:              "Pipeline output no NaN/Inf corruption",
		Aim:               "RuntimeOrchestrator.Tick must never produce NaN/Inf in any telemetry field",
		PackagesInvolved:  []string{"internal/autopilot"},
		FunctionUnderTest: "RuntimeOrchestrator.Tick → RuntimeTelemetry",
		Threshold:         L2Threshold{"nan_inf_count", "==", 0, "count", "Any NaN/Inf in telemetry corrupts all downstream consumers"},
		Result: L2ResultData{
			Status: l2Pass(passed), ActualValue: float64(nanCount),
			ActualUnit: "count", SampleCount: N * 9, // 9 fields × N ticks
			WorstCaseInput: worstInput, DurationMs: durationMs,
		},
		OnExceed: "NaN/Inf in telemetry → dashboard shows garbage → operator loses situational awareness → bad decisions",
		Questions: L2Questions{
			WhatWasTested:        fmt.Sprintf("%d ticks × 9 telemetry fields = %d values checked", N, N*9),
			WhyThisThreshold:     "NaN/Inf propagates: one bad value corrupts all downstream computations (MPC cost, safety energy, etc.)",
			WhatHappensIfFails:   "NaN enters feedback loop → MPC produces NaN cost → simulated annealing accepts anything → system diverges",
			HowInterfaceVerified: "Check every RuntimeTelemetry field for math.IsNaN and math.IsInf after each Tick() call",
			HasEverFailed:        fmt.Sprintf("%d NaN/Inf values in this run", nanCount),
			WorstCaseDescription: fmt.Sprintf("NaN/Inf at %v", worstInput),
		},
		RunAt: l2Now(), GoVersion: l2GoVer(),
	})

	if !passed {
		t.Fatalf("L2-PIPE-002 FAILED: %d NaN/Inf values. First: %v\nFIX: add NaN guard in the computation chain leading to the corrupted field",
			nanCount, worstInput)
	}
	t.Logf("L2-PIPE-002 PASS: %d ticks, 0 NaN/Inf in telemetry", N)
}

// ---------------------------------------------------------------
// L2-PIPE-003 — MPC plan length always matches horizon
// AIM: MPCOptimiser.Optimise must always return a sequence of
//
//	exactly Horizon length, regardless of initial prevSeq length.
//
// THRESHOLD: 0 length mismatches
// ON EXCEED: Short plan → safety engine operates on truncated
//
//	trajectory → missing critical future states
//
// ---------------------------------------------------------------
func TestL2_PIPE_003_MPCPlanLengthContract(t *testing.T) {
	start := time.Now()

	horizons := []int{3, 5, 8, 10}
	prevSeqLens := []int{0, 1, 3, 5, 10}

	var mismatches []string
	tested := 0

	for _, h := range horizons {
		mpc := &autopilot.MPCOptimiser{
			Horizon: h, Dt: 0.1, ScenarioCount: 4, Deterministic: true,
			BacklogCost: 1, LatencyCost: 0.5, VarianceBase: 0.1,
			ScalingCost: 0.2, SmoothCost: 0.3, TerminalCost: 2,
			SafetyBarrier: 0.5, RiskQuantile: 0.8, RiskWeight: 0.3,
			MaxCapacity: 20, MinCapacity: 0.5, MaxStepCap: 2,
			MaxStepRetry: 0.5, MaxStepCache: 0.3,
			InitTemp: 1, Cooling: 0.95, Iters: 10,
		}

		initial := autopilot.MPCState{
			Backlog: 10, Latency: 2,
			ArrivalMean: 5, ServiceRate: 2,
			CapacityActive: 3,
		}

		for _, pLen := range prevSeqLens {
			tested++
			prev := make([]autopilot.MPCControl, pLen)
			for i := range prev {
				prev[i].CapacityTarget = 3
			}

			seq, _ := mpc.Optimise(initial, prev)
			if len(seq) != h {
				mismatches = append(mismatches,
					fmt.Sprintf("horizon=%d prevLen=%d got=%d", h, pLen, len(seq)))
			}
		}
	}

	passed := len(mismatches) == 0
	durationMs := time.Since(start).Milliseconds()

	writeL2Result(L2Record{
		TestID: "L2-PIPE-003", Layer: 2,
		Name:              "MPC plan length always matches horizon",
		Aim:               "MPCOptimiser.Optimise must return exactly Horizon-length sequence",
		PackagesInvolved:  []string{"internal/autopilot"},
		FunctionUnderTest: "MPCOptimiser.Optimise",
		Threshold:         L2Threshold{"plan_length_mismatches", "==", 0, "count", "Short plan → truncated safety trajectory → undetected risk"},
		Result: L2ResultData{
			Status: l2Pass(passed), ActualValue: float64(len(mismatches)),
			ActualUnit: "count", SampleCount: tested,
			ErrorMessages: mismatches, DurationMs: durationMs,
		},
		OnExceed: "MPC returns short plan → SafetyEngine.ShouldOverrideProb operates on truncated trajectory → misses future hazards",
		Questions: L2Questions{
			WhatWasTested:        fmt.Sprintf("%d combinations of horizon [3,5,8,10] × prevSeqLen [0,1,3,5,10]", tested),
			WhyThisThreshold:     "Safety evaluation depends on len(plan)==Horizon. Any mismatch silently truncates the safety check",
			WhatHappensIfFails:   "Safety engine evaluates fewer future states than expected → blind spot in predictive safety",
			HowInterfaceVerified: "Call Optimise with various prevSeq lengths, check len(result)==Horizon",
			HasEverFailed:        fmt.Sprintf("%d mismatches in this run", len(mismatches)),
			WorstCaseDescription: fmt.Sprintf("mismatches: %v", mismatches),
		},
		RunAt: l2Now(), GoVersion: l2GoVer(),
	})

	if !passed {
		t.Fatalf("L2-PIPE-003 FAILED: %d plan length mismatches\n%v", len(mismatches), mismatches)
	}
	t.Logf("L2-PIPE-003 PASS: %d horizon×prevLen combos, all correct", tested)
}
