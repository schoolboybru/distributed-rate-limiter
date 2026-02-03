local key = KEYS[1]
local requested = tonumber(ARGV[1])
local capacity = tonumber(ARGV[2])
local refill_rate = tonumber(ARGV[3])

local time = redis.call("TIME")
local now = tonumber(time[1]) + tonumber(time[2]) / 1000000

local tokens = tonumber(redis.call("HGET", key, "tokens"))
local last_ts = tonumber(redis.call("HGET", key, "ts"))

if tokens == nil then
	tokens = capacity
	last_ts = now
end

local elapsed = now - last_ts
local refill = elapsed * refill_rate
tokens = math.min(capacity, tokens + refill)

if tokens >= requested then
	tokens = tokens - requested
	redis.call("HSET", key, "tokens", tokens, "ts", now)
	return { 1, tokens }
else
	redis.call("HSET", key, "tokens", tokens, "ts", now)
	return { 0, tokens }
end
