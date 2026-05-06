package intelligence

import (
	"math"
	"sync"
)



type MetaInput struct {
	GlobalRisk float64
	RiskForecast []float64

	HazardUnc float64
	ModelUnc  float64
	EpistemicTrend float64

	PerfSignal float64
	PerfTrend  float64

	StabilityMargin float64
	EntropyProxy float64
	GradMagProxy float64
	ReplayNovelty float64

	CapacityPressure float64

	SLASeverity float64
	OscPenalty float64

	Regime int
}

type MetaOutput struct {
	AutonomyLevel   float64
	SafetyGain      float64
	ExplorationGate float64
	GovernanceMode  int
}

type MetaAutonomyController struct {
	mu sync.Mutex

	level float64

	modeBelief [3]float64 // autonomous / supervised / safety

	weights [6]float64 // adaptive governance weights

	regimeScore map[int]float64

	trendEW float64
}

/* ===== ctor ===== */

func NewMetaAutonomyController() *MetaAutonomyController {

	return &MetaAutonomyController{
		level: 0.5,
		modeBelief: [3]float64{0.4, 0.4, 0.2},
		weights: [6]float64{1.2, 1.1, 0.9, 0.8, 0.7, 0.6},
		regimeScore: make(map[int]float64),
	}
}

/* ===== main ===== */

func (m *MetaAutonomyController) Step(in MetaInput) MetaOutput {

	m.mu.Lock()
	defer m.mu.Unlock()

	rVec := m.riskShape(in.RiskForecast)

	obj := m.metaObjective(in, rVec)

	m.level =
		0.9*m.level +
			0.1*sigmoid(obj)

	m.updateModeBelief(in, rVec)

	sGain := m.marginSafety(in)

	explore := m.intelligentExplore(in)

	m.learnWeights(in, rVec)

	m.learnRegime(in)

	return MetaOutput{
		AutonomyLevel: m.level,
		SafetyGain: sGain,
		ExplorationGate: explore,
		GovernanceMode: argmax3(m.modeBelief),
	}
}

/* ===== objective ===== */

func (m *MetaAutonomyController) metaObjective(
	in MetaInput,
	rVec [3]float64,
) float64 {

	w := m.weights

	regBias := m.regimeScore[in.Regime]

	return w[0]*(in.PerfSignal-0.5) -
		w[1]*rVec[0] -
		w[2]*in.SLASeverity +
		w[3]*in.StabilityMargin -
		w[4]*in.OscPenalty +
		w[5]*regBias +
		0.6*in.PerfTrend -
		0.5*in.CapacityPressure
}

/* ===== risk shape ===== */

func (m *MetaAutonomyController) riskShape(
	rf []float64,
) [3]float64 {

	var spike, drift, mean float64

	n := float64(len(rf)) + 1e-6

	for i := range rf {

		mean += rf[i]

		if i > 0 {
			drift += math.Abs(rf[i] - rf[i-1])
		}

		if rf[i] > spike {
			spike = rf[i]
		}
	}

	mean /= n
	drift /= n

	return [3]float64{mean, drift, spike}
}

/* ===== exploration ===== */

func (m *MetaAutonomyController) intelligentExplore(
	in MetaInput,
) float64 {

	learnSignal :=
		0.4*in.EntropyProxy +
			0.3*in.GradMagProxy +
			0.3*in.ReplayNovelty

	riskSupp :=
		1 -
			sigmoid(
				in.GlobalRisk +
					in.EpistemicTrend,
			)

	return clamp(
		learnSignal*riskSupp*(1-m.level+0.2),
		0,
		1,
	)
}

/* ===== safety ===== */

func (m *MetaAutonomyController) marginSafety(
	in MetaInput,
) float64 {

	base :=
		1 -
			sigmoid(
				in.StabilityMargin-
					in.GlobalRisk,
			)

	return clamp(base*(1-m.level+0.25), 0, 2)
}

/* ===== mode belief ===== */

func (m *MetaAutonomyController) updateModeBelief(
	in MetaInput,
	rVec [3]float64,
) {

	scoreAuto :=
		1 - rVec[0] +
			0.5*in.PerfSignal

	scoreSup :=
		rVec[1] +
			0.3*in.ModelUnc

	scoreSafe :=
		rVec[2] +
			in.SLASeverity

	m.modeBelief =
		softmax3(
			blend3(
				m.modeBelief,
				[3]float64{
					scoreAuto,
					scoreSup,
					scoreSafe,
				},
				0.15,
			),
		)
}

/* ===== meta weight learning ===== */

func (m *MetaAutonomyController) learnWeights(
	in MetaInput,
	rVec [3]float64,
) {

	grad :=
		(in.PerfSignal-0.5) -
			rVec[0] -
			in.SLASeverity

	for i := range m.weights {

		m.weights[i] +=
			0.01 * grad

		m.weights[i] =
			clamp(m.weights[i], 0.2, 2.5)
	}

	m.trendEW =
		0.92*m.trendEW +
			0.08*math.Abs(in.PerfTrend)
}

/* ===== regime ===== */

func (m *MetaAutonomyController) learnRegime(
	in MetaInput,
) {

	r := m.regimeScore[in.Regime]

	r +=
		0.05*(in.PerfSignal-0.5) +
			0.04*in.StabilityMargin -
			0.06*in.SLASeverity -
			0.03*in.OscPenalty

	m.regimeScore[in.Regime] =
		clamp(r, -2, 2)
}

/* ===== utils ===== */

func softmax3(x [3]float64) [3]float64 {

	m := math.Max(x[0], math.Max(x[1], x[2]))

	var s float64
	var e [3]float64

	for i := 0; i < 3; i++ {
		e[i] = math.Exp(x[i] - m)
		s += e[i]
	}

	for i := 0; i < 3; i++ {
		e[i] /= s
	}

	return e
}

func blend3(a, b [3]float64, k float64) [3]float64 {

	return [3]float64{
		(1-k)*a[0] + k*b[0],
		(1-k)*a[1] + k*b[1],
		(1-k)*a[2] + k*b[2],
	}
}

func argmax3(x [3]float64) int {

	if x[0] > x[1] && x[0] > x[2] {
		return 0
	}
	if x[1] > x[2] {
		return 1
	}
	return 2
}

