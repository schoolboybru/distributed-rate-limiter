package limiter

import (
	"sync"
	"time"
)

type TokenBucket struct {
	capacity   float64
	refillRate float64
	tokens     float64
	lastRefill time.Time
	clock      Clock
	mu         sync.Mutex
}

func NewTokenBucket(capacity float64, refillRate float64, clock Clock) *TokenBucket {
	return &TokenBucket{
		capacity:   capacity,
		refillRate: refillRate,
		tokens:     capacity,
		lastRefill: clock.Now(),
		clock:      clock,
	}
}

func (tb *TokenBucket) refill() {
	now := tb.clock.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()

	if elapsed > 0 {
		tokensToAdd := elapsed * tb.refillRate
		tb.tokens = min(tb.tokens+tokensToAdd, tb.capacity)
		tb.lastRefill = now
	}
}

func (tb *TokenBucket) Allow(requested int) bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.refill()

	if float64(requested) > tb.capacity {
		return false
	}

	if tb.tokens >= float64(requested) {
		tb.tokens -= float64(requested)
		return true
	}

	return false
}
