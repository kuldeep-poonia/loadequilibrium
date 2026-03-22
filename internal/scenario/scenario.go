package scenario

import (
	"math"

	"github.com/loadequilibrium/loadequilibrium/internal/telemetry"
	"github.com/loadequilibrium/loadequilibrium/internal/topology"
)

type Disturbance struct {
	ScenarioID          string
	ServiceID           string
	Metric              string // "RequestRate", "Latency"
	Factor              float64
	Active              bool
	Phase               string // "ramp", "peak", "decay"
	PropagationDelayEst float64 // ms
	PropagationDepth    int
}

type Scenario interface {
	Evaluate(tick uint64, topo topology.GraphSnapshot, windows map[string]*telemetry.ServiceWindow) []Disturbance
}

type ResettableBurst struct {
	ScenarioID    string
	TargetService string
	StartTick     uint64
	DurationTicks uint64
	MaxFactor     float64
	RepeatEvery   uint64 // 0 for no repeat
}

func (s *ResettableBurst) Evaluate(tick uint64, topo topology.GraphSnapshot, windows map[string]*telemetry.ServiceWindow) []Disturbance {
	if tick < s.StartTick {
		return nil
	}
	
	cycleTick := tick - s.StartTick
	if s.RepeatEvery > 0 {
		cycleTick = cycleTick % s.RepeatEvery
	}

	if cycleTick > s.DurationTicks {
		return nil
	}

	relativeTick := float64(cycleTick)
	
	rampTicks := float64(s.DurationTicks) * 0.2
	decayTicks := float64(s.DurationTicks) * 0.2
	peakEnd := float64(s.DurationTicks) - decayTicks

	var factor float64
	phase := "peak"

	if relativeTick < rampTicks {
		factor = 1.0 + (s.MaxFactor-1.0)*math.Sin(math.Pi/2 * (relativeTick/rampTicks))
		phase = "ramp"
	} else if relativeTick > peakEnd {
		rem := float64(s.DurationTicks) - relativeTick
		factor = 1.0 + (s.MaxFactor-1.0)*math.Sin(math.Pi/2 * (rem/decayTicks))
		phase = "decay"
	} else {
		factor = s.MaxFactor
	}

	fanOutLimit := 3
	propDelay := factor * 2.5 * float64(fanOutLimit)

	return []Disturbance{
		{
			ScenarioID:          s.ScenarioID,
			ServiceID:           s.TargetService,
			Metric:              "RequestRate",
			Factor:              factor,
			Active:              true,
			Phase:               phase,
			PropagationDepth:    fanOutLimit,
			PropagationDelayEst: propDelay,
		},
	}
}
