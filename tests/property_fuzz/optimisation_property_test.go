package layer1

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"

	"github.com/loadequilibrium/loadequilibrium/internal/modelling"
	"github.com/loadequilibrium/loadequilibrium/internal/optimisation"
)

// ---------------------------------------------------------------
// L1-OPT-001 — MPC trajectory cost bounded [0, 1]
// AIM: MPCHorizonEval.Evaluate must produce TrajectoryCostAvg and
//      MaxTrajectoryCost within [0, 1] for any ServiceModelBundle.
// THRESHOLD: 0 out-of-bound values
// ON EXCEED: Cost normalisation broken → optimizer makes wrong decisions
// ---------------------------------------------------------------
func TestL1_OPT_001_MPCTrajectoryCostBounds(t *testing.T) {
	const seed int64 = 1337
	start := time.Now()
	params := gopter.DefaultTestParametersWithSeed(seed)
	params.MinSuccessfulTests = 20000
	properties := gopter.NewProperties(params)

	var (
		worstValue float64
		worstInput interface{}
		iterations int
		violations int
	)

	properties.Property("MPC trajectory costs in [0, 1]", prop.ForAll(
		func(rho, trend, serviceRate, pidOut, scale float64) bool {
			iterations++

			bundle := &modelling.ServiceModelBundle{
				Queue: modelling.QueueModel{
					ServiceID:        "test-svc",
					Utilisation:      rho,
					UtilisationTrend: trend,
					ServiceRate:      serviceRate,
					MeanQueueLen:     rho * 100,
				},
				Stochastic: modelling.StochasticModel{
					ArrivalCoV:         0.3,
					BurstAmplification: 1.2,
				},
			}

			mpc := optimisation.NewMPCHorizonEval(5, 2.0, 0.70)
			result := mpc.Evaluate(bundle, pidOut, scale)

			oob := false
			maxOOB := 0.0
			if result.TrajectoryCostAvg < -1e-9 || result.TrajectoryCostAvg > 1.0+1e-9 {
				oob = true
				maxOOB = math.Max(maxOOB, math.Abs(result.TrajectoryCostAvg))
			}
			if result.MaxTrajectoryCost < -1e-9 || result.MaxTrajectoryCost > 1.0+1e-9 {
				oob = true
				maxOOB = math.Max(maxOOB, math.Abs(result.MaxTrajectoryCost))
			}
			if math.IsNaN(result.AdjustedScaleFactor) || math.IsInf(result.AdjustedScaleFactor, 0) {
				oob = true
				maxOOB = 999
			}

			if oob {
				violations++
				if maxOOB > worstValue {
					worstValue = maxOOB
					worstInput = map[string]float64{
						"rho": rho, "trend": trend, "svc_rate": serviceRate,
						"pid_out": pidOut, "scale": scale,
						"traj_avg": result.TrajectoryCostAvg, "traj_max": result.MaxTrajectoryCost,
					}
				}
				return false
			}
			return true
		},
		gen.Float64Range(0.0, 1.5),   // rho from idle to overloaded
		gen.Float64Range(-0.5, 0.5),  // trend
		gen.Float64Range(0.1, 1000),  // service rate
		gen.Float64Range(-1.0, 1.0),  // PID output
		gen.Float64Range(0.5, 3.0),   // scale factor
	))

	properties.TestingRun(t)

	passed := violations == 0
	durationMs := time.Since(start).Milliseconds()

	writeL1Result(L1Record{
		TestID: "L1-OPT-001", Layer: 1,
		Name:              "MPC trajectory cost bounded [0, 1]",
		Aim:               "MPCHorizonEval.Evaluate must produce trajectory costs within [0, 1] for all inputs",
		Package:           "internal/optimisation",
		File:              "mpc.go",
		FunctionUnderTest: "MPCHorizonEval.Evaluate",
		Threshold:         L1Threshold{"trajectory_cost_oob_count", "==", 0, "count", "Trajectory cost is defined as normalised risk-latency in [0,1] by construction (tanh and sigmoid outputs)"},
		Result: L1ResultData{
			Status: l1Pass(passed), ActualValue: float64(violations), ActualUnit: "count",
			IterationsRun: iterations, Seed: seed,
			WorstCaseInput: worstInput, WorstCaseOutput: worstValue, DurationMs: durationMs,
		},
		OnExceed: "Trajectory cost outside [0,1] → composite objective function produces wrong scale factor → MPC steers system away from setpoint",
		Questions: L1Questions{
			WhatWasTested:        fmt.Sprintf("MPCHorizonEval.Evaluate across %d random (ρ, trend, μ, pid, scale) tuples", iterations),
			WhyThisThreshold:     "The cost function uses tanh(Wq*2) ∈ [0,1] and sigmoid ∈ [0,1], weighted sum with w_lat+w_risk=1.0, so output is mathematically in [0,1]",
			WhatHappensIfFails:   "MPC cost normalisation is broken; optimizer cannot compare trajectories correctly; control quality degrades unpredictably",
			IsDeterministic:      "Yes — gopter seed=1337",
			HasEverFailed:        fmt.Sprintf("%d violations found in this run", violations),
			WorstCaseDescription: fmt.Sprintf("worst value=%.6f at %v", worstValue, worstInput),
		},
		RunAt: l1Now(), GoVersion: l1GoVer(),
	})

	if !passed {
		t.Fatalf("L1-OPT-001 FAILED: %d violations. Worst value=%.6f. Input=%v\nFIX: check cost normalisation in MPCHorizonEval.Evaluate in internal/optimisation/mpc.go",
			violations, worstValue, worstInput)
	}
	t.Logf("L1-OPT-001 PASS: %d iterations, 0 cost bound violations", iterations)
}

// ---------------------------------------------------------------
// L1-OPT-002 — PID output saturation invariant
// AIM: PIDController.Update must always return output within
//      [OutputMin, OutputMax] for any gain/setpoint/measurement.
// THRESHOLD: 0 bound violations across 500,000 inputs
// ON EXCEED: Actuator receives command beyond configured limits
// ---------------------------------------------------------------
func TestL1_OPT_002_PIDBoundSafety(t *testing.T) {
	const seed int64 = 777
	start := time.Now()
	params := gopter.DefaultTestParametersWithSeed(seed)
	params.MinSuccessfulTests = 500000
	properties := gopter.NewProperties(params)

	var (
		worstOver  float64
		worstInput interface{}
		iterations int
		violations int
	)

	properties.Property("PID output within [OutputMin, OutputMax]", prop.ForAll(
		func(kp, ki, kd, setpoint, measured float64) bool {
			iterations++
			if kp <= 0 || ki < 0 || kd < 0 {
				return true // skip non-physical gains
			}
			pid := optimisation.NewPIDController(kp, ki, kd, setpoint, 0.01, 100.0)
			// The default OutputMin=-1.0, OutputMax=1.0 from NewPIDController.

			// Feed multiple measurements to exercise integral windup.
			now := time.Now()
			var lastOutput float64
			for step := 0; step < 10; step++ {
				lastOutput = pid.Update(measured, now)
				now = now.Add(2 * time.Second)
			}

			over := lastOutput - pid.OutputMax
			under := pid.OutputMin - lastOutput

			if over > worstOver {
				worstOver = over
				worstInput = map[string]float64{
					"kp": kp, "ki": ki, "kd": kd, "sp": setpoint,
					"pv": measured, "output": lastOutput,
				}
			}
			if under > worstOver {
				worstOver = under
			}

			// Allow float64 epsilon tolerance.
			if lastOutput > pid.OutputMax+1e-12 || lastOutput < pid.OutputMin-1e-12 {
				violations++
				return false
			}
			return true
		},
		gen.Float64Range(0.001, 50),
		gen.Float64Range(0, 50),
		gen.Float64Range(0, 50),
		gen.Float64Range(-100, 100),
		gen.Float64Range(-100, 100),
	))

	properties.TestingRun(t)

	passed := violations == 0
	durationMs := time.Since(start).Milliseconds()

	writeL1Result(L1Record{
		TestID: "L1-OPT-002", Layer: 1,
		Name:              "PID saturation limit safety",
		Aim:               "PIDController.Update must never exceed [OutputMin, OutputMax] for any gains/setpoint/measurement",
		Package:           "internal/optimisation",
		File:              "pid.go",
		FunctionUnderTest: "PIDController.Update",
		Threshold:         L1Threshold{"bound_violation_count", "==", 0, "count", "OutputMin/OutputMax are hard actuator limits — any violation is unacceptable"},
		Result: L1ResultData{
			Status: l1Pass(passed), ActualValue: float64(violations), ActualUnit: "count",
			IterationsRun: iterations, Seed: seed,
			WorstCaseInput: worstInput, WorstCaseOutput: worstOver, DurationMs: durationMs,
		},
		OnExceed: "PID output exceeds configured bounds → scale factor exceeds physical actuator range → control system instability or hardware damage",
		Questions: L1Questions{
			WhatWasTested:        fmt.Sprintf("PIDController.Update across %d random (kp,ki,kd,sp,pv) tuples, 10 steps each", iterations),
			WhyThisThreshold:     "PID has explicit clamp: output = max(OutputMin, min(output, OutputMax)). Any violation means clamp is bypassed or order-of-operations wrong",
			WhatHappensIfFails:   "Actuator commanded beyond physical range; in load-balancing context, scale factor goes beyond [0.5, 3.0] bounds → other safety checks cascade-fail",
			IsDeterministic:      "Yes — gopter seed=777",
			HasEverFailed:        fmt.Sprintf("%d violations, worst overshoot=%.6f", violations, worstOver),
			WorstCaseDescription: fmt.Sprintf("worst overshoot=%.6f at %v", worstOver, worstInput),
		},
		RunAt: l1Now(), GoVersion: l1GoVer(),
	})

	if !passed {
		t.Fatalf("L1-OPT-002 FAILED: %d violations. Worst overshoot=%.6f. Input=%v\nFIX: check output clamp in PIDController.Update in internal/optimisation/pid.go",
			violations, worstOver, worstInput)
	}
	t.Logf("L1-OPT-002 PASS: %d iterations, 0 saturation violations", iterations)
}

// ---------------------------------------------------------------
// L1-OPT-003 — MPC adjusted scale factor within [0.5, 3.0]
// AIM: MPC must never produce an AdjustedScaleFactor outside [0.5, 3.0].
//      This is enforced by an explicit clamp in Evaluate.
// THRESHOLD: 0 violations
// ON EXCEED: Scale factor exceeds range → downstream Engine applies
//            out-of-range capacity change → system instability
// ---------------------------------------------------------------
func TestL1_OPT_003_MPCScaleFactorBounds(t *testing.T) {
	const seed int64 = 2024
	start := time.Now()
	params := gopter.DefaultTestParametersWithSeed(seed)
	params.MinSuccessfulTests = 30000
	properties := gopter.NewProperties(params)

	var (
		worstValue float64
		worstInput interface{}
		iterations int
		violations int
	)

	properties.Property("MPC scale factor in [0.5, 3.0]", prop.ForAll(
		func(rho, trend, serviceRate, pidOut, scale float64) bool {
			iterations++

			bundle := &modelling.ServiceModelBundle{
				Queue: modelling.QueueModel{
					ServiceID:        "test-svc",
					Utilisation:      rho,
					UtilisationTrend: trend,
					ServiceRate:      serviceRate,
					MeanQueueLen:     math.Max(rho*50, 0),
				},
			}

			mpc := optimisation.NewMPCHorizonEval(5, 2.0, 0.70)
			result := mpc.Evaluate(bundle, pidOut, scale)

			sf := result.AdjustedScaleFactor
			if math.IsNaN(sf) || math.IsInf(sf, 0) || sf < 0.5-1e-9 || sf > 3.0+1e-9 {
				violations++
				oob := 0.0
				if sf < 0.5 {
					oob = 0.5 - sf
				} else if sf > 3.0 {
					oob = sf - 3.0
				} else {
					oob = 999 // NaN/Inf
				}
				if oob > worstValue {
					worstValue = oob
					worstInput = map[string]float64{
						"rho": rho, "trend": trend, "svc_rate": serviceRate,
						"pid_out": pidOut, "scale": scale, "result_sf": sf,
					}
				}
				return false
			}
			return true
		},
		gen.Float64Range(0.0, 2.0),
		gen.Float64Range(-1.0, 1.0),
		gen.Float64Range(0.01, 500),
		gen.Float64Range(-1.0, 1.0),
		gen.Float64Range(0.1, 5.0),
	))

	properties.TestingRun(t)

	passed := violations == 0
	durationMs := time.Since(start).Milliseconds()

	writeL1Result(L1Record{
		TestID: "L1-OPT-003", Layer: 1,
		Name:              "MPC adjusted scale factor bounds [0.5, 3.0]",
		Aim:               "MPCHorizonEval.Evaluate must clamp AdjustedScaleFactor to [0.5, 3.0]",
		Package:           "internal/optimisation",
		File:              "mpc.go",
		FunctionUnderTest: "MPCHorizonEval.Evaluate",
		Threshold:         L1Threshold{"scale_factor_oob_count", "==", 0, "count", "Hard clamp at line: adjusted = math.Max(0.5, math.Min(adjusted, 3.0)). Any OOB means clamp bypassed"},
		Result: L1ResultData{
			Status: l1Pass(passed), ActualValue: float64(violations), ActualUnit: "count",
			IterationsRun: iterations, Seed: seed,
			WorstCaseInput: worstInput, WorstCaseOutput: worstValue, DurationMs: durationMs,
		},
		OnExceed: "MPC scale factor exceeds [0.5, 3.0] → Engine.RunControl amplifies it further → actuator receives unbounded capacity change → system crash",
		Questions: L1Questions{
			WhatWasTested:        fmt.Sprintf("MPCHorizonEval.Evaluate across %d random input tuples", iterations),
			WhyThisThreshold:     "MPC has explicit clamp: math.Max(0.5, math.Min(adjusted, 3.0)). Any value outside this range means dead code path or clamp bypass",
			WhatHappensIfFails:   "Scale factor passes unchecked to Engine.RunControl which applies further amplification — cascading out-of-bounds through the entire control pipeline",
			IsDeterministic:      "Yes — gopter seed=2024",
			HasEverFailed:        fmt.Sprintf("%d violations", violations),
			WorstCaseDescription: fmt.Sprintf("worst OOB=%.6f at %v", worstValue, worstInput),
		},
		RunAt: l1Now(), GoVersion: l1GoVer(),
	})

	if !passed {
		t.Fatalf("L1-OPT-003 FAILED: %d violations. Worst OOB=%.6f. Input=%v\nFIX: verify clamp math.Max(0.5, math.Min(adjusted, 3.0)) in mpc.go Evaluate",
			violations, worstValue, worstInput)
	}
	t.Logf("L1-OPT-003 PASS: %d iterations, 0 scale factor bound violations", iterations)
}

// ---------------------------------------------------------------
// L1-OPT-004 — Trajectory planner feasibility monotonicity
// AIM: PlanTrajectory must select a feasible candidate when one exists.
//      A candidate is feasible if it stays below collapseThreshold.
// THRESHOLD: 0 cases where a feasible candidate exists but planner
//            selects an infeasible one
// ON EXCEED: Planner recommends a trajectory that crosses collapse,
//            defeating the safety purpose of the planner
// ---------------------------------------------------------------
func TestL1_OPT_004_TrajectoryPlannerFeasibility(t *testing.T) {
	const seed int64 = 54321
	start := time.Now()
	params := gopter.DefaultTestParametersWithSeed(seed)
	params.MinSuccessfulTests = 20000
	properties := gopter.NewProperties(params)

	var (
		worstInput interface{}
		iterations int
		violations int
	)

	properties.Property("planner selects feasible trajectory when available", prop.ForAll(
		func(rho, trend, serviceRate float64) bool {
			iterations++
			if serviceRate <= 0 {
				return true
			}

			bundle := &modelling.ServiceModelBundle{
				Queue: modelling.QueueModel{
					ServiceID:        "test-svc",
					Utilisation:      rho,
					UtilisationTrend: trend,
					ServiceRate:      serviceRate,
				},
				Stochastic: modelling.StochasticModel{
					ArrivalCoV:         0.3,
					BurstAmplification: 1.2,
				},
			}

			plan := optimisation.PlanTrajectory(bundle, 0.70, 5, 2.0, 0.90)

			// Check: if any candidate is feasible, the best must be too.
			hasFeasible := false
			for _, c := range plan.Candidates {
				if c.Feasible {
					hasFeasible = true
					break
				}
			}

			if hasFeasible {
				// The selected candidate must be feasible.
				bestIdx := -1
				for i, c := range plan.Candidates {
					if c.ScaleFactor == plan.BestScaleFactor {
						bestIdx = i
						break
					}
				}
				if bestIdx >= 0 && !plan.Candidates[bestIdx].Feasible {
					violations++
					worstInput = map[string]float64{
						"rho": rho, "trend": trend, "svc_rate": serviceRate,
						"best_sf": plan.BestScaleFactor,
					}
					return false
				}
			}
			return true
		},
		gen.Float64Range(0.0, 1.2),
		gen.Float64Range(-0.3, 0.3),
		gen.Float64Range(0.1, 500),
	))

	properties.TestingRun(t)

	passed := violations == 0
	durationMs := time.Since(start).Milliseconds()

	writeL1Result(L1Record{
		TestID: "L1-OPT-004", Layer: 1,
		Name:              "Trajectory planner feasibility selection",
		Aim:               "PlanTrajectory must select a feasible candidate when any feasible candidate exists",
		Package:           "internal/optimisation",
		File:              "trajectory_planner.go",
		FunctionUnderTest: "PlanTrajectory",
		Threshold:         L1Threshold{"infeasible_selection_count", "==", 0, "count", "A planner that selects infeasible trajectories when feasible ones exist has broken selection logic"},
		Result: L1ResultData{
			Status: l1Pass(passed), ActualValue: float64(violations), ActualUnit: "count",
			IterationsRun: iterations, Seed: seed,
			WorstCaseInput: worstInput, WorstCaseOutput: float64(violations), DurationMs: durationMs,
		},
		OnExceed: "Planner selects infeasible trajectory → system deliberately steered past collapse threshold → congestion collapse",
		Questions: L1Questions{
			WhatWasTested:        fmt.Sprintf("PlanTrajectory feasibility across %d random (ρ, trend, μ) inputs", iterations),
			WhyThisThreshold:     "The planner's contract is 'prefer feasible candidates'. Selecting infeasible when feasible exists is a logic error in candidate ranking",
			WhatHappensIfFails:   "Planner recommends a trajectory that crosses the collapse threshold, causing the Engine to drive the system into overload",
			IsDeterministic:      "Yes — gopter seed=54321",
			HasEverFailed:        fmt.Sprintf("%d violations", violations),
			WorstCaseDescription: fmt.Sprintf("infeasible selection at %v", worstInput),
		},
		RunAt: l1Now(), GoVersion: l1GoVer(),
	})

	if !passed {
		t.Fatalf("L1-OPT-004 FAILED: %d violations. Input=%v\nFIX: check candidate ranking in PlanTrajectory — feasible candidates must be preferred",
			violations, worstInput)
	}
	t.Logf("L1-OPT-004 PASS: %d iterations, planner always selects feasible when available", iterations)
}
