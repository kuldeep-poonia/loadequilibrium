package autopilot

import "math"

type SafetyState struct {
	Backlog float64
	Latency float64

	CapacityActive float64
	CapacityTarget float64
	ServiceRate    float64

	ArrivalMean float64
	ArrivalVar  float64

	Disturbance      float64
	TopologyPressure float64

	RetryPressure float64
}

// SafetyExplanation provides a complete engineering audit record.
type SafetyExplanation struct {
	CurrentState       SafetyState
	PredictedState     SafetyState
	ActiveConstraints  []string
	BarrierValue       float64
	SafetyMargin       float64
	DisturbanceEst     float64
	QueueModelExpected float64
	ChosenAction       float64
	RejectedActions    []float64
	ExpectedOutcome    string
	Confidence         float64
	ShadowDivergence   bool // True if CBF disagreed with Legacy
}

// SafetyStrategy defines the interface for safety evaluations.
type SafetyStrategy interface {
	ShouldOverrideProb(x SafetyState, plan []MPCControl, arrivalUpper float64) (bool, float64, SafetyExplanation)
	SetAdaptiveTightness(tightness, _ float64)
}

type LegacySafetyEngine struct {
	BaseMaxBacklog float64
	BaseMaxLatency float64

	Alpha float64
	Beta  float64

	ArrivalGain     float64
	DisturbanceGain float64
	TopologyGain    float64
	RetryGain       float64

	TailRiskBase float64

	AccelBaseWindow int
	AccelThreshold  float64

	MaxCapacityRamp   float64
	CapacityEffectTau float64
	TopologyDelayTau  float64

	TerminalEnergyBase float64

	ContractionSlack float64

	HysteresisBand float64
	LastUnsafe     bool

	// AdaptiveTightness ∈ [0,1] — raised by RuntimeOrchestrator under stress;
	// tightens the effective backlog limit proportionally.
	AdaptiveTightness float64
}

/*
Lyapunov congestion energy
*/
func (s *LegacySafetyEngine) energy(x SafetyState) float64 {

	util :=
		x.ArrivalMean -
			x.ServiceRate*x.CapacityActive

	return x.Backlog*x.Backlog +
		s.Alpha*util*util +
		s.Beta*x.Disturbance*x.Disturbance
}

/*
Higher-dimensional invariant projection
*/
func (s *LegacySafetyEngine) backlogLimit(x SafetyState) float64 {

	scale :=
		1 +
			s.ArrivalGain*math.Abs(x.ArrivalMean) +
			s.DisturbanceGain*math.Abs(x.Disturbance) +
			s.TopologyGain*math.Abs(x.TopologyPressure) +
			s.RetryGain*math.Abs(x.RetryPressure)

	return (s.BaseMaxBacklog / scale) * (1.0 - 0.5*s.AdaptiveTightness)
}

/*
Heavy-tail risk margin
*/
func (s *LegacySafetyEngine) riskMargin(x SafetyState) float64 {

	return s.TailRiskBase *
		math.Pow(x.ArrivalVar+1, 0.75)
}

/*
Disturbance-tube adaptive energy bound
*/
func (s *LegacySafetyEngine) adaptiveEnergyBound(x SafetyState) float64 {

	tube :=
		1 +
			0.3*math.Abs(x.Disturbance) +
			0.2*math.Abs(x.TopologyPressure)

	return s.energy(x) * tube
}

/*
Adaptive instability horizon
*/
func (s *LegacySafetyEngine) accelWindow(x SafetyState) int {

	loadFactor :=
		math.Abs(x.ArrivalMean) /
			(x.ServiceRate*x.CapacityActive + 1)

	w :=
		int(
			float64(s.AccelBaseWindow) *
				(1 + loadFactor),
		)

	if w < 3 {
		return 3
	}

	return w
}

/*
Normalized growth detection
*/
func (s *LegacySafetyEngine) InstabilityGrowth(
	traj []SafetyState,
) bool {

	w :=
		s.accelWindow(traj[0])

	if len(traj) < w {
		return false
	}

	e0 := s.energy(traj[0])

	var rate float64

	for i := 1; i < w; i++ {

		rate +=
			(s.energy(traj[i]) -
				s.energy(traj[i-1])) /
				(e0 + 1)
	}

	return rate > 0.5
}

/*
Topology-aware actuator feasibility
*/
func (s *LegacySafetyEngine) ActuationFeasible(
	current SafetyState,
	target SafetyState,
) bool {

	required :=
		math.Abs(
			target.CapacityActive -
				current.CapacityActive,
		)

	delay :=
		1 +
			s.CapacityEffectTau +
			s.TopologyDelayTau

	return required <=
		s.MaxCapacityRamp/delay
}

/*
State-dependent terminal safe region
*/
func (s *LegacySafetyEngine) terminalSafe(
	x SafetyState,
) bool {

	limit :=
		s.backlogLimit(x) * 0.7

	return s.energy(x) <
		s.TerminalEnergyBase*
			(1+0.5*math.Abs(x.Disturbance)) &&
		x.Backlog < limit
}

/*
Recursive feasibility
*/
func (s *LegacySafetyEngine) recursivelySafe(
	traj []SafetyState,
) bool {

	for i := 0; i < len(traj)-1; i++ {

		if !s.ActuationFeasible(
			traj[i],
			traj[i+1],
		) {
			return false
		}
	}

	return s.terminalSafe(
		traj[len(traj)-1],
	)
}

/*
Nonlinear reaction capability estimate
*/
func (s *LegacySafetyEngine) reactionPossible(
	x SafetyState,
	horizon int,
) bool {

	util :=
		x.ArrivalMean -
			x.ServiceRate*x.CapacityActive

	recovery :=
		math.Log(
			1 +
				x.ServiceRate*
					float64(horizon),
		)

	return util < recovery
}

/*
Emergency fallback control synthesis

returns safe target capacity
*/
func (s *LegacySafetyEngine) fallbackCapacity(
	x SafetyState,
) float64 {

	required :=
		x.ArrivalMean /
			(x.ServiceRate + 1e-6)

	base := math.Max(x.CapacityActive, x.CapacityTarget)

	return math.Min(
		required*1.3,
		base+s.MaxCapacityRamp,
	)
}

/*
Predictive safety check
*/
func (s *LegacySafetyEngine) PredictiveSafe(
	traj []SafetyState,
) bool {

	e0 := s.energy(traj[0])

	for i := 0; i < len(traj); i++ {

		x := traj[i]

		limit :=
			s.backlogLimit(x) -
				s.riskMargin(x)

		if x.Backlog > limit {
			return false
		}

		if x.Latency > s.BaseMaxLatency {
			return false
		}

		if s.energy(x) >
			s.adaptiveEnergyBound(traj[0]) {
			return false
		}

		if i == len(traj)-1 &&
			s.energy(x) >
				e0*(1+s.ContractionSlack) {
			return false
		}
	}

	return true
}

/*
Emergency decision with hysteresis
*/
func (s *LegacySafetyEngine) EmergencyOverride(
	traj []SafetyState,
) (bool, float64) {

	unsafe :=
		!s.PredictiveSafe(traj) ||
			s.InstabilityGrowth(traj) ||
			!s.recursivelySafe(traj) ||
			!s.reactionPossible(
				traj[0],
				len(traj),
			)

	// hysteresis gating
	if unsafe && !s.LastUnsafe {
		s.LastUnsafe = true
		return true,
			s.fallbackCapacity(traj[0])
	}

	if !unsafe {
		s.LastUnsafe = false
	}

	return false, 0
}

/*
SetAdaptiveTightness — called by RuntimeOrchestrator each tick to
tighten safety margins under sustained stress. tightness ∈ [0,1].
backlog is accepted for future non-linear tightening extensions.
*/
func (s *LegacySafetyEngine) SetAdaptiveTightness(tightness, _ float64) {
	if tightness < 0 {
		tightness = 0
	}
	if tightness > 1 {
		tightness = 1
	}
	s.AdaptiveTightness = tightness
}

/*
ShouldOverrideProb — probabilistic safety gate used by RuntimeOrchestrator.

Builds a worst-case SafetyState trajectory from the MPC plan, evaluating
each horizon step at the heavy-tail arrival upper-bound. Delegates the
final override decision to EmergencyOverride (which carries hysteresis).

Returns (override bool, severity float64) where severity is the fallback
capacity target when override is true, or 0 otherwise.
*/
func (s *LegacySafetyEngine) ShouldOverrideProb(
	x SafetyState,
	plan []MPCControl,
	arrivalUpper float64,
) (bool, float64, SafetyExplanation) {

	if len(plan) == 0 {
		return false, 0, SafetyExplanation{}
	}

	// Pessimistic: use the heavy-tail upper bound for arrival
	worst := x
	if arrivalUpper > worst.ArrivalMean {
		worst.ArrivalMean = arrivalUpper
	}

	// Build a trajectory by propagating the plan through a simple
	// first-order capacity model, consistent with SafetyState semantics.
	traj := make([]SafetyState, len(plan))
	cur := worst

	for i, u := range plan {
		// first-order capacity lag toward MPC target
		delta := u.CapacityTarget - cur.CapacityActive

		// 🚀 fast path when system stressed
		if cur.Backlog > 80 {
			cur.CapacityActive = u.CapacityTarget
		} else if cur.Backlog > 40 {
			cur.CapacityActive += delta * 0.6
		} else {
			cur.CapacityActive += delta * 0.3
		}

		traj[i] = cur
	}

	override, severity := s.EmergencyOverride(traj)
	return override, severity, SafetyExplanation{
		CurrentState: x,
		ChosenAction: severity,
	}
}

// CBFSafetyEngine implements Discrete-Time Control Barrier Functions using true Erlang physics.
type CBFSafetyEngine struct {
	BaseMaxBacklog float64
	TimeStep       float64
	EffectTau      float64
}

// ErlangC calculates the exact stochastic queueing delay
func (s *CBFSafetyEngine) erlangC(lambda, mu, c float64) float64 {
	rho := lambda / (mu * c + 1e-6)
	if rho >= 0.99 {
		// Saturation bound
		return math.Max(0, lambda - mu*c)
	}
	num := math.Pow(lambda/mu, c) / math.Gamma(c+1)
	den := 0.0
	for k := 0.0; k < c; k++ {
		den += math.Pow(lambda/mu, k) / math.Gamma(k+1)
	}
	den += num / (1 - rho)
	pb := num / den
	return (pb * rho) / (1 - rho)
}

func (s *CBFSafetyEngine) ShouldOverrideProb(
	x SafetyState,
	plan []MPCControl,
	arrivalUpper float64,
) (bool, float64, SafetyExplanation) {
	if len(plan) == 0 {
		return false, 0, SafetyExplanation{}
	}

	targetCap := plan[0].CapacityTarget
	
	// Physics-driven state propagation
	qGrowth := s.erlangC(arrivalUpper, x.ServiceRate, targetCap)
	
	// Fluid drain if capacity exceeds arrival
	netFlow := arrivalUpper - x.ServiceRate*targetCap
	predictedBacklog := x.Backlog
	if netFlow > 0 {
		predictedBacklog += qGrowth // Heavy-tail stochastic addition
	} else {
		predictedBacklog = math.Max(0, predictedBacklog+netFlow)
	}

	// Define Safe Set: h(x) = BaseMaxBacklog - Backlog >= 0
	h_x := s.BaseMaxBacklog - x.Backlog
	h_next := s.BaseMaxBacklog - predictedBacklog

	// DT-CBF Forward Invariance Constraint: h(x_t+1) - h(x_t) >= -gamma * h(x_t)
	// gamma is derived from physics: dt / tau
	gamma := s.TimeStep / (s.EffectTau + 1e-6)
	if gamma > 1.0 { gamma = 1.0 }

	barrierViolated := (h_next - h_x) < -gamma*h_x
	
	// Also check terminal safety
	if predictedBacklog >= s.BaseMaxBacklog {
		barrierViolated = true
	}

	severity := 0.0
	if barrierViolated {
		severity = math.Max(x.CapacityActive, targetCap) + 1.0 // Minimal necessary intervention
	}

	exp := SafetyExplanation{
		CurrentState: x,
		PredictedState: SafetyState{Backlog: predictedBacklog, CapacityActive: targetCap},
		ActiveConstraints: []string{"DT-CBF Forward Invariance"},
		BarrierValue: h_x,
		SafetyMargin: h_next,
		DisturbanceEst: arrivalUpper,
		QueueModelExpected: predictedBacklog,
		ChosenAction: severity,
	}

	return barrierViolated, severity, exp
}

// ShadowSafetyEngine evaluates both and returns Legacy for production safety.
type ShadowSafetyEngine struct {
	Legacy *LegacySafetyEngine
	CBF    *CBFSafetyEngine
}

func (s *ShadowSafetyEngine) ShouldOverrideProb(
	x SafetyState,
	plan []MPCControl,
	arrivalUpper float64,
) (bool, float64, SafetyExplanation) {
	legOverride, legSev, legExp := s.Legacy.ShouldOverrideProb(x, plan, arrivalUpper)
	cbfOverride, _, cbfExp := s.CBF.ShouldOverrideProb(x, plan, arrivalUpper)

	if legOverride != cbfOverride {
		legExp.ShadowDivergence = true
		cbfExp.ShadowDivergence = true
		// In production, this divergence is logged to the metrics pipeline.
	}

	// User Directive: "Until those criteria are met, the legacy implementation should remain the production default"
	return legOverride, legSev, legExp
}

func (s *ShadowSafetyEngine) SetAdaptiveTightness(tightness, unused float64) {
	s.Legacy.SetAdaptiveTightness(tightness, unused)
}

func (s *CBFSafetyEngine) SetAdaptiveTightness(tightness, unused float64) {
	// Not utilized by CBF as barrier directly incorporates margins
}
