package control

import "math"

type CostParams struct {
	InfraUnitCost float64
	SLAWeight     float64
	RiskWeight    float64
	BacklogWeight float64

	UtilTarget float64
	UtilBand   float64

	SmoothReplica float64
	SmoothRetry   float64
	SmoothQueue   float64
	SmoothCache   float64

	CacheCostWeight float64
}

func EvaluateHorizonCost(
	initial SystemState,
	b Bundle,
	traj Trajectory,
	cfg SimConfig,
	p CostParams,
	mem *RegimeMemory,
) float64 {

	horizonTime :=
		float64(cfg.HorizonSteps) * cfg.Dt

	// =========================
	// INFRA COST (normalized, stable)
	// =========================
	infraCost :=
    p.InfraUnitCost *
    math.Sqrt(float64(b.Replicas))

	// =========================
	// SLA COST
	// =========================
	slaCost :=
		p.SLAWeight *
			traj.SLAIntegral

	// =========================
	// RISK COST (adaptive + backlog aware)
	// =========================
	riskExponent := 1.3
	if mem != nil {
		riskExponent =
			1.2 + 0.8*mem.RiskEWMA
	}

	riskSignal :=
		traj.CollapseRisk +
			0.3*math.Min(1, traj.FinalBacklog/(b.QueueLimit+1))

	riskCost :=
		p.RiskWeight *
			math.Pow(riskSignal, riskExponent)

	// =========================
	// BACKLOG COST (aggressive)
	// =========================
	queuePressure :=
		traj.FinalBacklog /
			(b.QueueLimit + 1)

	backlogCost :=
		p.BacklogWeight *
			math.Pow(queuePressure, 2.5)

	if traj.FinalBacklog > float64(b.QueueLimit)*1.5 {
		backlogCost *= 2.5
	}

	if traj.PeakLatency > initial.SLATarget {
		backlogCost *= 1.8
	}

	// =========================
	// UTILIZATION COST (asymmetric)
	// =========================
	avgUtil :=
		initial.PredictedArrival /
			math.Max(
				float64(b.Replicas)*initial.ServiceRate,
				0.001,
			)

	utilCost := 0.0

	// under-utilization (mild penalty)
	if avgUtil < p.UtilTarget {
		utilCost += 0.5 * (p.UtilTarget - avgUtil)
	}

	// overload (quadratic penalty)
	if avgUtil > p.UtilTarget {
		overload := avgUtil - p.UtilTarget
		utilCost += 2.0 * overload * overload
	}

	// heavy overload (strong push to scale up)
	if avgUtil > p.UtilTarget+0.3 {
		overload := avgUtil - (p.UtilTarget + 0.3)
		utilCost += 5.0 * overload * overload
	}

	// =========================
	// ACTUATOR SMOOTHNESS
	// =========================
	repRange :=
		math.Max(
			float64(initial.MaxReplicas-initial.MinReplicas),
			1,
		)

	repSmooth :=
		p.SmoothReplica *
			math.Abs(
				float64(b.Replicas-initial.Replicas),
			) /
			repRange

	retryRange :=
		math.Max(
			float64(initial.MaxRetry-initial.MinRetry),
			1,
		)

	retrySmooth :=
		p.SmoothRetry *
			math.Abs(
				float64(b.RetryLimit-initial.RetryLimit),
			) /
			retryRange

	queueSmooth :=
		p.SmoothQueue *
			math.Abs(
				b.QueueLimit-float64(initial.QueueLimit),
			) /
			math.Max(float64(initial.QueueLimit), 1)

	cacheSmooth :=
		p.SmoothCache *
			math.Abs(
				b.CacheAggression-initial.CacheAggression,
			)

	// =========================
	// CACHE COST
	// =========================
	cacheCost :=
		p.CacheCostWeight *
			math.Pow(b.CacheAggression, 1.3) *
			horizonTime

	// =========================
	// OSCILLATION PENALTY
	// =========================
	oscPenalty := 0.0
	if mem != nil {
		oscPenalty =
			0.4 * mem.OscillationEWMA
	}

	// =========================
	// TOTAL COST
	// =========================
	total :=
		infraCost +
			slaCost +
			riskCost +
			backlogCost +
			utilCost +
			repSmooth +
			retrySmooth +
			queueSmooth +
			cacheSmooth +
			cacheCost +
			oscPenalty

	return total
}