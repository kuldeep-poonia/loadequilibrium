package autopilot

import "math"

type IdentificationState struct {

	// arrival estimation
	ArrivalFast      float64
	ArrivalSlow      float64
	ArrivalBlendCoef float64
	ArrivalEstimate  float64
	ArrivalVar       float64
	ArrivalWelfordM  float64
	ArrivalWelfordV  float64

	// bounded burst regime
	BurstEnergy float64

	// disturbance decomposition
	DisturbNoise float64
	ModelDrift   float64

	// rollout reliability (conditional)
	ReliabilityUp   float64
	ReliabilityDown float64

	// prediction errors
	QueueErr float64
	LatErr   float64

	// confidence
	ModelConfidence float64

	// heavy-tail envelope proxy
	ArrivalUpper float64

	// long-horizon regime memory
	SeasonalLoadMemory float64

	// meta-stability supervisory state
	StabilityPressure float64
}

type IdentificationSignals struct {
	MPCVarianceScale  float64
	SafetyMarginScale float64
	RolloutTrustScale float64
	DampingFactor     float64
}

// ArrivalEstimator encapsulates the logic for computing and updating arrival rates.
type ArrivalEstimator interface {
	Update(s IdentificationState, measured float64, e *IdentificationEngine) IdentificationState
}

// LegacyArrivalEstimator uses the historical 1.5x / 10 RPS heuristic threshold.
type LegacyArrivalEstimator struct{}

func (l *LegacyArrivalEstimator) Update(s IdentificationState, measured float64, e *IdentificationEngine) IdentificationState {
	fErr := measured - s.ArrivalFast
	s.ArrivalFast += e.FastGain * fErr

	sErr := measured - s.ArrivalSlow
	s.ArrivalSlow += e.SlowGain * sErr

	if measured > s.ArrivalSlow*1.5 && measured > 10 {
		s.ArrivalFast = measured
		s.ArrivalSlow = measured
	}

	blendError := math.Abs(s.ArrivalFast - s.ArrivalSlow)
	s.ArrivalBlendCoef += e.BlendGain * (blendError - s.ArrivalBlendCoef)
	alpha := s.ArrivalBlendCoef / (1 + s.ArrivalBlendCoef)
	s.ArrivalEstimate = alpha*s.ArrivalFast + (1-alpha)*s.ArrivalSlow
	s.ArrivalVar = (1-e.VarGain)*s.ArrivalVar + e.VarGain*fErr*fErr
	return s
}

// StatisticalArrivalEstimator uses Welford EWMA and Chebyshev bounds for burst detection.
type StatisticalArrivalEstimator struct {
	SigmaMultiplier float64
	NoiseFloorRatio float64
}

func (st *StatisticalArrivalEstimator) Update(s IdentificationState, measured float64, e *IdentificationEngine) IdentificationState {
	// Welford-style EWMA Variance tracking
	if s.ArrivalWelfordM == 0 {
		s.ArrivalWelfordM = measured
	}
	diff := measured - s.ArrivalWelfordM
	s.ArrivalWelfordM += e.VarGain * diff
	s.ArrivalWelfordV = (1-e.VarGain)*s.ArrivalWelfordV + e.VarGain*diff*diff

	fErr := measured - s.ArrivalFast
	s.ArrivalFast += e.FastGain * fErr
	sErr := measured - s.ArrivalSlow
	s.ArrivalSlow += e.SlowGain * sErr

	noiseFloor := math.Max(s.ArrivalEstimate*st.NoiseFloorRatio, 1.0)
	sigma := math.Sqrt(math.Max(s.ArrivalWelfordV, noiseFloor))
	jumpThreshold := s.ArrivalSlow + st.SigmaMultiplier*sigma

	if measured > jumpThreshold {
		s.ArrivalFast = measured
		s.ArrivalSlow = measured
	}

	blendError := math.Abs(s.ArrivalFast - s.ArrivalSlow)
	s.ArrivalBlendCoef += e.BlendGain * (blendError - s.ArrivalBlendCoef)
	alpha := s.ArrivalBlendCoef / (1 + s.ArrivalBlendCoef)
	s.ArrivalEstimate = alpha*s.ArrivalFast + (1-alpha)*s.ArrivalSlow
	
	// Preserve old ArrivalVar for backwards compatibility with other systems if any
	s.ArrivalVar = (1-e.VarGain)*s.ArrivalVar + e.VarGain*fErr*fErr

	return s
}

type IdentificationEngine struct {
	Dt float64

	FastGain  float64
	SlowGain  float64
	BlendGain float64
	VarGain   float64

	BurstGain  float64
	BurstDecay float64
	BurstCap   float64

	NoiseGain float64
	DriftGain float64

	BaseConfidenceFloor float64
	ConfidenceGain      float64

	ReliabilityGain  float64
	InfraSensitivity float64

	SLAWeightQueue   float64
	SLAWeightLatency float64

	EVTFactor float64

	SeasonalGain float64

	DampingGain float64

	// Strategy configuration
	ArrivalStrategy ArrivalEstimator
}

/*
arrival estimation with adaptive blending
*/
func (e *IdentificationEngine) updateArrival(
	s IdentificationState,
	measured float64,
) IdentificationState {
	if e.ArrivalStrategy == nil {
		e.ArrivalStrategy = &LegacyArrivalEstimator{}
	}
	return e.ArrivalStrategy.Update(s, measured, e)
}

/*
bounded burst regime energy
*/
func (e *IdentificationEngine) updateBurst(
	s IdentificationState,
	measured float64,
) IdentificationState {

	// CRITICAL FIX: prevent ArrivalVar-near-zero explosion.
	//
	// The 10-tick warmup at constant arrival decays ArrivalVar toward zero
	// (VarGain=0.3 → ArrivalVar *= 0.7^10 ≈ 0.028). Any load change then produces
	// excess = Δarrival / sqrt(0.028) ≈ Δarrival/0.167, causing BurstEnergy to max
	// out on every tick of a rising-load scenario — a pure measurement artifact.
	//
	// Fix: denominator floor = max(10% of estimated mean, 1.0).
	// A 1.67 rps increment on a 20 rps baseline → excess = 1.67/2.0 = 0.84 (real).
	// An actual burst doubling load (20→40) → excess = 20/2.0 = 10 (correctly high).
	relativeFloor := math.Max(s.ArrivalEstimate*0.10, 1.0)
	denom := math.Max(relativeFloor, math.Sqrt(s.ArrivalVar+1e-6))
	excess := (measured - s.ArrivalEstimate) / denom

	b :=
		math.Max(0, excess)

	// Mathematical upgrade: Adaptive Decay based on fast-to-slow variance ratio.
	// If ArrivalFast > ArrivalSlow, we are in an active burst; decay slowly to retain memory.
	// If ArrivalFast <= ArrivalSlow, burst is over; decay rapidly.
	ratio := 1.0
	if s.ArrivalSlow > 0 {
		ratio = s.ArrivalFast / s.ArrivalSlow
	}

	// Base decay is e.BurstDecay. We modulate it via the EWMA ratio.
	adaptiveDecay := e.BurstDecay
	if ratio > 1.05 {
		// Active burst: reduce decay (half-life extends) mathematically by square of intensity
		adaptiveDecay = e.BurstDecay / (ratio * ratio)
	} else {
		// Recovery: accelerate decay to clear phantom energy
		adaptiveDecay = e.BurstDecay * 2.0
	}

	if adaptiveDecay > 1.0 {
		adaptiveDecay = 1.0
	}
	if adaptiveDecay < 0.01 {
		adaptiveDecay = 0.01
	}

	s.BurstEnergy =
		(1-adaptiveDecay)*s.BurstEnergy +
			e.BurstGain*b

	if s.BurstEnergy > e.BurstCap {
		s.BurstEnergy = e.BurstCap
	}

	return s
}

/*
disturbance decomposition
*/
func (e *IdentificationEngine) updateDisturbance(
	s IdentificationState,
	residual float64,
) IdentificationState {

	s.DisturbNoise +=
		e.NoiseGain *
			(math.Abs(residual) - s.DisturbNoise)

	s.ModelDrift +=
		e.DriftGain *
			(residual - s.ModelDrift)

	return s
}

/*
error tracking
*/
func (e *IdentificationEngine) updateErrors(
	s IdentificationState,
	qErr float64,
	lErr float64,
) IdentificationState {

	s.QueueErr = qErr
	s.LatErr = lErr

	return s
}

/*
dynamic confidence floor
*/
func (e *IdentificationEngine) confidenceFloor(
	slaPressure float64,
	maturity float64,
) float64 {

	return e.BaseConfidenceFloor *
		(1 + slaPressure) *
		(1 + (1 - maturity))
}

/*
confidence update
*/
func (e *IdentificationEngine) updateConfidence(
	s IdentificationState,
	mpcQuality float64,
	safetyRate float64,
	queuePressure float64,
	slaPressure float64,
	maturity float64,
) IdentificationState {

	err :=
		e.SLAWeightQueue*math.Abs(s.QueueErr) +
			e.SLAWeightLatency*math.Abs(s.LatErr)

	loss :=
		err / (1 + err)

	external :=
		0.4*mpcQuality +
			0.3*(1-safetyRate) +
			0.3*(1-queuePressure)

	target :=
		(1 - loss) * external

	floor :=
		e.confidenceFloor(
			slaPressure,
			maturity,
		)

	s.ModelConfidence =
		math.Max(
			floor,
			(1-e.ConfidenceGain)*s.ModelConfidence+
				e.ConfidenceGain*target,
		)

	return s
}

/*
conditional reliability learning
*/
func (e *IdentificationEngine) updateReliability(
	s IdentificationState,
	success bool,
	scaleDelta float64,
	infraLoad float64,
) IdentificationState {

	v := 0.0
	if success {
		v = 1
	}

	w :=
		1 / (1 + e.InfraSensitivity*infraLoad)

	if scaleDelta >= 0 {

		s.ReliabilityUp =
			(1-e.ReliabilityGain)*s.ReliabilityUp +
				e.ReliabilityGain*v*w

	} else {

		s.ReliabilityDown =
			(1-e.ReliabilityGain)*s.ReliabilityDown +
				e.ReliabilityGain*v*w
	}

	return s
}

/*
EVT-style heavy-tail envelope proxy
*/
func (e *IdentificationEngine) updateEnvelope(
	s IdentificationState,
) IdentificationState {

	s.ArrivalUpper =
		s.ArrivalEstimate +
			e.EVTFactor*
				math.Sqrt(s.ArrivalVar+1e-6)*
				(1+math.Log1p(s.BurstEnergy))

	return s
}

/*
seasonal meta-memory
*/
func (e *IdentificationEngine) updateSeasonal(
	s IdentificationState,
	load float64,
) IdentificationState {

	s.SeasonalLoadMemory =
		(1-e.SeasonalGain)*s.SeasonalLoadMemory +
			e.SeasonalGain*load

	return s
}

/*
meta-stability supervisory damping
*/
func (e *IdentificationEngine) updateDamping(
	s IdentificationState,
) IdentificationState {

	instability :=
		math.Abs(s.QueueErr) +
			math.Abs(s.LatErr)

	s.StabilityPressure =
		(1-e.DampingGain)*s.StabilityPressure +
			e.DampingGain*
				math.Tanh(instability)

	return s
}

/*
adaptive signals output
*/
func (e *IdentificationEngine) signals(
	s IdentificationState,
) IdentificationSignals {
	conf := finiteClamp01(s.ModelConfidence)
	burst := finiteNonNegative(s.BurstEnergy)
	noise := finiteNonNegative(math.Abs(s.DisturbNoise))
	seasonal := finiteClamp(s.SeasonalLoadMemory, 0, 2)
	pressure := finiteClamp(s.StabilityPressure, 0, 2)

	// Dead-zone: residual burst/noise below 0.15 is sensor noise, not real signal.
	if burst < 0.15 {
		burst = 0
	}
	if noise < 0.15 {
		noise = 0
	}

	variance :=
		math.Min(
			3,
			1+0.4*burst+
				0.6*noise,
		)

	safety :=
		math.Min(
			3,
			1+(1-conf)+
				seasonal,
		)

	trust :=
		0.3 +
			0.7*math.Max(
				s.ReliabilityUp,
				s.ReliabilityDown,
			)

	damping :=
		1 + pressure

	return IdentificationSignals{
		MPCVarianceScale:  variance,
		SafetyMarginScale: safety,
		RolloutTrustScale: trust,
		DampingFactor:     damping,
	}
}

/*
main step
*/
func (e *IdentificationEngine) Step(
	s IdentificationState,

	measuredArrival float64,
	queueErr float64,
	latErr float64,
	residual float64,

	mpcQuality float64,
	safetyRate float64,
	queuePressure float64,

	slaPressure float64,
	maturity float64,

	rolloutSuccess bool,
	scaleDelta float64,
	infraLoad float64,
	totalLoad float64,

	pdeDensity float64,
	stabilityMargin float64,
) (IdentificationState, IdentificationSignals) {

	// Dynamically adjust inverse time constants (Gains) based on PDE/SDE physics.
	// When PDE shockwaves approach (density > 0.8) or stability margin collapses (<0.2),
	// the time constant must shorten (gain increases) to rapidly track changing conditions.
	// In calm state, gains revert to baseline to filter noise.
	shockwaveMultiplier := 1.0
	if pdeDensity > 0.8 {
		shockwaveMultiplier += (pdeDensity - 0.8) * 2.0 // Boost up to +40% if density approaches 1.0
	}
	if stabilityMargin > 0 && stabilityMargin < 0.2 {
		shockwaveMultiplier += (0.2 - stabilityMargin) * 5.0 // Boost up to +100% on collapse
	}
	shockwaveMultiplier = math.Min(shockwaveMultiplier, 3.0)

	dynEngine := *e
	dynEngine.FastGain = math.Min(e.FastGain*shockwaveMultiplier, 1.0)
	dynEngine.SlowGain = math.Min(e.SlowGain*shockwaveMultiplier, 1.0)
	dynEngine.VarGain = math.Min(e.VarGain*shockwaveMultiplier, 1.0)

	next := s

	next =
		dynEngine.updateArrival(next, measuredArrival)

	next =
		dynEngine.updateBurst(next, measuredArrival)

	next =
		dynEngine.updateDisturbance(next, residual)

	next =
		dynEngine.updateErrors(next, queueErr, latErr)

	next =
		dynEngine.updateConfidence(
			next,
			mpcQuality,
			safetyRate,
			queuePressure,
			slaPressure,
			maturity,
		)

	next =
		dynEngine.updateReliability(
			next,
			rolloutSuccess,
			scaleDelta,
			infraLoad,
		)

	next =
		dynEngine.updateEnvelope(next)

	next =
		dynEngine.updateSeasonal(next, totalLoad)

	next =
		dynEngine.updateDamping(next)

	next = sanitizeIdentificationState(next)

	sig :=
		dynEngine.signals(next)

	return next, sig
}

func sanitizeIdentificationState(s IdentificationState) IdentificationState {
	s.ArrivalFast = finiteNonNegative(s.ArrivalFast)
	s.ArrivalSlow = finiteNonNegative(s.ArrivalSlow)
	s.ArrivalBlendCoef = finiteNonNegative(s.ArrivalBlendCoef)
	s.ArrivalEstimate = finiteNonNegative(s.ArrivalEstimate)
	s.ArrivalVar = finiteNonNegative(s.ArrivalVar)
	s.BurstEnergy = finiteNonNegative(s.BurstEnergy)
	s.DisturbNoise = finiteOrZero(s.DisturbNoise)
	s.ModelDrift = finiteOrZero(s.ModelDrift)
	s.ReliabilityUp = finiteClamp01(s.ReliabilityUp)
	s.ReliabilityDown = finiteClamp01(s.ReliabilityDown)
	s.QueueErr = finiteOrZero(s.QueueErr)
	s.LatErr = finiteOrZero(s.LatErr)
	s.ModelConfidence = finiteClamp01(s.ModelConfidence)
	s.ArrivalUpper = finiteNonNegative(s.ArrivalUpper)
	s.SeasonalLoadMemory = finiteClamp(s.SeasonalLoadMemory, 0, 2)
	s.StabilityPressure = finiteClamp(s.StabilityPressure, 0, 2)
	return s
}

func finiteOrZero(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return v
}

func finiteNonNegative(v float64) float64 {
	v = finiteOrZero(v)
	if v < 0 {
		return 0
	}
	return v
}

func finiteClamp01(v float64) float64 {
	return finiteClamp(v, 0, 1)
}

func finiteClamp(v, lo, hi float64) float64 {
	v = finiteOrZero(v)
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
