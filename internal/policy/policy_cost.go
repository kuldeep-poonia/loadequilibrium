package policy

import "math"

// CostWeights define business priorities (dimensionless).
type CostWeights struct {
	SlaViolation float64
	InfraCost    float64
	ChangeCost   float64
	RiskCost     float64
	FutureCost   float64
}

// CostInput describes candidate action evaluation.
type CostInput struct {
	EstimatedLatency float64
	TargetLatency    float64

	DesiredReplicas int
	CurrentReplicas int

	PricePerReplica float64

	BaseSystemRisk float64
	PredictedLoad  float64
}

// CostBreakdown provides explainable optimisation output.
type CostBreakdown struct {
	TotalCost       float64
	SlaPenaltyCost  float64
	InfraCost       float64
	ReconfigCost    float64
	RiskPenaltyCost float64
	FuturePenalty   float64
}

// EvaluatePolicyCost computes upgraded objective.
func EvaluatePolicyCost(in CostInput, w CostWeights) CostBreakdown {

	// --- nonlinear SLA penalty (soft quadratic tail sensitivity)
	violation := math.Max(0, in.EstimatedLatency-in.TargetLatency)
	slaCost := w.SlaViolation * violation * violation

	// --- real infra cost model
	infraCost := w.InfraCost *
		float64(in.DesiredReplicas) *
		in.PricePerReplica

	// --- scaling inertia
	reconfigCost := w.ChangeCost *
		math.Abs(float64(in.DesiredReplicas-in.CurrentReplicas))

	// --- action dependent risk proxy
	capacityBuffer := float64(in.DesiredReplicas-in.CurrentReplicas) /
		float64(in.CurrentReplicas)

	actionRisk := in.BaseSystemRisk - 0.3*capacityBuffer
	actionRisk = math.Max(0, actionRisk)

	riskCost := w.RiskCost * actionRisk

	// --- tiny horizon awareness (predictive stress)
	loadPressure := in.PredictedLoad / float64(in.DesiredReplicas)
	futurePenalty := w.FutureCost * math.Max(0, loadPressure-1.0)

	total := slaCost + infraCost + reconfigCost + riskCost + futurePenalty

	return CostBreakdown{
		TotalCost:       total,
		SlaPenaltyCost:  slaCost,
		InfraCost:       infraCost,
		ReconfigCost:    reconfigCost,
		RiskPenaltyCost: riskCost,
		FuturePenalty:   futurePenalty,
	}
}