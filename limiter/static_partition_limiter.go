package limiter

import (
	"context"
	"errors"
)

type StaticPartitionLimiter struct {
	globalCapacity   float64
	globalRefillRate float64
	clock            Clock
	n                int
	metrics          Metrics
	keyedLimiter     *KeyedLimiter
}

type PartitionOption func(*StaticPartitionLimiter)

func WithClock(clock Clock) PartitionOption {
	return func(sl *StaticPartitionLimiter) {
		sl.clock = clock
	}
}

func WithPartitionMetrics(m Metrics) PartitionOption {
	return func(sl *StaticPartitionLimiter) {
		sl.metrics = m
	}
}

func NewStaticPartitionLimiter(capacity float64, refillRate float64, n int, opts ...PartitionOption) *StaticPartitionLimiter {
	if n < 1 {
		panic("Partition amount cannot be less than 1")
	}

	sl := &StaticPartitionLimiter{
		globalCapacity:   capacity,
		globalRefillRate: refillRate,
		n:                n,
		metrics:          NoopMetrics{},
		clock:            RealClock{},
	}

	for _, opt := range opts {
		opt(sl)
	}

	sl.keyedLimiter = NewKeyedLimiter(capacity/float64(n), refillRate/float64(n), sl.clock)
	return sl
}

func (sl *StaticPartitionLimiter) Allow(key string, tokens int) bool {
	result := sl.keyedLimiter.Allow(key, tokens)

	if result {
		sl.metrics.OnAllow(key)
	} else {
		sl.metrics.OnDeny(key)
	}

	return result
}

func (sl *StaticPartitionLimiter) Wait(ctx context.Context, key string, tokens int) error {
	result := sl.keyedLimiter.Wait(ctx, key, tokens)

	if result == nil {
		sl.metrics.OnAllow(key)
	} else if errors.Is(result, ErrExceedsCapacity) {
		sl.metrics.OnError(key, result)
	} else {
		sl.metrics.OnDeny(key)
	}

	return result
}
