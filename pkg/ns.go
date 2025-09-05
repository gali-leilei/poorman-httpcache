package pkg

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"httpcache/pkg/dbsqlc"

	"github.com/go-redis/cache/v9"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
	"github.com/vmihailenco/msgpack/v5"
)

// NameServer is a name server that resolves
// 1. service name to internal service ID
// 2. api key to internal api key ID
type NameServer struct {
	cache   *cache.Cache
	queries *dbsqlc.Queries
	logger  *slog.Logger
}

// NewNameServer creates a new NameServer instance
func NewNameServer(redisClient *redis.Client, db *pgx.Conn, logger *slog.Logger) *NameServer {
	queries := dbsqlc.New(db)
	cacheInstance := cache.New(&cache.Options{
		Redis: redisClient,
		Marshal: func(v any) ([]byte, error) {
			return msgpack.Marshal(v)
		},
		Unmarshal: func(b []byte, v any) error {
			return msgpack.Unmarshal(b, v)
		},
		LocalCache: cache.NewTinyLFU(1000, 10*time.Minute),
	})
	return &NameServer{
		cache:   cacheInstance,
		queries: queries,
		logger:  logger,
	}
}

// ResolveServiceName resolves a service name (string) to an internal service ID (int)
func (ns *NameServer) ResolveServiceName(serviceName string) (int, error) {
	key := fmt.Sprintf("service:%s", serviceName)

	// 1. First check cache (automatically checks both local TinyLFU and Redis)
	var serviceID int
	err := ns.cache.Get(context.Background(), key, &serviceID)
	if err == nil {
		ns.logger.Debug("service name resolved from cache", "service", serviceName, "id", serviceID)
		return serviceID, nil
	}

	// 2. Cache miss - check postgres
	service, err := ns.queries.GetServiceByName(context.Background(), serviceName)
	if err != nil {
		return 0, fmt.Errorf("GetServiceByName: %w", err)
	}
	serviceID = int(service.ID)

	// Cache the result (automatically sets in both local TinyLFU and Redis)
	err = ns.cache.Set(&cache.Item{
		Key:   key,
		Value: serviceID,
		TTL:   30 * time.Minute,
	})
	if err != nil {
		ns.logger.Warn("failed to cache service name", "service", serviceName, "error", err)
	}

	ns.logger.Debug("service name resolved from postgres", "service", serviceName, "id", serviceID)
	return serviceID, nil
}

// ResolveAPIKey resolves an API key (string) to an internal API key ID (int)
func (ns *NameServer) ResolveAPIKey(apiKey string) (int, error) {
	key := fmt.Sprintf("api_key:%s", apiKey)

	// 1. First check cache (automatically checks both local TinyLFU and Redis)
	var apiKeyID int
	err := ns.cache.Get(context.Background(), key, &apiKeyID)
	if err == nil {
		ns.logger.Debug("api key resolved from cache", "key", apiKey, "id", apiKeyID)
		return apiKeyID, nil
	}

	// 2. Cache miss - check postgres
	apiKeyData, err := ns.queries.GetAPIKeyByName(context.Background(), apiKey)
	if err != nil {
		return 0, err
	}
	apiKeyID = int(apiKeyData.ID)

	// Cache the result (automatically sets in both local TinyLFU and Redis)
	err = ns.cache.Set(&cache.Item{
		Key:   key,
		Value: apiKeyID,
		TTL:   30 * time.Minute,
	})
	if err != nil {
		ns.logger.Warn("failed to cache api key", "key", apiKey, "error", err)
	}

	ns.logger.Debug("api key resolved from postgres", "key", apiKey, "id", apiKeyID)
	return apiKeyID, nil
}
