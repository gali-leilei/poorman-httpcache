package main

import (
	"context"
	"errors"
	"fmt"
	"httpcache/pkg/dbsqlc"
	"log/slog"
	"strconv"

	"github.com/redis/go-redis/v9"
)

var writeQuotaAndMetric = `
local service_id = ARGV[1]
local api_key_id = ARGV[2]
local available = tonumber(ARGV[3])
local consumed = tonumber(ARGV[4])
local has_quota = ARGV[5]

local quota_key = "quota:" .. api_key_id .. ":" .. service_id
local metric_key = "metric:" .. api_key_id .. ":" .. service_id
local timestamp = redis.call("TIME")
local current_time = timestamp[1]

-- update quota
local quota_exists = redis.call("EXISTS", quota_key)
if quota_exists == 0 then
	redis.call("HMSET", quota_key, 
	"available", available, 
	"consumed", consumed,
	"updated_at", current_time)

	if has_quota == "true" then
		redis.call("HSET", quota_key, "service_account", "true")
	end
end

-- update metric
local metric_exists = redis.call("EXISTS", metric_key)
if metric_exists == 0 then
	redis.call("HMSET", metric_key, "updated_at", current_time)
end
return 1
`

type Record struct {
	ServiceID int64
	APIKeyID  int64
	Available int64
	Consumed  int64
	HasQuota  bool
}

// CacheWarmer warms up redis cache from postgres.
// 1. on service startup, populate `metrics` and `quota` table to redis.
type CacheWarmer struct {
	redis     *redis.Client
	queries   *dbsqlc.Queries
	logger    *slog.Logger
	luaScript *redis.Script
	batchSize int
}

func NewCacheWarmer(redisC *redis.Client, queries *dbsqlc.Queries, logger *slog.Logger, batchSize int) *CacheWarmer {
	return &CacheWarmer{
		redis:     redisC,
		queries:   queries,
		logger:    logger,
		luaScript: redis.NewScript(writeQuotaAndMetric),
		batchSize: batchSize,
	}
}

func (cw *CacheWarmer) BatchDo(ctx context.Context, records []Record) error {
	var errs []error
	keys := []string{}
	for idx, record := range records {
		argv := []string{
			strconv.FormatInt(record.ServiceID, 10),
			strconv.FormatInt(record.APIKeyID, 10),
			strconv.FormatInt(record.Available, 10),
			strconv.FormatInt(record.Consumed, 10),
			strconv.FormatBool(record.HasQuota),
		}
		_, err := cw.luaScript.Run(ctx, cw.redis, keys, argv).Result()
		if err != nil {
			errs = append(errs, fmt.Errorf("at index %d: %w", idx, err))
		}
	}

	if len(errs) > 0 {
		final := errors.Join(errs...)
		return fmt.Errorf("failed to write quota and metric: %w", final)
	}

	return nil
}

// Do warms up redis cache from postgres.
// 1. on service startup, populate `metrics` and `quota` table to redis.
// (A) for keys not present in redis, populate them.
// (B) for keys present in redis, but updated_at time is OLDER than updated_at time in postgres, update them.
// (C) else do nothing.
func (cw *CacheWarmer) Do(ctx context.Context) error {
	cw.logger.Info("Warming up cache...")

	var errs []error
	for offset := 0; ; offset += cw.batchSize {
		// Check for context cancellation before processing each batch
		select {
		case <-ctx.Done():
			cw.logger.Info("Cache warming cancelled by context", "processed_offset", offset)
			return ctx.Err()
		default:
		}

		sqlRows, err := cw.queries.GetAllQuotas(ctx, &dbsqlc.GetAllQuotasParams{
			Limit:  int32(cw.batchSize),
			Offset: int32(offset),
		})
		if err != nil {
			cw.logger.Error("failed to get all quotas", "error", err)
			errs = append(errs, fmt.Errorf("GetAllQuotas at offset %d: %w", offset, err))
			continue
		}
		if len(sqlRows) == 0 {
			break
		}
		batch := make([]Record, 0, cw.batchSize)
		for _, sqlRow := range sqlRows {
			batch = append(batch, Record{
				ServiceID: sqlRow.ServiceID,
				APIKeyID:  sqlRow.ApiKeyID,
				Available: int64(sqlRow.Available),
				Consumed:  int64(sqlRow.Consumed),
				HasQuota:  sqlRow.HasQuota,
			})
		}
		err = cw.BatchDo(ctx, batch)
		if err != nil {
			cw.logger.Error("failed to batch do", "error", err)
			errs = append(errs, fmt.Errorf("BatchDo at offset %d: %w", offset, err))
		}
	}

	if len(errs) > 0 {
		final := errors.Join(errs...)
		return fmt.Errorf("failed to batch do: %w", final)
	}

	return nil
}
