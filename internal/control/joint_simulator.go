package control

import "math"

type Trajectory struct {
	FinalBacklog float64
	PeakLatency  float64
	CollapseRisk float64
	SLAIntegral  float64
}

func SimulateBundle(
	initial SystemState,
	b Bundle,
	cfg SimConfig,
) Trajectory {

	backlog := initial.QueueDepth
	latency := math.Max(initial.Latency, cfg.BaseLatency)
	replicas := float64(initial.Replicas)

	retryIntensity := float64(initial.RetryLimit)

	warmState := 0.0
	survival := 1.0

	peakLatency := latency
	slaArea := 0.0

	for t := 0; t < cfg.HorizonSteps; t++ {

		// ---------- bounded disturbance ----------
		disturb :=
			cfg.DisturbanceStd *
				math.Sin(float64(t)*cfg.DisturbanceFreq)

		if disturb < -0.7 {
			disturb = -0.7
		}
		if disturb > 1.2 {
			disturb = 1.2
		}

		mult := 1 + disturb

if mult < 0.2 {
    mult = 0.2
}
if mult > 3 {
    mult = 3
}

baseArrival := initial.PredictedArrival * mult

		// ---------- retry storm feedback ----------
		retrySat :=
			math.Tanh(retryIntensity * 0.4)

		arrival :=
			baseArrival *
				(1 + cfg.RetryFeedbackGain*retrySat)

		// ---------- cache relief (saturating) ----------
		cacheDelta :=
			b.CacheAggression - initial.CacheAggression

		arrival *=
			math.Exp(-0.9 * cacheDelta)

		// ---------- admission control ----------
		drop :=
			1 /
				(1 +
					math.Exp(
						-0.05*(backlog-b.QueueLimit),
					))

		effectiveArrival :=
			arrival * (1 - drop)

		// ---------- logistic warmup ----------
		targetRep := float64(b.Replicas)

		rampError := targetRep - replicas

		warmState +=
			cfg.Dt *
				cfg.WarmupRate *
				rampError *
				(1 - math.Abs(warmState))

		if warmState > 1 {
			warmState = 1
		}
		if warmState < -1 {
			warmState = -1
		}

		replicas +=
			cfg.Dt *
				warmState *
				0.6

		if replicas < 1 {
			replicas = 1
		}

		// ---------- infra efficiency (soft decay) ----------
		eff :=
			1 /
				(1 +
					cfg.EfficiencyDecay*
						math.Log(1+replicas))

		service :=
			replicas *
				initial.ServiceRate *
				eff

		util :=
			effectiveArrival /
				math.Max(service, 0.01)

		// ---------- backlog dynamics ----------
		backlog +=
			cfg.Dt *
				(effectiveArrival - service)

		if backlog < 0 {
			backlog = 0
		}

		// ---------- bounded queue delay ----------
		queueDelay :=
			backlog /
				math.Max(service, 1)

		if queueDelay > cfg.MaxQueueDelay {
			queueDelay = cfg.MaxQueueDelay
		}

		// ---------- nonlinear latency state ----------
		utilPressure :=
			util / (1 + util)

		latency +=
			cfg.Dt *
				(0.7*utilPressure +
					0.5*queueDelay -
					0.9*(latency-cfg.BaseLatency))

		if latency < cfg.BaseLatency {
			latency = cfg.BaseLatency
		}

		if latency > peakLatency {
			peakLatency = latency
		}

		// ---------- retry storm memory with decay ----------
		retryIntensity +=
			cfg.Dt *
				(0.4*(latency/initial.SLATarget) -
					0.5*retryIntensity +
					0.3*drop)

		if retryIntensity < 0 {
			retryIntensity = 0
		}
		if retryIntensity > 10 {
			retryIntensity = 10
		}

		// ---------- bounded hazard ----------
		utilHaz :=
			cfg.HazardUtilGain *
				(util / (1 + util))

		backlogHaz :=
			cfg.HazardBacklogGain *
				math.Tanh(backlog/(b.QueueLimit+1))

		retryHaz :=
			cfg.HazardRetryGain *
				math.Tanh(retryIntensity * 0.5)

		hazard :=
			utilHaz + backlogHaz + retryHaz

		survival *=
			math.Exp(-hazard * cfg.Dt)

		// ---------- SLA integral ----------
		if latency > initial.SLATarget {
			slaArea +=
				(latency-initial.SLATarget) *
					cfg.Dt
		}
	}

	return Trajectory{
		FinalBacklog: backlog,
		PeakLatency:  peakLatency,
		CollapseRisk: 1 - survival,
		SLAIntegral:  slaArea,
	}
}