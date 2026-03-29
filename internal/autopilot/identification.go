package autopilot

import "math"



type IdentificationState struct {

	// arrival estimation
	ArrivalFast float64
	ArrivalSlow float64
	ArrivalBlendCoef float64
	ArrivalEstimate float64
	ArrivalVar float64

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

	MPCVarianceScale float64
	SafetyMarginScale float64
	RolloutTrustScale float64
	DampingFactor float64
}

type IdentificationEngine struct {

	Dt float64

	FastGain float64
	SlowGain float64
	BlendGain float64
	VarGain float64

	BurstGain float64
	BurstDecay float64
	BurstCap float64

	NoiseGain float64
	DriftGain float64

	BaseConfidenceFloor float64
	ConfidenceGain float64

	ReliabilityGain float64
	InfraSensitivity float64

	SLAWeightQueue float64
	SLAWeightLatency float64

	EVTFactor float64

	SeasonalGain float64

	DampingGain float64
}

/*
arrival estimation with adaptive blending
*/
func (e *IdentificationEngine) updateArrival(
	s IdentificationState,
	measured float64,
) IdentificationState {

	fErr := measured - s.ArrivalFast
	s.ArrivalFast += e.FastGain * fErr

	sErr := measured - s.ArrivalSlow
	s.ArrivalSlow += e.SlowGain * sErr

	blendError :=
		math.Abs(s.ArrivalFast - s.ArrivalSlow)

	s.ArrivalBlendCoef +=
		e.BlendGain *
			(blendError - s.ArrivalBlendCoef)

	alpha :=
		s.ArrivalBlendCoef /
			(1 + s.ArrivalBlendCoef)

	s.ArrivalEstimate =
		alpha*s.ArrivalFast +
			(1-alpha)*s.ArrivalSlow

	s.ArrivalVar =
		(1-e.VarGain)*s.ArrivalVar +
			e.VarGain*fErr*fErr

	return s
}

/*
bounded burst regime energy
*/
func (e *IdentificationEngine) updateBurst(
	s IdentificationState,
	measured float64,
) IdentificationState {

	excess :=
		(measured - s.ArrivalEstimate) /
			math.Sqrt(s.ArrivalVar+1e-6)

	b :=
		math.Max(0, excess)

	s.BurstEnergy =
		(1-e.BurstDecay)*s.BurstEnergy +
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
			(residual - s.DisturbNoise)

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
	s.LatErr   = lErr

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
		(1-loss)*external

	floor :=
		e.confidenceFloor(
			slaPressure,
			maturity,
		)

	s.ModelConfidence =
		math.Max(
			floor,
			(1-e.ConfidenceGain)*s.ModelConfidence +
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

	variance :=
		math.Min(
			3,
			1+0.4*s.BurstEnergy+
				0.6*math.Abs(s.DisturbNoise),
		)

	safety :=
		math.Min(
			3,
			1+(1-s.ModelConfidence)+
				s.SeasonalLoadMemory,
		)

	trust :=
		0.3 +
			0.7*math.Max(
				s.ReliabilityUp,
				s.ReliabilityDown,
			)

	damping :=
		1 + s.StabilityPressure

	return IdentificationSignals{
		MPCVarianceScale: variance,
		SafetyMarginScale: safety,
		RolloutTrustScale: trust,
		DampingFactor: damping,
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
) (IdentificationState, IdentificationSignals) {

	next := s

	next =
		e.updateArrival(next, measuredArrival)

	next =
		e.updateBurst(next, measuredArrival)

	next =
		e.updateDisturbance(next, residual)

	next =
		e.updateErrors(next, queueErr, latErr)

	next =
		e.updateConfidence(
			next,
			mpcQuality,
			safetyRate,
			queuePressure,
			slaPressure,
			maturity,
		)

	next =
		e.updateReliability(
			next,
			rolloutSuccess,
			scaleDelta,
			infraLoad,
		)

	next =
		e.updateEnvelope(next)

	next =
		e.updateSeasonal(next, totalLoad)

	next =
		e.updateDamping(next)

	sig :=
		e.signals(next)

	return next, sig
}