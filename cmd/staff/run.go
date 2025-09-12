package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"httpcache/pkg"
	"httpcache/pkg/staff"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/httprate"
	httprateredis "github.com/go-chi/httprate-redis"
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

	idToKey, err := staff.NewIDToKey(db)
	if err != nil {
		return fmt.Errorf("staff.NewIDToKey(): %w", err)
	}

	sendInstruction, err := staff.NewSendInstruction(cfg.ResendAPIKey, cfg.EmailDomain, cfg.ServiceDomain)
	if err != nil {
		return fmt.Errorf("staff.NewSendInstruction(): %w", err)
	}

	sendMail, err := staff.NewSendMail(cfg.ResendAPIKey, cfg.EmailDomain, cfg.AuthDomain)
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
	// configure cookie based on auth domain
	authDomain, err := url.Parse(cfg.AuthDomain)
	if err != nil {
		return fmt.Errorf("url.Parse(cfg.AuthDomain): %w", err)
	}
	sm.Cookie.Domain = authDomain.Hostname()
	sm.Cookie.Path = "/"
	if authDomain.Scheme == "https" {
		sm.Cookie.SameSite = http.SameSiteStrictMode
		sm.Cookie.Secure = true
	} else {
		sm.Cookie.SameSite = http.SameSiteLaxMode
		sm.Cookie.Secure = false
	}

	router := staff.NewRouter(sm, allowList, idToKey, sendMail, sendInstruction, form, logger)

	// rate limter
	rateLimter := httprate.Limit(
		20,
		time.Minute,
		httprate.WithKeyByIP(),
		httprateredis.WithRedisLimitCounter(&httprateredis.Config{
			Host:    cfg.RedisHost,
			Port:    uint16(cfg.RedisPort),
			DBIndex: 5,
		}),
	)

	// Single server listening on port 8080
	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: rateLimter(router),
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
