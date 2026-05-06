package intelligence

import "time"

type RuntimeInput struct {
	State          []float64
	Risk           float64
	RiskForecast   []float64
	HazardUnc      float64
	ModelUnc       float64
	StabilityVec   []float64
	Perf           float64
	PerfTrend      float64
	CapacityPress  float64
	SLASeverity    float64
	EntropyProxy   float64
	GradProxy      float64
	Novelty        float64
	Regime         int
	GovernanceHint int
	Policy         func([]float64) []float64
	PolicyUnc      float64
}

type RuntimeOutput struct {
	Action         []float64
	AutonomyLevel  float64
	GovernanceMode int
	Fallback       bool
}

type HazardOut struct {
	Mean           float64
	Uncertainty    float64
	EpistemicTrend float64
}

type HazardEstimator interface {
	Estimate(state, action []float64) HazardOut
}

type SignalLearner interface {
	Update(state []float64)
}

func (c *HazardValueCritic) Estimate(state, action []float64) HazardOut {
	feat := make([]float64, c.fDim)
	copy(feat, state)
	offset := len(state)
	if offset > len(feat) {
		offset = len(feat)
	}
	for i := 0; offset+i < len(feat) && i < len(action); i++ {
		feat[offset+i] = action[i]
	}

	meanHaz, aleatoric, epistemic := c.Predict(feat, 0)

	return HazardOut{
		Mean:           meanHaz,
		Uncertainty:    aleatoric + epistemic,
		EpistemicTrend: epistemic - aleatoric,
	}
}

type AdaptiveSignalAdapter struct {
	learner *AdaptiveSignalLearner
}

func NewAdaptiveSignalAdapter(learner *AdaptiveSignalLearner) *AdaptiveSignalAdapter {
	return &AdaptiveSignalAdapter{learner: learner}
}

func (a *AdaptiveSignalAdapter) Update(state []float64) {
	if a == nil || a.learner == nil {
		return
	}

	a.learner.Update(signalVectorFromState(state))
}

type PGRuntimePolicy struct {
	opt *PolicyGradientOptimizer
}

func NewPGRuntimePolicy(opt *PolicyGradientOptimizer) *PGRuntimePolicy {
	return &PGRuntimePolicy{opt: opt}
}

func (p *PGRuntimePolicy) Policy(state []float64) []float64 {
	if p == nil || p.opt == nil {
		return make([]float64, 4)
	}

	act := p.opt.Act(state)
	return []float64{
		act.ScaleOut,
		act.RetryBackoff,
		act.QueueShard,
		act.CacheBoost,
	}
}

func (p *PGRuntimePolicy) TryStep(
	state, action []float64,
	hazard, perf float64,
	elapsed time.Duration,
) {
	if p == nil || p.opt == nil {
		return
	}

	reward := perf - hazard - 0.05*elapsed.Seconds() - 0.01*vecNorm(action)
	p.opt.Observe(state, reward, hazard, false)
}

func (f *AutonomyDecisionFusion) CombineStrategic(
	uPolicy []float64,
	uPrev []float64,
	autoLevel float64,
	safetyGain float64,
	mode int,
	riskVec []float64,
	hz HazardOut,
) []float64 {
	out := f.Fuse(FusionInput{
		Stability:    riskVec,
		PolicyAction: uPolicy,
		MPCAction:    uPrev,
		PolicyUnc:    1 - autoLevel,
		MPCUnc:       safetyGain,
		HazardProb:   hz.Mean,
		RiskForecast: riskVec,
		Epistemic:    hz.Uncertainty,
		RegimeID:     mode,
		PerfSignal:   autoLevel,
		SLASeverity:  safetyGain,
	})

	return out.Action
}

func signalVectorFromState(state []float64) SignalVector {
	return SignalVector{
		Timestamp:          time.Now(),
		BacklogError:       stateAt(state, 0),
		LatencyError:       stateAt(state, 1),
		ErrorRateError:     stateAt(state, 2),
		CPUError:           stateAt(state, 3),
		QueueDrift:         stateAt(state, 0) - stateAt(state, 1),
		RetryAmplification: stateAt(state, 3),
	}
}

func stateAt(state []float64, idx int) float64 {
	if idx < 0 || idx >= len(state) {
		return 0
	}
	return state[idx]
}
