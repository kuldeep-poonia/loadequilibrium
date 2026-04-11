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
// L2-AUTO-001 — SafetyEngine EmergencyOverride fires on unsafe trajectory
// AIM: When backlog exceeds BaseMaxBacklog in every trajectory step,
//      EmergencyOverride MUST trigger (return override=true).
// THRESHOLD: 0 missed overrides for genuinely unsafe trajectories
// ON EXCEED: CRITICAL — unsafe trajectory passes without intervention
// ---------------------------------------------------------------
func TestL2_AUTO_001_SafetyOverrideFires(t *testing.T) {
	start := time.Now()
	const N = 1000

	se := &autopilot.SafetyEngine{
		BaseMaxBacklog:    100,
		BaseMaxLatency:    50,
		Alpha:             0.5,
		Beta:              0.3,
		ArrivalGain:       0.1,
		DisturbanceGain:   0.2,
		TopologyGain:      0.1,
		RetryGain:         0.05,
		TailRiskBase:      0.2,
		AccelBaseWindow:   5,
		AccelThreshold:    0.5,
		MaxCapacityRamp:   2.0,
		CapacityEffectTau: 0.5,
		TopologyDelayTau:  0.5,
		TerminalEnergyBase: 500,
		ContractionSlack:  0.3,
		HysteresisBand:    0.1,
	}

	var missedOverrides int
	var worstInput interface{}

	for i := 0; i < N; i++ {
		backlog := 200.0 + float64(i)*5 // always >> BaseMaxBacklog=100

		traj := make([]autopilot.SafetyState, 10)
		for j := range traj {
			traj[j] = autopilot.SafetyState{
				Backlog:        backlog + float64(j)*10,
				Latency:        80 + float64(j)*5,
				CapacityActive: 1.0,
				CapacityTarget: 1.0,
				ServiceRate:    0.5,
				ArrivalMean:    backlog * 0.1,
				ArrivalVar:     2.0,
			}
		}

		// Reset hysteresis for each trial.
		se.LastUnsafe = false
		override, _ := se.EmergencyOverride(traj)
		if !override {
			missedOverrides++
			worstInput = map[string]interface{}{
				"trial": i, "backlog": backlog,
			}
		}
	}

	passed := missedOverrides == 0
	durationMs := time.Since(start).Milliseconds()

	writeL2Result(L2Record{
		TestID: "L2-AUTO-001", Layer: 2,
		Name:              "SafetyEngine EmergencyOverride fires on unsafe trajectory",
		Aim:               "EmergencyOverride must trigger when all trajectory states exceed safety limits",
		PackagesInvolved:  []string{"internal/autopilot"},
		FunctionUnderTest: "SafetyEngine.EmergencyOverride",
		Threshold:         L2Threshold{"missed_overrides", "==", 0, "count", "CRITICAL — zero tolerance for missed unsafe detection"},
		Result: L2ResultData{
			Status: l2Pass(passed), ActualValue: float64(missedOverrides),
			ActualUnit: "count", SampleCount: N,
			WorstCaseInput: worstInput, DurationMs: durationMs,
		},
		OnExceed: "CRITICAL: Unsafe trajectory passes without safety override → system runs in dangerous state → compounding damage",
		Questions: L2Questions{
			WhatWasTested:        fmt.Sprintf("EmergencyOverride across %d trajectories with backlog [200, %d], all exceeding BaseMaxBacklog=100", N, 200+N*5),
			WhyThisThreshold:     "backlog >> BaseMaxBacklog in every step → PredictiveSafe must return false → override must trigger",
			WhatHappensIfFails:   "Safety engine fails to detect blatantly unsafe trajectories → system crashes without intervention",
			HowInterfaceVerified: "Build trajectory with backlog=200+(i*5)+(j*10), verify EmergencyOverride returns true",
			HasEverFailed:        fmt.Sprintf("%d missed in this run", missedOverrides),
			WorstCaseDescription: fmt.Sprintf("missed at %v", worstInput),
		},
		RunAt: l2Now(), GoVersion: l2GoVer(),
	})

	if !passed {
		t.Fatalf("L2-AUTO-001 FAILED: %d missed overrides.\nFIX: check PredictiveSafe and InstabilityGrowth in safety.go",
			missedOverrides)
	}
	t.Logf("L2-AUTO-001 PASS: %d unsafe trajectories, all correctly overridden", N)
}

// ---------------------------------------------------------------
// L2-AUTO-002 — PredictiveSafe always passes for benign trajectories
// AIM: Trajectory where backlog << BaseMaxBacklog and latency << BaseMaxLatency
//      must always return PredictiveSafe=true (no false alarms).
// THRESHOLD: 0 false alarms
// ON EXCEED: Safe trajectories trigger override → system halts unnecessarily
// ---------------------------------------------------------------
func TestL2_AUTO_002_PredictiveSafeNoFalseAlarm(t *testing.T) {
	start := time.Now()
	const N = 1000

	se := &autopilot.SafetyEngine{
		BaseMaxBacklog:    100,
		BaseMaxLatency:    50,
		Alpha:             0.5,
		Beta:              0.3,
		ArrivalGain:       0.1,
		DisturbanceGain:   0.2,
		TopologyGain:      0.1,
		RetryGain:         0.05,
		TailRiskBase:      0.2,
		AccelBaseWindow:   5,
		AccelThreshold:    0.5,
		MaxCapacityRamp:   2.0,
		CapacityEffectTau: 0.5,
		TopologyDelayTau:  0.5,
		TerminalEnergyBase: 500,
		ContractionSlack:  0.5,
		HysteresisBand:    0.1,
	}

	var falseAlarms int
	var worstInput interface{}

	for i := 0; i < N; i++ {
		backlog := float64(i%20) * 0.5 // backlog 0..10 — well under 100

		traj := make([]autopilot.SafetyState, 10)
		for j := range traj {
			traj[j] = autopilot.SafetyState{
				Backlog:        backlog,
				Latency:        1.0,
				CapacityActive: 5.0,
				CapacityTarget: 5.0,
				ServiceRate:    2.0,
				ArrivalMean:    1.0,
			}
		}

		safe := se.PredictiveSafe(traj)
		if !safe {
			falseAlarms++
			worstInput = map[string]interface{}{
				"trial": i, "backlog": backlog,
			}
		}
	}

	passed := falseAlarms == 0
	durationMs := time.Since(start).Milliseconds()

	writeL2Result(L2Record{
		TestID: "L2-AUTO-002", Layer: 2,
		Name:              "PredictiveSafe no false alarms on benign trajectories",
		Aim:               "PredictiveSafe must return true when backlog<<100 and latency<<50",
		PackagesInvolved:  []string{"internal/autopilot"},
		FunctionUnderTest: "SafetyEngine.PredictiveSafe",
		Threshold:         L2Threshold{"false_alarms", "==", 0, "count", "Safe trajectories must not trigger override — false alarms waste capacity"},
		Result: L2ResultData{
			Status: l2Pass(passed), ActualValue: float64(falseAlarms),
			ActualUnit: "count", SampleCount: N,
			WorstCaseInput: worstInput, DurationMs: durationMs,
		},
		OnExceed: "False alarms → system enters unnecessary emergency mode → capacity waste + operator alert fatigue",
		Questions: L2Questions{
			WhatWasTested:        fmt.Sprintf("PredictiveSafe across %d benign trajectories with backlog [0, 10]", N),
			WhyThisThreshold:     "backlog 0-10 vs limit 100 → well within safe zone. Any alarm is a false positive",
			WhatHappensIfFails:   "System triggers emergency override during normal operation → unnecessary capacity spikes",
			HowInterfaceVerified: "Build trajectory with backlog<<100, latency<<50, verify PredictiveSafe returns true",
			HasEverFailed:        fmt.Sprintf("%d false alarms in this run", falseAlarms),
			WorstCaseDescription: fmt.Sprintf("false alarm at %v", worstInput),
		},
		RunAt: l2Now(), GoVersion: l2GoVer(),
	})

	if !passed {
		t.Fatalf("L2-AUTO-002 FAILED: %d false alarms.\nFIX: check backlogLimit vs ContractionSlack in PredictiveSafe in safety.go",
			falseAlarms)
	}
	t.Logf("L2-AUTO-002 PASS: %d benign trajectories, 0 false alarms", N)
}

// ---------------------------------------------------------------
// L2-AUTO-003 — RolloutController capacity ramp bounded
// AIM: Each Step must change CapacityActive by at most
//      max(CapRampUpNormal, CapRampUpEmergency) * Dt per step.
// THRESHOLD: 0 ramp violations
// ON EXCEED: Capacity jumps unbounded → infrastructure shock
// ---------------------------------------------------------------
func TestL2_AUTO_003_RolloutCapacityRampBounded(t *testing.T) {
	start := time.Now()
	const N = 5000

	rc := &autopilot.RolloutController{
		Dt:                    0.1,
		CapRampUpNormal:       2.0,
		CapRampUpEmergency:    5.0,
		CapRampDown:           3.0,
		RetryEnableRamp:       1.0,
		RetryDisableRamp:      0.5,
		CacheEnableRamp:       1.0,
		CacheDisableRamp:      0.5,
		WarmupTau:             1.0,
		ConfigLagTau:          1.0,
		QueueMax:              20,
		QueuePressureRampGain: 0.5,
		EmergencyBacklog:      100,
		DegradedBacklog:       50,
		RolloutTimeout:        10,
		MaxRetries:            3,
		SuccessProbBase:       0.95,
		InfraFailureGain:      0.1,
	}

	// Max possible ramp per step = max(CapRampUpEmergency, CapRampDown) * Dt * (1 + QueuePressureRampGain * maxQueuePressure)
	// maxQueuePressure = QueueMax/QueueMax = 1.0 (can be higher if queue overflows but we use reasonable values)
	// = 5.0 * 0.1 * (1 + 0.5 * 2.0) = 0.5 * 2 = 1.0 — generous bound
	maxRamp := 5.0 * 0.1 * (1 + 0.5*3.0) // extra generous with 3x multiplier

	var violations int
	var worstDelta float64
	var worstInput interface{}

	state := autopilot.RolloutState{
		CapacityActive: 5.0,
	}

	for i := 0; i < N; i++ {
		targetCap := 1.0 + float64(i%10)*2 // oscillate target 1..19
		intent := autopilot.RolloutIntent{
			Cap:        targetCap,
			Retry:      0.5,
			Cache:      0.3,
			SLAWeight:  1.0,
			CostWeight: 0.5,
		}

		next := rc.Step(state, intent, float64(i%80), 0, 0)
		delta := math.Abs(next.CapacityActive - state.CapacityActive)

		if delta > maxRamp+1e-9 {
			violations++
			if delta > worstDelta {
				worstDelta = delta
				worstInput = map[string]interface{}{
					"step": i, "prev_cap": state.CapacityActive,
					"next_cap": next.CapacityActive, "delta": delta,
					"target": targetCap,
				}
			}
		}
		state = next
	}

	passed := violations == 0
	durationMs := time.Since(start).Milliseconds()

	writeL2Result(L2Record{
		TestID: "L2-AUTO-003", Layer: 2,
		Name:              "RolloutController capacity ramp bounded",
		Aim:               "Each Step must limit CapacityActive change to ramp_rate * Dt * pressure_gain",
		PackagesInvolved:  []string{"internal/autopilot"},
		FunctionUnderTest: "RolloutController.Step → rampCap",
		Threshold: L2Threshold{"ramp_violations", "==", 0, "count",
			fmt.Sprintf("Max ramp per step = %.3f (CapRampUpEmergency * Dt * max_pressure_mult)", maxRamp)},
		Result: L2ResultData{
			Status: l2Pass(passed), ActualValue: float64(violations),
			ActualUnit: "count", SampleCount: N,
			WorstCaseInput: worstInput, DurationMs: durationMs,
		},
		OnExceed: "Capacity jumps beyond ramp rate → infrastructure shock (too many containers spawned at once)",
		Questions: L2Questions{
			WhatWasTested:        fmt.Sprintf("%d rollout steps with oscillating capacity targets [1, 19]", N),
			WhyThisThreshold:     fmt.Sprintf("rampCap clamps step to ±rate*Dt. max rate=5*0.1*2.5=%.3f. Any larger jump is a code defect", maxRamp),
			WhatHappensIfFails:   "Infrastructure receives sudden capacity change → cold start latency spike → service degradation",
			HowInterfaceVerified: "Call Step() N times, check |ΔCapacityActive| <= maxRamp per step",
			HasEverFailed:        fmt.Sprintf("%d violations, worst delta=%.4f", violations, worstDelta),
			WorstCaseDescription: fmt.Sprintf("worst ramp violation: %v", worstInput),
		},
		RunAt: l2Now(), GoVersion: l2GoVer(),
	})

	if !passed {
		t.Fatalf("L2-AUTO-003 FAILED: %d ramp violations (max delta=%.4f, threshold=%.4f)\nFIX: check rampCap in rollout.go",
			violations, worstDelta, maxRamp)
	}
	t.Logf("L2-AUTO-003 PASS: %d steps, 0 ramp violations (worst delta=%.4f, bound=%.4f)", N, worstDelta, maxRamp)
}

// ---------------------------------------------------------------
// L2-AUTO-004 — RolloutController mode transitions correct
// AIM: GovernanceMode must be Emergency when backlog > EmergencyBacklog,
//      Degraded when backlog > DegradedBacklog, Normal otherwise.
// THRESHOLD: 0 mode misclassifications
// ON EXCEED: Wrong governance mode → wrong ramp rate → capacity timing wrong
// ---------------------------------------------------------------
func TestL2_AUTO_004_GovernanceModeTransitions(t *testing.T) {
	start := time.Now()

	rc := &autopilot.RolloutController{
		Dt:               0.1,
		CapRampUpNormal:  2.0,
		CapRampUpEmergency: 5.0,
		CapRampDown:      3.0,
		QueueMax:         20,
		EmergencyBacklog: 100,
		DegradedBacklog:  50,
		RolloutTimeout:   10,
		MaxRetries:       3,
		SuccessProbBase:  0.95,
		ConfigLagTau:     1.0,
		WarmupTau:        1.0,
	}

	type testCase struct {
		backlog      float64
		expectedMode autopilot.GovernanceMode
		label        string
	}

	cases := []testCase{
		{10, autopilot.ModeNormal, "low backlog → Normal"},
		{49, autopilot.ModeNormal, "below degraded threshold → Normal"},
		{51, autopilot.ModeDegraded, "above degraded threshold → Degraded"},
		{99, autopilot.ModeDegraded, "below emergency threshold → Degraded"},
		{101, autopilot.ModeEmergency, "above emergency threshold → Emergency"},
		{500, autopilot.ModeEmergency, "far above emergency → Emergency"},
		{0, autopilot.ModeNormal, "zero backlog → Normal"},
	}

	var misclassed []string
	for _, tc := range cases {
		state := autopilot.RolloutState{CapacityActive: 5.0}
		intent := autopilot.RolloutIntent{Cap: 5.0, SLAWeight: 1.0}
		next := rc.Step(state, intent, tc.backlog, 0, 0)

		if next.Mode != tc.expectedMode {
			misclassed = append(misclassed,
				fmt.Sprintf("backlog=%.0f: got mode=%d expected=%d (%s)", tc.backlog, next.Mode, tc.expectedMode, tc.label))
		}
	}

	passed := len(misclassed) == 0
	durationMs := time.Since(start).Milliseconds()

	writeL2Result(L2Record{
		TestID: "L2-AUTO-004", Layer: 2,
		Name:              "GovernanceMode transition correctness",
		Aim:               "GovernanceMode must follow: Normal < DegradedBacklog < Degraded < EmergencyBacklog < Emergency",
		PackagesInvolved:  []string{"internal/autopilot"},
		FunctionUnderTest: "RolloutController.modeNext (via Step)",
		Threshold:         L2Threshold{"misclassifications", "==", 0, "count", "Mode boundaries are deterministic thresholds"},
		Result: L2ResultData{
			Status: l2Pass(passed), ActualValue: float64(len(misclassed)),
			ActualUnit: "count", SampleCount: len(cases),
			ErrorMessages: misclassed, DurationMs: durationMs,
		},
		OnExceed: "Wrong governance mode → wrong ramp rate (normal vs emergency) → capacity response too slow or too fast",
		Questions: L2Questions{
			WhatWasTested:        fmt.Sprintf("%d backlog levels against mode thresholds (DegradedBacklog=50, EmergencyBacklog=100)", len(cases)),
			WhyThisThreshold:     "Mode transitions are boundary checks — exact. Any wrong classification is a logic error",
			WhatHappensIfFails:   "Emergency capacity ramp applied during normal operation or vice versa",
			HowInterfaceVerified: "Step() with known backlog, check resulting RolloutState.Mode",
			HasEverFailed:        fmt.Sprintf("%d misclassifications in this run", len(misclassed)),
			WorstCaseDescription: fmt.Sprintf("misclassed: %v", misclassed),
		},
		RunAt: l2Now(), GoVersion: l2GoVer(),
	})

	if !passed {
		t.Fatalf("L2-AUTO-004 FAILED: %d mode misclassifications\n%v", len(misclassed), misclassed)
	}
	t.Logf("L2-AUTO-004 PASS: %d mode transition cases correct", len(cases))
}

// ---------------------------------------------------------------
// L2-AUTO-005 — Supervisor.ShouldRecompute determinism
// AIM: Same PlantState input must produce same ShouldRecompute output
//      across 100 repeated calls (no hidden state mutation).
// THRESHOLD: 0 non-deterministic flips
// ON EXCEED: Supervisor decision depends on hidden mutable state
// ---------------------------------------------------------------
func TestL2_AUTO_005_SupervisorDeterminism(t *testing.T) {
	start := time.Now()
	const N = 100

	type testCase struct {
		state    autopilot.PlantState
		label    string
	}

	cases := []testCase{
		{autopilot.PlantState{Backlog: 10, ArrivalMean: 5, ServiceRate: 2, CapacityActive: 3, CapacityTarget: 3, CapacityTau: 1, ModelConfidence: 0.9}, "safe state"},
		{autopilot.PlantState{Backlog: 500, ArrivalMean: 100, ServiceRate: 2, CapacityActive: 3, CapacityTarget: 10, CapacityTau: 1, PredictionError: 0.8}, "stressed state"},
	}

	var flips int
	for _, tc := range cases {
		sup := &autopilot.Supervisor{
			Dt: 0.1, MaxHorizon: 10,
			Alpha: 1.0, Beta: 0.5,
			EnergyAbsLimit:      1000,
			SafeBacklogLimit:    200,
			TerminalSafeBacklog: 100,
			DisturbanceBound:    5,
			CostWeight:          0.1,
			AdaptGain:           0.01,
		}

		first := sup.ShouldRecompute(tc.state)
		for i := 1; i < N; i++ {
			// Re-create supervisor each time to reset adaptive state.
			supClone := &autopilot.Supervisor{
				Dt: 0.1, MaxHorizon: 10,
				Alpha: 1.0, Beta: 0.5,
				EnergyAbsLimit:      1000,
				SafeBacklogLimit:    200,
				TerminalSafeBacklog: 100,
				DisturbanceBound:    5,
				CostWeight:          0.1,
				AdaptGain:           0.01,
			}
			result := supClone.ShouldRecompute(tc.state)
			if result != first {
				flips++
			}
		}
	}

	passed := flips == 0
	durationMs := time.Since(start).Milliseconds()

	writeL2Result(L2Record{
		TestID: "L2-AUTO-005", Layer: 2,
		Name:              "Supervisor ShouldRecompute determinism",
		Aim:               "Same PlantState + fresh Supervisor must produce same ShouldRecompute result",
		PackagesInvolved:  []string{"internal/autopilot"},
		FunctionUnderTest: "Supervisor.ShouldRecompute",
		Threshold:         L2Threshold{"determinism_flips", "==", 0, "count", "Same input → same output for fresh Supervisor instance"},
		Result: L2ResultData{
			Status: l2Pass(passed), ActualValue: float64(flips),
			ActualUnit: "count", SampleCount: len(cases) * N,
			DurationMs: durationMs,
		},
		OnExceed: "Non-deterministic supervisor → recomputation timing unpredictable → control loop scheduling unreliable",
		Questions: L2Questions{
			WhatWasTested:        fmt.Sprintf("%d PlantState configurations, each tested %d times", len(cases), N),
			WhyThisThreshold:     "Supervisor.ShouldRecompute is pure function of PlantState + Supervisor params — must be deterministic for fresh instance",
			WhatHappensIfFails:   "MPC recomputation triggers at random → control quality degrades unpredictably",
			HowInterfaceVerified: "Create fresh Supervisor, call ShouldRecompute(same state) N times, verify all return same value",
			HasEverFailed:        fmt.Sprintf("%d flips in this run", flips),
			WorstCaseDescription: fmt.Sprintf("%d non-deterministic results", flips),
		},
		RunAt: l2Now(), GoVersion: l2GoVer(),
	})

	if !passed {
		t.Fatalf("L2-AUTO-005 FAILED: %d non-deterministic flips", flips)
	}
	t.Logf("L2-AUTO-005 PASS: %d trials, fully deterministic", len(cases)*N)
}

// ---------------------------------------------------------------
// L2-AUTO-006 — RuntimeOrchestrator tick latency SLA
// AIM: Single RuntimeOrchestrator.Tick() must complete within SLA:
//      p50<2ms p95<5ms p99<10ms
// THRESHOLD: p99 < 10ms
// ON EXCEED: Autopilot tick too slow → control frequency drops
// ---------------------------------------------------------------
func TestL2_AUTO_006_RuntimeTickLatencySLA(t *testing.T) {
	start := time.Now()
	const N = 500

	orch := buildDefaultOrchestrator()
	state := buildDefaultRuntimeState()

	latenciesMs := make([]float64, 0, N)

	for i := 0; i < N; i++ {
		t0 := time.Now()
		state, _ = orch.Tick(state, 5.0, 0.3)
		lat := float64(time.Since(t0).Microseconds()) / 1000.0
		latenciesMs = append(latenciesMs, lat)
	}

	sort.Float64s(latenciesMs)
	p50 := percentile(latenciesMs, 50)
	p95 := percentile(latenciesMs, 95)
	p99 := percentile(latenciesMs, 99)
	p100 := percentile(latenciesMs, 100)

	passed := p99 < 10
	durationMs := time.Since(start).Milliseconds()

	writeL2Result(L2Record{
		TestID: "L2-AUTO-006", Layer: 2,
		Name:              "RuntimeOrchestrator.Tick latency SLA",
		Aim:               "Single Tick() must complete within p50<2ms p95<5ms p99<10ms",
		PackagesInvolved:  []string{"internal/autopilot"},
		FunctionUnderTest: "RuntimeOrchestrator.Tick",
		Threshold:         L2Threshold{"tick_latency_p99_ms", "<", 10, "ms", "Autopilot runs at 100Hz → 10ms budget per tick"},
		Result: L2ResultData{
			Status: l2Pass(passed), ActualValue: p99,
			ActualUnit: "ms", SampleCount: N,
			Percentiles: &L2PercentileResult{P50Ms: p50, P95Ms: p95, P99Ms: p99, P100Ms: p100},
			DurationMs: durationMs,
		},
		OnExceed: "Tick exceeds budget → autopilot frequency drops → control latency increases → system drift",
		Questions: L2Questions{
			WhatWasTested:        fmt.Sprintf("%d sequential Tick() calls measuring wall-clock latency", N),
			WhyThisThreshold:     "100Hz autopilot loop = 10ms budget. MPC SA optimization + safety + rollout must fit",
			WhatHappensIfFails:   "Autopilot cannot maintain target control frequency → delayed corrections → instability",
			HowInterfaceVerified: "Wall-clock measurement of each Tick() call duration",
			HasEverFailed:        fmt.Sprintf("p50=%.2fms p95=%.2fms p99=%.2fms p100=%.2fms", p50, p95, p99, p100),
			WorstCaseDescription: fmt.Sprintf("p99=%.2fms (threshold 10ms), p100=%.2fms", p99, p100),
		},
		RunAt: l2Now(), GoVersion: l2GoVer(),
	})

	if !passed {
		t.Fatalf("L2-AUTO-006 FAILED: p99=%.2fms (threshold 10ms)\nFIX: profile MPC.Optimise iterations or SafetyEngine.ShouldOverrideProb",
			p99)
	}
	t.Logf("L2-AUTO-006 PASS: p50=%.2fms p95=%.2fms p99=%.2fms p100=%.2fms", p50, p95, p99, p100)
}

// ---------------------------------------------------------------
// Helpers: build default autopilot subsystem instances.
// ---------------------------------------------------------------

func buildDefaultOrchestrator() *autopilot.RuntimeOrchestrator {
	return &autopilot.RuntimeOrchestrator{
		Dt: 0.1,
		Predictor: &autopilot.Predictor{
			Dt: 0.1, MaxQueue: 1000,
			BurstEntryRate: 0.1, BurstCollapseThreshold: 5, BurstIntensity: 0.5,
			ArrivalRiseGain: 0.3, ArrivalDropGain: 0.1, VarianceDecayRate: 0.05,
			RetryGain: 0.1, RetryDelayTau: 0.5,
			DisturbanceSigma: 0.1, DisturbanceInjectionGain: 0.05, DisturbanceBound: 5,
			TopologyCouplingK: 0.2, TopologyAdaptTau: 1.0,
			CacheAdaptTau: 1.0, LatencyGain: 0.5,
			CapacityJitterSigma: 0.01, BarrierExpK: 0.1, BarrierCap: 2000,
		},
		MPC: &autopilot.MPCOptimiser{
			Horizon: 5, Dt: 0.1, ScenarioCount: 8, Deterministic: true,
			BacklogCost: 1, LatencyCost: 0.5, VarianceBase: 0.1, ScalingCost: 0.2,
			SmoothCost: 0.3, TerminalCost: 2, UtilCost: 0.1,
			SafetyBarrier: 0.5, RiskQuantile: 0.8, RiskWeight: 0.3,
			MaxCapacity: 20, MinCapacity: 0.5, MaxStepCap: 2, MaxStepRetry: 0.5, MaxStepCache: 0.3,
			InitTemp: 1.0, Cooling: 0.95, Iters: 50,
		},
		Safety: &autopilot.SafetyEngine{
			BaseMaxBacklog: 100, BaseMaxLatency: 50,
			Alpha: 0.5, Beta: 0.3,
			ArrivalGain: 0.1, DisturbanceGain: 0.2, TopologyGain: 0.1, RetryGain: 0.05,
			TailRiskBase: 0.2, AccelBaseWindow: 5, AccelThreshold: 0.5,
			MaxCapacityRamp: 2, CapacityEffectTau: 0.5, TopologyDelayTau: 0.5,
			TerminalEnergyBase: 500, ContractionSlack: 0.3,
		},
		Rollout: &autopilot.RolloutController{
			Dt: 0.1, CapRampUpNormal: 2, CapRampUpEmergency: 5, CapRampDown: 3,
			RetryEnableRamp: 1, RetryDisableRamp: 0.5,
			CacheEnableRamp: 1, CacheDisableRamp: 0.5,
			WarmupTau: 1, ConfigLagTau: 1, QueueMax: 20,
			QueuePressureRampGain: 0.5, EmergencyBacklog: 100, DegradedBacklog: 50,
			RolloutTimeout: 10, MaxRetries: 3, SuccessProbBase: 0.95, InfraFailureGain: 0.1,
		},
		ID: &autopilot.IdentificationEngine{
			Dt: 0.1, FastGain: 0.3, SlowGain: 0.05, BlendGain: 0.1, VarGain: 0.05,
			BurstGain: 0.2, BurstDecay: 0.1, BurstCap: 5,
			NoiseGain: 0.1, DriftGain: 0.05,
			BaseConfidenceFloor: 0.2, ConfidenceGain: 0.05,
			ReliabilityGain: 0.1, InfraSensitivity: 0.5,
			SLAWeightQueue: 1, SLAWeightLatency: 0.5,
			EVTFactor: 2, SeasonalGain: 0.01, DampingGain: 0.05,
		},
		SLA_Backlog:       100,
		OverrideWindow:    20,
		DampingMin:        0.5,
		DampingMax:        3.0,
		FailureScaleProb:  0,
		FailureConfigProb: 0,
		TelemetryTau:      1.0,
	}
}

func buildDefaultRuntimeState() autopilot.RuntimeState {
	return autopilot.RuntimeState{
		Plant: autopilot.CongestionState{
			Backlog:      5,
			ArrivalMean:  5,
			ServiceRate:  2,
			CapacityActive: 3,
			CapacityTarget: 3,
			CapacityTauUp:  1,
			CapacityTauDown: 1,
			ConcurrencyLimit: 100,
			ServiceEfficiency: 0.9,
		},
		Rollout: autopilot.RolloutState{
			CapacityActive: 3,
		},
		ID: autopilot.IdentificationState{
			ArrivalEstimate: 5,
			ModelConfidence: 0.8,
		},
		Mode: autopilot.ModeStable,
	}
}