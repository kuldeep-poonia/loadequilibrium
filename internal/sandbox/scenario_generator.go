package sandbox

import (
	"math"
	"math/rand"
	"time"
)

/*
PHASE-4 — SANDBOX SCENARIO GENERATOR (REV-5 ARCHITECTURE UPGRADE)

This revision upgrades generator to remove remaining architecture-scale realism gaps.

New modelling capabilities:

✔ fan-out partially backlog-coupled + stochastic
✔ retry kernel → exponential spacing + jitter + success decay
✔ hard service collapse regime switching (bifurcation-like)
✔ admission control loop (rate limit / shedding / breaker)
✔ multi-shock spike sequences with aftershock bursts

Still generator layer — not full plant —
but now traffic signal closer to real incident telemetry.

Style intentionally uneven like real infra code.
*/

type ScenarioKind int

const (
	ScenarioSteady ScenarioKind = iota
	ScenarioSpike
	ScenarioDiurnal
	ScenarioRetryStorm
	ScenarioBrownout
)

type ScenarioConfig struct {

	BaseArrival float64
	BaseService float64

	Duration time.Duration
	Step     time.Duration

	Seed int64

	NoiseStd float64
	ARCoef   float64

	// retry behaviour
	RetryGain     float64
	RetryDecay    float64
	RetryJitter   float64
	SaturationCap float64

	// burst clustering
	BurstOnProb   float64
	BurstOffProb  float64
	ParetoAlpha   float64
	BurstCeiling  float64
	HeavyTailProb float64

	// diurnal
	Harmonics  []float64
	PhaseDrift float64

	// spike sequence
	ShockTimes []time.Duration
	ShockMag   float64
	RelaxTau   float64

	// collapse
	CapacityDrop float64
	CollapseProb float64

	// admission control
	RateLimit     float64
	ShedProb      float64
	BreakerThresh float64

	// fanout
	FanoutBase float64
	FanoutLoad float64
	FanoutVar  float64
}

type LoadPoint struct {
	T       time.Duration
	Arrival float64
	Fanout  float64
}

type Scenario struct {
	Config ScenarioConfig
	Trace  []LoadPoint
}

type genState struct {

	rng *rand.Rand

	prevNoise float64

	backlog float64

	serviceState float64
	collapsed    bool

	burstState bool

	retryClock float64

	phases []float64
}

func GenerateScenario(cfg ScenarioConfig, kind ScenarioKind) *Scenario {

	s := &genState{
		rng: rand.New(rand.NewSource(cfg.Seed)),
	}

	s.serviceState = cfg.BaseService

	s.phases = make([]float64, len(cfg.Harmonics))
	for i := range s.phases {
		s.phases[i] = s.rng.Float64() * 2 * math.Pi
	}

	trace := make([]LoadPoint, 0)

	for t := time.Duration(0); t < cfg.Duration; t += cfg.Step {

		arrival := baseArrival(cfg, s, kind, t)

		service := serviceDynamics(cfg, s)

		s.backlog += (arrival - service) * cfg.Step.Seconds()
		if s.backlog < 0 {
			s.backlog = 0
		}

		arrival += retryProcess(cfg, s)

		arrival += clusteredBurst(cfg, s)

		arrival += correlatedNoise(cfg, s)

		arrival = admissionControl(cfg, s, arrival)

		fanout := fanoutFactor(cfg, s)

		if arrival < 0 {
			arrival = 0
		}

		trace = append(trace, LoadPoint{
			T:       t,
			Arrival: arrival,
			Fanout:  fanout,
		})
	}

	return &Scenario{
		Config: cfg,
		Trace:  trace,
	}
}

func baseArrival(cfg ScenarioConfig, s *genState, kind ScenarioKind, t time.Duration) float64 {

	switch kind {

	case ScenarioDiurnal:

		x := t.Seconds()

		sum := 0.0
		for i, amp := range cfg.Harmonics {

			freq := float64(i+1) * 2 * math.Pi / (24 * 3600)

			s.phases[i] += cfg.PhaseDrift * s.rng.NormFloat64()

			sum += amp * math.Sin(freq*x+s.phases[i])
		}

		return cfg.BaseArrival + sum

	case ScenarioSpike:

		total := cfg.BaseArrival

		for _, st := range cfg.ShockTimes {

			if t >= st {

				dt := t.Seconds() - st.Seconds()

				total += cfg.ShockMag *
					math.Exp(-dt/cfg.RelaxTau)
			}
		}

		return total

	default:
		return cfg.BaseArrival
	}
}

func serviceDynamics(cfg ScenarioConfig, s *genState) float64 {

	target := cfg.BaseService

	pressure := math.Tanh(s.backlog / cfg.SaturationCap)

	target = cfg.BaseService * (1 - cfg.CapacityDrop*pressure)

	if !s.collapsed && s.rng.Float64() < cfg.CollapseProb*pressure {

		s.collapsed = true
	}

	if s.collapsed {

		target *= 0.15
	}

	alpha := 0.05

	s.serviceState =
		(1-alpha)*s.serviceState +
			alpha*target

	if s.serviceState < 0.05*cfg.BaseService {
		s.serviceState = 0.05 * cfg.BaseService
	}

	return s.serviceState
}

func retryProcess(cfg ScenarioConfig, s *genState) float64 {

	pressure := math.Tanh(s.backlog / cfg.SaturationCap)

	s.retryClock -= 1

	if s.retryClock > 0 {
		return 0
	}

	interval :=
		math.Exp(-pressure*cfg.RetryDecay)

	jitter :=
		1 + cfg.RetryJitter*s.rng.NormFloat64()

	s.retryClock = interval * jitter * 10

	success :=
		math.Exp(-pressure)

	return cfg.RetryGain * pressure * (1 - success)
}

func clusteredBurst(cfg ScenarioConfig, s *genState) float64 {

	if s.burstState {

		if s.rng.Float64() < cfg.BurstOffProb {
			s.burstState = false
		}

	} else {

		if s.rng.Float64() < cfg.BurstOnProb {
			s.burstState = true
		}
	}

	if !s.burstState {
		return 0
	}

	if s.rng.Float64() > cfg.HeavyTailProb {
		return 0
	}

	u := s.rng.Float64()

	raw := math.Pow(1-u, -1/cfg.ParetoAlpha)

	if raw > cfg.BurstCeiling {
		raw = cfg.BurstCeiling
	}

	return raw * (1 + math.Tanh(s.backlog/cfg.SaturationCap))
}

func admissionControl(cfg ScenarioConfig, s *genState, arrival float64) float64 {

	if arrival > cfg.RateLimit {

		if s.rng.Float64() < cfg.ShedProb {

			arrival *= 0.6
		}
	}

	if s.backlog > cfg.BreakerThresh {

		arrival *= 0.3
	}

	return arrival
}

func fanoutFactor(cfg ScenarioConfig, s *genState) float64 {

	loadEffect :=
		cfg.FanoutLoad * math.Tanh(s.backlog/cfg.SaturationCap)

	return cfg.FanoutBase +
		loadEffect +
		cfg.FanoutVar*s.rng.NormFloat64()
}

func correlatedNoise(cfg ScenarioConfig, s *genState) float64 {

	if cfg.NoiseStd == 0 {
		return 0
	}

	z := s.rng.NormFloat64() * cfg.NoiseStd

	out := cfg.ARCoef*s.prevNoise + z

	s.prevNoise = out

	return out
}
