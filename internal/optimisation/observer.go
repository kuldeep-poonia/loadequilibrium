package optimisation

import (
	"log"
	"math"

	"github.com/loadequilibrium/loadequilibrium/internal/modelling"
	"github.com/loadequilibrium/loadequilibrium/internal/simulation"
	"github.com/loadequilibrium/loadequilibrium/internal/topology"
)

// ObserverSignal holds the evaluated dynamic variables for closed-loop control.
type ObserverSignal struct {
	QueueVelocity      float64
	UtilisationGrad    float64
	DisturbanceEnergy  float64
	StabilityScore     float64

	// Legacy fields mirrored for controller abstraction compatibility
	StabilityEnvelope  float64
	DisturbanceDecay   float64
	RecoveryActivation float64
}

type observerState struct {
	queueHistory    [3]float64
	historyIdx      int
	historyCount    int
	lastArrivalRate float64
	lastServiceRate float64
	utilGradient    float64
	filteredQueueV  float64
}

// AdaptiveStateObserver produces meaningful dynamic signals for the controller.
type AdaptiveStateObserver struct {
	states map[string]*observerState
}

func NewAdaptiveStateObserver() *AdaptiveStateObserver {
	return &AdaptiveStateObserver{
		states: make(map[string]*observerState),
	}
}

// Observe evaluates the non-linear trajectories of the orchestrator state map.
func (o *AdaptiveStateObserver) Observe(
	bundles map[string]*modelling.ServiceModelBundle,
	simResult *simulation.SimulationResult,
	topo topology.GraphSnapshot,
) map[string]ObserverSignal {

	signals := make(map[string]ObserverSignal, len(bundles))
	energyMap := make(map[string]float64, len(bundles))

	// Pass 1: Local physics estimation
	for id, b := range bundles {
		st := o.states[id]
		if st == nil {
			st = &observerState{
				lastArrivalRate: b.Queue.ArrivalRate,
				lastServiceRate: b.Queue.ServiceRate,
			}
			o.states[id] = st
		}

		// A. Sliding-Window Queue Derivative (velocity)
		// compute queue velocity using weighted difference across recent teleporting windows
		st.queueHistory[st.historyIdx] = b.Queue.MeanQueueLen
		st.historyIdx = (st.historyIdx + 1) % 3
		if st.historyCount < 3 {
			st.historyCount++
		}

		rawVelocity := 0.0
		if st.historyCount == 3 {
			oldestIdx := st.historyIdx // The oldest value is what we are about to overwrite
			rawVelocity = (b.Queue.MeanQueueLen - st.queueHistory[oldestIdx]) / 2.0
		}

		// F. Temporal Coherence Filter (Bounded smoothing for velocity)
		// preservers trend direction while suppressing measurement noise
		st.filteredQueueV = 0.6*rawVelocity + 0.4*st.filteredQueueV
		queueVelocity := st.filteredQueueV

		// B. Utilisation Gradient Estimator
		// Derive utilisation trend from arrival/service imbalance integrated over short horizon
		currImbalance := b.Queue.ArrivalRate - b.Queue.ServiceRate
		prevImbalance := st.lastArrivalRate - st.lastServiceRate
		st.utilGradient = 0.8*st.utilGradient + 0.2*(currImbalance-prevImbalance)
		st.lastArrivalRate = b.Queue.ArrivalRate
		st.lastServiceRate = b.Queue.ServiceRate

		// C. Local Disturbance Energy Metric
		// Estimate shock magnitude via variance of arrival rate, latency, and queue len
		arrivalVar := b.Signal.EWMAVariance
		latencyVal := b.Queue.AdjustedWaitMs
		queueVal := b.Queue.MeanQueueLen

		energy := (arrivalVar * 1.5) + (latencyVal * 0.1) + (queueVal * 0.5)
		if energy < 0 {
			energy = 0
		}
		energyMap[id] = energy

		// D. Normalised Stability Envelope
		// Produce dimensionless stability score scaling variance relative to capacity
		capacity := math.Max(b.Queue.Concurrency, 1.0)
		normVariance := arrivalVar / capacity
		stabilityScore := math.Max(0.0, 1.0-(normVariance*0.1)-math.Max(0, st.utilGradient*0.5))
		if stabilityScore > 1.0 {
			stabilityScore = 1.0
		}

		signals[id] = ObserverSignal{
			QueueVelocity:   queueVelocity,
			UtilisationGrad: st.utilGradient,
			StabilityScore:  stabilityScore,
		}
	}

	// Pass 2: Graph-Weighted Aggregation (Propagate disturbance influence)
	for id := range signals {
		localEnergy := energyMap[id]
		propagatedEnergy := 0.0

		// E. Propagate influence through service dependency graph using attenuated edges
		for _, edge := range topo.Edges {
			if edge.Target == id {
				if upstreamEnergy, ok := energyMap[edge.Source]; ok {
					propagatedEnergy += upstreamEnergy * (edge.Weight * 0.3)
				}
			}
		}

		totalDisturbanceEnergy := localEnergy + propagatedEnergy

		sig := signals[id]
		sig.DisturbanceEnergy = totalDisturbanceEnergy

		// Legacy projection mapped to preserve existing controller integrations exactly
		decay := 0.0
		if simResult != nil && simResult.RecoveryConvergenceMs > 0 {
			decay = math.Max(0, 1.0-(simResult.RecoveryConvergenceMs/5000.0))
		}
		if sig.UtilisationGrad < -0.01 && sig.QueueVelocity <= 0.0 {
			decay += 0.5
		}
		activation := 0.0
		if decay > 0.2 && sig.StabilityScore > 0.3 && sig.QueueVelocity <= 0.05 {
			activation = math.Min(decay*sig.StabilityScore, 1.0)
		}
		sig.RecoveryActivation = activation
		sig.StabilityEnvelope = sig.StabilityScore
		sig.DisturbanceDecay = decay
		signals[id] = sig

		// G. Observer Signal Integrity Logging
		log.Printf("[observer] svc=%s queue_velocity=%.3f utilisation_gradient=%.3f disturbance_energy=%.3f stability_score=%.3f",
			id, sig.QueueVelocity, sig.UtilisationGrad, sig.DisturbanceEnergy, sig.StabilityScore)
	}

	// Prune dead services
	for id := range o.states {
		if _, exists := bundles[id]; !exists {
			delete(o.states, id)
		}
	}

	return signals
}
