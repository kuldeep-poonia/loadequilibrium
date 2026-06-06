package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/loadequilibrium/loadequilibrium/internal/actuator"
	"github.com/loadequilibrium/loadequilibrium/internal/metrics"
	"github.com/loadequilibrium/loadequilibrium/internal/runtime"
	"github.com/loadequilibrium/loadequilibrium/internal/scenario"
	"github.com/loadequilibrium/loadequilibrium/internal/security"
	"github.com/loadequilibrium/loadequilibrium/internal/streaming"
	"github.com/loadequilibrium/loadequilibrium/internal/telemetry"
	"github.com/loadequilibrium/loadequilibrium/internal/ws"
)

// ── pool: reusable []MetricPoint slices ──────────────────────────────────────

var ingestPointPool = sync.Pool{
	New: func() any {
		pts := make([]telemetry.MetricPoint, 0, 32)
		return &pts
	},
}

var acceptedResponse = []byte(`{"status":"accepted"}`)

// ── Server ───────────────────────────────────────────────────────────────────

// Server is the LoadEquilibrium HTTP server.
// All security enforcement happens in Handler() before any route handler runs.
type Server struct {
	store    *telemetry.Store
	hub      *streaming.Hub
	actuator *actuator.CoalescingActuator
	orch     *runtime.Orchestrator
	scen     *scenario.SuperpositionEngine
	mux      *http.ServeMux
	upgrader ws.Upgrader
	metrics  *metrics.Handler
	Counters *metrics.Counters
	audit    *security.AuditLogger

	// Rate limiters: separate limits for ingest and control paths.
	// Ingest: high burst (collector sends every 2s per service batch)
	// Control: low burst (human operators issue occasional commands)
	ingestRL  *security.RateLimiter
	controlRL *security.RateLimiter

	secCfg security.AuthConfig
}

// ServerConfig holds everything NewServer needs.
type ServerConfig struct {
	IngestToken    string   // required for production
	ControlToken   string   // if empty, falls back to IngestToken
	DashboardToken string   // if empty, dashboard is unauthenticated (ok for internal)
	AllowedOrigins []string // CORS allowed origins; nil = allow all (internal networks)
	TLSEnabled     bool
	MetricsToken   string // if set, GET /metrics requires this token

	// Rate limits (requests/second sustained; burst is 3× sustained)
	IngestRPS  float64 // default: 5000 (collector batches are small and frequent)
	ControlRPS float64 // default: 10 (human operators, not scripts)
}

func NewServer(store *telemetry.Store, hub *streaming.Hub, token string) *Server {
	return NewServerWithConfig(store, hub, ServerConfig{IngestToken: token})
}

func NewServerWithConfig(store *telemetry.Store, hub *streaming.Hub, cfg ServerConfig) *Server {
	if cfg.IngestRPS <= 0 {
		cfg.IngestRPS = 5000
	}
	if cfg.ControlRPS <= 0 {
		cfg.ControlRPS = 10
	}

	security.WarnWeakToken(cfg.IngestToken)
	if cfg.ControlToken != "" && cfg.ControlToken != cfg.IngestToken {
		security.WarnWeakToken(cfg.ControlToken)
	}

	counters := &metrics.Counters{}
	audit := security.NewAuditLogger()

	s := &Server{
		store:    store,
		hub:      hub,
		mux:      http.NewServeMux(),
		Counters: counters,
		metrics:  metrics.NewHandler(store, counters),
		audit:    audit,
		// Ingest: 3× burst headroom for bursty collector batches
		ingestRL: security.NewRateLimiter(cfg.IngestRPS*3, cfg.IngestRPS, 5*time.Minute),
		// Control: 30× burst — still only ~300 reqs before throttle kicks in
		controlRL: security.NewRateLimiter(cfg.ControlRPS*30, cfg.ControlRPS, 5*time.Minute),
		secCfg: security.AuthConfig{
			IngestToken:    cfg.IngestToken,
			ControlToken:   cfg.ControlToken,
			DashboardToken: cfg.DashboardToken,
			AllowedOrigins: cfg.AllowedOrigins,
			TLSEnabled:     cfg.TLSEnabled,
		},
		upgrader: ws.Upgrader{
			ReadBufferSize:  8192,
			WriteBufferSize: 65536,
			CheckOrigin: func(r *http.Request) bool {
				// Origin validation is handled by CORSMiddleware before upgrade.
				// Allow all at the WS layer — CORS already filtered.
				return true
			},
		},
	}
	s.routes()
	return s
}

func (s *Server) SetOrchestrator(orch *runtime.Orchestrator)      { s.orch = orch }
func (s *Server) SetActuator(act *actuator.CoalescingActuator)    { s.actuator = act }
func (s *Server) SetScenarios(scen *scenario.SuperpositionEngine) { s.scen = scen }

// Handler builds the fully hardened handler chain.
//
// Middleware order (outermost → innermost):
//  1. Security headers (X-Frame-Options, CSP, HSTS, etc.)
//  2. CORS policy (strict origin whitelist, no wildcard)
//  3. Auth + rate limiting (per route class, constant-time token compare)
//  4. Route handler
func (s *Server) Handler() http.Handler {
	return security.SecurityHeaders(s.secCfg.TLSEnabled)(
		security.CORSMiddleware(s.secCfg.AllowedOrigins)(
			security.AuthMiddleware(s.secCfg, s.audit, s.ingestRL, s.controlRL)(
				s.mux,
			),
		),
	)
}

func (s *Server) routes() {
	// WebSocket — auth enforced by AuthMiddleware before upgrade
	s.mux.HandleFunc("/ws", s.hub.HandleUpgrade)

	// UI — served from ./ui/ directory (no auth; UI loads the token from env)
	uiDir := http.Dir("ui")
	uiServer := http.FileServer(uiDir)
	s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") ||
			strings.HasPrefix(r.URL.Path, "/ws") ||
			strings.HasPrefix(r.URL.Path, "/health") ||
			strings.HasPrefix(r.URL.Path, "/metrics") {
			http.NotFound(w, r)
			return
		}
		uiServer.ServeHTTP(w, r)
	})

	// ── Telemetry ingest (hot path) ───────────────────────────────────────────
	s.mux.HandleFunc("/api/v1/ingest", s.handleIngest())

	// ── Read-only endpoints ───────────────────────────────────────────────────
	s.mux.HandleFunc("/api/v1/snapshot", s.handleSnapshot())

	// ── Control plane (requires ControlToken) ────────────────────────────────
	s.mux.HandleFunc("/api/v1/control/toggle", s.handleControl("toggle"))
	s.mux.HandleFunc("/api/v1/control/chaos-run", s.handleControl("chaos-run"))
	s.mux.HandleFunc("/api/v1/control/replay-burst", s.handleControl("replay-burst"))
	s.mux.HandleFunc("/api/v1/policy/update", s.handlePolicyUpdate())
	s.mux.HandleFunc("/api/v1/runtime/step", s.handleRuntimeStep())
	s.mux.HandleFunc("/api/v1/sandbox/trigger", s.handleSandboxTrigger())
	s.mux.HandleFunc("/api/v1/simulation/control", s.handleSimulationControl())
	s.mux.HandleFunc("/api/v1/intelligence/rollout", s.handleIntelligenceRollout())
	s.mux.HandleFunc("/api/v1/alerts/ack", s.handleAlertAck())

	// ── Audit log (control token required) ───────────────────────────────────
	// Exposes the last 200 security events for operator review.
	s.mux.HandleFunc("/api/v1/audit", s.handleAuditLog())

	// ── Health + metrics ──────────────────────────────────────────────────────
	s.mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		clientCount := 0
		if s.hub != nil {
			clientCount = s.hub.ClientCount()
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":    "ok",
			"component": "api_headless",
			"clients":   clientCount,
		})
	})
	s.mux.Handle("/metrics", s.metrics)
}

// ── handleIngest — hardened hot path ─────────────────────────────────────────

func (s *Server) handleIngest() http.HandlerFunc {
	const maxIngestBodyBytes = 4 << 20 // 4 MiB

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		if s.store == nil {
			http.Error(w, `{"error":"telemetry store offline"}`, http.StatusServiceUnavailable)
			return
		}

		// ── Decode ────────────────────────────────────────────────────────────
		body, err := io.ReadAll(io.LimitReader(r.Body, maxIngestBodyBytes))
		if err != nil {
			s.Counters.IngestErrors.Add(1)
			http.Error(w, `{"error":"failed to read body"}`, http.StatusBadRequest)
			return
		}
		body = bytes.TrimSpace(body)
		if len(body) == 0 {
			s.Counters.IngestErrors.Add(1)
			http.Error(w, `{"error":"empty payload"}`, http.StatusBadRequest)
			return
		}

		ptsPtr := ingestPointPool.Get().(*[]telemetry.MetricPoint)
		pts := (*ptsPtr)[:0]

		switch body[0] {
		case '[':
			if err := json.Unmarshal(body, &pts); err != nil {
				ingestPointPool.Put(ptsPtr)
				s.Counters.IngestErrors.Add(1)
				http.Error(w, `{"error":"invalid array payload"}`, http.StatusBadRequest)
				return
			}
		case '{':
			var pt telemetry.MetricPoint
			if err := json.Unmarshal(body, &pt); err != nil {
				ingestPointPool.Put(ptsPtr)
				s.Counters.IngestErrors.Add(1)
				http.Error(w, `{"error":"invalid object payload"}`, http.StatusBadRequest)
				return
			}
			pts = append(pts, pt)
		default:
			ingestPointPool.Put(ptsPtr)
			s.Counters.IngestErrors.Add(1)
			http.Error(w, `{"error":"payload must be object or array"}`, http.StatusBadRequest)
			return
		}

		// ── Batch size guard ──────────────────────────────────────────────────
		if err := security.ValidateBatchSize(len(pts)); err != nil {
			ingestPointPool.Put(ptsPtr)
			s.Counters.IngestErrors.Add(1)
			http.Error(w, `{"error":"batch too large"}`, http.StatusRequestEntityTooLarge)
			return
		}

		// ── Per-point validation + ingest ─────────────────────────────────────
		accepted := 0
		for i := range pts {
			// Validate service_id: length, charset — no injection possible
			if err := security.ValidateServiceID(pts[i].ServiceID); err != nil {
				s.Counters.IngestErrors.Add(1)
				continue // skip invalid point, don't reject whole batch
			}
			s.store.Ingest(&pts[i])
			s.Counters.IngestTotal.Add(1)
			accepted++
		}

		*ptsPtr = pts[:0]
		ingestPointPool.Put(ptsPtr)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		w.Write(acceptedResponse) //nolint:errcheck
	}
}

// ── handleSnapshot ────────────────────────────────────────────────────────────

func (s *Server) handleSnapshot() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		data := s.hub.GetLastPayloadJSON()
		if data == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(data) //nolint:errcheck
	}
}

// ── handleAuditLog ────────────────────────────────────────────────────────────

func (s *Server) handleAuditLog() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		events := s.audit.Recent(200)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"count":  len(events),
			"events": events,
		})
	}
}

// ── handleControl ─────────────────────────────────────────────────────────────

func (s *Server) handleControl(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		if s.orch == nil {
			writeError(w, http.StatusServiceUnavailable, "runtime orchestrator offline")
			return
		}

		var req struct {
			Enabled       *bool   `json:"enabled"`
			ServiceID     string  `json:"service_id"`
			DurationTicks uint64  `json:"duration_ticks"`
			Factor        float64 `json:"factor"`
			RequestFactor float64 `json:"request_factor"`
			LatencyFactor float64 `json:"latency_factor"`
		}
		if err := decodeOptionalJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid control payload")
			return
		}

		// Validate service_id if provided
		if req.ServiceID != "" {
			if err := security.ValidateServiceID(req.ServiceID); err != nil {
				writeError(w, http.StatusBadRequest, "invalid service_id")
				return
			}
		}

		ip := security.RealIP(r)
		s.audit.Log(security.AuditEvent{
			IP:        ip,
			Path:      r.URL.Path,
			Method:    r.Method,
			EventType: "control_action",
			Detail:    action,
		})

		switch action {
		case "toggle":
			enabled := s.orch.ToggleActuation()
			if req.Enabled != nil {
				enabled = s.orch.SetActuationEnabled(*req.Enabled)
			}
			log.Printf("[api] actuation enabled=%v ip=%s", enabled, ip)
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"status":            "applied",
				"action":            action,
				"actuation_enabled": enabled,
				"control_plane":     s.orch.ControlState(),
			})
		case "chaos-run":
			if s.scen == nil {
				writeError(w, http.StatusServiceUnavailable, "scenario engine offline")
				return
			}
			duration := boundedTicks(req.DurationTicks, 30, 1, 600)
			target := serviceTarget(req.ServiceID)
			requestFactor := boundedFloat(firstNonZero(req.RequestFactor, req.Factor), 2.5, 1.0, 10.0)
			latencyFactor := boundedFloat(req.LatencyFactor, 1.6, 1.0, 10.0)
			start := s.orch.TickCount() + 1
			until := start + duration
			s.scen.SetMode("on")
			s.scen.SetOverlay("api-chaos-run", &scenario.WindowedDisturbance{
				ScenarioID:    "api-chaos-run",
				TargetService: target,
				StartTick:     start,
				DurationTicks: duration,
				RequestFactor: requestFactor,
				LatencyFactor: latencyFactor,
			}, until)
			s.orch.ForceSimulation(duration + 1)
			log.Printf("[api] chaos-run target=%s start=%d until=%d rf=%.2f lf=%.2f ip=%s",
				target, start, until, requestFactor, latencyFactor, ip)
			writeJSON(w, http.StatusAccepted, map[string]interface{}{
				"status":         "scheduled",
				"action":         action,
				"target_service": target,
				"start_tick":     start,
				"until_tick":     until,
				"request_factor": requestFactor,
				"latency_factor": latencyFactor,
			})
		case "replay-burst":
			if s.scen == nil {
				writeError(w, http.StatusServiceUnavailable, "scenario engine offline")
				return
			}
			duration := boundedTicks(req.DurationTicks, 20, 1, 600)
			target := serviceTarget(req.ServiceID)
			factor := boundedFloat(req.Factor, 2.0, 1.0, 10.0)
			start := s.orch.TickCount() + 1
			until := start + duration
			s.scen.SetMode("on")
			s.scen.SetOverlay("api-replay-burst", &scenario.ResettableBurst{
				ScenarioID:    "api-replay-burst",
				TargetService: target,
				StartTick:     start,
				DurationTicks: duration,
				MaxFactor:     factor,
			}, until)
			s.orch.ForceSimulation(duration + 1)
			log.Printf("[api] replay-burst target=%s start=%d until=%d factor=%.2f ip=%s",
				target, start, until, factor, ip)
			writeJSON(w, http.StatusAccepted, map[string]interface{}{
				"status":         "scheduled",
				"action":         action,
				"target_service": target,
				"start_tick":     start,
				"until_tick":     until,
				"factor":         factor,
			})
		default:
			writeError(w, http.StatusNotFound, "unknown control action")
		}
	}
}

// ── handlePolicyUpdate ────────────────────────────────────────────────────────

func (s *Server) handlePolicyUpdate() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		if s.orch == nil {
			writeError(w, http.StatusServiceUnavailable, "runtime orchestrator offline")
			return
		}
		var req struct {
			Preset string `json:"preset"`
		}
		if err := decodeOptionalJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid policy payload")
			return
		}
		preset := s.orch.SetPolicyPreset(req.Preset)
		ip := security.RealIP(r)
		s.audit.Log(security.AuditEvent{
			IP: ip, Path: r.URL.Path, Method: r.Method,
			EventType: "control_action",
			Detail:    "policy_update preset=" + preset,
		})
		log.Printf("[api] policy preset=%s ip=%s", preset, ip)
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":        "applied",
			"preset":        preset,
			"control_plane": s.orch.ControlState(),
		})
	}
}

// ── handleRuntimeStep ─────────────────────────────────────────────────────────

func (s *Server) handleRuntimeStep() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		if s.orch == nil {
			writeError(w, http.StatusServiceUnavailable, "runtime orchestrator offline")
			return
		}
		tick, err := s.orch.StepOnce(time.Now())
		if errors.Is(err, runtime.ErrTickInFlight) {
			writeJSON(w, http.StatusConflict, map[string]interface{}{
				"status": "busy", "tick": tick,
			})
			return
		}
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, err.Error())
			return
		}
		s.audit.Log(security.AuditEvent{
			IP: security.RealIP(r), Path: r.URL.Path, Method: r.Method,
			EventType: "control_action", Detail: "runtime_step",
		})
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status": "executed", "tick": tick,
			"control_plane": s.orch.ControlState(),
		})
	}
}

// ── handleSandboxTrigger ──────────────────────────────────────────────────────

func (s *Server) handleSandboxTrigger() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		if s.orch == nil {
			writeError(w, http.StatusServiceUnavailable, "runtime orchestrator offline")
			return
		}
		var req struct {
			Type          string `json:"type"`
			DurationTicks uint64 `json:"duration_ticks"`
		}
		if err := decodeOptionalJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid sandbox payload")
			return
		}
		if strings.TrimSpace(req.Type) == "" {
			req.Type = "experiment"
		}
		duration := boundedTicks(req.DurationTicks, 10, 1, 120)
		until := s.orch.ForceSandbox(duration)
		s.audit.Log(security.AuditEvent{
			IP: security.RealIP(r), Path: r.URL.Path, Method: r.Method,
			EventType: "control_action", Detail: "sandbox type=" + req.Type,
		})
		writeJSON(w, http.StatusAccepted, map[string]interface{}{
			"status": "scheduled", "type": req.Type, "until_tick": until,
		})
	}
}

// ── handleSimulationControl ───────────────────────────────────────────────────

func (s *Server) handleSimulationControl() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		if s.orch == nil {
			writeError(w, http.StatusServiceUnavailable, "runtime orchestrator offline")
			return
		}
		var req struct {
			Action        string `json:"action"`
			DurationTicks uint64 `json:"duration_ticks"`
		}
		if err := decodeOptionalJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid simulation payload")
			return
		}
		action := strings.ToLower(strings.TrimSpace(req.Action))
		if action == "" {
			action = "run"
		}
		duration := boundedTicks(req.DurationTicks, 10, 1, 600)
		switch action {
		case "run", "start", "force":
			until := s.orch.ForceSimulation(duration)
			writeJSON(w, http.StatusAccepted, map[string]interface{}{
				"status": "scheduled", "action": action, "until_tick": until,
			})
		case "reset":
			s.orch.RequestSimulationReset()
			writeJSON(w, http.StatusAccepted, map[string]interface{}{
				"status": "scheduled", "action": action,
			})
		case "stop":
			s.orch.ForceSimulation(0)
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"status": "applied", "action": action,
			})
		default:
			writeError(w, http.StatusBadRequest, "unknown simulation action")
		}
	}
}

// ── handleIntelligenceRollout ─────────────────────────────────────────────────

func (s *Server) handleIntelligenceRollout() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		if s.orch == nil {
			writeError(w, http.StatusServiceUnavailable, "runtime orchestrator offline")
			return
		}
		var req struct {
			DurationTicks uint64 `json:"duration_ticks"`
		}
		if err := decodeOptionalJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid intelligence payload")
			return
		}
		duration := boundedTicks(req.DurationTicks, 10, 1, 120)
		until := s.orch.ForceIntelligenceRollout(duration)
		writeJSON(w, http.StatusAccepted, map[string]interface{}{
			"status": "scheduled", "until_tick": until,
		})
	}
}

// ── handleAlertAck ────────────────────────────────────────────────────────────

func (s *Server) handleAlertAck() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		if s.orch == nil {
			writeError(w, http.StatusServiceUnavailable, "runtime orchestrator offline")
			return
		}
		var req struct {
			AlertID string `json:"alert_id"`
		}
		if err := decodeOptionalJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid alert payload")
			return
		}
		if strings.TrimSpace(req.AlertID) == "" {
			writeError(w, http.StatusBadRequest, "alert_id is required")
			return
		}
		count, ok := s.orch.AcknowledgeAlert(req.AlertID, time.Now())
		if !ok {
			writeError(w, http.StatusNotFound, "alert not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status": "applied", "alert_id": req.AlertID, "count": count,
		})
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func decodeOptionalJSON(r *http.Request, dst interface{}) error {
	if r.Body == nil || r.ContentLength == 0 {
		return nil
	}
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(dst); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func boundedTicks(value, fallback, min, max uint64) uint64 {
	if value == 0 {
		value = fallback
	}
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func boundedFloat(value, fallback, min, max float64) float64 {
	if value == 0 {
		value = fallback
	}
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func firstNonZero(values ...float64) float64 {
	for _, v := range values {
		if v != 0 {
			return v
		}
	}
	return 0
}

func serviceTarget(serviceID string) string {
	serviceID = strings.TrimSpace(serviceID)
	if serviceID == "" {
		return "*"
	}
	return serviceID
}