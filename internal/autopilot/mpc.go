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

	PrevBacklog float64
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
	// Sync with Predictor.go (Line 150) to eliminate model mismatch
	return x.CapacityActive + 0.3*(target-x.CapacityActive)
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

	cap = math.Max(m.MinCapacity, math.Min(m.MaxCapacity, cap))

	effectiveCache := math.Min(0.5, math.Max(0, u.CacheRelief))
	service := x.ServiceRate * cap

	retry := math.Min(
		u.RetryFactor*x.ArrivalMean,
		x.ArrivalMean*0.5,
	)

	dist :=
		m.disturb(x, scenario)

	rawArrival :=
		x.ArrivalMean +
			dist +
			0.35*x.TopologyPressure

	maxJump := x.ArrivalMean * (1.3 + 0.7*math.Sqrt(x.ArrivalVar+1))

	arrival := rawArrival
	if arrival > maxJump {
		arrival = maxJump
	}

	arrival = 0.8*x.ArrivalMean + 0.2*arrival
	arrival *= 1 - effectiveCache

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

	next.PrevBacklog = x.Backlog

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

	// NO HARD THRESHOLD: Smooth collapse penalty
	collapsePenalty := math.Pow(x.Backlog, 3) * 0.05

	// Adaptive utilization target based on variance
	util := x.ArrivalMean / (x.ServiceRate*x.CapacityActive + 1e-6)
	volUtil := math.Sqrt(x.ArrivalVar + 1)
	targetUtil := 0.95 - math.Min(0.3, volUtil/10.0)
	utilPenalty := math.Pow(math.Max(0, util-targetUtil), 2) * 30

	// -------------------------------
	// 3) REQUIRED CAPACITY - DYNAMIC SAFETY
	// -------------------------------
	pressure := x.ArrivalMean - x.ServiceRate*x.CapacityActive
	vol := math.Sqrt(x.ArrivalVar + 1)
	safety := 1 + math.Min(1.0, vol/10.0+math.Max(0, pressure)/100.0)
	required := x.ArrivalMean / (x.ServiceRate + 1e-6) * safety

	excess := u.CapacityTarget - required
	excessCost := 0.0
	if excess > 0 {
		excessWeight := 2.0 * (1 + math.Max(0, -pressure))
		excessCost = excess * excess * excessWeight
	}

	deficit := required - u.CapacityTarget
	deficitCost := 0.0
	if deficit > 0 {
		deficitWeight := 20.0 * (1 + math.Max(0, pressure))

		deficitCost = deficit * deficit * deficit * deficitWeight
	}

	// -------------------------------
	// 5) SMOOTHNESS (no jumps)
	// -------------------------------

	idleWeight := math.Exp(-x.Backlog)

	minIdle := required * 0.8
	diff := math.Max(0, minIdle-u.CapacityTarget)

	idlePenalty := idleWeight * diff * diff * 3.0 // Drastically lower idle punishment

	//

	retryPenalty :=
		u.RetryFactor * u.RetryFactor *
			(1 + math.Max(0, pressure) + vol)
	// Cache benefit (reduces load)
	cacheBenefit := u.CacheRelief * x.ArrivalMean

	growth := x.Backlog - x.PrevBacklog

	growthPenalty := 0.0
	if growth > 0 {
		growthPenalty = growth * growth * (20.0 + math.Sqrt(x.ArrivalVar+1)) // Heavily penalize unhandled queue growth
	}

	return m.LatencyCost*x.Latency*x.Latency +
		m.VarianceBase*math.Sqrt(x.ArrivalVar+1) +
		excessCost +
		deficitCost +

		idlePenalty +
		m.SmoothCost*math.Abs(u.CapacityTarget-prev.CapacityTarget) +
		m.barrier(x.Backlog) +
		collapsePenalty +
		utilPenalty +
		retryPenalty +
		growthPenalty -
		0.5*cacheBenefit
}

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
	initial = sanitizeMPCState(initial)

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
				safeCost(m.cost(x, seq[t], prev))

			x =
				m.propagate(x, seq[t], s)
		}

		total += safeCost(m.terminal(x))

		costs[s] = total
	}

	sort.Float64s(costs)

	qIdx :=
		int(
			m.RiskQuantile *
				float64(len(costs)),
		)
	if qIdx >= len(costs) {
		qIdx = len(costs) - 1
	}
	if qIdx < 0 {
		qIdx = 0
	}

	cvar := 0.0

	for i := qIdx; i < len(costs); i++ {
		cvar += costs[i]
	}

	cvar /= math.Max(1, float64(len(costs)-qIdx))

	mean := 0.0
	for _, c := range costs {
		mean += c
	}
	mean /= float64(len(costs))

	score := mean + m.RiskWeight*cvar
	return safeCost(score),
		safeCost(cvar)
}

/*
Mutation with clamping
*/
func (m *MPCOptimiser) mutate(
	seq []MPCControl,
	x MPCState,
) {
	m.initRNG()

	bias := (x.ArrivalMean / (x.ServiceRate + 1e-6))

	// 80% time: local mutation (stable)
	if m.rng.Float64() < 0.8 {
		i := m.rng.Intn(len(seq))

		seq[i].CapacityTarget =
			math.Max(m.MinCapacity,
				math.Min(m.MaxCapacity,
					seq[i].CapacityTarget+
						(m.rng.Float64()-0.5)*m.MaxStepCap+
						0.1*(bias-seq[i].CapacityTarget)))

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
	if d > 1 {
		d = 1
	}
	m.IterModifier = d
}

func (m *MPCOptimiser) effectiveIters() int {
	base := m.Iters
	if base <= 0 {
		base = 1
	}
	if base > 32 {
		base = 32
	}
	if m.IterModifier <= 0 {
		return base
	}
	n := int(math.Max(1, float64(base)*m.IterModifier))
	if max := int(math.Ceil(float64(base))); n > max {
		return max
	}
	return n
}

func (m *MPCOptimiser) Optimise(
	initial MPCState,
	prevSeq []MPCControl,
) ([]MPCControl, float64) {

	m.initRNG()
	initial = sanitizeMPCState(initial)

	seq := make([]MPCControl, m.Horizon)
	copy(seq, prevSeq)

	// -------------------------------
	// 1) WARM START
	// -------------------------------
	warmRequired := initial.ArrivalMean / math.Max(initial.ServiceRate, 1)
	for i := 0; i < m.Horizon; i++ {
		if i >= len(prevSeq) {
			seq[i].CapacityTarget = warmRequired
		} else {
			// 20% anchor to required prevents stale spike plans from drifting
			seq[i].CapacityTarget =
				0.6*seq[i].CapacityTarget +
					0.2*initial.CapacityActive +
					0.2*warmRequired
		}
	}

	// -------------------------------
	// 2) OPTIMIZATION LOOP
	// -------------------------------
	if len(prevSeq) > 0 && initial.ArrivalMean > 1e-6 {
        // Infer previous arrival from warm-started capacity target
        prevImpliedArrival := prevSeq[0].CapacityTarget * initial.ServiceRate
        if prevImpliedArrival > 1e-6 {
            ratio := initial.ArrivalMean / prevImpliedArrival
            if ratio > 1.30 || ratio < 0.70 {
                // Regime shift detected (>30% arrival change): hard reset to required
                for i := range seq {
                    seq[i].CapacityTarget = warmRequired
                    seq[i].RetryFactor = 0
                    seq[i].CacheRelief = 0
                }
            }
        }
    }

    // -------------------------------
    // 2) OPTIMIZATION LOOP
    // -------------------------------
    best, tail := m.evaluate(initial, seq)
	temp := m.InitTemp

	candidate := make([]MPCControl, m.Horizon)
	copy(candidate, seq)

	for iter := 0; iter < m.effectiveIters(); iter++ {

		m.mutate(candidate, initial)

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

	conf := 1 / (1 + math.Sqrt(safeCost(tail)))
	if math.IsNaN(conf) || math.IsInf(conf, 0) {
		conf = 0
	}

	return seq, conf
}

func sanitizeMPCState(x MPCState) MPCState {
	x.Backlog = finiteNonNegative(x.Backlog)
	x.Latency = finiteNonNegative(x.Latency)
	x.ArrivalMean = finiteNonNegative(x.ArrivalMean)
	x.ArrivalVar = finiteNonNegative(x.ArrivalVar)
	x.TopologyPressure = finiteOrZero(x.TopologyPressure)
	x.TopologyState = finiteOrZero(x.TopologyState)
	x.ServiceRate = math.Max(finiteNonNegative(x.ServiceRate), 1e-6)
	x.CapacityActive = math.Max(finiteNonNegative(x.CapacityActive), 0.5)
	x.PrevBacklog = finiteNonNegative(x.PrevBacklog)
	return x
}

func safeCost(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 1e12
	}
	if v < 0 {
		return 0
	}
	return v
}
