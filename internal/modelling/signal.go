package modelling

import (
	"math"
	"sync"
	"time"

	"github.com/loadequilibrium/loadequilibrium/internal/telemetry"
)

const (
	cusumSlack     = 0.5
	cusumThreshold = 5.0
)

// SignalProcessor maintains per-service EWMA and CUSUM signal state.
// It is goroutine-safe so the model stage can run services concurrently.
type SignalProcessor struct {
	fastAlpha float64
	slowAlpha float64
	spikeK    float64
	mu        sync.Mutex
	states    map[string]*signalPerService
}

type signalPerService struct {
	fastEWMA     float64
	fastVariance float64
	slowEWMA     float64
	cusumPos     float64
	cusumNeg     float64
	initialized  bool
	lastUpdate   time.Time
}

func NewSignalProcessor(fastAlpha, slowAlpha, spikeK float64) *SignalProcessor {
	return &SignalProcessor{
		fastAlpha: fastAlpha,
		slowAlpha: slowAlpha,
		spikeK:    spikeK,
		states:    make(map[string]*signalPerService),
	}
}

// Update ingests a new window observation and returns the updated SignalState.
// Goroutine-safe — lock is held only for map access, not computation.
//
// Signal conditioning pipeline (applied before EWMA):
//  1. Spike rejection: if the raw sample deviates from the current estimate by
//     more than spikeK × σ, Winsorise it to mean ± spikeK×σ rather than
//     feeding the raw outlier into the EWMA (which would permanently bias it).
//  2. EWMA + CUSUM: standard dual-timescale tracking and change-point detection.
func (sp *SignalProcessor) Update(w *telemetry.ServiceWindow) SignalState {
	x := w.LastRequestRate
	id := w.ServiceID

	sp.mu.Lock()
	st, ok := sp.states[id]
	if !ok {
		st = &signalPerService{}
		sp.states[id] = st
	}
	sp.mu.Unlock()

	ss := SignalState{ServiceID: id, ComputedAt: time.Now()}

	sp.mu.Lock()
	defer sp.mu.Unlock()

	if !st.initialized {
		st.fastEWMA = x
		st.slowEWMA = x
		st.fastVariance = math.Max(x*0.01, 1.0)
		st.initialized = true
		st.lastUpdate = w.LastObservedAt
		ss.FastEWMA = x
		ss.SlowEWMA = x
		return ss
	}

	// ── Noise pre-filter: spike rejection via Winsorisation ──────────────────
	// If the incoming sample is beyond spikeK standard deviations from the
	// current fast EWMA, clamp it before feeding into any further computation.
	// This prevents a single measurement outlier (e.g. a metrics scrape artefact)
	// from permanently biasing the EWMA trajectory.
	stdDev := math.Sqrt(math.Max(st.fastVariance, 1e-12))
	filteredX := x
	deviation := x - st.fastEWMA
	if math.Abs(deviation) > sp.spikeK*stdDev {
		// Winsorise: project back to the boundary, preserving direction.
		filteredX = st.fastEWMA + math.Copysign(sp.spikeK*stdDev, deviation)
	}

	prevFast := st.fastEWMA
	st.fastEWMA = sp.fastAlpha*filteredX + (1-sp.fastAlpha)*st.fastEWMA
	st.slowEWMA = sp.slowAlpha*filteredX + (1-sp.slowAlpha)*st.slowEWMA

	// Variance tracks the filtered signal, not the raw outlier.
	diff := filteredX - prevFast
	st.fastVariance = (1-sp.fastAlpha)*(st.fastVariance+sp.fastAlpha*diff*diff)

	// Spike detection compares raw vs filtered to flag outliers.
	ss.SpikeDetected = math.Abs(x-prevFast) > sp.spikeK*stdDev

	// CUSUM operates on the filtered signal (normalised deviation from EWMA).
	normalisedDiff := diff / math.Max(stdDev, 1e-12)
	st.cusumPos = math.Max(0, st.cusumPos+normalisedDiff-cusumSlack)
	st.cusumNeg = math.Max(0, st.cusumNeg-normalisedDiff-cusumSlack)
	ss.ChangePointDetected = st.cusumPos > cusumThreshold || st.cusumNeg > cusumThreshold
	if ss.ChangePointDetected {
		st.cusumPos = 0
		st.cusumNeg = 0
	}

	ss.FastEWMA = st.fastEWMA
	ss.SlowEWMA = st.slowEWMA
	ss.EWMAVariance = st.fastVariance
	ss.CUSUMPos = st.cusumPos
	ss.CUSUMNeg = st.cusumNeg
	st.lastUpdate = w.LastObservedAt

	return ss
}

// Prune removes signal state for services that are no longer active.
func (sp *SignalProcessor) Prune(activeIDs map[string]struct{}) {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	for id := range sp.states {
		if _, ok := activeIDs[id]; !ok {
			delete(sp.states, id)
		}
	}
}
