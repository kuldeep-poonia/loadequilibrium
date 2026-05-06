package intelligence

import (
	"math"
	"math/rand"
)



type RolloutInput struct {
	State []float64
	Action []float64
	Regime int

	ModelUnc float64
	HazardUnc float64

	SLAWeight []float64

	Policy func([]float64) []float64
	PolicyUnc float64
}

type StabilityForecast struct {
	RiskTrajectory []float64
	ModeVector []float64
	SpectralGrowth float64
}

type PredictiveStabilityRollout struct {
	dim int
	act int

	A [][]float64
	B [][]float64

	P [][]float64

	burstCov [][]float64
	burstState []float64

	tipLevel float64

	baseH int
}

func NewPredictiveStabilityRollout(dim, act int) *PredictiveStabilityRollout {

	// P8: Normalize A's spectral radius to 0.5 so the initial state-transition
	// model is stable from tick 0. Without normalization, randomMatrix(dim,dim,1.0)
	// has expected spectral radius ≈ sqrt(dim) ≈ 2.0 for dim=4, causing rollout
	// trajectories to diverge in the first ~20 ticks, producing false high-risk
	// forecasts that trigger safety fallbacks before RLS has converged.
	// RLS continuously refines A; the normalized initial value only matters early.
	return &PredictiveStabilityRollout{
		dim: dim,
		act: act,
		A: normalizeSpectralRadius(randomMatrix(dim, dim, 1.0), 0.5),
		B: randomMatrix(dim, act, 1.0),
		P: identityMatrix(dim, 1),

		burstCov: randSPD(dim),
		burstState: make([]float64, dim),

		tipLevel: 1.0,

		baseH: 8,
	}
}

/* ===== main ===== */

func (r *PredictiveStabilityRollout) Forecast(in RolloutInput) StabilityForecast {

	h := r.adaptiveHorizon(in)

	x := clone(in.State)
	u := clone(in.Action)

	traj := make([]float64, h)
	mode := make([]float64, r.dim)

	specAccum := 0.0

	for t := 0; t < h; t++ {

		xPrev := clone(x)

		x = r.step(x, u, in)

		r.identifyRLS(xPrev, x, u)

		uPol := clone(u)
		if in.Policy != nil {
			uPol = in.Policy(x)
			if len(uPol) != len(u) {
				aligned := make([]float64, len(u))
				copy(aligned, uPol)
				uPol = aligned
			}
		}

		for i := range u {
			u[i] =
				(1-in.PolicyUnc)*uPol[i] +
					in.PolicyUnc*u[i]
		}

		if t%2 == 0 { // yha spectral thinning
			specAccum += r.trajSpectral(x)
		}

		risk := r.barrierRisk(x, in)

		traj[t] = risk

		for i := range mode {
			mode[i] += math.Abs(x[i])
		}

		r.updateTip(risk)
	}

	for i := range mode {
		mode[i] /= float64(h)
	}

	return StabilityForecast{
		RiskTrajectory: traj,
		ModeVector: mode,
		SpectralGrowth: specAccum / float64(h),
	}
}

/* ===== dynamics ===== */

func (r *PredictiveStabilityRollout) step(x, u []float64, in RolloutInput) []float64 {

	nx := make([]float64, r.dim)

	burst := r.multiBurst(in.Regime)

	for i := 0; i < r.dim; i++ {

		sum := 0.0
		for j := 0; j < r.dim; j++ {
			sum += r.A[i][j] * math.Tanh(x[j])
		}

		ctrl := 0.0
		for k := 0; k < r.act; k++ {
			ctrl += r.B[i][k] * u[k]
		}

		nx[i] =
			x[i] +
				sum +
				ctrl +
				0.2*burst[i]
	}

	return r.projectState(nx)
}

/* ===== identification (matrix RLS) ===== */

func (r *PredictiveStabilityRollout) identifyRLS(xPrev, x, u []float64) {

	for i := 0; i < r.dim; i++ {

		phiState := tanhVec(xPrev)
		y := x[i]

		pred := dot(r.A[i], phiState) + dot(r.B[i], u)
		err := y - pred
		gain := 0.05 / (1 + vecNorm(phiState) + vecNorm(u))

		for j := range r.A[i] {
			r.A[i][j] += gain * err * phiState[j]
		}
		for j := range r.B[i] {
			r.B[i][j] += gain * err * u[j]
		}
		if i < len(r.P) && i < len(r.P[i]) {
			r.P[i][i] = 0.99*r.P[i][i] + 0.01*err*err
		}
	}
}

/* ===== spectral trajectory ===== */

func (r *PredictiveStabilityRollout) trajSpectral(x []float64) float64 {

	v := randomVector(r.dim, 1.0)

	for it := 0; it < 4; it++ {

		nv := make([]float64, r.dim)

		for i := 0; i < r.dim; i++ {
			for j := 0; j < r.dim; j++ {
				nv[i] += r.A[i][j] * v[j]
			}
		}

		n := vecNorm(nv) + 1e-6
		for i := range v {
			v[i] = nv[i] / n
		}
	}

	num := dot(v, mulMatVec(r.A, v))

	return math.Abs(num)
}

/* ===== multivariate burst ===== */

func (r *PredictiveStabilityRollout) multiBurst(reg int) []float64 {

	out := make([]float64, r.dim)

	for i := 0; i < r.dim; i++ {

		z := rand.NormFloat64()

		r.burstState[i] =
			0.6*r.burstState[i] +
				0.4*z/(0.4+0.3*float64(reg)+math.Abs(z))

		out[i] = r.burstState[i]
	}

	return out
}

/* ===== risk ===== */

func (r *PredictiveStabilityRollout) barrierRisk(x []float64, in RolloutInput) float64 {

	w := in.SLAWeight
	if len(w) != r.dim {
		w = ones(r.dim)
	}

	base := 0.0
	for i := 0; i < r.dim; i++ {
		base += w[i] * sigmoid(x[i])
	}

	inter := 0.0
	for i := 0; i < r.dim; i++ {
		for j := i + 1; j < r.dim; j++ {
			inter += math.Abs(x[i] * x[j])
		}
	}

	bar :=
		1 /
			(1 +
				math.Exp(
					-2*(inter-r.tipLevel),
				))

	return clamp(0.5*base/float64(r.dim)+0.5*bar, 0, 1)
}

func (r *PredictiveStabilityRollout) updateTip(risk float64) {

	r.tipLevel += 0.01 * (risk - 0.5)

	r.tipLevel = clamp(r.tipLevel, 0.6, 2.5)
}

/* ===== horizon ===== */

func (r *PredictiveStabilityRollout) adaptiveHorizon(in RolloutInput) int {

	u :=
		math.Sqrt(in.ModelUnc*in.ModelUnc +
			in.HazardUnc*in.HazardUnc)

	h :=
		int(
			float64(r.baseH) *
				(1 + 2*math.Tanh(u)),
		)

	return clampInt(h, 4, 26)
}

/* ===== state feasibility ===== */

func (r *PredictiveStabilityRollout) projectState(x []float64) []float64 {

	for i := range x {

		if x[i] < 0 {
			x[i] = 0
		}

		if x[i] > 6 {
			x[i] = 6 - 0.2*(x[i]-6)
		}
	}

	return x
}

/* ===== utils ===== */

func tanhVec(x []float64) []float64 {

	v := make([]float64, len(x))
	for i := range x {
		v[i] = math.Tanh(x[i])
	}
	return v
}

func mulVec(P [][]float64, x []float64) []float64 {

	v := make([]float64, len(x))
	for i := range P {
		for j := range x {
			v[i] += P[i][j] * x[j]
		}
	}
	return v
}

func mulMatVec(A [][]float64, x []float64) []float64 {

	v := make([]float64, len(x))
	for i := range A {
		for j := range x {
			v[i] += A[i][j] * x[j]
		}
	}
	return v
}

func randSPD(n int) [][]float64 {

	M := randomMatrix(n, n, 1.0)
	C := identityMatrix(n, 1)

	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			C[i][j] += 0.2 * M[i][j] * M[j][i]
		}
	}

	return C
}

func ones(n int) []float64 {

	o := make([]float64, n)
	for i := range o {
		o[i] = 1
	}
	return o
}

// normalizeSpectralRadius rescales matrix A so that its dominant eigenvalue
// (spectral radius) equals targetRadius.
//
// P8: Used to initialize the RLS state-transition matrix A with a guaranteed
// stable spectral radius. Power iteration (20 steps) estimates the dominant
// eigenvalue; every entry of A is then scaled by targetRadius/rho.
// For targetRadius=0.5 the system is strictly stable with a 2x safety margin.
func normalizeSpectralRadius(A [][]float64, targetRadius float64) [][]float64 {
	n := len(A)
	if n == 0 {
		return A
	}

	// Power iteration to estimate the dominant eigenvalue magnitude.
	v := make([]float64, n)
	for i := range v {
		v[i] = 1.0 / math.Sqrt(float64(n)) // unit-norm start
	}

	rho := 1.0
	for iter := 0; iter < 20; iter++ {
		nv := mulMatVec(A, v)
		rho = vecNorm(nv) + 1e-9
		for i := range v {
			v[i] = nv[i] / rho
		}
	}

	if rho < 1e-6 {
		return A // degenerate — return as-is, RLS will correct
	}

	scale := targetRadius / rho
	result := make([][]float64, n)
	for i := range A {
		result[i] = make([]float64, len(A[i]))
		for j := range A[i] {
			result[i][j] = A[i][j] * scale
		}
	}
	return result
}
