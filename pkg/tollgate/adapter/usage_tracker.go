package adapter

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"httpcache/pkg/dbsqlc"

	"github.com/jackc/pgx/v5/pgtype"
)

// UsageTracker handles usage aggregation and flushing to PostgreSQL
type UsageTracker struct {
	redis  RedisClient
	db     *dbsqlc.Queries
	logger *slog.Logger
	ctx    context.Context
	wg     sync.WaitGroup
}

// NewUsageTracker creates a new usage tracker
func NewUsageTracker(ctx context.Context, redis RedisClient, db *dbsqlc.Queries, logger *slog.Logger) *UsageTracker {
	tracker := &UsageTracker{
		redis:  redis,
		db:     db,
		logger: logger,
		ctx:    ctx,
	}

	// Start background aggregation flusher
	tracker.wg.Add(1)
	go tracker.aggregationFlusher(ctx)

	return tracker
}

// aggregationFlusher periodically flushes aggregated usage data to PostgreSQL
func (ut *UsageTracker) aggregationFlusher(ctx context.Context) {
	defer ut.wg.Done()

	ticker := time.NewTicker(30 * time.Second) // Flush every 30 seconds
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := ut.flushAggregatedUsage(ut.ctx); err != nil {
				ut.logger.Error("Failed to flush aggregated usage", "error", err)
			}
		case <-ctx.Done():
			// Final flush before shutdown - ignore errors during shutdown
			if err := ut.flushAggregatedUsage(ctx); err != nil {
				ut.logger.Error("Failed to flush during shutdown", "error", err)
			}
			return
		}
	}
}

// flushAggregatedUsage flushes buffered minute aggregations to PostgreSQL
func (ut *UsageTracker) flushAggregatedUsage(ctx context.Context) error {
	pattern := "usage_buffer:*"
	iter := ut.redis.Scan(ctx, 0, pattern, 100).Iterator()

	flushed := 0
	for iter.Next(ctx) {
		key := iter.Val()
		// Parse key: usage_buffer:{api_key_id}:{service_id}:{minute_timestamp}
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

// Shutdown gracefully shuts down the usage tracker
func (ut *UsageTracker) Shutdown(ctx context.Context) error {
	// Wait for workers to finish with timeout
	done := make(chan struct{})
	go func() {
		ut.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("usage tracker shutdown timeout")
	}
}
