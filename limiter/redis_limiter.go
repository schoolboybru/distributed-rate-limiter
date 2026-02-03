package limiter

import (
	"context"
	_ "embed"
	"time"

	"github.com/redis/go-redis/v9"
)

//go:embed scripts/token_bucket.lua
var tokenBucketScript string

type RedisLimiter struct {
	client     *redis.Client
	script     *redis.Script
	capacity   float64
	refillRate float64
	keyPrefix  string
}

func NewRedisLimiter(client *redis.Client, capacity float64, refillRate float64, keyPrefix string) *RedisLimiter {
	return &RedisLimiter{
		client:     client,
		script:     redis.NewScript(tokenBucketScript),
		capacity:   capacity,
		refillRate: refillRate,
		keyPrefix:  keyPrefix,
	}
}

func (r *RedisLimiter) Allow(key string, tokens int) bool {
	result, err := r.script.Run(context.Background(), r.client, []string{r.keyPrefix + key}, tokens, r.capacity, r.refillRate).Result()

	if err != nil {
		return false
	}

	resSlice := result.([]interface{})
	allowed := resSlice[0].(int64) == 1

	return allowed

}

func (r *RedisLimiter) Wait(ctx context.Context, key string, tokens int) error {
	if float64(tokens) > r.capacity {
		return ErrExceedsCapacity
	}

	for {
		if r.Allow(key, tokens) {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
}
