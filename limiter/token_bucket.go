package limiter

import (
	"context"
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

// Wait blocks until the requested tokens are available or the context is cancelled.
// Returns ErrExceedsCapacity if requested tokens exceed bucket capacity.
// Returns ctx.Err() if context is cancelled or times out while waiting.
func (tb *TokenBucket) Wait(ctx context.Context, requested int) error {
	if float64(requested) > tb.capacity {
		return ErrExceedsCapacity
	}

	for {
		tb.mu.Lock()

		tb.refill()
		if tb.tokens >= float64(requested) {
			tb.tokens -= float64(requested)
			tb.mu.Unlock()
			return nil
		}

		waitDuration := tb.timeUntilAvailable(requested)
		tb.mu.Unlock()

		timer := time.NewTimer(waitDuration)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
			// Continue loop to try again
		}
	}
}

// timeUntilAvailable calculates the duration until the requested tokens are available
// Must be called with tb.mu held.
func (tb *TokenBucket) timeUntilAvailable(requested int) time.Duration {
	tb.refill()

	deficit := float64(requested) - tb.tokens

	if deficit <= 0 {
		return 0
	}

	seconds := deficit / tb.refillRate
	return time.Duration(seconds * float64(time.Second))
}
