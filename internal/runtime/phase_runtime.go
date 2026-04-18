package runtime

import (
	"fmt"
	"hash/fnv"
	"log"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/loadequilibrium/loadequilibrium/internal/autopilot"
	"github.com/loadequilibrium/loadequilibrium/internal/config"
	"github.com/loadequilibrium/loadequilibrium/internal/intelligence"
	"github.com/loadequilibrium/loadequilibrium/internal/modelling"
	"github.com/loadequilibrium/loadequilibrium/internal/optimisation"
	"github.com/loadequilibrium/loadequilibrium/internal/policy"
	"github.com/loadequilibrium/loadequilibrium/internal/sandbox"
)

type phaseServiceRuntime struct {
	policyState    policy.EngineState
	autopilot      *autopilot.RuntimeOrchestrator
	autopilotState autopilot.RuntimeState
	pgPolicy       *intelligence.PGRuntimePolicy
	adapter        *intelligence.AutonomyControlAdapter
	lastSandbox    sandbox.ExperimentOutput
}

type phaseRuntime struct {
	cfg                    *config.Config
	services               map[string]*phaseServiceRuntime
	controlMu              sync.RWMutex
	policyPreset           string
	forceSandboxUntil      uint64
	forceIntelligenceUntil uint64
}

func newPhaseRuntime(cfg *config.Config) *phaseRuntime {
	return &phaseRuntime{
		cfg:          cfg,
		services:     make(map[string]*phaseServiceRuntime),
		policyPreset: "balanced",
	}
}

func (p *phaseRuntime) SetPolicyPreset(preset string) string {
	if p == nil {
		return "balanced"
	}
	normalised := normalisePolicyPreset(preset)
	p.controlMu.Lock()
	defer p.controlMu.Unlock()
	p.policyPreset = normalised
	return normalised
}

func (p *phaseRuntime) PolicyPreset() string {
	if p == nil {
		return "balanced"
	}
	p.controlMu.RLock()
	defer p.controlMu.RUnlock()
	if p.policyPreset == "" {
		return "balanced"
	}
	return p.policyPreset
}

func (p *phaseRuntime) ForceSandboxUntil(untilTick uint64) {
	if p == nil {
		return
	}
	p.controlMu.Lock()
	defer p.controlMu.Unlock()
	p.forceSandboxUntil = untilTick
}

func (p *phaseRuntime) ForcedSandboxUntil() uint64 {
	if p == nil {
		return 0
	}
	p.controlMu.RLock()
	defer p.controlMu.RUnlock()
	return p.forceSandboxUntil
}

func (p *phaseRuntime) ForceIntelligenceUntil(untilTick uint64) {
	if p == nil {
		return
	}
	p.controlMu.Lock()
	defer p.controlMu.Unlock()
	p.forceIntelligenceUntil = untilTick
}

func (p *phaseRuntime) ForcedIntelligenceUntil() uint64 {
	if p == nil {
		return 0
	}
	p.controlMu.RLock()
	defer p.controlMu.RUnlock()
	return p.forceIntelligenceUntil
}

func (p *phaseRuntime) sandboxForced(tick uint64) bool {
	return p != nil && p.ForcedSandboxUntil() > 0 && tick <= p.ForcedSandboxUntil()
}

func (p *phaseRuntime) intelligenceForced(tick uint64) bool {
	return p != nil && p.ForcedIntelligenceUntil() > 0 && tick <= p.ForcedIntelligenceUntil()
}

func normalisePolicyPreset(preset string) string {
	switch strings.ToLower(strings.TrimSpace(preset)) {
	case "latency", "fast":
		return "latency"
	case "stability", "conservative", "safe":
		return "stability"
	case "cost", "efficient":
		return "cost"
	case "aggressive":
		return "aggressive"
	default:
		return "balanced"
	}
}

func (p *phaseRuntime) apply(
	tick uint64,
	now time.Time,
	bundles map[string]*modelling.ServiceModelBundle,
	objective optimisation.ObjectiveScore,
	directives map[string]optimisation.ControlDirective,
) map[string]optimisation.ControlDirective {
	if p == nil {
		return directives
	}

	out := make(map[string]optimisation.ControlDirective, len(bundles))
	for id, directive := range directives {
		out[id] = directive
	}

	active := make(map[string]struct{}, len(bundles))

	for id, bundle := range bundles {
		active[id] = struct{}{}

		service := p.ensureService(id)
		base := out[id]
		if base.ServiceID == "" {
			base = optimisation.ControlDirective{
				ServiceID:         id,
				ComputedAt:        now,
				ScaleFactor:       1,
				TargetUtilisation: p.cfg.UtilisationSetpoint,
				Active:            true,
			}
		}

		phase1 := p.evaluatePolicy(bundle, base, &service.policyState)
		phase2 := p.recommend(bundle, base, phase1, objective)

		// Grab last known sandbox risk for infra state before RL / Autopilot
		lastSimRisk := 0.0
		if service.lastSandbox.Meta.HorizonID != "" {
			lastSimRisk = service.lastSandbox.Meta.UnifiedRisk
		}

		infra := intelligence.InfraState{
			QueueDepth:       bundle.Queue.MeanQueueLen,
			LatencyP95:       bundle.Queue.AdjustedWaitMs,
			CPUUsage:         phaseClamp(bundle.Queue.Utilisation, 0, 1.5),
			RetryRate:        phaseClamp(float64(phase1.Retry.RetryLimit)/phaseMax(float64(p.currentRetry(bundle)), 1), 0, 4),
			CapacityPressure: phaseClamp(phaseMax(phase2.RiskScore, lastSimRisk), 0, 1.5),
			SLASeverity:      p.slaSeverity(bundle, objective),
			PerfScore:        phaseClamp(1-objective.CompositeScore, 0, 1),
		}
		if p.intelligenceForced(tick) {
			infra.CapacityPressure = phaseMax(infra.CapacityPressure, 1.0)
			infra.SLASeverity = phaseMax(infra.SLASeverity, 1.0)
		}

		// STEP 4: Relocate RL execution before Autopilot
		intel := service.adapter.Step(infra)

		autoState, autoTel := p.runAutopilot(service, bundle, base, phase1, phase2, intel)
		service.autopilotState = autoState

		simAdvice := service.lastSandbox.Advice
		if p.sandboxForced(tick) || (tick%10 == 0 && p.shouldRunSandbox(bundle, phase1, phase2, objective)) {
			if outSim, err := p.runSandbox(id, tick, bundle, base, phase1, phase2, autoTel); err == nil {
				service.lastSandbox = outSim
				simAdvice = outSim.Advice
			} else {
				log.Printf("[phase4] sandbox skipped service=%s err=%v", id, err)
			}
		}

		out[id] = p.mergeDirective(base, bundle, phase1, phase2, simAdvice, autoTel, intel, objective)
	}

	for id := range p.services {
		if _, ok := active[id]; !ok {
			delete(p.services, id)
		}
	}

	return out
}

func (p *phaseRuntime) ensureService(id string) *phaseServiceRuntime {
	if svc, ok := p.services[id]; ok {
		return svc
	}

	tickSec := phaseMax(p.cfg.TickInterval.Seconds(), 0.1)
	actDim := 4

	predictor := &autopilot.Predictor{
		Dt:                       tickSec,
		MaxQueue:                 5000,
		BurstEntryRate:           0.10,
		BurstCollapseThreshold:   20,
		BurstIntensity:           0.35,
		ArrivalRiseGain:          0.25,
		ArrivalDropGain:          0.12,
		VarianceDecayRate:        0.10,
		RetryGain:                0.30,
		RetryDelayTau:            1.5,
		DisturbanceSigma:         0.05,
		DisturbanceInjectionGain: 0.02,
		DisturbanceBound:         0.40,
		TopologyCouplingK:        0.35,
		TopologyAdaptTau:         2.0,
		CacheAdaptTau:            2.0,
		LatencyGain:              0.50,
		CapacityJitterSigma:      0.04,
		BarrierExpK:              0.005,
		BarrierCap:               10000,
	}

	mpc := &autopilot.MPCOptimiser{
		Horizon:       phaseMaxInt(p.cfg.PredictiveHorizonTicks, 4),
		Dt:            tickSec,
		ScenarioCount: 4,
		BurstProb:     0.20,
		BacklogCost:   1.0,
		LatencyCost:   0.5,
		VarianceBase:  0.2,
		ScalingCost:   0.1,
		SmoothCost:    0.1,
		TerminalCost:  0.3,
		UtilCost:      0.2,
		SafetyBarrier: 0.15,
		RiskQuantile:  0.75,
		RiskWeight:    0.4,
		MaxCapacity:   3.0,
		MinCapacity:   0.5,
		MaxStepCap:    0.5,
		MaxStepRetry:  0.4,
		MaxStepCache:  0.3,
		InitTemp:      1.0,
		Cooling:       0.95,
		Iters:         40, // minimum for SA to explore meaningfully; effectiveIters() can scale this up
	}

	safety := &autopilot.SafetyEngine{
		BaseMaxBacklog:     2000,
		BaseMaxLatency:     2500,
		Alpha:              0.4,
		Beta:               0.2,
		ArrivalGain:        0.01,
		DisturbanceGain:    0.2,
		TopologyGain:       0.2,
		RetryGain:          0.1,
		TailRiskBase:       0.15,
		AccelBaseWindow:    3,
		AccelThreshold:     0.2,
		MaxCapacityRamp:    1.0,
		CapacityEffectTau:  1.0,
		TopologyDelayTau:   1.0,
		TerminalEnergyBase: 1e6,
		ContractionSlack:   0.2,
		HysteresisBand:     0.05,
	}

	rollout := &autopilot.RolloutController{
		Dt:                    tickSec,
		CapRampUpNormal:       0.5,
		CapRampUpEmergency:    0.9,
		CapRampDown:           0.4,
		RetryEnableRamp:       0.5,
		RetryDisableRamp:      0.3,
		CacheEnableRamp:       0.4,
		CacheDisableRamp:      0.3,
		WarmupTau:             1.0,
		ConfigLagTau:          2.0,
		QueueMax:              16,
		QueuePressureRampGain: 0.5,
		EmergencyBacklog:      300,
		DegradedBacklog:       150,
		RolloutTimeout:        2 * tickSec,
		MaxRetries:            3,
		SuccessProbBase:       0.95,
		InfraFailureGain:      0.4,
	}

	ident := &autopilot.IdentificationEngine{
		Dt:                  tickSec,
		FastGain:            0.35,
		SlowGain:            0.10,
		BlendGain:           0.10,
		VarGain:             0.10,
		BurstGain:           0.10,
		BurstDecay:          0.05,
		BurstCap:            3,
		NoiseGain:           0.20,
		DriftGain:           0.05,
		BaseConfidenceFloor: 0.20,
		ConfidenceGain:      0.15,
		ReliabilityGain:     0.10,
		InfraSensitivity:    0.5,
		SLAWeightQueue:      0.5,
		SLAWeightLatency:    0.5,
		EVTFactor:           2.0,
		SeasonalGain:        0.05,
		DampingGain:         0.10,
	}

	autoRuntime := &autopilot.RuntimeOrchestrator{
		Dt:                tickSec,
		Predictor:         predictor,
		MPC:               mpc,
		Safety:            safety,
		Rollout:           rollout,
		ID:                ident,
		SLA_Backlog:       100,
		OverrideWindow:    16,
		DampingMin:        1.0,
		DampingMax:        3.0,
		FailureScaleProb:  0.02,
		FailureConfigProb: 0.01,
		TelemetryTau:      2 * tickSec,
	}

	learner := intelligence.NewAdaptiveSignalAdapter(intelligence.NewAdaptiveSignalLearner(6))
	roll := intelligence.NewPredictiveStabilityRollout(4, actDim)
	hazard := intelligence.NewHazardValueCritic(8)
	pgPolicy := intelligence.NewPGRuntimePolicy(intelligence.NewPolicyGradientOptimizer(4))
	rt := intelligence.NewIntelligenceRuntime(intelligence.RuntimeModules{
		Meta:    intelligence.NewMetaAutonomyController(),
		Safety:  intelligence.NewSafetyConstraintProjector(actDim),
		Rollout: roll,
		Hazard:  hazard,
		Fusion:  intelligence.NewAutonomyDecisionFusion(actDim),
		Learner: learner,
		Trainer: pgPolicy,
	}, actDim)

	orc := intelligence.NewAutonomyOrchestrator(
		rt,
		intelligence.NewAutonomyTelemetryModel(),
		intelligence.NewSafetyConstraintProjector(actDim),
		roll,
		hazard,
		actDim,
	)

	adapter := intelligence.NewAutonomyControlAdapter(orc, roll, actDim)
	adapter.BindPolicy(pgPolicy.Policy)

	service := &phaseServiceRuntime{
		autopilot: autoRuntime,
		pgPolicy:  pgPolicy,
		adapter:   adapter,
	}
	p.services[id] = service
	return service
}

func (p *phaseRuntime) policyWeights() policy.CostWeights {
	switch p.PolicyPreset() {
	case "latency":
		return policy.CostWeights{
			SlaViolation: 1.45,
			InfraCost:    0.20,
			ChangeCost:   0.10,
			RiskCost:     0.85,
			FutureCost:   0.35,
		}
	case "stability":
		return policy.CostWeights{
			SlaViolation: 1.10,
			InfraCost:    0.20,
			ChangeCost:   0.25,
			RiskCost:     1.25,
			FutureCost:   0.30,
		}
	case "cost":
		return policy.CostWeights{
			SlaViolation: 0.85,
			InfraCost:    0.60,
			ChangeCost:   0.30,
			RiskCost:     0.55,
			FutureCost:   0.15,
		}
	case "aggressive":
		return policy.CostWeights{
			SlaViolation: 1.20,
			InfraCost:    0.15,
			ChangeCost:   0.05,
			RiskCost:     0.95,
			FutureCost:   0.40,
		}
	default:
		return policy.CostWeights{
			SlaViolation: 1.0,
			InfraCost:    0.25,
			ChangeCost:   0.15,
			RiskCost:     0.75,
			FutureCost:   0.20,
		}
	}
}

func (p *phaseRuntime) evaluatePolicy(
	bundle *modelling.ServiceModelBundle,
	base optimisation.ControlDirective,
	state *policy.EngineState,
) policy.EngineDecision {
	currentReplicas := p.currentReplicas(bundle)
	currentRetry := p.currentRetry(bundle)
	currentQueue := phaseMax(bundle.Queue.MeanQueueLen+1, 1)
	weights := p.policyWeights()

	input := policy.EngineInput{
		Scaling: policy.ScalingSignal{
			PredictedLoad:     phaseMax(bundle.Queue.ArrivalRate, 0.1),
			CurrentReplicas:   currentReplicas,
			TargetLatency:     500,
			ObservedLatency:   phaseMax(bundle.Queue.AdjustedWaitMs, 1),
			MinReplicas:       1,
			MaxReplicas:       phaseMaxInt(currentReplicas+4, 4),
			ScaleCooldownCost: 0.1,
			InstanceCost:      1.0,
			SlaPenaltyWeight:  1.0,
		},
		Retry: policy.RetrySignal{
			ObservedErrorRate: bundle.Stochastic.RiskPropagation,
			TimeoutRate:       phaseClamp(bundle.Queue.AdjustedWaitMs/1000.0, 0, 1),
			QueueDepth:        bundle.Queue.MeanQueueLen,
			PredictedArrival:  bundle.Queue.ArrivalRate,
			ServiceCapacity:   phaseMax(bundle.Queue.ServiceRate, 0.1),
			BaseSystemRisk:    bundle.Stability.CollapseRisk,
			CurrentRetryLimit: currentRetry,
			MinRetryLimit:     1,
			MaxRetryLimit:     phaseMaxInt(currentRetry+3, 3),
			MinBackoff:        1.0,
			MaxBackoff:        3.0,
		},
		Queue: policy.QueueSignal{
			CurrentQueueDepth: bundle.Queue.MeanQueueLen,
			PredictedArrival:  bundle.Queue.ArrivalRate,
			ServiceCapacity:   phaseMax(bundle.Queue.ServiceRate, 0.1),
			ObservedLatency:   phaseMax(bundle.Queue.AdjustedWaitMs, 1),
			TargetLatency:     500,
			BaseSystemRisk:    bundle.Stability.CollapseRisk,
			CurrentQueueLimit: currentQueue,
			MinQueueLimit:     1,
			MaxQueueLimit:     phaseMax(currentQueue*2, 10),
			MaxStep:           phaseMax(currentQueue*0.25, 1),
		},
		Cache: policy.CacheSignal{
			CurrentHitRate:         phaseClamp(1-bundle.Stochastic.RiskPropagation, 0, 1),
			TargetHitRate:          0.85,
			PredictedArrival:       bundle.Queue.ArrivalRate,
			ServiceCapacity:        phaseMax(bundle.Queue.ServiceRate, 0.1),
			ObservedLatency:        phaseMax(bundle.Queue.AdjustedWaitMs, 1),
			TargetLatency:          500,
			CacheableRatio:         0.6,
			BaseMemoryPressure:     phaseClamp(bundle.Queue.Utilisation, 0, 1),
			BaseSystemRisk:         bundle.Stability.CollapseRisk,
			CurrentCacheAggression: phaseClamp(1-base.ScaleFactor/3.0, 0, 1),
			MinAggression:          0,
			MaxAggression:          1,
			BaseStep:               0.1,
		},
		CostWeights: weights,
	}

	return policy.EvaluatePolicies(input, state)
}

func (p *phaseRuntime) recommend(
	bundle *modelling.ServiceModelBundle,
	base optimisation.ControlDirective,
	phase1 policy.EngineDecision,
	objective optimisation.ObjectiveScore,
) sandbox.PolicyRecommendation {
	comp := sandbox.ComparisonResult{
		CostScore:          1 / (1 + phase1.GlobalCost),
		UtilityScore:       phaseClamp(1-bundle.Stability.CollapseRisk, 0, 1),
		CollapseEnergy:     bundle.Stability.CollapseRisk * (1 + bundle.Stability.OscillationRisk),
		InteractionRisk:    phaseClamp(bundle.Queue.UpstreamPressure+bundle.Stability.FeedbackGain, 0, 2),
		GlobalScore:        phaseClamp(phase1.Confidence-objective.CompositeScore, -1, 1),
		Stable:             !bundle.Stability.IsUnstable,
		SafetyMarg:         bundle.Stability.StabilityMargin,
		Confidence:         phaseClamp(phase1.Confidence, 0, 1),
		TemporalRobustness: 1 / (1 + math.Abs(bundle.Stability.StabilityDerivative)),
	}

	return sandbox.RecommendPolicy(
		comp,
		sandbox.RecommendationSignals{
			ThroughputMargin: bundle.Queue.ServiceRate - bundle.Queue.ArrivalRate,
			CostGradient:     base.CostGradient,
			DegradationRate:  bundle.Stability.StabilityDerivative,
		},
		sandbox.RecommendationConfig{
			CapacityGain:      0.40,
			EfficiencyGain:    0.25,
			DampingGain:       0.20,
			RetryGain:         0.20,
			BrownoutGain:      0.15,
			RiskCollapseW:     0.55,
			RiskInteractW:     0.30,
			RiskViabilityW:    0.15,
			SLA_CollapseRef:   phaseMax(p.cfg.CollapseThreshold, 0.1),
			SLA_InteractRef:   1.0,
			SLA_MinThroughput: phaseMax(bundle.Queue.ServiceRate*0.75, 0.1),
			RiskThreshold:     0.65,
			TrendGain:         0.75,
			SoftmaxTemp:       1.0,
		},
	)
}

func (p *phaseRuntime) runAutopilot(
	service *phaseServiceRuntime,
	bundle *modelling.ServiceModelBundle,
	base optimisation.ControlDirective,
	phase1 policy.EngineDecision,
	phase2 sandbox.PolicyRecommendation,
	intel intelligence.MPCWeighting,
) (autopilot.RuntimeState, autopilot.RuntimeTelemetry) {
	state := service.autopilotState

	// STEP 2: Wire Policy as Hard MPC Bounds
	// The policy heuristic is securely converted into a mathematical boundary
	// constrained within the MPC search engine, preserving MPC actuation authority.
	currentReplicas := float64(p.currentReplicas(bundle))
	if service.autopilot != nil && service.autopilot.MPC != nil && currentReplicas > 0 {
		service.autopilot.MPC.MinCapacity = float64(phase1.Scaling.MinReplicas) / currentReplicas
		service.autopilot.MPC.MaxCapacity = float64(phase1.Scaling.MaxReplicas) / currentReplicas
	}

	// STEP 3: Wire Sandbox as MPC Risk Barrier
	// Maps the asynchronous simulated collapse probability into the core MPC
	// objective formulation, forcing the optimizer to defensively penalize
	// paths that the Sandbox flagged as probabilistically unsafe.
	if service.autopilot != nil && service.autopilot.MPC != nil {
		combinedRisk := phaseMax(phase2.RiskScore, service.lastSandbox.Meta.UnifiedRisk)
		service.autopilot.MPC.SafetyBarrier = 0.15 + 2.0*combinedRisk
		service.autopilot.MPC.RiskWeight = 0.4 + 1.5*combinedRisk

		// STEP 4: Wire RL as Cost Tuning Modifier
		service.autopilot.MPC.RiskWeight += intel.RiskWeight
		service.autopilot.MPC.SmoothCost = phaseMax(0.01, service.autopilot.MPC.SmoothCost+intel.SmoothCost)
	}

	if state.Rollout.CapacityActive == 0 {
		state.Rollout.CapacityActive = phaseMax(base.ScaleFactor, 0.5)
		state.ID.ModelConfidence = phaseClamp(phase1.Confidence, 0.2, 1)
	}

	state.Plant = autopilot.CongestionState{
		Backlog:               bundle.Queue.MeanQueueLen,
		ArrivalMean:           bundle.Queue.ArrivalRate,
		ArrivalVar:            phaseMax(bundle.Stochastic.ArrivalCoV, 0.01),
		ServiceRate:           phaseMax(bundle.Queue.ServiceRate, 0.1),
		ServiceEfficiency:     phaseClamp(1-bundle.Stability.CollapseRisk, 0.1, 1),
		ConcurrencyLimit:      phaseMax(bundle.Queue.Concurrency, 1),
		CapacityActive:        phaseMax(state.Rollout.CapacityActive, 0.5),
		CapacityTarget:        phaseMax(base.ScaleFactor+0.15*phase2.CapacityDelta, 0.5),
		CapacityTauUp:         1.0,
		CapacityTauDown:       1.0,
		RetryFactor:           phaseClamp(float64(phase1.Retry.RetryLimit)/phaseMax(float64(p.currentRetry(bundle)), 1), 0, 3),
		Latency:               phaseMax(bundle.Queue.AdjustedWaitMs/100.0, 0),
		CPUPressure:           phaseClamp(bundle.Queue.Utilisation, 0, 1.5),
		NetworkJitter:         bundle.Stability.OscillationRisk,
		UpstreamPressure:      phaseClamp(bundle.Queue.UpstreamPressure, 0, 1.5),
		TopologyAmplification: phaseMax(1+bundle.Stability.FeedbackGain, 1),
		CacheRelief:           phaseClamp(phase1.Cache.CacheAggression, 0, 1),
	}

	state.ID.ArrivalEstimate = bundle.Queue.ArrivalRate
	state.ID.ArrivalVar = phaseMax(bundle.Stochastic.ArrivalCoV, 0.01)
	state.ID.ModelConfidence = phaseClamp(phase1.Confidence, 0.1, 1)

	next, telemetry := service.autopilot.Tick(
		state,
		bundle.Queue.ArrivalRate,
		phaseClamp(phase2.RiskScore+bundle.Queue.UpstreamPressure, 0, 1),
	)

	return next, telemetry
}

func (p *phaseRuntime) shouldRunSandbox(
	bundle *modelling.ServiceModelBundle,
	phase1 policy.EngineDecision,
	phase2 sandbox.PolicyRecommendation,
	objective optimisation.ObjectiveScore,
) bool {
	return objective.CompositeScore > 0.55 ||
		phase1.GlobalRisk > 0.60 ||
		phase2.RiskUp ||
		bundle.Stability.CollapseRisk > 0.50
}

func (p *phaseRuntime) runSandbox(
	serviceID string,
	tick uint64,
	bundle *modelling.ServiceModelBundle,
	base optimisation.ControlDirective,
	phase1 policy.EngineDecision,
	phase2 sandbox.PolicyRecommendation,
	auto autopilot.RuntimeTelemetry,
) (sandbox.ExperimentOutput, error) {
	seed := int64(p.hashService(serviceID, tick))
	scenarioCfg := sandbox.ScenarioConfig{
		BaseArrival:   phaseMax(bundle.Queue.ArrivalRate, 0.1),
		BaseService:   phaseMax(bundle.Queue.ServiceRate, 0.1),
		Duration:      6 * time.Second,
		Step:          time.Second,
		Seed:          seed,
		NoiseStd:      phaseClamp(bundle.Stochastic.ArrivalCoV, 0.01, 0.2),
		ARCoef:        0.6,
		RetryGain:     0.4 + 0.2*phaseClamp(float64(phase1.Retry.RetryLimit), 0, 3),
		RetryDecay:    0.6,
		RetryJitter:   0.15,
		SaturationCap: phaseMax(bundle.Queue.MeanQueueLen+10, 10),
		BurstOnProb:   0.10 + 0.10*bundle.Stochastic.BurstAmplification,
		BurstOffProb:  0.50,
		ParetoAlpha:   1.5,
		BurstCeiling:  3.0,
		HeavyTailProb: 0.30,
		Harmonics:     []float64{0.20, 0.10},
		PhaseDrift:    0.01,
		ShockTimes:    []time.Duration{2 * time.Second, 4 * time.Second},
		ShockMag:      0.2 + 0.4*phase2.RiskScore,
		RelaxTau:      2.0,
		CapacityDrop:  0.10 + 0.20*bundle.Stability.CollapseRisk,
		CollapseProb:  0.05 + 0.10*bundle.Stability.CollapseRisk,
		RateLimit:     phaseMax(bundle.Queue.ServiceRate*1.2, 1),
		ShedProb:      0.1,
		BreakerThresh: phaseMax(bundle.Queue.MeanQueueLen*1.5, 5),
		FanoutBase:    1.0,
		FanoutLoad:    0.2,
		FanoutVar:     0.05,
	}

	scenario := sandbox.GenerateScenario(scenarioCfg, sandbox.ScenarioSpike)
	hash := fmt.Sprintf("%x", p.hashService(serviceID, tick^0x9e3779b97f4a7c15))

	experiment := sandbox.ExperimentScenario{
		BaseJobs: []sandbox.SimulationJob{
			{ID: serviceID + "-base-a", Scenario: scenario, PlantCfg: sandbox.PlantConfig{CapacityScale: 1}},
			{ID: serviceID + "-base-b", Scenario: scenario, PlantCfg: sandbox.PlantConfig{CapacityScale: 1.0 + 0.05*phaseClamp(bundle.Stochastic.ArrivalCoV, 0, 1)}},
		},
		CandJobs: []sandbox.SimulationJob{
			{
				ID:       serviceID + "-cand-a",
				Scenario: scenario,
				PlantCfg: sandbox.PlantConfig{
					CapacityScale: phaseClamp(base.ScaleFactor+0.20*phase2.CapacityDelta+0.05*auto.Capacity, 0.5, 2.5),
					RetryBias:     phaseClamp(phase2.RetryPressureDelta, -1, 1),
					CacheRelief:   phaseClamp(phase1.Cache.CacheAggression+phase2.EfficiencyDelta, 0, 1),
				},
			},
			{
				ID:       serviceID + "-cand-b",
				Scenario: scenario,
				PlantCfg: sandbox.PlantConfig{
					CapacityScale: phaseClamp(base.ScaleFactor+0.20*phase2.CapacityDelta, 0.5, 2.5),
					RetryBias:     phaseClamp(phase2.RetryPressureDelta, -1, 1),
					CacheRelief:   phaseClamp(phase1.Cache.CacheAggression+phase2.EfficiencyDelta, 0, 1),
				},
			},
		},
		BaseHash: hash,
		CandHash: hash,
	}

	return sandbox.RunSandboxExperiment(
		sandbox.ExperimentContext{
			Seed:      seed,
			HorizonID: fmt.Sprintf("%s-%d", serviceID, tick),
		},
		sandbox.ExperimentInput{
			Scenario: &experiment,
		},
		sandbox.ExperimentConfig{
			Weights: sandbox.ComparisonWeights{
				LatencyW:          0.3,
				TailW:             0.2,
				BacklogW:          0.2,
				OscW:              0.1,
				SettleW:           0.1,
				ThroughputW:       0.2,
				CollapseW:         0.4,
				InteractW:         0.2,
				SLA_TailLimit:     2.0,
				SLA_CollapseLimit: phaseMax(p.cfg.CollapseThreshold, 0.1),
				SLA_MinThroughput: phaseMax(bundle.Queue.ServiceRate*0.75, 0.1),
				Hysteresis:        0.05,
				Discount:          0.1,
			},
			RecCfg: sandbox.RecommendationConfig{
				CapacityGain:      0.40,
				EfficiencyGain:    0.25,
				DampingGain:       0.15,
				RetryGain:         0.20,
				BrownoutGain:      0.10,
				RiskCollapseW:     0.60,
				RiskInteractW:     0.25,
				RiskViabilityW:    0.15,
				SLA_CollapseRef:   phaseMax(p.cfg.CollapseThreshold, 0.1),
				SLA_InteractRef:   1.0,
				SLA_MinThroughput: phaseMax(bundle.Queue.ServiceRate*0.75, 0.1),
				RiskThreshold:     0.65,
				TrendGain:         0.70,
				SoftmaxTemp:       1.0,
			},
			RunSimulation: true,
			ExecCfg: sandbox.ExecutorConfig{
				InitWorkers:     1,
				MaxWorkers:      2,
				JobBuffer:       4,
				ResultBuffer:    4,
				ScalerInterval:  250 * time.Millisecond,
				EnableAutoscale: false,
				IdleTimeout:     500 * time.Millisecond,
				ReorderWindow:   8,
				FlushOnCancel:   true,
			},
		},
		sandbox.RecommendationSignals{
			ThroughputMargin: bundle.Queue.ServiceRate - bundle.Queue.ArrivalRate,
			CostGradient:     base.CostGradient,
			DegradationRate:  bundle.Stability.StabilityDerivative,
		},
	)
}

// computePrecisionFusedScale implements a MAP estimator that fuses
// multiple optimizer preferences into a single action with
// mathematically grounded authority assignment.
//
// Each optimizer i contributes:
//   - ui: preferred scale factor
//   - sigmai: uncertainty in that preference (state-derived, not fixed)
//
// The result u* = (sum ui*preci) / (sum preci) is the minimum-variance
// unbiased estimator when each optimizer's preference is modeled as a
// Gaussian measurement of the true optimal scale.
//
// Authority is earned by confidence, not assigned by hard-coded weight.
// At collapse risk=0: PID dominates (sigma_pid~=0). As risk->1: policy and
// sandbox gain authority because PID becomes uncertain.
func computePrecisionFusedScale(
	uPID, uPolicy, uMPC, uSandbox, uIntel float64,
	collapseRisk, policyConf, mpcConf, sandboxRisk float64,
) float64 {
	const eps = 1e-4 // numerical floor prevents division by zero

	// sigmai derived from system-state signals, not hardcoded.
	// Each maps a confidence/risk proxy to [eps, 1.0+eps].
	sigPID := eps + collapseRisk
	sigPolicy := eps + phaseClamp(1.0-policyConf, eps, 1.0)
	sigMPC := eps + phaseClamp(1.0-mpcConf, eps, 1.0)
	sigSandbox := eps + phaseClamp(sandboxRisk, eps, 1.0)
	sigIntel := eps + 0.75 // RL: fixed high uncertainty - it is always learning

	// Precision = 1/sigma^2 (higher precision = more authority)
	precPID := 1.0 / (sigPID * sigPID)
	precPolicy := 1.0 / (sigPolicy * sigPolicy)
	precMPC := 1.0 / (sigMPC * sigMPC)
	precSandbox := 1.0 / (sigSandbox * sigSandbox)
	precIntel := 1.0 / (sigIntel * sigIntel)

	totalPrec := precPID + precPolicy + precMPC + precSandbox + precIntel
	if totalPrec < eps {
		return phaseClamp(uPID, 0.45, 3.0) // degenerate guard: fallback to PID
	}

	u := (precPID*uPID + precPolicy*uPolicy + precMPC*uMPC +
		precSandbox*uSandbox + precIntel*uIntel) / totalPrec

	return phaseClamp(u, 0.45, 3.0)
}

func (p *phaseRuntime) mergeDirective(
	base optimisation.ControlDirective,
	bundle *modelling.ServiceModelBundle,
	phase1 policy.EngineDecision,
	phase2 sandbox.PolicyRecommendation,
	sim sandbox.PolicyRecommendation,
	auto autopilot.RuntimeTelemetry,
	intel intelligence.MPCWeighting,
	objective optimisation.ObjectiveScore,
) optimisation.ControlDirective {
	currentReplicas := p.currentReplicas(bundle)
	scaleFromPolicy := float64(phase1.Scaling.DesiredReplicas) / float64(currentReplicas)
	uPID := phaseClamp(base.ScaleFactor, 0.45, 3.0)
	uPolicy := phaseClamp(scaleFromPolicy, 0.45, 3.0)
	uMPC := phaseClamp(auto.Capacity, 0.45, 3.0)
	uSandbox := phaseClamp(1+phase2.CapacityDelta+sim.CapacityDelta, 0.45, 3.0)
	uIntel := phaseClamp(1, 0.45, 3.0) // RL Scale Action deactivated (Phase 4), now cost-tuned

	// MPC confidence proxy: lower OverrideRate -> MPC is not fighting the plant -> higher confidence.
	mpcConf := phaseClamp(1.0-auto.OverrideRate, 0, 1.0)
	// Sandbox risk: worst of the two sandbox passes.
	sandboxRisk := phaseMax(phase2.RiskScore, sim.RiskScore)

	scale := computePrecisionFusedScale(
		uPID, uPolicy, uMPC, uSandbox, uIntel,
		bundle.Stability.CollapseRisk,
		phase1.Confidence,
		mpcConf,
		sandboxRisk,
	)

	targetUtil := phaseClamp(
		p.cfg.UtilisationSetpoint-
			0.03*phase2.BrownoutDelta-
			0.02*sim.BrownoutDelta,
		0.45,
		0.90,
	)

	merged := base
	merged.ComputedAt = time.Now()
	merged.ServiceID = base.ServiceID
	if merged.ServiceID == "" {
		merged.ServiceID = bundle.Queue.ServiceID
	}
	merged.ScaleFactor = phaseClamp(scale, 0.45, 3.0)
	merged.TargetUtilisation = targetUtil
	merged.Active = true
	merged.CostGradient = phaseMax(
		base.CostGradient,
		phase1.GlobalRisk+phase2.RiskScore+sim.RiskScore,
	)
	merged.PredictiveTarget = phaseClamp(
		base.PredictiveTarget+0.10*objective.CompositeScore+0.05*auto.OverrideRate,
		0,
		1.5,
	)
	merged.PlannerScaleFactor = merged.ScaleFactor
	merged.PlannerProbabilisticScore = phaseClamp(
		phaseMax(phase2.RiskScore, sim.RiskScore),
		0,
		1,
	)
	return merged
}

func (p *phaseRuntime) currentReplicas(bundle *modelling.ServiceModelBundle) int {
	return phaseMaxInt(int(math.Round(phaseMax(bundle.Queue.ServiceRate, 1))), 1)
}

func (p *phaseRuntime) currentRetry(bundle *modelling.ServiceModelBundle) int {
	return phaseMaxInt(int(math.Round(1+bundle.Stochastic.BurstAmplification)), 1)
}

func (p *phaseRuntime) slaSeverity(
	bundle *modelling.ServiceModelBundle,
	objective optimisation.ObjectiveScore,
) float64 {
	ref := objective.ReferenceLatencyMs
	if ref <= 0 {
		ref = 500
	}
	return phaseClamp(bundle.Queue.AdjustedWaitMs/ref, 0, 2)
}

func (p *phaseRuntime) hashService(id string, tick uint64) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(fmt.Sprintf("%s-%d", id, tick)))
	return h.Sum64()
}

func phaseClamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func phaseMax(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func phaseMaxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
