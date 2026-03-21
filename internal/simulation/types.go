package simulation

// EventKind classifies discrete simulation events.
type EventKind int

const (
	EventArrival   EventKind = iota
	EventDeparture
	EventShock
	EventRecovery
)

// Event is a single discrete-event simulation event.
type Event struct {
	Time              float64
	Kind              EventKind
	ServiceID         string
	ServiceDurationMs float64
	ArrivalTime       float64
}

// ServiceSimState is mutable per-service state during a simulation run.
type ServiceSimState struct {
	ServiceID    string
	ArrivalRate  float64
	BaseRate     float64 // original rate before shock
	ServiceRate  float64
	Concurrency  int
	Utilisation  float64
	QueueLen     int
	InService    int
	MaxQueueLen  int
	TotalArrived int64
	TotalServed  int64
	TotalDropped int64
	SumWaitMs    float64
	Shocked      bool
	// Recovery tracking
	ShockPeakRate       float64
	RecoveryStartMs     float64
	RecoveryConvergedAt float64
	// Queue distribution tracking (for variance computation).
	QueueLenSamples    int64
	QueueLenSum        float64
	QueueLenSumSq      float64
	SaturationSamples  int64
	CollapseCount      int
	// SLA tracking: count of requests with wait > SLA threshold.
	SLAExceedances int64
	SLAChecked     int64 // total served requests with SLA evaluated
	SLAThresholdMs float64 // per-service SLA latency threshold in ms (0 = disabled)
}

// ServiceOutcome is a per-service simulation result.
type ServiceOutcome struct {
	ServiceID       string
	FinalQueueLen   int
	PeakQueueLen    int
	ThroughputRatio float64
	MeanWaitMs      float64
	Saturated       bool
	PeakUtilisation float64 // max utilisation observed during simulation
	RecoveryTimeMs  float64 // simulated time from shock peak back to base-rate steady-state
	// Queue state distribution at end of horizon.
	// These capture the spread of queue lengths observed during the run.
	QueueLenMean     float64 // time-averaged queue length over the run
	QueueLenVariance float64 // variance of queue length samples (proxy for stability)
}

// SimulationMeta contains run metadata for budget accounting.
type SimulationMeta struct {
	WallTimeMs    float64 // real wall-clock time consumed
	BudgetUsedPct float64 // 0..100 percentage of budget consumed
	EventsPerMs   float64 // simulation throughput
}

// SimulationResult is the full output of one simulation run.
type SimulationResult struct {
	HorizonMs        float64
	Services         map[string]ServiceOutcome
	SystemStable     bool
	CollapseDetected bool
	CascadeTriggered bool
	EventsProcessed  int
	Meta             SimulationMeta
	RecoveryConvergenceMs float64
	DegradedServiceCount  int
	CascadeFailureProbability  map[string]float64
	QueueDistributionAtHorizon map[string]QueueDistributionSnapshot
	// SLAViolationProbability: per-service fraction of requests that exceeded
	// the SLA latency threshold during the simulation run.
	// P(SLA violation) = requests_exceeding_threshold / total_served.
	// When SLALatencyThresholdMs == 0, this map is empty.
	SLAViolationProbability map[string]float64
}

// QueueDistributionSnapshot captures queue state at a point in virtual time.
type QueueDistributionSnapshot struct {
	MeanQueueLen     float64 // time-averaged queue length
	VarQueueLen      float64 // variance of queue length samples
	P95QueueLen      float64 // approximate 95th percentile (from mean + 1.645×σ)
	SaturationFrac   float64 // fraction of departures where queue was > 50% max depth
	UtilisationAtEnd float64 // ρ at end of horizon (current InService / Concurrency)
}
