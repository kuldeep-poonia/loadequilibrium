package intelligence

import (
	"time"
)

/*
Ultra-Advanced Autonomy Orchestrator v3

Major upgrades:

• autonomy mode propagated downward (runtime gating + fusion hint)
• safe-anchor periodic re-validation using rollout + hazard
• adaptive fallback risk weighting derived from telemetry
• probabilistic degradation boundary (EW anomaly score)
• short-horizon trajectory certification
• regime-aware hysteresis memory depth
• simple meta-learning of degrade / recovery timing
*/

type AutonomyMode int

const (
	ModeAdvisory AutonomyMode = iota
	ModeSupervised
	ModeAutonomous
	ModeSafetyOnly
)

type OrchestratorInput struct {
	RuntimeIn   RuntimeInput
	TelemetryIn TelemetryInput
}

type OrchestratorOutput struct {
	Action []float64

	Confidence  float64
	Health      float64
	Instability float64

	Mode         AutonomyMode
	DegradeScore float64
}

type AutonomyOrchestrator struct {
	rt    *IntelligenceRuntime
	telm  *AutonomyTelemetryModel
	safeP *SafetyConstraintProjector
	roll  *PredictiveStabilityRollout
	haz   HazardEstimator

	mode AutonomyMode

	safeAnchor []float64

	anomalyEW float64
	modeEW    map[int]float64

	recoveryBase float64

	// lastModeTransitionAt records when we last transitioned to ModeAutonomous.
	// Used as a non-blocking cooldown gate replacing the removed time.Sleep.
	lastModeTransitionAt time.Time
}

func NewAutonomyOrchestrator(
	rt *IntelligenceRuntime,
	telm *AutonomyTelemetryModel,
	safeP *SafetyConstraintProjector,
	roll *PredictiveStabilityRollout,
	haz HazardEstimator,
	actDim int,
) *AutonomyOrchestrator {
	orchSafeDefault := make([]float64, actDim)
	if actDim > 0 {
		orchSafeDefault[0] = 1.0
	}

	return &AutonomyOrchestrator{
		rt:   rt,
		telm: telm,
		safeP: safeP,
		roll: roll,
		haz:  haz,
		// P9: Start in ModeSupervised so the RL policy warms up before driving
		// autonomous decisions. The orchestrator self-promotes to ModeAutonomous
		// once anomalyScore < 0.45 and health/confidence thresholds are met.
		mode:         ModeSupervised,
		safeAnchor:   orchSafeDefault,
		modeEW:       make(map[int]float64),
		recoveryBase: 3,
	}
}

/* ===== main ===== */

func (o *AutonomyOrchestrator) Step(
	in OrchestratorInput,
) OrchestratorOutput {

	/* propagate mode downward */

	in.RuntimeIn.GovernanceHint = int(o.mode)

	rtOut := o.rt.Tick(in.RuntimeIn)

	telOut := o.telm.Step(in.TelemetryIn)

	score := o.anomalyScore(telOut)

	o.updateMode(score, telOut, in)

	act :=
		o.certifyTrajectory(
			rtOut.Action,
			in.RuntimeIn,
		)

	return OrchestratorOutput{
		Action:       act,
		Confidence:   telOut.Confidence,
		Health:       telOut.Health,
		Instability:  telOut.Instability,
		Mode:         o.mode,
		DegradeScore: score,
	}
}

/* ===== anomaly boundary ===== */

func (o *AutonomyOrchestrator) anomalyScore(
	t TelemetryOutput,
) float64 {

	z :=
		1.3*(1-t.Health) +
			1.1*t.Instability +
			0.9*(1-t.Confidence)

	o.anomalyEW =
		0.9*o.anomalyEW +
			0.1*sigmoid(z)

	return o.anomalyEW
}

/* ===== mode logic ===== */

func (o *AutonomyOrchestrator) updateMode(
	score float64,
	t TelemetryOutput,
	in OrchestratorInput,
) {

	reg := in.RuntimeIn.Regime

	o.modeEW[reg] =
		0.85*o.modeEW[reg] +
			0.15*score

	recWin :=
		time.Duration(
			(o.recoveryBase+
				4*o.modeEW[reg]) *
				float64(time.Second),
		)

	switch {

	case score > 0.8:
		o.mode = ModeSafetyOnly

	case score > 0.6:
		o.mode = ModeSupervised

	case score > 0.45:
		o.mode = ModeAdvisory

	default:
		if o.mode != ModeAutonomous &&
			o.modeEW[reg] < 0.35 &&
			t.Health > 0.6 &&
			t.Confidence > 0.65 {

			// P1: Replaced time.Sleep(recWin/20) with a non-blocking time gate.
			// The sleep previously blocked the hot tick path for 75ms-1.8s on every
			// mode recovery, guaranteeing tick deadline violations and triggering the
			// safety escalation cascade. The gate enforces identical cooldown semantics
			// without blocking the calling goroutine.
			if time.Since(o.lastModeTransitionAt) >= recWin/20 {
				o.mode = ModeAutonomous
				o.lastModeTransitionAt = time.Now()
			}
		}
	}

	/* meta-learn recovery baseline */

	o.recoveryBase =
		clamp(
			o.recoveryBase+
				0.02*(score-0.5),
			1.5,
			6,
		)
}

/* ===== trajectory certification ===== */

func (o *AutonomyOrchestrator) certifyTrajectory(
	a []float64,
	in RuntimeInput,
) []float64 {

	if hasNaN(a) || vecNorm(a) > 9 {

		return o.certifiedFallback(in)
	}

	/* short rollout safety check */

	fc :=
		o.roll.Forecast(
			RolloutInput{
				State:     in.State,
				Action:    a,
				Regime:    in.Regime,
				ModelUnc:  in.ModelUnc,
				HazardUnc: in.HazardUnc,
				SLAWeight: in.StabilityVec,
				Policy:    in.Policy,
			},
		)

	hz :=
		o.haz.Estimate(in.State, a)

	risk :=
		0.6*avg(fc.RiskTrajectory) +
			0.4*hz.Mean

	if risk > 0.8 {

		return o.certifiedFallback(in)
	}

	o.safeAnchor = clone(a)

	return a
}

/* ===== fallback ===== */

func (o *AutonomyOrchestrator) certifiedFallback(
	in RuntimeInput,
) []float64 {

	/* re-validate anchor */

	fc :=
		o.roll.Forecast(
			RolloutInput{
				State:     in.State,
				Action:    o.safeAnchor,
				Regime:    in.Regime,
				ModelUnc:  in.ModelUnc,
				HazardUnc: in.HazardUnc,
				SLAWeight: in.StabilityVec,
				Policy:    in.Policy,
			},
		)

	if avg(fc.RiskTrajectory) > 0.85 {

		o.safeAnchor = make([]float64, len(o.safeAnchor))
	}

	/* adaptive fallback economics */

	scale :=
		1.2 +
			0.8*sigmoid(
				in.Risk+
					in.CapacityPress,
			)

	return o.safeP.Project(
		SafetyInput{
			Action:        o.safeAnchor,
			PrevAction:    o.safeAnchor,
			State:         in.State,
			StabilityVec:  in.StabilityVec,
			Risk:          in.Risk,
			HazardProxy:   in.Risk,
			CapacityPress: in.CapacityPress,
			SLAWeight:     scale,
		},
	).Action
}

/* ===== utils ===== */
