package physics

import (
	"math"
	"math/rand"
	"os"
)

type PlantParams struct {

	// phase switching
	Lambda float64
	HighPhasePersistence float64

	// inflow relaxation
	InflowRelaxLow  float64
	InflowRelaxHigh float64
	InflowMeanLow   float64
	InflowMeanHigh  float64

	// service physics (advanced dissipative)
	BaseService   float64
	ServiceAlpha  float64
	ServiceBeta   float64
	RecoveryGain  float64

	// global stochastic stability gain
	StabilityGain float64

	// hazard physics (cyber damping coupling)
	HazardGain  float64
	HazardPower float64
	HazardHeal  float64
	HazardDrag  float64

	// reservoir physics
	ReservoirLambda  float64
	ReservoirCap     float64
	ArrivalBoostGain float64

	// noise physics (sub-linear diffusion)
	NoiseBase      float64
	NoiseGain      float64
	NoiseSpikeGain float64
}

type FluidPlant struct {

	Q float64
	A float64
	S float64

	Phase int

	Z float64
	R float64

	Sigma float64
	T     float64

	P   PlantParams
	rng *rand.Rand
}

func NewFluidPlant(seed int64) *FluidPlant {

	params := PlantParams{

		Lambda: 0.25,
		HighPhasePersistence: 4.5,

		InflowRelaxLow:  0.5,
		InflowRelaxHigh: 0.7,
		InflowMeanLow:   0.6,
		InflowMeanHigh:  1.05,

		BaseService:  1.6,
		ServiceAlpha: 0.35,
		ServiceBeta:  0.45,
		RecoveryGain: 0.02,

		StabilityGain: 0.42,

		HazardGain:  0.03,
		HazardPower: 2.0,
		HazardHeal:  0.003,
		HazardDrag:  0.015,

		ReservoirLambda:  0.09,
		ReservoirCap:     5.0,
		ArrivalBoostGain: 0.15,

		NoiseBase:      0.02,
		NoiseGain:      0.09,
		NoiseSpikeGain: 0.25,
	}

	return &FluidPlant{
		Q:   0,
		A:   0.6,
		S:   1,
		Z:   0,
		R:   0,
		T:   0,
		P:   params,
		rng: rand.New(rand.NewSource(seed)),
	}
}

func (p *FluidPlant) updatePhase(dt float64) {
	if os.Getenv("CRITICAL_LOAD_MODE") == "on" {
		return
	}

	lambda := p.P.Lambda
	mu := lambda / p.P.HighPhasePersistence

	if p.Phase == 0 {
		if p.rng.Float64() < lambda*dt {
			p.Phase = 1
		}
	} else {
		if p.rng.Float64() < mu*dt {
			p.Phase = 0
		}
	}
}

func (p *FluidPlant) updateInflow(dt float64) {
	if os.Getenv("CRITICAL_LOAD_MODE") == "on" {
		p.A = p.P.InflowMeanHigh
		return
	}

	if p.Phase == 0 {
		p.A += -p.P.InflowRelaxLow*(p.A-p.P.InflowMeanLow)*dt
	} else {
		p.A += -p.P.InflowRelaxHigh*(p.A-p.P.InflowMeanHigh)*dt
	}
}

func (p *FluidPlant) updateService() {

	q := math.Max(p.Q, 0)
	// coupled nonlinear saturating capacity law (with hazard degradation)
	p.S = p.P.BaseService / (1.0 + p.P.ServiceAlpha*q + p.P.ServiceBeta*p.Z)

	// optional mild recovery term
	p.S += p.P.RecoveryGain * math.Sqrt(q)
}

func (p *FluidPlant) updateHazard(dt float64) {

	load := math.Pow(p.Q/(1+p.Q), p.P.HazardPower)

	p.Z += (p.P.HazardGain*load -
		p.P.HazardHeal*p.Z -
		p.P.HazardDrag*p.Q) * dt

	if p.Z < 0 {
		p.Z = 0
	}
}

func (p *FluidPlant) updateReservoir(dt float64) {

	p.R += ((p.A - p.S) - p.P.ReservoirLambda*p.R) * dt

	if p.R > p.P.ReservoirCap {
		p.R = p.P.ReservoirCap
	}
	if p.R < -p.P.ReservoirCap {
		p.R = -p.P.ReservoirCap
	}
}

func (p *FluidPlant) updateNoise() {

	load := math.Sqrt(p.Q / (1 + p.Q))

	// noise increases near congestion threshold and with hazard
	p.Sigma = p.P.NoiseBase + p.P.NoiseGain*load + p.P.NoiseSpikeGain*p.Z
}

func (p *FluidPlant) Step(dt float64) {

	p.updatePhase(dt)
	p.updateInflow(dt)
	p.updateService()
	p.updateHazard(dt)
	p.updateReservoir(dt)
	p.updateNoise()

	// positive feedback from reservoir (unmet demand pressure) into arrival
	if os.Getenv("CRITICAL_LOAD_MODE") != "on" {
		p.A += p.P.ArrivalBoostGain * math.Max(p.R, 0) * dt
	}

	dW := p.rng.NormFloat64() * math.Sqrt(dt)

	// dissipative stochastic drift (research-grade stabiliser)
	net := (p.A - p.S) - p.P.StabilityGain*p.Q

	p.Q += net*dt + p.Sigma*dW

	if p.Q < 0 {
		p.Q = 0
	}

	p.T += dt
}

func (p *FluidPlant) Snapshot() map[string]float64 {

	return map[string]float64{
		"q":     p.Q,
		"a":     p.A,
		"s":     p.S,
		"z":     p.Z,
		"r":     p.R,
		"sigma": p.Sigma,
		"t":     p.T,
	}
}