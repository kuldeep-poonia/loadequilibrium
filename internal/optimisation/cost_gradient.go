package optimisation

import (
	"math"

	"github.com/loadequilibrium/loadequilibrium/internal/modelling"
	"github.com/loadequilibrium/loadequilibrium/internal/topology"
)

// ServiceCostContribution quantifies how much each service contributes to
// the global objective function. This enables stability-aware control:
// services with high cost contribution deserve tighter control action.
type ServiceCostContribution struct {
	ServiceID string

	// LatencyCost: this service's weighted contribution to the predicted P99 score.
	LatencyCost float64

	// StabilityCost: 1 - StabilityMargin, weighted by arrival rate.
	StabilityCost float64

	// CascadeCost: contribution to system cascade risk (proportional to FeedbackGain).
	CascadeCost float64

	// TotalCost: weighted sum of all three dimensions (same weights as ComputeObjective).
	TotalCost float64

	// CostGradient: dTotalCost/dρ — how fast the cost increases as utilisation rises.
	// High gradient = aggressive control needed; near-zero = cost insensitive to ρ.
	CostGradient float64
}

// ComputeCostGradients derives per-service cost contributions and gradients
// from the current model bundles and topology snapshot.
// This is called once per control cycle and passed to RunControl for
// stability-aware actuation scaling.
func ComputeCostGradients(
	bundles map[string]*modelling.ServiceModelBundle,
	topo topology.GraphSnapshot,
	refLatencyMs float64,
) map[string]ServiceCostContribution {
	const (
		wLatency     = 0.40
		wStability   = 0.20
		wCascade     = 0.30
		wOscillation = 0.10
	)

	result := make(map[string]ServiceCostContribution, len(bundles))
	if len(bundles) == 0 {
		return result
	}

	// Normalise arrival rates for latency weighting.
	totalArrival := 0.0
	for _, b := range bundles {
		totalArrival += math.Max(b.Queue.ArrivalRate, 0.01)
	}
	if totalArrival <= 0 {
		totalArrival = 1
	}

	for id, b := range bundles {
		rho := b.Queue.Utilisation
		arrivalWeight := math.Max(b.Queue.ArrivalRate, 0.01) / totalArrival

		p99 := b.Queue.AdjustedWaitMs
		if math.IsInf(p99, 0) || math.IsNaN(p99) {
			p99 = 1e5
		}
		latencyCost := wLatency * math.Min(p99/refLatencyMs, 1.0) * arrivalWeight

		// Stability cost: (1 - StabilityMargin) weighted by arrival share.
		stabCost := wStability * math.Max(1.0-b.Stability.StabilityMargin, 0) * arrivalWeight

		// Cascade cost: FeedbackGain × CollapseRisk captures downstream blast radius.
		cascadeCost := wCascade * b.Stability.CascadeAmplificationScore * arrivalWeight

		// Oscillation cost.
		oscCost := wOscillation * b.Stability.OscillationRisk * arrivalWeight

		totalCost := latencyCost + stabCost + cascadeCost + oscCost

		// Cost gradient: d(totalCost)/dρ
		// For latency: d(AdjustedWait)/dρ diverges as ρ→1 (M/M/c explosion).
		// d(Wq)/dρ ≈ μ⁻¹ / (1-ρ)² for M/M/1 approximation.
		const sigScale = 0.04
		dWaitDRho := 0.0
		if rho < 1.0 && b.Queue.ServiceRate > 0 {
			denomSq := (1.0 - rho) * (1.0 - rho)
			if denomSq > 1e-6 {
				dWaitDRho = (1.0 / b.Queue.ServiceRate) / denomSq * 1000.0 // ms
			}
		} else if rho >= 1.0 {
			dWaitDRho = 1e6 // overloaded — infinite sensitivity
		}
		dLatCostDRho := wLatency * math.Min(dWaitDRho/refLatencyMs, 10.0) * arrivalWeight

		// d(StabCost)/dρ: sigmoid derivative × weight.
		dRiskDRho := b.Stability.CollapseRisk * (1.0 - b.Stability.CollapseRisk) / sigScale
		dStabDRho := wStability * dRiskDRho * arrivalWeight

		costGradient := dLatCostDRho + dStabDRho

		result[id] = ServiceCostContribution{
			ServiceID:     id,
			LatencyCost:   latencyCost,
			StabilityCost: stabCost,
			CascadeCost:   cascadeCost,
			TotalCost:     totalCost,
			CostGradient:  math.Min(costGradient, 10.0),
		}
	}
	return result
}
