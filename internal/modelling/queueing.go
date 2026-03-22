package modelling

import (
	"log"
	"math"
	"sync"
	"time"

	"github.com/loadequilibrium/loadequilibrium/internal/telemetry"
	"github.com/loadequilibrium/loadequilibrium/internal/topology"
)

// PhysicalQueueState maintains continuous system momentum between disjoint ticks.
type PhysicalQueueState struct {
	accumulatedBacklog   float64
	arrivalMomentum      float64
	localArrivalEwma     float64
	delayBuffer          [3]float64
	delayIdx             int
	lastTickTime         time.Time
}

// QueuePhysicsEngine encapsulates the Stateful Erlang-C fluid approximations.
type QueuePhysicsEngine struct {
	mu     sync.Mutex
	states map[string]*PhysicalQueueState
}

// NewQueuePhysicsEngine returns an initialised physics solver.
func NewQueuePhysicsEngine() *QueuePhysicsEngine {
	return &QueuePhysicsEngine{
		states: make(map[string]*PhysicalQueueState),
	}
}

// Prune cleans up dead services.
func (pe *QueuePhysicsEngine) Prune(activeIDs map[string]struct{}) {
	pe.mu.Lock()
	defer pe.mu.Unlock()
	for id := range pe.states {
		if _, ok := activeIDs[id]; !ok {
			delete(pe.states, id)
		}
	}
}

// RunQueueModel computes queueing analysis using M/M/c (Erlang-C) as the primary
// model, upgraded with continuous physical accumulation dynamics.
func (pe *QueuePhysicsEngine) RunQueueModel(w *telemetry.ServiceWindow, topoSnap topology.GraphSnapshot, medianMode bool) QueueModel {
	pe.mu.Lock()
	st, ok := pe.states[w.ServiceID]
	if !ok {
		st = &PhysicalQueueState{
			lastTickTime: time.Now(),
			localArrivalEwma: w.MeanRequestRate,
		}
		pe.states[w.ServiceID] = st
	}
	pe.mu.Unlock()

	dt := time.Since(st.lastTickTime).Seconds()
	if dt <= 0 || dt > 10.0 {
		dt = 2.0 // default tick clamp
	}
	st.lastTickTime = time.Now()

	m := QueueModel{
		ServiceID:  w.ServiceID,
		ComputedAt: time.Now(),
		Confidence: confidenceFromSamples(w.SampleCount),
	}
	if w.MeanRequestRate <= 0 || w.MeanLatencyMs <= 0 {
		return m
	}
	m.Hazard = w.Hazard
	m.Reservoir = w.Reservoir

	// ── Signal trust weight ───────────────────────────────────────────────────
	// Use the telemetry window's ConfidenceScore as a trust multiplier on all
	// trend-based predictions. When data quality is poor, predictions are
	// attenuated toward zero — the model degrades gracefully rather than
	// producing confidently wrong forecasts.
	// trustWeight ∈ [0.1, 1.0]: 1.0 = full trust, 0.1 = almost no trust.
	trustWeight := math.Max(w.ConfidenceScore, 0.10)

	// F. Capacity-Normalised Dynamics
	// F. Capacity-Normalised Dynamics
	c := math.Max(math.Round(w.MeanActiveConns), 1.0)
	m.Concurrency = c

	// C. Backlog Accumulation Memory & A. Arrival Burst Inertia
	// TelemetryCoupler already applies topology delay to w.MeanRequestRate.
	currentArrival := w.MeanRequestRate
	if currentArrival > st.arrivalMomentum {
		st.arrivalMomentum = 0.8*currentArrival + 0.2*st.arrivalMomentum // fast attack on bursts
	} else {
		st.arrivalMomentum = 0.2*currentArrival + 0.8*st.arrivalMomentum // slow momentum decay
	}
	m.ArrivalRate = st.arrivalMomentum

	// D. Latency Feedback Coupling
	// Normalise accumulated queue across concurrency

	// MINIMAL PATCH: TelemetryCoupler already persists physical latency penalties into
	// w.MeanLatencyMs. We use it directly to prevent compounding exponential explosions.
	effectiveLatency := w.MeanLatencyMs
	if effectiveLatency <= 0 {
		effectiveLatency = 50.0
	}

	// B. Service Saturation Model (Resource contention collapse)
	// Apply hazard-based degradation to service rate
	hazardFactor := math.Exp(-w.Hazard * 0.1)
	serviceRatePerServer := (1000.0 / math.Max(effectiveLatency, 1e-3)) * hazardFactor
	m.ServiceRate = serviceRatePerServer * c
	
	// C. Backlog Accumulation Memory & A. Arrival Burst Inertia
	// Integrate fluid mechanics across tick dt
	netFlowNormalised := (m.ArrivalRate - m.ServiceRate) / c
	st.accumulatedBacklog += netFlowNormalised * c * dt
	if st.accumulatedBacklog < 0 {
		st.accumulatedBacklog = 0.0
	}
	m.MeanQueueLen = st.accumulatedBacklog

	// Derived metrics
	m.Utilisation = m.ArrivalRate / math.Max(m.ServiceRate, 1e-3)
	if m.Utilisation > 1.0 && m.ServiceRate > 0 {
		m.MeanWaitMs = (m.MeanQueueLen / m.ServiceRate) * 1000.0
	} else {
		// M/M/c steady state approximation for un-saturated conditions
		a := m.ArrivalRate / serviceRatePerServer
		erlangC := computeErlangC(c, a)
		denom := c * serviceRatePerServer * (1.0 - m.Utilisation)
		if denom > 0 {
			m.MeanWaitMs = (erlangC / denom) * 1000.0
		} else {
			m.MeanWaitMs = 0
		}
	}
	m.AdjustedWaitMs = m.MeanWaitMs
	m.MeanSojournMs = m.MeanWaitMs + effectiveLatency
	m.BurstFactor = 1.0
	
	rawTrend := utilTrendRegression(w, m.ServiceRate)
	m.UtilisationTrend = rawTrend * trustWeight

	// Path saturation & confidence degrade
	if m.Utilisation < 1.0 && m.UtilisationTrend > 1e-6 {
		ttsSec := (1.0 - m.Utilisation) / m.UtilisationTrend
		m.SaturationHorizon = time.Duration(ttsSec * float64(time.Second))
		m.NetworkSaturationHorizon = m.SaturationHorizon
	}
	m.UpstreamPressure = computeInboundPressure(w) * trustWeight

	covPenalty := math.Exp(-w.StdRequestRate / math.Max(w.MeanRequestRate, 1) * 0.5)
	if math.IsNaN(covPenalty) || covPenalty <= 0 { covPenalty = 0.5 }
	m.Confidence = m.Confidence * covPenalty * stalePenalty(w, 2.0)

	// G. Structured Physics Logging
	log.Printf("[physics] svc=%s queue_next=%.3f effective_service_rate=%.3f latency_feedback=%.3f propagation_delay_stage=%d",
		w.ServiceID, m.MeanQueueLen, m.ServiceRate, 1.0, st.delayIdx)

	return m
}

// computeInboundPressure estimates the normalised inbound load pressure on this
// service from upstream callers in the dependency graph.
//
// The ServiceWindow's UpstreamEdges field contains OUTBOUND calls — services
// this node calls. To estimate inbound pressure, we use two proxies:
//
//  1. Queue depth ratio: a deep queue relative to active connections implies
//     heavy inbound load regardless of what we know about callers.
//     queueRatio = LastQueueDepth / MeanActiveConns
//
//  2. Arrival rate variance: high CoV of arrivals indicates bursty inbound load
//     from upstream callers that are themselves under pressure.
//
// The result is combined and normalised to [0,1].
// NOTE: When the orchestrator injects CoupledArrivalRate (from the network solver),
// the window's MeanRequestRate already includes upstream injection, so this
// function captures residual pressure not yet accounted for.
func computeInboundPressure(w *telemetry.ServiceWindow) float64 {
	if w.MeanRequestRate <= 0 {
		return 0
	}

	// Signal 1: queue depth ratio — proxy for inbound overload.
	queueRatio := 0.0
	if w.MeanActiveConns > 0 {
		queueRatio = math.Min(w.LastQueueDepth/w.MeanActiveConns, 1.0)
	}

	// Signal 2: arrival variance — bursty callers under pressure.
	arrivalVariance := 0.0
	if w.MeanRequestRate > 0 {
		cov := w.StdRequestRate / w.MeanRequestRate
		// Map CoV to [0,1]: CoV=0 → 0, CoV=2 → 1.
		arrivalVariance = math.Min(cov/2.0, 1.0)
	}

	// Combine: queue depth carries 60% weight (direct observable), variance 40%.
	combined := 0.60*queueRatio + 0.40*arrivalVariance
	return math.Min(combined, 1.0)
}

// computeErlangC computes the Erlang-C probability C(c,a) that an arrival
// must wait. Uses the recursive formula to avoid factorial overflow.
// c = servers, a = offered load (Erlangs = λ/μ).
func computeErlangC(c, a float64) float64 {
	ci := int(math.Round(c))
	if ci < 1 {
		ci = 1
	}
	rho := a / c
	if rho >= 1.0 {
		return 1.0
	}

	logA := math.Log(a)
	terms := make([]float64, ci)
	for k := 0; k < ci; k++ {
		logTermK := float64(k)*logA - logFactorial(k)
		terms[k] = logTermK
	}
	// log-sum-exp
	maxTerm := terms[0]
	for _, t := range terms {
		if t > maxTerm {
			maxTerm = t
		}
	}
	sumExp := 0.0
	for _, t := range terms {
		sumExp += math.Exp(t - maxTerm)
	}
	logSumK := maxTerm + math.Log(sumExp)

	// Last term: a^c / (c! * (1-ρ))
	logLastTerm := float64(ci)*logA - logFactorial(ci) - math.Log(1.0-rho)

	d := logLastTerm - logSumK
	if d > 700 {
		return 1.0
	}
	ratio := math.Exp(d)
	return ratio / (1.0 + ratio)
}

// logFactorial returns log(n!) using exact summation for small n.
func logFactorial(n int) float64 {
	if n <= 1 {
		return 0
	}
	sum := 0.0
	for i := 2; i <= n; i++ {
		sum += math.Log(float64(i))
	}
	return sum
}

// estimateServiceCoV estimates the coefficient of variation of service time
// from the P99/mean latency ratio.
func estimateServiceCoV(w *telemetry.ServiceWindow) float64 {
	if w.LastP99LatencyMs <= 0 || w.MeanLatencyMs <= 0 {
		return 1.0 // assume exponential service time
	}
	// Under exponential distribution P99 ≈ 4.6·mean; CoV=1.
	expectedP99Ratio := 4.6
	actualRatio := w.LastP99LatencyMs / w.MeanLatencyMs
	cov := math.Sqrt(math.Max((actualRatio/expectedP99Ratio), 0.1))
	return math.Min(cov, 5.0)
}

// utilTrendRegression estimates dρ/dt (ρ per second) using a weighted
// two-point OLS over the analysis window.
//
// With only mean/last aggregates available (no raw point array), we use:
//   slope = (ρ_last - ρ_mean) / (halfWindow_sec)
//
// This is the minimum-variance linear estimator given the available statistics.
// The result is weighted by sample confidence so sparse windows produce near-zero trends.
// Clamped to [-0.5, +0.5] ρ/s to prevent runaway predictions from transient noise.
func utilTrendRegression(w *telemetry.ServiceWindow, serviceRate float64) float64 {
	if w.SampleCount < 3 || serviceRate <= 0 {
		return 0
	}
	// Assume tick interval ≈ 2s; halfWindow covers half the samples.
	halfWindowSec := float64(w.SampleCount) * 2.0 / 2.0
	if halfWindowSec <= 0 {
		return 0
	}
	lastUtil := w.LastRequestRate / serviceRate
	meanUtil := w.MeanRequestRate / serviceRate
	slope := (lastUtil - meanUtil) / halfWindowSec

	// Weight by sample confidence: sparser windows contribute less slope.
	conf := confidenceFromSamples(w.SampleCount)
	slope *= conf

	return math.Max(-0.5, math.Min(slope, 0.5))
}

// stalePenalty computes a confidence multiplier [0,1] based on how old the
// most recent observation is relative to expected tick cadence.
// At age=0 → 1.0; at age=3×tickInterval → 0.37; at age=6×tickInterval → 0.14.
func stalePenalty(w *telemetry.ServiceWindow, tickInterval float64) float64 {
	if w.LastObservedAt.IsZero() {
		return 1.0
	}
	ageSec := time.Since(w.LastObservedAt).Seconds()
	if ageSec <= 0 {
		return 1.0
	}
	// Exponential decay: τ = 3 tick intervals
	tau := 3.0 * tickInterval
	return math.Exp(-ageSec / tau)
}

func confidenceFromSamples(n int) float64 {
	if n <= 0 {
		return 0
	}
	return 1.0 - math.Exp(-float64(n)/15.0)
}
