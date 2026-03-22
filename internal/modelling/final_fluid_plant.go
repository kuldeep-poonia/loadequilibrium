package modelling

import "math"

// ============================================================
// Research-grade numerically stable stochastic fluid queue plant
// Designed for long-horizon nonlinear congestion experiments
// ============================================================

type FinalFluidPlant struct {

	// ---- base parameters ----
	Mu  float64
	Rho float64

	// arrival dynamics
	KappaA float64
	Nu     float64
	ChiA   float64
	Psi0   float64
	Qsat   float64
	Amax   float64

	// congestion service degradation
	Alpha float64
	Beta  float64
	Eta   float64

	// stochastic volatility
	Theta float64
	Zeta  float64
	Pexp  float64

	// slow hazard physics
	Eps   float64
	Gamma float64

	// reservoir coupling
	Omega   float64
	LambdaR float64

	// soft boundary
	KL    float64
	Delta float64

	// timestep
	Dt float64

	// ---- states ----
	A float64
	Q float64
	Z float64
	R float64
}

// ================= helpers =================

func (p *FinalFluidPlant) rhoEff() float64 {
	return p.Rho + p.Omega*math.Tanh(p.R)
}

func (p *FinalFluidPlant) psi(q float64) float64 {
	return p.Psi0 * q / (q + p.Qsat + 1e-6)
}

func (p *FinalFluidPlant) sigma() float64 {

	base := 0.3 * (1 + p.Theta*p.Q/(1+p.Q))
	hazard := math.Pow(1+p.Zeta*p.Z/(1+p.Z), p.Pexp)

	s := base * hazard

	return clamp(s, 0, 5)
}

func (p *FinalFluidPlant) service(u float64) float64 {

	// softened degradation (no exponential underflow)
	congestion := 1.0 / (1 + p.Alpha*math.Pow(p.Q, p.Beta))
	hazard := 1.0 / (1 + p.Eta*p.Z)

	return p.Mu * u * congestion * hazard
}

func (p *FinalFluidPlant) reflectionForce(q float64) float64 {
	return p.KL * math.Exp(-q/p.Delta)
}

// ================= main integrator =================

func (p *FinalFluidPlant) Step(control float64, dBH float64) (float64, float64, float64) {

	dt := p.Dt

	// -------- safety guards ----------
	if math.IsNaN(p.Q) || math.IsInf(p.Q, 0) {
		p.Q = 0
	}
	if math.IsNaN(p.A) || math.IsInf(p.A, 0) {
		p.A = p.Rho * p.Mu
	}
	if math.IsNaN(p.Z) || math.IsInf(p.Z, 0) {
		p.Z = 0
	}
	if math.IsNaN(p.R) || math.IsInf(p.R, 0) {
		p.R = 0
	}

	// -------- effective intensity -----
	rhoEff := p.rhoEff()

	// -------- arrival dynamics --------
	driftA := p.KappaA*(rhoEff*p.Mu-p.A) +
		p.psi(p.Q) -
		p.ChiA*p.A*p.A

	p.A += driftA*dt + p.Nu*dBH

	// background load floor
	minLoad := 0.05 * p.Mu
	if p.A < minLoad {
		p.A = minLoad
	}

	p.A = clamp(p.A, 0, p.Amax)

	// -------- service -----------------
	S := p.service(control)

	// -------- stochastic forcing -------
	sig := p.sigma()
	noise := sig * dBH

	// -------- queue drift --------------
	driftQ := (p.A - S)

	// nonlinear congestion pressure
	driftQ -= 0.002 * p.Q * p.Q

	// soft reflection near zero
	barrier := p.reflectionForce(p.Q)

	p.Q += (driftQ+barrier)*dt + noise

	p.Q = clamp(p.Q, 0, 5000)

	// -------- slow hazard physics ------
	hz := p.Eps * math.Pow(p.Q/(1+p.Q), p.Gamma)
	p.Z += hz * dt

	p.Z = clamp(p.Z, 0, 1000)

	// -------- reservoir manifold -------
	p.R += ((p.A - S) - p.LambdaR*p.R) * dt
	p.R = clamp(p.R, -20, 20)

	return p.Q, p.A, p.Z
}