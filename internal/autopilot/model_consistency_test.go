package autopilot

import (
	"math"
	"testing"
)

func TestCapacityLagConsistentAcrossMPCPredictorAndSafety(t *testing.T) {
	x := MPCState{CapacityActive: 4, ArrivalMean: 40, ServiceRate: 10}
	m := &MPCOptimiser{MinCapacity: 1, MaxCapacity: 20}
	mpcCap := m.capLag(x, 10)

	p := &Predictor{Dt: 1}
	predCap := p.capacityNext(CongestionState{CapacityActive: 4, CapacityTarget: 10})

	if math.Abs(mpcCap-predCap) > 1e-9 {
		t.Fatalf("capacity lag mismatch: mpc=%.4f predictor=%.4f", mpcCap, predCap)
	}
}

func TestCacheReliefReducesArrivalPressureInPredictor(t *testing.T) {
	p := &Predictor{
		Dt:                     1,
		MaxQueue:               1000,
		BurstCollapseThreshold: 20,
		ArrivalRiseGain:        0.2,
		ArrivalDropGain:        0.1,
		VarianceDecayRate:      0.1,
		RetryDelayTau:          1,
		TopologyAdaptTau:       1,
		CacheAdaptTau:          1,
		BarrierExpK:            0.001,
		BarrierCap:             10000,
	}
	base := CongestionState{
		Backlog:               100,
		ArrivalMean:           80,
		ArrivalVar:            0.1,
		ServiceRate:           10,
		ServiceEfficiency:     1,
		ConcurrencyLimit:      10,
		CapacityActive:        4,
		CapacityTarget:        4,
		TopologyAmplification: 1,
	}
	withoutCache := p.Step(base)
	base.CacheRelief = 0.5
	withCache := p.Step(base)

	if withCache.Backlog >= withoutCache.Backlog {
		t.Fatalf("cache relief should reduce backlog pressure: no_cache=%.4f with_cache=%.4f", withoutCache.Backlog, withCache.Backlog)
	}
}
