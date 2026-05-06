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

type WindowedDisturbance struct {
	ScenarioID    string
	TargetService string
	StartTick     uint64
	DurationTicks uint64
	RequestFactor float64
	LatencyFactor float64
	RepeatEvery   uint64
}

func disturbanceEnvelope(cycleTick, duration uint64) (float64, string) {
	if duration == 0 {
		return 1.0, "peak"
	}

	relativeTick := float64(cycleTick)
	rampTicks := math.Max(float64(duration)*0.2, 1)
	decayTicks := math.Max(float64(duration)*0.2, 1)
	peakEnd := math.Max(float64(duration)-decayTicks, rampTicks)

	if relativeTick < rampTicks {
		return math.Sin(math.Pi / 2 * (relativeTick / rampTicks)), "ramp"
	}
	if relativeTick > peakEnd {
		remaining := float64(duration) - relativeTick
		return math.Sin(math.Pi / 2 * (remaining / decayTicks)), "decay"
	}
	return 1.0, "peak"
}

func (s *WindowedDisturbance) Evaluate(tick uint64, topo topology.GraphSnapshot, windows map[string]*telemetry.ServiceWindow) []Disturbance {
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

	envelope, phase := disturbanceEnvelope(cycleTick, s.DurationTicks)
	disturbances := make([]Disturbance, 0, 2)

	if s.RequestFactor != 0 && math.Abs(s.RequestFactor-1.0) > 1e-9 {
		reqFactor := 1.0 + (s.RequestFactor-1.0)*envelope
		disturbances = append(disturbances, Disturbance{
			ScenarioID:          s.ScenarioID,
			ServiceID:           s.TargetService,
			Metric:              "RequestRate",
			Factor:              reqFactor,
			Active:              true,
			Phase:               phase,
			PropagationDepth:    2,
			PropagationDelayEst: reqFactor * 2.0,
		})
	}

	if s.LatencyFactor != 0 && math.Abs(s.LatencyFactor-1.0) > 1e-9 {
		latFactor := 1.0 + (s.LatencyFactor-1.0)*envelope
		disturbances = append(disturbances, Disturbance{
			ScenarioID:          s.ScenarioID,
			ServiceID:           s.TargetService,
			Metric:              "Latency",
			Factor:              latFactor,
			Active:              true,
			Phase:               phase,
			PropagationDepth:    2,
			PropagationDelayEst: latFactor * 2.0,
		})
	}

	return disturbances
}
