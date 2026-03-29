package intelligence

import (
	"math"
	"sync"
	"time"
)

type SignalVector struct {
	Timestamp time.Time
	BacklogError       float64
	LatencyError       float64
	ErrorRateError     float64
	CPUError           float64
	QueueDrift         float64
	RetryAmplification float64
}

type LearnedInstability struct {
	Score        float64
	Confidence   float64
	RegimeID     int
	RegimeProb   []float64
	HorizonRisk  float64
	OscModes     []float64
	Observability float64
}

type AdaptiveSignalLearner struct {
	mu sync.Mutex
	dim int

	A [][]float64
	P [][]float64
	lam float64

	mean []float64
	cov [][]float64
	covInv [][]float64
	covLam float64
	reg float64

	K int
	regMean [][]float64
	regCov [][][]float64
	regProb []float64
	trans [][]float64

	specBuf [][]float64
	specIdx int
	specWin int
	freqGrid []float64

	ctrlDelay int
	ctrlBound float64

	obsGram [][]float64
	obsEW float64
}

func NewAdaptiveSignalLearner(dim int) *AdaptiveSignalLearner {

	K := 4

	return &AdaptiveSignalLearner{
		dim: dim,
		A: identityMatrix(dim, 0.4),
		P: identityMatrix(dim, 40),
		lam: 0.996,

		mean: make([]float64, dim),
		cov: identityMatrix(dim, 1),
		covInv: identityMatrix(dim, 1),
		covLam: 0.996,
		reg: 1e-4,

		K: K,
		regMean: makeCentroids(K, dim),
		regCov: makeFullCovSet(K, dim),
		regProb: uniform(K),
		trans: makeTrans(K),

		specBuf: makeBuf(128, dim),
		specWin: 128,
		freqGrid: []float64{0.05, 0.12, 0.21},

		ctrlDelay: 3,
		ctrlBound: 4,

		obsGram: identityMatrix(dim, 0),
	}
}

func (l *AdaptiveSignalLearner) Update(v SignalVector) LearnedInstability {

	l.mu.Lock()
	defer l.mu.Unlock()

	x := vec(v)
	phi := nonlinearEmbed(x)

	l.updateCov(x)
	score := l.mahal(x)

	l.rls(phi, x)

	regProb := l.regimeFilter(x)
	reg := argmax(regProb)

	modes := l.spectral()

	risk := l.closedLoopRisk(x)

	obs := l.updateObs(phi)

	conf := l.conf(score, obs)

	return LearnedInstability{
		Score: score,
		Confidence: conf,
		RegimeID: reg,
		RegimeProb: regProb,
		HorizonRisk: risk,
		OscModes: modes,
		Observability: obs,
	}
}

/* ===== nonlinear embed ===== */

func nonlinearEmbed(x []float64) []float64 {

	n := len(x)
	out := make([]float64, 0, n+n*(n+1)/2)

	for i := 0; i < n; i++ {
		out = append(out, math.Tanh(x[i])) // yha nonlin compress
	}

	for i := 0; i < n; i++ {
		for j := i; j < n; j++ {
			out = append(out, x[i]*x[j]) // yha cross term add
		}
	}

	return out
}

/* ===== dynamics ===== */

func (l *AdaptiveSignalLearner) rls(phi []float64, x []float64) {

	for i := 0; i < l.dim; i++ {

		pred := dot(l.A[i], phi[:l.dim])
		err := x[i] - pred

		g := matVec(l.P, phi[:l.dim])
		den := l.lam + dot(phi[:l.dim], g)

		for j := 0; j < l.dim; j++ {
			l.A[i][j] += (g[j]/den)*err
		}

		l.P = joseph(l.P, phi[:l.dim], l.lam)
	}

	projectEigen(l.A, 0.95)
}

/* ===== covariance ===== */

func (l *AdaptiveSignalLearner) updateCov(x []float64) {

	for i := 0; i < l.dim; i++ {
		d := x[i] - l.mean[i]
		l.mean[i] += (1-l.covLam)*d
	}

	for i := 0; i < l.dim; i++ {
		for j := 0; j < l.dim; j++ {

			di := x[i] - l.mean[i]
			dj := x[j] - l.mean[j]

			l.cov[i][j] =
				l.covLam*l.cov[i][j] +
					(1-l.covLam)*di*dj

			if i == j {
				l.cov[i][j] += l.reg // yha conditioning
			}
		}
	}

	l.covInv = sherman(l.covInv, x, l.covLam)
}

func (l *AdaptiveSignalLearner) mahal(x []float64) float64 {

	d := 0.0
	for i := 0; i < l.dim; i++ {
		for j := 0; j < l.dim; j++ {
			di := x[i] - l.mean[i]
			dj := x[j] - l.mean[j]
			d += di * l.covInv[i][j] * dj
		}
	}
	return math.Sqrt(math.Abs(d))
}

/* ===== regime ===== */

func (l *AdaptiveSignalLearner) regimeFilter(x []float64) []float64 {

	like := make([]float64, l.K)

	for k := 0; k < l.K; k++ {

		inv := invertMatrix(l.regCov[k])
		d := 0.0

		for i := 0; i < l.dim; i++ {
			for j := 0; j < l.dim; j++ {
				di := x[i] - l.regMean[k][i]
				dj := x[j] - l.regMean[k][j]
				d += di * inv[i][j] * dj
			}
		}

		like[k] = math.Exp(-0.5 * d) // yha gaussian like
	}

	next := make([]float64, l.K)

	for j := 0; j < l.K; j++ {
		s := 0.0
		for i := 0; i < l.K; i++ {
			s += l.regProb[i] * l.trans[i][j]
		}
		next[j] = s * like[j]
	}

	normalize(next)
	l.regProb = next

	updateRegFull(l, x, argmax(next))

	return next
}

/* ===== spectral ===== */

func (l *AdaptiveSignalLearner) spectral() []float64 {

	modes := make([]float64, len(l.freqGrid))

	for fi, f := range l.freqGrid {

		s := 0.0
		c := 0.0

		for t := 0; t < l.specWin; t++ {
			p := 2 * math.Pi * f * float64(t)
			v := norm(l.specBuf[t])
			s += v * math.Sin(p)
			c += v * math.Cos(p)
		}

		modes[fi] = math.Sqrt(s*s+c*c) / float64(l.specWin)
	}

	return modes
}

/* ===== risk ===== */

func (l *AdaptiveSignalLearner) closedLoopRisk(x []float64) float64 {

	state := copyVec(x)
	delayBuf := make([][]float64, l.ctrlDelay)
	for i := range delayBuf {
		delayBuf[i] = make([]float64, l.dim)
	}

	risk := 0.0

	for k := 0; k < 14; k++ {

		next := matVec(l.A, state)

		ctrl := queuePolicy(state, l.ctrlBound)

		delayBuf[k%l.ctrlDelay] = ctrl

		for i := 0; i < l.dim; i++ {
			next[i] -= delayBuf[(k+1)%l.ctrlDelay][i] // yha delayed act
		}

		risk += math.Tanh(l.mahal(next) / 3.5)

		state = next
	}

	return math.Min(1, risk/14)
}

/* ===== observability ===== */

func (l *AdaptiveSignalLearner) updateObs(phi []float64) float64 {

	y := outer(phi[:l.dim], phi[:l.dim])

	for i := 0; i < l.dim; i++ {
		for j := 0; j < l.dim; j++ {
			l.obsGram[i][j] =
				0.97*l.obsGram[i][j] + 0.03*y[i][j] // yha gram update
		}
	}

	trace := 0.0
	for i := 0; i < l.dim; i++ {
		trace += l.obsGram[i][i]
	}

	normed := trace / (trace + 10) // yha scale fix

	l.obsEW = 0.9*l.obsEW + 0.1*normed

	return l.obsEW
}

/* ===== confidence ===== */

func (l *AdaptiveSignalLearner) conf(s, o float64) float64 {

	c := math.Exp(-0.18*s) * o

	if c < 0 {
		return 0
	}
	if c > 1 {
		return 1
	}
	return c
}

/* ===== utils ===== */

func projectEigen(A [][]float64, rho float64) {

	n := len(A)

	for i := 0; i < 6; i++ {

		v := adaptiveUnitVec(n)

		for k := 0; k < 10; k++ {
			v = matVec(A, v)
			v = normalizeVec(v)
		}

		l := norm(v)

		if l <= rho {
			return
		}

		scale := rho / l
		for i := range A {
			for j := range A {
				A[i][j] *= scale
			}
		}
	}
}

func queuePolicy(x []float64, b float64) []float64 {

	u := make([]float64, len(x))

	for i := range x {
		if x[i] > 1 {
			u[i] = math.Min(b, 0.8*x[i])
		} else {
			u[i] = 0.2 * x[i]
		}
	}
	return u
}

func updateRegFull(l *AdaptiveSignalLearner, x []float64, k int) {

	for i := 0; i < l.dim; i++ {
		d := x[i] - l.regMean[k][i]
		l.regMean[k][i] += 0.01 * d
	}

	for i := 0; i < l.dim; i++ {
		for j := 0; j < l.dim; j++ {

			di := x[i] - l.regMean[k][i]
			dj := x[j] - l.regMean[k][j]

			l.regCov[k][i][j] =
				0.995*l.regCov[k][i][j] + 0.005*di*dj
		}
	}

	for j := 0; j < l.K; j++ {
		l.trans[k][j] *= 0.995
	}
	l.trans[k][k] += 0.005
	normalize(l.trans[k])
}

/* small helpers */

func makeFullCovSet(K, d int) [][][]float64 {
	s := make([][][]float64, K)
	for k := range s {
		s[k] = identityMatrix(d, 2)
	}
	return s
}

func makeBuf(n, d int) [][]float64 {
	b := make([][]float64, n)
	for i := range b {
		b[i] = make([]float64, d)
	}
	return b
}

func makeTrans(K int) [][]float64 {
	t := make([][]float64, K)
	for i := range t {
		t[i] = make([]float64, K)
		t[i][i] = 1
	}
	return t
}

func makeCentroids(K, d int) [][]float64 {
	m := make([][]float64, K)
	for i := range m {
		m[i] = make([]float64, d)
	}
	return m
}

func vec(v SignalVector) []float64 {
	return []float64{
		v.BacklogError,
		v.LatencyError,
		v.ErrorRateError,
		v.CPUError,
		v.QueueDrift,
		v.RetryAmplification,
	}
}

func uniform(K int) []float64 {
	p := make([]float64, K)
	for i := range p {
		p[i] = 1 / float64(K)
	}
	return p
}

func matVec(m [][]float64, v []float64) []float64 {
	o := make([]float64, len(v))
	for i := range m {
		for j := range v {
			o[i] += m[i][j] * v[j]
		}
	}
	return o
}

func sherman(inv [][]float64, x []float64, lam float64) [][]float64 {

	v := matVec(inv, x)
	den := lam + dot(x, v)

	for i := range inv {
		for j := range inv {
			inv[i][j] =
				(inv[i][j] -
					v[i]*v[j]/den) / lam
		}
	}
	return inv
}

func joseph(P [][]float64, phi []float64, lam float64) [][]float64 {

	k := matVec(P, phi)
	den := lam + dot(phi, k)

	for i := range P {
		for j := range P {
			P[i][j] =
				(P[i][j] -
					k[i]*k[j]/den) / lam
		}
	}
	return P
}

func normalize(v []float64) {
	s := 0.0
	for _, x := range v {
		s += x
	}
	if s == 0 {
		return
	}
	for i := range v {
		v[i] /= s
	}
}

func argmax(v []float64) int {
	id := 0
	m := v[0]
	for i := 1; i < len(v); i++ {
		if v[i] > m {
			m = v[i]
			id = i
		}
	}
	return id
}

func outer(x, y []float64) [][]float64 {
	m := make([][]float64, len(x))
	for i := range m {
		m[i] = make([]float64, len(y))
		for j := range y {
			m[i][j] = x[i] * y[j]
		}
	}
	return m
}

func adaptiveUnitVec(n int) []float64 {
	v := make([]float64, n)
	for i := range v {
		v[i] = 1
	}
	return v
}

func normalizeVec(v []float64) []float64 {
	n := norm(v)
	if n == 0 {
		return v
	}
	for i := range v {
		v[i] /= n
	}
	return v
}

func copyVec(v []float64) []float64 {
	o := make([]float64, len(v))
	copy(o, v)
	return o
}
