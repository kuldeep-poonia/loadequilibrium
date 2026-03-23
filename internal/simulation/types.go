package simulation

import (
	"github.com/loadequilibrium/loadequilibrium/internal/physics"
)

// EventKind classifies discrete simulation events.
// ... (rest of file)
type EventKind int

const (
	EventArrival EventKind = iota
	EventDeparture
	EventShock
	EventRecovery
	EventTick
)

// Event is a single discrete-event simulation event.
type Event struct {
	Time      float64
	Kind      EventKind
	ServiceID string

	// For Arrival/Departure
	ServiceDurationMs float64
	ArrivalTime       float64
}

// ServiceSimState is mutable per-service state during a simulation run.
type ServiceSimState struct {
	ServiceID   string
	ArrivalRate float64
	BaseRate    float64 // original rate before shock
	ServiceRate float64
	Concurrency int
	Utilisation float64
	QueueLen    int
	InService   int
	MaxQueueLen int

	TotalArrived int64
	TotalServed  int64
	TotalDropped int64
	SumWaitMs    float64

	Shocked bool

	// Recovery tracking
	ShockPeakRate       float64
	RecoveryStartMs     float64
	RecoveryConvergedAt float64

	// Queue distribution tracking
	QueueLenSamples   int64
	QueueLenSum       float64
	QueueLenSumSq     float64
	SaturationSamples int64
	CollapseCount     int

	// SLA tracking
	SLAExceedances int64
	SLAChecked     int64
	SLAThresholdMs float64

	// =============================
	// Physics Engine (Fluid Plant)
	// =============================

	Plant *physics.FluidPlant

	// Physics States for Mapping
	QueueMass float64
	Hazard    float64
	Reservoir float64

	// PhysicsClock accumulates virtual time between physics updates.
	// Used to decimate stiff fluid integration from DES tick frequency.
	PhysicsClock float64

	// LastPhysicsTime is optional future extension:
	// allows exact Δt computation if we later move to adaptive stepping.
	LastPhysicsTime float64

	// PhysicsUpdateCount tracks the number of Step() calls for log throttling.
	PhysicsUpdateCount int64
}

// ServiceOutcome is a per-service simulation result.
type ServiceOutcome struct {
	ServiceID       string
	FinalQueueLen   int
	PeakQueueLen    int
	ThroughputRatio float64
	MeanWaitMs      float64
	Saturated       bool
	PeakUtilisation float64
	RecoveryTimeMs  float64

	QueueLenMean     float64
	QueueLenVariance float64

	// Physics Engine States
	FinalHazard    float64
	FinalReservoir float64
}

// SimulationMeta contains run metadata.
type SimulationMeta struct {
	WallTimeMs    float64
	BudgetUsedPct float64
	EventsPerMs   float64
}

// SimulationResult is the full output of one simulation run.
type SimulationResult struct {
	Services         map[string]ServiceOutcome
	HorizonMs        float64
	CascadeTriggered bool
	EventsProcessed  int
	Meta             SimulationMeta
	SystemStable     bool
	CollapseDetected bool

	RecoveryConvergenceMs float64
	DegradedServiceCount  int

	CascadeFailureProbability  map[string]float64
	QueueDistributionAtHorizon map[string]QueueDistributionSnapshot

	SLAViolationProbability map[string]float64
}

// QueueDistributionSnapshot captures queue state at a point in virtual time.
type QueueDistributionSnapshot struct {
	MeanQueueLen     float64
	VarQueueLen      float64
	P95QueueLen      float64
	SaturationFrac   float64
	UtilisationAtEnd float64
}