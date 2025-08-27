package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"httpcache/pkg/dbsqlc"
)

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

// MetaStore returns information aboout API key, user and service
type MetaStore struct {
	redis RedisClient
	db    *dbsqlc.Queries
}

// NewKeyMetadataStore creates a new Redis cache handler
func NewKeyMetadataStore(redis RedisClient, db *dbsqlc.Queries) *MetaStore {
	return &MetaStore{
		redis: redis,
		db:    db,
	}
}

// GetKey retrieves cached key metadata, loading from DB if not cached
func (c *MetaStore) GetKey(ctx context.Context, keyString string) (*KeyMetadata, error) {
	metaKey := fmt.Sprintf("key_meta:%s", keyString)

	// Try to get from cache first
	cached, err := c.redis.Get(ctx, metaKey).Result()
	if err == nil {
		var metadata KeyMetadata
		if err := json.Unmarshal([]byte(cached), &metadata); err == nil {
			return &metadata, nil
		}
	}

	// Cache miss or error, load from DB
	keyInfo, err := c.db.GetAPIKeyByKeyString(ctx, keyString)
	if err != nil {
		return nil, fmt.Errorf("failed to get key info: %w", err)
	}

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
func (c *MetaStore) GetService(ctx context.Context, serviceName string) (*ServiceMetadata, error) {
	metaKey := fmt.Sprintf("service_meta:%s", serviceName)

	// Try to get from cache first
	cached, err := c.redis.Get(ctx, metaKey).Result()
	if err == nil {
		var metadata ServiceMetadata
		if err := json.Unmarshal([]byte(cached), &metadata); err == nil {
			return &metadata, nil
		}
	}

	// Cache miss or error, load from DB
	serviceInfo, err := c.db.GetServiceByName(ctx, serviceName)
	if err != nil {
		return nil, fmt.Errorf("failed to get service info: %w", err)
	}

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
func (c *MetaStore) ResetKey(ctx context.Context, keyString string) error {
	metaKey := fmt.Sprintf("key_meta:%s", keyString)
	return c.redis.Del(ctx, metaKey).Err()
}

// ResetService removes cached metadata for a service
func (c *MetaStore) ResetService(ctx context.Context, serviceName string) error {
	metaKey := fmt.Sprintf("service_meta:%s", serviceName)
	return c.redis.Del(ctx, metaKey).Err()
}
