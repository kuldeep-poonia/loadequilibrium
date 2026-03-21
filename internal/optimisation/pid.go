package optimisation

import (
	"math"
	"time"
)

// PIDController implements a discrete-time PID controller with:
//   - Anti-windup integral clamping
//   - Hysteresis deadband to suppress chattering near setpoint
//   - Bounded output range
//   - Derivative filtering (N-filter) to suppress high-frequency noise
//   - Safe actuation bound: clamps per-tick output delta to prevent abrupt jumps
//   - Hysteresis band: suppresses micro-corrections when output barely changes
type PIDController struct {
	Kp, Ki, Kd  float64
	Setpoint     float64
	Deadband     float64
	OutputMin    float64
	OutputMax    float64
	IntegralMax  float64
	DerivativeN  float64 // filter coefficient (5..20 typical)

	// Safe actuation: clamp per-tick delta to this value (0 = disabled)
	MaxActuationStep float64

	// Hysteresis: suppress output update when |new - last| < HysteresisThreshold
	HysteresisThreshold float64

	integral      float64
	prevError     float64
	filteredDeriv float64
	prevTime      time.Time
	lastOutput    float64
	Active        bool
	HysteresisState string // "active" | "suppressed" | "deadband"
}

func NewPIDController(kp, ki, kd, setpoint, deadband, integralMax float64) *PIDController {
	return &PIDController{
		Kp:                  kp,
		Ki:                  ki,
		Kd:                  kd,
		Setpoint:            setpoint,
		Deadband:            deadband,
		OutputMin:           -1.0,
		OutputMax:           1.0,
		IntegralMax:         integralMax,
		DerivativeN:         10.0,
		MaxActuationStep:    0.15,
		HysteresisThreshold: 0.02,
	}
}

// Update advances the controller by one time step and returns the control output.
func (p *PIDController) Update(measurement float64, now time.Time) float64 {
	err := measurement - p.Setpoint

	if math.Abs(err) < p.Deadband {
		p.Active = false
		p.HysteresisState = "deadband"
		return p.lastOutput
	}
	p.Active = true

	dt := 2.0
	if !p.prevTime.IsZero() {
		dt = now.Sub(p.prevTime).Seconds()
		if dt <= 0 || dt > 30 {
			dt = 2.0
		}
	}

	// Proportional.
	proportional := p.Kp * err

	// Integral with anti-windup clamping.
	p.integral += err * dt
	p.integral = math.Max(-p.IntegralMax, math.Min(p.integral, p.IntegralMax))
	integral := p.Ki * p.integral

	// Derivative with N-filter: low-pass filter on raw derivative.
	rawDeriv := 0.0
	if !p.prevTime.IsZero() && dt > 0 {
		rawDeriv = (err - p.prevError) / dt
	}
	alpha := p.DerivativeN * dt / (1.0 + p.DerivativeN*dt)
	p.filteredDeriv = alpha*rawDeriv + (1-alpha)*p.filteredDeriv
	derivative := p.Kd * p.filteredDeriv

	output := proportional + integral + derivative
	output = math.Max(p.OutputMin, math.Min(output, p.OutputMax))

	// Safe actuation bound: clamp per-tick output delta.
	if p.MaxActuationStep > 0 {
		delta := output - p.lastOutput
		if math.Abs(delta) > p.MaxActuationStep {
			output = p.lastOutput + math.Copysign(p.MaxActuationStep, delta)
		}
	}

	// Hysteresis suppression: avoid micro-corrections.
	if math.Abs(output-p.lastOutput) < p.HysteresisThreshold {
		p.HysteresisState = "suppressed"
		p.prevError = err
		p.prevTime = now
		return p.lastOutput
	}

	p.HysteresisState = "active"
	p.prevError = err
	p.prevTime = now
	p.lastOutput = output
	return output
}

func (p *PIDController) Reset() {
	p.integral = 0
	p.prevError = 0
	p.filteredDeriv = 0
	p.prevTime = time.Time{}
	p.lastOutput = 0
	p.Active = false
	p.HysteresisState = ""
}
