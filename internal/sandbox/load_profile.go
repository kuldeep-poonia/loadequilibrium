package sandbox

import (
	"math"
	"math/rand"
	"time"
)

/*
PHASE-4 — LOAD PROFILE LIBRARY (REV-4 ADVANCED SPECTRAL / INCIDENT MODEL)

Sequence position:
2️⃣ after scenario_generator.go

This revision pushes envelope realism toward research-simulation grade.

Major advances:

✔ multi-shock stacking + oscillatory decay kernel
✔ RMS spectral normalization for arbitrary frequency sets
✔ asymmetric logistic trend (skew growth vs saturation)
✔ volatility floor protection in multiplicative mode
✔ smoothed duty-cycle step waveform (finite ramp)

Library still envelope-only layer.
Plant / retry / congestion handled elsewhere.

Human infra style intentionally uneven.
*/

type LoadProfileKind int

const (
	ProfileConstant LoadProfileKind = iota
	ProfileRamp
	ProfileSine
	ProfileMultiSine
	ProfileShockStack
	ProfileStepSmooth
	ProfileEnvelopeBlend
)

type StochasticMode int

const (
	StochasticAdd StochasticMode = iota
	StochasticMul
)

type Shock struct {
	Time     time.Duration
	Magnitude float64
}

type LoadProfileConfig struct {

	Kind LoadProfileKind

	Base float64
	Peak float64

	Period time.Duration

	Harmonics []float64
	Frequencies []float64

	RandomPhase bool
	Seed        int64

	// stacked shocks
	Shocks []Shock
	RiseTau float64
	DecayTau float64
	OscFreq float64
	OscDamp float64

	// asymmetric trend
	TrendRate float64
	TrendSkew float64
	TrendCap  float64

	BlendWeight float64

	// smooth step
	DutyCycle float64
	RampTau   float64

	MinClamp float64
	MaxClamp float64

	StochasticGain float64
	StochasticMode StochasticMode
}

type profileState struct {
	rng    *rand.Rand
	phases []float64
}

func NewProfileState(cfg LoadProfileConfig) *profileState {

	ps := &profileState{}

	if cfg.RandomPhase {

		ps.rng = rand.New(rand.NewSource(cfg.Seed))
		ps.phases = make([]float64, len(cfg.Harmonics))

		for i := range ps.phases {
			ps.phases[i] = ps.rng.Float64() * 2 * math.Pi
		}
	}

	return ps
}

func EvaluateProfile(
	cfg LoadProfileConfig,
	ps *profileState,
	t time.Duration,
	stochastic float64,
) float64 {

	val := cfg.Base

	switch cfg.Kind {

	case ProfileConstant:

		val = cfg.Base

	case ProfileRamp:

		val = cfg.Base + cfg.Peak*t.Seconds()

	case ProfileSine:

		phase :=
			2 * math.Pi *
				float64(t%cfg.Period) /
				float64(cfg.Period)

		val =
			cfg.Base +
				(cfg.Peak-cfg.Base)*
					(0.5 + 0.5*math.Sin(phase))

	case ProfileMultiSine:

		if len(cfg.Harmonics) == 0 {
			val = cfg.Base
			break
		}

		x := t.Seconds()

		sum := 0.0
		energy := 0.0

		for i, amp := range cfg.Harmonics {

			f := 2 * math.Pi * float64(i+1) /
				cfg.Period.Seconds()

			if len(cfg.Frequencies) > i {
				f = cfg.Frequencies[i]
			}

			phase := 0.0
			if cfg.RandomPhase && ps != nil {
				phase = ps.phases[i]
			}

			s := amp * math.Sin(f*x+phase)

			sum += s
			energy += s * s
		}

		if energy > 0 {
			sum = sum / math.Sqrt(energy/float64(len(cfg.Harmonics)))
		}

		val =
			cfg.Base +
				(cfg.Peak-cfg.Base)*
					(0.5 + 0.5*sum)

	case ProfileShockStack:

		total := 0.0

		for _, sh := range cfg.Shocks {

			if t < sh.Time {
				continue
			}

			dt := t.Seconds() - sh.Time.Seconds()

			rise :=
				1 / (1 + math.Exp(-dt/cfg.RiseTau))

			decay :=
				math.Exp(-dt / cfg.DecayTau)

			osc :=
				1 +
					math.Exp(-cfg.OscDamp*dt)*
						math.Sin(cfg.OscFreq*dt)

			total += sh.Magnitude * rise * decay * osc
		}

		val = cfg.Base * (1 + total)

	case ProfileStepSmooth:

		if cfg.Period == 0 {
			val = cfg.Base
			break
		}

		phase :=
			math.Mod(float64(t), float64(cfg.Period)) /
				float64(cfg.Period)

		target := cfg.Base
		if phase > cfg.DutyCycle {
			target = cfg.Peak
		}

		alpha :=
			1 - math.Exp(-t.Seconds()/cfg.RampTau)

		val =
			cfg.Base*(1-alpha) +
				target*alpha

	case ProfileEnvelopeBlend:

		x := t.Seconds()

		trend :=
			cfg.TrendCap /
				(1 +
					math.Exp(
						-cfg.TrendRate*
							(x-cfg.TrendSkew),
					))

		phase :=
			2 * math.Pi *
				float64(t%cfg.Period) /
				float64(cfg.Period)

		osc :=
			cfg.Peak *
				(0.5 + 0.5*math.Sin(phase))

		val =
			(1-cfg.BlendWeight)*trend +
				cfg.BlendWeight*osc
	}

	// stochastic modulation
	if cfg.StochasticMode == StochasticAdd {

		val += cfg.StochasticGain * stochastic

	} else {

		mul := 1 + cfg.StochasticGain*stochastic

		if mul < 0.2 {
			mul = 0.2
		}

		val *= mul
	}

	// clamp
	if cfg.MaxClamp > cfg.MinClamp {

		if val > cfg.MaxClamp {
			val = cfg.MaxClamp
		}

		if val < cfg.MinClamp {
			val = cfg.MinClamp
		}
	}

	return val
}
