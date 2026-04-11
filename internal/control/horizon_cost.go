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

	//  infra cost 
	infraCost :=
		float64(b.Replicas) *
			p.InfraUnitCost *
			horizonTime

	// SLA area 
	slaCost :=
		p.SLAWeight *
			traj.SLAIntegral

	// adaptive risk aversion 
	riskExponent := 1.3
	if mem != nil {
		riskExponent =
			1.2 + 0.8*mem.RiskEWMA
	}

	riskCost :=
		p.RiskWeight *
			math.Pow(traj.CollapseRisk, riskExponent)

	// ---------- backlog stress ----------
	queuePressure :=
		traj.FinalBacklog /
			(b.QueueLimit + 1)

	backlogCost :=
		p.BacklogWeight *
			math.Pow(queuePressure, 1.4)

	// ---------- trajectory utilisation proxy ----------
	avgUtil :=
		initial.PredictedArrival /
			math.Max(
				float64(b.Replicas)*initial.ServiceRate,
				0.001,
			)

	utilDeviation :=
		math.Abs(avgUtil - p.UtilTarget)

	utilCost := 0.0

	if utilDeviation > p.UtilBand {

		excess :=
			utilDeviation - p.UtilBand

		utilCost =
			excess * excess * 0.9
	}

	// ---------- actuator smoothness ----------
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

	// ---------- cache operating economics ----------
	cacheCost :=
		p.CacheCostWeight *
			math.Pow(b.CacheAggression, 1.3) *
			horizonTime

	// ---------- oscillation penalty hook ----------
	oscPenalty := 0.0
	if mem != nil {
		oscPenalty =
			0.4 * mem.OscillationEWMA
	}

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
