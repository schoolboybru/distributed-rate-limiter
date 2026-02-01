package limiter

import (
	"context"
	"sync"
	"testing"
	"time"
)

type MockClock struct {
	mu      sync.Mutex
	current time.Time
}

func (m *MockClock) Now() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.current
}

func (m *MockClock) Advance(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.current = m.current.Add(d)
}

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

func TestWait_ImmediateSuccess(t *testing.T) {
	clock := &MockClock{current: time.Now()}
	bucket := NewTokenBucket(10, 2, clock)

	err := bucket.Wait(context.Background(), 5)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if bucket.tokens != 5 {
		t.Errorf("expected 5 tokens remaining, got %f", bucket.tokens)
	}
}

func TestWait_BlocksUntilAvailable(t *testing.T) {
	clock := &MockClock{current: time.Now()}
	bucket := NewTokenBucket(10, 100, clock)

	bucket.Allow(10)

	done := make(chan error)
	go func() {
		done <- bucket.Wait(context.Background(), 5)
	}()

	time.Sleep(10 * time.Millisecond)
	clock.Advance(100 * time.Millisecond)

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("Wait did not return in time")
	}
}

func TestWait_ReturnsErrExceedsCapacity(t *testing.T) {
	clock := &MockClock{current: time.Now()}
	bucket := NewTokenBucket(10, 2, clock)

	err := bucket.Wait(context.Background(), 15)

	if err != ErrExceedsCapacity {
		t.Errorf("expected ErrExceedsCapacity, got %v", err)
	}
}

func TestWait_ContextCancellation(t *testing.T) {
	clock := &MockClock{current: time.Now()}
	bucket := NewTokenBucket(10, 2, clock)

	bucket.Allow(10)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error)
	go func() {
		done <- bucket.Wait(ctx, 5)
	}()

	time.Sleep(10 * time.Millisecond)

	cancel()

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("Wait did not return after context cancellation")
	}
}

func TestWait_ContextTimeout(t *testing.T) {
	clock := &MockClock{current: time.Now()}
	bucket := NewTokenBucket(10, 2, clock)

	bucket.Allow(10)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := bucket.Wait(ctx, 5)

	if err != context.DeadlineExceeded {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}
}

func TestWait_ConcurrentWaiters(t *testing.T) {
	clock := &MockClock{current: time.Now()}
	bucket := NewTokenBucket(10, 1000, clock)

	bucket.Allow(10)

	var wg sync.WaitGroup
	numWaiters := 5
	errors := make(chan error, numWaiters)

	wg.Add(numWaiters)
	for range numWaiters {
		go func() {
			defer wg.Done()
			errors <- bucket.Wait(context.Background(), 2)
		}()
	}

	go func() {
		for range 10 {
			time.Sleep(5 * time.Millisecond)
			clock.Advance(50 * time.Millisecond)
		}
	}()

	wg.Wait()
	close(errors)

	for err := range errors {
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	}
}
