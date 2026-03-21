package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/loadequilibrium/loadequilibrium/internal/config"
	"github.com/loadequilibrium/loadequilibrium/internal/dashboard"
	"github.com/loadequilibrium/loadequilibrium/internal/persistence"
	"github.com/loadequilibrium/loadequilibrium/internal/runtime"
	"github.com/loadequilibrium/loadequilibrium/internal/streaming"
	"github.com/loadequilibrium/loadequilibrium/internal/telemetry"
)

func main() {
	log.SetFlags(log.Ltime | log.Lmicroseconds)
	log.Println("[loadequilibrium] starting")

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

	srv := dashboard.New(store, hub)
	httpServer := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      srv.Handler(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 0,
		IdleTimeout:  60 * time.Second,
	}

	orch := runtime.New(cfg, store, hub, pw)

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
	pw.Close() // Writer.Close() is nil-safe

	log.Println("[loadequilibrium] exit")
}
