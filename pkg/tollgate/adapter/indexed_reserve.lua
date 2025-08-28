-- Enhanced reserve.lua with sync index tracking
-- All keys must be explicitly provided for Redis clustering compatibility
local quotaKey = KEYS[1]    -- Pre-constructed "quota:{apikey}:{service}" key
local metricKey = KEYS[2]   -- Pre-constructed usage "usage:{apikey}:{service}"
local syncIndexKey = KEYS[3] -- Pre-constructed "sync:pending:{timestamp}" key
local hasQuota = ARGV[1] == "true" -- whether this key has quota
local amount = tonumber(ARGV[2])  -- Amount to reserve

-- Get timestamp from Redis and truncate to 60 seconds (round down to nearest minute)
local timeResult = redis.call('TIME')
local timestamp = math.floor(timeResult[1] / 60) * 60

-- Get current quota only if has_quota is true
local remaining = -1
if hasQuota then
	local current = redis.call('GET', quotaKey)
	if current == false then
		return {-1, 'LOAD_REQUIRED'}
	end
	
	remaining = tonumber(current)
	if remaining < amount then
		return {remaining, 'EXHAUSTED'}
	end
	
	-- Decrement quota by amount
	remaining = remaining - amount
	redis.call('SET', quotaKey, remaining, "EX", 24*60*60) -- 1 day TTL
else
	-- No quota limit - always succeed but track consumption
	remaining = 999999 -- Unlimited indicator
end

-- Direct aggregation - increment usage buffer with timestamp
local usageKey = metricKey .. ":" .. timestamp
redis.call('INCRBY', usageKey, amount)
redis.call('EXPIRE', usageKey, 2*60*60) -- 2 hour TTL

-- Add to sync index for efficient scanning
-- Extract apikey:service from metricKey (remove "usage:" prefix)
local apikeySvc = string.sub(metricKey, 7) -- Remove "usage:" (6 chars + 1)
redis.call('SADD', syncIndexKey, apikeySvc)
redis.call('EXPIRE', syncIndexKey, 3*60*60) -- 3 hour TTL (longer than usage data)

return {remaining, 'OK'}
