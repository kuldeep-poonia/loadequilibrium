package autopilot

import (
	"math"
	"math/rand"
	"sort"
)

/*
PHASE-3 MPC — RESEARCH UPGRADE 5 (FINAL)

Adds:

• adaptive regime probability hook
• quantile-based CVaR risk objective
• extended annealing schedule
• deterministic fixed scenario mode
• hybrid terminal constraint (soft + barrier)
• variance memory + capacity damping
• calibrated confidence proxy
• extensible topology state placeholder
*/

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

/*
Regime-switch disturbance
*/
func (m *MPCOptimiser) disturb(
	x MPCState,
	scenario int,
) float64 {

	if m.Deterministic {

		// fixed reproducible scenarios
		return math.Sin(float64(scenario)) *
			0.3 * math.Sqrt(x.ArrivalVar+1)
	}

	if rand.Float64() < m.BurstProb {

		return rand.ExpFloat64() *
			0.7 * math.Sqrt(x.ArrivalVar+1)
	}

	return rand.NormFloat64() *
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

	arrival :=
		x.ArrivalMean +
			dist +
			0.35*x.TopologyPressure

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

	return m.BacklogCost*x.Backlog*x.Backlog +
		m.LatencyCost*x.Latency*x.Latency +
		m.VarianceBase*math.Sqrt(x.ArrivalVar+1) +
		m.ScalingCost*math.Pow(u.CapacityTarget, 1.25) +
		m.SmoothCost*math.Abs(
			u.CapacityTarget-prev.CapacityTarget,
		) +
		m.barrier(x.Backlog)
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
			math.Max(0, x.Backlog-5) // soft-hard hybrid
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

	i := rand.Intn(len(seq))

	seq[i].CapacityTarget =
		math.Max(m.MinCapacity,
			math.Min(m.MaxCapacity,
				seq[i].CapacityTarget+
					(rand.Float64()-0.5)*m.MaxStepCap))

	seq[i].RetryFactor =
		math.Max(0,
			seq[i].RetryFactor+
				(rand.Float64()-0.5)*m.MaxStepRetry)

	seq[i].CacheRelief =
		math.Max(0,
			seq[i].CacheRelief+
				(rand.Float64()-0.5)*m.MaxStepCache)
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

	seq := make([]MPCControl, m.Horizon)

	copy(seq, prevSeq)

	// PID warm-start bias (unchanged from original)
	for i := 0; i < m.Horizon; i++ {
		if i >= len(prevSeq) {
			seq[i].CapacityTarget = initial.CapacityActive
		} else {
			seq[i].CapacityTarget = 0.5*seq[i].CapacityTarget + 0.5*initial.CapacityActive
		}
	}

	best, tail := m.evaluate(initial, seq)
	temp := m.InitTemp

	// candidate holds the proposed mutation; seq holds the committed (accepted) state.
	candidate := make([]MPCControl, m.Horizon)
	copy(candidate, seq)

	for iter := 0; iter < m.effectiveIters(); iter++ {
		m.mutate(candidate)

		c, t := m.evaluate(initial, candidate)

		if c < best || rand.Float64() < math.Exp((best-c)/temp) {
			// Accept: commit candidate -> seq
			copy(seq, candidate)
			best = c
			tail = t
		} else {
			// Reject: revert candidate <- seq
			copy(candidate, seq)
		}

		temp *= m.Cooling
	}

	conf := 1 / (1 + math.Sqrt(tail))
	return seq, conf
}
