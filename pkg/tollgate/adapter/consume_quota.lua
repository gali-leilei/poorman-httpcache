-- All keys must be explicitly provided for Redis clustering compatibility
local quotaKey = KEYS[1]    -- Pre-constructed "quota:{apikey}" key
local bufferKey = KEYS[2]   -- Pre-constructed usage buffer key
local serviceKey = ARGV[1]  -- Service key for hash field
local hasQuota = ARGV[2] == "true"
local timestamp = ARGV[3]

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
	if remaining <= 0 then
		return {0, 'EXHAUSTED'}
	end
	
	-- Decrement quota
	remaining = remaining - 1
	redis.call('HSET', quotaKey, serviceKey, remaining)
	redis.call('HSET', quotaKey, 'last_used', timestamp)
else
	-- No quota key - always succeed but track consumption
	remaining = 999999 -- Unlimited indicator
end

-- Direct aggregation - increment usage buffer (key pre-constructed by caller)
redis.call('INCRBY', bufferKey, 1)
redis.call('EXPIRE', bufferKey, 7200) -- 2 hour TTL

return {remaining, 'OK'}
