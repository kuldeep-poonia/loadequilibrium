package autopilot

import (
	"encoding/json"
	"math"
	"math/rand"
	"os"
	"testing"
	"time"
)

// ----------------- CONFIG (MATCH YOUR STRUCTS) -----------------

func hardcoreMPC() *MPCOptimiser {
	return &MPCOptimiser{
		Horizon: 8,
		Dt:      1,

		ScenarioCount: 8,
		Deterministic: false,
		BurstProb:     0.25,

		BacklogCost:  4.0,
		LatencyCost:  1.2,
		VarianceBase: 0.6,
		ScalingCost:  0.02,
		SmoothCost:   12.0,
		TerminalCost: 1.2,
		UtilCost:     2.0,

		SafetyBarrier: 12,
		RiskQuantile:  0.8,
		RiskWeight:    0.6,

		MaxCapacity: 300,
		MinCapacity: 1,

		MaxStepCap:   40,
		MaxStepRetry: 0.6,
		MaxStepCache: 0.4,

		InitTemp: 1.0,
		Cooling:  0.94,
		Iters:    150,
	}
}

func hardcorePredictor() *Predictor {
	return &Predictor{
		Dt: 1,

		BurstEntryRate:         0.25,
		BurstCollapseThreshold: 40,
		BurstIntensity:         1.3,

		ArrivalRiseGain:   0.0,
		ArrivalDropGain:   0.0,
		VarianceDecayRate: 0.0,

		RetryGain:     0.25,
		RetryDelayTau: 4,

		DisturbanceSigma:         0.15,
		DisturbanceInjectionGain: 0.12,
		DisturbanceBound:         15,

		TopologyCouplingK: 0.35,
		TopologyAdaptTau:  4,

		CacheAdaptTau: 4,
		LatencyGain:   1.1,

		CapacityJitterSigma: 0.02,

		BarrierExpK: 0.02,
		BarrierCap:  15000,

		MaxQueue: 15000,
	}
}

func hardcoreSafety() *SafetyEngine {
	return &SafetyEngine{
		BaseMaxBacklog: 1500,
		BaseMaxLatency: 250,

		Alpha: 1,
		Beta:  1,

		ArrivalGain:     0.02,
		DisturbanceGain: 0.02,
		TopologyGain:    0.02,
		RetryGain:       0.02,

		TailRiskBase: 1.2,

		AccelBaseWindow: 6,
		AccelThreshold:  0.6,

		MaxCapacityRamp:    80,
		TerminalEnergyBase: 20000,
	}
}

// ----------------- RESULT STRUCT -----------------

type KPI struct {
	MaxBacklog     float64 `json:"max_backlog"`
	FinalBacklog   float64 `json:"final_backlog"`
	MeanBacklog    float64 `json:"mean_backlog"`
	MaxCapacity    float64 `json:"max_capacity"`
	MeanCapacity   float64 `json:"mean_capacity"`
	OscillationCnt int     `json:"oscillation_count"`
}

type StepLog struct {
	Step                 int     `json:"step"`
	Arrival              float64 `json:"arrival"`
	Backlog              float64 `json:"backlog"`
	Capacity             float64 `json:"capacity"`
	TargetCapacity       float64 `json:"target_capacity"`
	Instability          float64 `json:"instability"`
	Confidence           float64 `json:"confidence"`
	Anomaly              string  `json:"anomaly"`
	DecisionAction       string  `json:"decision_action"`
	DecisionDelta        float64 `json:"decision_delta"`
	DecisionMode         string  `json:"decision_mode"`
	MemoryEffectiveness  float64 `json:"memory_effectiveness"`
	MemoryOscillation    float64 `json:"memory_oscillation"`
	FinalAppliedCapacity float64 `json:"final_applied_capacity"`
}

// ----------------- CORE RUNNER -----------------

func runScenario(t *testing.T, seed int64, steps int, scenario string) (KPI, []map[string]float64, []StepLog) {
	rand.Seed(seed)

	mpc := hardcoreMPC()
	pred := hardcorePredictor()

	state := CongestionState{
		Backlog:        150,
		ArrivalMean:    280,
		ServiceRate:    55,
		CapacityActive: 6,
	}

	var (
		maxB, sumB, sumC, maxC float64
		oscCnt                 int
		prevCap                = state.CapacityActive
		trace                  []map[string]float64
		logs                   []StepLog
	    
	)
    var prev []MPCControl


	controller := &Controller{
    Dt: 1,

    Kb: 1.0,
    Kp: 1.0,
    Kg: 1.0,

    MPCWeight: 0.5,
    //SmoothTau: 5,

    //MinCapacity: 1,
    MaxCapacity: 300,
}


	var prevPlant PlantState	
	memory := NewRegimeMemory(20)
	var prevBacklog float64 = state.Backlog
	prevConfState := ConfidenceState{PrevConfidence: 1.0}
	var prevCapacityActive float64 = state.CapacityActive

	for step := 0; step < steps; step++ {

		switch scenario {
		case "stable":
			state.ArrivalMean = 280
		case "spike":
			if step > 20 && step < 40 {
				state.ArrivalMean = 1000
			} else {
				state.ArrivalMean = 280
			}
		case "drop":
			if step > 20 && step < 60 {
				state.ArrivalMean = 50
			} else {
				state.ArrivalMean = 280
			}
		case "oscillating":
			if step%10 < 5 {
				state.ArrivalMean = 600
			} else {
				state.ArrivalMean = 150
			}
		case "overload":
			if step > 10 {
				state.ArrivalMean = 800
			} else {
				state.ArrivalMean = 280
			}
		default: // default is the legacy ABC mix
			if step < 20 {
				state.ArrivalMean = 280
			} else if step < 40 {
				state.ArrivalMean = 500
			} else if step < 60 {
				state.ArrivalMean = 100
			} else {
				state.ArrivalMean = 280
			}
		}

		// 1. Feature extraction
		backlogGrowth := state.Backlog - prevBacklog
		retryRate := state.RetryFactor
		utilization := state.ArrivalMean / math.Max(1e-6, state.ServiceRate*state.CapacityActive)

		// 2. Instability computation
		oscScore := memory.GetOscillationScore()
		instInput := InstabilityInput{
			Backlog:     state.Backlog,
			BacklogRate: backlogGrowth,
			Latency:     0,
			LatencyRate: 0,
			RetryRate:   retryRate,
			Oscillation: oscScore,
			Utilization: utilization,
		}
		inst, _ := ComputeInstability(instInput)

		// 3. Memory READ
		trend := memory.GetTrend()
		eff := memory.GetEffectiveness()
		osc := memory.GetOscillationScore()
		stab := memory.GetStabilityScore()

		// 4. Confidence computation (FIX 1: purely memory/signal based)
		confInput := ConfidenceInput{
			TrendConsistency:     1.0 - math.Abs(trend.Instability),
			SignalAgreement:      stab,
			ControlEffectiveness: eff,
			Oscillation:          osc,
		}
		conf, newConfState := ComputeConfidence(prevConfState, confInput)
		prevConfState = newConfState
		conf *= (0.5 + 0.5*stab)

		// 5. Anomaly classification
		anomalyInput := AnomalyInput{
			Instability:   inst,
			Confidence:    conf,
			BacklogGrowth: backlogGrowth,
			LatencyTrend:  0,
			RetryPressure: retryRate,
			Oscillation:   osc,
		}
		anomaly := Classify(anomalyInput)

		mpcState := MPCState{
			Backlog:        state.Backlog,
			ArrivalMean:    state.ArrivalMean,
			ServiceRate:    state.ServiceRate,
			CapacityActive: state.CapacityActive,
			ArrivalVar:     1,
		}
		plan, _ := mpc.Optimise(mpcState, prev)
		prev = plan
		ctrl := plan[0]

		// 6. Decision policy
		decision := Decide(DecisionInput{
			Instability:    inst,
			Confidence:     conf,
			Anomaly:        anomaly,
			Backlog:        state.Backlog,
			Workers:        state.CapacityActive,
			TargetCapacity: ctrl.CapacityTarget,
			Effectiveness:  eff,
			Oscillation:    osc,
			Trend:          trend.Instability,
		})

		// 7. MPC optimisation (BASE plan only)
		

		// 8. Merge: MPC + Decision
		var sign float64 = 0
		if decision.Action == "scale_up" {
			sign = 1
		} else if decision.Action == "scale_down" {
			sign = -1
		}
		
		sup := &Supervisor{Dt: 1}
		clampedDelta := sup.ClampDecision(decision.ScaleDelta, osc, conf)
		
		decisionCapacity := prevCapacityActive + sign*clampedDelta*prevCapacityActive
		mpcCapacity := ctrl.CapacityTarget
		
		finalCapacity := conf*decisionCapacity + (1.0-conf)*mpcCapacity
		
		required := state.ArrivalMean / math.Max(state.ServiceRate, 1)
		minCapacity := required * 1.4
		if finalCapacity < minCapacity {
			finalCapacity = minCapacity
		}

		// Equilibrium gate: if the system is at rest (empty queue, ample slack,
		// stable gradient), hold at current capacity — stop self-perturbation.
		slack := state.ServiceRate*state.CapacityActive - state.ArrivalMean
		if state.Backlog < 3.0 && slack > 30.0 && math.Abs(backlogGrowth) < 1.0 {
			finalCapacity = prevCapacityActive
		}

		ctrl.CapacityTarget = finalCapacity

		// 9. Controller execution
		out := controller.Compute(
			PlantState{
				Backlog:        state.Backlog,
				ArrivalMean:    state.ArrivalMean,
				ArrivalP95:     state.ArrivalMean,
				ServiceRate:    state.ServiceRate,
				CapacityActive: state.CapacityActive,
			},
			prevPlant,
			ControlInput{
				CapacityTarget: ctrl.CapacityTarget,
				RetryFactor:    state.RetryFactor,
				CacheRelief:    0,
			},
		)

		state.CapacityTarget = out.CapacityTarget
		state.RetryFactor = out.RetryFactor

		// 10. Memory WRITE (using ACTUAL applied change)
		actualDelta := state.CapacityActive - prevCapacityActive
		memory.Add(MemoryEntry{
			Instability: inst,
			Confidence:  conf,
			Anomaly:     anomaly,
			Backlog:     state.Backlog,
			Workers:     state.CapacityActive,
			Action:      decision.Action,
			ScaleDelta:  actualDelta,
		})

		prevPlant = PlantState{
			Backlog:        state.Backlog,
			ArrivalMean:    state.ArrivalMean,
			ArrivalP95:     state.ArrivalMean,
			ServiceRate:    state.ServiceRate,
			CapacityActive: state.CapacityActive,
		}

		// ---- PLANT ----
		beforePlantBacklog := state.Backlog
		state = pred.Step(state)

		// 11. Update previous state
		prevBacklog = beforePlantBacklog
		prevCapacityActive = state.CapacityActive

		// ---- LOG ----
		t.Logf("step=%d backlog=%.2f cap=%.2f target=%.2f arrival=%.2f",
			step,
			state.Backlog,
			state.CapacityActive,
			state.CapacityTarget,
			state.ArrivalMean,
		)

		logs = append(logs, StepLog{
			Step:                 step,
			Arrival:              state.ArrivalMean,
			Backlog:              state.Backlog,
			Capacity:             state.CapacityActive,
			TargetCapacity:       ctrl.CapacityTarget,
			Instability:          inst,
			Confidence:           conf,
			Anomaly:              string(anomaly),
			DecisionAction:       decision.Action,
			DecisionDelta:        decision.ScaleDelta,
			DecisionMode:         string(decision.Mode),
			MemoryEffectiveness:  eff,
			MemoryOscillation:    osc,
			FinalAppliedCapacity: out.CapacityTarget,
		})

		t.Logf("step=%d backlog=%.2f cap=%.2f target=%.2f arrival=%.2f",
			step,
			state.Backlog,
			state.CapacityActive,
			ctrl.CapacityTarget,
			state.ArrivalMean,
		)

		// -------- invariants --------
		if math.IsNaN(state.Backlog) || math.IsInf(state.Backlog, 0) {
			t.Fatalf("invalid backlog (NaN/Inf)")
		}
		if state.Backlog < 0 {
			t.Fatalf("negative backlog")
		}
		if state.CapacityActive <= 0 {
			t.Fatalf("non-positive capacity")
		}

		// oscillation
		if math.Abs(state.CapacityActive-prevCap) > 40 {
			oscCnt++
		}
		prevCap = state.CapacityActive

		// KPIs
		if state.Backlog > maxB {
			maxB = state.Backlog
		}
		if state.CapacityActive > maxC {
			maxC = state.CapacityActive
		}
		sumB += state.Backlog
		sumC += state.CapacityActive

		trace = append(trace, map[string]float64{
			"step":     float64(step),
			"backlog":  state.Backlog,
			"capacity": state.CapacityActive,
			"arrival":  state.ArrivalMean,
		})
	}

	// Write step logs to file
	file, _ := os.Create("autopilot_result.json")
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	encoder.Encode(logs)

	return KPI{
		MaxBacklog:     maxB,
		FinalBacklog:   state.Backlog,
		MeanBacklog:    sumB / float64(steps),
		MaxCapacity:    maxC,
		MeanCapacity:   sumC / float64(steps),
		OscillationCnt: oscCnt,
	}, trace, logs
	}

func Test_Autopilot_Stress(t *testing.T) {
	scenarios := []string{"stable", "spike", "drop", "oscillating", "overload"}
	for _, sc := range scenarios {
		t.Run(sc, func(t *testing.T) {
			kpi, _, _ := runScenario(t, 42, 100, sc)
			if kpi.MaxBacklog > 15000 {
				t.Errorf("%s: backlog exploded %v", sc, kpi.MaxBacklog)
			}
			if kpi.OscillationCnt > 80 {
				t.Errorf("%s: too many oscillations %d", sc, kpi.OscillationCnt)
			}
		})
	}
}

func Test_Autopilot_Hardcore_Endurance(t *testing.T) {
	kpi, _, logs := runScenario(t, 42, 100, "default")
	
	file, _ := os.Create("autopilot_final.json")
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	encoder.Encode(logs)
	file.Close()

	if kpi.MaxBacklog > 5000 {
		t.Errorf("backlog exploded: %v", kpi.MaxBacklog)
	}
	if kpi.OscillationCnt > 60 {
		t.Errorf("too many oscillations: %d", kpi.OscillationCnt)
	}
}

func Test_Autopilot_MultiSeed(t *testing.T) {
	seeds := []int64{1, 7, 21, 42, time.Now().Unix() % 1000}

	for _, s := range seeds {
		kpi, _, _ := runScenario(t, s, 400, "default")

		if kpi.MaxBacklog > 6000 {
			t.Fatalf("seed %d unstable (max backlog %v)", s, kpi.MaxBacklog)
		}
	}
}

func Test_Autopilot_Deterministic_Mode(t *testing.T) {
	mpc := hardcoreMPC()
	mpc.Deterministic = true

	pred := hardcorePredictor()
	// रहेगा, लेकिन override disable करेंगे

	run := func() float64 {
		state := CongestionState{
			Backlog:        120,
			ArrivalMean:    250,
			ServiceRate:    60,
			CapacityActive: 6,
		}

		var prev []MPCControl

		for i := 0; i < 100; i++ {

			plan, _ := mpc.Optimise(MPCState{
				Backlog:        state.Backlog,
				ArrivalMean:    state.ArrivalMean,
				ServiceRate:    state.ServiceRate,
				CapacityActive: state.CapacityActive,
				ArrivalVar:     1,
			}, prev)

			prev = plan

			ctrl := plan[0]

			// ❌ SAFETY DISABLED (deterministic test ke liye)
			// if o, fb := safe.ShouldOverrideProb(...)

			state.CapacityTarget = ctrl.CapacityTarget
			state = pred.Step(state)

			// ✅ HARD FREEZE WHEN NO BACKLOG
			if state.Backlog == 0 {
				state.CapacityTarget = state.CapacityActive
			}
		}

		return state.Backlog
	}

	a := run()
	b := run()

	if math.Abs(a-b) > 1e-6 {
		t.Fatalf("deterministic mode violated: %v vs %v", a, b)
	}
}

// ----------------- JSON WRITER -----------------

func writeJSON(t *testing.T, path string, trace []map[string]float64, kpi KPI) {
	_ = os.MkdirAll("tests/results", 0755)

	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create file: %v", err)
	}
	defer f.Close()

	out := map[string]any{
		"kpi":   kpi,
		"trace": trace,
	}

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		t.Fatalf("encode json: %v", err)
	}
}