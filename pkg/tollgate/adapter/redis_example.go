package adapter

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"httpcache/pkg/dbsqlc"
	"httpcache/pkg/tollgate"

	"github.com/redis/go-redis/v9"
)

// ExampleRedisQuotaSetup demonstrates how to set up the Redis quota system
func ExampleRedisQuotaSetup() {
	// Initialize Redis client with your configuration
	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // no password
		DB:       0,  // default DB
		PoolSize: 10,
	})

	// Test Redis connection
	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}

	// Initialize PostgreSQL connection and queries
	// This assumes you have your database connection set up
	var db *dbsqlc.Queries // Initialize your database connection here

	// Initialize logger
	logger := slog.New(slog.NewJSONHandler(log.Writer(), &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Create Redis-backed tollgate with Bearer token extraction (service "main")
	tollgateMain := NewRedisQuotaTollgate(rdb, db, "main", logger, func(r *http.Request) string {
		// Extract API key from Authorization header
		auth := r.Header.Get("Authorization")
		return strings.TrimPrefix(auth, "Bearer ")
	})

	// Example HTTP handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("Request processed successfully")); err != nil {
			// Log error but can't change response status at this point
			_ = err // Acknowledge the error exists
		}
	})

	// Wrap your handler with the tollgate middleware
	quotaProtectedHandler := tollgateMain.HTTPHandlerMiddleware(handler)

	// Set up HTTP server
	mux := http.NewServeMux()
	mux.Handle("/api/", quotaProtectedHandler)

	server := &http.Server{
		Addr:         ":8080",
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	// Start server
	log.Printf("Starting server on :8080 with Redis quota management")
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server failed to start: %v", err)
	}
}

// ExampleCustomKeyExtractor shows how to use custom key extraction
func ExampleCustomKeyExtractor() {
	var rdb RedisClient     // Your Redis client
	var db *dbsqlc.Queries  // Your database queries
	var logger *slog.Logger // Your logger

	// Example with X-API-Key header (service "api")
	tollgateAPIKey := NewRedisQuotaTollgate(rdb, db, "api", logger, func(r *http.Request) string {
		return r.Header.Get("X-API-Key")
	})

	// Example with query parameter (service "web")
	tollgateQueryParam := NewRedisQuotaTollgate(rdb, db, "web", logger, func(r *http.Request) string {
		return r.URL.Query().Get("api_key")
	})

	// Example with custom header (service "mobile")
	tollgateCustom := NewRedisQuotaTollgate(rdb, db, "mobile", logger, func(r *http.Request) string {
		return r.Header.Get("X-Custom-Token")
	})

	// Use any of these tollgates as middleware
	_ = tollgateAPIKey
	_ = tollgateQueryParam
	_ = tollgateCustom
}

// ExampleGracefulShutdown shows how to properly shutdown the Redis adapter
// func ExampleGracefulShutdown() {
// 	var rdb RedisClient     // Your Redis client
// 	var db *dbsqlc.Queries  // Your database queries
// 	var logger *slog.Logger // Your logger

// 	// Create adapter directly for shutdown control (service "main")
// 	adapter := NewKeyValue(rdb, db, "main", logger)

// 	// ... use adapter ...

// 	// Graceful shutdown with timeout
// 	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
// 	defer cancel()

// 	// if err := adapter.Shutdown(ctx); err != nil {
// 	// 	logger.Error("Failed to shutdown Redis adapter gracefully", "error", err)
// 	// } else {
// 	// 	logger.Info("Redis adapter shutdown completed successfully")
// 	// }
// }

// ExampleMultiService shows how to set up multiple services
func ExampleMultiService() {
	var rdb RedisClient     // Your Redis client
	var db *dbsqlc.Queries  // Your database queries
	var logger *slog.Logger // Your logger

	// Create separate adapters for different services
	mainAdapter := NewKeyValue(rdb, db, "main", logger)
	premiumAdapter := NewKeyValue(rdb, db, "premium", logger)
	apiAdapter := NewKeyValue(rdb, db, "api", logger)

	// Each adapter manages quotas independently for their service
	keyExtractor := func(r *http.Request) string {
		return r.Header.Get("Authorization")
	}

	mainTollgate := tollgate.New(mainAdapter, keyExtractor)
	premiumTollgate := tollgate.New(premiumAdapter, keyExtractor)
	apiTollgate := tollgate.New(apiAdapter, keyExtractor)

	// Use different tollgates for different endpoints
	_ = mainTollgate
	_ = premiumTollgate
	_ = apiTollgate
}
