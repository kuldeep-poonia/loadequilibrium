package streaming

import "math"

func sanitizePayload(p *TickPayload) {
	if p == nil {
		return
	}

	p.TickHealthMs = safeNumber(p.TickHealthMs)
	p.DegradedFraction = safeNumber(p.DegradedFraction)
	p.JitterMs = safeNumber(p.JitterMs)

	for k, v := range p.SaturationCountdowns {
		p.SaturationCountdowns[k] = safeNumber(v)
	}

	for k, v := range p.PredictionHorizon {
		p.PredictionHorizon[k] = safeNumber(v)
	}

	for k, v := range p.PressureHeatmap {
		p.PressureHeatmap[k] = safeNumber(v)
	}
}

func safeNumber(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return v
}
