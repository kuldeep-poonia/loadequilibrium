package control

import "math"

type ControlRegime int

const (
	RegimeCalm ControlRegime = iota
	RegimeStressed
	RegimeUnstable
)

type RegimeMemory struct {

	LastAction ActionBundle

	UtilEWMA float64
	RiskEWMA float64

	LatencyRatioEWMA float64

	utilRawHistory []float64
	riskHistory    []float64

	Regime       ControlRegime
	StabilityAge int

	LastCost      float64
	CostTrendEWMA float64
	OscillationEWMA float64
}

type RegimeConfig struct {
	EWMAAlpha   float64
	HistorySize int

	BaseUtilStress   float64
	BaseRiskUnstable float64

	HysteresisMargin float64
}

func (r *RegimeMemory) Update(
	s SystemState,
	sla float64,
	cost float64,
	cfg RegimeConfig,
) {

	// EWMA states
	r.UtilEWMA =
		cfg.EWMAAlpha*s.Utilisation +
			(1-cfg.EWMAAlpha)*r.UtilEWMA

	r.RiskEWMA =
		cfg.EWMAAlpha*s.Risk +
			(1-cfg.EWMAAlpha)*r.RiskEWMA

	r.LatencyRatioEWMA =
		cfg.EWMAAlpha*(s.Latency/sla) +
			(1-cfg.EWMAAlpha)*r.LatencyRatioEWMA

	// ⭐ store RAW util for percentile
	r.utilRawHistory =
		append(r.utilRawHistory, s.Utilisation)

	r.riskHistory =
		append(r.riskHistory, r.RiskEWMA)

	if len(r.utilRawHistory) > cfg.HistorySize {
		r.utilRawHistory = r.utilRawHistory[1:]
	}
	if len(r.riskHistory) > cfg.HistorySize {
		r.riskHistory = r.riskHistory[1:]
	}

	utilThresh :=
		math.Max(
			cfg.BaseUtilStress,
			percentile(r.utilRawHistory, 0.7),
		)

	riskThresh :=
		math.Max(
			cfg.BaseRiskUnstable,
			percentile(r.riskHistory, 0.8),
		)

	prev := r.Regime

	// ⭐ hysteresis switching
	switch r.Regime {

	case RegimeCalm:
		if r.RiskEWMA > riskThresh {
			r.Regime = RegimeUnstable
		} else if r.UtilEWMA > utilThresh ||
			r.LatencyRatioEWMA > 1.1 {

			r.Regime = RegimeStressed
		}

	case RegimeStressed:
		if r.RiskEWMA > riskThresh+cfg.HysteresisMargin {
			r.Regime = RegimeUnstable
		} else if r.UtilEWMA <
			utilThresh-cfg.HysteresisMargin &&
			r.LatencyRatioEWMA < 1.05 {

			r.Regime = RegimeCalm
		}

	case RegimeUnstable:
		if r.RiskEWMA <
			riskThresh-cfg.HysteresisMargin {

			r.Regime = RegimeStressed
		}
	}

	if prev == r.Regime {
		r.StabilityAge++
	} else {
		r.StabilityAge = 0
	}

	// ⭐ robust cost trend (thresholded)
	if r.LastCost > 0 {

		delta := cost - r.LastCost

		if math.Abs(delta) > 0.02*r.LastCost {

			r.CostTrendEWMA =
				cfg.EWMAAlpha*delta +
					(1-cfg.EWMAAlpha)*r.CostTrendEWMA
		}
	}

	r.LastCost = cost
}

func (r *RegimeMemory) ExplorationProb() float64 {

	base :=
		0.02 +
			0.22*r.RiskEWMA

	// ⭐ stability-aware modulation
	if r.Regime == RegimeCalm &&
		r.StabilityAge > 10 {

		base *= 0.5
	}

	if r.Regime == RegimeUnstable &&
		r.StabilityAge > 5 {

		base *= 1.4
	}

	if r.CostTrendEWMA > 0 {
		base += 0.05
	}

	return clamp(base, 0.01, 0.40)
}

func (r *RegimeMemory) ScenarioBudget() int {

	// ⭐ sigmoid compute scaling
	x := r.RiskEWMA

	v :=
		2 +
			6/(1+math.Exp(-6*(x-0.5)))

	return int(clamp(v, 2, 8))
}

func (r *RegimeMemory) ReplicaRadius() int {

	return int(
		clamp(
			1+4*r.RiskEWMA,
			1,
			5,
		),
	)
}

func (r *RegimeMemory) QueueRadius() int {

	return int(
		clamp(
			1+3*r.UtilEWMA,
			1,
			6,
		),
	)
}

func (r *RegimeMemory) RetryRadius() int {

	return int(
		clamp(
			1+2*r.RiskEWMA,
			1,
			3,
		),
	)
}

func (r *RegimeMemory) CacheRadius() float64 {

	return clamp(
		0.1+0.5*r.UtilEWMA,
		0.1,
		0.6,
	)
}

func (r *RegimeMemory) ApplyDamping(
	current ActionBundle,
	next ActionBundle,
	bounds ActionBounds,
) ActionBundle {

	f := dampingFactor(r.Regime)

	out := ActionBundle{
		Replicas: int(
			float64(current.Replicas) +
				f*(float64(next.Replicas-current.Replicas)),
		),
		QueueLimit: current.QueueLimit +
			f*(next.QueueLimit-current.QueueLimit),
		RetryLimit: int(
			float64(current.RetryLimit) +
				f*(float64(next.RetryLimit-current.RetryLimit)),
		),
		CacheAggression:
			current.CacheAggression +
				f*(next.CacheAggression-current.CacheAggression),
	}

	// ⭐ hard clamp to actuator bounds
	out.Replicas =
		int(clamp(
			float64(out.Replicas),
			float64(bounds.MinReplicas),
			float64(bounds.MaxReplicas),
		))

	out.QueueLimit =
		clamp(
			out.QueueLimit,
			float64(bounds.MinQueue),
			float64(bounds.MaxQueue),
		)

	out.RetryLimit =
		int(clamp(
			float64(out.RetryLimit),
			float64(bounds.MinRetry),
			float64(bounds.MaxRetry),
		))

	out.CacheAggression =
		clamp(
			out.CacheAggression,
			bounds.MinCache,
			bounds.MaxCache,
		)

	return out
}

func dampingFactor(r ControlRegime) float64 {

	switch r {

	case RegimeCalm:
		return 0.5

	case RegimeStressed:
		return 0.8

	default:
		return 1.1
	}
}

func percentile(v []float64, p float64) float64 {

	if len(v) == 0 {
		return 0
	}

	tmp := make([]float64, len(v))
	copy(tmp, v)

	for i := 1; i < len(tmp); i++ {
		for j := i; j > 0 && tmp[j] < tmp[j-1]; j-- {
			tmp[j], tmp[j-1] = tmp[j-1], tmp[j]
		}
	}

	idx :=
		int(
			p * float64(len(tmp)-1),
		)

	return tmp[idx]
}

func clamp(x, lo, hi float64) float64 {

	if x < lo {
		return lo
	}
	if x > hi {
		return hi
	}
	return x
}
