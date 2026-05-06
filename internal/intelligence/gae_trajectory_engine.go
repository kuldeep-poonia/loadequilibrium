package intelligence

import (
	"math"
	"sync"
)

type TrajectoryStep struct {
	Feature   []float64
	Reward    float64
	Risk      float64
	Value     float64
	RiskValue float64 // yha hazard critic

	Done bool

	NextValue     float64
	NextRiskValue float64

	ISWeight float64
	RegimeID int
}

type GAETrajectoryEngine struct {
	mu sync.Mutex

	gamma  float64
	lambda float64

	maxSteps int
	maxEps   int

	safetyTheta [3]float64

	regimeLambda map[int]float64 // yha regime adaptive lambda

	buf   [][]TrajectoryStep
	head  int
	count int
}

func NewGAETrajectoryEngine(maxEpisodes int, maxSteps int) *GAETrajectoryEngine {

	b := make([][]TrajectoryStep, maxEpisodes)
	for i := range b {
		b[i] = make([]TrajectoryStep, 0, maxSteps)
	}

	return &GAETrajectoryEngine{
		gamma:  0.97,
		lambda: 0.92,

		maxSteps: maxSteps,
		maxEps:   maxEpisodes,

		safetyTheta: [3]float64{1.1, 0.7, 0.3},

		regimeLambda: make(map[int]float64),

		buf: b,
	}
}

func (g *GAETrajectoryEngine) StartEpisode() {

	g.mu.Lock()
	defer g.mu.Unlock()

	g.head = (g.head + 1) % g.maxEps
	g.buf[g.head] = g.buf[g.head][:0]

	if g.count < g.maxEps {
		g.count++
	}
}

func (g *GAETrajectoryEngine) AddStep(step TrajectoryStep) {

	g.mu.Lock()
	defer g.mu.Unlock()

	ep := &g.buf[g.head]

	if len(*ep) >= g.adaptiveHorizon(*ep) {
		copy((*ep)[0:], (*ep)[1:]) // yha adaptive truncate
		*ep = (*ep)[:len(*ep)-1]
	}

	*ep = append(*ep, step)
}

func (g *GAETrajectoryEngine) FinishEpisode() ([]float64, []float64) {

	g.mu.Lock()
	defer g.mu.Unlock()

	ep := g.buf[g.head]

	T := len(ep)
	if T == 0 {
		return nil, nil
	}

	adv := make([]float64, T)
	ret := make([]float64, T)

	nextAdv := 0.0
	nextRiskAdv := 0.0

	for t := T - 1; t >= 0; t-- {

		lmb := g.lambdaFor(ep[t].RegimeID)

		vNext := ep[t].NextValue
		rvNext := ep[t].NextRiskValue

		if ep[t].Done {
			vNext = 0
			rvNext = 0
		}

		rCost := g.safetyCost(ep[t].Risk)

		delta :=
			ep[t].Reward -
				rCost +
				g.gamma*vNext -
				ep[t].Value

		riskDelta :=
			ep[t].Risk +
				g.gamma*rvNext -
				ep[t].RiskValue

		adv[t] =
			delta +
				g.gamma*lmb*nextAdv -
				0.4*riskDelta - // yha explicit hazard coupling
				0.2*nextRiskAdv

		nextAdv = adv[t]
		nextRiskAdv = riskDelta

		ret[t] = adv[t] + ep[t].Value
	}

	g.regimeNormalize(adv, ep)

	g.clipAdv(adv)

	return adv, ret
}

/* ===== adaptive pieces ===== */

func (g *GAETrajectoryEngine) adaptiveHorizon(ep []TrajectoryStep) int {

	n := len(ep)
	if n < 10 {
		return g.maxSteps
	}

	varVar := 0.0
	m := 0.0

	for i := range ep {
		m += ep[i].Risk
	}
	m /= float64(n)

	for i := range ep {
		d := ep[i].Risk - m
		varVar += d * d
	}

	varVar /= float64(n)

	if varVar > 0.15 {
		return int(0.6 * float64(g.maxSteps)) // yha burst shorten
	}

	return g.maxSteps
}

func (g *GAETrajectoryEngine) lambdaFor(reg int) float64 {

	l, ok := g.regimeLambda[reg]
	if !ok {
		l = g.lambda
		g.regimeLambda[reg] = l
	}
	return l
}

func (g *GAETrajectoryEngine) regimeNormalize(a []float64, ep []TrajectoryStep) {

	type stat struct {
		m float64
		s float64
		n int
	}

	stats := map[int]*stat{}

	for i := range a {

		r := ep[i].RegimeID

		if stats[r] == nil {
			stats[r] = &stat{}
		}

		stats[r].m += a[i]
		stats[r].n++
	}

	for _, st := range stats {
		st.m /= float64(st.n)
	}

	for i := range a {

		r := ep[i].RegimeID
		d := a[i] - stats[r].m
		stats[r].s += d * d
	}

	for _, st := range stats {
		st.s = math.Sqrt(st.s/float64(st.n)) + 1e-6
	}

	for i := range a {

		r := ep[i].RegimeID
		w := ep[i].ISWeight
		if w <= 0 {
			w = 1
		}

		a[i] =
			((a[i] - stats[r].m) / stats[r].s) *
				w // yha IS correct usage
	}
}

func (g *GAETrajectoryEngine) clipAdv(a []float64) {

	for i := range a {

		if a[i] > 4 {
			a[i] = 4
		}
		if a[i] < -4 {
			a[i] = -4
		}
	}
}

func (g *GAETrajectoryEngine) safetyCost(r float64) float64 {

	z :=
		g.safetyTheta[0]*r +
			g.safetyTheta[1]*r*r +
			g.safetyTheta[2]*math.Sqrt(r+1e-6)

	return math.Tanh(z)
}

func (g *GAETrajectoryEngine) MetaAdapt(stabilityGrad float64) {

	g.mu.Lock()
	defer g.mu.Unlock()

	for i := range g.safetyTheta {

		g.safetyTheta[i] += 0.005 * stabilityGrad // yha smoother meta

		if g.safetyTheta[i] < 0.05 {
			g.safetyTheta[i] = 0.05
		}
	}
}