package adapter

import (
	"context"
	"fmt"
	"strconv"
	"time"
)

type SyncQuota func(ctx context.Context, ServiceMetaData ServiceMetadata, keyMeta KeyMetadata) (int, error)

// QuotaManager handles quota operations in Redis
type QuotaManager struct {
	serviceMetadata ServiceMetadata
	metaStore       MetaStore
	redis           RedisClient
}

// NewQuotaManager creates a new quota manager
func NewQuotaManager(ctx context.Context, redis RedisClient, metaStore MetaStore, serviceName string) (*QuotaManager, error) {
	// Get service metadata
	serviceMeta, err := metaStore.GetService(ctx, serviceName)
	if err != nil {
		return nil, fmt.Errorf("failed to get service metadata: %w", err)
	}

	return &QuotaManager{
		redis:           redis,
		metaStore:       metaStore,
		serviceMetadata: *serviceMeta,
	}, nil
}

// Reserve attempts to reserve a given amount of quota and returns success status
func (qm *QuotaManager) Reserve(ctx context.Context, keyMeta *KeyMetadata, amount int) (bool, error) {
	// Construct keys explicitly for Redis clustering compatibility
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	minuteTimestamp := time.Now().Truncate(time.Minute)

	keys := []string{
		fmt.Sprintf("quota:%s", keyMeta.APIKey),
		fmt.Sprintf("service_%s", qm.serviceMetadata.ServiceName),
		fmt.Sprintf("usage:%s:%s:%d", keyMeta.APIKey, qm.serviceMetadata.ServiceName, minuteTimestamp.Unix()),
	}

	argv := []interface{}{
		strconv.FormatBool(keyMeta.HasQuota),
		strconv.Itoa(amount),
		timestamp,
	}
	result, err := ReserveQuotaScript.Run(ctx, qm.redis, keys, argv...).Result()
	if err != nil {
		return false, fmt.Errorf("ReserveQuotaScript.Run: %w", err)
	}

	// Safely convert Redis script result
	values, ok := result.([]interface{})
	if !ok {
		return false, fmt.Errorf("ReserveQuotaScript.Run: expected []interface{}, got %T", result)
	}

	if len(values) != 2 {
		return false, fmt.Errorf("ReserveQuotaScript.Run: expected 2, got %d", len(values))
	}

	_, err = strconv.ParseInt(values[0].(string), 10, 64)
	if err != nil {
		return false, fmt.Errorf("ReserveQuotaScript.Run: result[0] expected int64, got %T", values[0])
	}

	status, ok := values[1].(string)
	if !ok {
		return false, fmt.Errorf("ReserveQuotaScript.Run: result[1] expected string, got %T", values[1])
	}

	switch status {
	case "LOAD_REQUIRED":
		return qm.setAndReserve(ctx, keyMeta, amount)
	case "EXHAUSTED":
		return false, nil // Not an error, just insufficient quota
	case "OK":
		return true, nil
	default:
		return false, fmt.Errorf("ReserveQuotaScript.Run: result[1] unknown status: %s", status)
	}
}

// Refund refunds a given amount of quota for a key
func (qm *QuotaManager) Refund(ctx context.Context, keyMeta *KeyMetadata, amount int) (bool, error) {
	// No-quota keys always return unlimited - nothing to refund
	if !keyMeta.HasQuota {
		return true, nil
	}

	serviceKey := fmt.Sprintf("service_%s", qm.serviceMetadata.ServiceName)
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)

	// Construct keys explicitly for Redis clustering compatibility
	quotaKey := fmt.Sprintf("quota:%s", keyMeta.APIKey)
	minuteTimestamp := time.Now().Truncate(time.Minute)
	usageKey := fmt.Sprintf("usage:%s:%s:%d", keyMeta.APIKey, qm.serviceMetadata.ServiceName, minuteTimestamp.Unix())

	result, err := RefundQuotaScript.Run(ctx, qm.redis,
		[]string{quotaKey, usageKey},
		serviceKey, strconv.Itoa(amount), timestamp).Result()
	if err != nil {
		return false, fmt.Errorf("redis refund failed: %w", err)
	}

	// Safely convert Redis script result
	values, ok := result.([]interface{})
	if !ok {
		return false, fmt.Errorf("redis script returned unexpected type, expected []interface{}, got %T", result)
	}

	if len(values) != 2 {
		return false, fmt.Errorf("redis script returned unexpected array length, expected 2, got %d", len(values))
	}

	_, ok = values[0].(int64)
	if !ok {
		return false, fmt.Errorf("redis script returned unexpected type for remaining quota, expected int64, got %T", values[0])
	}

	status, ok := values[1].(string)
	if !ok {
		return false, fmt.Errorf("redis script returned unexpected type for status, expected string, got %T", values[1])
	}

	switch status {
	case "NO_QUOTA":
		// No quota key exists in Redis - this is not an error, just means nothing to refund
		return true, nil
	case "OK":
		return true, nil
	default:
		return false, fmt.Errorf("unknown refund status: %s", status)
	}
}

// setAndReserve loads quota from PostgreSQL and reserves it atomically using singleflight
func (qm *QuotaManager) setAndReserve(ctx context.Context, keyMeta *KeyMetadata, amount int) (bool, error) {
	// Only load balance if key has quota
	if !keyMeta.HasQuota {
		// For no-quota keys, just track consumption and return unlimited
		qm.trackConsumption(ctx, keyMeta, amount)
		return true, nil
	}

	// Use singleflight to prevent concurrent loads for the same API key
	result, err := qm.metaStore.GetQuota(ctx, qm.serviceMetadata.ServiceName, keyMeta.APIKey)

	if err != nil {
		return false, fmt.Errorf("failed to load quota: %w", err)
	}

	// Now reserve the amount using the simplified set_and_reserve script
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	minuteTimestamp := time.Now().Truncate(time.Minute)

	// Construct keys explicitly for Redis clustering compatibility
	keys := []string{
		fmt.Sprintf("quota:%s", keyMeta.APIKey),
		fmt.Sprintf("service_%s", qm.serviceMetadata.ServiceName),
		fmt.Sprintf("usage:%s:%s:%d", keyMeta.APIKey, qm.serviceMetadata.ServiceName, minuteTimestamp.Unix()),
	}

	argv := []interface{}{
		strconv.Itoa(result),
		strconv.Itoa(amount),
		timestamp,
	}

	scriptResult, err := SetAndReserveScript.Run(ctx, qm.redis, keys, argv...).Result()
	if err != nil {
		return false, fmt.Errorf("SetAndReserveScript.Run: %w", err)
	}

	// Safely convert Redis script result - should match reserve.lua format
	values, ok := scriptResult.([]interface{})
	if !ok {
		return false, fmt.Errorf("SetAndReserveScript.Run: expected []interface{}, got %T", scriptResult)
	}

	if len(values) != 2 {
		return false, fmt.Errorf("SetAndReserveScript.Run: expected 2, got %d", len(values))
	}

	_, err = strconv.ParseInt(values[0].(string), 10, 64)
	if err != nil {
		return false, fmt.Errorf("SetAndReserveScript.Run: result[0] expected int64, got %T", values[0])
	}

	status, ok := values[1].(string)
	if !ok {
		return false, fmt.Errorf("SetAndReserveScript.Run: result[1] expected string, got %T", values[1])
	}

	switch status {
	case "EXHAUSTED":
		return false, nil // Not an error, just insufficient quota
	case "OK":
		return true, nil
	default:
		return false, fmt.Errorf("SetAndReserveScript.Run: result[1] unknown status: %s", status)
	}
}

// trackConsumption tracks consumption for no-quota keys using direct aggregation
func (qm *QuotaManager) trackConsumption(ctx context.Context, keyMeta *KeyMetadata, amount int) {
	minuteTimestamp := time.Now().Truncate(time.Minute)
	usageKey := fmt.Sprintf("usage:%s:%s:%d", keyMeta.APIKey, qm.serviceMetadata.ServiceName, minuteTimestamp.Unix())

	// Increment counter and set TTL
	pipe := qm.redis.Pipeline()
	pipe.IncrBy(ctx, usageKey, int64(amount))
	pipe.Expire(ctx, usageKey, 2*time.Hour)

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

	serviceKey := fmt.Sprintf("service_%s", qm.serviceMetadata.ServiceName)
	for keyString, quota := range updates {
		quotaKey := fmt.Sprintf("quota:%s", keyString)
		pipe.HSet(ctx, quotaKey, serviceKey, quota, "updated_at", time.Now().Unix())
		pipe.Expire(ctx, quotaKey, time.Hour)
	}

	_, err := pipe.Exec(ctx)
	return err
}
