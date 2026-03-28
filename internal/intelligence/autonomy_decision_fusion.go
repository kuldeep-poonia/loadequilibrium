package intelligence

import (
	"math"
	"math/rand"
	"sync"
)

/*
Frontier Fusion v4

• small coupled QP-style solver (iterative projected gradient)
• adaptive predictive risk weighting
• multi-mode stability vector gating
• frequency-phase oscillation detector
• asymmetric nonlinear feasibility projection
• stochastic safety margin injection
• regime performance-aware safety learning
*/

type FusionInput struct {
	State      []float64
	StateDeriv []float64
	Stability  []float64 // multi-mode stability vector

	PolicyAction []float64
	MPCAction    []float64

	PolicyUnc float64
	MPCUnc    float64

	HazardProb   float64
	RiskForecast []float64

	Epistemic float64

	RegimeID int

	PerfSignal  float64
	SLASeverity float64
}

type FusionOutput struct {
	Action         []float64
	SafetyOverride bool
	RiskScore      float64
}

type AutonomyDecisionFusion struct {
	mu sync.Mutex

	actDim int

	lastAction []float64

	regimeBias map[int]float64

	uncTrend float64

	freqEW  float64
	phaseEW float64
}

func NewAutonomyDecisionFusion(actDim int) *AutonomyDecisionFusion {
	return &AutonomyDecisionFusion{
		actDim:     actDim,
		lastAction: make([]float64, actDim),
		regimeBias: make(map[int]float64),
	}
}

/* ===== main ===== */

func (f *AutonomyDecisionFusion) Fuse(in FusionInput) FusionOutput {

	f.mu.Lock()
	defer f.mu.Unlock()

	risk := f.vectorRisk(in)

	th := f.dynamicThreshold(in)

	override := risk > th

	act := f.solveQP(in, override)

	act = f.nonlinearFeasible(act, in)

	act = f.stochasticMargin(act, risk)

	act = f.freqDamp(act, in)

	f.learnRegime(in)

	copy(f.lastAction, act)

	return FusionOutput{
		Action:         act,
		SafetyOverride: override,
		RiskScore:      risk,
	}
}

/* ===== vector risk ===== */

func (f *AutonomyDecisionFusion) vectorRisk(in FusionInput) float64 {

	r := in.HazardProb

	h := float64(len(in.RiskForecast))

	for i := range in.RiskForecast {

		w :=
			math.Exp(-float64(i)/h) *
				(1 + 0.5*f.uncTrend)

		r += w * in.RiskForecast[i]
	}

	stab := norm(in.Stability)

	f.uncTrend =
		0.93*f.uncTrend +
			0.07*in.Epistemic

	return clamp(sigmoid(r+0.6*stab), 0, 1)
}

func (f *AutonomyDecisionFusion) dynamicThreshold(in FusionInput) float64 {

	base := 0.55 + 0.25*f.bias(in.RegimeID)

	return clamp(
		base-0.2*math.Tanh(f.uncTrend),
		0.35,
		0.9,
	)
}

/* ===== QP iterative solver ===== */

func (f *AutonomyDecisionFusion) solveQP(in FusionInput, safe bool) []float64 {

	x := make([]float64, f.actDim)

	copy(x, f.lastAction)

	lr := 0.25

	for k := 0; k < 5; k++ {

		for i := 0; i < f.actDim; i++ {

			g :=
				2*(x[i]-in.PolicyAction[i])/(1+in.PolicyUnc) +
					2*(x[i]-in.MPCAction[i])/(1+in.MPCUnc)

			if safe {
				g += 0.8 * x[i]
			}

			x[i] -= lr * g
		}
	}

	return x
}

/* ===== nonlinear feasibility ===== */

func (f *AutonomyDecisionFusion) nonlinearFeasible(a []float64, in FusionInput) []float64 {

	out := make([]float64, len(a))

	load := norm(in.State)

	for i := range a {

		up := 2.0 + 0.5*math.Tanh(load) + 0.3*float64(i)
		lo := -1.5 - 0.4*math.Tanh(load)

		v := a[i]

		if v > up {
			v = up - 0.2*(v-up)*(v-up)
		}
		if v < lo {
			v = lo + 0.2*(lo-v)*(lo-v)
		}

		out[i] = v
	}

	return out
}

/* ===== stochastic safety ===== */

func (f *AutonomyDecisionFusion) stochasticMargin(a []float64, risk float64) []float64 {

	out := make([]float64, len(a))

	// Dithering amplitude shrinks as risk increases - maximum precision
	// when the system is most vulnerable. Exploration budget is allocated
	// to low-risk regimes where incorrect actions are recoverable.
	sigma := 0.15 * (1.0 - risk)

	for i := range a {

		out[i] =
			a[i] +
				sigma*(rand.Float64()-0.5)
	}

	return out
}

/* ===== oscillation detector ===== */

func (f *AutonomyDecisionFusion) freqDamp(a []float64, in FusionInput) []float64 {

	d := norm(in.StateDeriv)

	f.freqEW =
		0.9*f.freqEW +
			0.1*d

	f.phaseEW =
		0.92*f.phaseEW +
			0.08*math.Abs(d-f.freqEW)

	alpha :=
		clamp(
			0.4+
				0.5*math.Tanh(f.phaseEW),
			0.35,
			0.92,
		)

	out := make([]float64, len(a))

	for i := range a {
		out[i] =
			alpha*f.lastAction[i] +
				(1-alpha)*a[i]
	}

	return out
}

/* ===== regime learning ===== */

func (f *AutonomyDecisionFusion) learnRegime(in FusionInput) {

	b := f.bias(in.RegimeID)

	delta :=
		0.02*(in.PerfSignal-0.5) +
			0.03*in.SLASeverity

	f.regimeBias[in.RegimeID] =
		clamp(b+delta, -0.4, 0.6)
}

func (f *AutonomyDecisionFusion) bias(reg int) float64 {

	v, ok := f.regimeBias[reg]
	if !ok {
		v = 0
		f.regimeBias[reg] = v
	}
	return v
}

/* ===== utils ===== */
