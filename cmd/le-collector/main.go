package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/loadequilibrium/loadequilibrium/internal/collector"
)

func main() {
	log.SetFlags(log.Ltime | log.Lmicroseconds)

	cfg := collector.LoadConfig()
	c, err := collector.New(cfg)
	if err != nil {
		log.Fatalf(`{"component":"le-collector","level":"fatal","event":"init_failed","error":%q}`, err.Error())
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go c.Run(ctx)

	server := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      collector.NewHTTPHandler(c),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf(`{"component":"le-collector","level":"info","event":"listening","addr":%q}`, cfg.ListenAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf(`{"component":"le-collector","level":"fatal","event":"http_failed","error":%q}`, err.Error())
		}
	}()

	<-ctx.Done()
	log.Printf(`{"component":"le-collector","level":"info","event":"shutdown"}`)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)
}
