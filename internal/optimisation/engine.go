package optimisation

import (
	"log"
	"math"
	"sort"
	"time"

	"github.com/loadequilibrium/loadequilibrium/internal/config"
	"github.com/loadequilibrium/loadequilibrium/internal/modelling"
	"github.com/loadequilibrium/loadequilibrium/internal/simulation"
	"github.com/loadequilibrium/loadequilibrium/internal/topology"
)

// Engine runs the optimisation and control loop across all services.
type Engine struct {
	cfg           *config.Config
	pids          map[string]*PIDController
	lastScale     map[string]float64
	stictionCount map[string]int
	mpc           *MPCHorizonEval
	observer      *AdaptiveStateObserver
	// pressureLevel is set by the orchestrator each tick before RunControl is called.
	// 0=nominal  1=elevated  2=high  3=critical
	// Higher levels make PID more aggressive to defend against runtime overload.
	pressureLevel int
}

// SetPressureLevel injects the current runtime pressure level into the engine
// so that PID aggressiveness and actuation bounds adapt to system conditions.
// Must be called before RunControl each tick.
func (e *Engine) SetPressureLevel(level int) {
	if level < 0 {
		level = 0
	}
	if level > 3 {
		level = 3
	}
	e.pressureLevel = level
}

func NewEngine(cfg *config.Config) *Engine {
	horizonTicks := cfg.PredictiveHorizonTicks
	if horizonTicks <= 0 {
		horizonTicks = 5
	}
	return &Engine{
		cfg:           cfg,
		pids:          make(map[string]*PIDController),
		lastScale:     make(map[string]float64),
		stictionCount: make(map[string]int),
		mpc:           NewMPCHorizonEval(horizonTicks, cfg.TickInterval.Seconds(), cfg.UtilisationSetpoint),
		observer:      NewAdaptiveStateObserver(),
	}
}

// EvaluateCandidates evaluates scale candidates without selecting or emitting
// executable directives. This is the optimizer's advisory path for the single
// control-authority architecture.
func (e *Engine) EvaluateCandidates(
	bundles map[string]*modelling.ServiceModelBundle,
	costGradients map[string]ServiceCostContribution,
	lastSimResult *simulation.SimulationResult,
	topo topology.GraphSnapshot,
	now time.Time,
) map[string][]ControlCandidate {
	_ = lastSimResult
	_ = topo

	horizonTicks := e.cfg.PredictiveHorizonTicks
	if horizonTicks <= 0 {
		horizonTicks = 5
	}

	out := make(map[string][]ControlCandidate, len(bundles))
	for id, b := range bundles {
		plan := PlanTrajectory(
			b,
			e.cfg.UtilisationSetpoint,
			horizonTicks,
			e.cfg.TickInterval.Seconds(),
			e.cfg.CollapseThreshold,
		)

		gradient := 0.0
		if cg, ok := costGradients[id]; ok {
			gradient = cg.CostGradient
		}

		cands := make([]ControlCandidate, 0, len(plan.Candidates)+1)
		for _, c := range plan.Candidates {
			score := c.ProbabilisticScore
			if !c.Feasible {
				score += 10
			}
			if gradient > 0 {
				score += math.Min(gradient, 5) * 0.02
			}
			cands = append(cands, ControlCandidate{
				ServiceID:    id,
				ScaleFactor:  c.ScaleFactor,
				Score:        score,
				Feasible:     c.Feasible,
				Convergent:   c.ConvergesTo <= e.cfg.UtilisationSetpoint+0.03,
				PredictedRho: c.FinalRho,
				RiskScore:    c.TrajectoryScore,
				Uncertainty:  c.UncertaintyBand,
				Source:       "optimisation.trajectory",
				ComputedAt:   now,
			})
		}

		cands = append(cands, ControlCandidate{
			ServiceID:    id,
			ScaleFactor:  1.0,
			Score:        math.Max(0, b.Queue.Utilisation-e.cfg.UtilisationSetpoint),
			Feasible:     b.Queue.Utilisation < e.cfg.CollapseThreshold,
			Convergent:   math.Abs(b.Queue.Utilisation-e.cfg.UtilisationSetpoint) < 0.05,
			PredictedRho: b.Queue.Utilisation,
			RiskScore:    b.Stability.CollapseRisk,
			Uncertainty:  b.Stochastic.ArrivalCoV,
			Source:       "optimisation.hold",
			ComputedAt:   now,
		})

		sort.Slice(cands, func(i, j int) bool {
			return cands[i].Score < cands[j].Score
		})
		out[id] = cands
	}

	return out
}

// RunControl executes one PID control cycle per service.
// costGradients carries per-service d(cost)/dρ from ComputeCostGradients —
// services with high gradient receive amplified actuation.
func (e *Engine) RunControl(
	bundles map[string]*modelling.ServiceModelBundle,
	costGradients map[string]ServiceCostContribution,
	lastSimResult *simulation.SimulationResult,
	topo topology.GraphSnapshot,
	now time.Time,
) map[string]ControlDirective {
	observerSignals := e.observer.Observe(bundles, lastSimResult, topo)
	directives := make(map[string]ControlDirective, len(bundles))

	horizonTicks := e.cfg.PredictiveHorizonTicks
	if horizonTicks <= 0 {
		horizonTicks = 5
	}

	for id, b := range bundles {
		pid, ok := e.pids[id]
		if !ok {
			pid = NewPIDController(
				e.cfg.PIDKp, e.cfg.PIDKi, e.cfg.PIDKd,
				e.cfg.UtilisationSetpoint,
				e.cfg.PIDDeadband,
				e.cfg.PIDIntegralMax,
			)
			e.pids[id] = pid
		}

		rho := b.Queue.Utilisation

		// Dynamic hysteresis tuning: widen deadband when service is healthy;
		// narrow it when approaching collapse. Additionally, runtime pressure
		// overrides zone-based tuning — high pressure always tightens the deadband
		// so the controller acts more aggressively when the system is under load.
		switch {
		case e.pressureLevel >= 2:
			// High/critical runtime pressure: tightest deadband — react to small deviations.
			pid.Deadband = math.Max(e.cfg.PIDDeadband*0.4, 0.005)
		case b.Stability.CollapseZone == "collapse" || e.pressureLevel == 1:
			pid.Deadband = math.Max(e.cfg.PIDDeadband*0.5, 0.005)
		case b.Stability.CollapseZone == "warning":
			pid.Deadband = e.cfg.PIDDeadband
		default:
			// Safe zone, nominal pressure: wider deadband suppresses micro-corrections.
			pid.Deadband = math.Min(e.cfg.PIDDeadband*1.5, 0.06)
		}

		// Predictive target: project N ticks ahead using utilisation trend.
		// If rho is trending up, we issue control against the predicted future state.
		predictiveTarget := rho + b.Queue.UtilisationTrend*float64(horizonTicks)*2.0
		predictiveTarget = math.Max(0, math.Min(predictiveTarget, 1.5))

		output := pid.Update(rho, now)

		// CRITICAL FIX: Gate PID output based on collapse zone, not rho
		// During collapse zone (zone=collapse): ONLY allow dampening, NEVER amplify
		// This prevents amplification even when delayed telemetry makes rho appear high
		if b.Stability.CollapseZone == "collapse" && output > 0 {
			// Collapse zone: suppress any amplification from PID
			// During recovery from actual overload, even with stale telemetry,
			// we must prevent amplification until queue physically drains
			output = -0.2 // Small negative to gently reduce scale toward 1.0
		}

		// D. Soft Constraint Dynamics: replacing hard 0.5 saturation with a smooth exponential barrier
		bareScale := 1.0 + output
		var scaleFactor float64
		if bareScale < 0.6 {
			// Asymptote towards 0.45 smoothly instead of a hard panic clamp at 0.5
			scaleFactor = 0.45 + 0.15*math.Exp((bareScale-0.6)/0.15)
		} else {
			scaleFactor = math.Min(bareScale, 3.0)
		}

		obs := observerSignals[id]

		// B. Stability Margin Awareness: Modulate throttling aggressiveness
		if scaleFactor < 1.0 && obs.StabilityEnvelope > 0.3 {
			stabilityRelaxation := (obs.StabilityEnvelope - 0.3) * 0.25
			scaleFactor = math.Min(scaleFactor+stabilityRelaxation, 1.0)
		}

		// A. Recovery Incentive Term: Reward gradual utilisation restoration when queue pressure decreases
		// CRITICAL: Only apply recovery incentive in safe/warning zones, NOT during collapse
		// (collapse zone = ρ ≥ 0.90, in which case we need maximum dampening, not relief)
		recoveryIncentive := 0.0
		if b.Stability.CollapseZone != "collapse" && obs.RecoveryActivation > 0 && scaleFactor < 0.9 {
			recoveryIncentive = math.Min(obs.RecoveryActivation*0.5, 0.20)
			scaleFactor += recoveryIncentive
		}

		// C. Predictive Relief Estimation: Forecast decay
		// CRITICAL: Only add relief in safe/warning zones; during collapse, maintaining maximum dampening is required
		predictedReliefMs := 0.0
		if lastSimResult != nil && lastSimResult.RecoveryConvergenceMs > 0 {
			predictedReliefMs = lastSimResult.RecoveryConvergenceMs
		}
		predictedRelief := 0.0
		if b.Stability.CollapseZone != "collapse" && obs.DisturbanceDecay > 0 {
			predictedRelief = math.Min(obs.DisturbanceDecay*0.1, 0.1)
			scaleFactor += predictedRelief
		}

		// Actuation bound: limit per-tick scale factor change.
		// Under runtime pressure, widen the bound so the controller can respond more
		// sharply — the system needs faster recovery more than stability protection.
		maxScaleDelta := 0.30
		switch e.pressureLevel {
		case 2:
			maxScaleDelta = 0.50
		case 3:
			maxScaleDelta = 0.75 // critical pressure — allow large single-step actuation
		}
		if last, ok := e.lastScale[id]; ok {
			delta := scaleFactor - last
			if math.Abs(delta) > maxScaleDelta {
				scaleFactor = last + math.Copysign(maxScaleDelta, delta)
			}
		}
		e.lastScale[id] = scaleFactor

		// P4: Capture the PID-bounded scale BEFORE MPC and trajectory adjustments.
		// The anti-stiction check must compare against this snapshot, not against
		// e.lastScale[id] which gets overwritten three times in the lines below.
		// Previously, the stiction counter fired every tick (delta was always 0).
		prevScaleForStiction := scaleFactor

		// MPC short-horizon evaluation: adjust scale factor to avoid overshoot/undershoot.
		mpcRes := e.mpc.Evaluate(b, output, scaleFactor)
		scaleFactor = mpcRes.AdjustedScaleFactor
		e.lastScale[id] = scaleFactor

		// Stability-aware actuation amplification from cost gradient.
		// CRITICAL: During collapse, NEVER amplify — collapse requires dampening only
		gradientAmplification := 1.0
		if b.Stability.CollapseZone != "collapse" {
			if cg, ok := costGradients[id]; ok && cg.CostGradient > 0.5 {
				gradientAmplification = 1.0 + math.Min(cg.CostGradient/10.0*0.25, 0.25)
				scaleFactor = math.Min(scaleFactor*gradientAmplification, 3.0)
				e.lastScale[id] = scaleFactor
			}
		}

		// Trajectory planner: bounded objective-surface search over N candidate
		// scale factors to find the convergence-aware optimum.
		// The planner's recommendation is blended with the PID+MPC result:
		// if the planner finds a better convergent trajectory, we shift toward it.
		// CRITICAL: During collapse, NEVER blend with trajectory planner — use PID+MPC only.
		plan := PlanTrajectory(b, e.cfg.UtilisationSetpoint, horizonTicks,
			e.cfg.TickInterval.Seconds(), e.cfg.CollapseThreshold)
		if b.Stability.CollapseZone != "collapse" {
			// Only blend with planner in safe/warning zones
			if plan.BestScaleFactor > 0 && plan.ConvergenceAware {
				// Blend: 60% PID/MPC, 40% trajectory planner when planner is convergence-aware.
				scaleFactor = 0.60*scaleFactor + 0.40*plan.BestScaleFactor
			} else if plan.BestScaleFactor > 0 && !plan.ObjectiveSurfaceConvex {
				// Non-convex surface: rely more on planner which searched more broadly.
				scaleFactor = 0.45*scaleFactor + 0.55*plan.BestScaleFactor
			}
		}
		if b.Stability.CollapseZone != "safe" && rho >= e.cfg.UtilisationSetpoint {
			scaleFactor = math.Min(scaleFactor, 0.95)
		}
		scaleFactor = math.Max(0.45, math.Min(scaleFactor, 6.0))
		// E. Anti-Stiction Mechanism: Detect stuck limits and inject exploratory bump.
		// Compares final scale against prevScaleForStiction (the PID-bounded value
		// before MPC and planner) to detect when the whole optimizer chain has left
		// the scale genuinely unchanged — not just within the same assignment.
		stictionVal := 0
		if math.Abs(scaleFactor-prevScaleForStiction) < 0.02 && scaleFactor < 0.75 {
			e.stictionCount[id]++
			stictionVal = e.stictionCount[id]
			if stictionVal > 4 { // 5th tick of being stuck
				scaleFactor += 0.08 // explicit exploratory adjustment
				e.stictionCount[id] = 0
			}
		} else {
			e.stictionCount[id] = 0
		}
		e.lastScale[id] = scaleFactor

		// F. Observability: Emitting structured logs
		log.Printf("[control] svc=%s bare_out=%.3f recovery_incentive=%.3f stability_margin=%.3f predicted_relief_ms=%.0f adjusted_limit=%.3f stiction=%d",
			id, bareScale, recoveryIncentive, b.Stability.StabilityMargin, predictedReliefMs, scaleFactor, stictionVal)

		cgVal := 0.0
		if cg, ok := costGradients[id]; ok {
			cgVal = cg.CostGradient
		}

		directives[id] = ControlDirective{
			ServiceID:                 id,
			ComputedAt:                now,
			ScaleFactor:               scaleFactor,
			TargetUtilisation:         e.cfg.UtilisationSetpoint,
			Error:                     rho - e.cfg.UtilisationSetpoint,
			PIDOutput:                 output,
			Active:                    pid.Active,
			StabilityMargin:           b.Stability.StabilityMargin,
			HysteresisState:           pid.HysteresisState,
			ActuationBound:            maxScaleDelta,
			PredictiveTarget:          predictiveTarget,
			MPCPredictedRho:           mpcRes.PredictedRhoAtHorizon,
			MPCOvershootRisk:          mpcRes.OvershootRisk,
			MPCUnderactuationRisk:     mpcRes.UnderactuationRisk,
			CostGradient:              cgVal,
			TrajectoryCostAvg:         mpcRes.TrajectoryCostAvg,
			MaxTrajectoryCost:         mpcRes.MaxTrajectoryCost,
			PlannerScaleFactor:        plan.BestScaleFactor,
			PlannerConvergent:         plan.ConvergenceAware,
			PlannerConvex:             plan.ObjectiveSurfaceConvex,
			PlannerProbabilisticScore: plan.BestProbabilisticScore,
		}
	}

	// Remove PID and scale state for services that no longer report.
	for id := range e.pids {
		if _, ok := bundles[id]; !ok {
			delete(e.pids, id)
			delete(e.lastScale, id)
		}
	}

	return directives
}

// sortFloat64s sorts a float64 slice in ascending order (insertion sort — small N).
func sortFloat64s(s []float64) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// Objective: minimise composite of predicted tail latency, cascade failure
// probability, and weighted instability — balancing all four axes explicitly.
//
// Reference latency is derived dynamically from the observed P99 distribution,
// not a hardcoded constant, so the score is meaningful at any traffic level.
func ComputeObjective(
	bundles map[string]*modelling.ServiceModelBundle,
	topo topology.GraphSnapshot,
	now time.Time,
) ObjectiveScore {
	if len(bundles) == 0 {
		return ObjectiveScore{ComputedAt: now}
	}

	// Trade-off weights (explicit, not hidden).
	const (
		wLatency     = 0.40
		wCascade     = 0.30
		wInstability = 0.20
		wOscillation = 0.10
	)

	var (
		sumWeightedLatency float64
		sumWeight          float64
		harmonicStabSum    float64
		harmonicCount      int
		maxCollapseRisk    float64
		sumOscRisk         float64
		// Trend-adjusted stability: harmonic mean of TrendAdjustedMargin.
		trendStabSum   float64
		trendStabCount int
		// Risk acceleration: max positive StabilityDerivative across services.
		maxRiskAccel float64
	)

	// Compute dynamic reference latency: 90th percentile of adjusted wait times
	// across services. This anchors the score to actual system behaviour.
	waitSamples := make([]float64, 0, len(bundles))
	for _, b := range bundles {
		p99 := b.Queue.AdjustedWaitMs
		if !math.IsInf(p99, 0) && !math.IsNaN(p99) && p99 > 0 {
			waitSamples = append(waitSamples, p99)
		}
	}
	refLatencyMs := 500.0 // fallback
	if len(waitSamples) >= 3 {
		// Sort and take 90th pct as reference.
		sortFloat64s(waitSamples)
		idx := int(math.Round(float64(len(waitSamples)) * 0.90))
		if idx >= len(waitSamples) {
			idx = len(waitSamples) - 1
		}
		refLatencyMs = math.Max(waitSamples[idx]*1.5, 200.0) // 1.5× the observed p90 as target
	}

	for _, b := range bundles {
		w := math.Max(b.Queue.ArrivalRate, 0.01)
		p99 := b.Queue.AdjustedWaitMs
		if math.IsInf(p99, 0) || math.IsNaN(p99) {
			p99 = 1e5
		}
		sumWeightedLatency += p99 * w
		sumWeight += w

		m := b.Stability.StabilityMargin
		if m > 0 {
			harmonicStabSum += 1.0 / m
			harmonicCount++
		}
		if b.Stability.CollapseRisk > maxCollapseRisk {
			maxCollapseRisk = b.Stability.CollapseRisk
		}
		sumOscRisk += b.Stability.OscillationRisk

		// Trend-adjusted margin (pessimistic forward-looking stability).
		tam := b.Stability.TrendAdjustedMargin
		if tam > 0 {
			trendStabSum += 1.0 / tam
			trendStabCount++
		}

		// Risk acceleration — penalise services whose collapse risk is growing fast.
		if b.Stability.StabilityDerivative > maxRiskAccel {
			maxRiskAccel = b.Stability.StabilityDerivative
		}
	}

	predictedP99 := 0.0
	if sumWeight > 0 {
		predictedP99 = sumWeightedLatency / sumWeight
	}

	harmonicStability := 0.0
	if harmonicCount > 0 {
		harmonicStability = float64(harmonicCount) / harmonicStabSum
	}

	// Trend-adjusted stability harmonic mean (stricter than point-in-time margin).
	trendStability := harmonicStability
	if trendStabCount > 0 {
		ts := float64(trendStabCount) / trendStabSum
		// Blend: 60% trend-adjusted, 40% point-in-time.
		trendStability = 0.6*ts + 0.4*harmonicStability
	}

	meanOscRisk := sumOscRisk / float64(len(bundles))
	cascadeProb := topo.CriticalPath.CascadeRisk
	latencyScore := math.Min(predictedP99/refLatencyMs, 1.0)
	instabilityScore := math.Max(1.0-trendStability, 0)

	// Risk acceleration bonus penalty: if risk is growing fast, amplify instability score.
	// Clamped at 0.2 additional contribution.
	accelPenalty := math.Min(maxRiskAccel*2.0, 0.2)
	instabilityScore = math.Min(instabilityScore+accelPenalty, 1.0)

	composite := wLatency*latencyScore + wCascade*cascadeProb + wInstability*instabilityScore + wOscillation*meanOscRisk

	// TrajectoryScore: arrival-rate-weighted mean risk-latency cost of the
	// current ρ trajectory over a 5-step look-ahead per service.
	// Uses the same cost function as MPCHorizonEval but without PID correction
	// (uncontrolled baseline) to measure the urgency of intervention.
	var trajSum, trajW float64
	for _, b := range bundles {
		w := math.Max(b.Queue.ArrivalRate, 0.01)
		rho := b.Queue.Utilisation
		trend := b.Queue.UtilisationTrend
		const trajSteps = 5
		const trajTickSec = 2.0 // nominal
		var stepCostSum float64
		for k := 0; k < trajSteps; k++ {
			simRho := math.Max(rho+trend*trajTickSec*float64(k+1), 0)
			waitCost := 0.0
			if simRho < 1.0 && b.Queue.ServiceRate > 0 {
				wq := simRho / ((1.0 - simRho) * b.Queue.ServiceRate)
				waitCost = math.Tanh(wq * 2.0)
			} else if simRho >= 1.0 {
				waitCost = 1.0
			}
			riskCost := 1.0 / (1.0 + math.Exp(-(simRho-0.85)/0.06))
			stepCostSum += 0.55*waitCost + 0.45*riskCost
		}
		trajSum += (stepCostSum / trajSteps) * w
		trajW += w
	}
	trajectoryScore := 0.0
	if trajW > 0 {
		trajectoryScore = trajSum / trajW
	}

	return ObjectiveScore{
		ComputedAt:                now,
		PredictedP99LatencyMs:     predictedP99,
		CascadeFailureProbability: cascadeProb,
		WeightedStabilityMargin:   harmonicStability,
		MaxCollapseRisk:           maxCollapseRisk,
		OscillationRisk:           meanOscRisk,
		CompositeScore:            math.Min(composite, 1.0),
		LatencyWeight:             wLatency,
		UtilisationWeight:         wInstability,
		RiskWeight:                wCascade,
		PredictiveHorizon:         5,
		ReferenceLatencyMs:        refLatencyMs,
		TrendStabilityMargin:      trendStability,
		RiskAcceleration:          maxRiskAccel,
		TrajectoryScore:           math.Min(trajectoryScore, 1.0),
	}
}
