package collector

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"
)

func NewHTTPHandler(c *Collector) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":              "ok",
			"component":           "le_collector",
			"discovered_services": len(c.Targets()),
			"circuit_open":        c.Stats().CircuitOpen.Load(),
		})
	})
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		status := http.StatusOK
		state := "ready"
		if c.Stats().CircuitOpen.Load() {
			status = http.StatusServiceUnavailable
			state = "degraded"
		}
		writeJSON(w, status, map[string]interface{}{
			"status":              state,
			"discovered_services": len(c.Targets()),
		})
	})
	mux.HandleFunc("/targets", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		targets := c.Targets()
		sort.Slice(targets, func(i, j int) bool { return targets[i].ServiceID < targets[j].ServiceID })
		writeJSON(w, http.StatusOK, targets)
	})
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		writeCollectorMetrics(w, c.Stats(), len(c.Targets()), time.Now())
	})
	return mux
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeCollectorMetrics(w http.ResponseWriter, s *Stats, targets int, now time.Time) {
	gauge := func(name, help string, value float64) {
		fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s gauge\n%s %g\n", name, help, name, name, value)
	}
	counter := func(name, help string, value int64) {
		fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s counter\n%s %d\n", name, help, name, name, value)
	}
	gauge("le_collector_discovered_services", "Currently discovered services.", float64(targets))
	counter("le_collector_scrape_success_total", "Successful Prometheus scrapes.", s.ScrapeSuccessTotal.Load())
	counter("le_collector_scrape_error_total", "Failed Prometheus scrapes.", s.ScrapeErrorTotal.Load())
	counter("le_collector_points_built_total", "Telemetry points normalized from scrapes.", s.PointsBuiltTotal.Load())
	counter("le_collector_points_queued_total", "Telemetry points accepted into the ingest queue.", s.PointsQueuedTotal.Load())
	counter("le_collector_points_dropped_total", "Telemetry points dropped due to local backpressure.", s.PointsDroppedTotal.Load())
	counter("le_collector_batches_sent_total", "Batches accepted by LoadEquilibrium ingest.", s.BatchesSentTotal.Load())
	counter("le_collector_batches_error_total", "Batches that failed after retries.", s.BatchesErrorTotal.Load())
	if s.CircuitOpen.Load() {
		gauge("le_collector_circuit_open", "Whether ingest circuit breaker is open.", 1)
	} else {
		gauge("le_collector_circuit_open", "Whether ingest circuit breaker is open.", 0)
	}
	if ts := s.LastDiscoveryUnix.Load(); ts > 0 {
		gauge("le_collector_last_discovery_age_seconds", "Seconds since last successful Docker discovery.", now.Sub(time.Unix(ts, 0)).Seconds())
	}
	if ts := s.LastScrapeUnix.Load(); ts > 0 {
		gauge("le_collector_last_scrape_age_seconds", "Seconds since last scrape cycle.", now.Sub(time.Unix(ts, 0)).Seconds())
	}
	if ts := s.LastIngestUnix.Load(); ts > 0 {
		gauge("le_collector_last_ingest_age_seconds", "Seconds since last successful ingest batch.", now.Sub(time.Unix(ts, 0)).Seconds())
	}
	if ts := s.LastErrorUnix.Load(); ts > 0 {
		gauge("le_collector_last_error_age_seconds", "Seconds since last collector error.", now.Sub(time.Unix(ts, 0)).Seconds())
	}
}
