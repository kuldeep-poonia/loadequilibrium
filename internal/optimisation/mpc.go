package optimisation

import (
	"math"

	"github.com/loadequilibrium/loadequilibrium/internal/modelling"
)

// MPCHorizonEval evaluates a short predictive control horizon to determine
// whether the current PID output actually moves the system toward the setpoint
// over the next N ticks, or whether it risks overshoot.
//
// It runs a lightweight linear simulation of ρ(t+k) for k=1..horizon using
// the current utilisation trend as the uncontrolled trajectory and the PID
// output as a corrective step per tick.
//
// Returns an adjusted scale factor that is damped when the simulation predicts
// overshoot, and amplified when under-actuation is detected.
type MPCHorizonEval struct {
	horizon      int     // number of ticks to simulate ahead
	tickSec      float64 // seconds per tick (for trend extrapolation)
	setpoint     float64
	maxOvershoot float64 // max acceptable predicted overshoot above setpoint
}

// NewMPCHorizonEval creates a MPC evaluator.
func NewMPCHorizonEval(horizon int, tickSec, setpoint float64) *MPCHorizonEval {
	return &MPCHorizonEval{
		horizon:      horizon,
		tickSec:      tickSec,
		setpoint:     setpoint,
		maxOvershoot: 0.05, // 5% above setpoint is acceptable
	}
}

// MPCResult holds the MPC evaluation output for one service.
type MPCResult struct {
	// AdjustedScaleFactor is the MPC-corrected recommendation.
	AdjustedScaleFactor float64

	// PredictedRhoAtHorizon is ρ at the end of the prediction window.
	PredictedRhoAtHorizon float64

	// OvershootRisk is true when the trajectory passes above setpoint+maxOvershoot.
	OvershootRisk bool

	// UnderactuationRisk is true when rho is projected to remain above setpoint.
	UnderactuationRisk bool

	// TrajectoryCostAvg: mean per-step risk-latency cost over the horizon [0,1].
	// A high average indicates the control trajectory passes through dangerous zones.
	TrajectoryCostAvg float64

	// MaxTrajectoryCost: worst single-step cost over the horizon [0,1].
	// Used to detect transient spikes even when the average is acceptable.
	MaxTrajectoryCost float64
}

// sigmoid maps x → 1/(1+e^-x) using the standard logistic function.
func sigmoid(x float64) float64 {
	return 1.0 / (1.0 + math.Exp(-x))
}

// Evaluate runs the MPC simulation for one service.
//
// The trajectory is evaluated step-by-step over the full horizon. At each step:
//   - ρ is projected forward using trend + PID correction
//   - A per-step cost is computed as a risk-latency trade-off:
//     cost(k) = w_lat × normalisedWait(ρ) + w_risk × collapseRisk(ρ)
//   - The trajectory cost integral is used to detect overshoot/undershoot
//     not just by final position, but by the worst-case cost along the path.
//
// This transforms the MPC from a binary overshoot detector into a stability-aware
// trajectory optimiser that penalises paths passing through high-risk zones.
func (m *MPCHorizonEval) Evaluate(
	b *modelling.ServiceModelBundle,
	pidOutput float64,
	currentScale float64,
) MPCResult {
	rho := b.Queue.Utilisation
	trend := b.Queue.UtilisationTrend
	serviceRate := b.Queue.ServiceRate

	// PID correction per tick: models how capacity addition reduces ρ.
	pidCorrection := 0.0
	if currentScale > 1.0 {
		pidCorrection = (rho * (1.0 - 1.0/currentScale)) / float64(m.horizon)
	} else if currentScale < 1.0 {
		pidCorrection = (rho * (1.0 - currentScale)) / float64(m.horizon)
	}

	// Risk-latency trade-off weights.
	// These are deliberately asymmetric: latency cost grows nonlinearly near saturation,
	// so a trajectory that briefly passes through ρ=0.95 is much worse than one
	// that stays at ρ=0.85 — the integral captures this properly.
	const (
		wLat  = 0.55 // latency cost weight
		wRisk = 0.45 // collapse risk weight
	)

	simRho := rho
	overshootRisk := false
	minRho := rho
	maxTrajectoryCost := 0.0
	trajectoryCostSum := 0.0

	for k := 0; k < m.horizon; k++ {
		simRho += trend * m.tickSec
		simRho -= pidCorrection
		if simRho < 0 {
			simRho = 0
		}

		if simRho < minRho {
			minRho = simRho
		}
		if simRho < m.setpoint-m.maxOvershoot {
			overshootRisk = true
		}

		// Per-step cost: normalised M/M/1 wait + sigmoid collapse risk.
		// normalisedWait: Wq ∝ ρ/(1-ρ), normalised to [0,1] via tanh scaling.
		waitCost := 0.0
		if simRho < 1.0 && serviceRate > 0 {
			wq := simRho / ((1.0 - simRho) * serviceRate) // M/M/1 wait (s)
			waitCost = math.Tanh(wq * 2.0)                // 0→0, large→1
		} else if simRho >= 1.0 {
			waitCost = 1.0
		}
		riskCost := sigmoid((simRho - 0.85) / 0.06) // sigmoid centred at 0.85

		stepCost := wLat*waitCost + wRisk*riskCost
		trajectoryCostSum += stepCost
		if stepCost > maxTrajectoryCost {
			maxTrajectoryCost = stepCost
		}
	}

	trajectoryCostAvg := trajectoryCostSum / float64(m.horizon)
	undershoot := simRho > m.setpoint+m.maxOvershoot

	// Scale factor adjustment uses trajectory cost rather than final-position only.
	// High trajectory cost (path through risky zone) → amplify; overshoot → damp.
	// CRITICAL: During collapse (rho ≥ 0.85), NEVER amplify — collapse requires dampening only.
	adjusted := currentScale
	if overshootRisk && currentScale > 1.0 {
		overshootMag := math.Max(m.setpoint-m.maxOvershoot-minRho, 0)
		dampFactor := 1.0 - overshootMag*0.3
		adjusted = 1.0 + (currentScale-1.0)*math.Max(dampFactor, 0.3)
	} else if undershoot && currentScale > 1.0 && rho < 0.85 {
		// ONLY amplify when undershoot is detected AND not in collapse zone (rho < 0.85).
		// When rho ≥ 0.85, always damp to prevent positive feedback amplification.
		costAmplification := math.Min((simRho-m.setpoint)*0.3+trajectoryCostAvg*0.15, 0.20)
		adjusted = currentScale * (1.0 + costAmplification)
	}

	adjusted = math.Max(0.5, math.Min(adjusted, 3.0))

	return MPCResult{
		AdjustedScaleFactor:   adjusted,
		PredictedRhoAtHorizon: simRho,
		OvershootRisk:         overshootRisk,
		UnderactuationRisk:    undershoot,
		TrajectoryCostAvg:     trajectoryCostAvg,
		MaxTrajectoryCost:     maxTrajectoryCost,
	}
}
