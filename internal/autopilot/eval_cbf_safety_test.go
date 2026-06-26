package autopilot

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

type ValidationResult struct {
	InterventionsLegacy int
	InterventionsCBF    int
	MaxBacklogLegacy    float64
	MaxBacklogCBF       float64
	SLAViolationsLegacy int
	SLAViolationsCBF    int
	Safe                bool
}

func simulateCBF(mismatchFactor float64, adversarial bool) ValidationResult {
	// Setup Shadow Engine
	leg := &LegacySafetyEngine{
		BaseMaxBacklog:     2000,
		BaseMaxLatency:     2500,
		Alpha:              0.4,
		Beta:               0.2,
		ArrivalGain:        0.01,
		DisturbanceGain:    0.2,
		TopologyGain:       0.2,
		RetryGain:          0.1,
		TailRiskBase:       0.15,
		AccelBaseWindow:    3,
		AccelThreshold:     0.2,
		MaxCapacityRamp:    1.0,
		CapacityEffectTau:  1.0,
		TopologyDelayTau:   1.0,
		TerminalEnergyBase: 1e6,
		ContractionSlack:   0.2,
		HysteresisBand:     0.05,
	}

	cbf := &CBFSafetyEngine{
		BaseMaxBacklog: 2000,
		TimeStep:       1.0,
		EffectTau:      1.0,
	}

	shadow := &ShadowSafetyEngine{
		Legacy: leg,
		CBF:    cbf,
	}

	res := ValidationResult{Safe: true}

	var backlogLeg, backlogCBF float64
	capLeg, capCBF := 10.0, 10.0

	// Run 100 ticks
	for t := 0; t < 100; t++ {
		arrival := 100.0
		if adversarial && t > 20 && t < 30 {
			arrival = 500.0 // Massive DDoS spike
		}
		
		// Model mismatch applied to the estimation upper bound
		estArrival := arrival * mismatchFactor

		stateLeg := SafetyState{Backlog: backlogLeg, CapacityActive: capLeg, ServiceRate: 10, ArrivalMean: estArrival}
		stateCBF := SafetyState{Backlog: backlogCBF, CapacityActive: capCBF, ServiceRate: 10, ArrivalMean: estArrival}

		planLeg := []MPCControl{{CapacityTarget: capLeg}}
		planCBF := []MPCControl{{CapacityTarget: capCBF}}

		overLeg, sevLeg, _ := shadow.Legacy.ShouldOverrideProb(stateLeg, planLeg, estArrival)
		overCBF, sevCBF, _ := shadow.CBF.ShouldOverrideProb(stateCBF, planCBF, estArrival)

		if overLeg {
			res.InterventionsLegacy++
			capLeg = sevLeg
		}
		if overCBF {
			res.InterventionsCBF++
			capCBF = sevCBF
		}

		// Physics advance
		flowLeg := arrival - 10*capLeg
		flowCBF := arrival - 10*capCBF

		if flowLeg > 0 { backlogLeg += flowLeg } else { backlogLeg += flowLeg; if backlogLeg < 0 { backlogLeg = 0 } }
		if flowCBF > 0 { backlogCBF += flowCBF } else { backlogCBF += flowCBF; if backlogCBF < 0 { backlogCBF = 0 } }

		if backlogLeg > res.MaxBacklogLegacy { res.MaxBacklogLegacy = backlogLeg }
		if backlogCBF > res.MaxBacklogCBF { res.MaxBacklogCBF = backlogCBF }
	}

	if res.MaxBacklogLegacy > 2000 { res.SLAViolationsLegacy++ }
	if res.MaxBacklogCBF > 2000 { res.SLAViolationsCBF++ }

	return res
}

func TestEvaluateCBFSafety(t *testing.T) {
	fmt.Println("--- SAFETY SANDBOX: FORMAL VALIDATION & ROBUSTNESS ---")
	
	mismatches := []float64{0.7, 0.8, 0.9, 0.95, 1.0, 1.05, 1.1, 1.2, 1.3}
	
	fmt.Printf("\n1. Robustness to Model Error (Non-Adversarial)\n")
	fmt.Printf("%-10s | %-15s | %-15s | %-15s | %-15s\n", "Mismatch", "CBF Overrides", "Leg Overrides", "CBF MaxQ", "Leg MaxQ")
	for _, m := range mismatches {
		res := simulateCBF(m, false)
		fmt.Printf("%-10.2f | %-15d | %-15d | %-15.1f | %-15.1f\n", m, res.InterventionsCBF, res.InterventionsLegacy, res.MaxBacklogCBF, res.MaxBacklogLegacy)
	}

	fmt.Printf("\n2. Adversarial Burst (DDoS) with Mismatch\n")
	fmt.Printf("%-10s | %-15s | %-15s | %-15s | %-15s\n", "Mismatch", "CBF Overrides", "Leg Overrides", "CBF MaxQ", "Leg MaxQ")
	for _, m := range mismatches {
		res := simulateCBF(m, true)
		fmt.Printf("%-10.2f | %-15d | %-15d | %-15.1f | %-15.1f\n", m, res.InterventionsCBF, res.InterventionsLegacy, res.MaxBacklogCBF, res.MaxBacklogLegacy)
	}

	// Write JSON for dashboard
	f, _ := os.Create("cbf_safety_results.json")
	json.NewEncoder(f).Encode(map[string]string{"status": "evaluated"})
	f.Close()
}
