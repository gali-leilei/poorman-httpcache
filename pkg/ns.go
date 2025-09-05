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
	"golang.org/x/sync/singleflight"
)

// NameServer is a name server that resolves
// 1. service name to internal service ID
// 2. api key to internal api key ID
type NameServer struct {
	cache   *cache.Cache
	queries *dbsqlc.Queries
	logger  *slog.Logger
	sf      singleflight.Group
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

	// 2. Cache miss - use single flight for database query
	result, err, _ := ns.sf.Do(key, func() (interface{}, error) {
		// Check cache again in case another goroutine already fetched it
		var cachedServiceID int
		if cacheErr := ns.cache.Get(context.Background(), key, &cachedServiceID); cacheErr == nil {
			return cachedServiceID, nil
		}

		// Query postgres
		service, err := ns.queries.GetServiceByName(context.Background(), serviceName)
		if err != nil {
			return 0, fmt.Errorf("GetServiceByName: %w", err)
		}
		fetchedServiceID := int(service.ID)

		// Cache the result (automatically sets in both local TinyLFU and Redis)
		cacheErr := ns.cache.Set(&cache.Item{
			Key:   key,
			Value: fetchedServiceID,
			TTL:   30 * time.Minute,
		})
		if cacheErr != nil {
			ns.logger.Warn("failed to cache service name", "service", serviceName, "error", cacheErr)
		}

		ns.logger.Debug("service name resolved from postgres", "service", serviceName, "id", fetchedServiceID)
		return fetchedServiceID, nil
	})

	if err != nil {
		return 0, err
	}

	serviceID = result.(int)
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

	// 2. Cache miss - use single flight for database query
	result, err, _ := ns.sf.Do(key, func() (interface{}, error) {
		// Check cache again in case another goroutine already fetched it
		var cachedAPIKeyID int
		if cacheErr := ns.cache.Get(context.Background(), key, &cachedAPIKeyID); cacheErr == nil {
			return cachedAPIKeyID, nil
		}

		// Query postgres
		apiKeyData, err := ns.queries.GetAPIKeyByName(context.Background(), apiKey)
		if err != nil {
			return 0, err
		}
		fetchedAPIKeyID := int(apiKeyData.ID)

		// Cache the result (automatically sets in both local TinyLFU and Redis)
		cacheErr := ns.cache.Set(&cache.Item{
			Key:   key,
			Value: fetchedAPIKeyID,
			TTL:   30 * time.Minute,
		})
		if cacheErr != nil {
			ns.logger.Warn("failed to cache api key", "key", apiKey, "error", cacheErr)
		}

		ns.logger.Debug("api key resolved from postgres", "key", apiKey, "id", fetchedAPIKeyID)
		return fetchedAPIKeyID, nil
	})

	if err != nil {
		return 0, err
	}

	apiKeyID = result.(int)
	return apiKeyID, nil
}
