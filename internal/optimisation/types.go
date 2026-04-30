package optimisation

import "time"

type ControlDirective struct {
	ServiceID         string
	ComputedAt        time.Time
	ScaleFactor       float64
	TargetUtilisation float64
	Error             float64
	PIDOutput         float64
	Active            bool
	StabilityMargin   float64
	// Enhanced control fields
	HysteresisState  string  // "active" | "suppressed" | "deadband"
	ActuationBound   float64 // max allowed single-step scale change this tick
	PredictiveTarget float64 // projected target utilisation N ticks ahead
	// MPC evaluation outputs
	MPCPredictedRho       float64 // predicted ρ at end of MPC horizon
	MPCOvershootRisk      bool    // true when control action may overshoot setpoint
	MPCUnderactuationRisk bool    // true when rho remains above setpoint after horizon
	// Cost gradient at current operating point: d(global_cost)/dρ.
	CostGradient float64
	// Trajectory risk-latency scores from multi-step MPC evaluation.
	// TrajectoryCostAvg: mean per-step cost over the horizon — high value means
	// the predicted control path passes through high-risk operating zones.
	TrajectoryCostAvg float64
	// MaxTrajectoryCost: worst single-step cost — flags transient risk peaks.
	MaxTrajectoryCost float64
	// Trajectory planner outputs from bounded 7-candidate objective-surface search.
	PlannerScaleFactor float64 // optimal scale from surface search
	PlannerConvergent  bool    // true when planner selection converges to setpoint
	PlannerConvex      bool    // true when objective surface is locally convex
	// PlannerProbabilisticScore: risk-adjusted trajectory cost under arrival uncertainty.
	// Accounts for burst amplification — lower is safer under chaotic arrivals.
	PlannerProbabilisticScore float64
}

// ControlCandidate is an optimizer-produced advisory, not an executable command.
//
// The runtime control authority may use these candidates as search-space hints,
// but this type deliberately has no actuation fields beyond the candidate value
// and its evaluation metadata. A candidate becomes executable only after the
// control authority selects and constrains it into a ControlDirective.
type ControlCandidate struct {
	ServiceID    string
	ScaleFactor  float64
	Score        float64
	Feasible     bool
	Convergent   bool
	PredictedRho float64
	RiskScore    float64
	Uncertainty  float64
	Source       string
	ComputedAt   time.Time
}

type ObjectiveScore struct {
	ComputedAt                time.Time
	PredictedP99LatencyMs     float64
	CascadeFailureProbability float64
	WeightedStabilityMargin   float64
	MaxCollapseRisk           float64
	OscillationRisk           float64
	CompositeScore            float64
	// Trade-off transparency
	LatencyWeight     float64
	UtilisationWeight float64
	RiskWeight        float64
	PredictiveHorizon int
	// Dynamic objective fields
	ReferenceLatencyMs   float64 // dynamically computed latency reference for this tick
	TrendStabilityMargin float64 // trend-adjusted harmonic stability (pessimistic)
	RiskAcceleration     float64 // max d(CollapseRisk)/dt across services
	// TrajectoryScore: arrival-rate-weighted mean of per-service TrajectoryCostAvg.
	// Measures the risk-latency cost of the current control trajectory across the
	// prediction horizon. High value means the system is being steered through
	// dangerous operating zones — intervention quality is poor.
	TrajectoryScore float64
}
