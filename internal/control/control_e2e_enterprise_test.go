package control_test

import (
	"encoding/json"
	"math"
	"os"
	"testing"
	"fmt"

	ctrl "github.com/loadequilibrium/loadequilibrium/internal/control"
	auto "github.com/loadequilibrium/loadequilibrium/internal/autopilot"
)

type StepLog struct {
	Time float64 `json:"time"`

	Backlog float64 `json:"backlog"`
	Latency float64 `json:"latency"`
	Util    float64 `json:"util"`

	Replicas int     `json:"replicas"`
	Queue    int     `json:"queue"`
	Retry    int     `json:"retry"`
	Cache    float64 `json:"cache"`

	Risk float64 `json:"risk"`
	Cost float64 `json:"cost"`

	MPC_Target        float64
	MPC_Confidence    float64
	Decision_Action   string
	Decision_Delta    float64
	Rollout_Capacity  float64
	Override_Rate     float64
	Mode              int
	Confidence        float64

	AutopilotCap    float64 `json:"autopilot_cap"`
ControlSelected int     `json:"control_selected"`
ActuatorActual  float64 `json:"actuator_actual"`
Override        bool    `json:"override"`

}

type RunLog struct {
	Scenario string `json:"scenario"`

	FinalBacklog float64 `json:"final_backlog"`
	PeakLatency  float64 `json:"peak_latency"`
	TotalCost    float64 `json:"total_cost"`
	AvgUtil      float64 `json:"avg_util"`
	Oscillation  float64 `json:"oscillation"`

	ReplicaMin float64 `json:"replica_min"`
	ReplicaMax float64 `json:"replica_max"`
	ReplicaAvg float64 `json:"replica_avg"`

	Health string `json:"health"`
}

func Test_Control_Enterprise_E2E(t *testing.T) {

	scenarios := []string{
		"steady",
		"burst",
		"retry_storm",
		"degradation",
	}

	allRuns := []RunLog{}

	for _, scenario := range scenarios {

		log := runScenario(scenario)
		allRuns = append(allRuns, log)

		t.Logf("[DONE] %s → backlog=%.2f latency=%.2f cost=%.2f",
			scenario,
			log.FinalBacklog,
			log.PeakLatency,
			log.TotalCost,
		)
	}

	file, err := os.Create("control_full_report.json")
	if err != nil {
		t.Fatalf("file error: %v", err)
	}

	enc := json.NewEncoder(file)
	enc.SetIndent("", "  ")

	if err := enc.Encode(allRuns); err != nil {
		t.Fatalf("json encode error: %v", err)
	}

	file.Sync()
	file.Close()
}

// ======================= CORE =======================

func runScenario(name string) RunLog {

sys := ctrl.SystemState{
    Replicas:         3,
    QueueLimit:       50,
    RetryLimit:       3,
    CacheAggression:  0.2,
    QueueDepth:       200,
    PredictedArrival: 120,
    ArrivalRate:      120,
    ServiceRate:      10,
    Latency:          100,
    SLATarget:        120,
    MinReplicas:      1,
    MaxReplicas:      50,
    MinRetry:         1,
    MaxRetry:         10,
}
	controller := ctrl.Controller{
    OptimizerCfg: ctrl.OptimizerConfig{
        ScenarioCount:   6,
        BaseTemperature: 1.0,
        MaxEvaluate:     30,
        MinEvaluate:     5,
    },
    SimCfg: ctrl.SimConfig{
        HorizonSteps:      25,
        Dt:                1,
        BaseLatency:       100,
        DisturbanceStd:    0.3,
        DisturbanceFreq:   0.2,
        RetryFeedbackGain: 0.4,
        EfficiencyDecay:   0.15,
        MaxQueueDelay:     200,
        HazardUtilGain:    0.5,
        HazardBacklogGain: 0.4,
        HazardRetryGain:   0.3,
    },
    CostCfg: ctrl.CostParams{
        InfraUnitCost:   1,
        SLAWeight:       2,
        RiskWeight:      3,
        BacklogWeight:   2,
        UtilTarget:      0.7,
        UtilBand:        0.2,
        SmoothReplica:   0.3,
        SmoothRetry:     0.2,
        SmoothQueue:     0.2,
        SmoothCache:     0.2,
        CacheCostWeight: 0.5,
    },

    // 🔥 MOST IMPORTANT FIX
    ActuatorCfg: ctrl.ActuatorConfig{
        MinReplicas: 1,
        MaxReplicas: 50,
        MaxScaleRate: 10,
        ScaleCooldownSec: 2,
        WarmupRate: 0.5,

        MinQueue: 10,
        MaxQueue: 200,
        MaxQueueRate: 20,
        QueueLagTau: 2,
        QueueCooldownSec: 2,

        RetryRate: 0.5,
        RetryDisturbanceGain: 0.2,
        MinRetry: 1,
        MaxRetry: 10,

        CacheRate: 0.3,
        CacheMemPressureGain: 0.5,
    },
}
	runtime := auto.RuntimeOrchestrator{
		Dt: 1,

		Predictor: &auto.Predictor{Dt: 1, MaxQueue: 10000},
		MPC: &auto.MPCOptimiser{
			Horizon: 10,
			Dt:      1,
			ScenarioCount: 4,
			MaxCapacity: 50,
			MinCapacity: 1,
		},
		Safety: &auto.SafetyEngine{
			BaseMaxBacklog: 5000,
			BaseMaxLatency: 1000,
		},
		Rollout: &auto.RolloutController{
			Dt: 1,
			QueueMax: 20,
		},
		ID: &auto.IdentificationEngine{
			Dt: 1,
		},

		SLA_Backlog: 1000,
		OverrideWindow: 20,
	}

	var logs []StepLog
	totalCost := 0.0
	utilSum := 0.0
	peakLatency := 0.0

	rtState := auto.RuntimeState{
		Plant: auto.CongestionState{
			Backlog: sys.QueueDepth,
			ArrivalMean: sys.PredictedArrival,
			ServiceRate: sys.ServiceRate,
			CapacityActive: float64(sys.Replicas),
		},
		Rollout: auto.RolloutState{
			CapacityActive: float64(sys.Replicas),
		},
	}

	for step := 0; step < 200; step++ {

		switch name {

		case "burst":
			if step > 50 && step < 110 {
				sys.PredictedArrival = 120
			}

		case "retry_storm":
			if step > 60 {
				sys.RetryLimit = 8
			}

		case "degradation":
			if step > 80 {
				sys.ServiceRate = 5
			}
		}

		backlog := sys.QueueDepth

		// === AUTOPILOT STEP ===
		rtState.Plant.Backlog = sys.QueueDepth
		rtState.Plant.ArrivalMean = sys.PredictedArrival
		rtState.Plant.ServiceRate = sys.ServiceRate

		denom := float64(sys.Replicas) * sys.ServiceRate
		if denom < 1 {
			denom = 1
		}
		util := sys.PredictedArrival / denom

		if math.IsInf(util, 0) || math.IsNaN(util) {
			util = 0
		}

		rtState, telemetry := runtime.Tick(
			rtState,
			sys.PredictedArrival,
			util, // infra load approx
		)

		var mpcTarget float64
		if len(rtState.LastPlan) > 0 {
			mpcTarget = rtState.LastPlan[0].CapacityTarget
		}

		rolloutCap := rtState.Rollout.CapacityActive
		override := telemetry.OverrideRate
		mode := telemetry.Mode
		conf := telemetry.Confidence

		// === APPLY AUTOPILOT DECISION ===
		newCap := rtState.Rollout.CapacityActive
		autopilotCap := newCap
		fmt.Println("STEP:", step)
        fmt.Println("AUTOPILOT CAP:", newCap)

		sys.Replicas = int(math.Max(1, newCap))
		fmt.Println("SYS BEFORE CTRL:", sys.Replicas)

		// === FALLBACK CONTROLLER ===
		controller.Tick(
			&sys,
			backlog,
			0.15,
			0.2,
			1,
			float64(sys.Replicas),
		)

		controlSelected := controller.LastDecision.Replicas
actActual := controller.ActState.ReplicaActual
overrideFlag := int(autopilotCap) != sys.Replicas

		fmt.Println("SYS AFTER CTRL:", sys.Replicas)
fmt.Println("------")

		cost := util*2 + backlog*0.1

		totalCost += cost
		utilSum += util

		if sys.Latency > peakLatency {
			peakLatency = sys.Latency
		}

		logs = append(logs, StepLog{
			Time: float64(step),

			Backlog: backlog,
			Latency: sys.Latency,
			Util:    util,

			Replicas: sys.Replicas,
			Queue:    sys.QueueLimit,
			Retry:    sys.RetryLimit,
			Cache:    sys.CacheAggression,

			AutopilotCap:    autopilotCap,
ControlSelected: controlSelected,
ActuatorActual:  actActual,
Override:        overrideFlag,

			// Autopilot signals
			MPC_Target:       mpcTarget,
			MPC_Confidence:   telemetry.MPCConfidence,
			Decision_Action:  telemetry.DecisionAction,
			Decision_Delta:   telemetry.DecisionDelta,
			Rollout_Capacity: rolloutCap,
			Override_Rate:    override,
			Mode:             mode,
			Confidence:       conf,

			Risk: telemetry.Confidence,
			Cost: cost,
		})

		service := float64(sys.Replicas) * sys.ServiceRate

		sys.QueueDepth += sys.PredictedArrival - service
		if sys.QueueDepth > 10000 {
			sys.QueueDepth = 10000
		}

		if sys.QueueDepth < 0 {
			sys.QueueDepth = 0
		}

		utilPressure := util / (1 + util)

		sys.Latency +=
			0.5*utilPressure +
				0.3*(sys.QueueDepth/float64(sys.QueueLimit)) -
				0.4*(sys.Latency-100)

		if math.IsInf(sys.Latency, 0) || math.IsNaN(sys.Latency) {
			sys.Latency = 1000
		}

		if sys.Latency > 1000 {
			sys.Latency = 1000
		}

		if sys.Latency < 50 {
			sys.Latency = 50
		}
	}

	replicaMin := math.MaxFloat64
	replicaMax := 0.0
	replicaSum := 0.0

	for _, s := range logs {
		r := float64(s.Replicas)

		if r < replicaMin {
			replicaMin = r
		}
		if r > replicaMax {
			replicaMax = r
		}
		replicaSum += r
	}

	replicaAvg := replicaSum / float64(len(logs))

	health := "PASS"

	if peakLatency > 800 || sys.QueueDepth > 5000 {
		health = "FAIL"
	} else if peakLatency > 300 {
		health = "WARNING"
	}

	// 🔥 DEBUG JSON (STEP-BY-STEP)
file2, _ := os.Create("control_debug.json")
defer file2.Close()

enc2 := json.NewEncoder(file2)
enc2.SetIndent("", "  ")
enc2.Encode(logs)

	return RunLog{
		Scenario: name,

		FinalBacklog: sys.QueueDepth,
		PeakLatency:  peakLatency,
		TotalCost:    totalCost,
		AvgUtil:      utilSum / float64(len(logs)),
		Oscillation:  computeOscillation(logs),

		ReplicaMin: replicaMin,
		ReplicaMax: replicaMax,
		ReplicaAvg: replicaAvg,

		Health: health,
	}
}

// ======================= METRIC =======================

func computeOscillation(logs []StepLog) float64 {
	sum := 0.0

	for i := 1; i < len(logs); i++ {
		diff := math.Abs(float64(logs[i].Replicas - logs[i-1].Replicas))
		sum += diff
	}

	return sum / float64(len(logs))
}
