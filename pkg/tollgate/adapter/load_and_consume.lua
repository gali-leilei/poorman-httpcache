-- All keys must be explicitly provided for Redis clustering compatibility
local quotaKey = KEYS[1]    -- Pre-constructed quota key
local bufferKey = KEYS[2]   -- Pre-constructed usage buffer key
local serviceKey = ARGV[1]  -- Service key for hash field  
local initialQuota = tonumber(ARGV[2])
local timestamp = tonumber(ARGV[3])

-- Check if already loaded by another process
local existing = redis.call('HGET', quotaKey, serviceKey)
if existing ~= false then
	-- Already loaded, try consume
	local current = tonumber(existing)
	if current > 0 then
		local newRemaining = current - 1
		redis.call('HSET', quotaKey, serviceKey, newRemaining)
		redis.call('HSET', quotaKey, 'last_used', timestamp)
		
		-- Direct aggregation (key pre-constructed by caller)
		redis.call('INCRBY', bufferKey, 1)
		redis.call('EXPIRE', bufferKey, 7200)
		
		return newRemaining
	else
		return 0
	end
end

-- Load initial quota and consume
local newRemaining = initialQuota - 1
redis.call('HMSET', quotaKey, 
	serviceKey, newRemaining,
	'initial_quota', initialQuota,
	'loaded_at', timestamp,
	'last_used', timestamp
)
redis.call('EXPIRE', quotaKey, 3600) -- 1 hour TTL

-- Direct aggregation (key pre-constructed by caller)
redis.call('INCRBY', bufferKey, 1)
redis.call('EXPIRE', bufferKey, 7200)

return newRemaining
