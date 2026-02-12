package limiter

import (
	"context"
	_ "embed"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

//go:embed scripts/token_bucket.lua
var tokenBucketScript string

var ErrCircuitOpen = errors.New("circuit breaker is open")

type FailureMode int

const (
	FailOpen FailureMode = iota
	FailClosed
	FailDegrade
)

type RedisLimiter struct {
	client         *redis.Client
	script         *redis.Script
	capacity       float64
	refillRate     float64
	keyPrefix      string
	metrics        Metrics
	failureMode    FailureMode
	localLimiter   *KeyedLimiter
	circuitBreaker *CircuitBreaker
}

type Option func(*RedisLimiter)

func WithMetrics(m Metrics) Option {
	return func(r *RedisLimiter) {
		r.metrics = m
	}
}

func WithFailureMode(mode FailureMode) Option {
	return func(r *RedisLimiter) {
		r.failureMode = mode
	}
}

func WithCircuitBreaker(threshold int, timeout time.Duration) Option {
	return func(r *RedisLimiter) {
		r.circuitBreaker = NewCircuitBreaker(threshold, timeout, RealClock{})
	}
}

func NewRedisLimiter(client *redis.Client, capacity float64, refillRate float64, keyPrefix string, opts ...Option) *RedisLimiter {
	r := &RedisLimiter{
		client:      client,
		script:      redis.NewScript(tokenBucketScript),
		capacity:    capacity,
		refillRate:  refillRate,
		keyPrefix:   keyPrefix,
		metrics:     NoopMetrics{},
		failureMode: FailOpen,
	}

	for _, opt := range opts {
		opt(r)
	}

	if r.failureMode == FailDegrade {
		r.localLimiter = NewKeyedLimiter(capacity, refillRate, RealClock{})
	}

	return r
}

func (r *RedisLimiter) Allow(key string, tokens int) bool {
	if r.circuitBreaker != nil && !r.circuitBreaker.Allow() {
		r.metrics.OnError(key, ErrCircuitOpen)
		return r.handleFailure(key, tokens)
	}

	start := time.Now()

	result, err := r.script.Run(context.Background(), r.client, []string{r.keyPrefix + key}, tokens, r.capacity, r.refillRate).Result()

	r.metrics.OnLatency(key, time.Since(start))

	if err != nil {
		if r.circuitBreaker != nil {
			r.circuitBreaker.RecordFailure()
		}
		r.metrics.OnError(key, err)
		return r.handleFailure(key, tokens)
	}

	if r.circuitBreaker != nil {
		r.circuitBreaker.RecordSuccess()
	}

	resSlice := result.([]interface{})
	allowed := resSlice[0].(int64) == 1

	if allowed {
		r.metrics.OnAllow(key)
	} else {
		r.metrics.OnDeny(key)
	}

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

func (r *RedisLimiter) handleFailure(key string, tokens int) bool {
	switch r.failureMode {
	case FailOpen:
		r.metrics.OnAllow(key)
		return true
	case FailClosed:
		r.metrics.OnDeny(key)
		return false
	case FailDegrade:
		allowed := r.localLimiter.Allow(key, tokens)
		if allowed {
			r.metrics.OnAllow(key)
		} else {
			r.metrics.OnDeny(key)
		}
		return allowed
	default:
		return true
	}
}
