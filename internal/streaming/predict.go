package streaming

import (
	"math"

	"github.com/loadequilibrium/loadequilibrium/internal/modelling"
)

// BuildPredictionTimeline constructs N-tick utilisation prediction curves with
// 95% confidence intervals for each service.
//
// Model: ρ(t+k) = ρ₀ + trend × k × tickSec
// Confidence interval: ±z₀.₉₅ × σ_ρ × √k  (standard linear extrapolation CI)
// where σ_ρ is the coefficient of variation of arrivals scaled to utilisation units.
func BuildPredictionTimeline(
	bundles map[string]*modelling.ServiceModelBundle,
	horizon int,
	tickSec float64,
) map[string][]PredictionPoint {
	if horizon <= 0 {
		horizon = 8
	}
	const z95 = 1.645 // 95% CI z-score

	result := make(map[string][]PredictionPoint, len(bundles))
	for id, b := range bundles {
		rho := b.Queue.Utilisation
		trend := b.Queue.UtilisationTrend // ρ per second

		// Estimate σ_ρ from the stochastic CoV, scaled to utilisation units.
		// σ_ρ ≈ CoV × ρ × 0.5 (empirical damping for infrastructure forecasting).
		sigmaRho := b.Stochastic.ArrivalCoV * rho * 0.5
		if sigmaRho < 0.01 {
			sigmaRho = 0.01
		}

		points := make([]PredictionPoint, horizon+1)
		for k := 0; k <= horizon; k++ {
			predRho := rho + trend*float64(k)*tickSec
			predRho = math.Max(0, math.Min(predRho, 1.5)) // cap at 150% for display

			// 95% CI widens with √k (random walk model).
			ciHalf := z95 * sigmaRho * math.Sqrt(float64(k+1))

			points[k] = PredictionPoint{
				TickOffset: k,
				Rho:        predRho,
				Lower95:    math.Max(0, predRho-ciHalf),
				Upper95:    math.Min(1.5, predRho+ciHalf),
			}
		}
		result[id] = points
	}
	return result
}

// BuildRiskTimeline constructs per-service predicted risk trajectories over the
// prediction horizon, showing CollapseRisk evolution under current trends.
//
// At each tick k, we project:
//   ρ(k) = ρ₀ + trend × k × tickSec
//   CollapseRisk(k) = sigmoid((ρ(k) - threshold×0.95) / 0.04)
//
// This gives operators a "risk runway" — the number of ticks until each service
// crosses 0.5 or 0.8 CollapseRisk, enabling proactive decisions rather than
// reactive responses to already-occurred threshold breaches.
func BuildRiskTimeline(
	bundles map[string]*modelling.ServiceModelBundle,
	horizon int,
	tickSec float64,
	collapseThreshold float64,
) PredictiveRiskTimeline {
	if horizon <= 0 {
		horizon = 8
	}
	if collapseThreshold <= 0 {
		collapseThreshold = 0.85
	}

	result := make(PredictiveRiskTimeline, len(bundles))
	for id, b := range bundles {
		rho := b.Queue.Utilisation
		trend := b.Queue.UtilisationTrend
		points := make([]RiskTimelinePoint, horizon+1)
		for k := 0; k <= horizon; k++ {
			projRho := rho + trend*float64(k)*tickSec
			if projRho < 0 {
				projRho = 0
			}
			if projRho > 1.5 {
				projRho = 1.5
			}
			risk := 1.0 / (1.0 + math.Exp(-(projRho-collapseThreshold*0.95)/0.04))
			points[k] = RiskTimelinePoint{
				TickOffset:   k,
				Rho:          projRho,
				CollapseRisk: risk,
			}
		}
		result[id] = points
	}
	return result
}
