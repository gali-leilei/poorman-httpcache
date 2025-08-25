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

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	// general
	Port     int    `env:"PORT" envDefault:"8080"`
	LogLevel string `env:"LOG_LEVEL" envDefault:"debug"`
	// redis
	// RedisServer   string `env:"REDIS_SERVER" envDefault:"localhost:6379"`
	// RedisUsername string `env:"REDIS_USERNAME" envDefault:""`
	// RedisPassword string `env:"REDIS_PASSWORD" envDefault:""`
	// serper
	// SerperAPIKey string `env:"SERPER_API_KEY"`
	// jina
	// JinaAPIKey string `env:"JINA_API_KEY"`
	// admin
	AdminKey string `env:"ADMIN_KEY"`
	// Postgres
	PostgresURL string `env:"POSTGRES_URL"`
}

func run(ctx context.Context, cfg Config, logger *slog.Logger) error {
	// Create a single HTTP server with path-based routing
	mux := chi.NewRouter()

	// TODO: fix this

	db, err := pgx.Connect(ctx, cfg.PostgresURL)
	if err != nil {
		return fmt.Errorf("pgx.Connect: %w", err)
	}

	adminServer, err := api.NewServer(db, logger)
	if err != nil {
		return fmt.Errorf("NewServer: %w", err)
	}

	mux.Handle("/admin", api.Handler(adminServer))

	// Route /docs to serve index.html directly
	mux.HandleFunc("/docs", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, api.SwaggerUI, "index.html")
	})

	// Route /docs/* requests to api.SwaggerUI for other files
	mux.Handle("/docs/", http.StripPrefix("/docs/", http.FileServer(http.FS(api.SwaggerUI))))

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
	cfg, err := env.ParseAs[Config]()
	if err != nil {
		// Can't use logger here since it hasn't been created yet
		slog.Error("Failed to parse config", "error", err)
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
