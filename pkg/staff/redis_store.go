package staff

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisStore represents the session store.
type RedisStore struct {
	pool   *redis.Client
	prefix string
}

// NewRedisStore returns a new RedisStore instance. The pool parameter should be a pointer
// to a redigo connection pool. See https://godoc.org/github.com/gomodule/redigo/redis#Pool.
func NewRedisStore(redisOptions *redis.Options) *RedisStore {
	client := redis.NewClient(redisOptions)
	return NewRedisStoreWithPrefix("scs:session:", client)
}

// NewRedisStoreWithPrefix returns a new RedisStore instance. The pool parameter should be a pointer
// to a redigo connection pool. The prefix parameter controls the Redis key
// prefix, which can be used to avoid naming clashes if necessary.
func NewRedisStoreWithPrefix(prefix string, client *redis.Client) *RedisStore {
	return &RedisStore{
		pool:   client,
		prefix: prefix,
	}
}

// Find returns the data for a given session token from the RedisStore instance.
// If the session token is not found or is expired, the returned exists flag
// will be set to false.
func (r *RedisStore) Find(token string) (b []byte, exists bool, err error) {
	b, err = r.pool.Get(context.Background(), r.prefix+token).Bytes()
	if err == redis.Nil {
		return nil, false, nil
	} else if err != nil {
		return nil, false, err
	}
	return b, true, nil
}

// Commit adds a session token and data to the RedisStore instance with the
// given expiry time. If the session token already exists then the data and
// expiry time are updated.
func (r *RedisStore) Commit(token string, b []byte, expiry time.Time) error {
	_, err := r.pool.Set(context.Background(), r.prefix+token, b, time.Until(expiry)).Result()
	return err

}

// Delete removes a session token and corresponding data from the RedisStore
// instance.
func (r *RedisStore) Delete(token string) error {
	_, err := r.pool.Del(context.Background(), r.prefix+token).Result()
	return err
}

// All returns a map containing the token and data for all active (i.e.
// not expired) sessions in the RedisStore instance.
func (r *RedisStore) All() (map[string][]byte, error) {
	ctx := context.Background()
	iter := r.pool.Scan(ctx, 0, r.prefix+"*", 0).Iterator()

	sessions := make(map[string][]byte)

	for iter.Next(ctx) {
		token := iter.Val()[len(r.prefix):]
		data, exists, err := r.Find(token)
		if err == redis.Nil {
			return nil, nil
		} else if err != nil {
			return nil, err
		}
		if exists {
			sessions[token] = data
		}
	}

	return sessions, nil
}
