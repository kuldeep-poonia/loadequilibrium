package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/loadequilibrium/loadequilibrium/internal/actuator"
	"github.com/loadequilibrium/loadequilibrium/internal/actuator/backends"
	"github.com/loadequilibrium/loadequilibrium/internal/api"
	"github.com/loadequilibrium/loadequilibrium/internal/collector"
	"github.com/loadequilibrium/loadequilibrium/internal/config"
	"github.com/loadequilibrium/loadequilibrium/internal/persistence"
	"github.com/loadequilibrium/loadequilibrium/internal/runtime"
	"github.com/loadequilibrium/loadequilibrium/internal/scenario"
	"github.com/loadequilibrium/loadequilibrium/internal/streaming"
	"github.com/loadequilibrium/loadequilibrium/internal/telemetry"
)

func main() {
	log.SetFlags(log.Ltime | log.Lmicroseconds)
	log.Println("[loadequilibrium] starting")

	cfg := config.Load()

	store := telemetry.NewStore(cfg.RingBufferDepth, cfg.MaxServices, cfg.StaleServiceAge)
	hub := streaming.NewHub()

	var pw *persistence.Writer
	if cfg.DatabaseURL != "" {
		pw = persistence.NewWriter(cfg.DatabaseURL, 64)
		if pw != nil {
			log.Println("[persistence] connected")
		}
	} else {
		log.Println("[persistence] disabled (no DATABASE_URL)")
	}

	queueBackend := backends.NewQueueBackend()
	routerBackend := actuator.NewRouterBackend(queueBackend)

	httpEndpoint := strings.TrimSpace(os.Getenv("ACTUATOR_HTTP_ENDPOINT"))
	if httpEndpoint != "" {
		httpBackend := backends.NewHTTPBackend(httpEndpoint)
		httpServices := actuatorHTTPServices(os.Getenv("ACTUATOR_HTTP_SERVICES"))
		for _, serviceID := range httpServices {
			routerBackend.AddRoute(serviceID, httpBackend)
		}
		if len(httpServices) == 0 {
			log.Printf("[actuator] http backend configured endpoint=%s with no service routes; queue backend remains default", httpEndpoint)
		} else {
			log.Printf("[actuator] http backend configured endpoint=%s services=%s", httpEndpoint, strings.Join(httpServices, ","))
		}
	}

	act := actuator.NewCoalescingActuator(1024, routerBackend)

	scenarios := scenario.NewEngine()
	log.Println("[main] initialized scenario engine (real telemetry windows only)")

	orch := runtime.New(cfg, store, hub, pw, act, scenarios)

	srv := api.NewServer(store, hub, cfg.IngestToken)
	srv.SetOrchestrator(orch)
	srv.SetActuator(act)
	srv.SetScenarios(scenarios)

	// Wire Prometheus counters into the orchestrator so scale decisions,
	// tick counts, and SLA breaches are actually exported via /metrics.
	orch.SetCounters(srv.Counters)

	httpServer := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      srv.Handler(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 0,
		IdleTimeout:  60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go orch.Run(ctx)

	go func() {
		log.Printf("[http] listening on %s", cfg.ListenAddr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[http] fatal: %v", err)
		}
	}()

	// ── Embedded collector ────────────────────────────────────────────────────
	// Discovers Docker containers labelled le.enable=true on the Docker socket,
	// scrapes their /metrics endpoints, and POSTs to our own ingest API.
	// Runs as a goroutine inside the same process — no separate container needed.
	go startEmbeddedCollector(ctx, cfg.ListenAddr, cfg.IngestToken)

	<-ctx.Done()
	log.Println("[loadequilibrium] shutting down")

	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutCtx)
	_ = act.Close(shutCtx)
	pw.Close() // Writer.Close() is nil-safe

	log.Println("[loadequilibrium] exit")
}

// startEmbeddedCollector starts the Docker-socket-based collector inside this
// process. It overrides IngestURL to localhost so there is no network hop.
// The collector's IngestClient has built-in retry/backoff, so the brief window
// before the HTTP server is ready is handled automatically.
func startEmbeddedCollector(ctx context.Context, listenAddr, ingestToken string) {
	collCfg := collector.LoadConfig()

	// Point at ourselves — listenAddr is ":8080", so "http://127.0.0.1:8080"
	collCfg.IngestURL = "http://127.0.0.1" + listenAddr + "/api/v1/ingest"
	collCfg.IngestToken = ingestToken

	// Small grace period: let the HTTP listener bind before the first scrape
	// batch arrives. The IngestClient retries on failure so this is a soft guard.
	select {
	case <-time.After(3 * time.Second):
	case <-ctx.Done():
		return
	}

	coll, err := collector.New(collCfg)
	if err != nil {
		// Docker socket not available (e.g. running outside Docker).
		// This is not fatal — the user can still push metrics via the ingest API
		// using collector.py or any Prometheus-compatible push method.
		log.Printf("[collector] disabled (Docker socket unavailable): %v", err)
		return
	}

	log.Printf("[collector] embedded collector started — watching Docker socket for le.enable=true labels")
	coll.Run(ctx) // blocks until ctx is cancelled
}

func actuatorHTTPServices(raw string) []string {
	parts := strings.Split(raw, ",")
	services := make([]string, 0, len(parts))
	for _, part := range parts {
		serviceID := strings.TrimSpace(part)
		if serviceID == "" {
			continue
		}
		services = append(services, serviceID)
	}
	return services
}