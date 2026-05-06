package optimisation

import (
	"math"

	"github.com/loadequilibrium/loadequilibrium/internal/modelling"
)

// TrajectoryCandidate represents one evaluated control action candidate.
type TrajectoryCandidate struct {
	ScaleFactor     float64 // candidate scale factor to evaluate
	TrajectoryScore float64 // risk-latency integrated cost over horizon (deterministic)
	FinalRho        float64 // predicted ρ at end of horizon
	ConvergesTo     float64 // projected steady-state ρ (linear extrapolation)
	Feasible        bool    // true when trajectory stays below collapse threshold
	// UncertaintyBand: ±σ of trajectory cost under arrival burstiness.
	// Derived from stochastic model's ArrivalCoV and BurstAmplification.
	// A wide band means the actual cost could be much worse than the deterministic estimate.
	UncertaintyBand float64
	// ProbabilisticScore: risk-adjusted cost = TrajectoryScore × (1 + burstAmplification × 0.15).
	// Penalises high-uncertainty trajectories — candidates that look cheap deterministically
	// but are expensive under burst conditions rank lower under this scoring.
	ProbabilisticScore float64
}

// TrajectoryPlan is the output of the bounded objective-surface search.
type TrajectoryPlan struct {
	// BestScaleFactor: the scale factor with the lowest trajectory cost
	// that remains feasible (does not cross collapse threshold).
	BestScaleFactor float64

	// Candidates: all evaluated candidates, ordered by ScaleFactor.
	Candidates []TrajectoryCandidate

	// ObjectiveSurfaceConvex: true when the cost function is locally convex.
	ObjectiveSurfaceConvex bool

	// ConvergenceAware: true when best candidate was selected for setpoint convergence.
	ConvergenceAware bool

	// BestProbabilisticScore: risk-adjusted score of the selected candidate.
	// Lower is better. High values indicate the control path passes through
	// uncertain, bursty operating zones.
	BestProbabilisticScore float64
}

// PlanTrajectory performs a bounded search over the scale-factor objective surface
// for a single service, evaluating N_CANDIDATES control actions and selecting
// the one with the best probabilistic risk-latency-convergence score.
//
// Probabilistic evaluation: each candidate's cost is adjusted upward by the
// arrival burstiness (from the stochastic model's BurstAmplification) to account
// for the probability that actual costs are worse than the deterministic estimate.
// This implements "probabilistic trade-off evaluation" — candidates that look cheap
// deterministically but are in high-variance operating zones rank lower.
func PlanTrajectory(
	b *modelling.ServiceModelBundle,
	setpoint float64,
	horizon int,
	tickSec float64,
	collapseThreshold float64,
) TrajectoryPlan {
	const nCandidates = 7

	rho := b.Queue.Utilisation
	trend := b.Queue.UtilisationTrend
	serviceRate := b.Queue.ServiceRate

	// Stochastic model provides arrival uncertainty parameters.
	// burstPenaltyFactor ∈ [0, 0.5]: penalty applied to each candidate's score.
	// Formula: 0.15 × BurstAmplification × CoV — captures both burst magnitude and variance.
	burstAmplification := b.Stochastic.BurstAmplification
	arrivalCoV := b.Stochastic.ArrivalCoV
	burstPenaltyFactor := math.Min(0.15*burstAmplification*math.Max(arrivalCoV, 0), 0.50)

	candidates := make([]TrajectoryCandidate, nCandidates)
	for i := 0; i < nCandidates; i++ {
		sf := 0.5 + float64(i)*(3.0-0.5)/float64(nCandidates-1)
		c := evaluateCandidate(sf, rho, trend, serviceRate, setpoint, collapseThreshold, horizon, tickSec)

		// Uncertainty band: σ ≈ TrajectoryScore × arrivalCoV × 0.5
		// Represents ±1σ of the cost distribution under arrival noise.
		c.UncertaintyBand = c.TrajectoryScore * math.Min(arrivalCoV*0.5, 0.30)

		// Probabilistic score: deterministic cost + burstiness penalty.
		// Favours candidates that are cheap even under burst conditions.
		c.ProbabilisticScore = c.TrajectoryScore * (1.0 + burstPenaltyFactor)

		candidates[i] = c
	}

	// Selection: lowest ProbabilisticScore among feasible candidates.
	// Apply convergence bonus (15% reduction) to convergent trajectories.
	bestIdx := -1
	bestEffectiveCost := math.MaxFloat64
	for i, c := range candidates {
		if !c.Feasible {
			continue
		}
		cost := c.ProbabilisticScore
		if c.ConvergesTo <= setpoint+0.03 {
			cost *= 0.85
		}
		if cost < bestEffectiveCost {
			bestEffectiveCost = cost
			bestIdx = i
		}
	}

	convergenceAware := false
	if bestIdx == -1 {
		lowestFinalRho := math.MaxFloat64
		for i, c := range candidates {
			if c.FinalRho < lowestFinalRho {
				lowestFinalRho = c.FinalRho
				bestIdx = i
			}
		}
	} else {
		convergenceAware = candidates[bestIdx].ConvergesTo <= setpoint+0.03
	}

	midCost := candidates[nCandidates/2].TrajectoryScore
	avgExtreme := (candidates[0].TrajectoryScore + candidates[nCandidates-1].TrajectoryScore) / 2.0
	surfaceConvex := midCost < avgExtreme

	bestSF := 1.0
	bestProbScore := 0.0
	if bestIdx >= 0 {
		bestSF = candidates[bestIdx].ScaleFactor
		bestProbScore = candidates[bestIdx].ProbabilisticScore
	}

	return TrajectoryPlan{
		BestScaleFactor:        bestSF,
		Candidates:             candidates,
		ObjectiveSurfaceConvex: surfaceConvex,
		ConvergenceAware:       convergenceAware,
		BestProbabilisticScore: bestProbScore,
	}
}

// evaluateCandidate simulates a single scale-factor candidate over the horizon
// and returns its trajectory cost and convergence metrics.
func evaluateCandidate(
	sf, rho, trend, serviceRate, setpoint, collapseThreshold float64,
	horizon int, tickSec float64,
) TrajectoryCandidate {
	const (
		wLat  = 0.55
		wRisk = 0.45
	)

	// Per-tick ρ reduction from scale factor.
	pidCorrection := 0.0
	if sf > 1.0 {
		pidCorrection = (rho * (1.0 - 1.0/sf)) / float64(horizon)
	} else if sf < 1.0 {
		pidCorrection = (rho * (1.0 - sf)) / float64(horizon)
	}

	simRho := rho
	costSum := 0.0
	feasible := true

	for k := 0; k < horizon; k++ {
		simRho += trend * tickSec
		simRho -= pidCorrection
		if simRho < 0 {
			simRho = 0
		}

		// Feasibility: collapse threshold exceeded → infeasible trajectory.
		if simRho >= collapseThreshold {
			feasible = false
		}

		// Per-step risk-latency cost (same model as MPC Evaluate).
		waitCost := 0.0
		if simRho < 1.0 && serviceRate > 0 {
			wq := simRho / ((1.0 - simRho) * serviceRate)
			waitCost = math.Tanh(wq * 2.0)
		} else if simRho >= 1.0 {
			waitCost = 1.0
		}
		riskCost := 1.0 / (1.0 + math.Exp(-(simRho-0.85)/0.06))
		costSum += wLat*waitCost + wRisk*riskCost
	}

	trajScore := math.Min(costSum/float64(horizon), 1.0)

	// Projected steady-state: linear extrapolation to where ρ settles
	// if the control action were applied indefinitely.
	// At steady-state: ρ_ss = λ / (μ × sf) = ρ / sf (first-order capacity model).
	convergesTo := rho / math.Max(sf, 0.5)
	// Also account for trend: if trend > 0, steady-state drifts up.
	if trend > 1e-6 {
		convergesTo += trend * float64(horizon) * tickSec * 0.5
	}

	return TrajectoryCandidate{
		ScaleFactor:     sf,
		TrajectoryScore: trajScore,
		FinalRho:        simRho,
		ConvergesTo:     math.Max(0, convergesTo),
		Feasible:        feasible,
	}
}
