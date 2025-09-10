package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"httpcache/pkg"
	"httpcache/pkg/staff"

	"github.com/alexedwards/scs/v2"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
)

func run(ctx context.Context, cfg pkg.Config, logger *slog.Logger) error {
	// Connect to database
	db, err := pgx.Connect(ctx, cfg.PostgresURL)
	if err != nil {
		return fmt.Errorf("pgx.Connect: %w", err)
	}
	defer func(db *pgx.Conn, ctx context.Context) {
		err := db.Close(ctx)
		if err != nil {
			logger.Error("Failed to close database connection", "error", err)
		}
	}(db, ctx)

	form, err := staff.NewForm()
	if err != nil {
		return fmt.Errorf("staff.NewForm(): %w", err)
	}

	allowList, err := staff.NewAllowlist(db)
	if err != nil {
		return fmt.Errorf("staff.NewAllowlist(): %w", err)
	}

	sendMail, err := staff.NewSendMail(cfg.ResendAPIKey, cfg.EmailDomain, cfg.HostDomain)
	if err != nil {
		return fmt.Errorf("staff.NewSendMail(): %w", err)
	}

	sm := scs.New()
	store := staff.NewRedisStore(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.RedisHost, cfg.RedisPort),
		Username: cfg.RedisUsername,
		Password: cfg.RedisPassword,
		DB:       10,
	})
	if store == nil {
		return fmt.Errorf("staff.NewRedisStore(): %w", err)
	}
	sm.Store = store
	sm.Lifetime = 24 * time.Hour
	sm.Cookie.Name = "miromind-staff-session"
	// set to true for production
	// sm.Cookie.Secure = true

	router := staff.NewRouter(sm, allowList, sendMail, form, logger)

	// Single server listening on port 8080
	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: router,
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
