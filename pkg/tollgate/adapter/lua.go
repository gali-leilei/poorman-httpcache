package adapter

import (
	_ "embed"

	"github.com/redis/go-redis/v9"
)

//go:embed consume_quota.lua
var consumeQuotaScript string

//go:embed load_and_consume.lua
var loadAndConsumeScript string

// ConsumeQuotaScript is the Redis script for consuming quota
var ConsumeQuotaScript = redis.NewScript(consumeQuotaScript)

// LoadAndConsumeScript is the Redis script for loading and consuming quota
var LoadAndConsumeScript = redis.NewScript(loadAndConsumeScript)
