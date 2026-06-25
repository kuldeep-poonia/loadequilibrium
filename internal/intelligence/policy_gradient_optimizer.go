package intelligence

import (
	"math"
	"math/rand"
	"sync"
	"time"
)

type PolicyAction struct {
	ScaleOut     float64
	RetryBackoff float64
	QueueShard   float64
	CacheBoost   float64
}

type PolicyGradientOptimizer struct {
	mu sync.Mutex

	sDim int
	fDim int
	aDim int
	hDim int

	encW [][]float64
	encB []float64

	w1 [][]float64
	b1 []float64
	w2 [][]float64
	b2 []float64

	L [][]float64

	vw1 [][]float64
	vb1 []float64
	vw2 []float64
	vb2 float64

	alphaPi float64
	alphaV  float64
	gamma   float64
	lambda  float64

	klTarget float64

	clipRange         float64
	rewardMean        float64
	rewardStd         float64
	rewardN           float64
	weightRegularizer float64

	expBuf *ExperienceBuffer
	batch  int
	gae    *GAETrajectoryEngine

	NaNCount       int
	InfCount       int
	GradExplosions int
	ClampedUpdates int

	pendingFeat []float64
	pendingAct  []float64
	pendingMean []float64
	pendingChol [][]float64
	hasPending  bool

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

		alphaPi: 1e-4, // Reduced from 4e-4
		alphaV:  2e-4, // Reduced from 9e-4
		gamma:   0.97,
		lambda:  0.92,

		klTarget: 0.015,

		// NEW: Initialize clipping and reward normalization
		clipRange:         0.05, // Tighter clipping to 0.05
		rewardMean:        0.0,
		rewardStd:         1.0,
		rewardN:           1.0,
		weightRegularizer: 0.01,

		expBuf: NewExperienceBuffer(2048, 5),
		batch:  48,
		gae:    NewGAETrajectoryEngine(4, 128),

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

	for i := range act {
		act[i] = p.checkNumericAnomaly(act[i])
	}
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

	if !p.hasPending {
		return
	}

	normalizedReward := p.normalizeReward(reward)
	impact := math.Abs(normalizedReward) + 2*risk

	e := Experience{
		State:     p.pendingFeat,
		Action:    p.pendingAct,
		Mean:      p.pendingMean,
		Chol:      p.pendingChol,
		Reward:    normalizedReward,
		Risk:      risk,
		NextState: p.encode(nextState),
		Done:      done,
		Impact:    impact,
		Priority:  impact,
	}

	p.expBuf.Add(e)
	p.hasPending = false

	// GAE Integration
	vCur := p.value(p.pendingFeat)
	vNext := 0.0
	if !done && len(nextState) > 0 {
		vNext = p.value(p.encode(nextState))
	}
	step := TrajectoryStep{
		Feature:       p.pendingFeat,
		Action:        p.pendingAct,
		Mean:          p.pendingMean,
		Chol:          p.pendingChol,
		Reward:        normalizedReward,
		Risk:          risk,
		Value:         vCur,
		RiskValue:     safetyCost(risk),
		Done:          done,
		NextValue:     vNext,
		NextRiskValue: safetyCost(risk),
		ISWeight:      1.0,
		RegimeID:      0,
	}
	p.gae.AddStep(step)

	if done || p.gae.head >= p.gae.maxEps {
		// Train on on-policy GAE episode
		advs, _ := p.gae.FinishEpisode()
		if len(advs) > 0 {
			ep := p.gae.buf[p.gae.head]
			for i := range ep {
				exp := Experience{
					State:  ep[i].Feature,
					Action: ep[i].Action,
					Mean:   ep[i].Mean,
					Chol:   ep[i].Chol,
				}
				p.updateCritic(exp, advs[i])
				p.updateActor(exp, advs[i])
			}
		}
		p.gae.StartEpisode()
	}

	if p.expBuf.size >= p.batch {
		p.learn()
	}
}

func (p *PolicyGradientOptimizer) storeLast(feat, mean, act []float64) {
	p.pendingFeat = feat
	p.pendingAct = act
	p.pendingMean = mean
	p.pendingChol = cholCopy(p.L)
	p.hasPending = true
}

func (p *PolicyGradientOptimizer) learn() {
	n := min(p.batch, p.expBuf.size)

	batch, weights, _ := p.expBuf.Sample(n)

	advs := p.computeAdvantage(batch)

	for i := range batch {
		weightedAdv := advs[i] * weights[i]
		p.updateCritic(batch[i], weightedAdv)
		p.updateActor(batch[i], weightedAdv)
	}
}

func (p *PolicyGradientOptimizer) computeAdvantage(batch []Experience) []float64 {
	T := len(batch)
	adv := make([]float64, T)

	for t := 0; t < T; t++ {
		v := p.value(batch[t].State)
		vNext := 0.0

		if !batch[t].Done && len(batch[t].NextState) > 0 {
			vNext = p.value(batch[t].NextState)
		}

		// 1-step TD error since buffer is randomly sampled
		delta := batch[t].Reward - safetyCost(batch[t].Risk) +
			p.gamma*vNext - v

		adv[t] = delta
	}

	m := p.checkNumericAnomaly(mean(adv))
	s := p.checkNumericAnomaly(std(adv)) + 1e-6

	for i := range adv {
		adv[i] = p.checkNumericAnomaly((adv[i] - m) / s)
	}

	return adv
}

func (p *PolicyGradientOptimizer) updateActor(t Experience, adv float64) {

	mean, h := p.actorMean(t.State)

	cov := p.covMatrix()

	logpGradMean := mvnGradMean(t.Action, mean, cov)

	kl := klFull(t.Mean, t.Chol, mean, p.L)

	step := 1.0

	if kl > p.klTarget {
		step = p.klTarget / (kl + 1e-6)
		step = p.checkNumericAnomaly(step)
	}

	// NEW: Gradient clipping - accumulate gradients and clip by norm
	w2Grads := make([][]float64, p.aDim)
	b2Grads := make([]float64, p.aDim)
	w1Grads := make([][]float64, p.hDim)
	b1Grads := make([]float64, p.hDim)

	for a := 0; a < p.aDim; a++ {
		w2Grads[a] = make([]float64, p.hDim)
		for i := 0; i < p.hDim; i++ {
			w2Grads[a][i] = adv * logpGradMean[a] * h[i] * step
		}
		b2Grads[a] = adv * logpGradMean[a] * step
	}

	for i := 0; i < p.hDim; i++ {
		sum := 0.0
		for a := 0; a < p.aDim; a++ {
			sum += logpGradMean[a] * p.w2[a][i]
		}
		dh := adv * sum * (1 - h[i]*h[i]) * step
		w1Grads[i] = make([]float64, p.fDim)
		for j := 0; j < p.fDim; j++ {
			w1Grads[i][j] = dh * t.State[j]
		}
		b1Grads[i] = dh
	}

	// Compute gradient norm
	gradNorm := 0.0
	for a := 0; a < p.aDim; a++ {
		for i := 0; i < p.hDim; i++ {
			g := w2Grads[a][i]
			gradNorm += g * g
		}
		gradNorm += b2Grads[a] * b2Grads[a]
	}
	for i := 0; i < p.hDim; i++ {
		for j := 0; j < p.fDim; j++ {
			g := w1Grads[i][j]
			gradNorm += g * g
		}
		gradNorm += b1Grads[i] * b1Grads[i]
	}
	gradNorm = math.Sqrt(gradNorm)

	// Clip gradients if norm exceeds threshold
	clipFactor := 1.0
	if gradNorm > p.clipRange {
		p.GradExplosions++
		p.GradExplosions++
		clipFactor = p.clipRange / gradNorm
	}

	// Apply clipped gradients with regularization
	for a := 0; a < p.aDim; a++ {
		for i := 0; i < p.hDim; i++ {
			g := w2Grads[a][i] * clipFactor
			// Add L2 regularization
			g -= p.weightRegularizer * p.w2[a][i]
			// Clip individual update
			if g > p.clipRange {
				g = p.clipRange
			}
			if g < -p.clipRange {
				g = -p.clipRange
			}
			p.w2[a][i] += p.alphaPi * g
		}
		g := b2Grads[a] * clipFactor
		// Clip individual update
		if g > p.clipRange {
			g = p.clipRange
		}
		if g < -p.clipRange {
			g = -p.clipRange
		}
		p.b2[a] += p.alphaPi * g
	}

	for i := 0; i < p.hDim; i++ {
		for j := 0; j < p.fDim; j++ {
			g := w1Grads[i][j] * clipFactor
			// Add L2 regularization
			g -= p.weightRegularizer * p.w1[i][j]
			// Clip individual update
			if g > p.clipRange {
				g = p.clipRange
			}
			if g < -p.clipRange {
				g = -p.clipRange
			}
			p.w1[i][j] += p.alphaPi * g
		}
		g := b1Grads[i] * clipFactor
		// Clip individual update
		if g > p.clipRange {
			g = p.clipRange
		}
		if g < -p.clipRange {
			g = -p.clipRange
		}
		p.b1[i] += p.alphaPi * g
	}
}

func (p *PolicyGradientOptimizer) updateCritic(t Experience, adv float64) {

	target := adv + p.value(t.State)

	h := make([]float64, p.hDim)

	for i := 0; i < p.hDim; i++ {
		sum := p.vb1[i]
		for j := 0; j < p.fDim; j++ {
			sum += p.vw1[i][j] * t.State[j]
		}
		h[i] = math.Tanh(sum)
	}

	v := p.vb2
	for i := 0; i < p.hDim; i++ {
		v += p.vw2[i] * h[i]
	}

	err := target - v

	// NEW: Gradient clipping for critic
	// Compute critic gradients and their norm
	vw2Grads := make([]float64, p.hDim)
	vb2Grad := err
	vw1Grads := make([][]float64, p.hDim)
	vb1Grads := make([]float64, p.hDim)

	for i := 0; i < p.hDim; i++ {
		vw2Grads[i] = err * h[i]
		vw1Grads[i] = make([]float64, p.fDim)
		dh := err * p.vw2[i] * (1 - h[i]*h[i])
		for j := 0; j < p.fDim; j++ {
			vw1Grads[i][j] = dh * t.State[j]
		}
		vb1Grads[i] = dh
	}

	// Compute gradient norm
	gradNorm := vb2Grad * vb2Grad
	for i := 0; i < p.hDim; i++ {
		gradNorm += vw2Grads[i] * vw2Grads[i]
		for j := 0; j < p.fDim; j++ {
			g := vw1Grads[i][j]
			gradNorm += g * g
		}
		gradNorm += vb1Grads[i] * vb1Grads[i]
	}
	gradNorm = math.Sqrt(gradNorm)

	// Clip gradients
	clipFactor := 1.0
	if gradNorm > p.clipRange {
		p.GradExplosions++
		p.GradExplosions++
		clipFactor = p.clipRange / gradNorm
	}

	// Apply clipped gradients
	for i := 0; i < p.hDim; i++ {
		g := vw2Grads[i] * clipFactor
		g -= p.weightRegularizer * p.vw2[i] // L2 regularization
		// Clip individual update
		if g > p.clipRange {
			g = p.clipRange
		}
		if g < -p.clipRange {
			g = -p.clipRange
		}
		p.vw2[i] += p.alphaV * g
	}

	g := vb2Grad * clipFactor
	// Clip individual update
	if g > p.clipRange {
		g = p.clipRange
	}
	if g < -p.clipRange {
		g = -p.clipRange
	}
	p.vb2 += p.alphaV * g

	for i := 0; i < p.hDim; i++ {
		for j := 0; j < p.fDim; j++ {
			g := vw1Grads[i][j] * clipFactor
			g -= p.weightRegularizer * p.vw1[i][j] // L2 regularization
			// Clip individual update
			if g > p.clipRange {
				g = p.clipRange
			}
			if g < -p.clipRange {
				g = -p.clipRange
			}
			p.vw1[i][j] += p.alphaV * g
		}
		g := vb1Grads[i] * clipFactor
		// Clip individual update
		if g > p.clipRange {
			g = p.clipRange
		}
		if g < -p.clipRange {
			g = -p.clipRange
		}
		p.vb1[i] += p.alphaV * g
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
	if len(feat) == 0 {
		return 0
	}

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
			dm += (m1[i] - m0[i]) * inv1[i][j] * (m1[j] - m0[j])
		}
	}

	logdet := math.Log(det(c1)/det(c0) + 1e-6)

	return 0.5 * (tr + dm - float64(n) + logdet)
}

/* ===== small utils ===== */

func (p *PolicyGradientOptimizer) checkNumericAnomaly(val float64) float64 {
	if math.IsNaN(val) {
		p.NaNCount++
		return 0
	}
	if math.IsInf(val, 0) {
		p.InfCount++
		if val > 0 {
			return 4.0
		}
		return -4.0
	}
	return val
}

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

// NEW: Reward normalization for stable learning under noisy/delayed rewards
func (p *PolicyGradientOptimizer) normalizeReward(r float64) float64 {
	// Update running statistics
	alpha := 0.01 // Smoothing factor
	delta := r - p.rewardMean
	p.rewardMean += alpha * delta
	p.rewardMean = p.checkNumericAnomaly(p.rewardMean)
	p.rewardStd = math.Sqrt((1-alpha)*(p.rewardStd*p.rewardStd) + alpha*(delta*delta))
	p.rewardStd = p.checkNumericAnomaly(p.rewardStd)

	// Normalize reward
	if p.rewardStd > 1e-6 {
		return p.checkNumericAnomaly((r - p.rewardMean) / p.rewardStd)
	}
	return p.checkNumericAnomaly(r)
}

// NEW: Compute total L2 norm of all network weights for stability monitoring
func (p *PolicyGradientOptimizer) TotalWeightNorm() float64 {
	norm := 0.0

	// Encoder weights
	for i := range p.encW {
		for j := range p.encW[i] {
			w := p.encW[i][j]
			norm += w * w
		}
	}
	for _, b := range p.encB {
		norm += b * b
	}

	// Actor mean weights
	for i := range p.w1 {
		for j := range p.w1[i] {
			w := p.w1[i][j]
			norm += w * w
		}
	}
	for _, b := range p.b1 {
		norm += b * b
	}

	for i := range p.w2 {
		for j := range p.w2[i] {
			w := p.w2[i][j]
			norm += w * w
		}
	}
	for _, b := range p.b2 {
		norm += b * b
	}

	// Actor covariance
	for i := range p.L {
		for j := range p.L[i] {
			w := p.L[i][j]
			norm += w * w
		}
	}

	// Critic weights
	for i := range p.vw1 {
		for j := range p.vw1[i] {
			w := p.vw1[i][j]
			norm += w * w
		}
	}
	for _, b := range p.vb1 {
		norm += b * b
	}
	for _, w := range p.vw2 {
		norm += w * w
	}
	norm += p.vb2 * p.vb2

	return math.Sqrt(norm)
}
