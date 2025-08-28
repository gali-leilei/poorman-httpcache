// Package pkg provides utility functions for logging.
package pkg

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/httplog/v3"
)

// GetLogger returns the configured logger instance
func GetLogger(levelStr string) *slog.Logger {
	logLevel := slog.LevelDebug
	final := strings.ToUpper(levelStr)
	switch final {
	case "DEBUG":
		logLevel = slog.LevelDebug
	case "INFO":
		logLevel = slog.LevelInfo
	case "WARN":
		logLevel = slog.LevelWarn
	case "ERROR":
		logLevel = slog.LevelError
	}

	fmt.Println("final logLevel", logLevel)
	// Set up structured logging with JSON format and proper level
	opts := &slog.HandlerOptions{
		Level: logLevel,
		// AddSource: true,
		AddSource: false,
	}

	// Use JSON handler for structured logging
	handler := slog.NewJSONHandler(os.Stdout, opts)
	logger := slog.New(handler)

	// Replace the default slog logger
	return logger
}

// GetLoggerMiddleware returns a middleware that logs the request and response
func GetLoggerMiddleware(logger *slog.Logger) func(next http.Handler) http.Handler {
	mw := httplog.RequestLogger(logger, &httplog.Options{
		// Level defines the verbosity of the request logs:
		// slog.LevelDebug - log all responses (incl. OPTIONS)
		// slog.LevelInfo  - log responses (excl. OPTIONS)
		// slog.LevelWarn  - log 4xx and 5xx responses only (except for 429)
		// slog.LevelError - log 5xx responses only
		Level: slog.LevelInfo,

		// Set log output to Elastic Common Schema (ECS) format.
		Schema: httplog.SchemaECS,

		// RecoverPanics recovers from panics occurring in the underlying HTTP handlers
		// and middlewares. It returns HTTP 500 unless response status was already set.
		//
		// NOTE: Panics are logged as errors automatically, regardless of this setting.
		RecoverPanics: true,

		// Optionally, filter out some request logs.
		Skip: func(req *http.Request, respStatus int) bool {
			return respStatus == 404 || respStatus == 405
		},

		// Optionally, log selected request/response headers explicitly.
		LogRequestHeaders:  []string{"Origin"},
		LogResponseHeaders: []string{},

		// Optionally, enable logging of request/response body based on custom conditions.
		// Useful for debugging payload issues in development.
		// LogRequestBody:  isDebugHeaderSet,
		// LogResponseBody: isDebugHeaderSet,
	})
	return mw
}
