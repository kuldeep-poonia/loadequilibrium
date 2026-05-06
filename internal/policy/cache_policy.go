package policy

import "math"

type CacheSignal struct {
	CurrentHitRate float64
	TargetHitRate  float64

	PredictedArrival float64
	ServiceCapacity  float64

	ObservedLatency float64
	TargetLatency   float64

	CacheableRatio float64 // fraction of traffic cacheable

	BaseMemoryPressure float64 // base working-set pressure
	BaseSystemRisk     float64

	CurrentCacheAggression float64
	MinAggression          float64
	MaxAggression          float64

	BaseStep float64
}

type CacheDecision struct {
	CacheAggression float64

	Reason     string
	Confidence float64
	Risk       float64
}

func RecommendCachePolicy(s CacheSignal) CacheDecision {

	if s.BaseStep <= 0 {
		s.BaseStep = 0.05
	}

	util := 0.0
	if s.ServiceCapacity > 0 {
		util = s.PredictedArrival / s.ServiceCapacity
	}

	type candidate struct {
		aggr float64
	}

	step := s.BaseStep * (1 + util*0.5)

	candidates := []candidate{
		{s.CurrentCacheAggression - step},
		{s.CurrentCacheAggression},
		{s.CurrentCacheAggression + step},
	}

	if util > 1.15 {
		candidates = append(candidates,
			candidate{s.CurrentCacheAggression + 2*step})
	}

	bestScore := math.MaxFloat64
	secondScore := math.MaxFloat64
	bestAgg := s.CurrentCacheAggression

	var reason string
	var bestFutureHit float64

	for _, c := range candidates {

		if c.aggr < s.MinAggression || c.aggr > s.MaxAggression {
			continue
		}

		// diminishing returns hit model
		k := 2.5
		hitGain :=
			(1 - math.Exp(-k*c.aggr)) *
				(1 - s.CurrentHitRate)

		futureHit := math.Min(0.99, s.CurrentHitRate+hitGain)

		// miss pressure
		missPressure :=
			math.Max(0, s.TargetHitRate-futureHit)

		// cacheable load relief
		loadRelief :=
			hitGain * util * s.CacheableRatio

		// latency stress channel
		latRatio :=
			s.ObservedLatency / s.TargetLatency

		latStress :=
			math.Max(0, math.Pow(latRatio, 2)-1)

		// dynamic memory pressure
		aggrDelta :=
			math.Abs(c.aggr - s.CurrentCacheAggression)

		memPressure :=
			s.BaseMemoryPressure +
				0.6*aggrDelta

		actionRisk :=
			1 - math.Exp(-(s.BaseSystemRisk +
				0.5*missPressure +
				0.4*latStress +
				0.6*memPressure -
				0.5*loadRelief))

		score :=
			actionRisk +
				0.3*missPressure +
				0.25*memPressure +
				0.2*latStress

		if score < bestScore {

			secondScore = bestScore
			bestScore = score
			bestAgg = c.aggr
			bestFutureHit = futureHit

			if c.aggr > s.CurrentCacheAggression {
				reason = "Predicted load / latency risk → strengthen caching"
			} else if c.aggr < s.CurrentCacheAggression {
				reason = "Memory pressure → relax caching"
			} else {
				reason = "Cache steady regime"
			}

		} else if score < secondScore {
			secondScore = score
		}
	}

	margin := secondScore - bestScore
	confidence := 1 - math.Exp(-margin)

	finalRisk :=
		1 - math.Exp(-(s.BaseSystemRisk +
			(1 - bestFutureHit)))

	return CacheDecision{
		CacheAggression: bestAgg,
		Reason:          reason,
		Confidence:      confidence,
		Risk:            finalRisk,
	}
}