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

// Archiver defines the methods needed for usage tracking
type Archiver interface {
	Archive(ctx context.Context) error
}

// KeyValue implements the tollgate.Adapter interface using KeyValue with
// direct aggregation for PostgreSQL synchronization
type KeyValue struct {
	metaStore    MetaStore
	quotaManager *QuotaManager
	usageTracker Archiver
	logger       *slog.Logger
	cancel       context.CancelFunc
}

// NewKeyValueWithDependencies creates a new KeyValue with injected dependencies for testing
func NewKeyValueWithDependencies(
	metaStore MetaStore,
	quotaManager *QuotaManager,
	usageTracker Archiver,
	logger *slog.Logger,
	cancel context.CancelFunc,
) *KeyValue {
	return &KeyValue{
		metaStore:    metaStore,
		quotaManager: quotaManager,
		usageTracker: usageTracker,
		logger:       logger,
		cancel:       cancel,
	}
}

// NewKeyValue creates a new Redis adapter for a specific service with direct aggregation
func NewKeyValue(rdb RedisClient, db *dbsqlc.Queries, serviceName string, logger *slog.Logger) *KeyValue {
	keyStore := NewRedisMetadataStore(rdb, db)

	// Create context for background processes
	ctx, cancel := context.WithCancel(context.Background())

	// Create quota manager with service metadata
	quotaManager, err := NewQuotaManager(ctx, rdb, keyStore, serviceName)
	if err != nil {
		logger.Error("Failed to create quota manager", "error", err)
		panic(err) // or handle error appropriately
	}

	usageTracker := NewUsageTracker(ctx, rdb, db, logger)

	return NewKeyValueWithDependencies(keyStore, quotaManager, usageTracker, logger, cancel)
}

// Reserve reserves a given amount of quota for a key.
// Returns true if the reservation was successful, false if the quota is insufficient.
func (r *KeyValue) Reserve(ctx context.Context, key string, amount int) (bool, error) {
	// Get cached key keyMeta
	keyMeta, err := r.metaStore.GetKey(ctx, key)
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
func (r *KeyValue) Refund(ctx context.Context, key string, amount int) (bool, error) {
	// Get cached key keyMeta
	keyMeta, err := r.metaStore.GetKey(ctx, key)
	if err != nil {
		return false, fmt.Errorf("r.keyStore.GetKey: %w", err)
	}

	ok, err := r.quotaManager.Refund(ctx, keyMeta, amount)
	if err != nil {
		return false, fmt.Errorf("r.quotaManager.Refund: %w", err)
	}
	return ok, nil
}

// NewRedisQuotaTollgate creates a new Tollgate using Redis for high-performance quota management
func NewRedisQuotaTollgate(rdb RedisClient, db *dbsqlc.Queries, serviceID string, logger *slog.Logger, keyExtractor func(r *http.Request) string) *tollgate.Tollgate {
	adapter := NewKeyValue(rdb, db, serviceID, logger)
	return tollgate.New(adapter, keyExtractor)
}
