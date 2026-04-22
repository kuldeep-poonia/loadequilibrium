package autopilot

import (
	"math"
	"math/rand"
	"sort"
	"time"
)

type MPCState struct {
	Backlog float64
	Latency float64

	ArrivalMean float64
	ArrivalVar  float64

	TopologyPressure float64
	TopologyState    float64 // future graph extension

	ServiceRate    float64
	CapacityActive float64
}

type MPCControl struct {
	CapacityTarget float64
	RetryFactor    float64
	CacheRelief    float64
}

type MPCOptimiser struct {
	Horizon int
	Dt      float64

	ScenarioCount int
	Deterministic bool

	rng *rand.Rand

	// For deterministic mode: counter to ensure each Optimise call uses same RNG state
	deterministicCallCount int

	// adaptive regime hook
	BurstProb float64

	BacklogCost  float64
	LatencyCost  float64
	VarianceBase float64
	ScalingCost  float64
	SmoothCost   float64
	TerminalCost float64
	UtilCost     float64

	SafetyBarrier float64
	RiskQuantile  float64
	RiskWeight    float64

	MaxCapacity float64
	MinCapacity float64

	MaxStepCap   float64
	MaxStepRetry float64
	MaxStepCache float64

	InitTemp float64
	Cooling  float64
	Iters    int

	// runtime-adjustable cadence modifier — set by damping signal
	IterModifier float64
}

func (m *MPCOptimiser) initRNG() {
	// In non-deterministic mode, create once and reuse
	if !m.Deterministic {
		if m.rng != nil {
			return
		}
		m.rng = rand.New(rand.NewSource(time.Now().UnixNano()))
		return
	}

	// In deterministic mode, only initialize if not yet created
	if m.rng == nil {
		m.rng = rand.New(rand.NewSource(42))
	}
}

/*
Regime-switch disturbance
*/
func (m *MPCOptimiser) disturb(
	x MPCState,
	scenario int,
) float64 {
	m.initRNG()

	if m.Deterministic {

		// fixed reproducible scenarios
		return math.Sin(float64(scenario)) *
			0.3 * math.Sqrt(x.ArrivalVar+1)
	}

	if m.rng.Float64() < m.BurstProb {

		return m.rng.ExpFloat64() *
			0.7 * math.Sqrt(x.ArrivalVar+1)
	}

	return m.rng.NormFloat64() *
		0.25 * math.Sqrt(x.ArrivalVar+1)
}

/*
Variance evolution with memory + damping
*/
func (m *MPCOptimiser) varianceNext(
	x MPCState,
	retry float64,
	cap float64,
) float64 {

	damp :=
		1 / (1 + cap)

	return 0.8*x.ArrivalVar +
		0.15*math.Abs(x.TopologyPressure) +
		0.05*retry*damp
}

/*
Topology evolution placeholder
*/
func (m *MPCOptimiser) topologyNext(
	x MPCState,
	service float64,
) float64 {

	util :=
		x.ArrivalMean /
			(service + 1e-6)

	return 0.75*x.TopologyPressure +
		0.2*util +
		0.05*x.TopologyState
}

/*
Adaptive capacity lag
*/
func (m *MPCOptimiser) capLag(
	x MPCState,
	target float64,
) float64 {

	load :=
		x.ArrivalMean /
			(x.ServiceRate*x.CapacityActive + 1e-6)

	gain :=
		0.2 + 0.6/(1+load)

	return x.CapacityActive +
		gain*(target-x.CapacityActive)
}

/*
Propagation
*/
func (m *MPCOptimiser) propagate(
	x MPCState,
	u MPCControl,
	scenario int,
) MPCState {

	next := x

	cap :=
		m.capLag(x, u.CapacityTarget)

	service :=
		x.ServiceRate * cap *
			(1 - u.CacheRelief)

	retry :=
		u.RetryFactor *
			math.Sqrt(x.Backlog+1)

	dist :=
		m.disturb(x, scenario)

	rawArrival :=
		x.ArrivalMean +
			dist +
			0.35*x.TopologyPressure

	maxJump := x.ArrivalMean * 1.3

	arrival := rawArrival
	if arrival > maxJump {
		arrival = maxJump
	}

	arrival = 0.8*x.ArrivalMean + 0.2*arrival

	dQ :=
		(arrival + retry - service) * m.Dt

	next.Backlog =
		math.Max(0, x.Backlog+dQ)

	util :=
		arrival / (service + 1e-6)

	next.Latency =
		0.55*x.Latency +
			0.3*util*math.Sqrt(next.Backlog+1) +
			0.15*math.Sqrt(next.Backlog)

	next.TopologyPressure =
		m.topologyNext(x, service)

	next.ArrivalVar =
		m.varianceNext(x, retry, cap)

	next.CapacityActive = cap

	// emergency stabilization: aggressive capacity boost during backlog accumulation
	// Prevents model underestimation from causing backlog explosion

	return next
}

/*
Smooth barrier
*/
func (m *MPCOptimiser) barrier(q float64) float64 {

	return m.SafetyBarrier *
		math.Pow(q, 3) /
		(1 + q*q)
}

/*
Stage cost
*/
func (m *MPCOptimiser) cost(
	x MPCState,
	u MPCControl,
	prev MPCControl,
) float64 {

	// collapse penalty (hard stability constraint)
	collapsePenalty := 0.0
	if x.Backlog > 10 {
		collapsePenalty = 50 * math.Pow(x.Backlog-10, 2)
	}

	// utilization penalty (overload avoidance)
	util := x.ArrivalMean / (x.ServiceRate*x.CapacityActive + 1e-6)
	utilPenalty := math.Pow(math.Max(0, util-1), 2) * 20

	// ---- REQUIRED CAPACITY ----
	required := x.ArrivalMean / (x.ServiceRate + 1e-6)

	// ---- EXCESS BASED COST (FIXED) ----
	excess := u.CapacityTarget - required
	excessCost := 0.0

	if excess > 0 {
		excessCost = excess * excess * 10.0
	}

	// ---- UNDER-PROVISION PENALTY ----
	deficit := required - u.CapacityTarget
	deficitCost := 0.0

	if deficit > 0 {
		deficitCost = deficit * deficit * 50.0
	}

	return m.BacklogCost*x.Backlog*x.Backlog +
		m.LatencyCost*x.Latency*x.Latency +
		m.VarianceBase*math.Sqrt(x.ArrivalVar+1) +
		excessCost +                      // ✅ new
		deficitCost +                     // ✅ new
		m.SmoothCost*math.Abs(u.CapacityTarget-prev.CapacityTarget) +
		m.barrier(x.Backlog) +
		collapsePenalty +
		utilPenalty
}
/*
Terminal hybrid constraint
*/
func (m *MPCOptimiser) terminal(
	x MPCState,
) float64 {

	util :=
		x.ArrivalMean -
			x.ServiceRate*x.CapacityActive

	return m.TerminalCost*
		(x.Backlog*x.Backlog+
			x.Latency*x.Latency+
			util*util) +
		m.SafetyBarrier*
			math.Pow(math.Max(0, x.Backlog-5), 2)*5
}

/*
Evaluate risk via quantile CVaR
*/
func (m *MPCOptimiser) evaluate(
	initial MPCState,
	seq []MPCControl,
) (float64, float64) {

	costs :=
		make([]float64, m.ScenarioCount)

	for s := 0; s < m.ScenarioCount; s++ {

		x := initial
		total := 0.0

		for t := 0; t < m.Horizon; t++ {

			prev :=
				seq[int(math.Max(
					0,
					float64(t-1),
				))]

			total +=
				m.cost(x, seq[t], prev)

			x =
				m.propagate(x, seq[t], s)
		}

		total += m.terminal(x)

		costs[s] = total
	}

	sort.Float64s(costs)

	qIdx :=
		int(
			m.RiskQuantile *
				float64(len(costs)),
		)

	cvar := 0.0

	for i := qIdx; i < len(costs); i++ {
		cvar += costs[i]
	}

	cvar /= float64(len(costs) - qIdx)

	mean := 0.0
	for _, c := range costs {
		mean += c
	}
	mean /= float64(len(costs))

	return mean + m.RiskWeight*cvar,
		cvar
}

/*
Mutation with clamping
*/
func (m *MPCOptimiser) mutate(
	seq []MPCControl,
) {
	m.initRNG()

	// 80% time: local mutation (stable)
	if m.rng.Float64() < 0.8 {
		i := m.rng.Intn(len(seq))

		seq[i].CapacityTarget =
			math.Max(m.MinCapacity,
				math.Min(m.MaxCapacity,
					seq[i].CapacityTarget+
						(m.rng.Float64()-0.5)*m.MaxStepCap))

		seq[i].RetryFactor =
			math.Max(0,
				seq[i].RetryFactor+
					(m.rng.Float64()-0.5)*m.MaxStepRetry)

		seq[i].CacheRelief =
			math.Max(0,
				seq[i].CacheRelief+
					(m.rng.Float64()-0.5)*m.MaxStepCache)

		return
	}

	// 20% time: global mutation (escape local minima)
	for i := range seq {
		seq[i].CapacityTarget =
			math.Max(m.MinCapacity,
				math.Min(m.MaxCapacity,
					seq[i].CapacityTarget+
						(m.rng.Float64()-0.5)*m.MaxStepCap*0.5))
	}
}

/*
Main optimisation
*/
func (m *MPCOptimiser) SetCadenceModifier(d float64) {
	// d is the damping factor from the identification engine.
	// Higher damping (instability detected) → more iterations for safer plan.
	// Guard: never drop below 1, never exceed 4× base.
	if d <= 0 {
		return
	}
	m.IterModifier = d
}

func (m *MPCOptimiser) effectiveIters() int {
	if m.IterModifier <= 0 {
		return m.Iters
	}
	n := int(math.Max(1, float64(m.Iters)*m.IterModifier))
	if max := m.Iters * 4; n > max {
		return max
	}
	return n
}

func (m *MPCOptimiser) Optimise(
	initial MPCState,
	prevSeq []MPCControl,
) ([]MPCControl, float64) {
	m.initRNG()

	seq := make([]MPCControl, m.Horizon)
	copy(seq, prevSeq)

	// PID warm-start bias
	for i := 0; i < m.Horizon; i++ {
		if i >= len(prevSeq) {
			required := initial.ArrivalMean / math.Max(initial.ServiceRate, 1)
seq[i].CapacityTarget = required
		} else {
			seq[i].CapacityTarget = 0.5*seq[i].CapacityTarget + 0.5*initial.CapacityActive
		}
	}

	best, tail := m.evaluate(initial, seq)
	temp := m.InitTemp

	candidate := make([]MPCControl, m.Horizon)
	copy(candidate, seq)

	for iter := 0; iter < m.effectiveIters(); iter++ {
		m.mutate(candidate)

		c, t := m.evaluate(initial, candidate)

		if c < best || m.rng.Float64() < math.Exp((best-c)/temp) {
			copy(seq, candidate)
			best = c
			tail = t
		} else {
			copy(candidate, seq)
		}

		temp *= m.Cooling
	}

	// ---- CLEAN CONTROL BLOCK ----

	prevCapacity := initial.CapacityActive
	if len(prevSeq) > 0 {
		prevCapacity = prevSeq[0].CapacityTarget
	}

	// 📍 FIX 1 — MINIMUM START CAP
	minStartCap := initial.ArrivalMean / 10 // ~28

	if seq[0].CapacityTarget < minStartCap {
		seq[0].CapacityTarget = minStartCap
	}

	// 📍 FIX 2 — ANTI-FLIP HOLD (keep this)
	minHoldRatio := 0.9

	if seq[0].CapacityTarget < prevCapacity*minHoldRatio {
		seq[0].CapacityTarget = prevCapacity * minHoldRatio
	}

	// 📍 FIX 3 — OPTIONAL (fast ramp)
	if initial.Backlog > 50 {
		seq[0].CapacityTarget *= 1.1
	}

	// ---- MPC STARTUP BOOTSTRAP ----
	if len(prevSeq) == 0 {
		required := initial.ArrivalMean / math.Max(initial.ServiceRate, 1)
		bootstrap := required * 1.2

		if seq[0].CapacityTarget < bootstrap {
			seq[0].CapacityTarget = bootstrap
		}
	}

	// ---- MPC BACKLOG SPIKE GUARD ----
	if initial.Backlog > 50 {
		required := initial.ArrivalMean / math.Max(initial.ServiceRate, 1)
		emergency := required * 1.3

		if seq[0].CapacityTarget < emergency {
			seq[0].CapacityTarget = emergency
		}
	}

	backlog := initial.Backlog

	// -------------------------------
	// 1) BASE FLOOR (single source of truth)
	// -------------------------------
	baseFloor := 20.0

	// bootstrap: no history → avoid tiny caps
	if len(prevSeq) == 0 {
		baseFloor = math.Max(baseFloor, 50.0)
	}

	// backlog-based floor (piecewise but smooth-enough)
	switch {
	case backlog > 1500:
		baseFloor = math.Max(baseFloor, 120.0)
	case backlog > 700:
		baseFloor = math.Max(baseFloor, 100.0)
	case backlog > 300:
		baseFloor = math.Max(baseFloor, 80.0)
	case backlog > 100:
		baseFloor = math.Max(baseFloor, 60.0)
	case backlog > 20:
		baseFloor = math.Max(baseFloor, 40.0)
	}

	// apply floor once
	if seq[0].CapacityTarget < baseFloor {
		seq[0].CapacityTarget = baseFloor
	}

	// -------------------------------
	// 2) HOLD / NO-DOWNWARD ZONE
	// -------------------------------
	// under load or just-cleared → do not drop below previous
	if backlog > 50 && backlog < 100 {
		if seq[0].CapacityTarget < prevCapacity {
			seq[0].CapacityTarget = prevCapacity
		}
	}

	// -------------------------------
	// 3) MONOTONIC DECAY (rate-limited)
	// -------------------------------
	// allow decrease, but only gradually
	decayRate := 0.98
	minAllowed := prevCapacity * decayRate

	if seq[0].CapacityTarget < minAllowed {
		seq[0].CapacityTarget = minAllowed
	}
	// ---- STABILITY HOLD (CRITICAL FIX) ----
	if initial.Backlog < 20 {
		minAllowed := prevCapacity * 0.85

		if seq[0].CapacityTarget < minAllowed {
			seq[0].CapacityTarget = minAllowed
		}
	}
	// -------------------------------
	// 4) LIGHT SMOOTHING (avoid jitter)
	// -------------------------------
	alpha := 0.9 // keep high inertia
	seq[0].CapacityTarget = alpha*prevCapacity + (1-alpha)*seq[0].CapacityTarget

	// ---- TIGHT RATE LIMIT (MPC) ----
	maxStep := 10.0
	delta := seq[0].CapacityTarget - prevCapacity

	if delta > maxStep {
		seq[0].CapacityTarget = prevCapacity + maxStep
	}
	if delta < -maxStep {
		seq[0].CapacityTarget = prevCapacity - maxStep
	}

	// -------------------------------
	// 4.5) TRANSIENT BOOST (NEW)
	// -------------------------------
	if initial.Backlog > 0 && initial.Backlog < 200 {
		boosted := prevCapacity * 1.05
		if seq[0].CapacityTarget < boosted {
			seq[0].CapacityTarget = boosted
		}
	}

	// ---- HARD ANTI-DROP FLOOR ----
	hardMinAllowed := prevCapacity * 0.85

	if seq[0].CapacityTarget < hardMinAllowed {
		seq[0].CapacityTarget = hardMinAllowed
	}

	// ---- ABSOLUTE MIN START ----
	minStart := initial.ArrivalMean / 8 // ~35

	if seq[0].CapacityTarget < minStart {
		seq[0].CapacityTarget = minStart
	}

	// -------------------------------
	// 5) HARD BOUNDS (final clamp)
	// -------------------------------
	if seq[0].CapacityTarget < m.MinCapacity {
		seq[0].CapacityTarget = m.MinCapacity
	}
	if seq[0].CapacityTarget > m.MaxCapacity {
		seq[0].CapacityTarget = m.MaxCapacity
	}

	conf := 1 / (1 + math.Sqrt(tail))
	return seq, conf
}
