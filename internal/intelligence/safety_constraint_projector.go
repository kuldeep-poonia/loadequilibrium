package intelligence

import (
	"math"
)



type SafetyInput struct {
	Action     []float64
	PrevAction []float64
	State      []float64

	StabilityVec []float64

	Risk          float64
	HazardProxy   float64

	CapacityPress float64
	SLAWeight     float64
}

type SafetyOutput struct {
	Action []float64

	ViolationNorm float64
	ConstraintCost float64
}

type SafetyConstraintProjector struct {
	actDim int

	C [][]float64 // actuator coupling
}

func NewSafetyConstraintProjector(actDim int) *SafetyConstraintProjector {

	C := make([][]float64, actDim)

	for i := range C {
		C[i] = make([]float64, actDim)
		C[i][i] = 1
	}

	return &SafetyConstraintProjector{
		actDim: actDim,
		C: C,
	}
}

/* ===== main ===== */

func (s *SafetyConstraintProjector) Project(in SafetyInput) SafetyOutput {

	x := clone(in.Action)

	lambda := make([]float64, s.actDim)

	for it := 0; it < 18; it++ {

		g := s.totalGrad(x, lambda, in)

		step := 0.25 / (1 + 0.15*float64(it))

		for i := range x {
			x[i] -= step * g[i]
		}

		// dual update
		v := s.constraintVector(x, in)

		for i := range lambda {
			lambda[i] =
				math.Max(
					0,
					lambda[i]+0.2*v[i],
				)
		}

		if vecNorm(g) < 0.05 { // adaptive stop
			break
		}
	}

	x = s.integratedEnvelope(x, in)

	cost := vecNorm(s.constraintVector(x, in))

	return SafetyOutput{
		Action: x,
		ViolationNorm: vecNorm(diff(x, in.Action)),
		ConstraintCost: cost,
	}
}

/* ===== gradient ===== */

func (s *SafetyConstraintProjector) totalGrad(
	x []float64,
	lambda []float64,
	in SafetyInput,
) []float64 {

	g := make([]float64, len(x))

	load := vecNorm(in.State)

	/* box + envelope curvature */

	for i := range x {

		up :=
			2.8 +
				0.6*math.Tanh(load) -
				0.9*in.Risk

		lo :=
			-2.3 -
				0.5*math.Tanh(load)

		if x[i] > up {
			g[i] += 2*(x[i]-up) + 0.4*(x[i]-up)*(x[i]-up)
		}

		if x[i] < lo {
			g[i] += 2*(x[i]-lo) - 0.4*(x[i]-lo)*(x[i]-lo)
		}
	}

	/* true L1 capacity gradient */

	sum := 0.0
	for _, v := range x {
		sum += math.Abs(v)
	}

	capLim := 6 - 3*in.CapacityPress

	if sum > capLim {

		for i := range x {
			g[i] += math.Copysign(1, x[i]) * (sum - capLim)
		}
	}

	/* modal stability barrier */

	dir := unitNormalize(in.StabilityVec)

	for i := range x {

		proj :=
			dir[i%len(dir)] *
				x[i]

		bar :=
			sigmoid(
				3*(in.Risk-
					math.Abs(proj)),
			)

		g[i] += bar * dir[i%len(dir)]
	}

	/* actuator coupling quadratic */

	for i := range x {

		c := 0.0

		for j := range x {
			c += s.C[i][j] * x[j]
		}

		g[i] += 0.6 * c
	}

	/* rate constraint */

	for i := range x {

		d := x[i] - in.PrevAction[i]

		if math.Abs(d) > 1.4 {
			g[i] += 2 * (d - math.Copysign(1.4, d))
		}
	}

	/* hazard aligned risk economics */

	h := sigmoid(
		in.SLAWeight *
			(in.HazardProxy-in.Risk),
	)

	for i := range x {
		g[i] += h * x[i]
	}

	/* dual contribution */

	v := s.constraintVector(x, in)

	for i := range g {
		g[i] += lambda[i%len(lambda)] * v[i%len(v)]
	}

	return g
}

/* ===== constraint vector ===== */

func (s *SafetyConstraintProjector) constraintVector(
	x []float64,
	in SafetyInput,
) []float64 {

	v := make([]float64, len(x))

	sum := 0.0
	for _, a := range x {
		sum += math.Abs(a)
	}

	capLim := 6 - 3*in.CapacityPress

	for i := range v {
		v[i] = sum - capLim
	}

	return v
}

/* ===== envelope ===== */

func (s *SafetyConstraintProjector) integratedEnvelope(
	x []float64,
	in SafetyInput,
) []float64 {

	out := make([]float64, len(x))

	load := vecNorm(in.State)

	for i := range x {

		up :=
			3.2 +
				0.5*math.Tanh(load) -
				1.1*in.Risk

		lo :=
			-2.6 -
				0.6*math.Tanh(load)

		v := x[i]

		if v > up {
			v = up
		}

		if v < lo {
			v = lo
		}

		out[i] = v
	}

	return out
}

/* ===== utils ===== */

func unitNormalize(x []float64) []float64 {

	n := vecNorm(x) + 1e-6

	v := make([]float64, len(x))

	for i := range x {
		v[i] = x[i] / n
	}

	return v
}

