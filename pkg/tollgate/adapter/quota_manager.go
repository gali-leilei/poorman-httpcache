package adapter

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"httpcache/pkg/dbsqlc"

	"github.com/redis/go-redis/v9"
)

// QuotaManager handles quota operations in Redis
type QuotaManager struct {
	redis     RedisClient
	db        *dbsqlc.Queries
	serviceID string
}

// NewQuotaManager creates a new quota manager
func NewQuotaManager(redis RedisClient, db *dbsqlc.Queries, serviceID string) *QuotaManager {
	return &QuotaManager{
		redis:     redis,
		db:        db,
		serviceID: serviceID,
	}
}

// ConsumeQuota attempts to consume quota and returns remaining balance
func (qm *QuotaManager) ConsumeQuota(ctx context.Context, apiKey string, metadata *KeyMetadata) (int, error) {
	serviceKey := fmt.Sprintf("service_%s", qm.serviceID)
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)

	// Construct keys explicitly for Redis clustering compatibility
	quotaKey := fmt.Sprintf("quota:%s", apiKey)
	minuteTimestamp := time.Now().Truncate(time.Minute)
	bufferKey := fmt.Sprintf("usage_buffer:%s:%s:%d", metadata.APIKeyID, qm.serviceID, minuteTimestamp.Unix())

	result, err := ConsumeQuotaScript.Run(ctx, qm.redis,
		[]string{quotaKey, bufferKey},
		serviceKey, strconv.FormatBool(metadata.HasQuota), timestamp).Result()
	if err != nil {
		return 0, fmt.Errorf("redis consume failed: %w", err)
	}

	// Safely convert Redis script result
	values, ok := result.([]interface{})
	if !ok {
		return 0, fmt.Errorf("redis script returned unexpected type, expected []interface{}, got %T", result)
	}

	if len(values) != 2 {
		return 0, fmt.Errorf("redis script returned unexpected array length, expected 2, got %d", len(values))
	}

	remaining, ok := values[0].(int64)
	if !ok {
		return 0, fmt.Errorf("redis script returned unexpected type for remaining quota, expected int64, got %T", values[0])
	}

	status, ok := values[1].(string)
	if !ok {
		return 0, fmt.Errorf("redis script returned unexpected type for status, expected string, got %T", values[1])
	}

	switch status {
	case "LOAD_REQUIRED":
		return qm.loadAndConsume(ctx, apiKey, metadata)
	case "EXHAUSTED":
		return 0, fmt.Errorf("quota exhausted")
	case "OK":
		return int(remaining), nil
	default:
		return 0, fmt.Errorf("unknown status: %s", status)
	}
}

// GetBalance returns the current quota balance
func (qm *QuotaManager) GetBalance(ctx context.Context, apiKey string, metadata *KeyMetadata) (int, error) {
	// No-quota keys always return unlimited
	if !metadata.HasQuota {
		return 999999, nil
	}

	quotaKey := fmt.Sprintf("quota:%s", apiKey)
	serviceKey := fmt.Sprintf("service_%s", qm.serviceID)

	// Use HGET to check balance for this service
	result, err := qm.redis.HGet(ctx, quotaKey, serviceKey).Result()
	if err == redis.Nil {
		// Not in cache, load from PostgreSQL
		balance, err := qm.db.GetBalanceByKeyString(ctx, apiKey)
		if err != nil {
			return 0, fmt.Errorf("failed to get balance: %w", err)
		}

		// Cache using HSET with metadata
		qm.redis.HMSet(ctx, quotaKey, map[string]interface{}{
			serviceKey:  balance,
			"loaded_at": time.Now().Unix(),
			"load_type": "balance_check",
		})
		qm.redis.Expire(ctx, quotaKey, time.Hour)

		return int(balance), nil
	}
	if err != nil {
		return 0, fmt.Errorf("redis hget failed: %w", err)
	}

	remaining, _ := strconv.Atoi(result)
	return remaining, nil
}

// loadAndConsume loads quota from PostgreSQL and consumes it atomically
func (qm *QuotaManager) loadAndConsume(ctx context.Context, keyString string, metadata *KeyMetadata) (int, error) {
	// Only load balance if key has quota
	if !metadata.HasQuota {
		// For no-quota keys, just track consumption and return unlimited
		qm.trackConsumption(ctx, metadata.APIKeyID, qm.serviceID, 1)
		return 999999, nil
	}

	// Load from PostgreSQL for quota keys
	balance, err := qm.db.GetBalanceByKeyString(ctx, keyString)
	if err != nil {
		return 0, fmt.Errorf("failed to load balance: %w", err)
	}

	if balance <= 0 {
		return 0, fmt.Errorf("quota exhausted")
	}

	quotaKey := fmt.Sprintf("quota:%s", keyString)
	serviceKey := fmt.Sprintf("service_%s", qm.serviceID)
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)

	// Construct usage buffer key explicitly for Redis clustering compatibility
	minuteTimestamp := time.Now().Truncate(time.Minute)
	bufferKey := fmt.Sprintf("usage_buffer:%s:%s:%d", metadata.APIKeyID, qm.serviceID, minuteTimestamp.Unix())

	result, err := LoadAndConsumeScript.Run(ctx, qm.redis,
		[]string{quotaKey, bufferKey},
		serviceKey, strconv.Itoa(int(balance)), timestamp).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to load and consume: %w", err)
	}

	// Safely convert Redis script result
	remaining, ok := result.(int64)
	if !ok {
		return 0, fmt.Errorf("redis script returned unexpected type for remaining quota, expected int64, got %T", result)
	}

	return int(remaining), nil
}

// trackConsumption tracks consumption for no-quota keys using direct aggregation
func (qm *QuotaManager) trackConsumption(ctx context.Context, apiKey string, serviceName string, amount int) {
	minuteTimestamp := time.Now().Truncate(time.Minute)
	bufferKey := fmt.Sprintf("usage_buffer:%s:%s:%d", apiKey, serviceName, minuteTimestamp.Unix())

	// Increment counter and set TTL
	pipe := qm.redis.Pipeline()
	pipe.IncrBy(ctx, bufferKey, int64(amount))
	pipe.Expire(ctx, bufferKey, 2*time.Hour)

	if _, err := pipe.Exec(ctx); err != nil {
		// Log the error but don't fail the request - usage tracking is best effort
		// This prevents blocking the main quota consumption flow due to tracking failures
		// TODO: Consider using structured logging when available in the context
		_ = err // Acknowledge the error exists but don't fail the operation
	}
}

// BatchUpdateQuotas provides batch quota operations for administrative tasks
func (qm *QuotaManager) BatchUpdateQuotas(ctx context.Context, updates map[string]int) error {
	pipe := qm.redis.Pipeline()

	serviceKey := fmt.Sprintf("service_%s", qm.serviceID)
	for keyString, quota := range updates {
		quotaKey := fmt.Sprintf("quota:%s", keyString)
		pipe.HSet(ctx, quotaKey, serviceKey, quota, "updated_at", time.Now().Unix())
		pipe.Expire(ctx, quotaKey, time.Hour)
	}

	_, err := pipe.Exec(ctx)
	return err
}
