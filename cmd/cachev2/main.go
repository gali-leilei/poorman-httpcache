package main

import (
	"context"
	"fmt"
	"httpcache/pkg"
	"httpcache/pkg/dbsqlc"
	"httpcache/pkg/proxy"
	"httpcache/pkg/tollgate"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/middleware"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
)

// warmupCache warms up the cache by loading table `quota` from postgres into redis.
func warmupCache(ctx context.Context, cfg pkg.Config, logger *slog.Logger) error {
	redis := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.RedisHost, cfg.RedisPort),
		Username: cfg.RedisUsername,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
	if err := redis.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("failed to ping redis: %w", err)
	}

	defer redis.Close()

	db, err := pgx.Connect(ctx, cfg.PostgresURL)
	if err != nil {
		return fmt.Errorf("failed to connect to postgres: %w", err)
	}
	if err := db.Ping(ctx); err != nil {
		return fmt.Errorf("failed to ping postgres: %w", err)
	}
	defer db.Close(ctx)

	queries := dbsqlc.New(db)
	cw := pkg.NewCacheWarmer(redis, queries, logger, 1000)
	cw.Do(ctx)
	return nil
}

func NewNameServer(cfg pkg.Config, logger *slog.Logger) *pkg.NameServer {
	db, err := pgx.Connect(context.Background(), cfg.PostgresURL)
	if err != nil {
		logger.Error("Failed to connect to Postgres", "error", err)
		panic(err)
	}
	ns := pkg.NewNameServer(redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.RedisHost, cfg.RedisPort),
		Username: cfg.RedisUsername,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB + 3,
	}), db, logger)
	return ns
}

func NewJinaProxy(cfg pkg.Config, logger *slog.Logger) (http.Handler, error) {
	target, err := url.Parse("https://r.jina.ai")
	if err != nil {
		logger.Error("Failed to parse Jina target URL", "error", err)
		return nil, err
	}

	rp, err := proxy.New(
		proxy.WithRewrites(
			proxy.RewriteJinaPath(target),
			proxy.ReplaceJinaKey(cfg.JinaAPIKey),
			proxy.DebugRequest(logger),
		),
	)
	if err != nil {
		logger.Error("Failed to create Jina proxy", "error", err)
		return nil, err
	}

	nameServer := NewNameServer(cfg, logger)
	rdsAdapter := tollgate.NewRedisAdapter(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.RedisHost, cfg.RedisPort),
		Username: cfg.RedisUsername,
		Password: cfg.RedisPassword,
		// DB:       cfg.RedisDB + 1,
		DB: cfg.RedisDB,
	}, "jina", nameServer, logger)
	secretKeyExtract := func(r *http.Request) string {
		return strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	}
	tollgate := tollgate.New(rdsAdapter, secretKeyExtract, logger)

	return tollgate.HTTPHandlerMiddleware(rp), nil
}

func NewSerperProxy(cfg pkg.Config, logger *slog.Logger) (http.Handler, error) {
	target, err := url.Parse("https://google.serper.dev")
	if err != nil {
		logger.Error("Failed to parse Serper target URL", "error", err)
		return nil, err
	}

	rp, err := proxy.New(
		proxy.WithRewrites(
			proxy.RewriteSerperPath(target),
			proxy.ReplaceSerperKey(cfg.SerperAPIKey),
			proxy.DebugRequest(logger),
		),
	)
	if err != nil {
		logger.Error("Failed to create Serper proxy", "error", err)
		return nil, err
	}
	nameServer := NewNameServer(cfg, logger)
	rdsAdapter := tollgate.NewRedisAdapter(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.RedisHost, cfg.RedisPort),
		Username: cfg.RedisUsername,
		Password: cfg.RedisPassword,
		// DB:       cfg.RedisDB + 2,
		DB: cfg.RedisDB,
	}, "serper", nameServer, logger)
	secretKeyExtract := func(r *http.Request) string {
		return r.Header.Get("X-API-KEY")
	}
	tollgate := tollgate.New(rdsAdapter, secretKeyExtract, logger)

	return tollgate.HTTPHandlerMiddleware(rp), nil
}

func run(ctx context.Context, cfg pkg.Config, logger *slog.Logger) error {
	jinaProxy, err := NewJinaProxy(cfg, logger)
	if err != nil {
		return fmt.Errorf("NewJinaProxy: %w", err)
	}
	serperProxy, err := NewSerperProxy(cfg, logger)
	if err != nil {
		return fmt.Errorf("NewSerperProxy: %w", err)
	}

	// Create a single HTTP server with path-based routing
	mux := http.NewServeMux()

	mux.HandleFunc("/jina/", func(w http.ResponseWriter, r *http.Request) {
		jinaProxy.ServeHTTP(w, r)
	})
	mux.HandleFunc("/serper/", func(w http.ResponseWriter, r *http.Request) {
		serperProxy.ServeHTTP(w, r)
	})

	var h http.Handler = mux
	h = pkg.GetLoggerMiddleware(logger)(h)
	h = middleware.Recoverer(h)

	// Single server listening on port 8080
	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: h,
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
		slog.Error("Failed to parse config", "error", err)
		os.Exit(1)
	}
	logger := pkg.GetLogger(cfg.LogLevel)
	logger.Info("Config", "cfg", cfg)
	ctx := context.Background()

	if err := warmupCache(ctx, cfg, logger); err != nil {
		logger.Error("Failed to warm up redis cache", "error", err)
		os.Exit(1)
	}

	if err := run(ctx, cfg, logger); err != nil {
		logger.Error("Failed to run", "error", err)
		os.Exit(1)
	}
}
