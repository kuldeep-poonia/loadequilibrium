package scenario

import (
	"log"
	"math"

	"github.com/loadequilibrium/loadequilibrium/internal/telemetry"
	"github.com/loadequilibrium/loadequilibrium/internal/topology"
)

type SuperpositionEngine struct {
	scenarios []Scenario
}

func NewEngine(scenarios ...Scenario) *SuperpositionEngine {
	return &SuperpositionEngine{scenarios: scenarios}
}

func (e *SuperpositionEngine) ApplySuperposition(
	tick uint64, 
	windows map[string]*telemetry.ServiceWindow, 
	topo topology.GraphSnapshot,
) map[string]*telemetry.ServiceWindow {
	
	if len(e.scenarios) == 0 {
		return windows
	}

	var active []Disturbance
	for _, s := range e.scenarios {
		if ds := s.Evaluate(tick, topo, windows); ds != nil {
			active = append(active, ds...)
		}
	}

	if len(active) == 0 {
		return windows
	}

	mutated := make(map[string]*telemetry.ServiceWindow, len(windows))
	for id, w := range windows {
		clone := *w 
		mutated[id] = &clone
	}

	rateMultipliers := make(map[string]float64)
	latencyMultipliers := make(map[string]float64)

	for _, d := range active {
		if !d.Active {
			continue
		}
		
		switch d.Metric {
		case "RequestRate":
			if val, ok := rateMultipliers[d.ServiceID]; ok {
				rateMultipliers[d.ServiceID] = math.Max(val, d.Factor)
			} else {
				rateMultipliers[d.ServiceID] = d.Factor
			}
		case "Latency":
			if val, ok := latencyMultipliers[d.ServiceID]; ok {
				latencyMultipliers[d.ServiceID] = math.Max(val, d.Factor)
			} else {
				latencyMultipliers[d.ServiceID] = d.Factor
			}
		}

		log.Printf("[scenario] tick=%d id=%s phase=%s svc=%s metric=%s factor=%.2f depth=%d prop_delay_est=%.2fms",
			tick, d.ScenarioID, d.Phase, d.ServiceID, d.Metric, d.Factor, d.PropagationDepth, d.PropagationDelayEst)
	}

	for svc, rateMult := range rateMultipliers {
		w, ok := mutated[svc]
		if !ok {
			w = &telemetry.ServiceWindow{
				ServiceID:        svc,
				MeanRequestRate:  100.0,
				LastRequestRate:  100.0,
				StdRequestRate:   5.0,
				MeanActiveConns:  10.0,
				MeanLatencyMs:    15.0,
				MaxLatencyMs:     20.0,
				LastLatencyMs:    15.0,
				LastP99LatencyMs: 25.0,
				ConfidenceScore:  1.0,
				SignalQuality:    "synthetic",
				SampleCount:      30,
			}
			mutated[svc] = w
		}

		clamped := math.Min(rateMult, 10.0)
		
		w.MeanRequestRate *= clamped
		w.LastRequestRate *= clamped
		w.StdRequestRate *= clamped

		if len(w.UpstreamEdges) > 0 {
			newEdges := make(map[string]telemetry.EdgeWindow, len(w.UpstreamEdges))
			for tgt, edge := range w.UpstreamEdges {
				attenuated := 1.0 + (clamped - 1.0) * 0.95
				newEdges[tgt] = telemetry.EdgeWindow{
					TargetServiceID: edge.TargetServiceID,
					MeanCallRate:    edge.MeanCallRate * attenuated,
					MeanErrorRate:   edge.MeanErrorRate,
					MeanLatencyMs:   edge.MeanLatencyMs,
				}
			}
			w.UpstreamEdges = newEdges
		}
	}

	for svc, latMult := range latencyMultipliers {
		w, ok := mutated[svc]
		if !ok {
			w = &telemetry.ServiceWindow{
				ServiceID:        svc,
				MeanRequestRate:  100.0,
				LastRequestRate:  100.0,
				MeanActiveConns:  10.0,
				MeanLatencyMs:    15.0,
				MaxLatencyMs:     20.0,
				LastLatencyMs:    15.0,
				LastP99LatencyMs: 25.0,
				ConfidenceScore:  1.0,
				SignalQuality:    "synthetic",
				SampleCount:      30,
			}
			mutated[svc] = w
		}

		clamped := math.Min(latMult, 10.0)
		w.MeanLatencyMs *= clamped
		w.MaxLatencyMs *= clamped
		w.LastLatencyMs *= clamped
		w.LastP99LatencyMs *= clamped
	}

	return mutated
}
