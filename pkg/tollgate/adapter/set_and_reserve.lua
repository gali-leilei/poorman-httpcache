-- All keys must be explicitly provided for Redis clustering compatibility
local quotaKey = KEYS[1]    -- Pre-constructed "quota:{apikey}" key
local usageKey = KEYS[2]   -- Pre-constructed usage buffer key
local serviceKey = ARGV[1]  -- Service key for hash field
local initialQuota = tonumber(ARGV[2])  -- Initial quota amount to set
local amount = tonumber(ARGV[3])  -- Amount to reserve
local timestamp = ARGV[4]

-- Get current quota
local current = redis.call('HGET', quotaKey, serviceKey)
if current == false then
	-- No quota exists, set initial amount
	if initialQuota < amount then
		return {tostring(initialQuota), 'EXHAUSTED'}
	end
	
	-- Set initial quota and decrement by amount
	local remaining = initialQuota - amount
	redis.call('HSET', quotaKey, serviceKey, remaining)
	redis.call('HSET', quotaKey, 'last_used', timestamp)
	redis.call('EXPIRE', quotaKey, 3600) -- 1 hour TTL
	
	-- Direct aggregation - increment usage buffer (key pre-constructed by caller)
	redis.call('INCRBY', usageKey, amount)
	redis.call('EXPIRE', usageKey, 7200) -- 2 hour TTL
	
	return {tostring(remaining), 'OK'}
end

-- Quota exists, check if sufficient
local remaining = tonumber(current)
if remaining < amount then
	return {tostring(remaining), 'EXHAUSTED'}
end

-- Decrement quota by amount
remaining = remaining - amount
redis.call('HSET', quotaKey, serviceKey, remaining)
redis.call('HSET', quotaKey, 'last_used', timestamp)

-- Direct aggregation - increment usage buffer (key pre-constructed by caller)
redis.call('INCRBY', usageKey, amount)
redis.call('EXPIRE', usageKey, 7200) -- 2 hour TTL

return {tostring(remaining), 'OK'}
