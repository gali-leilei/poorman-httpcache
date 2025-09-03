package tollgate

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/redis/go-redis/v9"
)

const (
	// BucketSizeMinutes defines the time bucket size for metrics in minutes
	BucketSizeMinutes = 60
	// MetricTTLSeconds defines the TTL for metric buckets in seconds (1 day)
	MetricTTLSeconds = 24 * 60 * 60
	// StatusOK indicates successful quota operation
	StatusOK = "OK"
	// StatusExhausted indicates quota has been exhausted
	StatusExhausted = "EXHAUSTED"
)

var (
	// ErrInvalidKey is returned when key is empty
	ErrInvalidKey = errors.New("key cannot be empty")
	// ErrInvalidAmount is returned when amount is not positive
	ErrInvalidAmount = errors.New("amount must be positive")
	// ErrInvalidScriptResult is returned when Lua script returns unexpected result
	ErrInvalidScriptResult = errors.New("invalid script result format")
)

var updateQuota = `
local service_id = ARGV[1]
local api_key_id = ARGV[2]
local amount = tonumber(ARGV[3])
local bucket_size = tonumber(ARGV[4]) or 60
local ttl = tonumber(ARGV[5]) or (24 * 60 * 60)

local quota_key = "quota:" .. api_key_id .. ":" .. service_id
local metric_key = "metric:" .. api_key_id .. ":" .. service_id
local timestamp = redis.call("TIME")
local current_time = timestamp[1]
local bucket_timestamp = math.floor(current_time / bucket_size) * bucket_size
local bucket_key = "bucket:" .. bucket_timestamp

-- update quota
local service_account = redis.call("HGET", quota_key, "service_account")
if service_account == false then
	-- do not update available quota for accounts without service_account
	redis.call("HINCRBY", quota_key, "consumed", amount)
	redis.call("HSET", quota_key, "updated_at", current_time)
else
	local available_str = redis.call("HGET", quota_key, "available")
	local available = tonumber(available_str) or 0
	if available < amount then
		return {tostring(available), "EXHAUSTED"}
	end
	redis.call("HINCRBY", quota_key, "available", -amount)
	redis.call("HINCRBY", quota_key, "consumed", amount)
	redis.call("HSET", quota_key, "updated_at", current_time)
end

-- update metric
redis.call("HINCRBY", metric_key, bucket_key, amount)
redis.call("HEXPIRE", metric_key, bucket_key, ttl)

-- return available quota
local final_available_str = redis.call("HGET", quota_key, "available")
local final_available = tonumber(final_available_str) or 0
return {tostring(final_available), "OK"}
`

// RedisAdapter implements the Adapter interface using Redis as the backend
type RedisAdapter struct {
	redis     *redis.Client
	serviceID string
	luaScript *redis.Script
}

// NewRedisAdapter creates a new Redis-based quota adapter
func NewRedisAdapter(redisC *redis.Client, serviceID string) *RedisAdapter {
	return &RedisAdapter{
		redis:     redisC,
		serviceID: serviceID,
		luaScript: redis.NewScript(updateQuota),
	}
}

// parseUpdateQuotaResult parses the result from the Lua script
func parseUpdateQuotaResult(result any) (int, string, error) {
	values, ok := result.([]interface{})
	if !ok {
		return 0, "", fmt.Errorf("%w: failed to convert result to []interface{}", ErrInvalidScriptResult)
	}

	if len(values) != 2 {
		return 0, "", fmt.Errorf("%w: expected 2 values in result, got %d", ErrInvalidScriptResult, len(values))
	}

	// Redis Lua scripts return strings, so we need to convert
	availableStr, ok := values[0].(string)
	if !ok {
		return 0, "", fmt.Errorf("%w: failed to convert available to string", ErrInvalidScriptResult)
	}

	available, err := strconv.Atoi(availableStr)
	if err != nil {
		return 0, "", fmt.Errorf("%w: failed to parse available quota: %v", ErrInvalidScriptResult, err)
	}

	status, ok := values[1].(string)
	if !ok {
		return 0, "", fmt.Errorf("%w: failed to convert status to string", ErrInvalidScriptResult)
	}

	return available, status, nil
}

// Reserve attempts to reserve the specified amount of quota for the given key.
// Returns true if the reservation was successful, false if insufficient quota remains.
func (ra *RedisAdapter) Reserve(ctx context.Context, key string, amount int) (bool, error) {
	if key == "" {
		return false, ErrInvalidKey
	}
	if amount <= 0 {
		return false, ErrInvalidAmount
	}

	keys := []string{} // Empty keys as we're using ARGV for all parameters
	argv := []string{
		ra.serviceID,
		key,
		strconv.Itoa(amount),
		strconv.Itoa(BucketSizeMinutes),
		strconv.Itoa(MetricTTLSeconds),
	}

	result, err := ra.luaScript.Run(ctx, ra.redis, keys, argv).Result()
	if err != nil {
		return false, fmt.Errorf("failed to execute quota reservation: %w", err)
	}

	_, status, err := parseUpdateQuotaResult(result)
	if err != nil {
		return false, err
	}

	return status == StatusOK, nil
}

// Refund returns the specified amount of quota for the given key.
// Returns true if the refund was successful, false otherwise.
func (ra *RedisAdapter) Refund(ctx context.Context, key string, amount int) (bool, error) {
	if key == "" {
		return false, ErrInvalidKey
	}
	if amount <= 0 {
		return false, ErrInvalidAmount
	}

	keys := []string{} // Empty keys as we're using ARGV for all parameters
	argv := []string{
		ra.serviceID,
		key,
		strconv.Itoa(-amount), // Negative amount for refund
		strconv.Itoa(BucketSizeMinutes),
		strconv.Itoa(MetricTTLSeconds),
	}

	result, err := ra.luaScript.Run(ctx, ra.redis, keys, argv).Result()
	if err != nil {
		return false, fmt.Errorf("failed to execute quota refund: %w", err)
	}

	_, status, err := parseUpdateQuotaResult(result)
	if err != nil {
		return false, err
	}

	return status == StatusOK, nil
}
