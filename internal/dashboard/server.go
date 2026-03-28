package dashboard

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/loadequilibrium/loadequilibrium/internal/streaming"
	"github.com/loadequilibrium/loadequilibrium/internal/telemetry"
)

type Server struct {
	store       *telemetry.Store
	hub         *streaming.Hub
	mux         *http.ServeMux
	ingestToken string
}

func New(store *telemetry.Store, hub *streaming.Hub, ingestToken string) *Server {
	s := &Server{store: store, hub: hub, mux: http.NewServeMux(), ingestToken: ingestToken}
	s.registerRoutes()
	return s
}

func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("/api/v1/ingest", s.handleIngest)
	s.mux.HandleFunc("/ws", s.handleWebSocket)
	s.mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "ok",
			"time":    time.Now().UTC(),
			"clients": s.hub.ClientCount(),
		})
	})
	s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(dashboardHTML)
	})
}

func (s *Server) handleIngest(w http.ResponseWriter, r *http.Request) {
	if s.ingestToken != "" {
		tok := r.Header.Get("X-Ingest-Token")
		if tok == "" {
			tok = r.URL.Query().Get("token")
		}
		if tok != s.ingestToken {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var points []*telemetry.MetricPoint
	if err := json.Unmarshal(body, &points); err != nil {
		var single telemetry.MetricPoint
		if err2 := json.Unmarshal(body, &single); err2 != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		points = []*telemetry.MetricPoint{&single}
	}
	ingested := 0
	for _, p := range points {
		if p.ServiceID != "" {
			s.store.Ingest(p)
			ingested++
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]int{"ingested": ingested})
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	log.Printf("[ws] connect %s", r.RemoteAddr)
	s.hub.HandleUpgrade(w, r)
}
