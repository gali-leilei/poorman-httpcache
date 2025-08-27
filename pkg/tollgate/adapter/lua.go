package adapter

import (
	_ "embed"

	"github.com/redis/go-redis/v9"
)

//go:embed reserve.lua
var reserveQuotaScript string

//go:embed set_and_reserve.lua
var setAndReserveQuotaScript string

//go:embed refund.lua
var refundQuotaScript string

// ReserveQuotaScript is the Redis script for consuming quota
var ReserveQuotaScript = redis.NewScript(reserveQuotaScript)

// SetAndReserveScript is the Redis script for setting and consuming quota
var SetAndReserveScript = redis.NewScript(setAndReserveQuotaScript)

// RefundQuotaScript is the Redis script for refunding quota
var RefundQuotaScript = redis.NewScript(refundQuotaScript)
