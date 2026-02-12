package limiter

import (
	"testing"
	"time"
)

func TestCircuitBreaker_StartsClose(t *testing.T) {
	clock := &MockClock{current: time.Now()}
	cb := NewCircuitBreaker(3, 30*time.Second, clock)

	if cb.State() != CircuitClosed {
		t.Errorf("expecting state to be CircuitClosed, got %d", cb.State())
	}
}

func TestCircuitBreaker_OpensAfterThreshold(t *testing.T) {
	clock := &MockClock{current: time.Now()}
	cb := NewCircuitBreaker(3, 30*time.Second, clock)

	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()

	if cb.State() != CircuitOpen {
		t.Errorf("expecting state to be CircuitOpen, got %d", cb.State())
	}
}

func TestCircuitBreaker_FailsFastWhenOpen(t *testing.T) {
	clock := &MockClock{current: time.Now()}
	cb := NewCircuitBreaker(3, 30*time.Second, clock)

	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()

	if cb.Allow() {
		t.Error("expecting allow to be false")
	}
}

func TestCircuitBreaker_TransitionsToHalfOpen(t *testing.T) {
	clock := &MockClock{current: time.Now()}
	cb := NewCircuitBreaker(3, 30*time.Second, clock)

	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()
	clock.Advance(35 * time.Second)

	if !cb.Allow() {
		t.Error("expecting allow to be true")
	}

	if cb.State() != CircuitHalfOpen {
		t.Errorf("expecting state to be CircuitHalfOpen, got %d", cb.State())
	}
}

func TestCircuitBreaker_ClosesOnSuccess(t *testing.T) {
	clock := &MockClock{current: time.Now()}
	cb := NewCircuitBreaker(3, 30*time.Second, clock)

	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()
	clock.Advance(35 * time.Second)
	cb.Allow()
	cb.RecordSuccess()

	if cb.State() != CircuitClosed {
		t.Errorf("expecting state to be CircuitClosed, got %d", cb.State())
	}

	if cb.failures != 0 {
		t.Errorf("expecting failures to be 0, got %d", cb.failures)
	}
}

func TestCircuitBreaker_ReopensOnFailureInHalfOpen(t *testing.T) {
	clock := &MockClock{current: time.Now()}
	cb := NewCircuitBreaker(3, 30*time.Second, clock)

	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()
	clock.Advance(35 * time.Second)
	cb.Allow()
	cb.RecordFailure()

	if cb.State() != CircuitOpen {
		t.Errorf("expecting state to be CircuitOpen, got %d", cb.State())
	}
}
