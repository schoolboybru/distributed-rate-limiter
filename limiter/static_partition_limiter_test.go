package limiter

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPartition_Allow_BasicPartition(t *testing.T) {
	clock := &MockClock{current: time.Now()}
	sl := NewStaticPartitionLimiter(10, 1, 2, WithClock(clock))

	sl.Allow("key1", 1)
	sl.Allow("key1", 1)
	sl.Allow("key1", 1)
	sl.Allow("key1", 1)
	sl.Allow("key1", 1)

	if sl.Allow("key1", 1) {
		t.Error("Expect 6th request to be denied")
	}
}

func TestPartition_Allow_SinglePartition(t *testing.T) {
	clock := &MockClock{current: time.Now()}
	sl := NewStaticPartitionLimiter(10, 1, 1, WithClock(clock))

	for range 10 {
		sl.Allow("key1", 1)
	}

	if sl.Allow("key1", 1) {
		t.Error("Expect 11th request to be denied")
	}
}

func TestPartition_Allow_SeparateKeys(t *testing.T) {
	clock := &MockClock{current: time.Now()}
	sl := NewStaticPartitionLimiter(10, 1, 2, WithClock(clock))

	for range 5 {
		sl.Allow("keyA", 1)
		sl.Allow("keyB", 1)
	}

	if sl.Allow("keyA", 1) {
		t.Error("Expected 6th request for keyA to be denied")
	}

	if sl.Allow("keyB", 1) {
		t.Error("Expected 6th request for keyB to be denied")
	}
}

func TestPartition_Allow_MultipleInstances(t *testing.T) {
	clock := &MockClock{current: time.Now()}
	sl1 := NewStaticPartitionLimiter(10, 1, 2, WithClock(clock))
	sl2 := NewStaticPartitionLimiter(10, 1, 2, WithClock(clock))

	allowed1 := 0
	for range 10 {
		if sl1.Allow("keyA", 1) {
			allowed1++
		}
	}

	allowed2 := 0
	for range 10 {
		if sl2.Allow("keyA", 1) {
			allowed2++
		}
	}

	if allowed1 != 5 {
		t.Errorf("Expected allowed1 to be 5, got %d", allowed1)
	}

	if allowed2 != 5 {
		t.Errorf("Expected allowed2 to be 5, got %d", allowed2)
	}

	if allowed1+allowed2 != 10 {
		t.Errorf("Expected allowed1 + allowed2 to be 10, got %d", allowed1+allowed2)
	}
}

func TestPartition_Allow_FractionalPartition(t *testing.T) {
	clock := &MockClock{current: time.Now()}
	sl := NewStaticPartitionLimiter(10, 1, 3, WithClock(clock))

	sl.Allow("keyA", 1)
	sl.Allow("keyA", 1)
	sl.Allow("keyA", 1)

	if sl.Allow("keyA", 1) {
		t.Error("Expected 4th request for keyA to be denied")
	}
}

func TestPartition_Wait_SuccessAfterRefill(t *testing.T) {
	clock := &MockClock{current: time.Now()}
	sl := NewStaticPartitionLimiter(10, 1, 2, WithClock(clock))

	sl.Allow("keyA", 5)

	clock.Advance(2 * time.Second)

	err := sl.Wait(context.Background(), "keyA", 1)

	if err != nil {
		t.Errorf("Expected success after refil, got error: %s", err)
	}
}

func TestPartition_Wait_ContextCancellation(t *testing.T) {
	clock := &MockClock{current: time.Now()}
	sl := NewStaticPartitionLimiter(10, 1, 2, WithClock(clock))

	sl.Allow("keyA", 5)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)

	defer cancel()

	err := sl.Wait(ctx, "keyA", 2)

	if err != context.DeadlineExceeded {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}
}

func TestPartition_Wait_ExceedsCapacity(t *testing.T) {
	clock := &MockClock{current: time.Now()}
	sl := NewStaticPartitionLimiter(10, 1, 2, WithClock(clock))

	err := sl.Wait(context.Background(), "keyA", 6)

	if err != ErrExceedsCapacity {
		t.Errorf("expected ErrExceedsCapacity, got %v", err)
	}
}

func TestPartition_Allow_Concurrent(t *testing.T) {
	clock := &MockClock{current: time.Now()}
	sl := NewStaticPartitionLimiter(10, 1, 2, WithClock(clock))

	var wg sync.WaitGroup
	numGoroutines := 20
	wg.Add(numGoroutines)

	var count atomic.Int32
	for range numGoroutines {
		go func() {
			defer wg.Done()
			if sl.Allow("keyA", 1) {
				count.Add(1)
			}
		}()
	}

	wg.Wait()

	if count.Load() > 5 {
		t.Errorf("expected 5 goroutines to be allowed, got %d", count.Load())
	}
}

func TestPartition_PanicsOnInvalidN(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic")
		}
	}()
	NewStaticPartitionLimiter(10, 1, 0)
}
