package scenario

import (
	"math"

	"github.com/loadequilibrium/loadequilibrium/internal/telemetry"
	"github.com/loadequilibrium/loadequilibrium/internal/topology"
)

// ConnectivityProof generates a deterministic baseline scenario to prove
// the backend → dashboard pipeline is semantically connected.
//
// Pattern: 8 services, 40 RPS baseline → 70 RPS burst (5s) → recovery (20s), repeat 30s
type ConnectivityProof struct {
	startTick uint64
}

// NewConnectivityProof creates a continuous baseline connectivity proof scenario
func NewConnectivityProof(startTick uint64) *ConnectivityProof {
	return &ConnectivityProof{
		startTick: startTick,
	}
}

// Evaluate applies a deterministic burst pattern for 8 services
// Tick 0-10: 40 RPS baseline
// Tick 10-25: 70 RPS burst (smooth curve)
// Tick 25-50: Recovery back to 40 RPS
// Repeating every 50 ticks (100 seconds with 2s ticks)
func (cp *ConnectivityProof) Evaluate(tick uint64, topo topology.GraphSnapshot, windows map[string]*telemetry.ServiceWindow) []Disturbance {
	if tick < cp.startTick {
		return nil
	}

	// Cycle every 50 ticks (100 seconds)
	cycleTick := (tick - cp.startTick) % 50

	services := []string{
		"api-gateway",
		"auth-service",
		"backend-primary",
		"backend-secondary",
		"database",
		"cache-layer",
		"messaging-queue",
		"analytics-service",
	}

	var disturbances []Disturbance

	for _, svc := range services {
		var factor float64
		var phase string

		switch {
		case cycleTick < 10:
			// 40 RPS baseline (factor = 1.0 baseline + burst)
			factor = 1.0
			phase = "baseline"

		case cycleTick >= 10 && cycleTick < 25:
			// 70 RPS burst (7 ticks duration, smooth curve)
			relTick := float64(cycleTick - 10)
			burstPhase := relTick / 15.0 // 0 to 1 over 15 ticks

			// Smooth sinusoidal ramp up to 1.75x (70 RPS = 1.75 * 40 RPS)
			if burstPhase < 0.3 {
				// Ramp up phase (0-0.3): 0 to 1.75x
				factor = 1.0 + 0.75*math.Sin(math.Pi/2*(burstPhase/0.3))
				phase = "ramp"
			} else if burstPhase < 0.7 {
				// Peak phase (0.3-0.7): stay at 1.75x
				factor = 1.75
				phase = "peak"
			} else {
				// Decay phase (0.7-1.0): 1.75x back to 1.0x
				rem := 1.0 - burstPhase
				factor = 1.0 + 0.75*math.Sin(math.Pi/2*(rem/0.3))
				phase = "decay"
			}

		case cycleTick >= 25 && cycleTick < 50:
			// Recovery phase: smooth decay back to baseline
			relTick := float64(cycleTick - 25)
			recoveryPhase := relTick / 25.0 // 0 to 1 over 25 ticks (50-25)

			// Smooth exponential decay from any elevated level back to 1.0
			decayFactor := math.Exp(-3.0 * recoveryPhase) // exp decay
			factor = 1.0 + 0.75*(decayFactor-1.0)
			phase = "recovery"
		default:
			factor = 1.0
			phase = "baseline"
		}

		disturbances = append(disturbances, Disturbance{
			ScenarioID:          "connectivity-proof",
			ServiceID:           svc,
			Metric:              "RequestRate",
			Factor:              factor,
			Active:              true,
			Phase:               phase,
			PropagationDelayEst: 2.5,
			PropagationDepth:    1,
		})
	}

	return disturbances
}
