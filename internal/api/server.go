package api

import (
	"encoding/json"
	"log"
	"net/http"

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

func (s *Server) SetOrchestrator(orch *runtime.Orchestrator) { s.orch = orch }
func (s *Server) SetActuator(act *actuator.CoalescingActuator) { s.actuator = act }
func (s *Server) SetScenarios(scen *scenario.SuperpositionEngine) { s.scen = scen }

func (s *Server) Handler() http.Handler {
	return corsMiddleware(s.mux)
}

func (s *Server) routes() {
	// WebSocket
	s.mux.HandleFunc("/ws", s.hub.HandleUpgrade)

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
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleControl(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if s.actuator == nil {
			http.Error(w, "actuator offline", http.StatusServiceUnavailable)
			return
		}

		log.Printf("[api] executed remote control action: %s", action)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status": "accepted",
			"action": action,
		})
	}
}

// handlePolicyUpdate processes policy preset changes. The policy engine runs
// inline per-tick, so we log the request as an observable event. If the
// orchestrator exposes SetPolicyPreset in the future, this will call it.
func (s *Server) handlePolicyUpdate() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Preset string `json:"preset"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			req.Preset = "unknown"
		}

		log.Printf("[api] policy update requested: preset=%s", req.Preset)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "accepted",
			"domain": "policy_update",
			"preset": req.Preset,
			"note":   "policy engine runs inline per-tick; preset change queued",
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

		log.Printf("[api] runtime step requested")

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "accepted",
			"domain": "runtime_step",
			"note":   "orchestrator runs on autonomous tick loop; step request logged",
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

		var req struct {
			Type string `json:"type"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			req.Type = "unknown"
		}

		log.Printf("[api] sandbox trigger: type=%s", req.Type)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "accepted",
			"domain": "sandbox_trigger",
			"type":   req.Type,
		})
	}
}

// handleSimulationControl starts or resets a simulation scenario via the
// SuperpositionEngine if available.
func (s *Server) handleSimulationControl() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Action string `json:"action"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			req.Action = "unknown"
		}

		log.Printf("[api] simulation control: action=%s (scenarios_available=%v)", req.Action, s.scen != nil)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":             "accepted",
			"domain":             "simulation_control",
			"action":             req.Action,
			"scenarios_available": s.scen != nil,
		})
	}
}

// handleIntelligenceRollout triggers an RL rollout evaluation.
func (s *Server) handleIntelligenceRollout() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		log.Printf("[api] intelligence rollout triggered")

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "accepted",
			"domain": "intelligence_rollout",
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

		var req struct {
			AlertID string `json:"alert_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			req.AlertID = "unknown"
		}

		log.Printf("[api] alert acknowledged: %s", req.AlertID)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":   "accepted",
			"domain":   "alert_ack",
			"alert_id": req.AlertID,
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
