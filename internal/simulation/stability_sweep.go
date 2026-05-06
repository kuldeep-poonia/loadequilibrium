package simulation

import (
	"fmt"
	"github.com/loadequilibrium/loadequilibrium/internal/physics"
)

/*
# Fluid Plant & Arrival Persistence Walkthrough [LATEST]

## 1. Arrival Regime Persistence [NEW]
Improved the arrival dynamics in `physics.FluidPlant` by implementing a regime-holding process that ensures overloads persist long enough to build significant congestion.

### Improvements
- **HighPhasePersistence**: Introduced a parameter to control how much longer the high-load phase lasts compared to the low-to-high transition.
- **Dynamic Transitions**: Replaced the static `Mu` probability with a derived rate: $\mu = \lambda / \text{HighPhasePersistence}$.
- **Metastable Trajectories**: This change ensures that once the system enters a high-load state, it stays there long enough to challenge the dissipative service capacity, leading to more realistic congestion events.

## 2. Seeded Stability Sweep
*/
// RunStabilitySweep performs a parameter sweep of InflowMeanHigh to evaluate
// queue stability in the physics plant.
func RunStabilitySweep() {
	fmt.Println("[sweep] starting stability calibration sweep...")
	for inflow := 0.80; inflow <= 1.401; inflow += 0.05 {
		plant := physics.NewFluidPlant(1337)
		plant.P.InflowMeanHigh = inflow
		// Seed congestion state
		plant.Q = 5.0
		plant.Z = 0.5
		plant.R = 1.0

		const steps = 50000
		const dt = 0.05
		var sumQ float64
		maxQ := 0.0

		for i := 0; i < steps; i++ {
			plant.Step(dt)
			q := plant.Q
			sumQ += q
			if q > maxQ {
				maxQ = q
			}
		}

		avgQ := sumQ / float64(steps)
		fmt.Printf("[sweep] inflow=%.2f avgQ=%.3f maxQ=%.3f\n", inflow, avgQ, maxQ)
	}
}

// RunStabilitySweep2D performs a 2D parameter sweep of BaseService and InflowMeanHigh.
func RunStabilitySweep2D() {
	fmt.Println("[sweep2d] starting 2D stability calibration sweep...")
	for svc := 1.2; svc <= 2.201; svc += 0.2 {
		for inflow := 0.8; inflow <= 1.401; inflow += 0.1 {
			plant := physics.NewFluidPlant(1337)
			plant.P.BaseService = svc
			plant.P.InflowMeanHigh = inflow
			// Seed congestion state
			plant.Q = 5.0
			plant.Z = 0.5
			plant.R = 1.0

			const steps = 50000
			const dt = 0.05
			var sumQ float64
			maxQ := 0.0

			for i := 0; i < steps; i++ {
				plant.Step(dt)
				q := plant.Q
				sumQ += q
				if q > maxQ {
					maxQ = q
				}
			}

			avgQ := sumQ / float64(steps)
			fmt.Printf("[sweep2d] service=%.2f inflow=%.2f avgQ=%.3f maxQ=%.3f\n", svc, inflow, avgQ, maxQ)
		}
	}
}
