-- All keys must be explicitly provided for Redis clustering compatibility
local quotaKey = KEYS[1]    -- Pre-constructed "quota:{apikey}" key
local usageKey = KEYS[2]   -- Pre-constructed usage buffer key
local serviceKey = ARGV[1]  -- Service key for hash field
local amount = tonumber(ARGV[2])  -- Amount to refund
local timestamp = ARGV[3]

-- Get current quota
local current = redis.call('HGET', quotaKey, serviceKey)
if current == false then
	-- No quota key exists, nothing to refund
	return {0, 'NO_QUOTA'}
end

local remaining = tonumber(current)
-- Add back the refunded amount
local newRemaining = remaining + amount
redis.call('HSET', quotaKey, serviceKey, newRemaining)
redis.call('HSET', quotaKey, 'last_refund', timestamp)

-- Reduce the usage buffer to correct tracking
-- Only decrement if buffer exists and has enough to decrement
local bufferValue = redis.call('GET', usageKey)
if bufferValue ~= false then
	local currentBuffer = tonumber(bufferValue)
	if currentBuffer >= amount then
		redis.call('DECRBY', usageKey, amount)
	else
		-- Buffer has less than refund amount, set to 0
		redis.call('SET', usageKey, 0)
		redis.call('EXPIRE', usageKey, 7200) -- Keep TTL
	end
end

return {newRemaining, 'OK'}
