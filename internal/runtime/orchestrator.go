package runtime

import (
	"context"
	"log"
	"math"
	"sync"
	"time"

	"github.com/loadequilibrium/loadequilibrium/internal/config"
	"github.com/loadequilibrium/loadequilibrium/internal/modelling"
	"github.com/loadequilibrium/loadequilibrium/internal/optimisation"
	"github.com/loadequilibrium/loadequilibrium/internal/persistence"
	"github.com/loadequilibrium/loadequilibrium/internal/reasoning"
	"github.com/loadequilibrium/loadequilibrium/internal/simulation"
	"github.com/loadequilibrium/loadequilibrium/internal/streaming"
	"github.com/loadequilibrium/loadequilibrium/internal/telemetry"
	"github.com/loadequilibrium/loadequilibrium/internal/topology"
)

// stageIdx enumerates the 9 pipeline stages in execution order.
// Used as indices into the stageEWMA array — zero allocation, no map.
const (
	stageIdxPrune     = 0
	stageIdxWindows   = 1
	stageIdxTopology  = 2
	stageIdxCoupling  = 3
	stageIdxModelling = 4
	stageIdxOptimise  = 5
	stageIdxSim       = 6
	stageIdxReasoning = 7
	stageIdxBroadcast = 8
	numStageMetrics   = 9
)

// Orchestrator is the deterministic tick-based engine with:
//   - Tick deadline enforcement with adaptive stage-skipping on overrun
//   - Per-stage EWMA latency tracking (zero allocation, fixed array)
//   - Bounded concurrency for the model stage (worker-pool semaphore)
//   - Degraded-intelligence mode with soft confidence penalty on stale data
//   - Telemetry freshness gating (soft degrade + hard cutoff)
//   - Async simulation with drop-oldest semantics
//   - Signal state pruning
type Orchestrator struct {
	cfg       *config.Config
	store     *telemetry.Store
	graph     *topology.Graph
	signal    *modelling.SignalProcessor
	optEngine *optimisation.Engine
	reasoning *reasoning.Engine
	simRunner *simulation.Runner
	hub       *streaming.Hub
	pw        *persistence.Writer

	tickCount     uint64
	lastSimResult *simulation.SimulationResult
	prevTopo      topology.GraphSnapshot

	// windowN is the number of samples used per analysis window.
	windowN int

	// Runtime safety state.
	consecutiveOverruns int
	safetyMode          bool      // true when safetyLevel >= 2 (convenience alias for existing checks)
	lastTickScheduledAt time.Time
	totalOverruns       uint64

	// safetyLevel: graduated escalation 0=nominal 1=elevated 2=high 3=critical.
	// Each level tightens stage budgets and skips more non-critical work:
	//   0 → budgetFraction=8, simFreq=5, pred+persist normal
	//   1 → budgetFraction=7, simFreq=7, persist normal
	//   2 → budgetFraction=6, simFreq=10, persist skipped
	//   3 → budgetFraction=5, simFreq=15, pred+persist skipped, stochastic bypassed
	safetyLevel int

	// stageEWMA: per-stage rolling average latency (ms). Fixed array, zero alloc.
	stageEWMA [numStageMetrics]float64

	// stageEWMATrend: per-stage EWMA of d(EWMA)/dt — the rate of change of each
	// stage's rolling average. Positive = stage is getting slower, negative = faster.
	// Used for second-order workload forecasting: predictedMs(t+k) = EWMA + k × trend.
	// α_trend = 0.05 (slower than EWMA's 0.10) for a stable trend estimate.
	stageEWMATrend [numStageMetrics]float64

	// Adaptive tick cadence state.
	currentInterval time.Duration
	stableTickCount int

	// pressureLevel: 0=nominal 1=elevated 2=high 3=critical.
	pressureLevel int

	// simOverlayAge: ticks since the last simulation result was received.
	// Incremented each tick; reset to 0 when a new result arrives.
	// Used to mark the sim overlay as stale on the dashboard.
	simOverlayAge int
}

func New(
	cfg *config.Config,
	store *telemetry.Store,
	hub *streaming.Hub,
	pw *persistence.Writer,
) *Orchestrator {
	windowN := int(float64(cfg.RingBufferDepth) * cfg.WindowFraction)
	if windowN < 5 {
		windowN = 5
	}

	hub.SetMaxClients(cfg.MaxStreamClients)

	reasoningEngine := reasoning.NewEngine()
	reasoningEngine.SetMaxCooldowns(cfg.MaxReasoningCooldowns)

	simRunner := simulation.NewRunner(cfg.SimHorizonMs, cfg.SimShockFactor, cfg.SimAsyncBuffer)
	simRunner.SetStochasticMode(cfg.SimStochasticMode)
	if cfg.SLALatencyThresholdMs > 0 {
		simRunner.SetSLAThreshold(cfg.SLALatencyThresholdMs)
	}

	return &Orchestrator{
		cfg:             cfg,
		store:           store,
		graph:           topology.New(),
		signal:          modelling.NewSignalProcessor(cfg.EWMAFastAlpha, cfg.EWMASlowAlpha, cfg.SpikeZScore),
		optEngine:       optimisation.NewEngine(cfg),
		reasoning:       reasoningEngine,
		simRunner:       simRunner,
		hub:             hub,
		pw:              pw,
		windowN:         windowN,
		currentInterval: cfg.TickInterval,
	}
}

func (o *Orchestrator) Run(ctx context.Context) {
	// Use time.Timer instead of time.Ticker so we can reset to adaptive intervals.
	// The timer is reset AFTER each tick completes, meaning the interval is always
	// measured from the END of the previous tick — no cumulative drift under load.
	timer := time.NewTimer(o.currentInterval)
	defer timer.Stop()
	log.Printf("[engine] started  tick=%s  window=%d  maxSvc=%d  workers=%d  stochastic=%s  min=%s  max=%s",
		o.cfg.TickInterval, o.windowN, o.cfg.MaxServices,
		o.cfg.WorkerPoolSize, o.cfg.SimStochasticMode,
		o.cfg.MinTickInterval, o.cfg.MaxTickInterval)

	for {
		select {
		case <-ctx.Done():
			log.Println("[engine] shutdown")
			return
		case scheduled := <-timer.C:
			o.lastTickScheduledAt = scheduled
			o.tick(scheduled)
			// Adapt interval, then reset timer with the (possibly new) interval.
			// timer.C is drained at this point — Reset is safe per Go timer docs.
			o.adaptInterval()
			timer.Reset(o.currentInterval)
		}
	}
}

// adaptInterval adjusts currentInterval based on overrun history AND predicted load.
//
// Reactive: 2+ consecutive overruns → multiplicative stretch by TickAdaptStep.
// Proactive: when EWMA-predicted critical-stage cost > 85% of TickDeadline and no
//   overrun has occurred yet, pre-emptively stretch by √TickAdaptStep (softer).
//   This prevents the first overrun from occurring in the first place.
// Contraction: exponential-decay toward nominal — monotone, zero oscillation.
func (o *Orchestrator) adaptInterval() {
	adaptStep := o.cfg.TickAdaptStep
	if adaptStep <= 1.0 {
		adaptStep = 1.25
	}
	minI := o.cfg.MinTickInterval
	maxI := o.cfg.MaxTickInterval
	nominal := o.cfg.TickInterval
	if minI <= 0 {
		minI = nominal
	}
	if maxI <= 0 || maxI < nominal {
		maxI = nominal * 5
	}

	// Proactive stretch: second-order forecast EWMA + k×trend.
	// k=3 ticks ahead gives enough lead time to act before overrun.
	const adaptForecastHorizon = 3.0
	adaptPredMs := (o.stageEWMA[stageIdxPrune] + adaptForecastHorizon*o.stageEWMATrend[stageIdxPrune]) +
		(o.stageEWMA[stageIdxWindows] + adaptForecastHorizon*o.stageEWMATrend[stageIdxWindows]) +
		(o.stageEWMA[stageIdxTopology] + adaptForecastHorizon*o.stageEWMATrend[stageIdxTopology]) +
		(o.stageEWMA[stageIdxCoupling] + adaptForecastHorizon*o.stageEWMATrend[stageIdxCoupling]) +
		(o.stageEWMA[stageIdxModelling] + adaptForecastHorizon*o.stageEWMATrend[stageIdxModelling]) +
		(o.stageEWMA[stageIdxOptimise] + adaptForecastHorizon*o.stageEWMATrend[stageIdxOptimise])
	if adaptPredMs < 0 {
		adaptPredMs = 0
	}
	deadlineMs := float64(o.cfg.TickDeadline.Milliseconds())
	proactiveStretch := adaptPredMs > deadlineMs*0.85 &&
		o.currentInterval == nominal &&
		o.consecutiveOverruns == 0 &&
		o.tickCount > 10 // ignore warm-up ticks with cold EWMAs

	if o.consecutiveOverruns >= 2 {
		// Reactive full stretch.
		stretched := time.Duration(float64(o.currentInterval) * adaptStep)
		if stretched > maxI {
			stretched = maxI
		}
		if stretched != o.currentInterval {
			o.currentInterval = stretched
			log.Printf("[engine] interval STRETCHED to %s (overruns=%d)", o.currentInterval, o.consecutiveOverruns)
		}
		o.stableTickCount = 0
	} else if proactiveStretch {
		// Proactive soft stretch — √adaptStep is gentler than the full step.
		softFactor := math.Sqrt(adaptStep)
		stretched := time.Duration(float64(o.currentInterval) * softFactor)
		if stretched > maxI {
			stretched = maxI
		}
		if stretched != o.currentInterval {
			o.currentInterval = stretched
			log.Printf("[engine] interval PROACTIVE to %s (predictedCritical=%.0fms deadline=%.0fms)",
				o.currentInterval, adaptPredMs, deadlineMs)
		}
		o.stableTickCount = 0
	} else {
		o.stableTickCount++
		if o.currentInterval > nominal {
			// Exponential-decay contraction: each tick closes (1-1/adaptStep) of remaining gap.
			excess := float64(o.currentInterval - nominal)
			newExcess := excess / adaptStep
			newInterval := nominal + time.Duration(newExcess)
			if newInterval < nominal {
				newInterval = nominal
			}
			if newInterval < minI {
				newInterval = minI
			}
			if newInterval != o.currentInterval {
				o.currentInterval = newInterval
				if o.stableTickCount%5 == 0 {
					log.Printf("[engine] interval decaying to %s (stableTicks=%d)", o.currentInterval, o.stableTickCount)
				}
			}
			if o.currentInterval == nominal {
				o.stableTickCount = 0
			}
		}
	}
}


func (o *Orchestrator) tick(now time.Time) {
	tickStart := time.Now()
	o.tickCount++

	// ── Hard tick deadline ───────────────────────────────────────────────────
	// tickDeadline is the absolute wall-clock boundary for this tick.
	// Each optional stage checks this before executing. Once the deadline is
	// crossed, all remaining optional stages are deterministically skipped.
	tickDeadline := tickStart.Add(o.cfg.TickDeadline)

	// ── Stage priority model ─────────────────────────────────────────────────
	// CRITICAL stages run regardless of deadline: telemetry windows, topology,
	// network coupling, modelling, optimisation.
	// OPTIONAL stages are skipped when budget is exhausted:
	//   sim (6), extended prediction/diff (7b), persistence (9).
	//
	// This classification is evaluated once here and used to gate execution.
	// safetyMode additionally forces all optional stages off.

	// ── Jitter measurement ───────────────────────────────────────────────────
	jitterMs := 0.0
	if o.tickCount > 1 && !o.lastTickScheduledAt.IsZero() {
		jitterMs = float64(tickStart.Sub(o.lastTickScheduledAt).Microseconds()) / 1000.0
	}
	logStages := o.tickCount%30 == 0
	if jitterMs > float64(o.cfg.TickInterval.Milliseconds())*0.25 && o.tickCount%10 == 0 {
		log.Printf("[engine] jitter=%.1fms (%.0f%% of interval) tick=%d pressure=%d",
			jitterMs, jitterMs/float64(o.cfg.TickInterval.Milliseconds())*100,
			o.tickCount, o.pressureLevel)
	}

	// Per-stage EWMA budget — tighten when jitter is high or safety mode active.
	// Predictive: also tighten when EWMA of critical stages predicts overrun next tick.
	// Second-order workload forecast: EWMA + k*trend for 3-tick lookahead.
	// Trend captures whether stages are getting faster or slower over recent ticks.
	const forecastHorizon = 3.0
	predictedCriticalMs := (o.stageEWMA[stageIdxPrune] + forecastHorizon*o.stageEWMATrend[stageIdxPrune]) +
		(o.stageEWMA[stageIdxWindows] + forecastHorizon*o.stageEWMATrend[stageIdxWindows]) +
		(o.stageEWMA[stageIdxTopology] + forecastHorizon*o.stageEWMATrend[stageIdxTopology]) +
		(o.stageEWMA[stageIdxCoupling] + forecastHorizon*o.stageEWMATrend[stageIdxCoupling]) +
		(o.stageEWMA[stageIdxModelling] + forecastHorizon*o.stageEWMATrend[stageIdxModelling]) +
		(o.stageEWMA[stageIdxOptimise] + forecastHorizon*o.stageEWMATrend[stageIdxOptimise])
	if predictedCriticalMs < 0 {
		predictedCriticalMs = 0
	}
	deadlineMs := float64(o.cfg.TickDeadline.Milliseconds())
	predictedOverrun := predictedCriticalMs > deadlineMs*0.80

	budgetFraction := time.Duration(8)
	if o.safetyMode || predictedOverrun || jitterMs > float64(o.cfg.TickInterval.Milliseconds())*0.10 {
		budgetFraction = 6
	}
	stageSoftLimit := o.cfg.TickDeadline * budgetFraction / 10 / numStageMetrics

	recordStage := func(idx int, name string, start time.Time) {
		elapsed := time.Since(start)
		elapsedMs := float64(elapsed.Microseconds()) / 1000.0
		const alpha = 0.10
		const alphaTrend = 0.05 // slower — stable second-order trend
		prevEWMA := o.stageEWMA[idx]
		o.stageEWMA[idx] = alpha*elapsedMs + (1-alpha)*prevEWMA
		// Track trend: d(EWMA)/dt per tick for workload forecasting.
		rawTrend := o.stageEWMA[idx] - prevEWMA
		o.stageEWMATrend[idx] = alphaTrend*rawTrend + (1-alphaTrend)*o.stageEWMATrend[idx]
		if logStages {
			log.Printf("[engine] stage=%-12s elapsed=%s ewma=%.2fms trend=%+.3fms/tick tick=%d",
				name, elapsed.Round(time.Microsecond), o.stageEWMA[idx], o.stageEWMATrend[idx], o.tickCount)
		}
		if elapsed > stageSoftLimit*3 {
			log.Printf("[engine] stage=%s OVERBUDGET elapsed=%s limit=%s tick=%d",
				name, elapsed.Round(time.Millisecond), stageSoftLimit, o.tickCount)
		}
	}

	// ── CRITICAL Stage 1: Prune stale services ───────────────────────────────
	s1 := time.Now()
	pruned := o.store.Prune(now)
	if len(pruned) > 0 {
		log.Printf("[engine] pruned %d stale services", len(pruned))
	}
	recordStage(stageIdxPrune, "prune", s1)

	// ── CRITICAL Stage 2: Service windows with freshness scoring ─────────────
	// Hard cutoff: 3× tick interval — excludes severely stale windows entirely.
	// Soft degrade zone: windows within the cutoff but aged are included with
	// penalised ConfidenceScore so downstream models produce weaker predictions.
	s2 := time.Now()
	freshCutoff := o.cfg.TickInterval * 3
	windows := o.store.AllWindows(o.windowN, freshCutoff)
	if len(windows) == 0 {
		recordStage(stageIdxWindows, "windows", s2)
		return
	}
	recordStage(stageIdxWindows, "windows", s2)

	// ── Telemetry freshness gate ──────────────────────────────────────────────
	// Compute a system-wide staleness score = mean(1 - ConfidenceScore) across windows.
	// If severely stale (score > StalenessBypassThreshold), bypass stochastic modelling
	// and simulation. Core queue model + optimisation always runs.
	var sumConf float64
	for _, w := range windows {
		sumConf += w.ConfidenceScore
	}
	systemStaleness := 1.0 - sumConf/float64(len(windows))
	bypassDeepStages := systemStaleness > o.cfg.StalenessBypassThreshold
	if bypassDeepStages && o.tickCount%5 == 0 {
		log.Printf("[engine] staleness gate: score=%.2f > threshold=%.2f — bypassing deep modelling",
			systemStaleness, o.cfg.StalenessBypassThreshold)
	}

	// Degraded-intelligence assessment.
	degradedCount := 0
	for _, w := range windows {
		if w.SampleCount < 3 {
			degradedCount++
		}
	}
	degradedFraction := float64(degradedCount) / float64(len(windows))
	var degradedServices []string
	if degradedCount > 0 {
		degradedServices = make([]string, 0, degradedCount)
		for id, w := range windows {
			if w.SampleCount < 3 {
				degradedServices = append(degradedServices, id)
			}
		}
	}
	if degradedFraction > 0.5 {
		reducedN := o.windowN / 2
		if reducedN < 3 {
			reducedN = 3
		}
		if reducedN < o.windowN {
			if o.tickCount%10 == 0 {
				log.Printf("[engine] degraded: %.0f%% low-sample, windowN %d→%d",
					degradedFraction*100, o.windowN, reducedN)
			}
			if reduced := o.store.AllWindows(reducedN, freshCutoff); len(reduced) > 0 {
				windows = reduced
			}
		}
	} else if degradedCount > 0 && o.tickCount%10 == 0 {
		log.Printf("[engine] degraded intelligence: %d/%d services have <3 samples",
			degradedCount, len(windows))
	}

	// ── Pressure level computation ────────────────────────────────────────────
	// pressureLevel drives: PID aggressiveness, simulation depth, reasoning urgency.
	// Formula: max of four independent pressure signals:
	//   1. consecutive overruns (0→none, 1→elevated, 2→high, 3+→critical)
	//   2. jitter ratio (>25% → elevated, >50% → high)
	//   3. staleness (>0.5 → elevated, >0.7 → high)
	//   4. degraded fraction (>0.5 → elevated, >0.8 → high)
	overrunPressure := func() int {
		switch {
		case o.consecutiveOverruns >= 3:
			return 3
		case o.consecutiveOverruns == 2:
			return 2
		case o.consecutiveOverruns == 1:
			return 1
		default:
			return 0
		}
	}()
	jitterPressure := func() int {
		jitterRatio := jitterMs / float64(o.cfg.TickInterval.Milliseconds())
		switch {
		case jitterRatio > 0.50:
			return 2
		case jitterRatio > 0.25:
			return 1
		default:
			return 0
		}
	}()
	stalenessPressure := func() int {
		switch {
		case systemStaleness > 0.70:
			return 2
		case systemStaleness > 0.50:
			return 1
		default:
			return 0
		}
	}()
	degradedPressure := func() int {
		switch {
		case degradedFraction > 0.80:
			return 2
		case degradedFraction > 0.50:
			return 1
		default:
			return 0
		}
	}()
	newPressure := overrunPressure
	for _, p := range []int{jitterPressure, stalenessPressure, degradedPressure} {
		if p > newPressure {
			newPressure = p
		}
	}
	if newPressure != o.pressureLevel && o.tickCount%5 == 0 {
		log.Printf("[engine] pressure %d→%d (overrun=%d jitter=%d staleness=%d degraded=%d)",
			o.pressureLevel, newPressure,
			overrunPressure, jitterPressure, stalenessPressure, degradedPressure)
	}
	o.pressureLevel = newPressure

	// Propagate pressure to subsystems BEFORE they are called this tick.
	o.optEngine.SetPressureLevel(o.pressureLevel)
	o.simRunner.SetHorizonMultiplier(pressureToSimMultiplier(o.pressureLevel))
	o.reasoning.SetRuntimePressure(o.pressureLevel)

	// ── Pressure-adaptive window depth ───────────────────────────────────────
	// Under elevated/high pressure, scale the analysis window by mean telemetry
	// confidence. This bounds modelling cost: when data is both stale AND the
	// system is under timing pressure, we commit fewer resources to processing
	// uncertain data. The confidence-scaled window is a floor of 3.
	analysisWindowN := o.windowN
	if o.pressureLevel >= 1 && len(windows) > 0 {
		// meanConf already computed as (1 - systemStaleness).
		meanConf := 1.0 - systemStaleness
		// At pressure=1: scale = max(0.75, meanConf). At pressure=2+: max(0.5, meanConf).
		minScale := 0.75
		if o.pressureLevel >= 2 {
			minScale = 0.50
		}
		scale := meanConf
		if scale < minScale {
			scale = minScale
		}
		scaled := int(float64(o.windowN) * scale)
		if scaled < 3 {
			scaled = 3
		}
		if scaled < analysisWindowN {
			analysisWindowN = scaled
			if reduced := o.store.AllWindows(analysisWindowN, freshCutoff); len(reduced) > 0 {
				windows = reduced
			}
		}
	}
	s3 := time.Now()
	if logStages && analysisWindowN < o.windowN {
		log.Printf("[engine] windowN pressure-scaled %d→%d pressure=%d",
			o.windowN, analysisWindowN, o.pressureLevel)
	}
	o.graph.Update(windows)
	topoSnap := o.graph.Snapshot()
	topoSensitivity := modelling.ComputeTopologySensitivity(topoSnap)
	recordStage(stageIdxTopology, "topology", s3)

	// ── CRITICAL Stage 3b: Network coupling ───────────────────────────────────
	s3b := time.Now()
	netCoupling := modelling.ComputeNetworkCoupling(windows, topoSnap)
	netEquilibrium := modelling.ComputeNetworkEquilibrium(netCoupling, windows)
	// Fixed-point utilisation solver: Gauss-Seidel iteration until mutual coupling converges.
	// Runs every 3 ticks to amortise cost; result persists between runs.
	var fpResult modelling.FixedPointResult
	if o.tickCount%3 == 0 || o.tickCount == 1 {
		fpResult = modelling.ComputeFixedPointEquilibrium(windows, topoSnap)
	}
	// Perturbation sensitivity: run every 5 ticks (more expensive — N × solver cost).
	var perturbSensitivity map[string]float64
	if o.tickCount%5 == 0 && fpResult.Converged {
		perturbSensitivity = modelling.ComputePerturbationSensitivity(windows, topoSnap, fpResult.SystemicCollapseProb)
	}
	recordStage(stageIdxCoupling, "coupling", s3b)

	// ── CRITICAL Stage 4: Mathematical modelling ──────────────────────────────
	// Stochastic model is bypassed when staleness gate fires — queue + stability
	// still run because they drive the control decisions.
	s4 := time.Now()
	medianMode := o.cfg.ArrivalEstimatorMode == "median"
	bundles := make(map[string]*modelling.ServiceModelBundle, len(windows))
	activeIDs := make(map[string]struct{}, len(windows))

	workerCount := o.cfg.WorkerPoolSize
	if workerCount <= 0 {
		workerCount = 1
	}
	sem := make(chan struct{}, workerCount)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for id, w := range windows {
		id, w := id, w
		activeIDs[id] = struct{}{}
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer func() { <-sem; wg.Done() }()
			if nc, ok := netCoupling[id]; ok && nc.CoupledArrivalRate > w.MeanRequestRate {
				wCopy := *w
				wCopy.MeanRequestRate = nc.CoupledArrivalRate
				w = &wCopy
			}
			q := modelling.RunQueueModel(w, medianMode)
			// Stochastic model is expensive and confidence-sensitive.
			// Bypass when data is severely stale; use zero-value stochastic model.
			sm := modelling.StochasticModel{ServiceID: id, Confidence: 1.0 - systemStaleness}
			if !bypassDeepStages {
				sm = modelling.RunStochasticModel(w)
			}
			sig := o.signal.Update(w)
			stab := modelling.RunStabilityAssessment(q, sig, topoSnap, o.cfg.CollapseThreshold)
			b := &modelling.ServiceModelBundle{Queue: q, Stochastic: sm, Signal: sig, Stability: stab}
			mu.Lock()
			bundles[id] = b
			mu.Unlock()
		}()
	}
	wg.Wait()
	o.signal.Prune(activeIDs)
	recordStage(stageIdxModelling, "modelling", s4)

	// ── CRITICAL Stage 5: Optimisation / control ──────────────────────────────
	s5 := time.Now()
	costGradients := optimisation.ComputeCostGradients(bundles, topoSnap, 500.0)
	directives := o.optEngine.RunControl(bundles, costGradients, now)
	objective := optimisation.ComputeObjective(bundles, topoSnap, now)
	recordStage(stageIdxOptimise, "optimise", s5)

	// ── Hard deadline gate for optional stages ────────────────────────────────
	// After all CRITICAL stages are complete, check whether the hard deadline
	// has passed. Any optional stage that starts after tickDeadline is skipped.
	// This is the actual enforcement boundary — not just detection after the fact.
	pastDeadline := time.Now().After(tickDeadline)
	// Graduated skip policy — each safety level removes more optional work.
	skipSim := pastDeadline || bypassDeepStages
	skipPredDiff := pastDeadline || o.safetyLevel >= 3 || time.Now().After(tickDeadline.Add(-stageSoftLimit*2))
	skipPersist := pastDeadline || o.safetyLevel >= 2

	simFreq := uint64(5)
	switch {
	case o.safetyLevel >= 3:
		simFreq = 15
	case o.safetyLevel == 2:
		simFreq = 10
	case o.safetyLevel == 1:
		simFreq = 7
	}

	// ── OPTIONAL Stage 6: Async simulation ───────────────────────────────────
	s6 := time.Now()
	if o.tickCount%simFreq == 0 && !skipSim {
		o.simRunner.Submit(bundles, topoSnap, o.cfg.SimBudget)
	}
	if res := o.simRunner.Latest(); res != nil {
		o.lastSimResult = res
		o.simOverlayAge = 0
	} else {
		o.simOverlayAge++
	}
	// Multi-scenario comparison: run every 10 ticks to produce best/worst/median outcomes.
	var scenarioComp *streaming.ScenarioComparisonSnapshot
	if o.tickCount%10 == 0 && !skipSim && o.cfg.SimBudget >= 20*time.Millisecond {
		msr := o.simRunner.LatestMultiScenario(bundles, topoSnap, o.cfg.SimBudget, 3)
		if msr != nil {
			snap := streaming.ScenarioComparisonSnapshot{
				ScenarioCount:          msr.Comparison.ScenarioCount,
				BestCaseCollapse:       msr.Comparison.BestCaseCollapse,
				WorstCaseCollapse:      msr.Comparison.WorstCaseCollapse,
				MedianSLAViolation:     msr.Comparison.MedianSLAViolation,
				StableScenarioFraction: msr.Comparison.StableScenarioFraction,
				RecoveryConvergenceMin: msr.Comparison.RecoveryConvergenceMin,
				RecoveryConvergenceMax: msr.Comparison.RecoveryConvergenceMax,
			}
			scenarioComp = &snap
		}
	}
	recordStage(stageIdxSim, "sim", s6)

	// ── CRITICAL Stage 7: Reasoning ───────────────────────────────────────────
	s7 := time.Now()
	events := o.reasoning.AnalyseWithContext(bundles, topoSnap, objective, netEquilibrium, topoSensitivity, now)
	recordStage(stageIdxReasoning, "reasoning", s7)

	// ── Build overlays ────────────────────────────────────────────────────────
	satCountdowns := make(map[string]float64, len(bundles))
	stabilityZones := make(map[string]string, len(bundles))
	predHorizon := make(map[string]float64, len(bundles))
	for id, b := range bundles {
		countdown := -1.0
		if b.Queue.SaturationHorizon > 0 {
			countdown = b.Queue.SaturationHorizon.Seconds()
		}
		if b.Queue.NetworkSaturationHorizon > 0 {
			nsh := b.Queue.NetworkSaturationHorizon.Seconds()
			if countdown < 0 || nsh < countdown {
				countdown = nsh
			}
		}
		if countdown >= 0 {
			satCountdowns[id] = countdown
		}
		stabilityZones[id] = b.Stability.CollapseZone
		horizonTicks := o.cfg.PredictiveHorizonTicks
		if horizonTicks <= 0 {
			horizonTicks = 5
		}
		predRho := b.Queue.Utilisation + b.Queue.UtilisationTrend*float64(horizonTicks)*2.0
		if predRho < 0 {
			predRho = 0
		}
		predHorizon[id] = predRho
	}

	// ── OPTIONAL Stage 7b: Prediction timeline + topology diff ───────────────
	var predTimeline map[string][]streaming.PredictionPoint
	var topoDiff streaming.TopologyDiff
	if !skipPredDiff {
		ticSec := o.cfg.TickInterval.Seconds()
		predHorizon2 := o.cfg.PredictiveHorizonTicks
		if predHorizon2 <= 0 {
			predHorizon2 = 8
		}
		predTimeline = streaming.BuildPredictionTimeline(bundles, predHorizon2, ticSec)
		topoDiff = streaming.ComputeTopologyDiff(o.prevTopo, topoSnap)
		o.prevTopo = topoSnap
	} else {
		topoDiff = streaming.TopologyDiff{IsFull: false}
	}

	riskQueue := buildRiskQueue(bundles, topoSensitivity, netCoupling)
	pressureHeatmap := buildPressureHeatmap(bundles, netCoupling)

	// Build predictive risk timeline — per-service risk runway over prediction horizon.
	var riskTimeline streaming.PredictiveRiskTimeline
	if !skipPredDiff {
		ticSec := o.cfg.TickInterval.Seconds()
		ph := o.cfg.PredictiveHorizonTicks
		if ph <= 0 {
			ph = 8
		}
		riskTimeline = streaming.BuildRiskTimeline(bundles, ph, ticSec, o.cfg.CollapseThreshold)
	}

	// Build stability envelope from fixed-point equilibrium analysis.
	stEnv := buildStabilityEnvelope(fpResult, netEquilibrium, perturbSensitivity)

	// Build simulation overlay state from most recent result.
	var simOverlay *streaming.SimOverlayState
	if o.lastSimResult != nil {
		overlay := &streaming.SimOverlayState{
			HorizonMs:                 o.lastSimResult.HorizonMs,
			SimTickAge:                o.simOverlayAge,
			CascadeFailureProbability: o.lastSimResult.CascadeFailureProbability,
			SLAViolationProbability:   o.lastSimResult.SLAViolationProbability,
		}
		if len(o.lastSimResult.QueueDistributionAtHorizon) > 0 {
			overlay.P95QueueLen = make(map[string]float64, len(o.lastSimResult.QueueDistributionAtHorizon))
			overlay.SaturationFrac = make(map[string]float64, len(o.lastSimResult.QueueDistributionAtHorizon))
			for id, dist := range o.lastSimResult.QueueDistributionAtHorizon {
				overlay.P95QueueLen[id] = dist.P95QueueLen
				overlay.SaturationFrac[id] = dist.SaturationFrac
			}
		}
		simOverlay = overlay
	}

	// ── CRITICAL Stage 8: Broadcast ───────────────────────────────────────────
	s8 := time.Now()
	tickHealthMs := float64(time.Since(tickStart).Microseconds()) / 1000.0

	ncSnap := make(map[string]streaming.NetworkCouplingSnapshot, len(netCoupling))
	for id, nc := range netCoupling {
		ncSnap[id] = streaming.NetworkCouplingSnapshot{
			EffectivePressure:        nc.EffectivePressure,
			PathSaturationRisk:       nc.PathSaturationRisk,
			CoupledArrivalRate:       nc.CoupledArrivalRate,
			PathEquilibriumRho:       nc.PathEquilibriumRho,
			SaturationPathLength:     nc.SaturationPathLength,
			CongestionFeedbackScore:  nc.CongestionFeedbackScore,
			PathSaturationHorizonSec: nc.PathSaturationHorizonSec,
			PathCollapseProb:         nc.PathCollapseProb,
			SteadyStateP0:            nc.SteadyStateP0,
			SteadyStateMeanQueue:     nc.SteadyStateMeanQueue,
		}
	}
	eqSnap := streaming.NetworkEquilibriumSnapshot{
		SystemRhoMean:         netEquilibrium.SystemRhoMean,
		SystemRhoVariance:     netEquilibrium.SystemRhoVariance,
		EquilibriumDelta:      netEquilibrium.EquilibriumDelta,
		IsConverging:          netEquilibrium.IsConverging,
		MaxCongestionFeedback: netEquilibrium.MaxCongestionFeedback,
		CriticalServiceID:     netEquilibrium.CriticalServiceID,
		NetworkSaturationRisk: netEquilibrium.NetworkSaturationRisk,
	}
	sensSnap := streaming.TopologySensitivitySnapshot{
		SystemFragility:       topoSensitivity.SystemFragility,
		MaxAmplificationPath:  topoSensitivity.MaxAmplificationPath,
		MaxAmplificationScore: topoSensitivity.MaxAmplificationScore,
		ByService:             make(map[string]streaming.ServiceSensSnap, len(topoSensitivity.ByService)),
	}
	for id, ss := range topoSensitivity.ByService {
		sensSnap.ByService[id] = streaming.ServiceSensSnap{
			PerturbationScore: ss.PerturbationScore,
			DownstreamReach:   ss.DownstreamReach,
			UpstreamExposure:  ss.UpstreamExposure,
			IsKeystone:        ss.IsKeystone,
		}
		if ss.IsKeystone {
			sensSnap.KeystoneServices = append(sensSnap.KeystoneServices, id)
		}
	}

	payload := &streaming.TickPayload{
		Type:                 streaming.MsgTick,
		Bundles:              bundles,
		Topology:             topoSnap,
		Objective:            objective,
		Directives:           directives,
		Events:               events,
		SimResult:            o.lastSimResult,
		TopoDiff:             topoDiff,
		PredictionTimeline:   predTimeline,
		DegradedServices:     degradedServices,
		SaturationCountdowns: satCountdowns,
		StabilityZones:       stabilityZones,
		PredictionHorizon:    predHorizon,
		TickHealthMs:         tickHealthMs,
		DegradedFraction:     degradedFraction,
		NetworkCouplingData:  ncSnap,
		SafetyMode:           o.safetyMode,
		JitterMs:             jitterMs,
		NetworkEquilibrium:   eqSnap,
		TopologySensitivity:  sensSnap,
		PriorityRiskQueue:    riskQueue,
		PressureHeatmap:      pressureHeatmap,
		SimOverlay:           simOverlay,
		FixedPointEquilibrium: streaming.FixedPointSnapshot{
			EquilibriumRho:          fpResult.EquilibriumRho,
			SystemicCollapseProb:    fpResult.SystemicCollapseProb,
			ConvergedIterations:     fpResult.ConvergedInIterations,
			Converged:               fpResult.Converged,
			PerturbationSensitivity: perturbSensitivity,
			ConvergenceRate:         fpResult.ConvergenceRate,
			StabilityMargin:         fpResult.StabilityMargin,
		},
		ScenarioComparison: scenarioComp,
		RiskTimeline:       riskTimeline,
		StabilityEnvelope:  stEnv,
		RuntimeMetrics: streaming.RuntimeMetrics{
			AvgPruneMs:          o.stageEWMA[stageIdxPrune],
			AvgWindowsMs:        o.stageEWMA[stageIdxWindows],
			AvgTopologyMs:       o.stageEWMA[stageIdxTopology],
			AvgCouplingMs:       o.stageEWMA[stageIdxCoupling],
			AvgModellingMs:      o.stageEWMA[stageIdxModelling],
			AvgOptimiseMs:       o.stageEWMA[stageIdxOptimise],
			AvgSimMs:            o.stageEWMA[stageIdxSim],
			AvgReasoningMs:      o.stageEWMA[stageIdxReasoning],
			AvgBroadcastMs:      o.stageEWMA[stageIdxBroadcast],
			TotalOverruns:       o.totalOverruns,
			ConsecOverruns:      o.consecutiveOverruns,
			PredictedCriticalMs: predictedCriticalMs,
			PredictedOverrun:    predictedOverrun,
			SafetyLevel:         o.safetyLevel,
		},
	}
	o.hub.Broadcast(payload)
	recordStage(stageIdxBroadcast, "broadcast", s8)

	// ── OPTIONAL Stage 9: Async persistence ───────────────────────────────────
	if !skipPersist {
		o.pw.Enqueue(persistence.Snapshot{
			TickAt:    now,
			Bundles:   bundles,
			Topo:      topoSnap,
			Objective: objective,
			Events:    events,
			SimResult: o.lastSimResult,
		})
	} else if logStages {
		log.Printf("[engine] persistence skipped tick=%d (pressure=%d pastDeadline=%v)",
			o.tickCount, o.pressureLevel, pastDeadline)
	}

	// ── Tick overrun detection + graduated safety escalation ──────────────────
	totalElapsed := time.Since(tickStart)
	if totalElapsed > o.cfg.TickDeadline {
		o.consecutiveOverruns++
		o.totalOverruns++
		log.Printf("[engine] TICK OVERRUN tick=%d elapsed=%s deadline=%s consec=%d level=%d",
			o.tickCount, totalElapsed.Round(time.Millisecond),
			o.cfg.TickDeadline, o.consecutiveOverruns, o.safetyLevel)

		threshold := o.cfg.SafetyModeThreshold
		if threshold <= 0 {
			threshold = 3
		}
		// Graduated escalation: level rises by 1 at each threshold multiple.
		newLevel := o.consecutiveOverruns / threshold
		if newLevel > 3 {
			newLevel = 3
		}
		if newLevel > o.safetyLevel {
			o.safetyLevel = newLevel
			o.safetyMode = o.safetyLevel >= 2
			log.Printf("[engine] SAFETY LEVEL %d ACTIVATED consecutive=%d", o.safetyLevel, o.consecutiveOverruns)
		}
	} else {
		if o.consecutiveOverruns > 0 {
			o.consecutiveOverruns--
		}
		// De-escalate one level per 5 clean ticks.
		if o.safetyLevel > 0 && o.consecutiveOverruns == 0 && o.stableTickCount%5 == 0 {
			o.safetyLevel--
			o.safetyMode = o.safetyLevel >= 2
			if o.safetyLevel == 0 {
				log.Printf("[engine] SAFETY LEVEL CLEARED totalOverruns=%d", o.totalOverruns)
			} else {
				log.Printf("[engine] SAFETY LEVEL de-escalated to %d", o.safetyLevel)
			}
		}
	}
}

// pressureToSimMultiplier maps runtime pressure level to a simulation horizon multiplier.
// 0=nominal→1.0 full depth, 1=elevated→0.75, 2=high→0.5, 3=critical→0.25
func pressureToSimMultiplier(level int) float64 {
	switch level {
	case 1:
		return 0.75
	case 2:
		return 0.50
	case 3:
		return 0.25
	default:
		return 1.0
	}
}


// buildRiskQueue constructs a priority-ranked list of services ordered by
// composite urgency score: CollapseRisk × (1 + StabilityDerivative×10) × (1.3 if keystone).
// PathCollapseProb from the network equilibrium solver is incorporated so that
// services with high upstream-driven collapse risk rank higher even when local ρ is moderate.
func buildRiskQueue(
	bundles map[string]*modelling.ServiceModelBundle,
	sensitivity modelling.TopologySensitivity,
	netCoupling map[string]modelling.NetworkCoupling,
) []streaming.RiskQueueItem {
	items := make([]streaming.RiskQueueItem, 0, len(bundles))
	for id, b := range bundles {
		isKeystone := false
		if ss, ok := sensitivity.ByService[id]; ok {
			isKeystone = ss.IsKeystone
		}

		pathCollapseProb := 0.0
		if nc, ok := netCoupling[id]; ok {
			pathCollapseProb = nc.PathCollapseProb
		}

		// Urgency = max(local CollapseRisk, PathCollapseProb) × (1 + dRisk/dt×10) × keystoneBoost.
		// Using max() ensures upstream-driven risk isn't diluted by a low local ρ.
		baseRisk := b.Stability.CollapseRisk
		if pathCollapseProb > baseRisk {
			baseRisk = pathCollapseProb
		}
		urgency := baseRisk * (1.0 + b.Stability.StabilityDerivative*10.0)
		if isKeystone {
			urgency *= 1.3
		}
		if urgency > 1.0 {
			urgency = 1.0
		}

		urgencyClass := "nominal"
		switch {
		case urgency >= 0.70:
			urgencyClass = "critical"
		case urgency >= 0.40:
			urgencyClass = "warning"
		case urgency >= 0.15:
			urgencyClass = "elevated"
		}

		items = append(items, streaming.RiskQueueItem{
			ServiceID:        id,
			UrgencyScore:     urgency,
			CollapseRisk:     b.Stability.CollapseRisk,
			Rho:              b.Queue.Utilisation,
			IsKeystone:       isKeystone,
			PathCollapseProb: pathCollapseProb,
			UrgencyClass:     urgencyClass,
		})
	}
	// Insertion sort descending by UrgencyScore (small N, stable).
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && items[j].UrgencyScore > items[j-1].UrgencyScore; j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
	return items
}

// buildPressureHeatmap produces a normalised pressure intensity per service
// by combining utilisation (50%), upstream effective pressure (30%), and
// stability derivative acceleration (20%).
func buildPressureHeatmap(
	bundles map[string]*modelling.ServiceModelBundle,
	netCoupling map[string]modelling.NetworkCoupling,
) map[string]float64 {
	heatmap := make(map[string]float64, len(bundles))
	for id, b := range bundles {
		rho := b.Queue.Utilisation
		ep := 0.0
		if nc, ok := netCoupling[id]; ok {
			ep = nc.EffectivePressure
			if ep > 1.0 {
				ep = 1.0
			}
		}
		// Normalise StabilityDerivative — expected range roughly [-0.1, 0.1].
		dRisk := (b.Stability.StabilityDerivative + 0.1) / 0.2
		if dRisk < 0 {
			dRisk = 0
		}
		if dRisk > 1 {
			dRisk = 1
		}
		pressure := 0.5*rho + 0.3*ep + 0.2*dRisk
		if pressure > 1.0 {
			pressure = 1.0
		}
		heatmap[id] = pressure
	}
	return heatmap
}

// buildStabilityEnvelope derives the safe operating boundary from the fixed-point
// equilibrium solver and perturbation sensitivity analysis.
//
// SafeSystemRhoMax is estimated as: 1 - (ConvergenceRate × 0.15 + SystemicCollapseProb × 0.10)
// This is a conservative bound — the system is "inside the envelope" when its mean ρ
// is below this value, giving the solver room to converge stably.
func buildStabilityEnvelope(
	fp modelling.FixedPointResult,
	eq modelling.NetworkEquilibriumState,
	perturbSensitivity map[string]float64,
) streaming.StabilityEnvelopeSnapshot {
	// Safe max ρ: shrinks as convergenceRate approaches 1 (marginal stability)
	// and as systemic collapse probability rises.
	convergeRate := fp.ConvergenceRate
	if convergeRate <= 0 || math.IsNaN(convergeRate) {
		convergeRate = 1.0
	}
	safeMax := 1.0 - (convergeRate*0.15 + fp.SystemicCollapseProb*0.10)
	if safeMax < 0.40 {
		safeMax = 0.40 // floor — always give operators some headroom reading
	}
	if safeMax > 0.90 {
		safeMax = 0.90
	}

	headroom := safeMax - eq.SystemRhoMean

	// Most vulnerable service: highest perturbation sensitivity.
	mostVulnerable := ""
	worstDelta := 0.0
	for id, delta := range perturbSensitivity {
		if delta > worstDelta {
			worstDelta = delta
			mostVulnerable = id
		}
	}

	return streaming.StabilityEnvelopeSnapshot{
		SafeSystemRhoMax:       safeMax,
		CurrentSystemRhoMean:   eq.SystemRhoMean,
		EnvelopeHeadroom:       headroom,
		MostVulnerableService:  mostVulnerable,
		WorstPerturbationDelta: worstDelta,
	}
}
