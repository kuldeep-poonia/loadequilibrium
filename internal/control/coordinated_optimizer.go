package control

import (
	"math"
	"math/rand"
	"sort"
	"time"
)

type OptimizerConfig struct {
	ScenarioCount   int
	EarlyStopMargin float64
	BaseTemperature float64
	MaxEvaluate     int
	MinEvaluate     int
}

type bundleScore struct {
	Bundle Bundle
	Score  float64
}

func SelectBestBundle(
	initial SystemState,
	candidates []Bundle,
	cfg OptimizerConfig,
	simCfg SimConfig,
	costCfg CostParams,
	mem *RegimeMemory,
) Bundle {

	if len(candidates) == 0 {
		return bundleFromState(initial)
	}

	seed := simCfg.Seed
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	rng := rand.New(rand.NewSource(seed))

	// ---------- corrected heuristic util ----------
	sort.Slice(candidates, func(i, j int) bool {
		return cheapUtilProxy(initial, candidates[i]) <
			cheapUtilProxy(initial, candidates[j])
	})

	// ---------- regime adaptive temperature ----------
	temp := cfg.BaseTemperature
	inertiaWeight := 0.02

	if mem != nil {
		temp *= math.Exp(1.4 * mem.RiskEWMA)

		// unstable → low inertia
		inertiaWeight =
			0.12 * (1 - mem.RiskEWMA)
	}

	bestScore := math.MaxFloat64
	bestBundle := candidates[0]

	results := make([]bundleScore, 0)
	evaluated := 0

	for _, b := range candidates {

		if evaluated >= cfg.MaxEvaluate {
			break
		}

		totalCost := 0.0
		costs := make([]float64, 0)

		for s := 0; s < cfg.ScenarioCount; s++ {

			// fully random scenario seed
			simCfg.Seed = rng.Int63()

			traj := SimulateBundle(initial, b, simCfg)

			c := EvaluateHorizonCost(
				initial, b, traj,
				simCfg, costCfg, mem,
			)

			totalCost += c
			costs = append(costs, c)

			meanSoFar :=
				totalCost / float64(len(costs))

			// 🔥 disable aggressive early stop
if evaluated >= cfg.MinEvaluate &&
    meanSoFar >
        bestScore*(1+cfg.EarlyStopMargin*2) {
    break
}
		}

		if len(costs) == 0 {
			continue
		}

		meanCost :=
			totalCost / float64(len(costs))

		// downside risk
		downside := 0.0
		for _, c := range costs {
			if c > meanCost {
				d := c - meanCost
				downside += d * d
			}
		}
		downside =
			math.Sqrt(downside / float64(len(costs)))

		// 🔥 CRITICAL BACKLOG PENALTY
backlogPressure := initial.QueueDepth / float64(initial.QueueLimit)

score :=
    meanCost +
        0.3*downside +
        5.0*backlogPressure

		// inertia
		if mem != nil {
			score += inertiaWeight *
				actionDistance(
					mem.LastAction, b,
					initial.QueueLimit,
				)
		}

		// TRUE exploration:
		// probabilistic acceptance of worse bundle
		if score > bestScore {
			delta := score - bestScore
			p := math.Exp(-delta / math.Max(temp, 0.001))
			if rng.Float64() > p {
				evaluated++
				continue
			}
		}

		results = append(results, bundleScore{
			Bundle: b,
			Score:  score,
		})

		if score < bestScore {
			bestScore = score
			bestBundle = b
		}

		evaluated++
	}

	if len(results) == 0 {
		return bestBundle
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score < results[j].Score
	})

	top := results
	if len(top) > 6 {
		top = top[:6]
	}

	return diversityAwareSelect(initial, top)
}

func cheapUtilProxy(
	initial SystemState,
	b Bundle,
) float64 {

	base :=
		initial.PredictedArrival /
			math.Max(
				float64(b.Replicas)*initial.ServiceRate,
				0.001,
			)

	retryAmpl :=
		1 + 0.25*float64(b.RetryLimit-initial.RetryLimit)

	cacheRelief :=
		1 - 0.3*(b.CacheAggression-initial.CacheAggression)

	return base * retryAmpl * cacheRelief
}

func diversityAwareSelect(
	initial SystemState,
	top []bundleScore,
) Bundle {

	best := top[0]
	bestMetric := math.MaxFloat64

	for _, s := range top {

		dist :=
			actionDistance(
				bundleFromState(initial),
				s.Bundle,
				initial.QueueLimit,
			)

		metric :=
			s.Score + 0.18*dist

		if metric < bestMetric {
			bestMetric = metric
			best = s
		}
	}

	return best.Bundle
}

func actionDistance(
	a, b Bundle,
	queueScale int,
) float64 {

	return math.Abs(float64(a.Replicas-b.Replicas)) +
		0.5*math.Abs(float64(a.RetryLimit-b.RetryLimit)) +
		math.Abs(a.QueueLimit-b.QueueLimit)/
			math.Max(float64(queueScale), 1) +
		2*math.Abs(a.CacheAggression-b.CacheAggression)
}

func bundleFromState(s SystemState) Bundle {
	return Bundle{
		Replicas:        s.Replicas,
		RetryLimit:      s.RetryLimit,
		QueueLimit:      float64(s.QueueLimit),
		CacheAggression: s.CacheAggression,
	}
}
