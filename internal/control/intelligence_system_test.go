package control_test

import (
	"math"
	"testing"
	"time"

	ctrl "github.com/loadequilibrium/loadequilibrium/internal/control"
	
)

func baseState() ctrl.SystemState {
	return ctrl.SystemState{
		Replicas:         5,
		QueueLimit:       50,
		RetryLimit:       3,
		CacheAggression:  0.2,
		QueueDepth:       20,
		PredictedArrival: 40,
		ArrivalRate:      40,
		ServiceRate:      10,
		Latency:          100,
		SLATarget:        120,
		MinReplicas:      1,
		MaxReplicas:      50,
		MinRetry:         1,
		MaxRetry:         10,
	}
}

func Test_Intelligence_SignalSensitivity(t *testing.T) {
	a := ctrl.NewAuthority()

	state := baseState()

	lowLoad := a.Decide(ctrl.AuthorityInput{
		ServiceID: "svc",
		Tick:      1,
		Now:       time.Now(),
		State:     state,
		Config:    ctrl.AuthorityConfig{TargetUtilisation: 0.7},
	}).Directive.ScaleFactor

	// simulate high load
	state.QueueDepth = 200
	state.PredictedArrival = 150
	state.Utilisation = 2.0
	state.Risk = 0.8

	highLoad := a.Decide(ctrl.AuthorityInput{
		ServiceID: "svc",
		Tick:      2,
		Now:       time.Now(),
		State:     state,
		Config:    ctrl.AuthorityConfig{TargetUtilisation: 0.7},
	}).Directive.ScaleFactor

	if highLoad <= lowLoad {
		t.Fatalf("system not responding to signals: low=%.2f high=%.2f", lowLoad, highLoad)
	}
}

func Test_Intelligence_Adaptation(t *testing.T) {
	a := ctrl.NewAuthority()
	state := baseState()

	var prev float64

	for i := 1; i <= 10; i++ {
		state.QueueDepth += 10 // gradually increasing stress
		state.Risk += 0.05

		d := a.Decide(ctrl.AuthorityInput{
			ServiceID: "adaptive",
			Tick:      uint64(i),
			State:     state,
		}).Directive.ScaleFactor

		if i > 1 && d < prev {
			t.Fatalf("non-adaptive behavior: scale decreased under increasing stress")
		}

		prev = d
	}
}

func Test_Intelligence_Smoothness(t *testing.T) {
	a := ctrl.NewAuthority()
	state := baseState()

	var prev float64
	totalOsc := 0.0

	for i := 1; i <= 20; i++ {

		// small noise
		state.QueueDepth = 50 + float64(i%3)
		state.Risk = 0.3

		d := a.Decide(ctrl.AuthorityInput{
			ServiceID: "smooth",
			Tick:      uint64(i),
			State:     state,
		}).Directive.ScaleFactor

		if prev != 0 {
			step := math.Abs(d - prev)
			totalOsc += step
		}

		prev = d
	}

	if totalOsc > 3.0 {
		t.Fatalf("system oscillating too much: %.2f", totalOsc)
	}
}

func Test_Intelligence_StressResponse(t *testing.T) {
	a := ctrl.NewAuthority()
	state := baseState()

	state.QueueDepth = 500
	state.PredictedArrival = 300
	state.Utilisation = 3.0
	state.Risk = 0.9

	d := a.Decide(ctrl.AuthorityInput{
		ServiceID: "stress",
		Tick:      1,
		State:     state,
	}).Directive.ScaleFactor

	if d <= 1.0 {
		t.Fatalf("system failed to scale under stress: %.2f", d)
	}
}

func Test_Intelligence_Recovery(t *testing.T) {
	a := ctrl.NewAuthority()
	state := baseState()

	// first overload
	state.QueueDepth = 300
	state.Risk = 0.8

	high := a.Decide(ctrl.AuthorityInput{
		ServiceID: "recovery",
		Tick:      1,
		State:     state,
	}).Directive.ScaleFactor

	// then recovery
	state.QueueDepth = 20
	state.Risk = 0.2

	low := a.Decide(ctrl.AuthorityInput{
		ServiceID: "recovery",
		Tick:      2,
		State:     state,
	}).Directive.ScaleFactor

	if low >= high {
		t.Fatalf("system not recovering: high=%.2f low=%.2f", high, low)
	}
}

func Test_Intelligence_EndToEnd(t *testing.T) {
	controller := ctrl.Controller{
		OptimizerCfg: ctrl.OptimizerConfig{
			ScenarioCount: 5,
		},
	}

	sys := baseState()

	var prevReplicas float64
	osc := 0.0

	for step := 0; step < 100; step++ {

		// dynamic load pattern
		if step > 30 && step < 70 {
			sys.PredictedArrival = 120
		} else {
			sys.PredictedArrival = 40
		}

		controller.Tick(
			&sys,
			sys.QueueDepth,
			0.1,
			0.2,
			1,
			float64(sys.Replicas),
		)

		if prevReplicas != 0 {
			osc += math.Abs(float64(sys.Replicas) - prevReplicas)
		}

		prevReplicas = float64(sys.Replicas)

		// backlog evolution
		sys.QueueDepth += sys.PredictedArrival - float64(sys.Replicas)*sys.ServiceRate
		if sys.QueueDepth < 0 {
			sys.QueueDepth = 0
		}
	}

	if osc > 50 {
		t.Fatalf("too unstable system: oscillation %.2f", osc)
	}

	if sys.QueueDepth > 5000 {
		t.Fatalf("system failed to control backlog: %.2f", sys.QueueDepth)
	}
}
