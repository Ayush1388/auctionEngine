package ratelimit

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// tokenBucketScript is the same algorithm as Memory.Allow, run inside
// Redis so that every API instance shares one bucket per client.
//
// Why a Lua script: the steps "read tokens, refill, take one, write back"
// must be atomic. Done as separate commands from Go, two instances could
// both read "1 token left" and both allow a request. Redis runs a script
// start to finish without interleaving other commands, so it's the same
// read-modify-write race fixed by a lock in Memory, fixed here by Redis
// being single-threaded per script.
//
// The clock is Redis's own TIME, not the caller's. Instances whose clocks
// disagree by a few hundred milliseconds would otherwise refill buckets
// inconsistently.
//
//	KEYS[1] bucket key
//	ARGV[1] rate (tokens per second), ARGV[2] burst, ARGV[3] key TTL in ms
//	returns {allowed (0/1), remaining (int), retry_after seconds (string)}
var tokenBucketScript = redis.NewScript(`
local t = redis.call("TIME")
local now = tonumber(t[1]) + tonumber(t[2]) / 1000000
local rate = tonumber(ARGV[1])
local burst = tonumber(ARGV[2])

local data = redis.call("HMGET", KEYS[1], "tokens", "ts")
local tokens = tonumber(data[1])
local ts = tonumber(data[2])
if tokens == nil then
	tokens = burst
	ts = now
end

tokens = math.min(burst, tokens + math.max(0, now - ts) * rate)

local allowed = 0
local retry = 0
if tokens >= 1 then
	tokens = tokens - 1
	allowed = 1
else
	retry = (1 - tokens) / rate
end

redis.call("HSET", KEYS[1], "tokens", tokens, "ts", now)
redis.call("PEXPIRE", KEYS[1], ARGV[3])
return {allowed, math.floor(tokens), tostring(retry)}
`)

// Redis is a Limiter whose buckets live in Redis. Keys expire once a bucket
// would be full again, so idle clients cost nothing (no janitor needed).
type Redis struct {
	rdb    *redis.Client
	prefix string
}

func NewRedis(rdb *redis.Client, prefix string) *Redis {
	return &Redis{rdb: rdb, prefix: prefix}
}

func (l *Redis) Allow(ctx context.Context, rule Rule, key string) (Decision, error) {
	// Time for an empty bucket to refill completely, plus a second.
	ttl := time.Duration(float64(rule.Burst)/rule.Rate*float64(time.Second)) + time.Second

	res, err := tokenBucketScript.Run(ctx, l.rdb,
		[]string{l.prefix + "rl:" + rule.Name + ":" + key},
		rule.Rate, rule.Burst, ttl.Milliseconds(),
	).Slice()
	if err != nil {
		return Decision{}, fmt.Errorf("rate limit script: %w", err)
	}
	if len(res) != 3 {
		return Decision{}, fmt.Errorf("rate limit script: unexpected reply %v", res)
	}

	allowed, _ := res[0].(int64)
	remaining, _ := res[1].(int64)
	retryStr, _ := res[2].(string)
	retry, _ := strconv.ParseFloat(retryStr, 64)

	return Decision{
		Allowed:    allowed == 1,
		Limit:      rule.Burst,
		Remaining:  int(remaining),
		RetryAfter: time.Duration(math.Ceil(retry*1000)) * time.Millisecond,
	}, nil
}
