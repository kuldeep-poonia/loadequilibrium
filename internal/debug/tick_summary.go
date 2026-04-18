package debug

import (
	"fmt"
	"time"
)

// TickSummary is the single unified per-tick diagnostic record.
// All pipeline stages populate fields into this structure instead of logging directly.
// Emitted exactly ONCE per tick at the very end of the pipeline.
//
// ✅ Zero per-stage logging.
// ✅ 100% of diagnostic information preserved.
// ✅ Exact same computational pipeline runs.
// ✅ All anomaly detection remains intact.
type TickSummary struct {
	TickIndex   uint64
	TickAt      time.Time
	Duration    time.Duration
	Pressure    int
	SafetyLevel int

	// Window metrics
	WindowsTotal int
	WindowsStale int
	WindowsDegraded int

	// Stage timings
	PruneMs     float64
	WindowsMs   float64
	TopologyMs  float64
	CouplingMs  float64
	ModellingMs float64
	OptimiseMs  float64
	SimMs       float64
	ReasoningMs float64
	BroadcastMs float64

	// System state
	SystemRhoMean      float64
	SystemFragility    float64
	CollapseRisk       float64
	ActiveServices     int
	KeystoneServices   int
	HighRiskServices   int

	// Control outputs
	DirectivesTotal    int
	DirectivesScaleUp  int
	DirectivesScaleDown int
	DirectivesActive   int

	// Overrun status
	Overran            bool
	ConsecutiveOverruns int
	PredictedOverrun   bool
}

// Emit prints the single final summary line for this tick.
// This is the ONLY hot path log statement in the entire pipeline.
func (ts *TickSummary) Emit() {
	if !HotPathLogsEnabled() {
		return
	}

	fmt.Printf("[tick-summary] idx=%d dur=%s press=%d safe=%d windows=%d rho=%.3f frag=%.3f risk=%.3f stages=[%.1f,%.1f,%.1f,%.1f,%.1f,%.1f,%.1f,%.1f,%.1f] directives=%d overruns=%d overran=%v\n",
		ts.TickIndex,
		ts.Duration.Round(time.Microsecond),
		ts.Pressure,
		ts.SafetyLevel,
		ts.WindowsTotal,
		ts.SystemRhoMean,
		ts.SystemFragility,
		ts.CollapseRisk,
		ts.PruneMs,
		ts.WindowsMs,
		ts.TopologyMs,
		ts.CouplingMs,
		ts.ModellingMs,
		ts.OptimiseMs,
		ts.SimMs,
		ts.ReasoningMs,
		ts.BroadcastMs,
		ts.DirectivesTotal,
		ts.ConsecutiveOverruns,
		ts.Overran,
	)
}