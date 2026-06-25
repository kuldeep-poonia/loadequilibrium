package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/loadequilibrium/loadequilibrium/internal/actuator"
	"github.com/loadequilibrium/loadequilibrium/internal/actuator/backends"
	"github.com/loadequilibrium/loadequilibrium/internal/api"
	"github.com/loadequilibrium/loadequilibrium/internal/collector"
	"github.com/loadequilibrium/loadequilibrium/internal/config"
	"github.com/loadequilibrium/loadequilibrium/internal/persistence"
	rtime "github.com/loadequilibrium/loadequilibrium/internal/runtime"
	"github.com/loadequilibrium/loadequilibrium/internal/scenario"
	"github.com/loadequilibrium/loadequilibrium/internal/security"
	"github.com/loadequilibrium/loadequilibrium/internal/streaming"
	"github.com/loadequilibrium/loadequilibrium/internal/telemetry"
)

func main() {
	log.SetFlags(log.Ltime | log.Lmicroseconds)
	procs := runtime.GOMAXPROCS(0)
	log.Printf("[loadequilibrium] starting  procs=%d  go=%s", procs, runtime.Version())

	cfg := config.Load()

	// ── Token startup validation ──────────────────────────────────────────────
	// Warn loudly if default/weak tokens are in use. These logs appear in
	// docker logs / k8s logs and are visible to operators on first run.
	security.WarnWeakToken(cfg.IngestToken)
	if cfg.ControlToken != "" {
		security.WarnWeakToken(cfg.ControlToken)
	}

	store := telemetry.NewStore(cfg.RingBufferDepth, cfg.MaxServices, cfg.StaleServiceAge)
	hub := streaming.NewHub()
	hub.SetMaxClients(cfg.MaxStreamClients)

	// ── Persistence ───────────────────────────────────────────────────────────
	var pw *persistence.Writer
	if cfg.DatabaseURL != "" {
		pw = persistence.NewWriter(cfg.DatabaseURL, 64)
		if pw != nil {
			log.Println("[persistence] connected")
		}
	} else {
		log.Println("[persistence] disabled (DATABASE_URL not set)")
	}

	// ── Actuator ─────────────────────────────────────────────────────────────
	queueBackend := backends.NewQueueBackend()
	routerBackend := actuator.NewRouterBackend(queueBackend)

	if httpEndpoint := strings.TrimSpace(os.Getenv("ACTUATOR_HTTP_ENDPOINT")); httpEndpoint != "" {
		httpBackend := backends.NewHTTPBackend(httpEndpoint)
		for _, svc := range actuatorHTTPServices(os.Getenv("ACTUATOR_HTTP_SERVICES")) {
			routerBackend.AddRoute(svc, httpBackend)
		}
		log.Printf("[actuator] http backend endpoint=%s", httpEndpoint)
	}
	act := actuator.NewCoalescingActuator(1024, routerBackend)

	scenarios := scenario.NewEngine()
	orch := rtime.New(cfg, store, hub, pw, act, scenarios)

	// ── API server with full security stack ───────────────────────────────────
	srv := api.NewServerWithConfig(store, hub, api.ServerConfig{
		IngestToken:    cfg.IngestToken,
		ControlToken:   cfg.ControlToken,   // separate token for control plane
		DashboardToken: cfg.DashboardToken, // separate token for dashboard/WS
		AllowedOrigins: cfg.AllowedOrigins,
		TLSEnabled:     cfg.TLSEnabled,
		IngestRPS:      cfg.IngestRPS,
		ControlRPS:     cfg.ControlRPS,
	})
	srv.SetOrchestrator(orch)
	srv.SetActuator(act)
	srv.SetScenarios(scenarios)
	orch.SetCounters(srv.Counters)

	// ── HTTP server ───────────────────────────────────────────────────────────
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", cfg.ListenAddr)
	if err != nil {
		log.Fatalf("[http] bind %s: %v", cfg.ListenAddr, err)
	}
	httpServer := &http.Server{
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      0, // 0 = no timeout (required for WebSocket)
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    64 * 1024,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go orch.Run(ctx)
	go func() {
		log.Printf("[http] listening on %s  tls=%v", cfg.ListenAddr, cfg.TLSEnabled)
		if err := httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[http] fatal: %v", err)
		}
	}()

	go startEmbeddedCollector(ctx, cfg.ListenAddr, cfg.IngestToken)

	<-ctx.Done()
	log.Println("[loadequilibrium] shutting down")
	shutCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutCtx)
	_ = act.Close(shutCtx)
	pw.Close()
	log.Println("[loadequilibrium] exit")
}

func startEmbeddedCollector(ctx context.Context, listenAddr, ingestToken string) {
	collCfg := collector.LoadConfig()
	collCfg.IngestURL = "http://127.0.0.1" + listenAddr + "/api/v1/ingest"
	collCfg.IngestToken = ingestToken

	select {
	case <-time.After(2 * time.Second):
	case <-ctx.Done():
		return
	}

	coll, err := collector.New(collCfg)
	if err != nil {
		log.Printf("[collector] disabled: %v", err)
		return
	}
	log.Printf("[collector] started, label=%s", collCfg.DiscoveryLabel)
	coll.Run(ctx)
}

func actuatorHTTPServices(raw string) []string {
	var out []string
	for _, p := range strings.Split(raw, ",") {
		if s := strings.TrimSpace(p); s != "" {
			out = append(out, s)
		}
	}
	return out
}
