package intelligence

import (
	"math"
	"math/rand"
	"sort"
	"sync"
	"time"
)

type PolicyAction struct {
	ScaleOut     float64
	RetryBackoff float64
	QueueShard   float64
	CacheBoost   float64
}

type pgTransition struct {
	feat      []float64
	act       []float64
	mean      []float64
	chol      [][]float64
	reward    float64
	risk      float64
	done      bool
	nextFeat  []float64
	priority  float64
}

type PolicyGradientOptimizer struct {
	mu sync.Mutex

	sDim int
	fDim int
	aDim int
	hDim int

	// encoder
	encW [][]float64
	encB []float64

	// actor mean net
	w1 [][]float64
	b1 []float64
	w2 [][]float64
	b2 []float64

	// actor covariance param (full)
	L [][]float64

	// critic
	vw1 [][]float64
	vb1 []float64
	vw2 []float64
	vb2 float64

	alphaPi float64
	alphaV  float64
	gamma   float64
	lambda  float64

	klTarget float64

	replay []pgTransition
	replayMax int
	batch int

	rng *rand.Rand
}

func NewPolicyGradientOptimizer(stateDim int) *PolicyGradientOptimizer {

	aDim := 4
	enc := 32
	h := 32

	return &PolicyGradientOptimizer{
		sDim: stateDim,
		fDim: enc,
		aDim: aDim,
		hDim: h,

		encW: randomMatrix(enc, stateDim, 0.2),
		encB: make([]float64, enc),

		w1: randomMatrix(h, enc, 0.2),
		b1: make([]float64, h),
		w2: randomMatrix(aDim, h, 0.2),
		b2: make([]float64, aDim),

		L: identityMatrix(aDim, 0.4), // yha full cov chol

		vw1: randomMatrix(h, enc, 0.2),
		vb1: make([]float64, h),
		vw2: randomVector(h, 0.1),
		vb2: 0,

		alphaPi: 4e-4,
		alphaV:  9e-4,
		gamma:   0.97,
		lambda:  0.92,

		klTarget: 0.015,

		replayMax: 1024,
		batch: 48,

		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (p *PolicyGradientOptimizer) Act(state []float64) PolicyAction {

	p.mu.Lock()
	defer p.mu.Unlock()

	feat := p.encode(state)

	mean, _ := p.actorMean(feat)

	cov := p.covMatrix()

	act := sampleMVN(mean, cov, p.rng) // yha coupled explore

	safe := projectSafe(act) // yha safety shield

	p.storeLast(feat, mean, safe)

	return PolicyAction{
		ScaleOut:     safe[0],
		RetryBackoff: safe[1],
		QueueShard:   safe[2],
		CacheBoost:   safe[3],
	}
}

func (p *PolicyGradientOptimizer) Observe(nextState []float64, reward float64, risk float64, done bool) {

	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.replay) == 0 {
		return
	}

	last := &p.replay[len(p.replay)-1]

	last.reward = reward
	last.risk = risk
	last.done = done
	last.nextFeat = p.encode(nextState)

	last.priority = math.Abs(reward) + 2*risk // yha catastrophic weight

	if len(p.replay) >= p.batch {
		p.learn()
	}
}

func (p *PolicyGradientOptimizer) storeLast(feat, mean, act []float64) {

	ch := cholCopy(p.L)

	p.replay = append(p.replay, pgTransition{
		feat: feat,
		act: act,
		mean: mean,
		chol: ch,
		priority: 1,
	})

	if len(p.replay) > p.replayMax {
		p.replay = p.replay[1:]
	}
}

func (p *PolicyGradientOptimizer) learn() {

	sort.Slice(p.replay, func(i, j int) bool {
		return p.replay[i].priority > p.replay[j].priority
	})

	n := min(p.batch, len(p.replay))

	traj := p.replay[:n]

	advs := p.computeGAE(traj)

	for i := range traj {

		p.updateCritic(traj[i], advs[i])
		p.updateActor(traj[i], advs[i])
	}
}

func (p *PolicyGradientOptimizer) computeGAE(tr []pgTransition) []float64 {

	T := len(tr)
	adv := make([]float64, T)

	nextAdv := 0.0

	for t := T - 1; t >= 0; t-- {

		v := p.value(tr[t].feat)
		vNext := 0.0

		if !tr[t].done {
			vNext = p.value(tr[t].nextFeat)
		}

		delta := tr[t].reward - safetyCost(tr[t].risk) +
			p.gamma*vNext - v

		adv[t] = delta + p.gamma*p.lambda*nextAdv
		nextAdv = adv[t]
	}

	// yha normalize
	m := mean(adv)
	s := std(adv) + 1e-6

	for i := range adv {
		adv[i] = (adv[i] - m) / s
	}

	return adv
}

func (p *PolicyGradientOptimizer) updateActor(t pgTransition, adv float64) {

	mean, h := p.actorMean(t.feat)

	cov := p.covMatrix()

	logpGradMean := mvnGradMean(t.act, mean, cov)

	kl := klFull(t.mean, t.chol, mean, p.L)

	step := 1.0

	if kl > p.klTarget {
		step = p.klTarget / (kl + 1e-6) // yha adaptive step
	}

	for a := 0; a < p.aDim; a++ {

		for i := 0; i < p.hDim; i++ {
			g := adv * logpGradMean[a] * h[i] * step
			p.w2[a][i] += p.alphaPi * g
		}

		p.b2[a] += p.alphaPi * adv * logpGradMean[a] * step
	}

	for i := 0; i < p.hDim; i++ {

		sum := 0.0
		for a := 0; a < p.aDim; a++ {
			sum += logpGradMean[a] * p.w2[a][i]
		}

		dh := adv * sum * (1 - h[i]*h[i]) * step

		for j := 0; j < p.fDim; j++ {
			p.w1[i][j] += p.alphaPi * dh * t.feat[j]
		}
		p.b1[i] += p.alphaPi * dh
	}
}

func (p *PolicyGradientOptimizer) updateCritic(t pgTransition, adv float64) {

	target := adv + p.value(t.feat)

	h := make([]float64, p.hDim)

	for i := 0; i < p.hDim; i++ {
		sum := p.vb1[i]
		for j := 0; j < p.fDim; j++ {
			sum += p.vw1[i][j] * t.feat[j]
		}
		h[i] = math.Tanh(sum)
	}

	v := p.vb2
	for i := 0; i < p.hDim; i++ {
		v += p.vw2[i] * h[i]
	}

	err := target - v

	for i := 0; i < p.hDim; i++ {
		p.vw2[i] += p.alphaV * err * h[i]
	}

	p.vb2 += p.alphaV * err

	for i := 0; i < p.hDim; i++ {

		dh := err * p.vw2[i] * (1 - h[i]*h[i])

		for j := 0; j < p.fDim; j++ {
			p.vw1[i][j] += p.alphaV * dh * t.feat[j]
		}
		p.vb1[i] += p.alphaV * dh
	}
}

func (p *PolicyGradientOptimizer) actorMean(feat []float64) ([]float64, []float64) {

	h := make([]float64, p.hDim)

	for i := 0; i < p.hDim; i++ {
		sum := p.b1[i]
		for j := 0; j < p.fDim; j++ {
			sum += p.w1[i][j] * feat[j]
		}
		h[i] = math.Tanh(sum)
	}

	out := make([]float64, p.aDim)

	for a := 0; a < p.aDim; a++ {
		sum := p.b2[a]
		for i := 0; i < p.hDim; i++ {
			sum += p.w2[a][i] * h[i]
		}
		out[a] = 5 * math.Tanh(sum)
	}

	return out, h
}

func (p *PolicyGradientOptimizer) value(feat []float64) float64 {

	h := make([]float64, p.hDim)

	for i := 0; i < p.hDim; i++ {
		sum := p.vb1[i]
		for j := 0; j < p.fDim; j++ {
			sum += p.vw1[i][j] * feat[j]
		}
		h[i] = math.Tanh(sum)
	}

	v := p.vb2
	for i := 0; i < p.hDim; i++ {
		v += p.vw2[i] * h[i]
	}

	return v
}

func (p *PolicyGradientOptimizer) encode(s []float64) []float64 {

	out := make([]float64, p.fDim)

	for i := 0; i < p.fDim; i++ {

		sum := p.encB[i]

		for j := 0; j < p.sDim; j++ {
			sum += p.encW[i][j] * s[j]
		}

		out[i] = math.Tanh(sum) // yha representation learn
	}

	return out
}

/* ===== math helpers ===== */

func projectSafe(a []float64) []float64 {

	out := make([]float64, len(a))

	out[0] = clamp(a[0], 0, 6)
	out[1] = clamp(a[1], 0, 4)
	out[2] = clamp(a[2], 0, 5)
	out[3] = clamp(a[3], 0, 3)

	return out
}

func safetyCost(r float64) float64 {
	return 1.5*r + 0.6*r*r
}

func (p *PolicyGradientOptimizer) covMatrix() [][]float64 {

	L := p.L
	n := len(L)

	c := make([][]float64, n)
	for i := range c {
		c[i] = make([]float64, n)
		for j := 0; j <= i; j++ {
			for k := 0; k <= j; k++ {
				c[i][j] += L[i][k] * L[j][k]
			}
			c[j][i] = c[i][j]
		}
	}
	return c
}

func sampleMVN(mean []float64, cov [][]float64, rng *rand.Rand) []float64 {

	n := len(mean)

	z := make([]float64, n)
	for i := range z {
		z[i] = rng.NormFloat64()
	}

	L := chol(cov)

	out := make([]float64, n)

	for i := 0; i < n; i++ {
		sum := 0.0
		for j := 0; j <= i; j++ {
			sum += L[i][j] * z[j]
		}
		out[i] = mean[i] + sum
	}
	return out
}

func mvnGradMean(a, m []float64, cov [][]float64) []float64 {

	inv := invertMatrix(cov)
	g := make([]float64, len(a))

	for i := range a {
		for j := range a {
			g[i] += inv[i][j] * (a[j] - m[j])
		}
	}
	return g
}

func klFull(m0 []float64, L0 [][]float64, m1 []float64, L1 [][]float64) float64 {

	c0 := cholToCov(L0)
	c1 := cholToCov(L1)

	inv1 := invertMatrix(c1)

	n := len(m0)

	tr := 0.0
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			tr += inv1[i][j] * c0[j][i]
		}
	}

	dm := 0.0
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			dm += (m1[i]-m0[i]) * inv1[i][j] * (m1[j]-m0[j])
		}
	}

	logdet := math.Log(det(c1)/det(c0)+1e-6)

	return 0.5 * (tr + dm - float64(n) + logdet)
}

/* ===== small utils ===== */

func std(x []float64) float64 {
	m := mean(x)
	s := 0.0
	for _, v := range x {
		d := v - m
		s += d * d
	}
	return math.Sqrt(s / float64(len(x)))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func cholToCov(L [][]float64) [][]float64 {
	n := len(L)
	c := make([][]float64, n)
	for i := range c {
		c[i] = make([]float64, n)
		for j := 0; j <= i; j++ {
			for k := 0; k <= j; k++ {
				c[i][j] += L[i][k] * L[j][k]
			}
			c[j][i] = c[i][j]
		}
	}
	return c
}

func chol(a [][]float64) [][]float64 {

	n := len(a)
	L := make([][]float64, n)
	for i := range L {
		L[i] = make([]float64, n)
	}

	for i := 0; i < n; i++ {
		for j := 0; j <= i; j++ {

			sum := a[i][j]
			for k := 0; k < j; k++ {
				sum -= L[i][k] * L[j][k]
			}

			if i == j {
				if sum <= 0 {
					sum = 1e-6
				}
				L[i][j] = math.Sqrt(sum)
			} else {
				L[i][j] = sum / L[j][j]
			}
		}
	}
	return L
}

func cholCopy(L [][]float64) [][]float64 {
	n := len(L)
	c := make([][]float64, n)
	for i := range c {
		c[i] = make([]float64, n)
		copy(c[i], L[i])
	}
	return c
}


func det(a [][]float64) float64 {

	n := len(a)
	m := make([][]float64, n)
	for i := range m {
		m[i] = make([]float64, n)
		copy(m[i], a[i])
	}

	d := 1.0

	for i := 0; i < n; i++ {

		p := m[i][i]
		if math.Abs(p) < 1e-6 {
			return 1e-6
		}

		d *= p

		for j := i + 1; j < n; j++ {

			f := m[j][i] / p

			for k := i; k < n; k++ {
				m[j][k] -= f * m[i][k]
			}
		}
	}
	return math.Abs(d)
}
