package limiter

import (
	"context"
	"sync"
)

type KeyedLimiter struct {
	mu         sync.RWMutex
	buckets    map[string]*TokenBucket
	capacity   float64
	refillRate float64
	clock      Clock
}

func NewKeyedLimiter(capacity float64, refillRate float64, clock Clock) *KeyedLimiter {
	return &KeyedLimiter{
		capacity:   capacity,
		refillRate: refillRate,
		clock:      clock,
		buckets:    make(map[string]*TokenBucket),
	}
}

func (kl *KeyedLimiter) Allow(key string, tokens int) bool {
	bucket := kl.getOrCreateBucket(key)

	return bucket.Allow(tokens)

}

func (kl *KeyedLimiter) Wait(ctx context.Context, key string, tokens int) error {
	bucket := kl.getOrCreateBucket(key)

	return bucket.Wait(ctx, tokens)
}

func (kl *KeyedLimiter) getOrCreateBucket(key string) *TokenBucket {
	kl.mu.RLock()
	if value, ok := kl.buckets[key]; ok {
		kl.mu.RUnlock()
		return value
	}

	kl.mu.RUnlock()
	kl.mu.Lock()

	if value, ok := kl.buckets[key]; ok {
		kl.mu.Unlock()
		return value
	}

	bucket := NewTokenBucket(kl.capacity, kl.refillRate, kl.clock)

	kl.buckets[key] = bucket

	kl.mu.Unlock()

	return bucket

}
