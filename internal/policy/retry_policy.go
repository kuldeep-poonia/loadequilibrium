package policy

import (
	"math"
)

type RetrySignal struct {
	ObservedErrorRate float64
	TimeoutRate       float64

	QueueDepth float64

	PredictedArrival float64
	ServiceCapacity  float64

	BaseSystemRisk float64

	CurrentRetryLimit int
	MinRetryLimit     int
	MaxRetryLimit     int

	MinBackoff float64
	MaxBackoff float64
}

type RetryDecision struct {
	RetryLimit        int
	BackoffMultiplier float64

	Reason     string
	Confidence float64
	Risk       float64
}

func RecommendRetryPolicy(s RetrySignal) RetryDecision {

	// --- safety floor philosophy
	if s.MinRetryLimit < 1 {
		s.MinRetryLimit = 1
	}

	type candidate struct {
		limit   int
		backoff float64
	}

	candidates := []candidate{
		{s.CurrentRetryLimit - 1, 1.3},
		{s.CurrentRetryLimit, 1.6},
		{s.CurrentRetryLimit + 1, 2.0},
	}

	// utilisation-based predictive pressure
	utilisation := 0.0
	if s.ServiceCapacity > 0 {
		utilisation = s.PredictedArrival / s.ServiceCapacity
	}

	// congestion → allow aggressive throttling option
	if utilisation > 1.15 {
		candidates = append(candidates,
			candidate{s.CurrentRetryLimit - 2, 2.5})
	}

	bestScore := math.MaxFloat64
	secondScore := math.MaxFloat64

	bestLimit := s.CurrentRetryLimit
	bestBackoff := 1.6
	var reason string

	for _, c := range candidates {

		if c.limit < s.MinRetryLimit || c.limit > s.MaxRetryLimit {
			continue
		}

		if c.backoff < s.MinBackoff || c.backoff > s.MaxBackoff {
			continue
		}

		// retry load amplification model
		retryIntensity :=
			float64(c.limit) *
				(s.ObservedErrorRate + s.TimeoutRate)

		// queue delay risk channel
		delayStress := s.QueueDepth * retryIntensity

		// bounded nonlinear risk
		actionRisk :=
			1 - math.Exp(-(s.BaseSystemRisk + 0.4*delayStress))

		// objective score (joint limit + backoff effect)
		score :=
			actionRisk +
				0.6*retryIntensity +
				0.2*(1.0/c.backoff)

		if score < bestScore {
			secondScore = bestScore
			bestScore = score
			bestLimit = c.limit
			bestBackoff = c.backoff

			if c.limit < s.CurrentRetryLimit {
				reason = "Predicted saturation → retry suppression"
			} else if c.limit > s.CurrentRetryLimit {
				reason = "Transient failures → retry assist"
			} else {
				reason = "Retry equilibrium"
			}

		} else if score < secondScore {
			secondScore = score
		}
	}

	// decision margin based confidence
	margin := secondScore - bestScore
	confidence := 1.0 - math.Exp(-margin)

	finalRisk :=
		1 - math.Exp(-(s.BaseSystemRisk +
			float64(bestLimit)*(s.ObservedErrorRate+s.TimeoutRate)))

	return RetryDecision{
		RetryLimit:        bestLimit,
		BackoffMultiplier: bestBackoff,
		Reason:            reason,
		Confidence:        confidence,
		Risk:              finalRisk,
	}
}