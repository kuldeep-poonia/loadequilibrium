package modelling

import (
	"math"
	"time"

	"github.com/loadequilibrium/loadequilibrium/internal/telemetry"
)

// RunQueueModel computes queueing analysis using M/M/c (Erlang-C) as the primary
// model, with M/G/1 PKC correction for service-time variance and network physics:
//   - Coupled effective arrival rate from upstream dependency graph
//   - Congestion feedback: downstream saturation inflates this service's service time
//   - Path saturation horizon with linear + acceleration term
//   - Signal trust weighting: low ConfidenceScore scales back trend and CI estimates
//   - Numerical safety: NaN/Inf guards at all computed quantities
//
// medianMode selects the burst-resistant arrival rate estimator (Winsorised).
func RunQueueModel(w *telemetry.ServiceWindow, medianMode bool) QueueModel {
	m := QueueModel{
		ServiceID:  w.ServiceID,
		ComputedAt: time.Now(),
		Confidence: confidenceFromSamples(w.SampleCount),
	}
	if w.MeanRequestRate <= 0 || w.MeanLatencyMs <= 0 {
		return m
	}

	// ── Signal trust weight ───────────────────────────────────────────────────
	// Use the telemetry window's ConfidenceScore as a trust multiplier on all
	// trend-based predictions. When data quality is poor, predictions are
	// attenuated toward zero — the model degrades gracefully rather than
	// producing confidently wrong forecasts.
	// trustWeight ∈ [0.1, 1.0]: 1.0 = full trust, 0.1 = almost no trust.
	trustWeight := math.Max(w.ConfidenceScore, 0.10)

	// ── Delayed metric inference ─────────────────────────────────────────────
	// When telemetry is stale (LastObservedAt significantly old), extrapolate
	// the arrival rate forward using the utilisation trend rather than using the
	// last raw observation directly.
	//
	// This implements "delayed metric inference": if the last report is T seconds
	// old, the best estimate of current λ is: λ_est = λ_last × (1 + trend × T)
	// clamped to [0.5×λ_last, 2×λ_last] to prevent wild extrapolation.
	//
	// The inference degrades gracefully: confidence is already penalised by
	// stalePenalty, so this only affects the arrival rate used in the model
	// when the signal is stale but not yet pruned.
	inferredArrivalRate := w.LastRequestRate
	if !w.LastObservedAt.IsZero() {
		ageSec := time.Since(w.LastObservedAt).Seconds()
		// Apply inference only when noticeably stale (> 1 tick interval, approx 2s)
		// and we have a trend estimate from the window.
		if ageSec > 2.0 && w.SampleCount >= 3 {
			// Quick trend estimate: d(λ)/dt ≈ (LastRequestRate - MeanRequestRate) / halfWindowSec
			halfWindowSec := float64(w.SampleCount) * 2.0 / 2.0
			if halfWindowSec > 0 {
				dLambdaDt := (w.LastRequestRate - w.MeanRequestRate) / halfWindowSec
				extrapolated := w.LastRequestRate + dLambdaDt*ageSec*trustWeight
				// Clamp: ±50% of last known rate to prevent runaway inference.
				extrapolated = math.Max(extrapolated, w.LastRequestRate*0.5)
				extrapolated = math.Min(extrapolated, w.LastRequestRate*2.0)
				if extrapolated > 0 {
					inferredArrivalRate = extrapolated
				}
			}
		}
	}
	// ── Arrival rate estimation ───────────────────────────────────────────────
	if medianMode {
		m.ArrivalRate = medianBiasedRate(inferredArrivalRate, w.MeanRequestRate, w.StdRequestRate)
	} else {
		m.ArrivalRate = 0.6*inferredArrivalRate + 0.4*w.MeanRequestRate
	}
	if math.IsNaN(m.ArrivalRate) || m.ArrivalRate < 0 {
		m.ArrivalRate = w.MeanRequestRate
	}

	// ── Concurrency ───────────────────────────────────────────────────────────
	c := math.Max(math.Round(w.MeanActiveConns), 1)
	m.Concurrency = c

	// ── Service time estimation with congestion feedback ─────────────────────
	// Observed latency = service time + queue wait. When queue depth > 0, the
	// observed latency is inflated by waiting time. We deconvolve:
	//   E[S] ≈ observed_latency × (1 - queueFraction × 0.5)
	//
	// Congestion feedback: if this service's own queue is deep, downstream
	// dependencies may be slow-returning — they see our requests piling up and
	// their response latencies to us increase. We model this by inflating the
	// base service time estimate when the local queue depth is above 10% of
	// active connections. This captures the HOL (Head-of-Line) blocking effect
	// where a backlogged service incurs additional latency per request.
	queueFraction := math.Min(w.LastQueueDepth/(w.MeanActiveConns+1), 0.8)
	serviceTimeMs := w.MeanLatencyMs * (1.0 - queueFraction*0.5)

	// Congestion feedback amplification: deep local queue → inflate service time.
	// When qdepth > 20% of active connections, latency has an additional
	// contribution from downstream back-pressure. Model as linear amplification
	// capped at 2.0× (prevents runaway estimates from a single deep-queue observation).
	if queueFraction > 0.20 {
		// Each 10% of queue fraction beyond 20% adds 5% to service time.
		feedbackInflation := 1.0 + (queueFraction-0.20)*0.5
		if feedbackInflation > 2.0 {
			feedbackInflation = 2.0
		}
		serviceTimeMs *= feedbackInflation
	}

	// Guard: service time must be positive and finite.
	if serviceTimeMs <= 0 || math.IsNaN(serviceTimeMs) || math.IsInf(serviceTimeMs, 0) {
		serviceTimeMs = w.MeanLatencyMs
	}
	serviceTimeSec := serviceTimeMs / 1000.0

	muPerServer := 1.0 / serviceTimeSec
	m.ServiceRate = c * muPerServer

	// ── Coupled effective arrival rate ────────────────────────────────────────
	// Adjust the arrival rate to account for inbound load injected from upstream
	// callers in the dependency graph. This implements Part B req 1:
	// λ_effective = λ_local + Σ_callers(call_rate_from_caller × edge_weight_factor)
	//
	// Note: w.UpstreamEdges contains OUTBOUND calls from this service (services
	// it calls). To model INBOUND pressure, we use the edge weights from callers
	// via the UpstreamPressure scalar computed from topology. The orchestrator
	// also injects a pre-computed CoupledArrivalRate into w.MeanRequestRate
	// from the network coupling solver — so by the time RunQueueModel is called
	// the window's MeanRequestRate already reflects upstream injection.
	// We preserve the local estimate in m.ArrivalRate and build a coupled estimate.
	coupledArrivalRate := m.ArrivalRate * (1.0 + computeInboundPressure(w)*0.15)
	coupledArrivalRate = math.Min(coupledArrivalRate, m.ArrivalRate*3.0) // cap at 3×
	// Use the higher of local and coupled estimates (conservative).
	if coupledArrivalRate > m.ArrivalRate {
		m.ArrivalRate = coupledArrivalRate
	}

	// ── Utilisation ───────────────────────────────────────────────────────────
	m.Utilisation = m.ArrivalRate / m.ServiceRate
	if math.IsNaN(m.Utilisation) || math.IsInf(m.Utilisation, 0) {
		m.Utilisation = 0
		return m
	}
	rho := m.Utilisation

	// ── M/M/c Erlang-C queue analysis ─────────────────────────────────────────
	a := m.ArrivalRate / muPerServer
	if rho < 1.0 && a > 0 {
		erlangC := computeErlangC(c, a)
		denom := c * muPerServer * (1.0 - rho)
		if denom > 0 {
			m.MeanWaitMs = (erlangC / denom) * 1000.0
		}
		if math.IsNaN(m.MeanWaitMs) || math.IsInf(m.MeanWaitMs, 0) {
			m.MeanWaitMs = 0
		}
		m.MeanSojournMs = m.MeanWaitMs + serviceTimeMs
		m.MeanQueueLen = m.ArrivalRate * (m.MeanWaitMs / 1000.0)
		if math.IsNaN(m.MeanQueueLen) {
			m.MeanQueueLen = 0
		}
	} else {
		m.MeanQueueLen = math.Inf(1)
		m.MeanWaitMs = math.Inf(1)
		m.MeanSojournMs = math.Inf(1)
	}

	// ── M/G/1 PKC variance correction ─────────────────────────────────────────
	covSvc := estimateServiceCoV(w)
	m.BurstFactor = (1.0 + covSvc*covSvc) / 2.0
	if math.IsNaN(m.BurstFactor) {
		m.BurstFactor = 1.0
	}
	if rho < 1.0 {
		m.AdjustedWaitMs = m.MeanWaitMs * m.BurstFactor
		if math.IsNaN(m.AdjustedWaitMs) {
			m.AdjustedWaitMs = m.MeanWaitMs
		}
	} else {
		m.AdjustedWaitMs = math.Inf(1)
	}

	// ── Utilisation trend (OLS + confidence weighting) ────────────────────────
	// Scale the trend by the signal trust weight — low-confidence windows
	// produce attenuated trend estimates, reducing over-aggressive actuation.
	rawTrend := utilTrendRegression(w, m.ServiceRate)
	m.UtilisationTrend = rawTrend * trustWeight

	// ── Path saturation horizon solver ────────────────────────────────────────
	// Uses a second-order estimate: when the trend itself is accelerating
	// (positive second derivative, approximated from queue depth growth),
	// the linear extrapolation understimates how soon saturation occurs.
	//
	// Quadratic time-to-saturation: solves ρ + trend·t + ½·accel·t² = 1
	// → t = (-trend + sqrt(trend² + 2·accel·(1-ρ))) / accel
	// Falls back to linear when acceleration is near-zero.
	if rho < 1.0 && m.UtilisationTrend > 1e-6 {
		// Estimate acceleration from queue depth trend relative to mean.
		// queueFraction > 0.3 and increasing is a proxy for positive d²ρ/dt².
		accel := 0.0
		if queueFraction > 0.30 && m.UtilisationTrend > 0.01 {
			// Approximate: acceleration ≈ trend² / (1-ρ), capped at 0.05/s²
			accel = math.Min(m.UtilisationTrend*m.UtilisationTrend/(1.0-rho+1e-6), 0.05)
		}
		var ttsSec float64
		if accel < 1e-8 {
			// Linear: t = (1-ρ) / trend
			ttsSec = (1.0 - rho) / m.UtilisationTrend
		} else {
			// Quadratic: t = (-b + sqrt(b²+2a·c)) / a  where b=trend, a=accel, c=(1-ρ)
			discriminant := m.UtilisationTrend*m.UtilisationTrend + 2.0*accel*(1.0-rho)
			if discriminant > 0 {
				ttsSec = (-m.UtilisationTrend + math.Sqrt(discriminant)) / accel
			} else {
				ttsSec = (1.0 - rho) / m.UtilisationTrend
			}
		}
		if ttsSec > 0 && !math.IsNaN(ttsSec) && !math.IsInf(ttsSec, 0) {
			m.SaturationHorizon = time.Duration(ttsSec * float64(time.Second))
		}
	}

	// ── Upstream pressure and network-coupled saturation horizon ──────────────
	// Scale upstream pressure by trust weight — low-confidence windows should
	// not amplify upstream injection into aggressive saturation forecasts.
	m.UpstreamPressure = computeInboundPressure(w) * trustWeight

	if rho < 1.0 {
		// Network-coupled ρ: local + upstream injection damped by 0.15.
		// This models how upstream congestion raises the effective arrival rate.
		coupledRho := clamp(rho+m.UpstreamPressure*0.15, 0, 1.0)
		if coupledRho < 1.0 && m.UtilisationTrend > 1e-6 {
			netTtsSec := (1.0 - coupledRho) / m.UtilisationTrend
			if netTtsSec > 0 {
				m.NetworkSaturationHorizon = time.Duration(netTtsSec * float64(time.Second))
			}
		} else if coupledRho >= 1.0 {
			m.NetworkSaturationHorizon = 0
		} else {
			m.NetworkSaturationHorizon = m.SaturationHorizon
		}
	}

	// ── Confidence degradation ────────────────────────────────────────────────
	covPenalty := math.Exp(-w.StdRequestRate / math.Max(w.MeanRequestRate, 1) * 0.5)
	if math.IsNaN(covPenalty) || covPenalty <= 0 {
		covPenalty = 0.5
	}
	stale := stalePenalty(w, 2.0)
	m.Confidence = m.Confidence * covPenalty * stale
	if math.IsNaN(m.Confidence) || m.Confidence < 0 {
		m.Confidence = 0
	}

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
