package adapter

import (
	"context"
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
	cache        *KeyMetadataStore
	quotaManager *QuotaManager
	usageTracker *UsageTracker
	serviceID    string
	logger       *slog.Logger
	cancel       context.CancelFunc
}

// NewRedis creates a new Redis adapter for a specific service with direct aggregation
func NewRedis(rdb RedisClient, db *dbsqlc.Queries, serviceID string, logger *slog.Logger) *Redis {
	cache := NewKeyMetadataStore(rdb, db)
	quotaManager := NewQuotaManager(rdb, db, serviceID)

	// Create context for background processes
	ctx, cancel := context.WithCancel(context.Background())
	usageTracker := NewUsageTracker(ctx, rdb, db, logger)

	adapter := &Redis{
		cache:        cache,
		quotaManager: quotaManager,
		usageTracker: usageTracker,
		serviceID:    serviceID,
		logger:       logger,
		cancel:       cancel,
	}

	logger.Info("Redis adapter initialized with direct aggregation", "service_id", serviceID)
	return adapter
}

// Consume implements quota consumption with direct aggregation
func (r *Redis) Consume(ctx context.Context, key string) (int, error) {
	// Get cached key metadata
	metadata, err := r.cache.Get(ctx, key)
	if err != nil {
		return 0, err
	}

	return r.quotaManager.ConsumeQuota(ctx, key, metadata)
}

// Balance implements the Adapter interface - returns current quota balance
func (r *Redis) Balance(ctx context.Context, key string) (int, error) {
	// Get cached key metadata
	metadata, err := r.cache.Get(ctx, key)
	if err != nil {
		return 0, err
	}

	return r.quotaManager.GetBalance(ctx, key, metadata)
}

// ServiceID returns the service ID this adapter is configured for
func (r *Redis) ServiceID() string {
	return r.serviceID
}

// BatchUpdateQuotas provides batch quota operations for administrative tasks
func (r *Redis) BatchUpdateQuotas(ctx context.Context, updates map[string]int) error {
	return r.quotaManager.BatchUpdateQuotas(ctx, updates)
}

// InvalidateKeyCache removes cached metadata for a key
func (r *Redis) InvalidateKeyCache(ctx context.Context, key string) error {
	return r.cache.Reset(ctx, key)
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
