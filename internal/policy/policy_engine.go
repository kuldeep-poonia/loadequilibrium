package policy

import "math"

type EngineState struct {
	LastGlobalRisk float64
	LastReplicas   int
	OscillationScore float64
}

type EngineInput struct {

	Scaling ScalingSignal
	Retry   RetrySignal
	Queue   QueueSignal
	Cache   CacheSignal

	CostWeights CostWeights
}

type EngineDecision struct {
	Scaling ScalingDecision
	Retry   RetryDecision
	Queue   QueueDecision
	Cache   CacheDecision

	GlobalRisk float64
	GlobalCost float64

	Confidence   float64
	SystemReason string
}

func EvaluatePolicies(
	in EngineInput,
	state *EngineState,
) EngineDecision {

	// ---- evaluate individual policies
	scaling := RecommendScaling(in.Scaling)
	retry   := RecommendRetryPolicy(in.Retry)
	queue   := RecommendQueuePolicy(in.Queue)
	cache   := RecommendCachePolicy(in.Cache)

	// ---- light cross-policy coordination damping
	// example: scaling up + queue tightening → conflict
	if scaling.DesiredReplicas > in.Scaling.CurrentReplicas &&
		queue.QueueLimit < in.Queue.CurrentQueueLimit {

		queue.QueueLimit =
			(in.Queue.CurrentQueueLimit + queue.QueueLimit) / 2.0
	}

	// ---- adaptive risk fusion (context weighted)
	util := 0.0
	if in.Queue.ServiceCapacity > 0 {
		util = in.Queue.PredictedArrival / in.Queue.ServiceCapacity
	}

	scaleW := 0.25 + 0.2*math.Min(1.0, util)
	queueW := 0.25 + 0.2*math.Min(1.0, in.Queue.CurrentQueueDepth/in.Queue.CurrentQueueLimit)
	retryW := 0.25
	cacheW := 1.0 - (scaleW + queueW + retryW)

	globalRisk :=
		1 - math.Exp(-(
			scaleW*scaling.Risk +
				queueW*queue.Risk +
				retryW*retry.Risk +
				cacheW*cache.Risk))

	// ---- unified economic cost estimation (Phase-1 coarse)
	latencyProxy :=
		in.Queue.ObservedLatency

	cost := EvaluatePolicyCost(
		CostInput{
			EstimatedLatency: latencyProxy,
			TargetLatency:    in.Queue.TargetLatency,
			DesiredReplicas:  scaling.DesiredReplicas,
			CurrentReplicas:  in.Scaling.CurrentReplicas,
			PricePerReplica:  in.Scaling.InstanceCost,
			BaseSystemRisk:   globalRisk,
			PredictedLoad:    in.Scaling.PredictedLoad,
		},
		in.CostWeights,
	).TotalCost

	// ---- regime memory & hysteresis
	var reason string
	confidence :=
		(scaling.Confidence +
			retry.Confidence +
			queue.Confidence +
			cache.Confidence) / 4.0

	if state != nil {

		riskDelta := globalRisk - state.LastGlobalRisk

		if math.Abs(riskDelta) > 0.12 {
			state.OscillationScore += 1
		} else {
			state.OscillationScore *= 0.8
		}

		if state.OscillationScore > 3 {
			reason = "Detected control oscillation → damping regime"
		} else if globalRisk > 0.7 {
			reason = "High instability probability → defensive control"
		} else if globalRisk > 0.4 {
			reason = "Moderate stress → adaptive optimisation"
		} else {
			reason = "Stable backend regime"
		}

		state.LastGlobalRisk = globalRisk
		state.LastReplicas   = scaling.DesiredReplicas

	} else {

		if globalRisk > 0.7 {
			reason = "High instability probability"
		} else if globalRisk > 0.4 {
			reason = "Moderate load stress"
		} else {
			reason = "Stable backend regime"
		}
	}

	return EngineDecision{
		Scaling:     scaling,
		Retry:       retry,
		Queue:       queue,
		Cache:       cache,
		GlobalRisk:  globalRisk,
		GlobalCost:  cost,
		Confidence:  confidence,
		SystemReason: reason,
	}
}