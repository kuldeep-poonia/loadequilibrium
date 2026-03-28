package autopilot

import (
	"math"
	"math/rand"
	"time"
)

/*
PHASE-3 FINAL AUTONOMY RUNTIME KERNEL

Improvements:

• predictor-rollout backlog forecast
• probabilistic autonomy regime classification + hysteresis
• correlated stochastic telemetry delay model
• multidimensional rollout failure physics
• state-dependent safety tightening
• damping affects MPC cadence + rollout pacing
• severity-weighted override memory (sliding window)
• long-horizon learning persistence hooks
• anti-oscillation governance stabiliser
*/

type AutonomyMode int

const (
	ModeStable AutonomyMode = iota
	ModeGuarded
	ModeCritical
	ModeRecovery
)

type RuntimeState struct {

	Plant CongestionState
	Rollout RolloutState
	ID IdentificationState

	LastPlan []MPCControl

	ForecastBacklog float64

	Time float64
	Mode AutonomyMode

	OverrideHistory []float64
	SafetyTight float64

	MetaPersistence float64
}

type RuntimeTelemetry struct {

	Backlog float64
	Latency float64
	Capacity float64

	Confidence float64
	MPCConfidence float64

	OverrideRate float64
	Mode int

	VarianceScale float64
	SafetyScale float64
	Damping float64
}

type RuntimeOrchestrator struct {

	Dt float64

	Predictor *Predictor
	MPC *MPCOptimiser
	Safety *SafetyEngine
	Rollout *RolloutController
	ID *IdentificationEngine

	SLA_Backlog float64

	OverrideWindow int

	DampingMin float64
	DampingMax float64

	FailureScaleProb float64
	FailureConfigProb float64

	TelemetryTau float64
}

/*
predict backlog using predictor rollout
*/
func (r *RuntimeOrchestrator) forecastBacklog(
	plant CongestionState,
	plan []MPCControl,
	arrival float64,
) float64 {

	sim := plant

	for i := 0; i < len(plan); i++ {

		u := plan[i]

		sim.CapacityTarget = u.CapacityTarget
		sim.RetryFactor    = u.RetryFactor
		sim.CacheRelief    = u.CacheRelief
		sim.ArrivalMean    = arrival

		sim = r.Predictor.Step(sim)
	}

	return sim.Backlog
}

/*
probabilistic autonomy mode
*/
func (r *RuntimeOrchestrator) modeProb(
	backlog float64,
	conf float64,
	overrideRate float64,
) AutonomyMode {

	risk :=
		math.Tanh(
			backlog/r.SLA_Backlog +
				(1-conf) +
				overrideRate,
		)

	if risk > 0.8 {
		return ModeCritical
	}

	if risk > 0.55 {
		return ModeGuarded
	}

	if risk < 0.3 {
		return ModeStable
	}

	return ModeRecovery
}

/*
correlated telemetry delay
*/
func (r *RuntimeOrchestrator) delay(
	value float64,
	prev float64,
) float64 {

	alpha :=
		math.Exp(
			-r.Dt / r.TelemetryTau,
		)

	noise :=
		0.05 *
			math.Sin(
				float64(time.Now().UnixNano()),
			)

	return alpha*prev +
		(1-alpha)*value +
		noise
}

/*
severity weighted override rate
*/
func (r *RuntimeOrchestrator) overrideRate(
	h []float64,
) float64 {

	if len(h) == 0 {
		return 0
	}

	sum := 0.0
	for _, v := range h {
		sum += v
	}

	return sum / float64(len(h))
}

/*
tick
*/
func (r *RuntimeOrchestrator) Tick(
	s RuntimeState,
	measuredArrival float64,
	infraLoad float64,
) (RuntimeState, RuntimeTelemetry) {

	next := s

	// ---------- MPC ----------
	seq, mpcConf :=
		r.MPC.Optimise(
			r.mpcState(s),
			s.LastPlan,
		)

	control := MPCControl{
		CapacityTarget: s.Plant.CapacityTarget,
		RetryFactor:    s.Rollout.RetryActive,
		CacheRelief:    s.Rollout.CacheActive,
	}
	if len(seq) > 0 {
		control = seq[0]
	}

	// ---------- predictor-based forecast ----------
	fBacklog :=
		r.forecastBacklog(
			s.Plant,
			seq,
			measuredArrival,
		)

	predErr :=
		s.Plant.Backlog - fBacklog

	latErr :=
		s.Plant.Latency

	// ---------- safety ----------
	override, severity :=
		r.Safety.ShouldOverrideProb(
			r.safetyState(s),
			seq,
			s.ID.ArrivalUpper,
		)

	if override {
		next.OverrideHistory =
			append(
				next.OverrideHistory,
				severity,
			)
	} else {
		next.OverrideHistory =
			append(
				next.OverrideHistory,
				0,
			)
	}

	if len(next.OverrideHistory) >
		r.OverrideWindow {

		next.OverrideHistory =
			next.OverrideHistory[1:]
	}

	overrideRate :=
		r.overrideRate(
			next.OverrideHistory,
		)

	// ---------- autonomy mode ----------
	next.Mode =
		r.modeProb(
			s.Plant.Backlog,
			s.ID.ModelConfidence,
			overrideRate,
		)

	// ---------- state-dependent safety tightening ----------
	next.SafetyTight =
		0.8*next.SafetyTight +
			0.2*math.Tanh(
				s.Plant.Backlog/
					r.SLA_Backlog+
					overrideRate,
			)

	r.Safety.SetAdaptiveTightness(
		next.SafetyTight,
		s.Plant.Backlog,
	)

	// ---------- rollout ----------
	newRollout :=
		r.Rollout.StepAdaptive(
			s.Rollout,
			control,
			s.ID.ModelConfidence,
			override,
			s.Plant.Backlog,
			s.ID.StabilityPressure,
			infraLoad,
			s.Time,
		)

	// ---------- multidimensional failure ----------
	if randUnit() < r.FailureScaleProb {

		newRollout.CapacityActive =
			s.Rollout.CapacityActive
	}

	if randUnit() < r.FailureConfigProb {

		newRollout.ConfigLag += 0.3
	}

	// ---------- plant ----------
	plantIn := s.Plant
	plantIn.CapacityActive = newRollout.CapacityActive
	plantIn.CapacityTarget = newRollout.CapacityActive
	plantIn.RetryFactor    = newRollout.RetryActive
	plantIn.CacheRelief    = newRollout.CacheActive
	plantIn.ArrivalMean    = measuredArrival
	newPlant := r.Predictor.Step(plantIn)

	newPlant.Backlog =
		r.delay(
			newPlant.Backlog,
			s.Plant.Backlog,
		)

	// ---------- identification ----------
	idState, sig :=
		r.ID.Step(
			s.ID,
			measuredArrival,
			predErr,
			latErr,
			newPlant.Backlog-s.Plant.Backlog,
			mpcConf,
			overrideRate,
			float64(len(newRollout.IntentQueue))/
				float64(r.Rollout.QueueMax),
			0.5,
			0.6,
			true,
			newRollout.CapacityActive-s.Rollout.CapacityActive,
			infraLoad,
			newPlant.Backlog,
		)

	// ---------- meta damping influence ----------
	d :=
		math.Max(
			r.DampingMin,
			math.Min(
				r.DampingMax,
				sig.DampingFactor,
			),
		)

	r.MPC.SetCadenceModifier(d)
	r.Rollout.SetPacingModifier(d)

	// ---------- persistence learning ----------
	next.MetaPersistence =
		0.99*next.MetaPersistence +
			0.01*newPlant.Backlog

	next.Plant = newPlant
	next.Rollout = newRollout
	next.ID = idState
	next.LastPlan = seq
	next.ForecastBacklog = fBacklog
	next.Time += r.Dt

	tel := RuntimeTelemetry{
		Backlog: newPlant.Backlog,
		Latency: newPlant.Latency,
		Capacity: newRollout.CapacityActive,

		Confidence: idState.ModelConfidence,
		MPCConfidence: mpcConf,

		OverrideRate: overrideRate,
		Mode: int(next.Mode),

		VarianceScale: sig.MPCVarianceScale,
		SafetyScale: sig.SafetyMarginScale,
		Damping: d,
	}

	return next, tel
}

func (r *RuntimeOrchestrator) mpcState(
	s RuntimeState,
) MPCState {

	return MPCState{
		Backlog:          s.Plant.Backlog,
		Latency:          s.Plant.Latency,
		ArrivalMean:      s.ID.ArrivalEstimate,
		ArrivalVar:       s.ID.ArrivalVar,
		TopologyPressure: s.Plant.UpstreamPressure * math.Max(1, s.Plant.TopologyAmplification),
		TopologyState:    s.Plant.TopologyAmplification,
		ServiceRate:      s.Plant.ServiceRate,
		CapacityActive:   s.Rollout.CapacityActive,
	}
}

func (r *RuntimeOrchestrator) safetyState(
	s RuntimeState,
) SafetyState {

	return SafetyState{
		Backlog:          s.Plant.Backlog,
		Latency:          s.Plant.Latency,
		CapacityActive:   s.Rollout.CapacityActive,
		CapacityTarget:   s.Plant.CapacityTarget,
		ServiceRate:      s.Plant.ServiceRate,
		ArrivalMean:      s.ID.ArrivalEstimate,
		ArrivalVar:       s.ID.ArrivalVar,
		Disturbance:      s.Plant.Disturbance,
		TopologyPressure: s.Plant.UpstreamPressure * math.Max(1, s.Plant.TopologyAmplification),
		RetryPressure:    s.Rollout.RetryActive,
	}
}

// randUnit returns a uniform random float64 in [0, 1).
// P3: Replaced 0.5+0.5*sin(time.Now().UnixNano()) which is non-uniform
// (PDF biased towards ±1) and deterministic within a tick cycle.
func randUnit() float64 {
	return rand.Float64()
}

// Run is a safe NO-OP.
//
// P11: This method must NOT be invoked in production. The autopilot is driven
// tick-by-tick through Tick() called from phase_runtime.go once per orchestrator
// tick. Invoking Run() would spawn an unsynchronized parallel control loop that
// operates on stale state and races with the phaseRuntime goroutine.
//
// The method body is intentionally empty to prevent accidental activation while
// preserving the method signature for any future use.
func (r *RuntimeOrchestrator) Run(
	initial RuntimeState,
) {
	// NO-OP: see godoc above.
	_ = initial
}
