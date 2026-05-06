package layer7



import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/loadequilibrium/loadequilibrium/internal/actuator"
	"github.com/loadequilibrium/loadequilibrium/internal/actuator/backends"
	"github.com/loadequilibrium/loadequilibrium/internal/api"
	"github.com/loadequilibrium/loadequilibrium/internal/config"
	runtimepkg "github.com/loadequilibrium/loadequilibrium/internal/runtime"
	"github.com/loadequilibrium/loadequilibrium/internal/streaming"
	"github.com/loadequilibrium/loadequilibrium/internal/telemetry"
)

// ─── Result schema ─────────────────────────────────────────────────────────

type L7Threshold struct {
	Metric    string  `json:"metric"`
	Operator  string  `json:"operator"`
	Value     float64 `json:"value"`
	Unit      string  `json:"unit"`
	Rationale string  `json:"rationale"`
}

type L7ResultData struct {
	Status             string   `json:"status"`
	ActualValue        float64  `json:"actual_value"`
	ActualUnit         string   `json:"actual_unit"`
	ChaosInjected      string   `json:"chaos_injected"`
	GoroutinesBefore   int      `json:"goroutines_before"`
	GoroutinesPeak     int      `json:"goroutines_peak"`
	GoroutinesAfter    int      `json:"goroutines_after"`
	GoroutinesLeaked   int      `json:"goroutines_leaked"`
	TicksCompleted     int      `json:"ticks_completed"`
	PanicsDetected     int64    `json:"panics_detected"`
	ShutdownMs         int64    `json:"shutdown_duration_ms"`
	DurationMs         int64    `json:"duration_ms"`
	ErrorMessages      []string `json:"error_messages,omitempty"`
}

type L7Questions struct {
	WhatChaosWasInjected   string `json:"what_chaos_was_injected"`
	WhyThisThreshold       string `json:"why_this_threshold"`
	WhatHappensIfFails     string `json:"what_happens_if_it_fails"`
	HowChaosWasInjected    string `json:"how_chaos_was_injected"`
	HowRecoveryVerified    string `json:"how_recovery_was_verified"`
	ProductionEquivalent   string `json:"production_equivalent"`
}

type L7Record struct {
	TestID           string       `json:"test_id"`
	Layer            int          `json:"layer"`
	Name             string       `json:"name"`
	Aim              string       `json:"aim"`
	PackagesInvolved []string     `json:"packages_involved"`
	Threshold        L7Threshold  `json:"threshold"`
	Result           L7ResultData `json:"result"`
	OnExceed         string       `json:"on_exceed"`
	Questions        L7Questions  `json:"answered_questions"`
	RunAt            string       `json:"run_at"`
	GoVersion        string       `json:"go_version"`
}

var (
	l7Mu      sync.Mutex
	l7OutPath = "tests/results/layer7_results.json"
)

func writeL7Result(r L7Record) {
	l7Mu.Lock()
	defer l7Mu.Unlock()
	_ = os.MkdirAll("tests/results", 0o755)
	var existing []L7Record
	if raw, err := os.ReadFile(l7OutPath); err == nil {
		_ = json.Unmarshal(raw, &existing)
	}
	existing = append(existing, r)
	data, _ := json.MarshalIndent(existing, "", "  ")
	if err := os.WriteFile(l7OutPath, data, 0o644); err != nil {
		fmt.Printf("WARNING: could not write layer7 results: %v\n", err)
	}
}

func l7Now() string   { return time.Now().UTC().Format(time.RFC3339) }
func l7GoVer() string { return runtime.Version() }
func l7Status(passed bool) string {
	if passed {
		return "PASS"
	}
	return "FAIL"
}
func l7Goroutines() int { return runtime.NumGoroutine() }

// ─── testSystem mirrors main.go wiring ────────────────────────────────────

// testSystem holds all components constructed in the same order as main.go.
// Use newTestSystem() to construct; call Shutdown() to tear down.
type testSystem struct {
	Cfg      *config.Config
	Store    *telemetry.Store
	Hub      *streaming.Hub
	Act      *actuator.CoalescingActuator
	Queue    *backends.QueueBackend
	Router   *actuator.RouterBackend
	Orch     *runtimepkg.Orchestrator
	Srv      *api.Server
	HTTPSrv  *httptest.Server

	// orchCtx / orchCancel control orch.Run goroutine lifecycle.
	orchCtx    context.Context
	orchCancel context.CancelFunc
	orchDone   chan struct{}
}

// newTestSystem constructs the full component graph from main.go using
// test-safe config (fast ticks, no DB, no scenario engine, small buffers).
//
// Callers must call ts.Shutdown() when done.
func newTestSystem(tickMs int) *testSystem {
	cfg := testConfig(tickMs)

	store := telemetry.NewStore(cfg.RingBufferDepth, cfg.MaxServices, cfg.StaleServiceAge)
	hub := streaming.NewHub()
	hub.SetMaxClients(cfg.MaxStreamClients)

	// pw = nil (no DATABASE_URL in tests — matches main.go "else" branch)
	queueBackend := backends.NewQueueBackend()
	routerBackend := actuator.NewRouterBackend(queueBackend)
	act := actuator.NewCoalescingActuator(1024, routerBackend)

	// scen = nil, ScenarioMode="off" — matches main.go when SCENARIO_MODE=off
	orch := runtimepkg.New(cfg, store, hub, nil, act, nil)

	srv := api.NewServer(store, hub, "") // empty token = auth disabled
	srv.SetOrchestrator(orch)
	srv.SetActuator(act)

	httpSrv := httptest.NewServer(srv.Handler())

	orchCtx, orchCancel := context.WithCancel(context.Background())
	orchDone := make(chan struct{})

	ts := &testSystem{
		Cfg:        cfg,
		Store:      store,
		Hub:        hub,
		Act:        act,
		Queue:      queueBackend,
		Router:     routerBackend,
		Orch:       orch,
		Srv:        srv,
		HTTPSrv:    httpSrv,
		orchCtx:    orchCtx,
		orchCancel: orchCancel,
		orchDone:   orchDone,
	}

	// Start orchestrator goroutine — mirrors main.go "go orch.Run(ctx)".
	go func() {
		defer close(orchDone)
		orch.Run(orchCtx)
	}()

	return ts
}

// Shutdown mirrors main.go shutdown sequence:
//   httpServer.Shutdown(shutCtx) → act.Close(shutCtx) → pw.Close() [nil, skipped]
func (ts *testSystem) Shutdown() {
	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 1. Stop HTTP server.
	ts.HTTPSrv.Close()

	// 2. Cancel orchestrator context.
	ts.orchCancel()
	select {
	case <-ts.orchDone:
	case <-shutCtx.Done():
	}

	// 3. Close actuator (mirrors act.Close(shutCtx)).
	_ = ts.Act.Close(shutCtx)
	// pw.Close() skipped — pw is nil in tests.
}

// WaitForTicks blocks until the orchestrator has completed at least n ticks
// or timeout elapses. Uses the /health endpoint to confirm liveness.
func (ts *testSystem) WaitForTicks(n int, timeout time.Duration) bool {
	// Simple approach: poll hub.GetLastPayload() for SequenceNo >= n.
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		p := ts.Hub.GetLastPayload()
		if p != nil && int(p.SequenceNo) >= n {
			return true
		}
		time.Sleep(time.Duration(ts.Cfg.TickInterval.Milliseconds()/2) * time.Millisecond)
	}
	return false
}

// InjectTelemetry ingests a MetricPoint for the given service.
func (ts *testSystem) InjectTelemetry(svcID string, reqRate, latencyMean float64) {
	ts.Store.Ingest(&telemetry.MetricPoint{
		ServiceID:   svcID,
		Timestamp:   time.Now(),
		RequestRate: reqRate,
		ErrorRate:   0.005,
		Latency: telemetry.LatencyStats{
			Mean: latencyMean,
			P50:  latencyMean * 0.8,
			P95:  latencyMean * 1.5,
			P99:  latencyMean * 2.0,
		},
		ActiveConns: 10,
		QueueDepth:  5,
	})
}

// GetHTTP performs a GET request to the test HTTP server.
func (ts *testSystem) GetHTTP(path string) (int, error) {
	resp, err := http.Get(ts.HTTPSrv.URL + path)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}

// testConfig returns a test-safe config that mirrors config.Load() defaults
// but with short tick intervals and small buffers for fast test completion.
func testConfig(tickMs int) *config.Config {
	if tickMs <= 0 {
		tickMs = 100
	}
	tick := time.Duration(tickMs) * time.Millisecond
	return &config.Config{
		TickInterval:             tick,
		TickDeadline:             tick * 8,
		MinTickInterval:          tick / 2,
		MaxTickInterval:          tick * 10,
		TickAdaptStep:            1.25,
		RingBufferDepth:          64,
		MaxServices:              20,
		StaleServiceAge:          30 * time.Second,
		WindowFraction:           0.5,
		WorkerPoolSize:           2,
		MaxStreamClients:         10,
		EWMAFastAlpha:            0.30,
		EWMASlowAlpha:            0.10,
		SpikeZScore:              3.0,
		CollapseThreshold:        0.90,
		UtilisationSetpoint:      0.70,
		PredictiveHorizonTicks:   3,
		ArrivalEstimatorMode:     "ewma",
		SimHorizonMs:             500,
		SimShockFactor:           1.5,
		SimAsyncBuffer:           2,
		SimStochasticMode:        "disabled",
		SimBudget:                5 * time.Millisecond,
		MaxReasoningCooldowns:    5,
		SafetyModeThreshold:      5,
		ScenarioMode:             "off",
		SLALatencyThresholdMs:    500,
		StalenessBypassThreshold: 0.70,
		PIDKp:                    -1.5,
		PIDKi:                    -0.3,
		PIDKd:                    -0.1,
		PIDDeadband:              0.02,
		PIDIntegralMax:           2.0,
	}
}