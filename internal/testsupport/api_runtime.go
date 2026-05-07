package testsupport

import (
	"context"
	"net/http/httptest"
	"sync"
	"time"

	"github.com/loadequilibrium/loadequilibrium/internal/actuator"
	"github.com/loadequilibrium/loadequilibrium/internal/actuator/backends"
	"github.com/loadequilibrium/loadequilibrium/internal/api"
	"github.com/loadequilibrium/loadequilibrium/internal/config"
	runtimepkg "github.com/loadequilibrium/loadequilibrium/internal/runtime"
	"github.com/loadequilibrium/loadequilibrium/internal/scenario"
	"github.com/loadequilibrium/loadequilibrium/internal/streaming"
	"github.com/loadequilibrium/loadequilibrium/internal/telemetry"
)

type CleanupT interface {
	Helper()
	Cleanup(func())
	Fatalf(format string, args ...interface{})
}

type APIRuntimeOption func(*apiRuntimeOptions)

type apiRuntimeOptions struct {
	tickInterval      time.Duration
	tickDeadline      time.Duration
	ringBufferDepth   int
	maxServices       int
	maxStreamClients  int
	staleServiceAge   time.Duration
	ingestToken       string
	scenarioMode      string
	startOrchestrator bool
}

type APIRuntime struct {
	Cfg          *config.Config
	Store        *telemetry.Store
	Hub          *streaming.Hub
	Queue        *backends.QueueBackend
	Router       *actuator.RouterBackend
	Actuator     *actuator.CoalescingActuator
	Scenarios    *scenario.SuperpositionEngine
	Orchestrator *runtimepkg.Orchestrator
	Server       *api.Server
	HTTPServer   *httptest.Server

	runtimeCtx    context.Context
	runtimeCancel context.CancelFunc
	runtimeDone   chan struct{}
	startOnce     sync.Once
	shutdownOnce  sync.Once
}

func WithTickInterval(d time.Duration) APIRuntimeOption {
	return func(o *apiRuntimeOptions) {
		if d > 0 {
			o.tickInterval = d
		}
	}
}

func WithRingBufferDepth(n int) APIRuntimeOption {
	return func(o *apiRuntimeOptions) {
		if n > 0 {
			o.ringBufferDepth = n
		}
	}
}

func WithMaxServices(n int) APIRuntimeOption {
	return func(o *apiRuntimeOptions) {
		if n > 0 {
			o.maxServices = n
		}
	}
}

func WithMaxStreamClients(n int) APIRuntimeOption {
	return func(o *apiRuntimeOptions) {
		if n > 0 {
			o.maxStreamClients = n
		}
	}
}

func WithIngestToken(token string) APIRuntimeOption {
	return func(o *apiRuntimeOptions) {
		o.ingestToken = token
	}
}

func WithScenarioMode(mode string) APIRuntimeOption {
	return func(o *apiRuntimeOptions) {
		if mode != "" {
			o.scenarioMode = mode
		}
	}
}

func WithStartedOrchestrator(start bool) APIRuntimeOption {
	return func(o *apiRuntimeOptions) {
		o.startOrchestrator = start
	}
}

func NewAPIRuntime(t CleanupT, opts ...APIRuntimeOption) *APIRuntime {
	t.Helper()

	o := apiRuntimeOptions{
		tickInterval:      100 * time.Millisecond,
		ringBufferDepth:   256,
		maxServices:       50,
		maxStreamClients:  100,
		staleServiceAge:   5 * time.Minute,
		scenarioMode:      "off",
		startOrchestrator: true,
	}
	for _, opt := range opts {
		opt(&o)
	}
	if o.tickDeadline <= 0 {
		o.tickDeadline = o.tickInterval * 8 / 10
	}

	cfg := testConfig(o)
	store := telemetry.NewStore(cfg.RingBufferDepth, cfg.MaxServices, cfg.StaleServiceAge)
	hub := streaming.NewHub()
	hub.SetMaxClients(cfg.MaxStreamClients)

	queueBackend := backends.NewQueueBackend()
	routerBackend := actuator.NewRouterBackend(queueBackend)
	act := actuator.NewCoalescingActuator(1024, routerBackend)
	scenarios := scenario.NewEngine()

	orch := runtimepkg.New(cfg, store, hub, nil, act, scenarios)

	srv := api.NewServer(store, hub, cfg.IngestToken)
	srv.SetOrchestrator(orch)
	srv.SetActuator(act)
	srv.SetScenarios(scenarios)

	rtCtx, rtCancel := context.WithCancel(context.Background())
	rt := &APIRuntime{
		Cfg:           cfg,
		Store:         store,
		Hub:           hub,
		Queue:         queueBackend,
		Router:        routerBackend,
		Actuator:      act,
		Scenarios:     scenarios,
		Orchestrator:  orch,
		Server:        srv,
		HTTPServer:    httptest.NewServer(srv.Handler()),
		runtimeCtx:    rtCtx,
		runtimeCancel: rtCancel,
	}

	rt.requireComplete(t)
	if o.startOrchestrator {
		rt.StartOrchestrator()
	}
	t.Cleanup(rt.Shutdown)
	return rt
}

func (rt *APIRuntime) StartOrchestrator() {
	rt.startOnce.Do(func() {
		rt.runtimeDone = make(chan struct{})
		go func() {
			defer close(rt.runtimeDone)
			rt.Orchestrator.Run(rt.runtimeCtx)
		}()
	})
}

func (rt *APIRuntime) Shutdown() {
	rt.shutdownOnce.Do(func() {
		if rt.HTTPServer != nil {
			rt.HTTPServer.Close()
		}
		if rt.runtimeCancel != nil {
			rt.runtimeCancel()
		}

		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if rt.runtimeDone != nil {
			select {
			case <-rt.runtimeDone:
			case <-shutCtx.Done():
			}
		}

		if rt.Actuator != nil {
			_ = rt.Actuator.Close(shutCtx)
		}
	})
}

func (rt *APIRuntime) InjectTelemetry(serviceID string, requestRate, latencyMean float64) {
	rt.Store.Ingest(&telemetry.MetricPoint{
		ServiceID:   serviceID,
		Timestamp:   time.Now(),
		RequestRate: requestRate,
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

func (rt *APIRuntime) requireComplete(t CleanupT) {
	t.Helper()
	switch {
	case rt.Store == nil:
		t.Fatalf("test api runtime incomplete: telemetry store is nil")
	case rt.Hub == nil:
		t.Fatalf("test api runtime incomplete: streaming hub is nil")
	case rt.Queue == nil:
		t.Fatalf("test api runtime incomplete: queue backend is nil")
	case rt.Router == nil:
		t.Fatalf("test api runtime incomplete: actuator router is nil")
	case rt.Actuator == nil:
		t.Fatalf("test api runtime incomplete: actuator is nil")
	case rt.Scenarios == nil:
		t.Fatalf("test api runtime incomplete: scenario engine is nil")
	case rt.Orchestrator == nil:
		t.Fatalf("test api runtime incomplete: runtime orchestrator is nil")
	case rt.Server == nil:
		t.Fatalf("test api runtime incomplete: api server is nil")
	case rt.HTTPServer == nil:
		t.Fatalf("test api runtime incomplete: httptest server is nil")
	}
}

func testConfig(o apiRuntimeOptions) *config.Config {
	return &config.Config{
		TickInterval:             o.tickInterval,
		TickDeadline:             o.tickDeadline,
		MinTickInterval:          o.tickInterval / 2,
		MaxTickInterval:          o.tickInterval * 10,
		TickAdaptStep:            1.25,
		RingBufferDepth:          o.ringBufferDepth,
		MaxServices:              o.maxServices,
		StaleServiceAge:          o.staleServiceAge,
		WindowFraction:           0.5,
		WorkerPoolSize:           2,
		MaxStreamClients:         o.maxStreamClients,
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
		ScenarioMode:             o.scenarioMode,
		SLALatencyThresholdMs:    500,
		StalenessBypassThreshold: 0.70,
		PIDKp:                    -1.5,
		PIDKi:                    -0.3,
		PIDKd:                    -0.1,
		PIDDeadband:              0.02,
		PIDIntegralMax:           2.0,
		IngestToken:              o.ingestToken,
	}
}
