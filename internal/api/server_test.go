package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/loadequilibrium/loadequilibrium/internal/config"
	"github.com/loadequilibrium/loadequilibrium/internal/runtime"
	"github.com/loadequilibrium/loadequilibrium/internal/scenario"
	"github.com/loadequilibrium/loadequilibrium/internal/streaming"
	"github.com/loadequilibrium/loadequilibrium/internal/telemetry"
)

func TestIngestEndpointAcceptsTelemetryBatch(t *testing.T) {
	store := telemetry.NewStore(16, 4, time.Minute)
	server := NewServer(store, streaming.NewHub(), "secret")

	body := `[
		{
			"service_id": "checkout",
			"request_rate": 42,
			"error_rate": 0.02,
			"latency": { "p50": 10, "p95": 30, "p99": 50, "mean": 20 },
			"active_conns": 7,
			"queue_depth": 3
		}
	]`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/ingest", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Ingest-Token", "secret")
	rr := httptest.NewRecorder()

	server.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusAccepted, rr.Code, rr.Body.String())
	}

	window := store.Window("checkout", 1, 0)
	if window == nil {
		t.Fatal("expected ingested checkout telemetry window")
	}
	if window.LastRequestRate != 42 {
		t.Fatalf("expected last request rate 42, got %.2f", window.LastRequestRate)
	}
}

func TestIngestEndpointRejectsInvalidToken(t *testing.T) {
	store := telemetry.NewStore(16, 4, time.Minute)
	server := NewServer(store, streaming.NewHub(), "secret")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/ingest", strings.NewReader(`{"service_id":"checkout"}`))
	req.Header.Set("X-Ingest-Token", "wrong")
	rr := httptest.NewRecorder()

	server.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
	}
}

func TestControlToggleAppliesActuationState(t *testing.T) {
	store, hub, orch := newTestRuntime()
	server := NewServer(store, hub, "")
	server.SetOrchestrator(orch)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/control/toggle", strings.NewReader(`{"enabled":false}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	server.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, rr.Code, rr.Body.String())
	}
	if orch.ActuationEnabled() {
		t.Fatal("expected actuation to be disabled")
	}

	var body struct {
		ControlPlane struct {
			ActuationEnabled bool `json:"actuation_enabled"`
		} `json:"control_plane"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if body.ControlPlane.ActuationEnabled {
		t.Fatal("expected response control plane to report disabled actuation")
	}
}

func TestScenarioControlsScheduleRealOverlays(t *testing.T) {
	store, hub, orch := newTestRuntime()
	scen := scenario.NewEngine()
	scen.SetMode("off")
	orch = runtime.New(testConfig(), store, hub, nil, nil, scen)

	server := NewServer(store, hub, "")
	server.SetOrchestrator(orch)
	server.SetScenarios(scen)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/control/chaos-run", strings.NewReader(`{"duration_ticks":5,"factor":2.5}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	server.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusAccepted, rr.Code, rr.Body.String())
	}
	if scen.Mode() != "on" {
		t.Fatalf("expected scenario mode on, got %q", scen.Mode())
	}
	names := scen.OverlayNames(orch.TickCount() + 1)
	if len(names) != 1 || names[0] != "api-chaos-run" {
		t.Fatalf("expected api-chaos-run overlay, got %#v", names)
	}
	if orch.ControlState().ForcedSimulationUntil == 0 {
		t.Fatal("expected simulation force window to be scheduled")
	}
}

func TestDomainControlEndpointsMutateRuntimeState(t *testing.T) {
	store, hub, orch := newTestRuntime()
	server := NewServer(store, hub, "")
	server.SetOrchestrator(orch)

	cases := []struct {
		path string
		body string
		code int
	}{
		{"/api/v1/policy/update", `{"preset":"cost"}`, http.StatusOK},
		{"/api/v1/sandbox/trigger", `{"duration_ticks":4}`, http.StatusAccepted},
		{"/api/v1/simulation/control", `{"action":"run","duration_ticks":4}`, http.StatusAccepted},
		{"/api/v1/intelligence/rollout", `{"duration_ticks":4}`, http.StatusAccepted},
		{"/api/v1/alerts/ack", `{"alert_id":"alert-1"}`, http.StatusOK},
	}

	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		server.Handler().ServeHTTP(rr, req)
		if rr.Code != tc.code {
			t.Fatalf("%s expected status %d, got %d body=%s", tc.path, tc.code, rr.Code, rr.Body.String())
		}
	}

	state := orch.ControlState()
	if state.PolicyPreset != "cost" {
		t.Fatalf("expected policy preset cost, got %q", state.PolicyPreset)
	}
	if state.ForcedSandboxUntil == 0 || state.ForcedSimulationUntil == 0 || state.ForcedIntelligenceUntil == 0 {
		t.Fatalf("expected force windows to be scheduled: %#v", state)
	}
	if state.AcknowledgedAlertCount != 1 {
		t.Fatalf("expected one acknowledged alert, got %d", state.AcknowledgedAlertCount)
	}
}

func TestRuntimeStepExecutesTick(t *testing.T) {
	store, hub, orch := newTestRuntime()
	server := NewServer(store, hub, "")
	server.SetOrchestrator(orch)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/step", nil)
	rr := httptest.NewRecorder()

	server.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, rr.Code, rr.Body.String())
	}
	if orch.ProcessedTickCount() == 0 {
		t.Fatal("expected forced runtime step to increment processed tick count")
	}
	if hub.GetLastPayload() == nil {
		t.Fatal("expected forced runtime step to broadcast a tick payload")
	}
}

func newTestRuntime() (*telemetry.Store, *streaming.Hub, *runtime.Orchestrator) {
	cfg := testConfig()
	store := telemetry.NewStore(cfg.RingBufferDepth, cfg.MaxServices, cfg.StaleServiceAge)
	hub := streaming.NewHub()
	orch := runtime.New(cfg, store, hub, nil, nil, scenario.NewEngine())
	return store, hub, orch
}

func testConfig() *config.Config {
	return &config.Config{
		TickInterval:             100 * time.Millisecond,
		TickDeadline:             80 * time.Millisecond,
		RingBufferDepth:          32,
		WindowFraction:           0.25,
		WorkerPoolSize:           2,
		MaxServices:              8,
		StaleServiceAge:          time.Minute,
		SimBudget:                20 * time.Millisecond,
		SimHorizonMs:             1000,
		SimShockFactor:           1.5,
		SimAsyncBuffer:           2,
		UtilisationSetpoint:      0.70,
		CollapseThreshold:        0.90,
		EWMAFastAlpha:            0.30,
		EWMASlowAlpha:            0.10,
		SpikeZScore:              3.0,
		PIDKp:                    -1.5,
		PIDKi:                    -0.3,
		PIDKd:                    -0.1,
		PIDDeadband:              0.02,
		PIDIntegralMax:           2.0,
		MaxStreamClients:         8,
		ArrivalEstimatorMode:     "ewma",
		PredictiveHorizonTicks:   5,
		MaxReasoningCooldowns:    50,
		SimStochasticMode:        "exponential",
		SafetyModeThreshold:      3,
		MinTickInterval:          50 * time.Millisecond,
		MaxTickInterval:          time.Second,
		TickAdaptStep:            1.25,
		StalenessBypassThreshold: 0.70,
		SLALatencyThresholdMs:    500,
		ScenarioMode:             "off",
	}
}
