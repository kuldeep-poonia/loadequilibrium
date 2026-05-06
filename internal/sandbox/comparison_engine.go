package sandbox

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
)

type ComparisonResult struct {
	CostScore    float64
	UtilityScore float64

	CollapseEnergy  float64
	InteractionRisk float64

	GlobalScore float64

	Stable     bool
	SafetyMarg float64
	Confidence float64

	TemporalRobustness float64
}

type ComparisonWeights struct {
	LatencyW float64
	TailW    float64
	BacklogW float64
	OscW     float64
	SettleW  float64

	ThroughputW float64

	CollapseW float64
	InteractW float64

	SLA_TailLimit     float64
	SLA_CollapseLimit float64
	SLA_MinThroughput float64

	Hysteresis float64

	Discount float64 // exponential horizon discount
}

type SnapshotUncertainty struct {
	LatencyVar    float64
	TailVar       float64
	ThroughputVar float64
	BacklogVar    float64
}

func CompareSnapshotsMultiHorizon(
	base []BaselineSnapshot,
	cand []BaselineSnapshot,
	w ComparisonWeights,
	uBase SnapshotUncertainty,
	uCand SnapshotUncertainty,
) ComparisonResult {

	n := min(len(base), len(cand))
	if n == 0 {
		return ComparisonResult{}
	}

	var costAcc float64
	var utilAcc float64

	var collapseAcc float64
	var interactAcc float64

	var tempAcc float64

	wNorm := 0.0

	for i := 0; i < n; i++ {

		b := base[i]
		c := cand[i]

		// exponential recency weighting
		alpha :=
			math.Exp(
				-w.Discount *
					float64(n-1-i),
			)

		cost :=
			w.LatencyW*costImproveStd(b.MeanLatency, c.MeanLatency) +
				w.TailW*costImproveStd(b.LogTailIndex, c.LogTailIndex) +
				w.BacklogW*costImproveStd(b.MeanBacklog, c.MeanBacklog) +
				w.OscW*costImproveStd(b.OscillationIndex, c.OscillationIndex) +
				w.SettleW*costImproveStd(b.SettlingIndex, c.SettlingIndex)

		util :=
			w.ThroughputW *
				utilImproveStd(b.ThroughputMean, c.ThroughputMean)

		cE :=
			collapseEnergy(c)

		iR :=
			interactionRiskStd(c)

		costAcc += alpha * cost
		utilAcc += alpha * util

		collapseAcc += alpha * cE
		interactAcc += alpha * iR

		// multi-signal temporal variation
		if i > 0 {

			prev := cand[i-1]

			tempAcc +=
				stdDiff(c.MeanLatency, prev.MeanLatency) +
					stdDiff(c.MeanBacklog, prev.MeanBacklog) +
					stdDiff(c.ThroughputMean, prev.ThroughputMean) +
					stdDiff(c.CollapseFraction, prev.CollapseFraction)
		}

		wNorm += alpha
	}

	costScore := costAcc / wNorm
	utilScore := utilAcc / wNorm

	collapseMean := collapseAcc / wNorm
	interactionMean := interactAcc / wNorm

	global :=
		smoothUtility(costScore, utilScore) -
			w.CollapseW*collapseMean -
			w.InteractW*interactionMean

	safety :=
		worstSafetyMargin(
			cand,
			w,
		)

	stable :=
		safety > -w.Hysteresis

	conf :=
		covarianceAwareConfidence(uBase, uCand)

	robust :=
		1 / (1 + 0.25*tempAcc)

	return ComparisonResult{

		CostScore:    costScore,
		UtilityScore: utilScore,

		CollapseEnergy:  collapseMean,
		InteractionRisk: interactionMean,

		GlobalScore: global,

		Stable:     stable,
		SafetyMarg: safety,
		Confidence: conf,

		TemporalRobustness: robust,
	}
}

func collapseEnergy(c BaselineSnapshot) float64 {

	return c.CollapseFraction *
		math.Log(1+c.CollapseSeverity)
}

func interactionRiskStd(c BaselineSnapshot) float64 {

	zTail :=
		standardise(c.LogTailIndex)

	zBack :=
		standardise(c.MeanBacklog)

	return math.Tanh(zTail * zBack)
}

func smoothUtility(cost, util float64) float64 {

	return math.Tanh(cost) +
		0.5*math.Tanh(util)
}

func worstSafetyMargin(
	cand []BaselineSnapshot,
	w ComparisonWeights,
) float64 {

	m := math.MaxFloat64

	for _, c := range cand {

		tailMarg :=
			(w.SLA_TailLimit - c.LogTailIndex) /
				(w.SLA_TailLimit + 1e-6)

		collMarg :=
			(w.SLA_CollapseLimit - c.CollapseSeverity) /
				(w.SLA_CollapseLimit + 1e-6)

		thrMarg :=
			(c.ThroughputMean - w.SLA_MinThroughput) /
				(w.SLA_MinThroughput + 1e-6)

		local :=
			math.Min(
				math.Min(tailMarg, collMarg),
				thrMarg,
			)

		if local < m {
			m = local
		}
	}

	return m
}

func covarianceAwareConfidence(
	a, b SnapshotUncertainty,
) float64 {

	total :=
		a.LatencyVar +
			a.TailVar +
			a.BacklogVar +
			a.ThroughputVar +
			b.LatencyVar +
			b.TailVar +
			b.BacklogVar +
			b.ThroughputVar

	return 1 /
		(1 + math.Sqrt(total)*1.5)
}

/*
true dimensionless scaling proxy
log-ratio transform → robust across workload scales
*/

func costImproveStd(base, cand float64) float64 {

	if base <= 0 || cand <= 0 {
		return 0
	}

	return math.Log(base / cand)
}

func utilImproveStd(base, cand float64) float64 {

	if base <= 0 || cand <= 0 {
		return 0
	}

	return math.Log(cand / base)
}

/*
stdDiff returns a scale-invariant normalised absolute difference.
Used for temporal robustness accumulation across the horizon.
The denominator sum + 1e-6 guard prevents division by zero when
both values are near zero (e.g. a cold-start snapshot).
*/
func stdDiff(a, b float64) float64 {
	return math.Abs(a-b) / (math.Abs(a) + math.Abs(b) + 1e-6)
}

func standardise(x float64) float64 {

	return math.Log(1 + x)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

/*
----- PHASE 1: BASELINE VS ENGINE COMPARISON -----
*/

type Metrics struct {
	Failures     int
	RecoveryTime float64
	Throughput   float64
	TotalCost    float64
}

type Phase1ComparisonResult struct {
	Baseline    Metrics            `json:"baseline"`
	Engine      Metrics            `json:"engine"`
	Improvement map[string]float64 `json:"improvement"`
}

func ExtractMetrics(trace *PlantTrace) Metrics {
	if trace == nil || len(trace.Points) == 0 {
		return Metrics{}
	}

	n := len(trace.Points)
	var failureCount int
	var recoveryTime float64
	var totalThroughput float64
	var totalLatency float64

	inCollapse := false
	collapseStart := 0.0

	for _, p := range trace.Points {
		// Count failures
		if p.Collapsed && !inCollapse {
    failureCount++
    inCollapse = true
    collapseStart = p.Time.Seconds()
}

if !p.Collapsed && inCollapse {
    recoveryTime += p.Time.Seconds() - collapseStart
    inCollapse = false
}

		totalThroughput += p.Throughput
		totalLatency += p.Latency
	}

	var avgRecoveryTime float64
if failureCount > 0 {
    avgRecoveryTime = recoveryTime / float64(failureCount)
}

	meanThroughput := totalThroughput / float64(n)
	meanLatency := totalLatency / float64(n)

	// Cost: composite of latency + tail effects (higher latency = higher cost)
	totalCost := meanLatency + 0.5*float64(failureCount)

	return Metrics{
    Failures:     failureCount,
    RecoveryTime: avgRecoveryTime,
    Throughput:   meanThroughput,
    TotalCost:    totalCost,
}
}

func ComparePhase1Metrics(baseline, engine Metrics) map[string]float64 {
	improvement := make(map[string]float64)

	// Failures reduction (lower is better)
if baseline.Failures > 0 {
    improvement["failures_reduction"] =
        float64(baseline.Failures-engine.Failures) / float64(baseline.Failures)
} else {
    improvement["failures_reduction"] = 0
}
	// Recovery speed (baseline_time / engine_time, > 1 means improvement)
	if engine.RecoveryTime > 1e-6 {
		improvement["recovery_speed"] =
			baseline.RecoveryTime / engine.RecoveryTime
	} else {
		improvement["recovery_speed"] = 1.0
	}

	// Throughput improvement (engine / baseline, > 1 means improvement)
	if baseline.Throughput > 1e-6 {
		improvement["throughput_improvement"] =
			engine.Throughput / baseline.Throughput
	} else {
		improvement["throughput_improvement"] = 1.0
	}

	// Cost reduction (baseline_cost - engine_cost) / baseline_cost
	if baseline.TotalCost > 1e-6 {
		improvement["cost_reduction"] =
			(baseline.TotalCost - engine.TotalCost) / baseline.TotalCost
	} else {
		improvement["cost_reduction"] = 0
	}

	return improvement
}

func SavePhase1Result(result Phase1ComparisonResult, outputPath string) error {
	// Ensure directory exists
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Marshal to JSON with indentation
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}

	// Write to file
	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		return err
	}

	return nil
}
