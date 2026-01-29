package limiter

import (
	"sync"
	"testing"
	"time"
)

type MockClock struct {
	current time.Time
}

func (m *MockClock) Now() time.Time          { return m.current }
func (m *MockClock) Advance(d time.Duration) { m.current = m.current.Add(d) }

func TestNewTokenBucket_StartsFull(t *testing.T) {
	clock := &MockClock{current: time.Now()}
	bucket := NewTokenBucket(10, 2, clock)

	if bucket.tokens != bucket.capacity {
		t.Error("expected bucket capacity to be 10")
	}
}

func TestAllow_ConsumesTokens(t *testing.T) {
	clock := &MockClock{current: time.Now()}
	bucket := NewTokenBucket(10, 2, clock)

	bucket.Allow(5)

	if bucket.tokens != 5 {
		t.Error("expected bucket to have 5 tokens remaining")
	}
}

func TestAllow_DeniesWhenInsufficient(t *testing.T) {
	clock := &MockClock{current: time.Now()}
	bucket := NewTokenBucket(10, 2, clock)

	bucket.Allow(8)

	if bucket.Allow(6) {
		t.Error("expected to be denied with insufficient token amount")
	}

}

func TestAllow_DeniesWhenExceedsCapacity(t *testing.T) {
	clock := &MockClock{current: time.Now()}
	bucket := NewTokenBucket(10, 2, clock)

	if bucket.Allow(15) != false {
		t.Error("expected to be denied exceeding bucket capacity")
	}
}

func TestRefill_AddsTokensOverTime(t *testing.T) {
	clock := &MockClock{current: time.Now()}
	bucket := NewTokenBucket(10, 2, clock)

	bucket.Allow(10)

	clock.Advance(1 * time.Second)

	if !bucket.Allow(2) {
		t.Error("expected 2 tokens after 1 second")
	}

	if bucket.Allow(1) {
		t.Error("should have 0 tokens remaining")
	}
}

func TestRefill_CapsAtCapacity(t *testing.T) {
	clock := &MockClock{current: time.Now()}
	bucket := NewTokenBucket(10, 2, clock)

	bucket.Allow(5)

	clock.Advance(60 * time.Second)

	if !bucket.Allow(10) {
		t.Error("expected bucket to be full at capacity")
	}
	if bucket.Allow(1) {
		t.Error("expected bucket to be empty after draining capacity")
	}
}

func TestAllow_ConcurrentAccess(t *testing.T) {
	clock := &MockClock{current: time.Now()}
	bucket := NewTokenBucket(100, 2, clock)

	var wg sync.WaitGroup
	numGoroutines := 50

	wg.Add(numGoroutines)
	for range numGoroutines {
		go func() {
			defer wg.Done()
			bucket.Allow(1)
		}()
	}

	wg.Wait()

	if bucket.tokens != 50 {
		t.Errorf("expected 50 tokens remaining, got %f", bucket.tokens)
	}
}
