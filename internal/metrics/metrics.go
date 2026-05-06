// Package metrics exposes a Prometheus text-format /metrics endpoint.
// Deliberately uses only stdlib — no client_golang dependency — consistent
// with the rest of the codebase (zero external deps).
package metrics

import (
	"fmt"
	"io"
	"math"
	"net/http"
	"runtime"
	"sort"
	"sync/atomic"
	"time"

	"github.com/loadequilibrium/loadequilibrium/internal/telemetry"
)

// Counters holds atomically-updated application event counters.
// Wire these into the runtime/autopilot paths that emit decisions.
type Counters struct {
	TicksTotal      atomic.Int64
	SLABreachesTotal atomic.Int64
	ScaleUpTotal    atomic.Int64
	ScaleDownTotal  atomic.Int64
	HoldTotal       atomic.Int64
	IngestTotal     atomic.Int64
	IngestErrors    atomic.Int64
}

// Handler serves GET /metrics in Prometheus text exposition format (v0.0.4).
// It is safe for concurrent use.
type Handler struct {
	store    *telemetry.Store
	counters *Counters
	start    time.Time
}

// NewHandler creates a Handler. store and counters must not be nil.
func NewHandler(store *telemetry.Store, counters *Counters) *Handler {
	return &Handler{
		store:    store,
		counters: counters,
		start:    time.Now(),
	}
}

// ServeHTTP implements http.Handler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	h.write(w)
}

// write serialises all metrics to w in Prometheus text format.
func (h *Handler) write(w io.Writer) {
	// ── 1. Go runtime metrics ────────────────────────────────────────────
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	gauge(w, "go_goroutines", "Number of goroutines currently running.", nil,
		float64(runtime.NumGoroutine()))
	gauge(w, "go_heap_alloc_bytes", "Heap bytes currently allocated.", nil,
		float64(ms.HeapAlloc))
	gauge(w, "go_heap_sys_bytes", "Heap bytes obtained from the OS.", nil,
		float64(ms.HeapSys))
	counter(w, "go_gc_runs_total", "Total GC cycles since process start.", nil,
		float64(ms.NumGC))
	gauge(w, "go_gc_pause_last_ns", "Duration of the most recent GC STW pause in nanoseconds.", nil,
		float64(ms.PauseNs[(ms.NumGC+255)%256]))

	// ── 2. Process uptime ────────────────────────────────────────────────
	gauge(w, "loadequilibrium_uptime_seconds",
		"Seconds since the process started.", nil,
		time.Since(h.start).Seconds())

	// ── 3. Application counters ──────────────────────────────────────────
	counter(w, "loadequilibrium_ticks_total",
		"Total control ticks executed.", nil,
		float64(h.counters.TicksTotal.Load()))
	counter(w, "loadequilibrium_sla_breaches_total",
		"Total SLA breaches observed (backlog or latency threshold crossed).", nil,
		float64(h.counters.SLABreachesTotal.Load()))
	counter(w, "loadequilibrium_scale_decisions_total",
		"Total scaling decisions emitted, partitioned by direction.",
		map[string]string{"direction": "scale_up"},
		float64(h.counters.ScaleUpTotal.Load()))
	counter(w, "loadequilibrium_scale_decisions_total",
		"",
		map[string]string{"direction": "scale_down"},
		float64(h.counters.ScaleDownTotal.Load()))
	counter(w, "loadequilibrium_scale_decisions_total",
		"",
		map[string]string{"direction": "hold"},
		float64(h.counters.HoldTotal.Load()))
	counter(w, "loadequilibrium_ingest_total",
		"Total telemetry points ingested.", nil,
		float64(h.counters.IngestTotal.Load()))
	counter(w, "loadequilibrium_ingest_errors_total",
		"Total telemetry ingest failures (auth, parse, store-full).", nil,
		float64(h.counters.IngestErrors.Load()))

	// ── 4. Per-service metrics from telemetry store ──────────────────────
	windows := h.store.AllWindows(60, 30*time.Second)

	// Stable ordering for diff-friendly output
	serviceIDs := make([]string, 0, len(windows))
	for id := range windows {
		serviceIDs = append(serviceIDs, id)
	}
	sort.Strings(serviceIDs)

	for _, id := range serviceIDs {
		win := windows[id]
		if win == nil {
			continue
		}
		lbl := map[string]string{"service": id}

		gauge(w, "loadequilibrium_request_rate",
			"Observed request arrival rate (rps) for the service.", lbl,
			safeF(win.LastRequestRate))
		gauge(w, "loadequilibrium_error_rate",
			"Fraction of requests resulting in errors [0,1].", lbl,
			safeF(win.LastErrorRate))
		gauge(w, "loadequilibrium_latency_mean_ms",
			"Mean observed request latency in milliseconds.", lbl,
			safeF(win.MeanLatencyMs))
		gauge(w, "loadequilibrium_latency_p99_ms",
			"P99 observed request latency in milliseconds.", lbl,
			safeF(win.LastP99LatencyMs))
		gauge(w, "loadequilibrium_queue_depth",
			"Last observed queue depth (pending requests).", lbl,
			safeF(win.LastQueueDepth))
		gauge(w, "loadequilibrium_cpu_usage",
			"Mean CPU utilisation fraction [0,1].", lbl,
			safeF(win.MeanCPU))
		gauge(w, "loadequilibrium_confidence_score",
			"Signal confidence score [0,1] — low means prediction quality is degraded.", lbl,
			safeF(win.ConfidenceScore))
		gauge(w, "loadequilibrium_hazard_score",
			"Physics-engine hazard score for the service [0,1].", lbl,
			safeF(win.Hazard))
		gauge(w, "loadequilibrium_applied_scale",
			"Last scaling directive applied by the autopilot (1.0 = no change).", lbl,
			safeF(win.AppliedScale))
	}

	// ── 5. Store health ──────────────────────────────────────────────────
	gauge(w, "loadequilibrium_tracked_services",
		"Number of services currently tracked in the telemetry store.", nil,
		float64(len(windows)))
}

// ── helpers ───────────────────────────────────────────────────────────────────

// safeF returns 0 for NaN/Inf values so Prometheus never rejects the payload.
func safeF(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return v
}

// gauge writes a GAUGE metric line. If help is empty the HELP line is skipped
// (used for repeated label variants of the same metric name).
func gauge(w io.Writer, name, help string, labels map[string]string, value float64) {
	writeMetric(w, "gauge", name, help, labels, value)
}

// counter writes a COUNTER metric line.
func counter(w io.Writer, name, help string, labels map[string]string, value float64) {
	writeMetric(w, "counter", name, help, labels, value)
}

func writeMetric(w io.Writer, typ, name, help string, labels map[string]string, value float64) {
	if help != "" {
		fmt.Fprintf(w, "# HELP %s %s\n", name, help)
		fmt.Fprintf(w, "# TYPE %s %s\n", name, typ)
	}
	if len(labels) == 0 {
		fmt.Fprintf(w, "%s %g\n", name, value)
		return
	}
	// Stable label ordering
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Fprintf(w, "%s{", name)
	for i, k := range keys {
		if i > 0 {
			fmt.Fprint(w, ",")
		}
		fmt.Fprintf(w, `%s=%q`, k, labels[k])
	}
	fmt.Fprintf(w, "} %g\n", value)
}