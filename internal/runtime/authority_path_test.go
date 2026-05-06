package runtime

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/loadequilibrium/loadequilibrium/internal/config"
	"github.com/loadequilibrium/loadequilibrium/internal/modelling"
	"github.com/loadequilibrium/loadequilibrium/internal/optimisation"
	"github.com/loadequilibrium/loadequilibrium/internal/topology"
)

func TestRuntimeHasNoLateFusionAuthority(t *testing.T) {
	phaseSrc := readSource(t, "phase_runtime.go")
	orchSrc := readSource(t, "orchestrator.go")

	for _, forbidden := range []string{
		"mergeDirective",
		"computePrecisionFusedScale",
		"uPID",
		"uMPC",
		"uPolicy",
		"uSandbox",
		"uIntel",
	} {
		if strings.Contains(phaseSrc, forbidden) {
			t.Fatalf("late-fusion authority token still present: %s", forbidden)
		}
	}
	if strings.Contains(orchSrc, ".RunControl(") {
		t.Fatalf("runtime must not call optimisation.RunControl for executable directives")
	}
	if strings.Contains(orchSrc, ".actuator.Dispatch(") {
		t.Fatalf("runtime must dispatch through control.Dispatch, not direct actuator authority")
	}
	if !strings.Contains(orchSrc, "ctrl.Dispatch(") {
		t.Fatalf("runtime actuator handoff must go through control.Dispatch")
	}
}

func TestPhaseRuntimeAdvisoryToControlPipelineDeterministic(t *testing.T) {
	cfg := &config.Config{
		TickInterval:           time.Second,
		UtilisationSetpoint:    0.70,
		CollapseThreshold:      0.90,
		PredictiveHorizonTicks: 4,
	}
	now := time.Unix(100, 0)
	bundles := map[string]*modelling.ServiceModelBundle{
		"svc": {
			Queue: modelling.QueueModel{
				ServiceID:        "svc",
				ArrivalRate:      40,
				ServiceRate:      50,
				Concurrency:      10,
				Utilisation:      0.8,
				MeanQueueLen:     30,
				AdjustedWaitMs:   300,
				UtilisationTrend: 0.01,
			},
			Stochastic: modelling.StochasticModel{ServiceID: "svc", ArrivalCoV: 0.2, BurstAmplification: 0.3, Confidence: 0.8},
			Signal:     modelling.SignalState{ServiceID: "svc", FastEWMA: 40, SlowEWMA: 38},
			Stability: modelling.StabilityAssessment{
				ServiceID:                 "svc",
				StabilityMargin:           0.2,
				CollapseRisk:              0.35,
				CollapseZone:              "warning",
				TrendAdjustedMargin:       0.1,
				StabilityDerivative:       0.02,
				CascadeAmplificationScore: 0.1,
			},
		},
	}
	objective := optimisation.ComputeObjective(bundles, topology.GraphSnapshot{}, now)
	candidates := map[string][]optimisation.ControlCandidate{
		"svc": {
			{ServiceID: "svc", ScaleFactor: 1.2, Score: 0.1, Feasible: true},
			{ServiceID: "svc", ScaleFactor: 0.6, Score: 1.0, Feasible: true},
		},
	}

	first := runDeterministicPhaseApply(cfg, 1, now, bundles, objective, candidates)
	second := runDeterministicPhaseApply(cfg, 1, now, bundles, objective, candidates)

	if len(first) != 1 || len(second) != 1 {
		t.Fatalf("expected one directive per service: first=%d second=%d", len(first), len(second))
	}
	d1 := first["svc"]
	d2 := second["svc"]
	if !d1.Active || !d2.Active {
		t.Fatalf("control authority produced inactive directive: %+v %+v", d1, d2)
	}
	if d1.ScaleFactor <= 0 || d2.ScaleFactor <= 0 {
		t.Fatalf("invalid scale output: %.3f %.3f", d1.ScaleFactor, d2.ScaleFactor)
	}
	if d1.HysteresisState != "control-authority" || d2.HysteresisState != "control-authority" {
		t.Fatalf("directive did not originate from control authority: %+v %+v", d1, d2)
	}
	if d1.ScaleFactor != d2.ScaleFactor {
		t.Fatalf("same tick/input should be deterministic: %.6f != %.6f", d1.ScaleFactor, d2.ScaleFactor)
	}
}

func runDeterministicPhaseApply(
	cfg *config.Config,
	tick uint64,
	now time.Time,
	bundles map[string]*modelling.ServiceModelBundle,
	objective optimisation.ObjectiveScore,
	candidates map[string][]optimisation.ControlCandidate,
) map[string]optimisation.ControlDirective {
	p := newPhaseRuntime(cfg)
	svc := p.ensureService("svc")
	svc.autopilot.MPC.Deterministic = true
	svc.autopilot.FailureScaleProb = 0
	svc.autopilot.FailureConfigProb = 0
	return p.apply(tick, now, bundles, objective, candidates)
}

func readSource(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}
