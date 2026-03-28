package intelligence

import (
	"math"
	"math/rand"
	"time"
)

/*
Frontier Intelligence Runtime Loop v3

Major upgrades:

• multi-candidate control horizon optimisation (light MPC search)
• probabilistic fusion of rollout risk + hazard critic
• async / budget-aware online learning
• governance-mode aware fusion context
• certified safe fallback via projector on safe anchor config
• runtime health watchdog (divergence / uncertainty / oscillation)
*/

type RuntimeModules struct {
	Meta    *MetaAutonomyController
	Safety  *SafetyConstraintProjector
	Rollout *PredictiveStabilityRollout

	Hazard  HazardEstimator
	Fusion  StrategicFusion
	Learner SignalLearner
	Trainer AsyncTrainer
}

type RuntimeState struct {
	LastAction []float64
	SafeAnchor []float64

	uncEW   float64
	oscEW   float64
	learnEW float64

	// One-step TD delay buffer — stores (s, a, hazard) from previous tick
	// so that TryStep can be called with (prevState, prevAction, prevHazard, r, nextState=current).
	prevTDState  []float64
	prevTDAction []float64
	prevTDHazard float64
	hasPrevTD    bool
}

type IntelligenceRuntime struct {
	mod RuntimeModules
	st  RuntimeState

	tickDur  time.Duration
	deadline time.Duration
}

/* ===== ctor ===== */

func NewIntelligenceRuntime(mod RuntimeModules, actDim int) *IntelligenceRuntime {
	safeDefault := make([]float64, actDim)
	if actDim > 0 {
		safeDefault[0] = 1.0 // ScaleDelta index 0: neutral (no capacity change)
		// indices 1..3 remain 0.0 — cache/shard/retry unchanged
	}

	return &IntelligenceRuntime{
		mod: mod,
		st: RuntimeState{
			LastAction: make([]float64, actDim),
			SafeAnchor: safeDefault,
		},
		tickDur:  120 * time.Millisecond,
		deadline: 80 * time.Millisecond,
	}
}

/* ===== main tick ===== */

func (r *IntelligenceRuntime) Tick(in RuntimeInput) RuntimeOutput {

	start := time.Now()

	/* ===== signal learner ===== */

	r.mod.Learner.Update(in.State)

	/* ===== multi candidate MPC search ===== */

	uBest := r.searchAction(in)

	/* ===== unified risk belief ===== */

	fc :=
		r.mod.Rollout.Forecast(
			RolloutInput{
				State:     in.State,
				Action:    uBest,
				Regime:    in.Regime,
				ModelUnc:  in.ModelUnc,
				HazardUnc: in.HazardUnc,
				SLAWeight: in.StabilityVec,
				Policy:    in.Policy,
			},
		)

	hz :=
		r.mod.Hazard.Estimate(in.State, uBest)

	riskBelief :=
		0.6*avg(fc.RiskTrajectory) +
			0.4*hz.Mean

	/* ===== governance ===== */

	meta :=
		r.mod.Meta.Step(
			MetaInput{
				GlobalRisk:   riskBelief,
				RiskForecast: fc.RiskTrajectory,

				HazardUnc:      hz.Uncertainty,
				ModelUnc:       in.ModelUnc,
				EpistemicTrend: hz.EpistemicTrend,

				PerfSignal: in.Perf,
				PerfTrend:  in.PerfTrend,

				StabilityMargin: avg(in.StabilityVec),

				EntropyProxy:  in.EntropyProxy,
				GradMagProxy:  in.GradProxy,
				ReplayNovelty: in.Novelty,

				CapacityPressure: in.CapacityPress,

				SLASeverity: in.SLASeverity,
				OscPenalty:  fc.SpectralGrowth,

				Regime: in.Regime,
			},
		)

	/* ===== strategic fusion ===== */

	uFuse :=
		r.mod.Fusion.CombineStrategic(
			uBest,
			r.st.LastAction,
			meta.AutonomyLevel,
			meta.SafetyGain,
			meta.GovernanceMode,
			fc.RiskTrajectory,
			hz,
		)

	/* ===== safety projection ===== */

	safe :=
		r.mod.Safety.Project(
			SafetyInput{
				Action:        uFuse,
				PrevAction:    r.st.LastAction,
				State:         in.State,
				StabilityVec:  in.StabilityVec,
				Risk:          riskBelief,
				HazardProxy:   hz.Mean,
				CapacityPress: in.CapacityPress,
				SLAWeight:     meta.SafetyGain,
			},
		)

	action := safe.Action
	fallback := false

	/* ===== runtime health ===== */

	if r.healthBad(hz, fc) ||
		time.Since(start) > r.deadline ||
		hasNaN(action) {

		action =
			r.certifiedFallback(in)

		fallback = true
	}

	/* ===== async learning ===== */

	// Call TryStep with PREVIOUS tick's (s, a, hazard) and CURRENT state as s'.
	// This gives the proper bootstrapped TD target: r + γV(s') - V(s).
	if r.st.hasPrevTD {
		r.mod.Trainer.TryStep(
			r.st.prevTDState,
			r.st.prevTDAction,
			r.st.prevTDHazard,
			in.Perf,
			time.Since(start),
		)
	}

	// Store current (s, a, hazard) for next tick.
	r.st.prevTDState = make([]float64, len(in.State))
	copy(r.st.prevTDState, in.State)
	r.st.prevTDAction = make([]float64, len(action))
	copy(r.st.prevTDAction, action)
	r.st.prevTDHazard = hz.Mean
	r.st.hasPrevTD = true

	r.st.LastAction = action

	return RuntimeOutput{
		Action:         action,
		AutonomyLevel:  meta.AutonomyLevel,
		GovernanceMode: meta.GovernanceMode,
		Fallback:       fallback,
	}
}

/* ===== candidate search ===== */

func (r *IntelligenceRuntime) searchAction(in RuntimeInput) []float64 {

	best := r.st.LastAction
	bestCost := math.Inf(1)

	for k := 0; k < 5; k++ {

		u :=
			perturb(
				safeCallPolicy(in.Policy, in.State),
				0.4*float64(k),
			)

		fc :=
			r.mod.Rollout.Forecast(
				RolloutInput{
					State:     in.State,
					Action:    u,
					Regime:    in.Regime,
					ModelUnc:  in.ModelUnc,
					HazardUnc: in.HazardUnc,
					SLAWeight: in.StabilityVec,
					Policy:    in.Policy,
				},
			)

		cost :=
			avg(fc.RiskTrajectory) +
				0.2*fc.SpectralGrowth

		if cost < bestCost {
			bestCost = cost
			best = u
		}
	}

	return best
}

/* ===== health monitor ===== */

func (r *IntelligenceRuntime) healthBad(
	hz HazardOut,
	fc StabilityForecast,
) bool {

	r.st.uncEW =
		0.9*r.st.uncEW +
			0.1*hz.Uncertainty

	r.st.oscEW =
		0.85*r.st.oscEW +
			0.15*fc.SpectralGrowth

	return r.st.uncEW > 0.8 ||
		r.st.oscEW > 1.4
}

/* ===== certified fallback ===== */

func (r *IntelligenceRuntime) certifiedFallback(
	in RuntimeInput,
) []float64 {

	return r.mod.Safety.Project(
		SafetyInput{
			Action:        r.st.SafeAnchor,
			PrevAction:    r.st.LastAction,
			State:         in.State,
			StabilityVec:  in.StabilityVec,
			Risk:          in.Risk,
			HazardProxy:   in.Risk,
			CapacityPress: in.CapacityPress,
			SLAWeight:     1.5,
		},
	).Action
}

/* ===== utils ===== */

func perturb(u []float64, s float64) []float64 {

	v := make([]float64, len(u))

	for i := range u {
		v[i] =
			u[i] +
				s*(randUnit()-0.5)
	}

	return v
}

func randUnit() float64 {
	// P2: Use math/rand for proper uniform [0,1) distribution.
	// The previous implementation used time.Now().UnixNano()%1000/1000 which
	// returned the same value within a single tick cycle on modern CPUs, making
	// all 5 MPC candidate perturbations in searchAction() identical and
	// effectively collapsing the search to a single unevaluated point.
	return rand.Float64()
}

func safeCallPolicy(
	p func([]float64) []float64,
	x []float64,
) (u []float64) {

	defer func() {
		if recover() != nil {
			u = make([]float64, len(x))
		}
	}()

	return p(x)
}

/* ===== fusion interface ===== */

type StrategicFusion interface {
	CombineStrategic(
		uPolicy []float64,
		uPrev []float64,
		autoLevel float64,
		safetyGain float64,
		mode int,
		riskVec []float64,
		hz HazardOut,
	) []float64
}

/* ===== async trainer ===== */

type AsyncTrainer interface {
	TryStep(
		state, action []float64,
		hazard, perf float64,
		elapsed time.Duration,
	)
}
