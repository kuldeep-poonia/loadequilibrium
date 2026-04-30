package autopilot

import "math"



type ControlInput struct {
	CapacityTarget float64
	RetryFactor    float64
	CacheRelief    float64
}

type Controller struct {
	Dt float64

	Kb float64
	Kp float64
	Kg float64

	MPCWeight float64

	MaxCapacity float64

	PrevCapacity float64

	PrevCache float64
}

func (c *Controller) Compute(
	curr PlantState,
	prev PlantState,
	mpc ControlInput,
) ControlInput {

	// 
	// 1. SIGNALS
	// 

	backlog := curr.Backlog
	prevBacklog := prev.Backlog
	growth := backlog - prevBacklog

	service := curr.ServiceRate * curr.CapacityActive
	pressure := curr.ArrivalMean - service

	trackingError := (curr.ArrivalMean - service) / (curr.ArrivalMean + 1e-6)
	p := pressure / (service + 1e-6)
	b := backlog / (backlog + 100.0)
	g := growth / (math.Abs(prevBacklog) + 10.0)

	vol := math.Abs(curr.ArrivalP95 - curr.ArrivalMean)

	// 
	// 2. LYAPUNOV DRIFT
	// 

	drift := 2 * backlog * pressure / (service + 1e-6)
	driftNorm := drift / (backlog + 1.0)

	pClamped := math.Tanh(p)
	gClamped := math.Tanh(g)
	dClamped := math.Tanh(driftNorm)

	lyapunovSignal :=
		c.Kb*b +
			c.Kp*pClamped +
			c.Kg*gClamped +
			0.1*dClamped +
			0.5*trackingError

	// 
	// 3. STEADY STATE (CRITICAL FIX)
	// 

	steadyCap :=
		1.4 * curr.ArrivalMean / (curr.ServiceRate + 1e-6)

	// 
	// 4. SAFE REQUIRED REGION
	// 

	required :=
		steadyCap * (1 + math.Min(0.5, vol/10.0))

	maxSafe :=
		required * (2.2 + 0.8*math.Min(1.0, vol/10.0))

	// 
	// 5. LYAPUNOV CAPACITY
	// 

	lyapunovCap :=
		required * (1 + lyapunovSignal)

	// 
	// 6. INSTABILITY + MPC FUSION
	// 

	instability :=
		math.Sqrt(
			g*g +
				math.Max(0, p)*math.Max(0, p) +
				b*b +
				(vol/10.0)*(vol/10.0),
		)

	w := c.MPCWeight * (0.5 + 0.5*math.Min(1.0, instability))

	baseTarget :=
		(1-w)*lyapunovCap +
			w*mpc.CapacityTarget

	target := math.Max(
		required,
		math.Min(maxSafe, baseTarget),
	)

	
	err := math.Abs(target - curr.CapacityActive)

	if backlog > 0.6*curr.ArrivalMean*10 {
		// CRITICAL MODE: bypass smoothing
		return ControlInput{
			CapacityTarget: math.Min(maxSafe, required*1.5),
			RetryFactor:    0.8,
			CacheRelief:    0.9,
		}
	}

	// smooth transition (no hard jump)
	beta := 0.3 + 0.7*(err/(err+10))

	target = curr.CapacityActive + beta*(target-curr.CapacityActive)

	delta := target - curr.CapacityActive

	stress := backlog/(curr.ArrivalMean+1e-6)

maxRate := math.Max(
    5.0,
    (0.25+0.75*math.Min(1.0, stress))*curr.CapacityActive,
)
	maxStep := maxRate * c.Dt

	// Apply maxStep bound
	if delta > maxStep {
		delta = maxStep
	}
	if delta < -maxStep {
		delta = -maxStep
	}
	finalCap :=
		curr.CapacityActive + delta

		
	smoothPressure := math.Max(0, p)
	queueStress := backlog / 50.0

	retry := 0.4 * (1.0 - math.Exp(-(smoothPressure + queueStress)))

	

	rawCache :=
		math.Min(1.0,
			0.8*(backlog/(backlog+100.0))+
				0.4*(vol/10.0))

	alpha := 0.2
	cache := c.PrevCache + alpha*(rawCache-c.PrevCache)
	c.PrevCache = cache

	

	c.PrevCapacity = finalCap

	return ControlInput{
		CapacityTarget: finalCap,
		RetryFactor:    retry,
		CacheRelief:    cache,
	}
}
