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
	"github.com/loadequilibrium/loadequilibrium/internal/config"
	"github.com/loadequilibrium/loadequilibrium/internal/dashboard"
	"github.com/loadequilibrium/loadequilibrium/internal/persistence"
	"github.com/loadequilibrium/loadequilibrium/internal/runtime"
	"github.com/loadequilibrium/loadequilibrium/internal/scenario"
	"github.com/loadequilibrium/loadequilibrium/internal/streaming"
	"github.com/loadequilibrium/loadequilibrium/internal/telemetry"
)

func main() {
	log.SetFlags(log.Ltime | log.Lmicroseconds)
	log.Println("[loadequilibrium] starting - VER_2.2_SYNC_CHECK")
	time.Sleep(3 * time.Second)

	cfg := config.Load()

	store := telemetry.NewStore(cfg.RingBufferDepth, cfg.MaxServices, cfg.StaleServiceAge)
	hub := streaming.NewHub()
	// MaxStreamClients is wired inside runtime.New via hub.SetMaxClients — no action needed here.

	var pw *persistence.Writer
	if cfg.DatabaseURL != "" {
		pw = persistence.NewWriter(cfg.DatabaseURL, 64)
		if pw != nil {
			log.Println("[persistence] connected")
		}
	} else {
		log.Println("[persistence] disabled (no DATABASE_URL)")
	}

	srv := dashboard.New(store, hub, cfg.IngestToken)
	httpServer := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      srv.Handler(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 0,
		IdleTimeout:  60 * time.Second,
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

	scenarios := scenario.NewEngine() // empty by default; operators add via config or plugin

	orch := runtime.New(cfg, store, hub, pw, act, scenarios)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go orch.Run(ctx)

	go func() {
		log.Printf("[http] listening on %s", cfg.ListenAddr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[http] fatal: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("[loadequilibrium] shutting down")

	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutCtx)
	_ = act.Close(shutCtx)
	pw.Close() // Writer.Close() is nil-safe

	log.Println("[loadequilibrium] exit")
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
