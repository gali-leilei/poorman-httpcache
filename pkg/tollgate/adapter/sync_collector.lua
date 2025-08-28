-- Collect usage data for a specific timestamp using sync index
-- KEYS[1]: sync index key "sync:pending:{timestamp}"
-- ARGV[1]: timestamp to collect data for

local syncIndexKey = KEYS[1]
local timestamp = ARGV[1]

-- Get all apikey:service combinations for this timestamp
local apikeySvcs = redis.call('SMEMBERS', syncIndexKey)
local usageData = {}

for i, apikeySvc in ipairs(apikeySvcs) do
    local usageKey = "usage:" .. apikeySvc .. ":" .. timestamp
    local usage = redis.call('GET', usageKey)
    
    if usage then
        -- Split apikey:service
        local colon_pos = string.find(apikeySvc, ":")
        if colon_pos then
            local apikey = string.sub(apikeySvc, 1, colon_pos - 1)
            local service = string.sub(apikeySvc, colon_pos + 1)
            
            table.insert(usageData, {
                key = usageKey,
                apikey = apikey,
                service = service,
                timestamp = timestamp,
                usage = tonumber(usage) or 0
            })
        end
    end
end

-- Optional: Clean up the sync index after collecting
-- redis.call('DEL', syncIndexKey)

return usageData
