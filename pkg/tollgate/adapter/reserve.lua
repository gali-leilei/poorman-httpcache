-- All keys must be explicitly provided for Redis clustering compatibility
local quotaKey = KEYS[1]    -- Pre-constructed "quota:{apikey}" key
local usageKey = KEYS[2]   -- Pre-constructed usage buffer key
local serviceKey = ARGV[1]  -- Service key for hash field
local hasQuota = ARGV[2] == "true"
local amount = tonumber(ARGV[3])  -- Amount to reserve
local timestamp = ARGV[4]

-- Get current quota only if has_quota is true
local remaining = -1
if hasQuota then
	local current = redis.call('HGET', quotaKey, serviceKey)
	if current == false then
		-- Mark for lazy loading
		redis.call('HSET', quotaKey, serviceKey .. ":load", "pending")
		return {-1, 'LOAD_REQUIRED'}
	end
	
	remaining = tonumber(current)
	if remaining < amount then
		return {remaining, 'EXHAUSTED'}
	end
	
	-- Decrement quota by amount
	remaining = remaining - amount
	redis.call('HSET', quotaKey, serviceKey, remaining)
	redis.call('HSET', quotaKey, 'last_used', timestamp)
else
	-- No quota key - always succeed but track consumption
	remaining = 999999 -- Unlimited indicator
end

-- Direct aggregation - increment usage buffer (key pre-constructed by caller)
redis.call('INCRBY', usageKey, amount)
redis.call('EXPIRE', usageKey, 7200) -- 2 hour TTL

return {remaining, 'OK'}
