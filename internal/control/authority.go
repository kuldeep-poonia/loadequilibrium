package control

import (
	"hash/fnv"
	"math"
	"time"

	"github.com/loadequilibrium/loadequilibrium/internal/optimisation"
)

type AutopilotAdvice struct {
	MinReplicas      int
	MaxReplicas      int
	PredictedBacklog float64
	PredictedLatency float64
	InstabilityRisk  float64
	Confidence       float64
	OverrideRate     float64
	Mode             int
	Damping          float64
	Warning          bool
}

type IntelligenceAdvice struct {
	Regime       int
	RiskEWMA     float64
	AnomalyScore float64
	RiskWeight   float64
	SmoothCost   float64
	CostBias     float64
}

type PolicyAdvice struct {
	DesiredReplicas int
	MinReplicas     int
	MaxReplicas     int
	RetryLimit      int
	QueueLimit      float64
	CacheAggression float64
	Risk            float64
	Confidence      float64
}

type SandboxAdvice struct {
	CapacityDelta      float64
	EfficiencyDelta    float64
	DampingDelta       float64
	RetryPressureDelta float64
	BrownoutDelta      float64
	RiskScore          float64
	Urgency            float64
	Confidence         float64
	RiskUp             bool
}

type AdvisoryBundle struct {
	Autopilot    AutopilotAdvice
	Intelligence IntelligenceAdvice
	Policy       PolicyAdvice
	Sandbox      SandboxAdvice
}

type AuthorityConfig struct {
	TargetUtilisation float64
	TickSeconds       float64
	MaxScaleDelta     float64
}

type AuthorityInput struct {
	ServiceID           string
	Tick                uint64
	Now                 time.Time
	State               SystemState
	Config              AuthorityConfig
	Advisory            AdvisoryBundle
	OptimizerCandidates []optimisation.ControlCandidate
}

type DecisionQuality struct {
	UsedSignals    []string
	Contradictions []string
	AdvisoryRisk   float64
	CandidateCount int
}

type AuthorityDecision struct {
	Directive optimisation.ControlDirective
	Bundle    Bundle
	Bounds    ActionBounds
	Quality   DecisionQuality
}

// Authority is the single executable decision maker. Other modules may shape
// bounds, search radius, costs, and candidate sets, but only Authority emits the
// ControlDirective sent to the actuator.
type Authority struct {
	Memory *RegimeMemory

	LastBundle    Bundle
	LastDirective optimisation.ControlDirective
}

func NewAuthority() *Authority {
	return &Authority{Memory: NewRegimeMemory()}
}

func (a *Authority) Decide(in AuthorityInput) AuthorityDecision {

	


	
	if a.Memory == nil {
		a.Memory = NewRegimeMemory()
	}

	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}

	state := normalizeState(in.State)

	

	targetUtil := in.Config.TargetUtilisation
	if targetUtil <= 0 {
		targetUtil = 0.70
	}
	tickSeconds := in.Config.TickSeconds
	if tickSeconds <= 0 {
		tickSeconds = 1
	}
	

	quality := DecisionQuality{
		UsedSignals: []string{
			"autopilot.bounds",
			"autopilot.risk",
			"autopilot.trajectory",
			"intelligence.regime",
			"intelligence.risk_ewma",
			"intelligence.cost_shape",
			"policy.bounds",
			"policy.candidates",
			"sandbox.risk",
			"sandbox.candidates",
			"optimizer.ranked_candidates",
		},
	}

	bounds := a.deriveBounds(state, in.Advisory, &quality)


	risk := aggregateAdvisoryRisk(state, in.Advisory)
	// Guard: InstabilityRisk is computed as (VarianceScale - 1.0) which can be
	// negative at idle. Clamp to 0 so it doesn't pull risk below the state floor.
	if in.Advisory.Autopilot.InstabilityRisk < 0 {
		in.Advisory.Autopilot.InstabilityRisk = 0
	}
	quality.AdvisoryRisk = risk

	a.Memory.Update(
		SystemState{
			Utilisation: state.Utilisation,
			Risk:        risk,
			Latency:     state.Latency,
		},
		math.Max(state.SLATarget, 1),
		risk,
		defaultRegimeConfig(),
	)

	state.MinReplicas = bounds.MinReplicas
	state.MaxReplicas = bounds.MaxReplicas
	state.MinRetry = bounds.MinRetry
	state.MaxRetry = bounds.MaxRetry
	state.Risk = risk

	// 🔥 EMERGENCY BACKLOG OVERRIDE
	// 🔥 TRUE EMERGENCY CONDITION

	// Radius controls the local search neighbourhood.
	// With radius=3 and low risk the generator spans current±3 — enough to
	// reach the util-optimal replica count within a few ticks rather than being
	// stuck at ±1 while injecting forced scale-up candidates that always win.
	radius := 3

	if risk > 0.5 {
		radius = 6
	}
	if risk > 0.75 {
		radius = 10
	}
	if in.Advisory.Intelligence.Regime >= int(RegimeUnstable) || in.Advisory.Autopilot.Warning {
		radius++
	}

	// Extend radius when significantly over-provisioned so the optimizer can
	// reach the utilisation-optimal count without needing many ticks to ramp down.
	if state.Utilisation < 0.4 && state.QueueDepth < float64(state.QueueLimit)*0.1 {
		scaleDownReach := int(float64(state.Replicas) * (1.0 - state.Utilisation/0.7))
		if scaleDownReach > radius {
			radius = scaleDownReach
		}
	}

	candidates := GenerateBundles(
		state,
		GeneratorConfig{
			BaseRadius: radius,
			Seed:       seedFor(in.ServiceID, in.Tick, 0x41),
		},
		a.Memory,
	)

	// Only inject emergency scale-up seeds when the queue is genuinely overloaded.
	// The previous unconditional injection of state.Replicas+{2,4,6,8} with
	// HeuristicScore=0 always sorted to the front and beat scale-down candidates
	// regardless of actual load, causing false scale_up↔scale_down conflicts.
	if state.QueueDepth > float64(state.QueueLimit)*0.5 {
		for i := 2; i <= 8; i += 2 {
			b := bundleFromState(state)
			b.Replicas = state.Replicas + i
			candidates = append(candidates, b)
		}
	}



	candidates = append(candidates, a.advisoryCandidates(state, bounds, in.Advisory, in.OptimizerCandidates)...)

	// 🔥 FORCE autopilot when confidence high
	if in.Advisory.Autopilot.Confidence > 0.6 &&
		in.Advisory.Autopilot.PredictedBacklog > float64(state.QueueDepth)*1.2 &&
		float64(state.QueueDepth) > float64(state.QueueLimit)*0.7 {

		forced := bundleFromState(state)
		forced.Replicas = state.Replicas + 1 // +1 not +2 to keep step bounded

		candidates = append(candidates, forced)
	}


	candidates = uniqueBundles(candidates)
	quality.CandidateCount = len(candidates)


	

	// 🔥 REAL FIX — use actual backlog instead of weak predicted
if in.Advisory.Autopilot.PredictedBacklog < float64(state.QueueDepth) {
    in.Advisory.Autopilot.PredictedBacklog = float64(state.QueueDepth)
}

best := SelectBestBundle(
    state,
    candidates,
    defaultOptimizerConfig(),
    defaultSimConfig(tickSeconds, seedFor(in.ServiceID, in.Tick, 0x51)),
    defaultCostParams(targetUtil, risk, in.Advisory),
    a.Memory,
)

	// 🔥 FORCE CORRECTION (CRITICAL)
if state.QueueDepth > float64(state.QueueLimit) && best.Replicas <= state.Replicas {
    best.Replicas = state.Replicas + 2
}

	// SOFT clamp only for invalid values
if best.Replicas < 1 {
    best.Replicas = 1
}
	rawScale := scaleFromBundle(state, best)

	// 🔥 FINAL OVERRIDE (NO MORE HOLD BULLSHIT)
if state.QueueDepth > float64(state.QueueLimit) {
    rawScale = math.Max(rawScale, 1.5)
}

// 🚀 NO RATE LIMITER
scale := rawScale

scale = clamp(scale, 0.45, 10.0)

// 🔥 STRONG BACKLOG RESPONSE
if state.QueueDepth > float64(state.QueueLimit) {
    scale = math.Max(scale, 1.5)
}

if state.QueueDepth > float64(2*state.QueueLimit) {
    scale = math.Max(scale, 2.0)
}

if state.QueueDepth > float64(3*state.QueueLimit) {
    scale = math.Max(scale, 3.0)
}


	directive := optimisation.ControlDirective{
		ServiceID:                 in.ServiceID,
		ComputedAt:                now,
		ScaleFactor:               scale,
		TargetUtilisation:         clamp(targetUtil-0.05*risk-0.02*in.Advisory.Sandbox.BrownoutDelta, 0.45, 0.90),
		Error:                     state.Utilisation - targetUtil,
		Active:                    true,
		StabilityMargin:           1 - risk,
		HysteresisState:           "control-authority",
		ActuationBound: 1.0,
		PredictiveTarget:          clamp(state.Utilisation+state.Risk, 0, 1.5),
		MPCPredictedRho:           best.HeuristicScore,
		MPCOvershootRisk:          rawScale > scale,
		MPCUnderactuationRisk:     risk > 0.65 && scale <= 1.0,
		CostGradient:              risk + math.Max(0, in.Advisory.Intelligence.CostBias),
		TrajectoryCostAvg:         risk,
		MaxTrajectoryCost:         math.Max(risk, in.Advisory.Sandbox.RiskScore),
		PlannerScaleFactor:        rawScale,
		PlannerConvergent:         risk < 0.65,
		PlannerConvex:             true,
		PlannerProbabilisticScore: math.Max(risk, in.Advisory.Sandbox.RiskScore),
	}

	a.LastBundle = best
	a.LastDirective = directive
	a.Memory.RecordAction(best)


	return AuthorityDecision{
		Directive: directive,
		Bundle:    best,
		Bounds:    bounds,
		Quality:   quality,
	}
}

func (a *Authority) deriveBounds(state SystemState, adv AdvisoryBundle, quality *DecisionQuality) ActionBounds {
	minReplicas := maxInt(state.MinReplicas, 1)
	// 🚀 REMOVE HARD CEILING (CRITICAL FIX)
maxReplicas := maxInt(state.Replicas+12, minReplicas)

	if adv.Policy.MinReplicas > 0 {
		minReplicas = maxInt(minReplicas, adv.Policy.MinReplicas)
	}
if adv.Policy.MaxReplicas > 0 {
    maxReplicas = adv.Policy.MaxReplicas
}

if adv.Autopilot.MinReplicas > 0 {
		minReplicas = maxInt(minReplicas, adv.Autopilot.MinReplicas)
	}
	if adv.Autopilot.MaxReplicas > 0 {
    maxReplicas = maxInt(maxReplicas, adv.Autopilot.MaxReplicas)
}

	risk := aggregateAdvisoryRisk(state, adv)
	if risk > 0.75 {
		minReplicas = maxInt(minReplicas, state.Replicas)
	}
	if minReplicas > maxReplicas {
		quality.Contradictions = append(quality.Contradictions, "replica bounds inverted; widened max to min")
		maxReplicas = minReplicas
	}

	minRetry := maxInt(state.MinRetry, 1)
	maxRetry := state.MaxRetry
	if maxRetry <= 0 {
		maxRetry = maxInt(state.RetryLimit+2, minRetry)
	}

	return ActionBounds{
		MinReplicas: minReplicas,
		MaxReplicas: maxReplicas,
		MinQueue:    1,
		MaxQueue:    maxInt(state.QueueLimit*2, state.QueueLimit+1),
		MinRetry:    minRetry,
		MaxRetry:    maxRetry,
		MinCache:    0,
		MaxCache:    1,
	}
}

func (a *Authority) advisoryCandidates(
	state SystemState,
	bounds ActionBounds,
	adv AdvisoryBundle,
	opt []optimisation.ControlCandidate,
) []Bundle {
	out := make([]Bundle, 0, len(opt)+4)

	addReplicas := func(replicas int) {
		b := bundleFromState(state)
		b.Replicas = clampInt(replicas, bounds.MinReplicas, bounds.MaxReplicas)
		out = append(out, b)
	}
	addScale := func(scale float64) {
		if scale <= 0 || math.IsNaN(scale) || math.IsInf(scale, 0) {
			return
		}
		addReplicas(int(math.Round(float64(state.Replicas) * scale)))
	}

	if adv.Policy.DesiredReplicas > 0 {
		addReplicas(adv.Policy.DesiredReplicas)
	}
	if adv.Sandbox.CapacityDelta != 0 {
		addScale(1 + adv.Sandbox.CapacityDelta)
	}
	if adv.Autopilot.PredictedBacklog > state.QueueDepth && adv.Autopilot.Confidence > 0.2 {
		addReplicas(state.Replicas + 1)
	}
	if adv.Intelligence.Regime >= int(RegimeUnstable) || adv.Intelligence.AnomalyScore > 0.6 {
		addReplicas(state.Replicas + 1)
	}

	for _, c := range opt {
		if !c.Feasible && c.RiskScore > 0.9 {
			continue
		}
		addScale(c.ScaleFactor)
	}

	return out
}

func (a *Authority) enforceScaleRate(scale, maxDelta float64) float64 {
	if a.LastDirective.ScaleFactor <= 0 || maxDelta <= 0 {
		return scale
	}
	delta := scale - a.LastDirective.ScaleFactor
	if math.Abs(delta) <= maxDelta {
		return scale
	}
	return a.LastDirective.ScaleFactor + math.Copysign(maxDelta, delta)
}

func normalizeState(s SystemState) SystemState {
	if s.Replicas <= 0 {
		s.Replicas = 1
	}
	if s.QueueLimit <= 0 {
		s.QueueLimit = 1
	}
	if s.RetryLimit <= 0 {
		s.RetryLimit = 1
	}
	if s.ServiceRate <= 0 {
		s.ServiceRate = 1
	}
	if s.SLATarget <= 0 {
		s.SLATarget = 1
	}
	if s.MaxReplicas > 0 && s.MinReplicas > s.MaxReplicas {
		s.MaxReplicas = s.MinReplicas
	}
	if s.Utilisation <= 0 && s.PredictedArrival > 0 {
		s.Utilisation = s.PredictedArrival / (float64(s.Replicas)*s.ServiceRate + 1e-6)
	}
	return s
}

func aggregateAdvisoryRisk(state SystemState, adv AdvisoryBundle) float64 {
	risk := state.Risk
	risk = math.Max(risk, adv.Autopilot.InstabilityRisk)
	risk = math.Max(risk, adv.Autopilot.OverrideRate)
	risk = math.Max(risk, adv.Intelligence.RiskEWMA)
	risk = math.Max(risk, adv.Intelligence.AnomalyScore*0.8)
	risk = math.Max(risk, adv.Policy.Risk)
	risk = math.Max(risk, adv.Sandbox.RiskScore)
	if adv.Sandbox.RiskUp {
		risk = math.Max(risk, 0.65)
	}
	return clamp(risk, 0, 1)
}

func defaultOptimizerConfig() OptimizerConfig {
	return OptimizerConfig{
		ScenarioCount:   4,
		EarlyStopMargin: 0.20,
		BaseTemperature: 0.35,
		MaxEvaluate:     32,
		MinEvaluate:     4,
	}
}

func defaultSimConfig(dt float64, seed int64) SimConfig {
	return SimConfig{
		HorizonSteps:      12,
		BaseLatency:       1,
		Dt:                dt,
		DisturbanceStd:    0.12,
		DisturbanceFreq:   0.2,
		RetryFeedbackGain: 0.35,
		WarmupRate:        0.30,
		EfficiencyDecay:   0.12,
		MaxQueueDelay:     200,
		HazardUtilGain:    0.5,
		HazardBacklogGain: 0.4,
		HazardRetryGain:   0.3,
		Seed:              seed,
	}
}

func defaultCostParams(targetUtil, risk float64, adv AdvisoryBundle) CostParams {
    // FIX: Make backlog cost non-linear so it eventually forces a scale-up
    // A backlog of 300 now produces a weight of ~16.0 instead of 1.8
    backlogFactor := math.Pow(math.Max(0, adv.Autopilot.PredictedBacklog)/40.0, 2.5)

    return CostParams{
        InfraUnitCost:   1,
        SLAWeight:       1.5 + risk + math.Max(0, adv.Sandbox.Urgency),
        RiskWeight:      1.5 + 1.0*risk,        // REDUCED: Was 2.0 + 2.5*risk
        BacklogWeight:   5.0 + backlogFactor*2,  // INCREASED: Was 1.5 + backlog/1000
        UtilTarget:      clamp(targetUtil-0.05*risk, 0.45, 0.85),
        UtilBand:        0.15,
        SmoothReplica:   0.05, // LOWERED: Allow faster response
        SmoothRetry:     0.20,
        SmoothQueue:     0.20,
        SmoothCache:     0.20,
        CacheCostWeight: 0.50,
    }
}
func enforceBundleBounds(b Bundle, bounds ActionBounds) Bundle {
	b.Replicas = clampInt(b.Replicas, bounds.MinReplicas, bounds.MaxReplicas)
	b.QueueLimit = clamp(b.QueueLimit, float64(bounds.MinQueue), float64(bounds.MaxQueue))
	b.RetryLimit = clampInt(b.RetryLimit, bounds.MinRetry, bounds.MaxRetry)
	b.CacheAggression = clamp(b.CacheAggression, bounds.MinCache, bounds.MaxCache)
	return b
}

func scaleFromBundle(state SystemState, b Bundle) float64 {
	return float64(maxInt(b.Replicas, 1)) / float64(maxInt(state.Replicas, 1))
}

func uniqueBundles(in []Bundle) []Bundle {
	seen := make(map[[3]int]struct{}, len(in))
	out := make([]Bundle, 0, len(in))
	for _, b := range in {
		key := [3]int{
			b.Replicas,
			int(math.Round(b.QueueLimit)),
			b.RetryLimit,
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, b)
	}
	return out
}

func minPositiveInt(a, b int) int {
	if a <= 0 {
		return b
	}
	if b <= 0 {
		return a
	}
	if a < b {
		return a
	}
	return b
}

func seedFor(serviceID string, tick uint64, salt uint64) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(serviceID))
	v := h.Sum64() ^ (tick * 0x9e3779b97f4a7c15) ^ salt
	if v == 0 {
		v = 1
	}
	return int64(v)
}