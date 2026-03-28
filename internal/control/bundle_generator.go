package control

import (
	"math"
	"math/rand"
	"sort"
	"time"
)

type Bundle struct {
	Replicas        int
	QueueLimit      float64
	RetryLimit      int
	CacheAggression float64
	HeuristicScore  float64
}

func GenerateLocalBundles(
	current SystemState,
	cfg BundleConfig,
	mem *RegimeMemory,
) []Bundle {

	// ---------- regime shaping ----------
	risk := 0.0
	if mem != nil {
		risk = mem.RiskEWMA
	}

	repRadius := int(float64(cfg.ReplicaRadius) * (1 + 0.7*risk))
	queueRadius := int(float64(cfg.QueueRadius) * (1 + 0.7*risk))
	cacheRadius := int(float64(cfg.CacheRadius) * (1 + 0.7*risk))
	retryRadius := 1 + int(1.0*risk)

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	var bundles []Bundle

	// ---------- clean integer replica grid ----------
	for dr := -repRadius; dr <= repRadius; dr++ {

		if math.Abs(float64(dr)) > float64(cfg.MaxScaleStep) {
			continue
		}

		rep := current.Replicas + dr
		if rep < cfg.MinReplicas || rep > cfg.MaxReplicas {
			continue
		}

		for dq := -queueRadius; dq <= queueRadius; dq++ {

			queue :=
				float64(current.QueueLimit) +
					float64(dq)*cfg.QueueStep

			if queue < cfg.MinQueue || queue > cfg.MaxQueue {
				continue
			}

			for rr := -retryRadius; rr <= retryRadius; rr++ {

				retry := current.RetryLimit + rr
				if retry < cfg.MinRetry || retry > cfg.MaxRetry {
					continue
				}

				for dc := -cacheRadius; dc <= cacheRadius; dc++ {

					// stochastic thinning (soft)
					if len(bundles) > 20 &&
						rng.Float64() > cfg.GenerationKeepProb {
						continue
					}

					cache :=
						current.CacheAggression +
							float64(dc)*cfg.CacheStep

					if cache < cfg.MinCache || cache > cfg.MaxCache {
						continue
					}

					// ---------- physics proxy ----------

					queuePressure :=
						current.QueueDepth /
							math.Max(queue, 1)

					admissionDrop :=
						1.0 /
							(1.0 +
								math.Exp(
									-0.05*
										(current.QueueDepth-queue),
								))

					retryAmpl :=
						1 +
							cfg.RetryAmplification*
								math.Pow(
									math.Max(0,
										float64(retry-current.RetryLimit)),
									1.25,
								)

					cacheRelief :=
						math.Exp(
							-1.1*
								(cache-current.CacheAggression),
						)

					effectiveArrival :=
						current.PredictedArrival *
							retryAmpl *
							cacheRelief *
							(1 - 0.4*admissionDrop)

					eff :=
						1.0 /
							(1 +
								cfg.EfficiencyDecay*
									math.Abs(
										float64(rep-current.Replicas),
									))

					capacity :=
						float64(rep) *
							current.ServiceRate *
							math.Max(eff, 0.55)

					util :=
						effectiveArrival /
							math.Max(capacity, 0.0001)

					// ---------- soft penalties instead of hard cut ----------

					pressurePenalty :=
						math.Log(1 + queuePressure)

					retryPenalty :=
						math.Log(1 + retryAmpl)

					// smoother growth signal
					growth :=
						(current.PredictedArrival -
							current.ArrivalRate) /
							math.Max(current.ArrivalRate, 10)

					futurePressure :=
						1 / (1 + math.Exp(-0.8*growth))

					// ---------- delta normalisation ----------

					repDelta :=
						math.Abs(
							float64(rep-current.Replicas),
						) /
							math.Max(float64(current.Replicas), 1)

					queueDelta :=
						math.Abs(
							queue-float64(current.QueueLimit),
						) /
							math.Max(float64(current.QueueLimit), 1)

					retryDelta :=
						math.Abs(
							float64(retry-current.RetryLimit),
						) /
							float64(cfg.MaxRetry)

					cacheDelta :=
						math.Abs(
							cache-current.CacheAggression,
						)

					// unified infra proxy
					infraProxy :=
						0.05*float64(rep) +
							0.02*queuePressure +
							0.03*retryDelta

					score :=
						math.Abs(util-cfg.TargetUtil) +
							cfg.QueueWeight*pressurePenalty +
							cfg.ReplicaMovePenalty*repDelta +
							cfg.QueueMovePenalty*queueDelta +
							cfg.RetryMovePenalty*retryDelta +
							cfg.CacheMovePenalty*cacheDelta +
							0.18*futurePressure +
							retryPenalty +
							infraProxy

					bundles = append(
						bundles,
						Bundle{
							Replicas:        rep,
							QueueLimit:      queue,
							RetryLimit:      retry,
							CacheAggression: cache,
							HeuristicScore:  score,
						},
					)
				}
			}
		}
	}

	// ---------- minimum candidate guarantee ----------
	if len(bundles) < 5 {
		bundles = append(bundles, Bundle{
			Replicas:        current.Replicas,
			QueueLimit:      float64(current.QueueLimit),
			RetryLimit:      current.RetryLimit,
			CacheAggression: current.CacheAggression,
			HeuristicScore:  0,
		})
	}

	sort.Slice(bundles,
		func(i, j int) bool {
			return bundles[i].HeuristicScore <
				bundles[j].HeuristicScore
		})

	// ---------- diversity ----------
	var selected []Bundle

	for _, b := range bundles {

		ok := true

		for _, s := range selected {

			dist :=
				math.Abs(float64(b.Replicas-s.Replicas))/
					math.Max(float64(current.Replicas), 1) +
					math.Abs(b.QueueLimit-s.QueueLimit)/
						math.Max(float64(current.QueueLimit), 1) +
					math.Abs(float64(b.RetryLimit-s.RetryLimit))/
						float64(cfg.MaxRetry) +
					1.5*math.Abs(b.CacheAggression-s.CacheAggression)

			if dist < cfg.MinBundleDistance {
				ok = false
				break
			}
		}

		if ok {
			selected = append(selected, b)
		}

		if len(selected) >= cfg.TopK {
			break
		}
	}

	return selected
}
