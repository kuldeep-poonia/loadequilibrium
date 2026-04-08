package scenario

import (
	"log"
	"math"
	"os"
	"strings"
	"sync"

	"github.com/loadequilibrium/loadequilibrium/internal/telemetry"
	"github.com/loadequilibrium/loadequilibrium/internal/topology"
)

func isScenarioDisabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("SCENARIO_MODE")))
	return v == "off" || v == "false" || v == "0"
}

type SuperpositionEngine struct {
	mu           sync.RWMutex
	scenarios    []Scenario
	overlays     map[string]scheduledScenario
	ScenarioMode string
}

type scheduledScenario struct {
	scenario  Scenario
	untilTick uint64
}

func NewEngine(scenarios ...Scenario) *SuperpositionEngine {
	base := make([]Scenario, len(scenarios))
	copy(base, scenarios)
	return &SuperpositionEngine{
		scenarios: base,
		overlays:  make(map[string]scheduledScenario),
	}
}

func (e *SuperpositionEngine) SetOverlay(name string, scenario Scenario, untilTick uint64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.overlays[name] = scheduledScenario{scenario: scenario, untilTick: untilTick}
}

func (e *SuperpositionEngine) clearExpiredLocked(tick uint64) {
	for name, slot := range e.overlays {
		if slot.untilTick > 0 && tick > slot.untilTick {
			delete(e.overlays, name)
		}
	}
}

func (e *SuperpositionEngine) OverlayNames(tick uint64) []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.clearExpiredLocked(tick)
	names := make([]string, 0, len(e.overlays))
	for name := range e.overlays {
		names = append(names, name)
	}
	return names
}

func (e *SuperpositionEngine) scenariosForTick(tick uint64) []Scenario {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.clearExpiredLocked(tick)
	all := make([]Scenario, 0, len(e.scenarios)+len(e.overlays))
	all = append(all, e.scenarios...)
	for _, slot := range e.overlays {
		all = append(all, slot.scenario)
	}
	return all
}

func (e *SuperpositionEngine) ApplySuperposition(
	tick uint64, 
	windows map[string]*telemetry.ServiceWindow, 
	topo topology.GraphSnapshot,
) map[string]*telemetry.ServiceWindow {
	
	if isScenarioDisabled() || e.ScenarioMode == "off" {
		return windows
	}

	scenarios := e.scenariosForTick(tick)
	if len(scenarios) == 0 {
		return windows
	}

	var active []Disturbance
	for _, s := range scenarios {
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

	applyToServices := func(target string, fn func(string)) {
		if target == "*" {
			for id := range mutated {
				fn(id)
			}
			return
		}
		fn(target)
	}

	for svc, rateMult := range rateMultipliers {
		applyToServices(svc, func(serviceID string) {
		w, ok := mutated[serviceID]
		if !ok {
			w = &telemetry.ServiceWindow{
				ServiceID:        serviceID,
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
			mutated[serviceID] = w
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
		})
	}

	for svc, latMult := range latencyMultipliers {
		applyToServices(svc, func(serviceID string) {
		w, ok := mutated[serviceID]
		if !ok {
			w = &telemetry.ServiceWindow{
				ServiceID:        serviceID,
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
			mutated[serviceID] = w
		}

		clamped := math.Min(latMult, 10.0)
		w.MeanLatencyMs *= clamped
		w.MaxLatencyMs *= clamped
		w.LastLatencyMs *= clamped
		w.LastP99LatencyMs *= clamped
		})
	}

	return mutated
}
