package autopilot

import (
	"math"
	"testing"
)

// ----------------------
// COMMON CONFIG
// ----------------------

func corePredictor() *Predictor {
	return &Predictor{
		Dt: 1,

		ArrivalRiseGain:   0.0,
		ArrivalDropGain:   0.0,
		VarianceDecayRate: 0.0,

		RetryGain: 0.0,

		DisturbanceSigma: 0.0,
		DisturbanceBound: 0.0,

		CacheAdaptTau: 5,
		LatencyGain:   1.0,

		CapacityJitterSigma: 0.0,
		MaxQueue:            15000,
	}
}

func coreMPC() *MPCOptimiser {
	return &MPCOptimiser{
		Horizon:     5,
		MinCapacity: 1,
		MaxCapacity: 300,
		Iters:       20,

		SmoothCost:  2.0,
		ScalingCost: 0.005,
	}
}

// ----------------------
// HELPER
// ----------------------

func runStep(p *Predictor, m *MPCOptimiser, state CongestionState) CongestionState {

	mpcState := MPCState{
		Backlog:        state.Backlog,
		ArrivalMean:    state.ArrivalMean,
		ServiceRate:    state.ServiceRate,
		CapacityActive: state.CapacityActive,
	}

	plan, _ := m.Optimise(mpcState, nil)

	state.CapacityTarget = plan[0].CapacityTarget

	return p.Step(state)
}

// ----------------------
// 1. EDGE CASE TEST
// ----------------------

func Test_Autopilot_EdgeCases(t *testing.T) {

	p := corePredictor()
	m := coreMPC()

	cases := []CongestionState{
		{Backlog: 0, ArrivalMean: 0, ServiceRate: 50, CapacityActive: 10},
		{Backlog: 15000, ArrivalMean: 500, ServiceRate: 50, CapacityActive: 1},
		{Backlog: 0, ArrivalMean: 300, ServiceRate: 0, CapacityActive: 10},
		{Backlog: 1000, ArrivalMean: 0, ServiceRate: 50, CapacityActive: 0},
	}

	for _, c := range cases {
		next := runStep(p, m, c)

		if math.IsNaN(next.Backlog) || math.IsInf(next.Backlog, 0) {
			t.Fatal("invalid backlog state")
		}
	}
}

// ----------------------
// 2. TRANSITION TEST
// ----------------------

func Test_Autopilot_LoadTransitions(t *testing.T) {

	p := corePredictor()
	m := coreMPC()

	state := CongestionState{
		Backlog:        0,
		ArrivalMean:    50,
		ServiceRate:    50,
		CapacityActive: 10,
	}

	loadPattern := []float64{
		50, 50, 50,
		300, 300, 300,
		50, 50, 50,
	}

	for i := 0; i < 100; i++ {

		state.ArrivalMean = loadPattern[i%len(loadPattern)]

		state = runStep(p, m, state)

		if state.Backlog > 15000 {
			t.Fatal("transition caused explosion")
		}
	}
}

// ----------------------
// 3. PREDICTOR STABILITY
// ----------------------

func Test_Predictor_NoDrift(t *testing.T) {

	p := corePredictor()

	state := CongestionState{
		Backlog:        0,
		ArrivalMean:    280,
		ServiceRate:    50,
		CapacityActive: 50,
	}

	initial := state.ArrivalMean

	for i := 0; i < 200; i++ {
		state = p.Step(state)
	}

	if math.Abs(state.ArrivalMean-initial) > 1 {
		t.Fatal("arrival drift detected")
	}
}

// ----------------------
// 4. MPC DECISION QUALITY
// ----------------------

func Test_MPC_NoOverreaction(t *testing.T) {

	m := coreMPC()

	state := MPCState{
		Backlog:        0,
		ArrivalMean:    280,
		ServiceRate:    50,
		CapacityActive: 50,
	}

	plan, _ := m.Optimise(state, nil)

	if plan[0].CapacityTarget < 2 {
		t.Fatal("mpc overreacted downward")
	}
}

// ----------------------
// 5. DETERMINISM TEST
// ----------------------

func Test_Autopilot_Deterministic(t *testing.T) {

	p := corePredictor()
	m := coreMPC()

	state1 := CongestionState{
		Backlog:        100,
		ArrivalMean:    280,
		ServiceRate:    50,
		CapacityActive: 20,
	}

	state2 := state1

	for i := 0; i < 50; i++ {
		state1 = runStep(p, m, state1)
		state2 = runStep(p, m, state2)
	}

	if math.Abs(state1.Backlog-state2.Backlog) > 1e-6 {
		t.Fatal("non-deterministic behavior detected")
	}
}

// ----------------------
// 6. EFFICIENCY TEST
// ----------------------

func Test_Autopilot_MinimalCapacity(t *testing.T) {

	p := corePredictor()
	m := coreMPC()

	state := CongestionState{
		Backlog:        0,
		ArrivalMean:    280,
		ServiceRate:    50,
		CapacityActive: 10,
	}

	var maxCap float64

	for i := 0; i < 100; i++ {
		state = runStep(p, m, state)

		if state.CapacityActive > maxCap {
			maxCap = state.CapacityActive
		}
	}

	if maxCap > 200 {
		t.Fatal("over-scaling detected")
	}
}

func Test_Autopilot_Chaos(t *testing.T) {

	p := corePredictor()
	m := coreMPC()

	state := CongestionState{
		Backlog:        0,
		ArrivalMean:    100,
		ServiceRate:    50,
		CapacityActive: 10,
	}

	for i := 0; i < 300; i++ {

		if i%50 == 0 {
			state.ArrivalMean = 800
		} else if i%30 == 0 {
			state.ArrivalMean = 50
		}

		state = runStep(p, m, state)

		if state.Backlog > 5000 {
			t.Fatal("chaos failed")
		}
	}
}
