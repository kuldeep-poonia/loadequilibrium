package runtime

import (
	"github.com/loadequilibrium/loadequilibrium/internal/modelling"
	"github.com/loadequilibrium/loadequilibrium/internal/telemetry"
	"github.com/loadequilibrium/loadequilibrium/internal/topology"
)

func (o *Orchestrator) buildSimulationBundles(
	windows map[string]*telemetry.ServiceWindow,
	topo topology.GraphSnapshot,
) map[string]*modelling.ServiceModelBundle {
	if o == nil || len(windows) == 0 {
		return nil
	}

	medianMode := o.cfg.ArrivalEstimatorMode == "median"
	signal := modelling.NewSignalProcessor(o.cfg.EWMAFastAlpha, o.cfg.EWMASlowAlpha, o.cfg.SpikeZScore)
	queuePhysics := modelling.NewQueuePhysicsEngine()

	bundles := make(map[string]*modelling.ServiceModelBundle, len(windows))
	for id, w := range windows {
		if w == nil {
			continue
		}
		q := queuePhysics.RunQueueModel(w, topo, medianMode)
		sig := signal.Update(w)
		stab := modelling.RunStabilityAssessment(q, sig, topo, o.cfg.CollapseThreshold)
		bundles[id] = &modelling.ServiceModelBundle{
			Queue:      q,
			Signal:     sig,
			Stability:  stab,
			Stochastic: modelling.StochasticModel{ServiceID: id, Confidence: 0.7},
		}
	}
	return bundles
}
