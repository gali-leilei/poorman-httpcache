// package main is the main package for the admin server
package main

import (
	"context"
	"fmt"
	"httpcache/pkg"
	"httpcache/pkg/api"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

func run(ctx context.Context, cfg pkg.Config, logger *slog.Logger) error {
	// Create a single HTTP server with path-based routing
	mux := chi.NewRouter()

	// A good base middleware stack
	mux.Use(middleware.RequestID)
	mux.Use(middleware.RealIP)
	mux.Use(pkg.GetLoggerMiddleware(logger))
	mux.Use(middleware.Recoverer)

	db, err := pgx.Connect(ctx, cfg.PostgresURL)
	if err != nil {
		return fmt.Errorf("pgx.Connect: %w", err)
	}

	apiServer := api.NewServer(db, logger, cfg.AdminKey)
	adminHandler := api.HandlerWithOptions(apiServer, api.ChiServerOptions{BaseURL: ""})
	mux.Handle("/*", adminHandler)

	// Redirect /docs to /docs/ for proper relative path resolution
	mux.HandleFunc("GET /docs", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, api.SwaggerAsset, "index.html")
	})

	// Route /docs/ to serve index.html
	// Note: strip `/docs` not `/docs/`.
	mux.Handle("GET /docs/*", http.StripPrefix("/docs", http.FileServer(http.FS(api.SwaggerAsset))))

	// Single server listening on port 8080
	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: mux,
	}

	// Start the single server
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Server failed", "error", err)
			return
		}
	}()

	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Wait for shutdown signal
	<-ctx.Done()
	logger.Info("Received shutdown signal, shutting down server...")

	// Create a context with a timeout for graceful shutdown
	shutdownCtx := context.Background()
	shutdownCtx, shutdownCancel := context.WithTimeout(shutdownCtx, 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("Error shutting down server", "error", err)
		return err
	}
	return nil
}

func main() {
	// parse with generics
	cfg, err := pkg.GetConfig()
	if err != nil {
		// Can't use logger here since it hasn't been created yet
		slog.Error("Failed to parse config.", "error", err)
		os.Exit(1)
	}
	logger := pkg.GetLogger(cfg.LogLevel)
	logger.Info("Config", "cfg", cfg)
	ctx := context.Background()

	if err := run(ctx, cfg, logger); err != nil {
		logger.Error("Failed to run", "error", err)
		os.Exit(1)
	}
}
