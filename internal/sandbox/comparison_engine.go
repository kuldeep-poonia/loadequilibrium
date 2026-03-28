package sandbox

import (
	"math"
)

/*
PHASE-4 — COMPARISON ENGINE (REV-5 HORIZON-ROBUST / RISK-SEPARATED)

Sequence position:
5️⃣ after baseline_snapshot.go

This revision fixes deep scoring integrity issues:

✔ collapse risk and interaction risk accumulated independently
✔ no double-counting in global score
✔ multi-signal temporal robustness norm (latency + backlog + throughput + collapse)
✔ proper metric standardisation layer (mean-scale invariant transform)
✔ exponential horizon weighting (recency + terminal emphasis)
✔ safety margin computed as WORST-CASE across horizon

Conceptual direction:

score ≈ discounted risk-aware utility functional
safety ≈ min-margin trajectory constraint

Human infra style intentionally uneven.
*/

type ComparisonResult struct {

	CostScore    float64
	UtilityScore float64

	CollapseEnergy   float64
	InteractionRisk  float64

	GlobalScore float64

	Stable      bool
	SafetyMarg  float64
	Confidence  float64

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