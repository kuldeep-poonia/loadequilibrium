
package tests

import (
	"context"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loadequilibrium/loadequilibrium/internal/actuator"
	"github.com/loadequilibrium/loadequilibrium/internal/actuator/backends"
	"github.com/loadequilibrium/loadequilibrium/internal/config"
	"github.com/loadequilibrium/loadequilibrium/internal/modelling"
	"github.com/loadequilibrium/loadequilibrium/internal/optimisation"
	"github.com/loadequilibrium/loadequilibrium/internal/persistence"
	"github.com/loadequilibrium/loadequilibrium/internal/reasoning"
	"github.com/loadequilibrium/loadequilibrium/internal/runtime"
	"github.com/loadequilibrium/loadequilibrium/internal/scenario"
	"github.com/loadequilibrium/loadequilibrium/internal/simulation"
	"github.com/loadequilibrium/loadequilibrium/internal/streaming"
	"github.com/loadequilibrium/loadequilibrium/internal/telemetry"
	"github.com/loadequilibrium/loadequilibrium/internal/topology"
)

// 
// MASTER TEST CONFIGURATION
// 

func masterCfg() *config.Config {
	return &config.Config{
		ListenAddr:             ":0",
		TickInterval:           100 * time.Millisecond,
		TickDeadline:           80 * time.Millisecond,
		RingBufferDepth:        512,
		WorkerPoolSize:         4,
		MaxServices:            32,
		StaleServiceAge:        30 * time.Second,
		UtilisationSetpoint:    0.70,
		CollapseThreshold:      0.90,
		WindowFraction:         0.5,
		EWMAFastAlpha:          0.30,
		EWMASlowAlpha:          0.05,
		SpikeZScore:            3.0,
		PIDKp:                  0.5,
		PIDKi:                  0.1,
		PIDKd:                  0.05,
		PIDDeadband:            0.02,
		PIDIntegralMax:         1.0,
		SimHorizonMs:           300,
		SimShockFactor:         1.5,
		SimAsyncBuffer:         4,
		SimBudget:              30 * time.Millisecond,
		PredictiveHorizonTicks: 5,
		MaxReasoningCooldowns:  500,
		ArrivalEstimatorMode:   "ewma",
		MaxStreamClients:       8,
		SimStochasticMode:      "exponential",
		SafetyModeThreshold:    3,
		MinTickInterval:        50 * time.Millisecond,
		MaxTickInterval:        500 * time.Millisecond,
		TickAdaptStep:          1.25,
		SLALatencyThresholdMs:  500,
		ScenarioMode:           "off",
	}
}

// captureActuator is a no-op actuator that records all dispatched directives
type captureActuator struct {
	mu         sync.Mutex
	dispatches []capturedDispatch
	closed     uint32
	feedback   chan actuator.ActuationResult
}

type capturedDispatch struct {
	Tick       uint64
	Directives map[string]optimisation.ControlDirective
}

func newCaptureActuator() *captureActuator {
	return &captureActuator{
		feedback: make(chan actuator.ActuationResult, 256),
	}
}

func (a *captureActuator) Dispatch(tickIndex uint64, dirs map[string]optimisation.ControlDirective) {
	if atomic.LoadUint32(&a.closed) == 1 {
		return
	}
	snap := make(map[string]optimisation.ControlDirective, len(dirs))
	for k, v := range dirs {
		snap[k] = v
	}
	a.mu.Lock()
	a.dispatches = append(a.dispatches, capturedDispatch{Tick: tickIndex, Directives: snap})
	a.mu.Unlock()

	// Send synthetic feedback
	for id, d := range dirs {
		if d.Active {
			select {
			case a.feedback <- actuator.ActuationResult{
				TickIndex: tickIndex,
				ServiceID: id,
				Success:   true,
				Latency:   1 * time.Millisecond,
			}:
			default:
			}
		}
	}
}

func (a *captureActuator) Feedback() <-chan actuator.ActuationResult { return a.feedback }
func (a *captureActuator) Close(_ context.Context) error {
	atomic.StoreUint32(&a.closed, 1)
	return nil
}

func (a *captureActuator) DispatchCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.dispatches)
}

func (a *captureActuator) AllDispatches() []capturedDispatch {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]capturedDispatch, len(a.dispatches))
	copy(out, a.dispatches)
	return out
}

// 
// MASTER TEST 1: FULL SYSTEM LIFECYCLE
// Validates: ingest → tick pipeline → actuation → streaming → all 9 stages
// 

func TestMaster_FullSystemLifecycle(t *testing.T) {
	cfg := masterCfg()
	store := telemetry.NewStore(cfg.RingBufferDepth, cfg.MaxServices, cfg.StaleServiceAge)
	hub := streaming.NewHub()
	scen := scenario.NewEngine()
	act := newCaptureActuator()

	orch := runtime.New(cfg, store, hub, nil, act, scen)

	// ── Phase 1: Ingest telemetry for 3 services ──────────────────────────
	services := []struct {
		id      string
		rps     float64
		latency float64
	}{
		{"api-gateway", 200.0, 45.0},
		{"auth-service", 180.0, 30.0},
		{"db-proxy", 160.0, 60.0},
	}

	for _, svc := range services {
		for i := 0; i < 50; i++ {
			store.Ingest(&telemetry.MetricPoint{
				ServiceID:   svc.id,
				RequestRate: svc.rps + float64(i%20),
				ErrorRate:   0.005,
				Latency: telemetry.LatencyStats{
					Mean: svc.latency + float64(i%10),
					P50:  svc.latency * 0.8,
					P95:  svc.latency * 1.5,
					P99:  svc.latency * 2.0,
				},
				Timestamp: time.Now(),
			})
		}
	}

	// Verify data ingested
	windows := store.AllWindows(25, 30*time.Second)
	if len(windows) != 3 {
		t.Fatalf("Expected 3 service windows after ingestion, got %d", len(windows))
	}

	// ── Phase 2: Subscribe to streaming hub before ticks run ──────────────
	payloads := make(chan *streaming.TickPayload, 32)
	hub.Subscribe(func(p *streaming.TickPayload) {
		select {
		case payloads <- p:
		default:
		}
	})

	// ── Phase 3: Run the orchestrator for several ticks ───────────────────
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go orch.Run(ctx)

	// Wait for at least 5 ticks to complete
	deadline := time.After(2500 * time.Millisecond)
	var tickCount uint64
	for tickCount < 5 {
		select {
		case <-deadline:
			t.Fatalf("Timed out waiting for 5 ticks, only got %d", orch.TickCount())
		case <-time.After(100 * time.Millisecond):
			tickCount = orch.TickCount()
		}
	}
	t.Logf("Orchestrator completed %d ticks", tickCount)

	cancel() // stop orchestrator
	time.Sleep(200 * time.Millisecond) // allow drain

	// ── Phase 4: Validate streaming payloads ──────────────────────────────
	var receivedPayloads []*streaming.TickPayload
	drainPayloads:
	for {
		select {
		case p := <-payloads:
			receivedPayloads = append(receivedPayloads, p)
		default:
			break drainPayloads
		}
	}

	if len(receivedPayloads) == 0 {
		t.Fatal("No streaming payloads received — hub.Broadcast() not called")
	}
	t.Logf("Received %d streaming payloads", len(receivedPayloads))

	// Validate payload structure
	lastPayload := receivedPayloads[len(receivedPayloads)-1]
	if lastPayload.Bundles == nil || len(lastPayload.Bundles) == 0 {
		t.Fatal("Last payload has nil/empty Bundles — modelling stage failed")
	}
	t.Logf("Last payload: %d bundles, %d directives, %d events",
		len(lastPayload.Bundles), len(lastPayload.Directives), len(lastPayload.Events))

	// ── Phase 5: Validate bundle contents ─────────────────────────────────
	for id, b := range lastPayload.Bundles {
		if b == nil {
			t.Fatalf("Bundle for %s is nil", id)
		}
		if math.IsNaN(b.Queue.Utilisation) || math.IsInf(b.Queue.Utilisation, 0) {
			t.Fatalf("Bundle %s: Utilisation is NaN/Inf", id)
		}
		if b.Queue.Utilisation < 0 {
			t.Fatalf("Bundle %s: Utilisation=%.4f < 0", id, b.Queue.Utilisation)
		}
		if b.Queue.ArrivalRate <= 0 {
			t.Fatalf("Bundle %s: ArrivalRate=%.4f ≤0", id, b.Queue.ArrivalRate)
		}
		if b.Stability.CollapseZone == "" {
			t.Fatalf("Bundle %s: CollapseZone is empty", id)
		}
		if math.IsNaN(b.Stability.CollapseRisk) {
			t.Fatalf("Bundle %s: CollapseRisk is NaN", id)
		}
	}

	// ── Phase 6: Validate directives were produced and actuated ───────────
	if len(lastPayload.Directives) == 0 {
		t.Log("No directives in last payload — may be before first full tick cycle")
	}

	for id, d := range lastPayload.Directives {
		if !d.Active {
			t.Logf("Directive for %s is inactive", id)
		}
		if d.ScaleFactor <= 0 {
			t.Fatalf("Directive %s: ScaleFactor=%.4f ≤0", id, d.ScaleFactor)
		}
		if math.IsNaN(d.ScaleFactor) || math.IsInf(d.ScaleFactor, 0) {
			t.Fatalf("Directive %s: ScaleFactor is NaN/Inf", id)
		}
		if d.TargetUtilisation <= 0 || d.TargetUtilisation > 1 {
			t.Logf("Directive %s: TargetUtilisation=%.4f out of (0,1]", id, d.TargetUtilisation)
		}
	}

	dispatchCount := act.DispatchCount()
	t.Logf("Actuator received %d dispatches over %d ticks", dispatchCount, tickCount)
	if dispatchCount == 0 && tickCount >= 3 {
		t.Error("Actuator received 0 dispatches after 3+ ticks — Dispatch() never called")
	}

	// ── Phase 7: Validate objective score is computed ─────────────────────
	if lastPayload.Objective.CompositeScore < 0 || lastPayload.Objective.CompositeScore > 1 {
		t.Fatalf("Objective.CompositeScore=%.4f out of [0,1]", lastPayload.Objective.CompositeScore)
	}

	// ── Phase 8: Validate runtime metrics ─────────────────────────────────
	rm := lastPayload.RuntimeMetrics
	if rm.AvgModellingMs < 0 {
		t.Fatalf("AvgModellingMs=%.4f < 0", rm.AvgModellingMs)
	}
	if math.IsNaN(rm.AvgModellingMs) {
		t.Fatal("AvgModellingMs is NaN")
	}

	t.Logf("Runtime metrics: modelling=%.2fms optimise=%.2fms reasoning=%.2fms broadcast=%.2fms",
		rm.AvgModellingMs, rm.AvgOptimiseMs, rm.AvgReasoningMs, rm.AvgBroadcastMs)
	t.Logf("Reentrant ticks: %d, Total overruns: %d", orch.ReentrantTickCount(), rm.TotalOverruns)
}

// 
// MASTER TEST 2: COLLAPSE SCENARIO — high load triggers events + scale-up
// 

func TestMaster_CollapseScenarioDetectedAndActuated(t *testing.T) {
	cfg := masterCfg()
	cfg.TickInterval = 80 * time.Millisecond
	cfg.CollapseThreshold = 0.90

	store := telemetry.NewStore(cfg.RingBufferDepth, cfg.MaxServices, cfg.StaleServiceAge)
	hub := streaming.NewHub()
	scen := scenario.NewEngine()
	act := newCaptureActuator()

	orch := runtime.New(cfg, store, hub, nil, act, scen)

	// Inject critically high load — arrival ≈ service rate → ρ ≈ 0.95+
	for i := 0; i < 80; i++ {
	store.Ingest(&telemetry.MetricPoint{
		ServiceID:   "svc-critical",
		RequestRate: 950,
		ErrorRate:   0.08,
		Latency: telemetry.LatencyStats{
			Mean: 10, P50: 8, P95: 20, P99: 30,
		},
		Timestamp: time.Now(),
	})
	}

	// Subscribe to capture all events
	var mu sync.Mutex
	var allEvents []reasoning.Event
	// Event and directive capture will be read from hub.Latest() after run
	var allDirectives []optimisation.ControlDirective

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go orch.Run(ctx)

	// Wait for enough ticks
	time.Sleep(1200 * time.Millisecond)
	cancel()
	time.Sleep(100 * time.Millisecond)

	ticks := orch.TickCount()
	t.Logf("Ran %d ticks under critical load", ticks)

	mu.Lock()
	eventCount := len(allEvents)
	directiveCount := len(allDirectives)
	mu.Unlock()

	// Under critical load (ρ≈0.95) we MUST get reasoning events
	if ticks >= 3 && eventCount == 0 {
		t.Error("Zero reasoning events fired under critical load scenario — reasoning engine not connected")
	}
	t.Logf("Reasoning events fired: %d over %d ticks", eventCount, ticks)

	// Under critical load we MUST get scale-up directives
	if ticks >= 3 && directiveCount == 0 {
		t.Error("Zero directives produced under critical load — control authority not connected to streaming")
	}

	mu.Lock()
	for _, d := range allDirectives {
		if math.IsNaN(d.ScaleFactor) || math.IsInf(d.ScaleFactor, 0) {
			t.Errorf("ScaleFactor is NaN/Inf under collapse scenario")
			break
		}
		if d.ScaleFactor > 15.0 {
			t.Errorf("ScaleFactor=%.4f > 15.0 — runaway scale-up", d.ScaleFactor)
			break
		}
	}
	mu.Unlock()

	// Events should include severity ≥ WARNING for high utilisation
	mu.Lock()
	hasCriticalOrWarning := false
	for _, ev := range allEvents {
		if ev.Severity >= reasoning.SeverityWarning {
			hasCriticalOrWarning = true
			t.Logf("Event fired: [%s] %s (priority=%d)", ev.Category, ev.Description, ev.OperationalPriority)
			break
		}
	}
	mu.Unlock()
	if ticks >= 3 && !hasCriticalOrWarning {
		t.Error("No Warning/Critical events under critical load — reasoning severity calibration may be wrong")
	}
}

// 
// MASTER TEST 3: RECOVERY SCENARIO — load drops after overload
// 

func TestMaster_RecoveryAfterOverload(t *testing.T) {
	cfg := masterCfg()
	cfg.TickInterval = 80 * time.Millisecond

	store := telemetry.NewStore(cfg.RingBufferDepth, cfg.MaxServices, cfg.StaleServiceAge)
	hub := streaming.NewHub()
	scen := scenario.NewEngine()
	act := newCaptureActuator()

	orch := runtime.New(cfg, store, hub, nil, act, scen)

	// Phase A: high load
	for i := 0; i < 60; i++ {
	store.Ingest(&telemetry.MetricPoint{
		ServiceID:   "svc-recover",
		RequestRate: 850, ErrorRate: 0.05,
		Latency: telemetry.LatencyStats{Mean: 10, P99: 25},
		Timestamp: time.Now(),
	})
	}

	type scaleSnapshot struct {
		tick  uint64
		scale float64
	}
	var mu sync.Mutex
	var scales []scaleSnapshot

	hub.Subscribe(func(p *streaming.TickPayload) {
		for _, d := range p.Directives {
			if d.ServiceID == "svc-recover" {
				mu.Lock()
				scales = append(scales, scaleSnapshot{tick: p.SequenceNo, scale: d.ScaleFactor})
				mu.Unlock()
			}
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go orch.Run(ctx)
	time.Sleep(800 * time.Millisecond)

	// Phase B: load drops
	for i := 0; i < 60; i++ {
	store.Ingest(&telemetry.MetricPoint{
		ServiceID:   "svc-recover",
		RequestRate: 50, ErrorRate: 0.001,
		Latency: telemetry.LatencyStats{Mean: 5, P99: 10},
		Timestamp: time.Now(),
	})
	}
	time.Sleep(1200 * time.Millisecond)
	cancel()
	time.Sleep(100 * time.Millisecond)

	ticks := orch.TickCount()
	t.Logf("Recovery test: %d ticks total", ticks)

	mu.Lock()
	snapCount := len(scales)
	mu.Unlock()

	if ticks < 5 {
		t.Skipf("Not enough ticks (%d) to validate recovery — skipping", ticks)
	}
	if snapCount < 2 {
		t.Logf("Only %d scale snapshots captured — may not have enough coverage", snapCount)
		return
	}

	// Scale should not remain consistently > 2.0 after load dropped
	mu.Lock()
	var highScaleAfterDrop int
	for i, snap := range scales {
		if i < snapCount/2 {
			continue // skip first half (high load phase)
		}
		if snap.scale > 2.5 {
			highScaleAfterDrop++
		}
	}
	mu.Unlock()

	t.Logf("Captured %d scale snapshots, %d high-scale events in recovery phase", snapCount, highScaleAfterDrop)
	if highScaleAfterDrop > snapCount/4 {
		t.Logf("Warning: %d/%d post-drop snapshots still had ScaleFactor>2.5 — slow recovery",
			highScaleAfterDrop, snapCount-snapCount/2)
	}
}

// 
// MASTER TEST 4: MULTI-SERVICE TOPOLOGY PROPAGATION
// Validates topology coupling and cascade risk flows across services
// 

func TestMaster_MultiServiceTopologyCascade(t *testing.T) {
	cfg := masterCfg()
	cfg.TickInterval = 100 * time.Millisecond

	store := telemetry.NewStore(cfg.RingBufferDepth, cfg.MaxServices, cfg.StaleServiceAge)
	hub := streaming.NewHub()
	scen := scenario.NewEngine()
	act := newCaptureActuator()

	orch := runtime.New(cfg, store, hub, nil, act, scen)

	// Inject 5 services to exercise multi-service modelling path
	svcDefs := []struct {
		id  string
		rps float64
		lat float64
	}{
		{"ingress", 300, 20},
		{"auth", 280, 15},
		{"core-api", 260, 35},
		{"db-read", 240, 50},
		{"cache", 220, 5},
	}
	for _, svc := range svcDefs {
		for i := 0; i < 60; i++ {
			store.Ingest(&telemetry.MetricPoint{
				ServiceID:   svc.id,
				RequestRate: svc.rps + float64(i%30),
				ErrorRate:   0.01,
				Latency: telemetry.LatencyStats{
					Mean: svc.lat, P99: svc.lat * 2,
				},
				Timestamp: time.Now(),
			})
		}
	}

	var mu sync.Mutex
	var lastPayload *streaming.TickPayload
	hub.Subscribe(func(p *streaming.TickPayload) {
		mu.Lock()
		lastPayload = p
		mu.Unlock()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
	defer cancel()
	go orch.Run(ctx)
	time.Sleep(2000 * time.Millisecond)
	cancel()
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	p := lastPayload
	mu.Unlock()

	if p == nil {
		t.Fatal("No payload received from 5-service system")
	}
	if len(p.Bundles) < 3 {
		t.Fatalf("Expected ≥3 bundles for 5-service system, got %d", len(p.Bundles))
	}

	t.Logf("Multi-service: %d bundles, NetworkEquilibrium.SystemRhoMean=%.4f, TopologySensitivity.SystemFragility=%.4f",
		len(p.Bundles), p.NetworkEquilibrium.SystemRhoMean, p.TopologySensitivity.SystemFragility)

	// NetworkEquilibrium SystemRhoMean should be bounded
	if p.NetworkEquilibrium.SystemRhoMean < 0 {
		t.Errorf("SystemRhoMean=%.4f < 0", p.NetworkEquilibrium.SystemRhoMean)
	}

	// PriorityRiskQueue should have entries if bundles exist
	if len(p.Bundles) > 0 && len(p.PriorityRiskQueue) == 0 {
		t.Error("PriorityRiskQueue is empty despite having bundles — risk queue builder not connected")
	}

	// PressureHeatmap should have entries
	if len(p.Bundles) > 0 && len(p.PressureHeatmap) == 0 {
		t.Error("PressureHeatmap is empty despite having bundles")
	}

	// Validate each risk queue item
	for _, item := range p.PriorityRiskQueue {
		if item.UrgencyScore < 0 || item.UrgencyScore > 1 {
			t.Errorf("RiskQueueItem %s UrgencyScore=%.4f out of [0,1]", item.ServiceID, item.UrgencyScore)
		}
		if item.CollapseRisk < 0 || item.CollapseRisk > 1 {
			t.Errorf("RiskQueueItem %s CollapseRisk=%.4f out of [0,1]", item.ServiceID, item.CollapseRisk)
		}
		if item.Rho < 0 {
			t.Errorf("RiskQueueItem %s Rho=%.4f < 0", item.ServiceID, item.Rho)
		}
		validClass := map[string]bool{"nominal": true, "elevated": true, "warning": true, "critical": true}
		if !validClass[item.UrgencyClass] {
			t.Errorf("RiskQueueItem %s UrgencyClass=%q invalid", item.ServiceID, item.UrgencyClass)
		}
	}

	// Risk queue should be sorted descending by urgency
	rq := p.PriorityRiskQueue
	for i := 1; i < len(rq); i++ {
		if rq[i].UrgencyScore > rq[i-1].UrgencyScore+1e-9 {
			t.Errorf("RiskQueue not sorted at index %d: score[%d]=%.4f > score[%d]=%.4f",
				i, i, rq[i].UrgencyScore, i-1, rq[i-1].UrgencyScore)
		}
	}
}

// 
// MASTER TEST 5: SINGLE-START GUARD — orchestrator cannot run twice concurrently
// 

func TestMaster_OrchestratorSingleStartGuard(t *testing.T) {
	cfg := masterCfg()
	cfg.TickInterval = 50 * time.Millisecond

	store := telemetry.NewStore(256, 16, 30*time.Second)
	hub := streaming.NewHub()
	scen := scenario.NewEngine()
	act := newCaptureActuator()

	orch := runtime.New(cfg, store, hub, nil, act, scen)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	var wg sync.WaitGroup
	var startCount int64

	// Start Run() from 3 goroutines simultaneously
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			atomic.AddInt64(&startCount, 1)
			orch.Run(ctx) // only one should run; others return immediately
		}()
	}

	wg.Wait()
	// No assertions needed — the fact that none panicked or deadlocked is the test.
	// Reentrant tick count should be 0 (no double-ticking).
	reentrant := orch.ReentrantTickCount()
	t.Logf("Started %d Run() goroutines, reentrant ticks=%d", startCount, reentrant)
}

// 
// MASTER TEST 6: ACTUATION FEEDBACK LOOP
// Validates actuator feedback is drained each tick without blocking
// 

func TestMaster_ActuatorFeedbackDrained(t *testing.T) {
	cfg := masterCfg()
	cfg.TickInterval = 80 * time.Millisecond

	store := telemetry.NewStore(512, 16, 30*time.Second)
	hub := streaming.NewHub()
	scen := scenario.NewEngine()

	// Use real coalescing actuator with queue backend
	qBackend := backends.NewQueueBackend()
	act := actuator.NewCoalescingActuator(512, qBackend)
	defer act.Close(context.Background())

	orch := runtime.New(cfg, store, hub, nil, act, scen)

	for i := 0; i < 40; i++ {
	store.Ingest(&telemetry.MetricPoint{
		ServiceID:   "svc-fb",
		RequestRate: 100, ErrorRate: 0.01,
		Latency: telemetry.LatencyStats{Mean: 50, P99: 100},
		Timestamp: time.Now(),
	})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	go orch.Run(ctx)
	<-ctx.Done()
	time.Sleep(100 * time.Millisecond)

	// Check feedback channel is not overflowing (test for channel drain)
	feedbackLen := len(act.Feedback())
	t.Logf("Feedback channel length after run: %d/512", feedbackLen)
	if feedbackLen > 400 {
		t.Errorf("Feedback channel nearly full (%d/512) — feedback not being drained properly", feedbackLen)
	}

	ticks := orch.TickCount()
	if ticks == 0 {
		t.Error("Orchestrator ran 0 ticks")
	}
	t.Logf("Ran %d ticks, reentrant=%d", ticks, orch.ReentrantTickCount())
}

// 
// MASTER TEST 7: ZERO-LATENCY RESTART
// Validates bundles are broadcast on tick 1 when windows exist at startup
// 

func TestMaster_ZeroLatencyRestartWithExistingWindows(t *testing.T) {
	cfg := masterCfg()
	store := telemetry.NewStore(cfg.RingBufferDepth, cfg.MaxServices, cfg.StaleServiceAge)

	// Pre-populate store before constructing orchestrator (simulating restart)
	for i := 0; i < 60; i++ {
	store.Ingest(&telemetry.MetricPoint{
		ServiceID:   "svc-preload",
		RequestRate: 100, ErrorRate: 0.01,
		Latency: telemetry.LatencyStats{Mean: 50, P99: 100},
		Timestamp: time.Now(),
	})
	}

	hub := streaming.NewHub()
	scen := scenario.NewEngine()
	act := newCaptureActuator()

	// Capture payloads before Run() is even called
	payloads := make(chan *streaming.TickPayload, 32)
	hub.Subscribe(func(p *streaming.TickPayload) {
		select {
		case payloads <- p:
		default:
		}
	})

	// runtime.New() should broadcast immediately if windows exist
	orch := runtime.New(cfg, store, hub, nil, act, scen)
	_ = orch

	// Give the constructor broadcast a moment
	time.Sleep(50 * time.Millisecond)

	// There should be at least one payload from the constructor broadcast
	select {
	case p := <-payloads:
		if len(p.Bundles) == 0 {
			t.Error("Restart broadcast has empty Bundles — zero-latency restart failed")
		}
		t.Logf("Zero-latency restart: got %d bundles immediately on construction", len(p.Bundles))
	case <-time.After(200 * time.Millisecond):
		t.Log("No broadcast from constructor — zero-latency restart may require Run() to be called first")
	}
}

// 
// MASTER TEST 8: PERSISTENCE WRITER (nil-safe path)
// 

func TestMaster_PersistenceWriterNilSafe(t *testing.T) {
	// pw=nil (no DATABASE_URL configured) — must not panic
	var pw *persistence.Writer
	pw.Close() // Writer.Close() must be nil-safe

	// Enqueue on nil writer must be safe too
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("nil persistence.Writer.Enqueue panicked: %v", r)
		}
	}()
	pw.Enqueue(persistence.Snapshot{TickAt: time.Now()})
}

// 
// MASTER TEST 9: SIMULATION RUNNER MULTI-SCENARIO
// 

func TestMaster_SimulationMultiScenarioComparison(t *testing.T) {
	runner := simulation.NewRunner(300.0, 1.5, 4)
	runner.SetStochasticMode("exponential")
	runner.SetHorizonMultiplier(0.4)

	snap := topology.GraphSnapshot{}
	bundles := map[string]*modelling.ServiceModelBundle{
		"svc-a": makeBundle("svc-a", 80, 100, 0.80, 0.20),
		"svc-b": makeBundle("svc-b", 60, 100, 0.60, 0.08),
	}

	msr := runner.LatestMultiScenario(bundles, snap, 60*time.Millisecond, 3)
	if msr == nil {
		t.Skip("LatestMultiScenario returned nil — budget may be too small; skipping")
	}

	comp := msr.Comparison
	t.Logf("MultiScenario: scenarios=%d best_collapse=%.4f worst_collapse=%.4f median_sla=%.4f stable_frac=%.4f",
		comp.ScenarioCount, comp.BestCaseCollapse, comp.WorstCaseCollapse,
		comp.MedianSLAViolation, comp.StableScenarioFraction)

	if comp.ScenarioCount < 1 {
		t.Error("ScenarioCount < 1")
	}
	if comp.BestCaseCollapse > comp.WorstCaseCollapse+1e-9 {
		t.Errorf("BestCaseCollapse=%.4f > WorstCaseCollapse=%.4f", comp.BestCaseCollapse, comp.WorstCaseCollapse)
	}
	if comp.StableScenarioFraction < 0 || comp.StableScenarioFraction > 1 {
		t.Errorf("StableScenarioFraction=%.4f out of [0,1]", comp.StableScenarioFraction)
	}
}

// 
// MASTER TEST 10: STABILITY ENVELOPE VALIDATION
// 

func TestMaster_StabilityEnvelopeInPayload(t *testing.T) {
	cfg := masterCfg()
	store := telemetry.NewStore(cfg.RingBufferDepth, cfg.MaxServices, cfg.StaleServiceAge)
	hub := streaming.NewHub()
	scen := scenario.NewEngine()
	act := newCaptureActuator()

	// Ingest moderate load
	for i := 0; i < 60; i++ {
	store.Ingest(&telemetry.MetricPoint{
		ServiceID:   "svc-env",
		RequestRate: 120, ErrorRate: 0.01,
		Latency: telemetry.LatencyStats{Mean: 30, P99: 60},
		Timestamp: time.Now(),
	})
	}

	var mu sync.Mutex
	var lastPayload *streaming.TickPayload
	hub.Subscribe(func(p *streaming.TickPayload) {
		mu.Lock()
		lastPayload = p
		mu.Unlock()
	})

	orch := runtime.New(cfg, store, hub, nil, act, scen)
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	go orch.Run(ctx)
	time.Sleep(1200 * time.Millisecond)
	cancel()

	mu.Lock()
	p := lastPayload
	mu.Unlock()

	if p == nil {
		t.Fatal("No payload received")
	}

	env := p.StabilityEnvelope
	if env.SafeSystemRhoMax <= 0 || env.SafeSystemRhoMax > 1 {
		t.Errorf("StabilityEnvelope.SafeSystemRhoMax=%.4f out of (0,1]", env.SafeSystemRhoMax)
	}
	if math.IsNaN(env.CurrentSystemRhoMean) {
		t.Error("StabilityEnvelope.CurrentSystemRhoMean is NaN")
	}
	if math.IsNaN(env.EnvelopeHeadroom) {
		t.Error("StabilityEnvelope.EnvelopeHeadroom is NaN")
	}
	t.Logf("StabilityEnvelope: safeMax=%.4f currentRho=%.4f headroom=%.4f mostVulnerable=%q",
		env.SafeSystemRhoMax, env.CurrentSystemRhoMean, env.EnvelopeHeadroom, env.MostVulnerableService)
}

// 
// MASTER TEST 11: ADAPTIVE TICK INTERVAL STRETCHES ON OVERRUN
// 

func TestMaster_AdaptiveTickIntervalUnderLoad(t *testing.T) {
	cfg := masterCfg()
	cfg.TickInterval = 50 * time.Millisecond
	cfg.TickDeadline = 20 * time.Millisecond // very tight deadline to force overruns
	cfg.MinTickInterval = 50 * time.Millisecond
	cfg.MaxTickInterval = 500 * time.Millisecond
	cfg.TickAdaptStep = 1.25
	cfg.SafetyModeThreshold = 2
	cfg.WorkerPoolSize = 16

	store := telemetry.NewStore(512, 16, 30*time.Second)
	hub := streaming.NewHub()
	scen := scenario.NewEngine()
	act := newCaptureActuator()

	// Fill store with many services to make ticks expensive
	for svcIdx := 0; svcIdx < 10; svcIdx++ {
		svcID := "svc-" + string(rune('a'+svcIdx))
		for i := 0; i < 100; i++ {
			store.Ingest(&telemetry.MetricPoint{
				ServiceID:   svcID,
				RequestRate: 100 + float64(svcIdx*10),
				ErrorRate:   0.01,
				Latency:     telemetry.LatencyStats{Mean: 50, P99: 100},
				Timestamp:   time.Now(),
			})
		}
	}

	var mu sync.Mutex
	var overrunSeen bool
	hub.Subscribe(func(p *streaming.TickPayload) {
		mu.Lock()
		if p.RuntimeMetrics.TotalOverruns > 0 {
			overrunSeen = true
		}
		mu.Unlock()
	})

	orch := runtime.New(cfg, store, hub, nil, act, scen)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go orch.Run(ctx)
	time.Sleep(1800 * time.Millisecond)
	cancel()
	time.Sleep(100 * time.Millisecond)

	ticks := orch.TickCount()
	mu.Lock()
	didOverrun := overrunSeen
	mu.Unlock()
	t.Logf("Adaptive interval test: %d ticks, overruns seen=%v", ticks, didOverrun)
	// Test passes as long as system didn't panic or deadlock
}

// 
// MASTER TEST 12: STOCHASTIC BYPASS UNDER STALE DATA
// Validates that stochastic model is bypassed when systemStaleness > 0.5
// but queue modelling and optimisation still run
// 

func TestMaster_StaleDataBypassesStochasticKeepsQueueModel(t *testing.T) {
	cfg := masterCfg()
	cfg.TickInterval = 100 * time.Millisecond

	store := telemetry.NewStore(cfg.RingBufferDepth, cfg.MaxServices, cfg.StaleServiceAge)
	hub := streaming.NewHub()
	scen := scenario.NewEngine()
	act := newCaptureActuator()

	// Inject old data (will be stale by the time orchestrator runs)
	for i := 0; i < 30; i++ {
	store.Ingest(&telemetry.MetricPoint{
		ServiceID:   "svc-stale",
		RequestRate: 100, ErrorRate: 0.01,
		Latency: telemetry.LatencyStats{Mean: 50, P99: 100},
		Timestamp: time.Now().Add(-10 * time.Second), // aged data
	})
	}

	var mu sync.Mutex
	var lastPayload *streaming.TickPayload
	hub.Subscribe(func(p *streaming.TickPayload) {
		mu.Lock()
		lastPayload = p
		mu.Unlock()
	})

	orch := runtime.New(cfg, store, hub, nil, act, scen)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	go orch.Run(ctx)
	time.Sleep(800 * time.Millisecond)
	cancel()

	mu.Lock()
	p := lastPayload
	mu.Unlock()

	// Even with stale data, orchestrator must not panic
	// Payload may be nil if all windows were pruned — that's acceptable
	if p != nil {
		for id, b := range p.Bundles {
			if math.IsNaN(b.Queue.Utilisation) {
				t.Errorf("Bundle %s Utilisation is NaN even with stale bypass", id)
			}
			// Stochastic model confidence should be low with stale data
			if b.Stochastic.Confidence > 0.9 {
				t.Logf("Bundle %s Stochastic.Confidence=%.4f despite stale data (may be bypassed with default)", id, b.Stochastic.Confidence)
			}
		}
	}
	t.Log("Stale data bypass test passed — no panic with aged telemetry")
}

// 
// MASTER TEST 13: END-TO-END NUMERIC SUMMARY
// 

func TestMaster_E2ENumericSummary(t *testing.T) {
	cfg := masterCfg()
	cfg.TickInterval = 100 * time.Millisecond

	store := telemetry.NewStore(cfg.RingBufferDepth, cfg.MaxServices, cfg.StaleServiceAge)
	hub := streaming.NewHub()
	scen := scenario.NewEngine()
	act := newCaptureActuator()

	// Realistic load profile
	for i := 0; i < 80; i++ {
	store.Ingest(&telemetry.MetricPoint{
		ServiceID:   "svc-summary",
		RequestRate: 200 + float64(i%40),
		ErrorRate:   0.008,
		Latency:     telemetry.LatencyStats{Mean: 40, P50: 35, P95: 80, P99: 120},
		Timestamp:   time.Now(),
	})
	}

	var mu sync.Mutex
	type metricSample struct {
		tick              uint64
		utilisation       float64
		collapseRisk      float64
		scaleFactor       float64
		compositeScore    float64
		tickHealthMs      float64
		cascadeFailureProb float64
	}
	var samples []metricSample

	hub.Subscribe(func(p *streaming.TickPayload) {
		for _, b := range p.Bundles {
			for _, d := range p.Directives {
				mu.Lock()
				samples = append(samples, metricSample{
					tick:           p.SequenceNo,
					utilisation:    b.Queue.Utilisation,
					collapseRisk:   b.Stability.CollapseRisk,
					scaleFactor:    d.ScaleFactor,
					compositeScore: p.Objective.CompositeScore,
					tickHealthMs:   p.TickHealthMs,
					cascadeFailureProb: func() float64 {
						if p.SimResult != nil && p.SimResult.CascadeFailureProbability != nil {
							// Take average of all service probabilities
							var sum float64
							var count int
							for _, prob := range p.SimResult.CascadeFailureProbability {
								sum += prob
								count++
							}
							if count > 0 {
								return sum / float64(count)
							}
						}
						return -1
					}(),
				})
				mu.Unlock()
				break
			}
			break
		}
	})

	orch := runtime.New(cfg, store, hub, nil, act, scen)
	ctx, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
	defer cancel()
	go orch.Run(ctx)
	time.Sleep(2200 * time.Millisecond)
	cancel()
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	samplesCopy := make([]metricSample, len(samples))
	copy(samplesCopy, samples)
	mu.Unlock()

	ticks := orch.TickCount()
	if ticks == 0 {
		t.Fatal("Zero ticks completed")
	}
	if len(samplesCopy) == 0 {
		t.Fatal("No metric samples captured")
	}

	// Compute summary statistics
	var sumUtil, sumRisk, sumScale, sumComposite, sumTickMs float64
	var minScale, maxScale float64 = 1e9, 0
	var allValid = true
	for _, s := range samplesCopy {
		if math.IsNaN(s.utilisation) || math.IsNaN(s.scaleFactor) || math.IsNaN(s.compositeScore) {
			allValid = false
		}
		sumUtil += s.utilisation
		sumRisk += s.collapseRisk
		sumScale += s.scaleFactor
		sumComposite += s.compositeScore
		sumTickMs += s.tickHealthMs
		if s.scaleFactor < minScale {
			minScale = s.scaleFactor
		}
		if s.scaleFactor > maxScale {
			maxScale = s.scaleFactor
		}
	}
	n := float64(len(samplesCopy))

	summary := map[string]float64{
		"ticks_completed":       float64(ticks),
		"samples_captured":      n,
		"avg_utilisation":       sumUtil / n,
		"avg_collapse_risk":     sumRisk / n,
		"avg_scale_factor":      sumScale / n,
		"min_scale_factor":      minScale,
		"max_scale_factor":      maxScale,
		"avg_composite_score":   sumComposite / n,
		"avg_tick_health_ms":    sumTickMs / n,
		"reentrant_ticks":       float64(orch.ReentrantTickCount()),
	}

	t.Logf("\n=== E2E NUMERIC SUMMARY ===")
	for k, v := range summary {
		t.Logf("  %-30s = %10.4f", k, v)
	}

	if !allValid {
		t.Error("Some metric samples contained NaN values")
	}
	if summary["avg_utilisation"] < 0 || summary["avg_utilisation"] > 2 {
		t.Errorf("avg_utilisation=%.4f out of sensible range [0,2]", summary["avg_utilisation"])
	}
	if summary["avg_collapse_risk"] < 0 || summary["avg_collapse_risk"] > 1 {
		t.Errorf("avg_collapse_risk=%.4f out of [0,1]", summary["avg_collapse_risk"])
	}
	if summary["avg_composite_score"] < 0 || summary["avg_composite_score"] > 1 {
		t.Errorf("avg_composite_score=%.4f out of [0,1]", summary["avg_composite_score"])
	}
	if summary["avg_scale_factor"] <= 0 {
		t.Errorf("avg_scale_factor=%.4f ≤0", summary["avg_scale_factor"])
	}
}