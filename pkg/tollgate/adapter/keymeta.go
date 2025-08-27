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
	APIKeyID string `json:"api_key_id"`
	HasQuota bool   `json:"has_quota"`
}

// KeyMetadataStore handles Redis caching operations for API key metadata
type KeyMetadataStore struct {
	redis RedisClient
	db    *dbsqlc.Queries
}

// NewKeyMetadataStore creates a new Redis cache handler
func NewKeyMetadataStore(redis RedisClient, db *dbsqlc.Queries) *KeyMetadataStore {
	return &KeyMetadataStore{
		redis: redis,
		db:    db,
	}
}

// Get retrieves cached key metadata, loading from DB if not cached
func (c *KeyMetadataStore) Get(ctx context.Context, keyString string) (*KeyMetadata, error) {
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
		APIKeyID: fmt.Sprintf("%d", keyInfo.ID),
		HasQuota: keyInfo.HasQuota.Bool,
	}

	// Cache the metadata for 1 hour
	metadataJSON, _ := json.Marshal(metadata)
	c.redis.SetEx(ctx, metaKey, string(metadataJSON), time.Hour)

	return metadata, nil
}

// Reset removes cached metadata for a key
func (c *KeyMetadataStore) Reset(ctx context.Context, keyString string) error {
	metaKey := fmt.Sprintf("key_meta:%s", keyString)
	return c.redis.Del(ctx, metaKey).Err()
}
