package limiter

import (
	"context"
	_ "embed"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
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
	logger         zerolog.Logger
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

func WithLogger(logger zerolog.Logger) Option {
	return func(r *RedisLimiter) {
		r.logger = logger
	}
}

func WithCircuitBreaker(threshold int, timeout time.Duration) Option {
	return func(r *RedisLimiter) {
		r.circuitBreaker = NewCircuitBreaker(threshold, timeout, RealClock{})
		r.circuitBreaker.logger = r.logger.With().Str("component", "circuit_breaker").Logger()
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
		logger:      zerolog.Nop(),
		failureMode: FailOpen,
	}

	for _, opt := range opts {
		opt(r)
	}

	if r.circuitBreaker != nil {
		r.circuitBreaker.logger = r.logger.With().Str("component", "circuit_breaker").Logger()
	}

	if r.failureMode == FailDegrade {
		r.localLimiter = NewKeyedLimiter(capacity, refillRate, RealClock{})
	}

	return r
}

func (r *RedisLimiter) Allow(key string, tokens int) bool {
	if r.circuitBreaker != nil && !r.circuitBreaker.Allow() {
		r.logger.Warn().
			Str("key", key).
			Msg("circuit breaker open; using failure mode fallback")
		r.metrics.OnError(key, ErrCircuitOpen)
		return r.handleFailure(key, tokens)
	}

	start := time.Now()

	result, err := r.script.Run(context.Background(), r.client, []string{r.keyPrefix + key}, tokens, r.capacity, r.refillRate).Result()

	r.metrics.OnLatency(key, time.Since(start))

	if err != nil {
		r.logger.Warn().
			Str("key", key).
			Err(err).
			Msg("redis script execution failed; using failure mode fallback")
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
	failureMode := failureModeString(r.failureMode)

	switch r.failureMode {
	case FailOpen:
		r.logger.Debug().Str("key", key).Str("failure_mode", failureMode).Msg("failure mode applied")
		r.metrics.OnAllow(key)
		return true
	case FailClosed:
		r.logger.Debug().Str("key", key).Str("failure_mode", failureMode).Msg("failure mode applied")
		r.metrics.OnDeny(key)
		return false
	case FailDegrade:
		allowed := r.localLimiter.Allow(key, tokens)
		r.logger.Debug().
			Str("key", key).
			Str("failure_mode", failureMode).
			Bool("allowed", allowed).
			Msg("failure mode applied")
		if allowed {
			r.metrics.OnAllow(key)
		} else {
			r.metrics.OnDeny(key)
		}
		return allowed
	default:
		r.logger.Debug().Str("key", key).Str("failure_mode", "unknown").Msg("unknown failure mode; defaulting to allow")
		return true
	}
}

func failureModeString(mode FailureMode) string {
	switch mode {
	case FailOpen:
		return "fail_open"
	case FailClosed:
		return "fail_closed"
	case FailDegrade:
		return "fail_degrade"
	default:
		return "unknown"
	}
}
