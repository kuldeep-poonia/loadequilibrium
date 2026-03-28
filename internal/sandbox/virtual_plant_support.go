package sandbox

import "math"

// RunVirtualPlant executes a lightweight deterministic virtual plant over a
// generated scenario trace. It is intentionally minimal and keeps the existing
// sandbox interfaces intact for experiment orchestration.
func RunVirtualPlant(
	scenario *Scenario,
	cfg PlantConfig,
) *PlantTrace {
	if scenario == nil || len(scenario.Trace) == 0 {
		return &PlantTrace{}
	}

	trace := &PlantTrace{
		Points: make([]PlantPoint, 0, len(scenario.Trace)),
	}

	if cfg.CapacityScale <= 0 {
		cfg.CapacityScale = 1
	}

	service := math.Max(scenario.Config.BaseService*cfg.CapacityScale, 1e-3)
	backlog := 0.0
	latency := 25.0

	for _, point := range scenario.Trace {
		fanout := math.Max(point.Fanout, 0.1)
		arrival := math.Max(point.Arrival*fanout, 0)
		arrival *= 1 + 0.15*cfg.RetryBias
		arrival *= math.Max(0.6, 1-0.10*cfg.CacheRelief)

		capacityPenalty := 1 + 0.15*math.Tanh(backlog/(service+1))
		effectiveService := service * (1 + 0.10*cfg.CacheRelief) / capacityPenalty
		throughput := math.Min(arrival, effectiveService)

		backlog += (arrival - throughput) * scenario.Config.Step.Seconds()
		if backlog < 0 {
			backlog = 0
		}

		util := arrival / (effectiveService + 1e-6)
		latency = 0.8*latency +
			0.2*(20+40*util+15*math.Sqrt(backlog+1))

		collapsed := backlog > 5*effectiveService || latency > 1500

		trace.Points = append(trace.Points, PlantPoint{
			Time:        point.T,
			Latency:     latency,
			Backlog:     backlog,
			Throughput:  throughput,
			ServiceRate: effectiveService,
			Collapsed:   collapsed,
		})
	}

	return trace
}

func estimateUncertainty(snaps []BaselineSnapshot) SnapshotUncertainty {
	if len(snaps) == 0 {
		return SnapshotUncertainty{}
	}

	lat := make([]float64, 0, len(snaps))
	tail := make([]float64, 0, len(snaps))
	thr := make([]float64, 0, len(snaps))
	back := make([]float64, 0, len(snaps))

	for _, snap := range snaps {
		lat = append(lat, snap.MeanLatency)
		tail = append(tail, snap.LogTailIndex)
		thr = append(thr, snap.ThroughputMean)
		back = append(back, snap.MeanBacklog)
	}

	return SnapshotUncertainty{
		LatencyVar:    variance(lat),
		TailVar:       variance(tail),
		ThroughputVar: variance(thr),
		BacklogVar:    variance(back),
	}
}

func variance(x []float64) float64 {
	if len(x) == 0 {
		return 0
	}

	mean := 0.0
	for _, v := range x {
		mean += v
	}
	mean /= float64(len(x))

	sum := 0.0
	for _, v := range x {
		d := v - mean
		sum += d * d
	}

	return sum / float64(len(x))
}
