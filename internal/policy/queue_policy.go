package policy

import "math"

type QueueSignal struct {
	CurrentQueueDepth float64

	PredictedArrival float64
	ServiceCapacity  float64

	ObservedLatency float64
	TargetLatency   float64

	BaseSystemRisk float64

	CurrentQueueLimit float64
	MinQueueLimit     float64
	MaxQueueLimit     float64

	MaxStep float64 // absolute adjustment step
}

type QueueDecision struct {
	QueueLimit float64
	DrainPriority float64

	Reason     string
	Confidence float64
	Risk       float64
}

func RecommendQueuePolicy(s QueueSignal) QueueDecision {

	// ---- safety guards
	if s.MinQueueLimit < 1 {
		s.MinQueueLimit = 1
	}
	if s.MaxStep <= 0 {
		s.MaxStep = 10
	}

	utilisation := 0.0
	if s.ServiceCapacity > 0 {
		utilisation = s.PredictedArrival / s.ServiceCapacity
	}

	type candidate struct {
		limit float64
		drain float64
	}

	candidates := []candidate{
		{s.CurrentQueueLimit - s.MaxStep, 1.3},
		{s.CurrentQueueLimit, 1.0},
		{s.CurrentQueueLimit + s.MaxStep, 0.8},
	}

	// congestion anticipation option
	if utilisation > 1.1 {
		candidates = append(candidates,
			candidate{s.CurrentQueueLimit - 2*s.MaxStep, 1.6})
	}

	bestScore := math.MaxFloat64
	secondScore := math.MaxFloat64

	bestLimit := s.CurrentQueueLimit
	bestDrain := 1.0
	var reason string

	for _, c := range candidates {

		if c.limit < s.MinQueueLimit || c.limit > s.MaxQueueLimit {
			continue
		}

		// effective service impact of drain policy
		effectiveService := s.ServiceCapacity * c.drain
		util := 0.0
		if effectiveService > 0 {
			util = s.PredictedArrival / effectiveService
		}

		// saturated backlog energy
		backlogRatio := s.CurrentQueueDepth / c.limit
		backlogEnergy := math.Min(3.0, backlogRatio*util)

		// nonlinear latency stress
		latRatio := s.ObservedLatency / s.TargetLatency
		latencyStress := math.Max(0, math.Pow(latRatio, 2)-1)

		actionRisk :=
			1 - math.Exp(-(s.BaseSystemRisk +
				0.6*backlogEnergy +
				0.4*latencyStress))

		score :=
			actionRisk +
				0.3*backlogEnergy +
				0.2*(1.0/c.drain)

		if score < bestScore {
			secondScore = bestScore
			bestScore = score

			bestLimit = c.limit
			bestDrain = c.drain

			if c.limit < s.CurrentQueueLimit {
				reason = "Predicted congestion → queue tightening"
			} else if c.limit > s.CurrentQueueLimit {
				reason = "Burst tolerance → queue relaxation"
			} else {
				reason = "Queue stability zone"
			}

		} else if score < secondScore {
			secondScore = score
		}
	}

	margin := secondScore - bestScore
	confidence := 1 - math.Exp(-margin)

	finalRisk :=
		1 - math.Exp(-(s.BaseSystemRisk +
			s.CurrentQueueDepth/bestLimit))

	return QueueDecision{
		QueueLimit:    bestLimit,
		DrainPriority: bestDrain,
		Reason:        reason,
		Confidence:    confidence,
		Risk:          finalRisk,
	}
}