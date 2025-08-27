package adapter

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"httpcache/pkg/dbsqlc"
	"httpcache/pkg/tollgate"

	"github.com/redis/go-redis/v9"
)

// RedisClient interface defines the methods we need from Redis client
type RedisClient interface {
	redis.Cmdable
}

// Redis implements the tollgate.Adapter interface using Redis with
// direct aggregation for PostgreSQL synchronization
type Redis struct {
	keyStore     *MetaStore
	quotaManager *QuotaManager
	usageTracker *UsageTracker
	serviceID    string
	logger       *slog.Logger
	cancel       context.CancelFunc
}

// NewRedis creates a new Redis adapter for a specific service with direct aggregation
func NewRedis(rdb RedisClient, db *dbsqlc.Queries, serviceID string, logger *slog.Logger) *Redis {
	keyStore := NewKeyMetadataStore(rdb, db)

	// Create context for background processes
	ctx, cancel := context.WithCancel(context.Background())

	// Create quota manager with service metadata
	quotaManager, err := NewQuotaManager(ctx, rdb, db, keyStore, serviceID)
	if err != nil {
		logger.Error("Failed to create quota manager", "error", err)
		panic(err) // or handle error appropriately
	}

	usageTracker := NewUsageTracker(ctx, rdb, db, logger)

	adapter := &Redis{
		keyStore:     keyStore,
		quotaManager: quotaManager,
		usageTracker: usageTracker,
		serviceID:    serviceID,
		logger:       logger,
		cancel:       cancel,
	}

	logger.Info("Redis adapter initialized with direct aggregation", "service_id", serviceID)
	return adapter
}

// Reserve reserves a given amount of quota for a key.
// Returns true if the reservation was successful, false if the quota is insufficient.
func (r *Redis) Reserve(ctx context.Context, key string, amount int) (bool, error) {
	// Get cached key keyMeta
	keyMeta, err := r.keyStore.GetKey(ctx, key)
	if err != nil {
		return false, fmt.Errorf("r.keyStore.GetKey: %w", err)
	}

	ok, err := r.quotaManager.Reserve(ctx, keyMeta, amount)
	if err != nil {
		return false, fmt.Errorf("r.quotaManager.Reserve: %w", err)
	}
	return ok, nil
}

// Refund refunds a given amount of quota for a key.
// Returns true if the refund was successful, false if the quota is insufficient.
func (r *Redis) Refund(ctx context.Context, key string, amount int) (bool, error) {
	// Get cached key keyMeta
	keyMeta, err := r.keyStore.GetKey(ctx, key)
	if err != nil {
		return false, fmt.Errorf("r.keyStore.GetKey: %w", err)
	}

	ok, err := r.quotaManager.Refund(ctx, keyMeta, amount)
	if err != nil {
		return false, fmt.Errorf("r.quotaManager.Refund: %w", err)
	}
	return ok, nil
}

// Shutdown gracefully shuts down the Redis adapter
func (r *Redis) Shutdown(ctx context.Context) error {
	r.logger.Info("Starting Redis adapter shutdown")

	// Cancel the background context to stop the usage tracker
	r.cancel()

	// Wait for the usage tracker to shutdown
	return r.usageTracker.Shutdown(ctx)
}

// NewRedisQuotaTollgate creates a new Tollgate using Redis for high-performance quota management
func NewRedisQuotaTollgate(rdb RedisClient, db *dbsqlc.Queries, serviceID string, logger *slog.Logger, keyExtractor func(r *http.Request) string) *tollgate.Tollgate {
	adapter := NewRedis(rdb, db, serviceID, logger)
	return tollgate.New(adapter, keyExtractor)
}
