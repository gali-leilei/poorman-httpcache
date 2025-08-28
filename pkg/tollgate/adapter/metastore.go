package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"httpcache/pkg/dbsqlc"

	"golang.org/x/sync/singleflight"
)

// MetaStore is an interface that provides methods to get and set metadata for an API key and service.
type MetaStore interface {
	GetKey(ctx context.Context, keyString string) (*KeyMetadata, error)
	GetService(ctx context.Context, serviceName string) (*ServiceMetadata, error)
	ResetKey(ctx context.Context, keyString string) error
	ResetService(ctx context.Context, serviceName string) error
	GetQuota(ctx context.Context, serviceName string, keyString string) (int, error)
	ResetQuota(ctx context.Context, serviceName string, keyString string) error
}

// KeyMetadata represents cached metadata for an API key
type KeyMetadata struct {
	APIKeyID int64  `json:"api_key_id"`
	APIKey   string `json:"api_key"`
	HasQuota bool   `json:"has_quota"`
	Status   string `json:"status"`
}

// ServiceMetadata represents cached metadata for a service
type ServiceMetadata struct {
	ServiceID    int64  `json:"service_id"`
	ServiceName  string `json:"service_name"`
	DefaultQuota int32  `json:"default_quota"`
}

// RealMetaStore returns information aboout API key, user and service
// it use redis always, or singleflight to DB to avoid race condition.
type RealMetaStore struct {
	redis RedisClient
	db    *dbsqlc.Queries
	sf    singleflight.Group
}

// NewRedisMetadataStore creates a new Redis cache handler
func NewRedisMetadataStore(redis RedisClient, db *dbsqlc.Queries) *RealMetaStore {
	return &RealMetaStore{
		redis: redis,
		db:    db,
	}
}

// GetKey retrieves cached key metadata, loading from DB if not cached
func (c *RealMetaStore) GetKey(ctx context.Context, keyString string) (*KeyMetadata, error) {
	metaKey := fmt.Sprintf("key_meta:%s", keyString)

	// Try to get from cache first
	cached, err := c.redis.Get(ctx, metaKey).Result()
	if err == nil {
		var metadata KeyMetadata
		if err := json.Unmarshal([]byte(cached), &metadata); err == nil {
			return &metadata, nil
		}
	}

	// Cache miss or error, load from DB using singleflight
	sfKey := fmt.Sprintf("key_db:%s", keyString)
	result, err, _ := c.sf.Do(sfKey, func() (interface{}, error) {
		return c.db.GetAPIKeyByKeyString(ctx, keyString)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get key info: %w", err)
	}
	keyInfo := result.(*dbsqlc.GetAPIKeyByKeyStringRow)

	metadata := &KeyMetadata{
		APIKeyID: keyInfo.ID,
		APIKey:   keyInfo.KeyString,
		HasQuota: keyInfo.HasQuota,
		Status:   keyInfo.Status,
	}

	// Cache the metadata for 1 hour
	metadataJSON, _ := json.Marshal(metadata)
	c.redis.SetEx(ctx, metaKey, string(metadataJSON), time.Hour)

	return metadata, nil
}

// GetService retrieves cached service metadata, loading from DB if not cached
func (c *RealMetaStore) GetService(ctx context.Context, serviceName string) (*ServiceMetadata, error) {
	metaKey := fmt.Sprintf("service_meta:%s", serviceName)

	// Try to get from cache first
	cached, err := c.redis.Get(ctx, metaKey).Result()
	if err == nil {
		var metadata ServiceMetadata
		if err := json.Unmarshal([]byte(cached), &metadata); err == nil {
			return &metadata, nil
		}
	}

	// Cache miss or error, load from DB using singleflight
	sfKey := fmt.Sprintf("service_db:%s", serviceName)
	result, err, _ := c.sf.Do(sfKey, func() (interface{}, error) {
		return c.db.GetServiceByName(ctx, serviceName)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get service info: %w", err)
	}
	serviceInfo := result.(*dbsqlc.Services)

	metadata := &ServiceMetadata{
		ServiceID:    serviceInfo.ID,
		ServiceName:  serviceInfo.Name,
		DefaultQuota: serviceInfo.DefaultQuota,
	}

	// Cache the metadata for 1 hour
	metadataJSON, _ := json.Marshal(metadata)
	c.redis.SetEx(ctx, metaKey, string(metadataJSON), time.Hour)

	return metadata, nil
}

// ResetKey removes cached metadata for a key
func (c *RealMetaStore) ResetKey(ctx context.Context, keyString string) error {
	metaKey := fmt.Sprintf("key_meta:%s", keyString)
	return c.redis.Del(ctx, metaKey).Err()
}

// ResetService removes cached metadata for a service
func (c *RealMetaStore) ResetService(ctx context.Context, serviceName string) error {
	metaKey := fmt.Sprintf("service_meta:%s", serviceName)
	return c.redis.Del(ctx, metaKey).Err()
}

func (c *RealMetaStore) GetQuota(ctx context.Context, serviceName string, keyString string) (int, error) {
	quotaKey := fmt.Sprintf("quota:%s:%s", serviceName, keyString)

	// Try to get from cache first
	cached, err := c.redis.Get(ctx, quotaKey).Result()
	if err == nil {
		var quota int
		if err := json.Unmarshal([]byte(cached), &quota); err == nil {
			return quota, nil
		}
	}

	// Cache miss or error, load from DB using singleflight
	sfKey := fmt.Sprintf("quota_db:%s:%s", serviceName, keyString)
	result, err, _ := c.sf.Do(sfKey, func() (interface{}, error) {
		return c.db.GetQuota(ctx, &dbsqlc.GetQuotaParams{
			KeyString: keyString,
			Name:      serviceName,
		})
	})
	if err != nil {
		return 0, fmt.Errorf("GetQuota: %w", err)
	}
	res := result.(*dbsqlc.GetQuotaRow)

	quota := int(res.RemainingQuota)

	// Cache the quota for 5 minutes (shorter than key/service metadata due to frequent changes)
	quotaJSON, _ := json.Marshal(quota)
	c.redis.SetEx(ctx, quotaKey, string(quotaJSON), 5*time.Minute)

	return quota, nil
}

// ResetQuota removes cached quota for a specific service and key combination
func (c *RealMetaStore) ResetQuota(ctx context.Context, serviceName string, keyString string) error {
	quotaKey := fmt.Sprintf("quota:%s:%s", serviceName, keyString)
	return c.redis.Del(ctx, quotaKey).Err()
}
