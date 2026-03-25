package limiter

import (
	"sync"
	"time"

	"github.com/rs/zerolog"
)

type CircuitState int

const (
	CircuitClosed CircuitState = iota
	CircuitOpen
	CircuitHalfOpen
)

type CircuitBreaker struct {
	mu          sync.Mutex
	state       CircuitState
	failures    int
	threshold   int
	timeout     time.Duration
	lastFailure time.Time
	clock       Clock
	logger      zerolog.Logger
}

func NewCircuitBreaker(threshold int, timeout time.Duration, clock Clock) *CircuitBreaker {
	return &CircuitBreaker{
		state:     CircuitClosed,
		threshold: threshold,
		timeout:   timeout,
		clock:     clock,
		logger:    zerolog.Nop(),
	}
}

func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		return true
	case CircuitOpen:
		if cb.clock.Now().Sub(cb.lastFailure) >= cb.timeout {
			cb.state = CircuitHalfOpen
			cb.logger.Info().
				Str("from", circuitStateString(CircuitOpen)).
				Str("to", circuitStateString(CircuitHalfOpen)).
				Msg("circuit breaker state transition")
			return true
		}
		return false
	case CircuitHalfOpen:
		return true
	default:
		return true
	}
}

func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	prevState := cb.state
	cb.failures = 0
	cb.state = CircuitClosed

	if prevState != CircuitClosed {
		cb.logger.Info().
			Str("from", circuitStateString(prevState)).
			Str("to", circuitStateString(CircuitClosed)).
			Msg("circuit breaker state transition")
	}
}

func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	prevState := cb.state
	cb.failures++
	cb.lastFailure = cb.clock.Now()

	if cb.failures >= cb.threshold {
		if cb.state != CircuitOpen {
			cb.state = CircuitOpen
			cb.logger.Warn().
				Str("from", circuitStateString(prevState)).
				Str("to", circuitStateString(CircuitOpen)).
				Int("failures", cb.failures).
				Int("threshold", cb.threshold).
				Msg("circuit breaker state transition")
		}
	}
}

func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	return cb.state
}

func circuitStateString(state CircuitState) string {
	switch state {
	case CircuitClosed:
		return "closed"
	case CircuitOpen:
		return "open"
	case CircuitHalfOpen:
		return "half_open"
	default:
		return "unknown"
	}
}
