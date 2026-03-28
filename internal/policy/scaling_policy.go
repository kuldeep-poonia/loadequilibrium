package policy

import "math"

type ScalingSignal struct {
	PredictedLoad     float64
	CurrentReplicas   int
	TargetLatency     float64
	ObservedLatency   float64

	MinReplicas int
	MaxReplicas int

	ScaleCooldownCost float64
	InstanceCost      float64
	SlaPenaltyWeight  float64
}

type ScalingDecision struct {
	DesiredReplicas int
	MinReplicas     int
	MaxReplicas     int
	Score           float64

	Reason     string
	Confidence float64
	Risk       float64
}

func RecommendScaling(s ScalingSignal) ScalingDecision {

	bestReplicas := s.CurrentReplicas
	bestScore := math.MaxFloat64

	// --- prediction-driven candidate expansion ---
	candidates := []int{
		s.CurrentReplicas - 1,
		s.CurrentReplicas,
		s.CurrentReplicas + 1,
		s.CurrentReplicas + 2,
	}

	// high predicted load → allow aggressive scale
	if s.PredictedLoad > 0 {
		loadFactor := s.PredictedLoad / float64(s.CurrentReplicas)

		if loadFactor > 1.3 {
			candidates = append(candidates, s.CurrentReplicas+3)
		}
		if loadFactor > 1.6 {
			candidates = append(candidates, s.CurrentReplicas+4)
		}
	}

	var reason string

	for _, r := range candidates {

		if r < s.MinReplicas || r > s.MaxReplicas {
			continue
		}

		// simplistic latency model (phase-1 acceptable)
		estimatedLatency := s.ObservedLatency *
			float64(s.CurrentReplicas) / float64(r)

		slaViolation := math.Max(0, estimatedLatency-s.TargetLatency)

		cost := s.SlaPenaltyWeight*slaViolation +
			s.InstanceCost*float64(r) +
			s.ScaleCooldownCost*math.Abs(float64(r-s.CurrentReplicas))

		if cost < bestScore {
			bestScore = cost
			bestReplicas = r

			if r > s.CurrentReplicas {
				reason = "Predicted load increase → proactive scaling"
			} else if r < s.CurrentReplicas {
				reason = "Low utilization → cost optimization"
			} else {
				reason = "Stable operating zone"
			}
		}
	}

	// simple confidence heuristic
	confidence := 1.0 / (1.0 + bestScore)

	// risk proxy (distance from safety band)
	risk := math.Abs(float64(bestReplicas-s.CurrentReplicas)) /
		float64(s.MaxReplicas)

	return ScalingDecision{
		DesiredReplicas: bestReplicas,
		MinReplicas:     s.MinReplicas,
		MaxReplicas:     s.MaxReplicas,
		Score:           bestScore,
		Reason:          reason,
		Confidence:      confidence,
		Risk:            risk,
	}
}