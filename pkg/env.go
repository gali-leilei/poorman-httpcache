package pkg

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
)

type Config struct {
	// general
	Port     int    `env:"PORT" envDefault:"8080"`
	LogLevel string `env:"LOG_LEVEL" envDefault:"debug"`
	// redis
	RedisURL      string `env:"REDIS_URL" envDefault:"redis://localhost:6379"`
	RedisHost     string
	RedisPort     int
	RedisDB       int
	RedisUsername string
	RedisPassword string
	// serper
	SerperAPIKey string `env:"SERPER_API_KEY"`
	// jina
	JinaAPIKey string `env:"JINA_API_KEY"`
	// Internal use, single key only
	InternalKey string `env:"INTERNAL_KEY"`
	// Admin API key for admin endpoints
	AdminKey string `env:"ADMIN_KEY"`
	// postgres
	PostgresURL      string `env:"POSTGRES_URL" envDefault:"postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable"`
	PostgresHost     string
	PostgresPort     int
	PostgresUser     string
	PostgresPassword string
	PostgresDB       string
	// resend
	ResendAPIKey string `env:"RESEND_API_KEY"`
	EmailDomain  string `env:"EMAIL_DOMAIN"`
}

// GetConfig parses the environment variables and hydrates the Config struct.
func GetConfig() (Config, error) {
	cfg, err := env.ParseAs[Config]()
	if err != nil {
		return Config{}, err
	}

	// now hydrate the redis config
	redisURL, err := url.Parse(cfg.RedisURL)
	if err != nil {
		return cfg, fmt.Errorf("url.Parse(cfg.RedisURL): %w", err)
	}
	if redisURL.Scheme != "redis" {
		return cfg, fmt.Errorf("redisURL.Scheme: expected 'redis', got %s", redisURL.Scheme)
	}
	cfg.RedisHost = redisURL.Hostname()
	cfg.RedisPort, err = strconv.Atoi(redisURL.Port())
	if err != nil {
		return cfg, fmt.Errorf("strconv.Atoi(redisURL.Port()): %w", err)
	}
	cfg.RedisUsername = redisURL.User.Username()
	rdPwd, ok := redisURL.User.Password()
	if !ok {
		return cfg, fmt.Errorf("redisURL.User.Password() not set: %w", err)
	}
	cfg.RedisPassword = rdPwd
	cfg.RedisDB, err = strconv.Atoi(strings.TrimPrefix(redisURL.Path, "/"))
	if err != nil {
		return cfg, fmt.Errorf("strconv.Atoi(redisURL.Path): %w", err)
	}

	// now we hydrate the postgres config
	postgresURL, err := url.Parse(cfg.PostgresURL)
	if err != nil {
		return cfg, fmt.Errorf("url.Parse(cfg.PostgresURL): %w", err)
	}
	if postgresURL.Scheme != "postgresql" {
		return cfg, fmt.Errorf("postgresURL.Scheme: expected 'postgresql', got %s", postgresURL.Scheme)
	}
	cfg.PostgresHost = postgresURL.Hostname()
	cfg.PostgresPort, err = strconv.Atoi(postgresURL.Port())
	if err != nil {
		return cfg, fmt.Errorf("strconv.Atoi(postgresURL.Port()): %w", err)
	}
	cfg.PostgresUser = postgresURL.User.Username()
	pgPwd, ok := postgresURL.User.Password()
	if !ok {
		return cfg, fmt.Errorf("postgresURL.User.Password() not set: %w", err)
	}
	cfg.PostgresPassword = pgPwd
	cfg.PostgresDB = strings.TrimPrefix(redisURL.Path, "/")

	fmt.Printf("cfg: %+v\n", cfg)

	if err := testConfig(cfg); err != nil {
		return cfg, fmt.Errorf("testConfig: %w", err)
	}

	return cfg, nil
}

func testConfig(cfg Config) error {
	ctx := context.Background()
	// test postgres
	dbConn, err := pgx.Connect(ctx, cfg.PostgresURL)
	if err != nil {
		return fmt.Errorf("pgx.Connect: %w", err)
	}
	defer func() {
		if err := dbConn.Close(ctx); err != nil {
			slog.Error("dbConn.Close(ctx)", "error", err)
		}
	}()
	testCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := dbConn.Ping(testCtx); err != nil {
		slog.Error("dbConn.Ping(testCtx)", "error", err)
		return fmt.Errorf("dbConn.Ping(testCtx): %w", err)
	}

	// test redis
	rdClient := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.RedisHost, cfg.RedisPort),
		Username: cfg.RedisUsername,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
	defer func() {
		if err := rdClient.Close(); err != nil {
			slog.Error("rdClient.Close()", "error", err)
		}
	}()
	if err := rdClient.Ping(testCtx).Err(); err != nil {
		slog.Error("rdClient.Ping(testCtx)", "error", err)
		return fmt.Errorf("rdClient.Ping(testCtx): %w", err)
	}
	return nil

}
