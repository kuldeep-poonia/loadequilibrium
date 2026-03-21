package simulation

import (
	"container/heap"
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/loadequilibrium/loadequilibrium/internal/modelling"
	"github.com/loadequilibrium/loadequilibrium/internal/topology"
)

const (
	maxQueueDepth = 10000
	maxEvents     = 400000
	// Pareto shape parameter α=1.5 gives finite mean, heavy tail
	paretoAlpha = 1.5
)

// heapPool reuses event heap backing slices to reduce GC pressure.
var heapPool = sync.Pool{
	New: func() interface{} {
		h := make(eventHeap, 0, 2048)
		return &h
	},
}

// Runner executes budget-bounded discrete-event simulations asynchronously.
type Runner struct {
	horizonMs         float64
	shockFactor       float64
	stochasticMode    string
	results           chan SimulationResult
	rng               *rand.Rand
	horizonMultiplier float64
	scenarioCount     int
	// slaThresholdMs: per-request latency SLA threshold. Passed into each run.
	slaThresholdMs float64
}

// SetSLAThreshold configures the latency SLA threshold for simulation tracking.
func (r *Runner) SetSLAThreshold(ms float64) {
	if ms > 0 {
		r.slaThresholdMs = ms
	}
}

// SetScenarioCount configures Monte-Carlo depth. count ∈ [1, 8].
func (r *Runner) SetScenarioCount(count int) {
	if count < 1 {
		count = 1
	}
	if count > 8 {
		count = 8
	}
	r.scenarioCount = count
}

// SetHorizonMultiplier scales the simulation virtual-time horizon.
// multiplier ∈ [0.1, 1.0]: 1.0 = full depth, lower = shallower/faster.
// Use to reduce simulation cost under runtime pressure.
func (r *Runner) SetHorizonMultiplier(multiplier float64) {
	if multiplier < 0.1 {
		multiplier = 0.1
	}
	if multiplier > 1.0 {
		multiplier = 1.0
	}
	r.horizonMultiplier = multiplier
}

func NewRunner(horizonMs, shockFactor float64, asyncBuffer int) *Runner {
	return &Runner{
		horizonMs:         horizonMs,
		shockFactor:       shockFactor,
		stochasticMode:    "exponential",
		results:           make(chan SimulationResult, asyncBuffer),
		rng:               rand.New(rand.NewSource(time.Now().UnixNano())),
		horizonMultiplier: 1.0,
		scenarioCount:     1,
		slaThresholdMs:    500.0,
	}
}

// SetStochasticMode switches the inter-arrival distribution.
func (r *Runner) SetStochasticMode(mode string) {
	if mode == "pareto" || mode == "exponential" {
		r.stochasticMode = mode
	}
}

// Submit launches async Monte-Carlo simulation. Non-blocking, drop-oldest.
// When scenarioCount > 1, runs N independent scenarios with separate RNG seeds
// and merges the results into a single probability-averaged output:
//   - CollapseDetected / CascadeTriggered are OR'd across scenarios
//   - CascadeFailureProbability is averaged across scenarios (empirical P(collapse))
//   - Service outcomes use the mean of continuous metrics across scenarios
// CPU budget is divided equally among scenarios.
func (r *Runner) Submit(
	bundles map[string]*modelling.ServiceModelBundle,
	topo topology.GraphSnapshot,
	budget time.Duration,
) {
	select {
	case <-r.results:
	default:
	}

	snap := snapshotBundles(bundles, r.slaThresholdMs)
	mode := r.stochasticMode
	effectiveHorizonMs := r.horizonMs * r.horizonMultiplier
	nScenarios := r.scenarioCount
	if nScenarios < 1 {
		nScenarios = 1
	}
	// Budget per scenario — each runs independently with equal share.
	scenarioBudget := time.Duration(int64(budget) / int64(nScenarios))
	if scenarioBudget < 5*time.Millisecond {
		scenarioBudget = 5 * time.Millisecond
		nScenarios = 1 // not enough budget for multi-scenario
	}

	seeds := make([]int64, nScenarios)
	for i := range seeds {
		seeds[i] = r.rng.Int63()
	}

	go func() {
		if nScenarios == 1 {
			rng := rand.New(rand.NewSource(seeds[0]))
			res := run(snap, topo, scenarioBudget, effectiveHorizonMs, r.shockFactor, mode, rng)
			select {
			case r.results <- res:
			default:
			}
			return
		}

		// Multi-scenario: run N independent simulations and merge.
		runs := make([]SimulationResult, nScenarios)
		var wg sync.WaitGroup
		for i := 0; i < nScenarios; i++ {
			i := i
			rng := rand.New(rand.NewSource(seeds[i]))
			wg.Add(1)
			go func() {
				defer wg.Done()
				runs[i] = run(snap, topo, scenarioBudget, effectiveHorizonMs, r.shockFactor, mode, rng)
			}()
		}
		wg.Wait()

		merged := mergeScenarios(runs)
		select {
		case r.results <- merged:
		default:
		}
	}()
}

// MultiScenarioResult holds individual runs for comparison alongside the merged result.
type MultiScenarioResult struct {
	Comparison ScenarioComparisonData
	Runs       []SimulationResult
}

// ScenarioComparisonData is the comparison summary produced by LatestMultiScenario.
// It is mirrored in streaming.ScenarioComparisonSnapshot for JSON serialisation.
type ScenarioComparisonData struct {
	ScenarioCount          int
	BestCaseCollapse       float64
	WorstCaseCollapse      float64
	MedianSLAViolation     float64
	StableScenarioFraction float64
	RecoveryConvergenceMin float64
	RecoveryConvergenceMax float64
}

// LatestMultiScenario runs N independent scenarios synchronously within the given
// budget and returns individual run results and a comparison summary.
// Returns nil when nScenarios < 2 or budget is insufficient.
func (r *Runner) LatestMultiScenario(
	bundles map[string]*modelling.ServiceModelBundle,
	topo topology.GraphSnapshot,
	budget time.Duration,
	nScenarios int,
) *MultiScenarioResult {
	if nScenarios < 2 {
		return nil
	}
	scenarioBudget := time.Duration(int64(budget) / int64(nScenarios))
	if scenarioBudget < 5*time.Millisecond {
		return nil
	}
	effectiveHorizonMs := r.horizonMs * r.horizonMultiplier
	snap := snapshotBundles(bundles, r.slaThresholdMs)
	mode := r.stochasticMode

	runs := make([]SimulationResult, nScenarios)
	var wg sync.WaitGroup
	for i := 0; i < nScenarios; i++ {
		i := i
		rng := rand.New(rand.NewSource(r.rng.Int63()))
		wg.Add(1)
		go func() {
			defer wg.Done()
			runs[i] = run(snap, topo, scenarioBudget, effectiveHorizonMs, r.shockFactor, mode, rng)
		}()
	}
	wg.Wait()

	// Build comparison snapshot from individual runs.
	bestCollapse, worstCollapse := 1.0, 0.0
	var slaValues []float64
	stableCount := 0
	recoveryMin, recoveryMax := math.MaxFloat64, -1.0

	for _, res := range runs {
		// Approximate systemic collapse from services that saturated.
		saturatedCount := 0
		for _, svc := range res.Services {
			if svc.Saturated {
				saturatedCount++
			}
		}
		collapseEst := 0.0
		if len(res.Services) > 0 {
			collapseEst = float64(saturatedCount) / float64(len(res.Services))
		}
		if collapseEst < bestCollapse {
			bestCollapse = collapseEst
		}
		if collapseEst > worstCollapse {
			worstCollapse = collapseEst
		}
		if res.SystemStable {
			stableCount++
		}
		for _, p := range res.SLAViolationProbability {
			slaValues = append(slaValues, p)
		}
		if res.RecoveryConvergenceMs >= 0 {
			if res.RecoveryConvergenceMs < recoveryMin {
				recoveryMin = res.RecoveryConvergenceMs
			}
			if res.RecoveryConvergenceMs > recoveryMax {
				recoveryMax = res.RecoveryConvergenceMs
			}
		}
	}

	// Median SLA violation across all services and runs.
	medianSLA := 0.0
	if len(slaValues) > 0 {
		// Insertion sort for small N.
		for i := 1; i < len(slaValues); i++ {
			for j := i; j > 0 && slaValues[j] < slaValues[j-1]; j-- {
				slaValues[j], slaValues[j-1] = slaValues[j-1], slaValues[j]
			}
		}
		medianSLA = slaValues[len(slaValues)/2]
	}
	if recoveryMin == math.MaxFloat64 {
		recoveryMin = 0
	}
	if recoveryMax < 0 {
		recoveryMax = 0
	}

	return &MultiScenarioResult{
		Comparison: ScenarioComparisonData{
			ScenarioCount:          nScenarios,
			BestCaseCollapse:       bestCollapse,
			WorstCaseCollapse:      worstCollapse,
			MedianSLAViolation:     medianSLA,
			StableScenarioFraction: float64(stableCount) / float64(nScenarios),
			RecoveryConvergenceMin: recoveryMin,
			RecoveryConvergenceMax: recoveryMax,
		},
		Runs: runs,
	}
}
//
// Merging semantics:
//   - SystemStable: AND across scenarios (stable only if all scenarios are stable)
//   - CollapseDetected / CascadeTriggered: OR (detected if any scenario hit it)
//   - CascadeFailureProbability: mean across scenarios (empirical probability)
//   - Per-service continuous metrics: mean across scenarios
//   - RecoveryConvergenceMs: mean of non-negative values; -1 if any scenario didn't converge
//   - DegradedServiceCount: mean rounded to nearest int
func mergeScenarios(runs []SimulationResult) SimulationResult {
	if len(runs) == 0 {
		return SimulationResult{}
	}
	if len(runs) == 1 {
		return runs[0]
	}

	merged := SimulationResult{
		HorizonMs:                  runs[0].HorizonMs,
		Services:                   make(map[string]ServiceOutcome),
		CascadeFailureProbability:  make(map[string]float64),
		QueueDistributionAtHorizon: make(map[string]QueueDistributionSnapshot),
		SLAViolationProbability:    make(map[string]float64),
		SystemStable:               true,
	}

	// Aggregate scalar fields.
	var totalEvents int
	var totalWallMs, totalBudgetPct float64
	var recoverySum float64
	recoveryCount := 0
	recoveryUnconverged := false
	var degradedSum float64

	for _, r := range runs {
		if !r.SystemStable {
			merged.SystemStable = false
		}
		if r.CollapseDetected {
			merged.CollapseDetected = true
		}
		if r.CascadeTriggered {
			merged.CascadeTriggered = true
		}
		totalEvents += r.EventsProcessed
		totalWallMs += r.Meta.WallTimeMs
		totalBudgetPct += r.Meta.BudgetUsedPct
		if r.RecoveryConvergenceMs >= 0 {
			recoverySum += r.RecoveryConvergenceMs
			recoveryCount++
		} else if r.RecoveryConvergenceMs == -1 {
			recoveryUnconverged = true
		}
		degradedSum += float64(r.DegradedServiceCount)
	}

	n := float64(len(runs))
	merged.EventsProcessed = totalEvents / len(runs)
	merged.Meta = SimulationMeta{
		WallTimeMs:    totalWallMs / n,
		BudgetUsedPct: math.Min(totalBudgetPct/n, 100),
		EventsPerMs:   float64(merged.EventsProcessed) / math.Max(totalWallMs/n, 1e-3),
	}
	if recoveryUnconverged {
		merged.RecoveryConvergenceMs = -1
	} else if recoveryCount > 0 {
		merged.RecoveryConvergenceMs = recoverySum / float64(recoveryCount)
	}
	merged.DegradedServiceCount = int(math.Round(degradedSum / n))

	// Aggregate per-service outcomes: collect service IDs from all runs.
	serviceIDs := make(map[string]struct{})
	for _, r := range runs {
		for id := range r.Services {
			serviceIDs[id] = struct{}{}
		}
	}

	for id := range serviceIDs {
		var sumFinalQ, sumPeakQ, sumThroughput, sumMeanWait int
		var sumPeakUtil, sumQMean, sumQVar, sumRecovery float64
		var countSat, countValid int

		for _, r := range runs {
			svc, ok := r.Services[id]
			if !ok {
				continue
			}
			sumFinalQ += svc.FinalQueueLen
			sumPeakQ += svc.PeakQueueLen
			sumThroughput += int(svc.ThroughputRatio * 1000)
			sumMeanWait += int(svc.MeanWaitMs)
			sumPeakUtil += svc.PeakUtilisation
			sumQMean += svc.QueueLenMean
			sumQVar += svc.QueueLenVariance
			sumRecovery += svc.RecoveryTimeMs
			if svc.Saturated {
				countSat++
			}
			countValid++
		}
		if countValid == 0 {
			continue
		}
		cf := float64(countValid)
		merged.Services[id] = ServiceOutcome{
			ServiceID:        id,
			FinalQueueLen:    sumFinalQ / countValid,
			PeakQueueLen:     sumPeakQ / countValid,
			ThroughputRatio:  float64(sumThroughput) / cf / 1000.0,
			MeanWaitMs:       float64(sumMeanWait) / cf,
			Saturated:        countSat > countValid/2, // majority-vote saturation
			PeakUtilisation:  sumPeakUtil / cf,
			QueueLenMean:     sumQMean / cf,
			QueueLenVariance: sumQVar / cf,
			RecoveryTimeMs:   sumRecovery / cf,
		}

		// Cascade failure probability: mean empirical drop rate across scenarios.
		var cfpSum float64
		var cfpCount int
		for _, r := range runs {
			if p, ok := r.CascadeFailureProbability[id]; ok {
				cfpSum += p
				cfpCount++
			}
		}
		if cfpCount > 0 {
			merged.CascadeFailureProbability[id] = cfpSum / float64(cfpCount)
		}

		// SLA violation probability: mean across scenarios.
		var slaSum float64
		var slaCount int
		for _, r := range runs {
			if p, ok := r.SLAViolationProbability[id]; ok {
				slaSum += p
				slaCount++
			}
		}
		if slaCount > 0 {
			merged.SLAViolationProbability[id] = slaSum / float64(slaCount)
		}

		// Queue distribution: mean across scenarios.
		var p95Sum, satFracSum, utilEndSum float64
		var qdCount int
		for _, r := range runs {
			if qd, ok := r.QueueDistributionAtHorizon[id]; ok {
				p95Sum += qd.P95QueueLen
				satFracSum += qd.SaturationFrac
				utilEndSum += qd.UtilisationAtEnd
				qdCount++
			}
		}
		if qdCount > 0 {
			qMean := merged.Services[id].QueueLenMean
			qVar := merged.Services[id].QueueLenVariance
			merged.QueueDistributionAtHorizon[id] = QueueDistributionSnapshot{
				MeanQueueLen:     qMean,
				VarQueueLen:      qVar,
				P95QueueLen:      p95Sum / float64(qdCount),
				SaturationFrac:   satFracSum / float64(qdCount),
				UtilisationAtEnd: utilEndSum / float64(qdCount),
			}
		}
	}

	return merged
}

// Latest returns the most recent simulation result without blocking.
func (r *Runner) Latest() *SimulationResult {
	select {
	case res := <-r.results:
		return &res
	default:
		return nil
	}
}

// bundleSnap is a minimal snapshot of bundle data used by the simulator.
type bundleSnap struct {
	id              string
	arrivalRate     float64 // req/ms
	serviceRate     float64 // req/ms
	concurrency     int
	utilisation     float64
	slaThresholdMs  float64 // 0 = SLA tracking disabled
}

func snapshotBundles(bundles map[string]*modelling.ServiceModelBundle, slaMs float64) []bundleSnap {
	out := make([]bundleSnap, 0, len(bundles))
	for id, b := range bundles {
		c := int(math.Max(math.Round(b.Queue.Concurrency), 1))
		out = append(out, bundleSnap{
			id:             id,
			arrivalRate:    b.Queue.ArrivalRate / 1000.0,
			serviceRate:    b.Queue.ServiceRate / 1000.0,
			concurrency:    c,
			utilisation:    b.Queue.Utilisation,
			slaThresholdMs: slaMs,
		})
	}
	return out
}

// run is a pure function: runs the DES and returns results.
func run(
	snaps []bundleSnap,
	topo topology.GraphSnapshot,
	budget time.Duration,
	horizonMs, shockFactor float64,
	stochasticMode string,
	rng *rand.Rand,
) SimulationResult {
	wallStart := time.Now()
	deadline := wallStart.Add(budget)
	sched := newSchedulerFromPool()
	defer sched.returnToPool()

	states := make(map[string]*ServiceSimState, len(snaps))
	cascadeEdges, edgeWeights := buildCascadeEdges(topo)

	for _, s := range snaps {
		states[s.id] = &ServiceSimState{
			ServiceID:      s.id,
			ArrivalRate:    s.arrivalRate,
			BaseRate:       s.arrivalRate,
			ServiceRate:    s.serviceRate,
			Concurrency:    s.concurrency,
			Utilisation:    s.utilisation,
			SLAThresholdMs: s.slaThresholdMs,
		}
		if s.arrivalRate > 0 {
			sched.Schedule(Event{
				Time:      interarrival(rng, 1.0/s.arrivalRate, stochasticMode),
				Kind:      EventArrival,
				ServiceID: s.id,
			})
		}
	}

	// Schedule load shock at 35% of horizon on the highest-utilisation service.
	shockTarget := highestUtilService(snaps)
	if shockTarget != "" {
		sched.Schedule(Event{Time: horizonMs * 0.35, Kind: EventShock, ServiceID: shockTarget})
	}

	collapseDetected := false
	cascadeTriggered := false
	eventCount := 0

	for {
		if time.Now().After(deadline) {
			break
		}
		e, ok := sched.Next()
		if !ok || e.Time > horizonMs {
			break
		}
		eventCount++
		if eventCount > maxEvents {
			break
		}

		st, exists := states[e.ServiceID]
		if !exists {
			continue
		}

		switch e.Kind {
		case EventArrival:
			handleArrival(e, st, sched, stochasticMode, rng)

		case EventDeparture:
			handleDeparture(e, st, sched, stochasticMode, rng)

		case EventShock:
			applyShock(e, st, states, sched, cascadeEdges, edgeWeights,
				shockFactor, horizonMs, stochasticMode, rng)
			cascadeTriggered = len(cascadeEdges[e.ServiceID]) > 0

		case EventRecovery:
			handleRecovery(e, st, sched, stochasticMode, rng, horizonMs)
		}

		if st.QueueLen >= maxQueueDepth {
			collapseDetected = true
		}
	}

	wallMs := float64(time.Since(wallStart).Microseconds()) / 1000.0
	budgetMs := float64(budget.Microseconds()) / 1000.0
	budgetUsed := 0.0
	if budgetMs > 0 {
		budgetUsed = (wallMs / budgetMs) * 100.0
	}
	evPerMs := 0.0
	if wallMs > 0 {
		evPerMs = float64(eventCount) / wallMs
	}

	result := SimulationResult{
		HorizonMs:        sched.Clock,
		Services:         make(map[string]ServiceOutcome, len(states)),
		CollapseDetected: collapseDetected,
		CascadeTriggered: cascadeTriggered,
		SystemStable:     !collapseDetected,
		EventsProcessed:  eventCount,
		Meta: SimulationMeta{
			WallTimeMs:    wallMs,
			BudgetUsedPct: math.Min(budgetUsed, 100),
			EventsPerMs:   evPerMs,
		},
	}

	// Recovery convergence: latest virtual time at which all shocked services
	// returned within 2% of base rate. -1 if still shocked at horizon.
	var maxConvergeMs float64 = -1
	hasShocked := false
	degradedCount := 0
	for _, st := range states {
		if st.ShockPeakRate > 0 {
			hasShocked = true
			if st.RecoveryConvergedAt > 0 && st.RecoveryConvergedAt > maxConvergeMs {
				maxConvergeMs = st.RecoveryConvergedAt
			} else if st.Shocked && st.RecoveryConvergedAt == 0 {
				maxConvergeMs = -1
			}
		}
		// Partially degraded: hit >50% max queue but not full saturation.
		if st.MaxQueueLen > maxQueueDepth/2 && st.MaxQueueLen < maxQueueDepth {
			degradedCount++
		}
	}
	if hasShocked {
		result.RecoveryConvergenceMs = maxConvergeMs
	}
	result.DegradedServiceCount = degradedCount

	result.CascadeFailureProbability = make(map[string]float64, len(states))
	result.QueueDistributionAtHorizon = make(map[string]QueueDistributionSnapshot, len(states))
	result.SLAViolationProbability = make(map[string]float64, len(states))

	for id, st := range states {
		var ratio, meanWait float64
		if st.TotalArrived > 0 {
			ratio = float64(st.TotalServed) / float64(st.TotalArrived)
		}
		if st.TotalServed > 0 {
			meanWait = st.SumWaitMs / float64(st.TotalServed)
		}
		peakUtil := 0.0
		if st.Concurrency > 0 {
			peakUtil = math.Min(float64(st.MaxQueueLen+st.Concurrency)/float64(st.Concurrency), 2.0)
		}
		recoveryMs := 0.0
		if st.Shocked && st.RecoveryStartMs > 0 {
			recoveryMs = sched.Clock - st.RecoveryStartMs
		}
		qlMean, qlVar := 0.0, 0.0
		if st.QueueLenSamples > 0 {
			n := float64(st.QueueLenSamples)
			qlMean = st.QueueLenSum / n
			qlVar = st.QueueLenSumSq/n - qlMean*qlMean
			if qlVar < 0 {
				qlVar = 0
			}
		}
		result.Services[id] = ServiceOutcome{
			ServiceID:        id,
			FinalQueueLen:    st.QueueLen,
			PeakQueueLen:     st.MaxQueueLen,
			ThroughputRatio:  ratio,
			MeanWaitMs:       meanWait,
			Saturated:        st.MaxQueueLen >= maxQueueDepth,
			PeakUtilisation:  peakUtil,
			RecoveryTimeMs:   recoveryMs,
			QueueLenMean:     qlMean,
			QueueLenVariance: qlVar,
		}

		// CascadeFailureProbability: empirical drop rate from this simulation run.
		// P(collapse) = TotalDropped / TotalArrived; floored at 5% when queue was hit.
		failureProb := 0.0
		if st.TotalArrived > 0 {
			failureProb = float64(st.TotalDropped) / float64(st.TotalArrived)
		}
		if st.CollapseCount > 0 && failureProb < 0.05 {
			failureProb = 0.05
		}
		result.CascadeFailureProbability[id] = math.Min(failureProb, 1.0)

		// SLA violation probability: fraction of served requests that exceeded SLA threshold.
		if st.SLAChecked > 0 && st.SLAThresholdMs > 0 {
			result.SLAViolationProbability[id] = math.Min(float64(st.SLAExceedances)/float64(st.SLAChecked), 1.0)
		}
		satFrac := 0.0
		if st.QueueLenSamples > 0 {
			satFrac = float64(st.SaturationSamples) / float64(st.QueueLenSamples)
		}
		qlStd := math.Sqrt(qlVar)
		utilAtEnd := 0.0
		if st.Concurrency > 0 {
			utilAtEnd = math.Min(float64(st.InService)/float64(st.Concurrency), 2.0)
		}
		result.QueueDistributionAtHorizon[id] = QueueDistributionSnapshot{
			MeanQueueLen:     qlMean,
			VarQueueLen:      qlVar,
			P95QueueLen:      qlMean + 1.645*qlStd,
			SaturationFrac:   satFrac,
			UtilisationAtEnd: utilAtEnd,
		}
	}
	return result
}

func handleArrival(e Event, st *ServiceSimState, sched interface{ Schedule(Event) }, mode string, rng *rand.Rand) {
	st.TotalArrived++
	arrivalTime := e.Time

	if st.QueueLen >= maxQueueDepth {
		st.TotalDropped++
		st.CollapseCount++ // accumulate collapse frequency for failure probability
	} else if st.InService < st.Concurrency {
		st.InService++
		svcDur := interarrival(rng, 1.0/math.Max(st.ServiceRate, 1e-12), "exponential") // service always exponential
		sched.Schedule(Event{
			Time:              arrivalTime + svcDur,
			Kind:              EventDeparture,
			ServiceID:         e.ServiceID,
			ServiceDurationMs: svcDur,
			ArrivalTime:       arrivalTime,
		})
	} else {
		st.QueueLen++
		if st.QueueLen > st.MaxQueueLen {
			st.MaxQueueLen = st.QueueLen
		}
	}

	if st.ArrivalRate > 0 {
		sched.Schedule(Event{
			Time:      arrivalTime + interarrival(rng, 1.0/st.ArrivalRate, mode),
			Kind:      EventArrival,
			ServiceID: e.ServiceID,
		})
	}
}

func handleDeparture(e Event, st *ServiceSimState, sched interface{ Schedule(Event) }, mode string, rng *rand.Rand) {
	st.TotalServed++
	st.InService--

	waitMs := e.Time - e.ArrivalTime - e.ServiceDurationMs
	if waitMs > 0 {
		st.SumWaitMs += waitMs
	}

	// Sample queue length distribution at each departure for variance tracking.
	ql := float64(st.QueueLen)
	st.QueueLenSamples++
	st.QueueLenSum += ql
	st.QueueLenSumSq += ql * ql
	// Track saturation fraction: fraction of departures where queue > 50% max depth.
	if st.QueueLen > maxQueueDepth/2 {
		st.SaturationSamples++
	}
	// SLA tracking: record whether this request's total wait exceeded the threshold.
	// waitMs is the queueing wait; total latency ≈ waitMs + service time.
	// We track against the configured threshold passed in via the run closure.
	st.SLAChecked++
	if waitMs > 0 {
		// SLA threshold is stored in the state via the bundleSnap slaThresholdMs field.
		// For requests with wait > 0, we already know queueing contributed.
		// Use a conservative threshold: if waitMs alone exceeds threshold, it's a violation.
		if waitMs > st.SLAThresholdMs {
			st.SLAExceedances++
		}
	}

	if st.QueueLen > 0 {
		st.QueueLen--
		st.InService++
		svcDur := interarrival(rng, 1.0/math.Max(st.ServiceRate, 1e-12), "exponential")
		sched.Schedule(Event{
			Time:              e.Time + svcDur,
			Kind:              EventDeparture,
			ServiceID:         e.ServiceID,
			ServiceDurationMs: svcDur,
			ArrivalTime:       e.Time,
		})
	}
}

// applyShock propagates load shock using probabilistic BFS with stochastic branching.
//
// Each downstream hop fires with probability proportional to the edge weight:
//   P(cascade reaches tgt) = edge_weight × (0.6^hop)
// This models the reality that not every high-traffic edge will cascade —
// the edge weight acts as both an amplitude damping AND a branching probability.
// The rng is used for Bernoulli trial at each hop.
func applyShock(
	e Event,
	st *ServiceSimState,
	states map[string]*ServiceSimState,
	sched interface{ Schedule(Event) },
	cascadeEdges map[string][]string,
	edgeWeights map[[2]string]float64,
	shockFactor, horizonMs float64,
	mode string, rng *rand.Rand,
) {
	st.ArrivalRate = st.BaseRate * shockFactor
	st.ShockPeakRate = st.ArrivalRate
	st.Shocked = true

	type bfsItem struct {
		id  string
		hop int
		amp float64
	}
	queue := []bfsItem{{e.ServiceID, 0, shockFactor}}
	visited := map[string]bool{e.ServiceID: true}

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur.hop > 3 {
			continue
		}
		for _, tgt := range cascadeEdges[cur.id] {
			if visited[tgt] {
				continue
			}
			ew := edgeWeights[[2]string{cur.id, tgt}]
			// Deterministic amplitude decay per hop.
			childAmp := cur.amp * 0.6 * ew
			if childAmp < 1.1 {
				continue
			}

			// Stochastic branching: cascade fires with probability = edge_weight.
			// This represents the realistic uncertainty in whether congestion
			// actually propagates across a given dependency at any given moment.
			branchProb := math.Min(ew*math.Pow(0.7, float64(cur.hop)), 1.0)
			if rng.Float64() > branchProb {
				continue // this hop doesn't cascade this run
			}

			visited[tgt] = true
			if tgtSt, ok := states[tgt]; ok && !tgtSt.Shocked {
				tgtSt.ArrivalRate = tgtSt.BaseRate * childAmp
				tgtSt.ShockPeakRate = tgtSt.ArrivalRate
				tgtSt.Shocked = true
				recoveryAt := e.Time + horizonMs*0.25
				tgtSt.RecoveryStartMs = recoveryAt
				sched.Schedule(Event{
					Time:      recoveryAt,
					Kind:      EventRecovery,
					ServiceID: tgt,
				})
				queue = append(queue, bfsItem{tgt, cur.hop + 1, childAmp})
			}
		}
	}

	recoveryAt := e.Time + horizonMs*0.30
	st.RecoveryStartMs = recoveryAt
	sched.Schedule(Event{Time: recoveryAt, Kind: EventRecovery, ServiceID: e.ServiceID})

	if st.ArrivalRate > 0 {
		sched.Schedule(Event{
			Time:      e.Time + interarrival(rng, 1.0/st.ArrivalRate, mode),
			Kind:      EventArrival,
			ServiceID: e.ServiceID,
		})
	}
}

// handleRecovery applies exponential ramp-down from shocked rate to base rate.
// Recovery time constant τ = 15% of horizon — approximately exponential convergence
// to within 5% of base rate after ~3τ.
func handleRecovery(e Event, st *ServiceSimState, sched interface{ Schedule(Event) }, mode string, rng *rand.Rand, horizonMs float64) {
	tau := horizonMs * 0.15
	if tau <= 0 {
		tau = 1000
	}
	excess := st.ArrivalRate - st.BaseRate
	if excess > 0.0 {
		// Exponential step: reduce excess by factor e^(-1) = 0.368 per tau.
		st.ArrivalRate = st.BaseRate + excess*math.Exp(-1.0)
		// Schedule next recovery step unless within 2% of base.
		if math.Abs(st.ArrivalRate-st.BaseRate) > st.BaseRate*0.02 {
			sched.Schedule(Event{
				Time:      e.Time + tau,
				Kind:      EventRecovery,
				ServiceID: e.ServiceID,
			})
		} else {
			st.ArrivalRate = st.BaseRate
			st.Shocked = false
			if st.RecoveryConvergedAt == 0 {
				st.RecoveryConvergedAt = e.Time + tau
			}
		}
	} else {
		st.ArrivalRate = st.BaseRate
		st.Shocked = false
		if st.RecoveryConvergedAt == 0 {
			st.RecoveryConvergedAt = e.Time
		}
	}

	if st.ArrivalRate > 0 {
		sched.Schedule(Event{
			Time:      e.Time + interarrival(rng, 1.0/st.ArrivalRate, mode),
			Kind:      EventArrival,
			ServiceID: e.ServiceID,
		})
	}
}

// interarrival samples an inter-arrival time from the chosen distribution.
func interarrival(rng *rand.Rand, mean float64, mode string) float64 {
	if mean <= 0 {
		return 0
	}
	switch mode {
	case "pareto":
		// Pareto distribution: scale = mean*(α-1)/α for finite mean.
		// x_m = mean * (α-1)/α; sample = x_m / U^(1/α)
		xm := mean * (paretoAlpha - 1.0) / paretoAlpha
		u := rng.Float64()
		if u <= 0 {
			u = 1e-15
		}
		return xm / math.Pow(u, 1.0/paretoAlpha)
	default: // exponential
		u := rng.Float64()
		if u <= 0 {
			u = 1e-15
		}
		return -math.Log(u) * mean
	}
}

func buildCascadeEdges(topo topology.GraphSnapshot) (map[string][]string, map[[2]string]float64) {
	edges := make(map[string][]string)
	weights := make(map[[2]string]float64)
	for _, e := range topo.Edges {
		if e.Weight > 0.2 {
			edges[e.Source] = append(edges[e.Source], e.Target)
			weights[[2]string{e.Source, e.Target}] = e.Weight
		}
	}
	return edges, weights
}

func highestUtilService(snaps []bundleSnap) string {
	best := ""
	bestU := 0.0
	for _, s := range snaps {
		if s.utilisation > bestU {
			bestU = s.utilisation
			best = s.id
		}
	}
	return best
}

// ── Pool-backed Scheduler ─────────────────────────────────────────────────

type pooledScheduler struct {
	Scheduler
	rawHeap *eventHeap
}

func newSchedulerFromPool() *pooledScheduler {
	h := heapPool.Get().(*eventHeap)
	*h = (*h)[:0]
	ps := &pooledScheduler{rawHeap: h}
	ps.h = *h
	heap.Init(&ps.h)
	return ps
}

func (ps *pooledScheduler) returnToPool() {
	*ps.rawHeap = ps.h
	heapPool.Put(ps.rawHeap)
}
