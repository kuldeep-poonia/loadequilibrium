package reasoning

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/loadequilibrium/loadequilibrium/internal/modelling"
	"github.com/loadequilibrium/loadequilibrium/internal/optimisation"
	"github.com/loadequilibrium/loadequilibrium/internal/topology"
)

// Engine applies a deterministic, hysteresis-gated rule set to model outputs.
type Engine struct {
	cooldowns      map[string]time.Time
	cooldownOrder  []string
	cooldownPeriod time.Duration
	maxCooldowns   int
	// runtimePressure carries the orchestrator's pressure level (0–3).
	// Under high pressure the reasoning engine shortens cooldown periods so that
	// urgent events fire more frequently and operators see them sooner.
	runtimePressure int
}

// SetRuntimePressure injects the orchestrator's current pressure level.
// Must be called before Analyse/AnalyseWithContext each tick.
// level: 0=nominal, 1=elevated, 2=high, 3=critical.
func (e *Engine) SetRuntimePressure(level int) {
	e.runtimePressure = level
	// Adapt cooldown period to pressure:
	// nominal=30s, elevated=20s, high=10s, critical=5s
	switch level {
	case 1:
		e.cooldownPeriod = 20 * time.Second
	case 2:
		e.cooldownPeriod = 10 * time.Second
	case 3:
		e.cooldownPeriod = 5 * time.Second
	default:
		e.cooldownPeriod = 30 * time.Second
	}
}

func NewEngine() *Engine {
	return &Engine{
		cooldowns:      make(map[string]time.Time),
		cooldownOrder:  make([]string, 0, 64),
		cooldownPeriod: 30 * time.Second,
		maxCooldowns:   500,
	}
}

// SetMaxCooldowns configures the LRU cap on the cooldown map.
func (e *Engine) SetMaxCooldowns(n int) {
	if n > 0 {
		e.maxCooldowns = n
	}
}

// AnalyseWithContext runs the full reasoning analysis including network equilibrium
// and topology sensitivity signals for richer causal inference.
func (e *Engine) AnalyseWithContext(
	bundles map[string]*modelling.ServiceModelBundle,
	topo topology.GraphSnapshot,
	obj optimisation.ObjectiveScore,
	netEq modelling.NetworkEquilibriumState,
	topoSens modelling.TopologySensitivity,
	now time.Time,
) []Event {
	events := e.Analyse(bundles, topo, obj, now)

	// Rule: network equilibrium diverging with high saturation risk.
	if !netEq.IsConverging && netEq.NetworkSaturationRisk > 0.50 {
		if e.canFire("net:diverging", now) {
			events = append(events, Event{
				ID: newID(), Timestamp: now,
				Severity: SeverityWarning, Category: "EQUILIBRIUM",
				Description: fmt.Sprintf(
					"Network load diverging from equilibrium — saturation risk %.0f%%, δ=%.3f",
					netEq.NetworkSaturationRisk*100, netEq.EquilibriumDelta),
				Recommendation: "Inspect " + netEq.CriticalServiceID + " — highest equilibrium ρ in network",
				Evidence: Evidence{
					CascadeRisk: netEq.NetworkSaturationRisk,
					Utilisation: netEq.SystemRhoMean,
				},
				UncertaintyScore:    0.25,
				OperationalPriority: computePriority(SeverityWarning, netEq.SystemRhoMean, 0),
				ModelChain: fmt.Sprintf(
					"cause=upstream_pressure→model=bellman_ford_propagation→prediction=net_sat_risk=%.0f%%→action=inspect_%s",
					netEq.NetworkSaturationRisk*100, netEq.CriticalServiceID),
			})
		}
	}

	// Rule: high system fragility from topology sensitivity.
	if topoSens.SystemFragility > 0.65 {
		sev := SeverityWarning
		if topoSens.SystemFragility > 0.85 {
			sev = SeverityCritical
		}
		if e.canFire("topo:fragility", now) {
			keystones := topoSens.MaxAmplificationPath
			if len(keystones) > 3 {
				keystones = keystones[:3]
			}
			events = append(events, Event{
				ID: newID(), Timestamp: now,
				Severity: sev, Category: "TOPOLOGY",
				Description: fmt.Sprintf(
					"System topology fragility %.0f%% — amplification chain: %v",
					topoSens.SystemFragility*100, keystones),
				Recommendation: "Apply bulkheads and timeouts on amplification chain services",
				Evidence: Evidence{
					CascadeRisk: topoSens.SystemFragility,
				},
				UncertaintyScore:    0.20,
				OperationalPriority: computePriority(sev, topoSens.SystemFragility, 0),
				ModelChain: fmt.Sprintf(
					"cause=weight_topology→model=bellman_ford_amplification→prediction=fragility=%.0f%%→action=bulkhead",
					topoSens.SystemFragility*100),
			})
		}
	}

	// Rule: keystone service approaching warning zone.
	for id, b := range bundles {
		if ss, ok := topoSens.ByService[id]; ok && ss.IsKeystone {
			if b.Stability.CollapseRisk > 0.4 && b.Queue.Utilisation > 0.70 {
				if e.canFire(id+":keystone_risk", now) {
					events = append(events, Event{
						ID: newID(), Timestamp: now, ServiceID: id,
						Severity: SeverityWarning, Category: "TOPOLOGY",
						Description: fmt.Sprintf(
							"%s is a keystone service (reach=%d) at ρ=%.1f%% — blast radius is system-wide",
							id, ss.DownstreamReach, b.Queue.Utilisation*100),
						Recommendation: "Prioritise this service for scaling — downstream reach multiplies impact",
						Evidence: Evidence{
							Utilisation:  b.Queue.Utilisation,
							CollapseRisk: b.Stability.CollapseRisk,
						},
						UncertaintyScore:    1.0 - b.Queue.Confidence,
						OperationalPriority: computePriority(SeverityWarning, b.Queue.Utilisation, b.Queue.UtilisationTrend),
						ModelChain: fmt.Sprintf(
							"cause=keystone_load→model=perturbation_score(%.2f)→prediction=wide_cascade→action=prioritise_scale",
							ss.PerturbationScore),
					})
				}
			}
		}
	}

	sortBySeverity(events)
	return events
}

func (e *Engine) Analyse(
	bundles map[string]*modelling.ServiceModelBundle,
	topo topology.GraphSnapshot,
	obj optimisation.ObjectiveScore,
	now time.Time,
) []Event {
	var events []Event
	for _, b := range bundles {
		events = append(events, e.evalService(b, now)...)
	}
	events = append(events, e.evalTopology(topo, now)...)
	events = append(events, e.evalObjective(obj, now)...)
	sortBySeverity(events)
	return events
}

func (e *Engine) evalService(b *modelling.ServiceModelBundle, now time.Time) []Event {
	var events []Event
	id := b.Queue.ServiceID
	rho := b.Queue.Utilisation

	// Uncertainty: inverse of model confidence, boosted when CUSUM detects instability.
	// Base: 1 - model confidence. Signal instability (high CUSUM) adds up to 0.2 more.
	baseUncertainty := 1.0 - b.Queue.Confidence
	cusumPressure := math.Min((b.Signal.CUSUMPos+b.Signal.CUSUMNeg)/20.0, 0.2)
	uncertainty := math.Min(baseUncertainty+cusumPressure, 1.0)

	// Utilisation bands.
	if rho >= 0.95 {
		if e.canFire(id+":util:crit", now) {
			events = append(events, Event{
				ID: newID(), Timestamp: now, ServiceID: id,
				Severity: SeverityCritical, Category: "UTILISATION",
				Description:     fmt.Sprintf("%s at %.1f%% utilisation — saturation imminent", id, rho*100),
				Recommendation:  "Scale out immediately; activate circuit breaker",
				Evidence:        Evidence{Utilisation: rho, StabilityMargin: b.Stability.StabilityMargin},
				UncertaintyScore: uncertainty,
				OperationalPriority: computePriority(SeverityCritical, rho, b.Queue.UtilisationTrend),
				ModelChain: fmt.Sprintf("cause=high_arrival→model=M/M/c(ρ=%.2f)→prediction=queue_diverge→action=scale_out", rho),
			})
		}
	} else if rho >= 0.80 {
		if e.canFire(id+":util:warn", now) {
			events = append(events, Event{
				ID: newID(), Timestamp: now, ServiceID: id,
				Severity: SeverityWarning, Category: "UTILISATION",
				Description:     fmt.Sprintf("%s at %.1f%% — approaching saturation", id, rho*100),
				Recommendation:  "Prepare scaling; review upstream rate limits",
				Evidence:        Evidence{Utilisation: rho},
				UncertaintyScore: uncertainty,
				OperationalPriority: computePriority(SeverityWarning, rho, b.Queue.UtilisationTrend),
				ModelChain: fmt.Sprintf("cause=rising_load→model=M/M/c(ρ=%.2f)→prediction=saturation_risk→action=plan_scale", rho),
			})
		}
	}

	// Saturation horizon.
	if b.Queue.SaturationHorizon > 0 && b.Queue.SaturationHorizon < 5*time.Minute {
		secs := b.Queue.SaturationHorizon.Seconds()
		sev := SeverityWarning
		if secs < 60 {
			sev = SeverityCritical
		}
		if e.canFire(id+":horizon", now) {
			events = append(events, Event{
				ID: newID(), Timestamp: now, ServiceID: id,
				Severity: sev, Category: "SATURATION",
				Description:     fmt.Sprintf("%s saturates in %.0fs at current trend (uncertainty=%.0f%%)", id, secs, uncertainty*100),
				Recommendation:  "Intervene before queue becomes unbounded",
				Evidence:        Evidence{Utilisation: rho, SaturationSec: secs},
				UncertaintyScore: uncertainty,
				OperationalPriority: computePriority(sev, rho, b.Queue.UtilisationTrend),
				ModelChain: fmt.Sprintf("cause=load_trend→model=linear_extrapolation→prediction=saturation_in_%.0fs→action=preempt", secs),
			})
		}
	}

	// Trend-adjusted margin negative: projected rho exceeds 1.0 within MPC horizon.
	// This is more conservative than the saturation horizon (which uses local trend only).
	if b.Stability.TrendAdjustedMargin < 0 {
		if e.canFire(id+":tam_negative", now) {
			projectedRho := rho - b.Stability.TrendAdjustedMargin // TrendAdjustedMargin = 1 - projectedRho
			events = append(events, Event{
				ID: newID(), Timestamp: now, ServiceID: id,
				Severity: SeverityCritical, Category: "SATURATION",
				Description:     fmt.Sprintf("%s projected ρ=%.2f within 20s (trend-adjusted margin=%.3f)", id, projectedRho, b.Stability.TrendAdjustedMargin),
				Recommendation:  "Immediate capacity intervention required — trend extrapolation crosses ρ=1",
				Evidence:        Evidence{Utilisation: rho, SaturationSec: 20},
				UncertaintyScore: uncertainty * 0.8, // trend-adjusted is more certain
				OperationalPriority: 9,
				ModelChain: fmt.Sprintf("cause=trend_projection→model=linear_forward_20s→prediction=rho=%.2f→action=immediate_scale", projectedRho),
			})
		}
	}

	// Stability derivative acceleration: risk growing rapidly even if absolute risk is moderate.
	if b.Stability.StabilityDerivative > 0.03 && rho > 0.6 {
		if e.canFire(id+":risk_accel", now) {
			events = append(events, Event{
				ID: newID(), Timestamp: now, ServiceID: id,
				Severity: SeverityWarning, Category: "STABILITY",
				Description:     fmt.Sprintf("%s collapse risk accelerating at %.3f/s — load trend amplifying instability", id, b.Stability.StabilityDerivative),
				Recommendation:  "Pre-emptive intervention while risk is still manageable; trend will compound",
				Evidence:        Evidence{Utilisation: rho, CollapseRisk: b.Stability.CollapseRisk},
				UncertaintyScore: uncertainty,
				OperationalPriority: computePriority(SeverityWarning, rho, b.Queue.UtilisationTrend),
				ModelChain: fmt.Sprintf("cause=load_trend→model=d(sigmoid)/dt→prediction=risk_accel=%.3f/s→action=preempt", b.Stability.StabilityDerivative),
			})
		}
	}

	// Network saturation horizon (coupled upstream pressure).
	if b.Queue.NetworkSaturationHorizon > 0 &&
		b.Queue.NetworkSaturationHorizon < b.Queue.SaturationHorizon &&
		b.Queue.UpstreamPressure > 0.2 {
		netSecs := b.Queue.NetworkSaturationHorizon.Seconds()
		if e.canFire(id+":net_horizon", now) {
			events = append(events, Event{
				ID: newID(), Timestamp: now, ServiceID: id,
				Severity: SeverityWarning, Category: "SATURATION",
				Description:    fmt.Sprintf("%s coupled saturation in %.0fs (upstream pressure=%.0f%%)", id, netSecs, b.Queue.UpstreamPressure*100),
				Recommendation: "Inspect upstream call rates; apply backpressure at ingress",
				Evidence:       Evidence{Utilisation: rho, SaturationSec: netSecs},
				UncertaintyScore: uncertainty,
				OperationalPriority: computePriority(SeverityWarning, rho, b.Queue.UtilisationTrend),
				ModelChain: fmt.Sprintf("cause=upstream_pressure(%.0f%%)→model=coupled_queue→prediction=net_saturation_in_%.0fs→action=backpressure", b.Queue.UpstreamPressure*100, netSecs),
			})
		}
	}

	// Adjusted queue wait.
	adjWait := b.Queue.AdjustedWaitMs
	if !math.IsInf(adjWait, 0) && !math.IsNaN(adjWait) && adjWait > 500 {
		sev := SeverityWarning
		if adjWait > 2000 {
			sev = SeverityCritical
		}
		if e.canFire(id+":wait", now) {
			events = append(events, Event{
				ID: newID(), Timestamp: now, ServiceID: id,
				Severity: sev, Category: "LATENCY",
				Description:     fmt.Sprintf("%s queue wait %.0fms (burst×%.2f)", id, adjWait, b.Queue.BurstFactor),
				Recommendation:  "Apply token-bucket rate limiting upstream; reduce burst factor",
				Evidence:        Evidence{QueueWaitMs: adjWait, BurstFactor: b.Queue.BurstFactor, Utilisation: rho},
				UncertaintyScore: uncertainty,
				OperationalPriority: computePriority(sev, rho, b.Queue.UtilisationTrend),
				ModelChain: fmt.Sprintf("cause=burst_arrival→model=M/G/1_PKC(CoV²)→prediction=wait=%.0fms→action=rate_limit", adjWait),
			})
		}
	}

	// Collapse risk.
	if b.Stability.CollapseRisk > 0.85 {
		if e.canFire(id+":collapse", now) {
			events = append(events, Event{
				ID: newID(), Timestamp: now, ServiceID: id,
				Severity: SeverityCritical, Category: "STABILITY",
				Description:     fmt.Sprintf("%s collapse risk %.0f%% [zone=%s] — nonlinear saturation region", id, b.Stability.CollapseRisk*100, b.Stability.CollapseZone),
				Recommendation:  "Enable circuit breaker; shed non-critical traffic; horizontal scale",
				Evidence:        Evidence{CollapseRisk: b.Stability.CollapseRisk, Utilisation: rho, StabilityMargin: b.Stability.StabilityMargin},
				UncertaintyScore: uncertainty,
				OperationalPriority: computePriority(SeverityCritical, rho, b.Queue.UtilisationTrend),
				ModelChain: fmt.Sprintf("cause=high_rho→model=nonlinear_sigmoid→prediction=collapse_risk=%.0f%%→action=circuit_breaker", b.Stability.CollapseRisk*100),
			})
		}
	}

	// Cascade amplification — new signal.
	if b.Stability.CascadeAmplificationScore > 0.5 {
		if e.canFire(id+":cascade_amp", now) {
			events = append(events, Event{
				ID: newID(), Timestamp: now, ServiceID: id,
				Severity: SeverityWarning, Category: "CASCADE",
				Description:    fmt.Sprintf("%s cascade amplification %.2f — high downstream risk", id, b.Stability.CascadeAmplificationScore),
				Recommendation: "Implement bulkheads on downstream services; add timeout policies",
				Evidence:       Evidence{CascadeRisk: b.Stability.CascadeAmplificationScore, Utilisation: rho},
				UncertaintyScore: uncertainty,
				OperationalPriority: computePriority(SeverityWarning, rho, b.Queue.UtilisationTrend),
				ModelChain: fmt.Sprintf("cause=feedback_gain→model=cascade_amplification→prediction=downstream_overload→action=bulkhead"),
			})
		}
	}

	// Oscillation risk.
	if b.Stability.OscillationRisk > 0.6 && rho > 0.65 {
		if e.canFire(id+":oscillation", now) {
			events = append(events, Event{
				ID: newID(), Timestamp: now, ServiceID: id,
				Severity: SeverityWarning, Category: "STABILITY",
				Description:     fmt.Sprintf("%s oscillation risk %.0f%% — EWMA two-timescale divergence", id, b.Stability.OscillationRisk*100),
				Recommendation:  "Check for feedback loops; review auto-scaling hysteresis",
				Evidence:        Evidence{OscillationRisk: b.Stability.OscillationRisk, Utilisation: rho},
				UncertaintyScore: uncertainty,
				OperationalPriority: computePriority(SeverityWarning, rho, 0),
				ModelChain: "cause=ewma_divergence→model=two_timescale_rms→prediction=oscillation→action=hysteresis_review",
			})
		}
	}

	// Arrival spike.
	if b.Signal.SpikeDetected {
		if e.canFire(id+":spike", now) {
			events = append(events, Event{
				ID: newID(), Timestamp: now, ServiceID: id,
				Severity: SeverityWarning, Category: "ANOMALY",
				Description:      fmt.Sprintf("%s arrival spike detected (>3σ from EWMA)", id),
				Recommendation:   "Verify no retry storm; check upstream release or cron burst",
				Evidence:         Evidence{Utilisation: rho},
				UncertaintyScore: uncertainty,
				OperationalPriority: computePriority(SeverityWarning, rho, b.Queue.UtilisationTrend),
				ModelChain: "cause=sudden_arrival_jump→model=ewma_spike_z_score→prediction=transient_overload→action=investigate_source",
			})
		}
	}

	// Regime change.
	if b.Signal.ChangePointDetected {
		if e.canFire(id+":changepoint", now) {
			events = append(events, Event{
				ID: newID(), Timestamp: now, ServiceID: id,
				Severity: SeverityInfo, Category: "ANOMALY",
				Description:     fmt.Sprintf("%s CUSUM change-point: workload regime shift detected", id),
				Recommendation:  "Verify deployment, feature flag, or traffic source change",
				UncertaintyScore: uncertainty,
				OperationalPriority: 2,
				ModelChain: "cause=cusum_threshold_crossed→model=cusum_bilateral→prediction=regime_change→action=audit_traffic_source",
			})
		}
	}

	return events
}

func (e *Engine) evalTopology(topo topology.GraphSnapshot, now time.Time) []Event {
	var events []Event
	cp := topo.CriticalPath
	if len(cp.Nodes) < 2 || cp.CascadeRisk < 0.70 {
		return events
	}
	sev := SeverityWarning
	if cp.CascadeRisk > 0.90 {
		sev = SeverityCritical
	}
	key := fmt.Sprintf("topo:cascade:%d", len(cp.Nodes))
	if e.canFire(key, now) {
		events = append(events, Event{
			ID: newID(), Timestamp: now,
			Severity: sev, Category: "CASCADE",
			Description:     fmt.Sprintf("Critical path cascade risk %.0f%% across %d services", cp.CascadeRisk*100, len(cp.Nodes)),
			Recommendation:  "Isolate critical-path services; implement bulkheads and timeouts",
			Evidence:        Evidence{CascadeRisk: cp.CascadeRisk},
			UncertaintyScore: 0.2, // topology model is structural, lower uncertainty
			OperationalPriority: computePriority(sev, cp.CascadeRisk, 0),
			ModelChain: fmt.Sprintf("cause=critical_path_load→model=graph_cascade_sigmoid→prediction=cascade_risk=%.0f%%→action=bulkhead", cp.CascadeRisk*100),
		})
	}
	return events
}

func (e *Engine) evalObjective(obj optimisation.ObjectiveScore, now time.Time) []Event {
	var events []Event
	if obj.CompositeScore > 0.80 {
		if e.canFire("obj:critical", now) {
			events = append(events, Event{
				ID: newID(), Timestamp: now,
				Severity: SeverityCritical, Category: "OBJECTIVE",
				Description:     fmt.Sprintf("System composite risk %.0f%% — system-wide degradation (latency=40%% cascade=30%% instability=20%% osc=10%%)", obj.CompositeScore*100),
				Recommendation:  "Activate incident response; shape traffic at ingress",
				Evidence:        Evidence{CompositeScore: obj.CompositeScore, CascadeRisk: obj.CascadeFailureProbability, QueueWaitMs: obj.PredictedP99LatencyMs},
				UncertaintyScore: 0.15,
				OperationalPriority: 9,
				ModelChain: fmt.Sprintf("cause=multi_axis_degradation→model=composite_objective→prediction=system_impairment=%.0f%%→action=incident_response", obj.CompositeScore*100),
			})
		}
	}
	return events
}

func (e *Engine) canFire(key string, now time.Time) bool {
	if last, ok := e.cooldowns[key]; ok && now.Sub(last) < e.cooldownPeriod {
		return false
	}
	// LRU eviction: if at cap, remove oldest entries.
	if len(e.cooldowns) >= e.maxCooldowns {
		e.evictOldest(10)
	}
	e.cooldowns[key] = now
	e.cooldownOrder = append(e.cooldownOrder, key)
	return true
}

// evictOldest removes the n oldest cooldown entries by timestamp.
func (e *Engine) evictOldest(n int) {
	if len(e.cooldowns) == 0 {
		return
	}
	type kv struct{ k string; t time.Time }
	entries := make([]kv, 0, len(e.cooldowns))
	for k, t := range e.cooldowns {
		entries = append(entries, kv{k, t})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].t.Before(entries[j].t) })
	if n > len(entries) {
		n = len(entries)
	}
	for i := 0; i < n; i++ {
		delete(e.cooldowns, entries[i].k)
	}
	// Rebuild order slice.
	e.cooldownOrder = e.cooldownOrder[:0]
	for k := range e.cooldowns {
		e.cooldownOrder = append(e.cooldownOrder, k)
	}
}

// computePriority maps severity, utilisation, and trend to a 0..9 operational priority.
func computePriority(sev Severity, rho, trend float64) int {
	base := 0
	switch sev {
	case SeverityCritical:
		base = 7
	case SeverityWarning:
		base = 4
	case SeverityInfo:
		base = 1
	}
	// Boost by trend severity.
	trendBoost := 0
	if trend > 0.1 {
		trendBoost = 1
	}
	// Boost by utilisation proximity to saturation.
	rhoBoost := 0
	if rho > 0.90 {
		rhoBoost = 1
	}
	p := base + trendBoost + rhoBoost
	if p > 9 {
		p = 9
	}
	return p
}

func sortBySeverity(events []Event) {
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].Severity != events[j].Severity {
			return events[i].Severity > events[j].Severity
		}
		return events[i].OperationalPriority > events[j].OperationalPriority
	})
}

func newID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
