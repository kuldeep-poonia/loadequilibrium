package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/loadequilibrium/loadequilibrium/internal/actuator"
	"github.com/loadequilibrium/loadequilibrium/internal/runtime"
	"github.com/loadequilibrium/loadequilibrium/internal/scenario"
	"github.com/loadequilibrium/loadequilibrium/internal/streaming"
	"github.com/loadequilibrium/loadequilibrium/internal/telemetry"
	"github.com/loadequilibrium/loadequilibrium/internal/ws"
)

type Server struct {
	store    *telemetry.Store
	hub      *streaming.Hub
	token    string
	actuator *actuator.CoalescingActuator
	orch     *runtime.Orchestrator
	scen     *scenario.SuperpositionEngine
	mux      *http.ServeMux
	upgrader ws.Upgrader
}

func NewServer(store *telemetry.Store, hub *streaming.Hub, token string) *Server {
	s := &Server{
		store: store,
		hub:   hub,
		token: token,
		mux:   http.NewServeMux(),
		upgrader: ws.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
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

func (s *Server) Handler() http.Handler {
	return corsMiddleware(s.mux)
}

func (s *Server) routes() {
	// WebSocket
	s.mux.HandleFunc("/ws", s.hub.HandleUpgrade)

	// Telemetry ingestion
	s.mux.HandleFunc("/api/v1/ingest", s.handleIngest())

	// Actuator proxy endpoints
	s.mux.HandleFunc("/api/v1/control/toggle", s.handleControl("toggle"))
	s.mux.HandleFunc("/api/v1/control/chaos-run", s.handleControl("chaos-run"))
	s.mux.HandleFunc("/api/v1/control/replay-burst", s.handleControl("replay-burst"))

	// Domain-specific safe control-plane endpoints
	s.mux.HandleFunc("/api/v1/policy/update", s.handlePolicyUpdate())
	s.mux.HandleFunc("/api/v1/runtime/step", s.handleRuntimeStep())
	s.mux.HandleFunc("/api/v1/sandbox/trigger", s.handleSandboxTrigger())
	s.mux.HandleFunc("/api/v1/simulation/control", s.handleSimulationControl())
	s.mux.HandleFunc("/api/v1/intelligence/rollout", s.handleIntelligenceRollout())
	s.mux.HandleFunc("/api/v1/alerts/ack", s.handleAlertAck())

	// Snapshot endpoint — returns last cached tick payload via REST
	s.mux.HandleFunc("/api/v1/snapshot", s.handleSnapshot())

	// Health
	s.mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":    "ok",
			"component": "api_headless",
			"clients":   s.hub.ClientCount(),
		})
	})
}

// corsMiddleware adds CORS headers for dashboard access
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Ingest-Token")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleIngest() http.HandlerFunc {
	const maxIngestBodyBytes = 4 << 20

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if s.store == nil {
			http.Error(w, "telemetry store offline", http.StatusServiceUnavailable)
			return
		}

		if s.token != "" {
			headerToken := r.Header.Get("X-Ingest-Token")
			bearerToken := "Bearer " + s.token
			if headerToken != s.token && r.Header.Get("Authorization") != bearerToken {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}

		var raw json.RawMessage
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxIngestBodyBytes))
		if err := decoder.Decode(&raw); err != nil {
			http.Error(w, "invalid telemetry payload", http.StatusBadRequest)
			return
		}

		raw = bytes.TrimSpace(raw)
		if len(raw) == 0 {
			http.Error(w, "empty telemetry payload", http.StatusBadRequest)
			return
		}

		points := make([]telemetry.MetricPoint, 0, 1)
		switch raw[0] {
		case '[':
			if err := json.Unmarshal(raw, &points); err != nil {
				http.Error(w, "invalid telemetry point array", http.StatusBadRequest)
				return
			}
		case '{':
			var point telemetry.MetricPoint
			if err := json.Unmarshal(raw, &point); err != nil {
				http.Error(w, "invalid telemetry point", http.StatusBadRequest)
				return
			}
			points = append(points, point)
		default:
			http.Error(w, "telemetry payload must be an object or array", http.StatusBadRequest)
			return
		}

		if len(points) == 0 {
			http.Error(w, "telemetry payload contained no points", http.StatusBadRequest)
			return
		}

		for i := range points {
			if points[i].ServiceID == "" {
				http.Error(w, "telemetry point missing service_id", http.StatusBadRequest)
				return
			}
			s.store.Ingest(&points[i])
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "accepted",
			"points": len(points),
		})
	}
}

func (s *Server) handleControl(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
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

		switch action {
		case "toggle":
			enabled := s.orch.ToggleActuation()
			if req.Enabled != nil {
				enabled = s.orch.SetActuationEnabled(*req.Enabled)
			}
			log.Printf("[api] actuation enabled=%v", enabled)
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"status":              "applied",
				"action":              action,
				"actuation_enabled":   enabled,
				"actuator_configured": s.actuator != nil,
				"control_plane":       s.orch.ControlState(),
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
			log.Printf("[api] scenario overlay chaos-run target=%s start=%d until=%d req_factor=%.2f latency_factor=%.2f",
				target, start, until, requestFactor, latencyFactor)
			writeJSON(w, http.StatusAccepted, map[string]interface{}{
				"status":         "scheduled",
				"action":         action,
				"target_service": target,
				"start_tick":     start,
				"until_tick":     until,
				"request_factor": requestFactor,
				"latency_factor": latencyFactor,
				"scenario_mode":  s.scen.Mode(),
				"control_plane":  s.orch.ControlState(),
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
			log.Printf("[api] scenario overlay replay-burst target=%s start=%d until=%d factor=%.2f",
				target, start, until, factor)
			writeJSON(w, http.StatusAccepted, map[string]interface{}{
				"status":         "scheduled",
				"action":         action,
				"target_service": target,
				"start_tick":     start,
				"until_tick":     until,
				"factor":         factor,
				"scenario_mode":  s.scen.Mode(),
				"control_plane":  s.orch.ControlState(),
			})
		default:
			writeError(w, http.StatusNotFound, "unknown control action")
		}
	}
}

// handlePolicyUpdate applies policy presets to the runtime policy engine.
func (s *Server) handlePolicyUpdate() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
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
		log.Printf("[api] policy preset applied=%s", preset)
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":        "applied",
			"domain":        "policy_update",
			"preset":        preset,
			"control_plane": s.orch.ControlState(),
		})
	}
}

// handleRuntimeStep requests a forced tick from the orchestrator.
func (s *Server) handleRuntimeStep() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if s.orch == nil {
			writeError(w, http.StatusServiceUnavailable, "runtime orchestrator offline")
			return
		}

		tick, err := s.orch.StepOnce(time.Now())
		if errors.Is(err, runtime.ErrTickInFlight) {
			writeJSON(w, http.StatusConflict, map[string]interface{}{
				"status":        "busy",
				"domain":        "runtime_step",
				"tick":          tick,
				"control_plane": s.orch.ControlState(),
			})
			return
		}
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, err.Error())
			return
		}

		log.Printf("[api] runtime step executed tick=%d", tick)
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":        "executed",
			"domain":        "runtime_step",
			"tick":          tick,
			"control_plane": s.orch.ControlState(),
		})
	}
}

// handleSandboxTrigger enqueues a sandbox experiment scenario.
func (s *Server) handleSandboxTrigger() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
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
		log.Printf("[api] sandbox forced type=%s until_tick=%d", req.Type, until)

		writeJSON(w, http.StatusAccepted, map[string]interface{}{
			"status":         "scheduled",
			"domain":         "sandbox_trigger",
			"type":           req.Type,
			"until_tick":     until,
			"duration_ticks": duration,
			"control_plane":  s.orch.ControlState(),
		})
	}
}

// handleSimulationControl starts, stops, or resets the real simulation runner.
func (s *Server) handleSimulationControl() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
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
			log.Printf("[api] simulation forced until_tick=%d", until)
			writeJSON(w, http.StatusAccepted, map[string]interface{}{
				"status":         "scheduled",
				"domain":         "simulation_control",
				"action":         action,
				"until_tick":     until,
				"duration_ticks": duration,
				"control_plane":  s.orch.ControlState(),
			})
		case "reset":
			s.orch.RequestSimulationReset()
			log.Printf("[api] simulation reset requested")
			writeJSON(w, http.StatusAccepted, map[string]interface{}{
				"status":        "scheduled",
				"domain":        "simulation_control",
				"action":        action,
				"control_plane": s.orch.ControlState(),
			})
		case "stop":
			s.orch.ForceSimulation(0)
			log.Printf("[api] simulation force window cleared")
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"status":        "applied",
				"domain":        "simulation_control",
				"action":        action,
				"control_plane": s.orch.ControlState(),
			})
		default:
			writeError(w, http.StatusBadRequest, "unknown simulation action")
		}
	}
}

// handleIntelligenceRollout triggers an RL rollout evaluation.
func (s *Server) handleIntelligenceRollout() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
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
		log.Printf("[api] intelligence rollout forced until_tick=%d", until)
		writeJSON(w, http.StatusAccepted, map[string]interface{}{
			"status":         "scheduled",
			"domain":         "intelligence_rollout",
			"until_tick":     until,
			"duration_ticks": duration,
			"control_plane":  s.orch.ControlState(),
		})
	}
}

// handleAlertAck acknowledges an alert.
func (s *Server) handleAlertAck() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
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
			writeError(w, http.StatusBadRequest, "alert_id is required")
			return
		}
		log.Printf("[api] alert acknowledged: %s", req.AlertID)

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":                   "applied",
			"domain":                   "alert_ack",
			"alert_id":                 req.AlertID,
			"acknowledged_alert_count": count,
			"control_plane":            s.orch.ControlState(),
		})
	}
}

// handleSnapshot returns the last cached tick payload as REST JSON.
func (s *Server) handleSnapshot() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		data := s.hub.GetLastPayloadJSON()
		if data == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]string{
				"status": "no_data",
				"reason": "no tick payload cached yet",
			})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	}
}

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

func firstNonZero(values ...float64) float64 {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
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

func serviceTarget(serviceID string) string {
	serviceID = strings.TrimSpace(serviceID)
	if serviceID == "" {
		return "*"
	}
	return serviceID
}
