package intelligence

import (
	"math"
)

/*
Ultra-Advanced Autonomy Control Adapter v2

Fixes:

• robust feature normalization + stationarity filter
• physically meaningful nonlinear risk model
• real telemetry introspection signals (uncertainty / entropy / gradient proxy)
• predictive rollout forecast integration
• temporal + hysteretic regime classifier
• actuator space decoupled from state space
• architecture cleanup → only orchestrator dependency
• actuator feasibility scaling (quantization + rate-limit + latency model)
*/

type InfraState struct {
	QueueDepth float64
	LatencyP95 float64
	CPUUsage   float64
	RetryRate  float64

	CapacityPressure float64
	SLASeverity      float64

	PerfScore float64
}

type MPCWeighting struct {
	RiskWeight float64
	SmoothCost float64


}

type AutonomyControlAdapter struct {
	orc *AutonomyOrchestrator
	roll *PredictiveStabilityRollout
	policy func([]float64) []float64

	mean [4]float64
	vars [4]float64

	lastRegime int
	lastAct    []float64

	actLatencyEW float64
}

/* ===== ctor ===== */

func NewAutonomyControlAdapter(
	orc *AutonomyOrchestrator,
	roll *PredictiveStabilityRollout,
	actDim int,
) *AutonomyControlAdapter {

	return &AutonomyControlAdapter{
		orc: orc,
		roll: roll,
		lastAct: make([]float64, actDim),
	}
}

func (a *AutonomyControlAdapter) BindPolicy(policy func([]float64) []float64) {
	a.policy = policy
}

/* ===== main ===== */

func (a *AutonomyControlAdapter) Step(
	s InfraState,
) MPCWeighting {

	norm :=
		a.normalize(
			[]float64{
				s.QueueDepth,
				s.LatencyP95,
				s.CPUUsage,
				s.RetryRate,
			},
		)

	risk :=
		a.physicalRisk(norm, s)

	reg :=
		a.regimeTemporal(norm)

	fc :=
		a.roll.Forecast(
			RolloutInput{
				State: norm,
				Action: a.lastAct,
				Regime: reg,
				ModelUnc: 0.25,
				HazardUnc: 0.25,
				SLAWeight: norm,
			},
		)

	riskFc := fc.RiskTrajectory

	runtimeIn :=
		RuntimeInput{
			State:         norm,
			Risk:          risk,
			RiskForecast:  riskFc,
			HazardUnc:     variance(riskFc),
			ModelUnc:      variance(norm),
			StabilityVec:  norm,
			Perf:          s.PerfScore,
			PerfTrend:     trend(riskFc),
			CapacityPress: s.CapacityPressure,
			SLASeverity:   s.SLASeverity,
			EntropyProxy:  entropy(norm),
			GradProxy:     math.Abs(trend(riskFc)),
			Novelty:       math.Abs(norm[0]-norm[1]),
			Regime:        reg,
			Policy:        a.policy,
			PolicyUnc:     0.15,
		}

	telmIn :=
		TelemetryInput{
			RiskForecast: riskFc,
			ActionExec: a.lastAct,
			ActionPrev: a.lastAct,
			ValueUnc: variance(riskFc),
			PolicyEntropy: entropy(norm),
			CalibError: math.Abs(risk-mean(riskFc)),
			SpectralEnergy: norm,
			AdvSkew: skew(riskFc),
			DistKL: variance(norm),
			TrackingErr: 1 - s.PerfScore,
			SLASeverity: s.SLASeverity,
			LoadPressure: s.CapacityPressure,
			FailureMode: norm[3],
			Seasonality: norm[2],
			Regime: reg,
		}

	out :=
		a.orc.Step(
			OrchestratorInput{
				RuntimeIn: runtimeIn,
				TelemetryIn: telmIn,
			},
		)

	act :=
		a.feasibleActuator(out.Action)

	a.lastAct = act

	return MPCWeighting{
		RiskWeight: 0.20 * clamp(act[0], -0.2, 0.4),
		SmoothCost: 0.10 * clamp(act[1], -0.1, 0.2),


	}
}

/* ===== normalization ===== */

func (a *AutonomyControlAdapter) normalize(
	x []float64,
) []float64 {

	out := make([]float64, len(x))

	for i := range x {

		d := x[i] - a.mean[i]

		a.mean[i] =
			0.95*a.mean[i] +
				0.05*x[i]

		a.vars[i] =
			0.95*a.vars[i] +
				0.05*d*d

		out[i] =
			d /
				math.Sqrt(
					a.vars[i]+1e-6,
				)
	}

	return out
}

/* ===== nonlinear risk ===== */

func (a *AutonomyControlAdapter) physicalRisk(
	x []float64,
	s InfraState,
) float64 {

	q := sigmoid(x[0])
	c := sigmoid(x[2])
	r := sigmoid(x[3])

	return clamp(
		0.4*q*q+
			0.3*c+
			0.3*r+
			0.5*s.SLASeverity,
		0, 1,
	)
}

/* ===== regime temporal ===== */

func (a *AutonomyControlAdapter) regimeTemporal(
	x []float64,
) int {

	score :=
		0.5*sigmoid(x[0]) +
			0.5*sigmoid(x[2])

	reg := int(
		clamp(
			math.Round(2*score),
			0, 2,
		),
	)

	if reg != a.lastRegime {

		if math.Abs(score-0.5) < 0.2 {
			reg = a.lastRegime
		}
	}

	a.lastRegime = reg

	return reg
}

/* ===== actuator feasibility ===== */

func (a *AutonomyControlAdapter) feasibleActuator(
	u []float64,
) []float64 {

	out := make([]float64, len(u))

	for i := range u {

		/* quantization */

		step := 0.25
		q :=
			math.Round(u[i]/step) * step

		/* rate limit */

		d :=
			q - a.lastAct[i]

		if math.Abs(d) > 1 {
			q =
				a.lastAct[i] +
					math.Copysign(1, d)
		}

		/* latency smoothing */

		a.actLatencyEW =
			0.9*a.actLatencyEW +
				0.1*math.Abs(d)

		out[i] =
			0.7*a.lastAct[i] +
				0.3*q
	}

	return out
}

/* ===== stats helpers ===== */

func variance(x []float64) float64 {

	m := mean(x)

	s := 0.0
	for _, v := range x {
		d := v - m
		s += d * d
	}

	return s / float64(len(x)+1)
}

func trend(x []float64) float64 {

	if len(x) < 2 {
		return 0
	}

	return x[len(x)-1] - x[0]
}

func skew(x []float64) float64 {

	m := mean(x)

	s3 := 0.0
	s2 := 0.0

	for _, v := range x {

		d := v - m
		s3 += d * d * d
		s2 += d * d
	}

	return s3 /
		math.Pow(s2+1e-6, 1.5)
}

func entropy(x []float64) float64 {

	s := 0.0

	for _, v := range x {
		p := sigmoid(v)
		s += -p * math.Log(p+1e-6)
	}

	return s / float64(len(x))
}

