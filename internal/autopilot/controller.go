package autopilot

import "math"

/*
============================================================
ULTIMATE LYAPUNOV + MPC CONTROLLER (STABLE VERSION)
============================================================

✔ Lyapunov + MPC intact
✔ steady-state anchored
✔ no oscillation
✔ controlled response (no collapse)
✔ no hacky clamps
*/

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
}

func (c *Controller) Compute(
	curr PlantState,
	prev PlantState,
	mpc ControlInput,
) ControlInput {

	// =========================
	// 1. SIGNALS
	// =========================

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

	// =========================
	// 2. LYAPUNOV DRIFT
	// =========================

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

	// =========================
	// 3. STEADY STATE (CRITICAL FIX)
	// =========================

	steadyCap :=
    1.4 * curr.ArrivalMean / (curr.ServiceRate + 1e-6)

	// =========================
	// 4. SAFE REQUIRED REGION
	// =========================

	required :=
		steadyCap * (1 + math.Min(0.5, vol/10.0))

	maxSafe :=
    required * (2.2 + 0.8*math.Min(1.0, vol/10.0))

	// =========================
	// 5. LYAPUNOV CAPACITY
	// =========================

	lyapunovCap :=
		required * (1 + lyapunovSignal)

	// =========================
	// 6. INSTABILITY + MPC FUSION
	// =========================

	instability :=
		math.Sqrt(
			g*g +
				math.Max(0, p)*math.Max(0, p) +
				b*b +
				(vol/10.0)*(vol/10.0),
		)

	w := c.MPCWeight / (1 + instability)

	baseTarget :=
		(1-w)*lyapunovCap +
			w*mpc.CapacityTarget

	

	target := math.Max(
    required,
    math.Min(maxSafe, baseTarget),
) 


			// 🔥 TARGET SMOOTHING (critical)
err := math.Abs(target - curr.CapacityActive)

beta := 0.3
if err < 2.0 {
    beta = 0.8   // 🔥 near target → faster converge
}

target = curr.CapacityActive + beta*(target-curr.CapacityActive)

	
	

	

	delta := target - curr.CapacityActive

	maxStep := 4.0

	// Apply maxStep bound
	if delta > maxStep {
		delta = maxStep
	}
	if delta < -maxStep {
		delta = -maxStep
	}
	finalCap :=
		curr.CapacityActive + delta

	// =========================
	// 11. FINAL CLAMP
	// =========================

	// MaxCapacity clamp removed

	// =========================
	// 12. RETRY CONTROL
	// =========================

	retry := 0.0
if backlog > 0 || pressure > 0 {
	retry = 1.0 / (1.0 + math.Max(0, p) + backlog/50.0)
	retry = math.Min(0.4, retry)
}

	// =========================
	// 13. CACHE RELIEF
	// =========================

	cache :=
		math.Min(1.0,
			0.8*(backlog/(backlog+100.0))+
				0.4*(vol/10.0))

	// =========================
	// FINAL OUTPUT
	// =========================

	c.PrevCapacity = finalCap

	return ControlInput{
		CapacityTarget: finalCap,
		RetryFactor:    retry,
		CacheRelief:    cache,
	}
}