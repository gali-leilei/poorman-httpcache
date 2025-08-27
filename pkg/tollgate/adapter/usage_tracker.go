package adapter

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"httpcache/pkg/dbsqlc"

	"github.com/jackc/pgx/v5/pgtype"
)

// UsageTracker handles usage data aggregation and flushing to database
type UsageTracker struct {
	redis  RedisClient
	db     *dbsqlc.Queries
	logger *slog.Logger
}

// NewUsageTracker creates a new usage tracker without background processing
func NewUsageTracker(ctx context.Context, redis RedisClient, db *dbsqlc.Queries, logger *slog.Logger) *UsageTracker {
	return &UsageTracker{
		redis:  redis,
		db:     db,
		logger: logger,
	}
}

// Archive flushes buffered minute aggregations to PostgreSQL
func (ut *UsageTracker) Archive(ctx context.Context) error {
	pattern := "usage:*"
	iter := ut.redis.Scan(ctx, 0, pattern, 100).Iterator()

	flushed := 0
	for iter.Next(ctx) {
		key := iter.Val()
		// Parse key: usage:{api_key_id}:{service_id}:{minute_timestamp}
		parts := strings.Split(key, ":")
		if len(parts) != 4 {
			continue
		}

		apiKeyID, err1 := strconv.ParseInt(parts[1], 10, 64)
		serviceID, err2 := strconv.ParseInt(parts[2], 10, 64)
		minuteTimestamp, err3 := strconv.ParseInt(parts[3], 10, 64)
		if err1 != nil || err2 != nil || err3 != nil {
			continue
		}

		// Get and reset the count atomically
		count, err := ut.redis.GetDel(ctx, key).Int()
		if err != nil || count <= 0 {
			continue
		}

		// Upsert to PostgreSQL
		_, err = ut.db.UpsertMinuteUsage(ctx, &dbsqlc.UpsertMinuteUsageParams{
			ApiKeyID:          apiKeyID,
			ServiceID:         serviceID,
			ConsumptionAmount: int32(count),
			MinuteTimestamp:   pgtype.Timestamptz{Time: time.Unix(minuteTimestamp, 0), Valid: true},
		})

		if err != nil {
			// Re-add to Redis with shorter TTL for retry
			ut.redis.SetEx(ctx, key, count, 5*time.Minute)
			ut.logger.Error("Failed to flush usage data", "key", key, "error", err)
		} else {
			flushed++
		}
	}

	if flushed > 0 {
		ut.logger.Debug("Flushed aggregated usage data", "records", flushed)
	}

	return iter.Err()
}
