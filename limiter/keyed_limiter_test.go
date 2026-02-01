package limiter

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestKeyedLimiter_SeparateBuckets(t *testing.T) {
	clock := &MockClock{current: time.Now()}
	keyedLimiter := NewKeyedLimiter(5, 2, clock)

	keyedLimiter.Allow("user-1", 5)

	if keyedLimiter.Allow("user-1", 1) {
		t.Errorf("expected allow to return false for user-1")
	}

	if !keyedLimiter.Allow("user-2", 5) {
		t.Errorf("expected allow to return true for user-2")
	}
}

func TestKeyedLimiter_SameKeySharesBucket(t *testing.T) {
	clock := &MockClock{current: time.Now()}
	keyedLimiter := NewKeyedLimiter(10, 2, clock)

	if !keyedLimiter.Allow("user-1", 3) {
		t.Errorf("expected allow to return true for user-1")
	}
	if !keyedLimiter.Allow("user-1", 3) {
		t.Errorf("expected allow to return true for user-1 again")
	}
	if keyedLimiter.Allow("user-1", 5) {
		t.Error("expected allow to return false for user-1")
	}
}

func TestKeyedLimiter_Wait(t *testing.T) {
	clock := &MockClock{current: time.Now()}
	keyedLimiter := NewKeyedLimiter(10, 2, clock)

	keyedLimiter.Allow("user-1", 10)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := keyedLimiter.Wait(ctx, "user-1", 2)

	if err != context.DeadlineExceeded {
		t.Errorf("expected error to be DeadlineExceeded, got %v", err)
	}
}

func TestKeyedLimiter_ConcurrentAccess(t *testing.T) {
	clock := &MockClock{current: time.Now()}
	keyedLimiter := NewKeyedLimiter(10, 1000, clock)

	var wg sync.WaitGroup
	num := 5
	results := make(chan bool, num*2)

	wg.Add(num)
	for i := range num {
		go func() {
			defer wg.Done()
			for range 2 {
				user := fmt.Sprintf("user-%d", i)
				results <- keyedLimiter.Allow(user, 2)
			}
		}()
	}

	wg.Wait()
	close(results)

	for res := range results {
		if !res {
			t.Error("expected all allow to return true")
		}
	}
}

func TestKeyedLimiter_ConcurrentSameKey(t *testing.T) {
	clock := &MockClock{current: time.Now()}
	keyedLimiter := NewKeyedLimiter(100, 1000, clock)

	var wg sync.WaitGroup
	num := 50
	results := make(chan bool, num)

	wg.Add(num)
	for range num {
		go func() {
			defer wg.Done()
			results <- keyedLimiter.Allow("same-key", 1)
		}()
	}

	wg.Wait()
	close(results)

	if keyedLimiter.buckets["same-key"].tokens != 50 {
		t.Errorf("expected same-key bucket to have 50 tokens, go %f", keyedLimiter.buckets["same-key"].tokens)
	}
}
