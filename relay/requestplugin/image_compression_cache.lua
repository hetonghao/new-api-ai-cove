local prefix = KEYS[1]
local state = prefix .. ':state'
local metadata = prefix .. ':metadata'
local ids = prefix .. ':ids'
local expires = prefix .. ':expires'
local lru = prefix .. ':lru'
local nodes = prefix .. ':nodes'
local operation, revision, enabled, maxBytes, ttl, token = ARGV[1], tonumber(ARGV[2]), ARGV[3], tonumber(ARGV[4]), tonumber(ARGV[5]), ARGV[6]
local clock = redis.call('TIME')
local now = tonumber(clock[1]) * 1000 + math.floor(tonumber(clock[2]) / 1000)

local function remove(key)
    local raw = redis.call('HGET', metadata, key)
    if not raw then return 0 end
    local row = cjson.decode(raw)
    redis.call('UNLINK', prefix .. ':image:' .. key)
    redis.call('HDEL', metadata, key)
    redis.call('HDEL', ids, row.id)
    redis.call('ZREM', lru, key)
    redis.call('ZREM', expires, key)
    redis.call('HINCRBY', state, 'used', -row.compressed_bytes)
    return 1
end

local function clear()
    local keys = redis.call('ZRANGE', lru, 0, -1)
    for _, key in ipairs(keys) do redis.call('UNLINK', prefix .. ':image:' .. key) end
    redis.call('UNLINK', metadata, ids, lru, expires)
    redis.call('HSET', state, 'used', 0)
    return #keys
end

local function prune()
    local keys = redis.call('ZRANGEBYSCORE', expires, '-inf', now)
    for _, key in ipairs(keys) do remove(key) end
    if #keys > 0 then redis.call('HINCRBY', state, 'expirations', #keys) end
end

local function fit(incoming, incomingCount)
    local used = tonumber(redis.call('HGET', state, 'used') or '0')
    local count = redis.call('ZCARD', lru)
    while used + incoming > maxBytes or count + incomingCount > 4096 do
        local victim = redis.call('ZRANGE', lru, 0, 0)[1]
        if not victim then break end
        remove(victim)
        redis.call('HINCRBY', state, 'evictions', 1)
        used = tonumber(redis.call('HGET', state, 'used'))
        count = count - 1
    end
end

local current = redis.call('HMGET', state, 'revision', 'enabled', 'ttl', 'token')
if current[1] and revision < tonumber(current[1]) then return {''} end
prune()
if not current[1] or revision > tonumber(current[1]) then
    -- A node can miss a disable/enable cycle while Redis is unavailable.
    if current[1] and (revision > tonumber(current[1]) + 1 or enabled == '0' or ttl ~= tonumber(current[3])) then
        redis.call('HINCRBY', state, 'removals', clear())
    end
    redis.call('HSET', state, 'revision', revision, 'enabled', enabled, 'ttl', ttl, 'token', token)
    fit(0, 0)
    current[4] = token
end
local generation = current[4]
local ttlMillis = ttl * 1000

if operation == 'lookup' then
    if enabled == '0' then return {''} end
    local node = ARGV[7]
    local reply, hits, misses = {generation}, 0, 0
    for i = 8, #ARGV, 2 do
        local key = ARGV[i]
        local occurrences = tonumber(ARGV[i + 1])
        local imageKey = prefix .. ':image:' .. key
        local value = redis.call('GET', imageKey)
        local raw = redis.call('HGET', metadata, key)
        if value and raw then
            local row = cjson.decode(raw)
            row.hits = row.hits + occurrences
            row.last_hit_at = tonumber(clock[1])
            row.expires_at = tonumber(clock[1]) + ttl
            redis.call('PEXPIRE', imageKey, ttlMillis)
            redis.call('ZADD', expires, now + ttlMillis, key)
            raw = cjson.encode(row)
            redis.call('HSET', metadata, key, raw)
            redis.call('ZADD', lru, redis.call('HINCRBY', state, 'sequence', 1), key)
            hits = hits + occurrences
        else
            if raw then remove(key) end
            value, raw = false, false
            misses = misses + occurrences
        end
        reply[#reply + 1], reply[#reply + 2] = raw, value
    end
    if hits > 0 then
        redis.call('HINCRBY', state, 'hits', hits)
        if node ~= '' then redis.call('HINCRBY', nodes, node .. ':hits', hits) end
    end
    if misses > 0 then
        redis.call('HINCRBY', state, 'misses', misses)
        if node ~= '' then redis.call('HINCRBY', nodes, node .. ':misses', misses) end
    end
    return reply
elseif operation == 'store' then
    if enabled == '0' or generation ~= ARGV[7] then return {generation} end
    for i = 8, #ARGV, 3 do
        local key, raw, payload = ARGV[i], ARGV[i + 1], ARGV[i + 2]
        local size = string.len(payload)
        if size <= maxBytes and not redis.call('HGET', metadata, key) then
            local row = cjson.decode(raw)
            row.created_at = tonumber(clock[1])
            row.expires_at = tonumber(clock[1]) + ttl
            fit(size, 1)
            redis.call('SET', prefix .. ':image:' .. key, payload, 'PX', ttlMillis)
            redis.call('HSET', metadata, key, cjson.encode(row))
            redis.call('HSET', ids, row.id, key)
            redis.call('ZADD', lru, redis.call('HINCRBY', state, 'sequence', 1), key)
            redis.call('ZADD', expires, now + ttlMillis, key)
            redis.call('HINCRBY', state, 'used', size)
        end
    end
    return {generation}
elseif operation == 'snapshot' then
    local start, count = tonumber(ARGV[7]), tonumber(ARGV[8])
    local rows = {}
    for _, key in ipairs(redis.call('ZREVRANGE', lru, start, start + count - 1)) do
        local raw = redis.call('HGET', metadata, key)
        if raw then rows[#rows + 1] = raw end
    end
    return {generation, redis.call('HMGET', state, 'used', 'hits', 'misses', 'evictions', 'expirations', 'removals'), redis.call('ZCARD', lru), rows, redis.call('HGETALL', nodes)}
elseif operation == 'remove' then
    if generation ~= ARGV[7] then return {generation, -1} end
    local removed = 0
    if ARGV[8] == '1' then
        removed = clear()
    else
        for i = 9, #ARGV do
            local key = redis.call('HGET', ids, ARGV[i])
            if key then removed = removed + remove(key) end
        end
    end
    if removed > 0 or ARGV[8] == '1' then
        redis.call('HSET', state, 'token', token)
        redis.call('HINCRBY', state, 'removals', removed)
    end
    return {generation, removed}
end
return {generation}
