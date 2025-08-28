/*
MIT License

Copyright (c) 2018 Victor Springer

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package cache

import (
	"context"
	"log/slog"
	"time"

	"github.com/go-redis/cache/v9"
	"github.com/redis/go-redis/v9"
	"github.com/vmihailenco/msgpack/v5"
)

// Adapter interface for HTTP cache middleware client.
type Adapter interface {
	// Get retrieves the cached response by a given key. It also
	// returns true or false, whether it exists or not.
	Get(ctx context.Context, key uint64) ([]byte, bool)

	// Set caches a response for a given key until an expiration date.
	Set(key uint64, response []byte, expiration time.Time)

	// Release frees cache for a given key.
	Release(ctx context.Context, key uint64)
}

// RedisAdapter is the Redis adapter data structure.
type RedisAdapter struct {
	store  *cache.Cache
	logger *slog.Logger
}

// Get implements the cache Adapter interface Get method.
func (ra *RedisAdapter) Get(ctx context.Context, key uint64) ([]byte, bool) {
	var c []byte
	if err := ra.store.Get(ctx, KeyAsString(key), &c); err == nil {
		return c, true
	}

	return nil, false
}

// Set implements the cache Adapter interface Set method.
func (ra *RedisAdapter) Set(key uint64, response []byte, expiration time.Time) {
	err := ra.store.Set(&cache.Item{
		Key:   KeyAsString(key),
		Value: response,
		TTL:   time.Until(expiration),
	})
	if err != nil {
		ra.logger.Error("Failed to set cache", "key", KeyAsString(key), "error", err)
	}
}

// Release implements the cache Adapter interface Release method.
func (ra *RedisAdapter) Release(ctx context.Context, key uint64) {
	if err := ra.store.Delete(ctx, KeyAsString(key)); err != nil {
		ra.logger.Error("Failed to delete cache entry", "key", key, "error", err)
	}
}

// NewRedisAdapter initializes Redis adapter
func NewRedisAdapter(opt *redis.ClusterOptions, logger *slog.Logger) *RedisAdapter {
	cluster := redis.NewClusterClient(opt)
	store := cache.New(&cache.Options{
		Redis: cluster,
		Marshal: func(v any) ([]byte, error) {
			return msgpack.Marshal(v)
		},
		Unmarshal: func(b []byte, v any) error {
			return msgpack.Unmarshal(b, v)
		},
		LocalCache: cache.NewTinyLFU(1000, 10*time.Minute),
	})
	return &RedisAdapter{
		store:  store,
		logger: logger,
	}
}
