package intelligence

import (
	"math"
	"math/rand"
	"sync"
	"time"
)

type Experience struct {
	State     []float64
	Action    []float64
	Reward    float64
	Risk      float64
	NextState []float64
	Done      bool

	EpisodeID int
	RegimeID  int

	Timestamp time.Time

	TDError float64
	Novelty float64
	Impact  float64

	Priority float64
}

type ExperienceBuffer struct {
	mu sync.Mutex

	capacity int
	size     int

	data []Experience

	base int
	tree []float64

	alpha float64
	beta  float64

	metaTheta []float64 // yha nonlinear meta param

	minPrio float64

	episodeSlots map[int]map[int]struct{}
	regimeSlots  map[int][]int

	driftEW []float64
	driftDecay float64

	rng *rand.Rand
}

func NewExperienceBuffer(cap int, regimes int) *ExperienceBuffer {

	base := 1
	for base < cap {
		base <<= 1
	}

	return &ExperienceBuffer{
		capacity: cap,
		data:     make([]Experience, cap),

		base: base,
		tree: make([]float64, 2*base),

		alpha: 0.7,
		beta:  0.4,

		metaTheta: []float64{0.8, 0.6, 0.7, 0.9},

		minPrio: 1e-4,

		episodeSlots: make(map[int]map[int]struct{}),
		regimeSlots:  make(map[int][]int),

		driftEW: make([]float64, regimes),
		driftDecay: 0.995,

		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (b *ExperienceBuffer) Add(e Experience) {

	b.mu.Lock()
	defer b.mu.Unlock()

	idx := b.nextInsertIndex()

	old := b.data[idx]

	if old.EpisodeID != 0 {
		delete(b.episodeSlots[old.EpisodeID], idx)
	}

	b.data[idx] = e

	if b.episodeSlots[e.EpisodeID] == nil {
		b.episodeSlots[e.EpisodeID] = make(map[int]struct{})
	}
	b.episodeSlots[e.EpisodeID][idx] = struct{}{}

	b.regimeSlots[e.RegimeID] =
		append(b.regimeSlots[e.RegimeID], idx)

	e.Priority = b.computePriority(e)

	b.updateTree(idx, e.Priority)

	b.updateDrift(e.RegimeID)

	b.size++
}

func (b *ExperienceBuffer) Sample(batch int) ([]Experience, []float64, []int) {

	b.mu.Lock()
	defer b.mu.Unlock()

	total := b.tree[1]

	out := make([]Experience, batch)
	w := make([]float64, batch)
	idxs := make([]int, batch)

	for i := 0; i < batch; i++ {

		r := b.rng.Float64() * total

		idx := b.sampleTree(r)

		out[i] = b.data[idx]
		idxs[i] = idx

		p := math.Max(b.priorityAt(idx), b.minPrio) / total

		w[i] = math.Pow(float64(b.size)*p, -b.beta)
	}

	maxW := 0.0
	for _, v := range w {
		if v > maxW {
			maxW = v
		}
	}
	for i := range w {
		w[i] /= (maxW + 1e-9)
	}

	// yha anneal beta
	if b.beta < 1 {
		b.beta += 0.0002
	}

	return out, w, idxs
}

func (b *ExperienceBuffer) RegimeBatch(reg int, k int) []Experience {

	b.mu.Lock()
	defer b.mu.Unlock()

	idxs := b.regimeSlots[reg]

	if len(idxs) == 0 {
		return nil
	}

	out := make([]Experience, k)

	for i := 0; i < k; i++ {

		best := b.sampleTree(b.rng.Float64() * b.tree[1])

		out[i] = b.data[best]
	}

	return out
}

func (b *ExperienceBuffer) DriftScore(reg int) float64 {
	return b.driftEW[reg]
}

/* ===== nonlinear priority ===== */

func (b *ExperienceBuffer) computePriority(e Experience) float64 {

	x1 := math.Pow(e.TDError+1e-6, b.alpha)
	x2 := e.Risk
	x3 := e.Novelty
	x4 := e.Impact

	z :=
		b.metaTheta[0]*x1*x2 +
			b.metaTheta[1]*x1*x3 +
			b.metaTheta[2]*x2*x3 +
			b.metaTheta[3]*x4

	p := math.Abs(math.Tanh(z))

	if p < b.minPrio {
		p = b.minPrio
	}

	return p
}

/* ===== drift ===== */

func (b *ExperienceBuffer) updateDrift(reg int) {

	for i := range b.driftEW {
		b.driftEW[i] *= b.driftDecay
	}

	b.driftEW[reg] += (1 - b.driftDecay)
}

/* ===== insert logic ===== */

func (b *ExperienceBuffer) nextInsertIndex() int {

	if b.size < b.capacity {
		return b.size
	}

	for i := 0; i < b.capacity; i++ {

		idx := (b.size + i) % b.capacity

		if b.data[idx].Done {
			return idx
		}
	}

	return b.size % b.capacity
}

/* ===== segment tree ===== */

func (b *ExperienceBuffer) updateTree(idx int, p float64) {

	i := idx + b.base

	b.tree[i] = math.Max(p, b.minPrio)

	for i >>= 1; i >= 1; i >>= 1 {
		b.tree[i] = b.tree[i<<1] + b.tree[i<<1|1]
	}
}

func (b *ExperienceBuffer) sampleTree(v float64) int {

	i := 1

	for i < b.base {

		if v <= b.tree[i<<1] {
			i = i << 1
		} else {
			v -= b.tree[i<<1]
			i = i<<1 | 1
		}
	}

	return i - b.base
}

func (b *ExperienceBuffer) priorityAt(idx int) float64 {
	return b.tree[idx+b.base]
}