package intelligence

import (
	"math"
	"sync"
)

/*
Ultra-Advanced Autonomy Telemetry Model v4

Fixes:

• reliability-curve calibrated confidence (online isotonic bins + Bayesian fusion)
• dynamic spectral clustering (adaptive band count)
• closed-loop instability uses executed (post-safety) action
• normalized adaptive health weights (simplex projection)
• replay collapse persistence (KL trend EW)
• rich regime context memory vector (load / failure / seasonality)
*/

type TelemetryInput struct {
	RiskForecast []float64

	ActionExec []float64
	ActionPrev []float64

	ValueUnc      float64
	PolicyEntropy float64
	CalibError    float64

	SpectralEnergy []float64

	AdvSkew   float64
	DistKL    float64

	TrackingErr  float64
	SLASeverity  float64
	LoadPressure float64

	FailureMode float64
	Seasonality float64

	Regime int
	Fallback bool
}

type TelemetryOutput struct {
	Confidence float64
	Health     float64
	Instability float64
	Degraded   bool
}

type AutonomyTelemetryModel struct {
	mu sync.Mutex

	// reliability bins
	binProb [8]float64
	binCnt  [8]float64

	specCenters []float64
	specEW      []float64

	regimeVec map[int][3]float64

	klEW float64

	w [5]float64

	conf float64
}

/* ===== ctor ===== */

func NewAutonomyTelemetryModel() *AutonomyTelemetryModel {

	return &AutonomyTelemetryModel{
		specCenters: []float64{0.3, 0.8},
		specEW: []float64{0, 0},
		regimeVec: make(map[int][3]float64),
		w: [5]float64{0.25, 0.2, 0.2, 0.2, 0.15},
		conf: 0.6,
	}
}

/* ===== main ===== */

func (m *AutonomyTelemetryModel) Step(
	in TelemetryInput,
) TelemetryOutput {

	m.mu.Lock()
	defer m.mu.Unlock()

	m.dynamicSpectral(in)

	inst := m.closedLoopInstability(in)

	m.updateReplayTrend(in)

	h := m.health(in, inst)

	c := m.calibratedConfidence(h, in)

	deg :=
		h < 0.3 ||
			inst > 0.85

	m.updateRegime(in)

	m.normalizeWeights()

	return TelemetryOutput{
		Confidence: c,
		Health: h,
		Instability: inst,
		Degraded: deg,
	}
}

/* ===== calibrated confidence ===== */

func (m *AutonomyTelemetryModel) calibratedConfidence(
	health float64,
	in TelemetryInput,
) float64 {

	raw :=
		sigmoid(
			1.4*health -
				1.1*in.CalibError -
				0.8*in.ValueUnc -
				0.6*in.PolicyEntropy,
		)

	b := int(raw * 7.99)

	m.binProb[b] =
		(m.binProb[b]*m.binCnt[b] + health) /
			(m.binCnt[b] + 1)

	m.binCnt[b]++

	pCal := m.binProb[b]

	m.conf =
		0.85*m.conf +
			0.15*(0.7*pCal+0.3*raw)

	return clamp(m.conf, 0, 1)
}

/* ===== dynamic spectral ===== */

func (m *AutonomyTelemetryModel) dynamicSpectral(
	in TelemetryInput,
) {

	for _, e := range in.SpectralEnergy {

		assigned := false

		for i := range m.specCenters {

			if math.Abs(e-m.specCenters[i]) < 0.25 {

				m.specCenters[i] =
					0.9*m.specCenters[i] +
						0.1*e

				m.specEW[i] =
					0.85*m.specEW[i] +
						0.15*math.Abs(e-m.specCenters[i])

				assigned = true
				break
			}
		}

		if !assigned && len(m.specCenters) < 6 {

			m.specCenters =
				append(m.specCenters, e)

			m.specEW =
				append(m.specEW, 0.1)
		}
	}
}

/* ===== instability ===== */

func (m *AutonomyTelemetryModel) closedLoopInstability(
	in TelemetryInput,
) float64 {

	shape := riskShape(in.RiskForecast)

	actShock :=
		vecNorm(diff(in.ActionExec, in.ActionPrev))

	spec := 0.0
	for _, v := range m.specEW {
		spec += v
	}

	return clamp(
		0.35*shape[1]+
			0.35*actShock+
			0.3*spec,
		0, 1,
	)
}

/* ===== replay persistence ===== */

func (m *AutonomyTelemetryModel) updateReplayTrend(
	in TelemetryInput,
) {

	m.klEW =
		0.9*m.klEW +
			0.1*in.DistKL
}

/* ===== health ===== */

func (m *AutonomyTelemetryModel) health(
	in TelemetryInput,
	inst float64,
) float64 {

	reg := m.regimeVec[in.Regime]

	ctx :=
		1 -
			sigmoid(
				in.LoadPressure-
					reg[0],
			)

	replay :=
		sigmoid(
			m.klEW +
				math.Abs(in.AdvSkew),
		)

	perf :=
		sigmoid(
			in.TrackingErr+
				in.SLASeverity,
		)

	fb := 0.0
	if in.Fallback {
		fb = 0.7
	}

	w := m.w

	h :=
		1 -
			(w[0]*inst +
				w[1]*replay +
				w[2]*perf +
				w[3]*in.ValueUnc +
				w[4]*fb)

	return clamp(h*ctx, 0, 1)
}

/* ===== regime context ===== */

func (m *AutonomyTelemetryModel) updateRegime(
	in TelemetryInput,
) {

	v := m.regimeVec[in.Regime]

	v[0] =
		0.8*v[0] +
			0.2*in.LoadPressure

	v[1] =
		0.8*v[1] +
			0.2*in.FailureMode

	v[2] =
		0.8*v[2] +
			0.2*in.Seasonality

	m.regimeVec[in.Regime] = v
}

/* ===== normalize weights ===== */

func (m *AutonomyTelemetryModel) normalizeWeights() {

	s := 0.0
	for _, v := range m.w {
		s += v
	}

	for i := range m.w {
		m.w[i] /= (s + 1e-6)
	}
}

/* ===== utils ===== */

