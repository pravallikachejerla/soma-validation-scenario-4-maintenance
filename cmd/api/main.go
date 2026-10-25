// Command api runs the HTTP API server.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/somagen/scenario4/internal/application"
	"github.com/somagen/scenario4/internal/httpapi"
	"github.com/somagen/scenario4/internal/observability"
	"github.com/somagen/scenario4/internal/storage"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://pricing:pricing@localhost:5432/pricing?sslmode=disable"
	}
	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	commit := os.Getenv("BUILD_COMMIT")
	if commit == "" {
		commit = "dev"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var store storage.Store
	if os.Getenv("STORAGE_BACKEND") == "memory" {
		store = storage.NewMemoryStore()
	} else {
		ps, err := storage.NewPostgresStore(ctx, dsn)
		if err != nil {
			log.Printf("postgres init failed, falling back to memory: %v", err)
			store = storage.NewMemoryStore()
		} else {
			store = ps
		}
	}
	defer store.Close(context.Background())

	logger := observability.Default("api")
	metrics := observability.NewMetrics()
	ps := application.NewPricingService(store, logger, metrics)
	as := application.NewAdminService(store, 42)

	srv := &httpapi.Server{
		Pricing: ps,
		Admin:   as,
		Store:   store,
		Logger:  logger,
		Metrics: metrics,
		Build: httpapi.BuildInfo{
			Commit:  commit,
			BuiltAt: time.Now().UTC().Format(time.RFC3339),
		},
	}
	mux := httpapi.NewServer(srv)

	hs := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      15 * time.Second,
	}

	// Graceful shutdown.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-stop
		logger.Info(context.Background(), "api.shutdown", nil)
		shutdownCtx, cancelShut := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancelShut()
		_ = hs.Shutdown(shutdownCtx)
	}()

	logger.Info(context.Background(), "api.listening", map[string]any{"addr": addr})
	if err := hs.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("listen: %v", err)
	}
}
