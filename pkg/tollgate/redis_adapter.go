package tollgate

import (
	"context"
	"errors"
	"strconv"

	"github.com/redis/go-redis/v9"
)

var updateQuota = `
local service_id = ARGV[1]
local api_key_id = ARGV[2]
local amount = tonumber(ARGV[3])

local quota_key = "quota:" .. api_key_id .. ":" .. service_id
local metric_key = "metric:" .. api_key_id .. ":" .. service_id
local timestamp = redis.call("TIME")
local bucket_timestamp = math.floor(timestamp[1] / 60) * 60
local bucket_key = "bucket:" .. bucket_timestamp

-- update quota
local service_account = redis.call("HGET", quota_key, "service_account")
if service_account == nil then
	-- do not update available quota
	redis.call("HINCRBY", quota_key, "consumed", amount)
	redis.call("HSET", quota_key, "updated_at", timestamp)
else
	local available = redis.call("HGET", quota_key, "available")
	if available < amount then
		return {available, "EXHAUSTED"}
	end
	redis.call("HINCRBY", quota_key, "available", -amount)
	redis.call("HINCRBY", quota_key, "consumed", amount)
	redis.call("HSET", quota_key, "updated_at", timestamp)
end

-- update metric
redis.call("HINCRBY", metric_key, bucket_key, amount)
redis.call("HEXPIRE", metric_key, bucket_key, 1*24*60*60) -- 1 day

-- return available quota
local available = redis.call("HGET", quota_key, "available")
return {available, "OK"}
`

type RedisAdapter struct {
	redis     *redis.Client
	serviceID string
	luaScript *redis.Script
}

func NewRedisAdapter(redisC *redis.Client, serviceID string) *RedisAdapter {
	return &RedisAdapter{redis: redisC, serviceID: serviceID, luaScript: redis.NewScript(updateQuota)}
}

func parseUpdateQuotaResult(result any) (int, string, error) {
	values, ok := result.([]interface{})
	if !ok {
		return 0, "", errors.New("failed to convert result to []interface{}")
	}

	if len(values) != 2 {
		return 0, "", errors.New("failed to convert result to []interface{}")
	}

	available, ok := values[0].(int)
	if !ok {
		return 0, "", errors.New("failed to convert available to int")
	}

	status, ok := values[1].(string)
	if !ok {
		return 0, "", errors.New("failed to convert status to string")
	}

	return available, status, nil
}

func (ra *RedisAdapter) Reserve(ctx context.Context, key string, amount int) (bool, error) {

	keys := []string{}
	argv := []string{ra.serviceID, key, strconv.Itoa(amount)}

	result, err := ra.luaScript.Run(ctx, ra.redis, keys, argv).Result()
	if err != nil {
		return false, err
	}

	_, status, err := parseUpdateQuotaResult(result)
	if err != nil {
		return false, err
	}

	return status == "OK", nil
}

func (ra *RedisAdapter) Refund(ctx context.Context, key string, amount int) (bool, error) {
	keys := []string{}
	argv := []string{ra.serviceID, key, strconv.Itoa(-amount)}

	result, err := ra.luaScript.Run(ctx, ra.redis, keys, argv).Result()
	if err != nil {
		return false, err
	}

	_, status, err := parseUpdateQuotaResult(result)
	if err != nil {
		return false, err
	}

	return status == "OK", nil
}
