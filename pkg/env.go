package pkg

import (
	"fmt"
	"net/url"
	"strconv"

	"github.com/caarlos0/env/v11"
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
	// postgres
	PostgresURL      string `env:"POSTGRES_URL" envDefault:"postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable"`
	PostgresHost     string
	PostgresPort     int
	PostgresUser     string
	PostgresPassword string
	PostgresDB       string
	// resend
	ResendAPIKey string `env:"RESEND_API_KEY"`
}

func GetConfig() (Config, error) {
	cfg, err := env.ParseAs[Config]()
	if err != nil {
		return Config{}, err
	}

	// now we parse the redis url
	redisURL, err := url.Parse(cfg.RedisURL)
	if err != nil {
		return cfg, fmt.Errorf("url.Parse(cfg.RedisURL): %w", err)
	}
	if redisURL.Scheme != "redis" {
		return cfg, fmt.Errorf("redisURL.Scheme: expected 'redis', got %s", redisURL.Scheme)
	}
	cfg.RedisHost = redisURL.Host
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
	cfg.RedisDB, err = strconv.Atoi(redisURL.Path)
	if err != nil {
		return cfg, fmt.Errorf("strconv.Atoi(redisURL.Path): %w", err)
	}

	// now we parse the postgres url
	postgresURL, err := url.Parse(cfg.PostgresURL)
	if err != nil {
		return cfg, fmt.Errorf("url.Parse(cfg.PostgresURL): %w", err)
	}
	if postgresURL.Scheme != "postgres" {
		return cfg, fmt.Errorf("postgresURL.Scheme: expected 'postgres', got %s", postgresURL.Scheme)
	}
	cfg.PostgresHost = postgresURL.Host
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
	cfg.PostgresDB = postgresURL.Path

	return cfg, nil
}
