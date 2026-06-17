package llm

import (
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// ==============================================================================
// Circuit Breaker
// ==============================================================================

// CircuitState represents the state of a circuit breaker.
type CircuitState int

const (
	// CircuitClosed is the normal state where requests pass through.
	CircuitClosed CircuitState = iota
	// CircuitOpen is the state where requests are blocked due to failures.
	CircuitOpen
	// CircuitHalfOpen is the state where a test request is allowed.
	CircuitHalfOpen
)

// String returns a string representation of the circuit state.
func (s CircuitState) String() string {
	switch s {
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

// CircuitBreaker implements the circuit breaker pattern for provider resilience.
type CircuitBreaker struct {
	failureThreshold int
	resetTimeout     time.Duration
	mu               sync.RWMutex
	state            CircuitState
	failures         int
	lastFailureTime  time.Time
	logger           *slog.Logger
}

// NewCircuitBreaker creates a new circuit breaker.
func NewCircuitBreaker(failureThreshold int, resetTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		failureThreshold: failureThreshold,
		resetTimeout:     resetTimeout,
		state:            CircuitClosed,
		failures:         0,
		logger:           slog.Default(),
	}
}

// RecordSuccess records a successful operation and resets the circuit.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == CircuitHalfOpen {
		cb.state = CircuitClosed
		cb.logger.Info("circuit breaker closed after successful test")
	}
	cb.failures = 0
}

// RecordFailure records a failed operation and potentially opens the circuit.
func (cb *CircuitBreaker) RecordFailure() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	cb.lastFailureTime = time.Now()

	if cb.failures >= cb.failureThreshold {
		cb.state = CircuitOpen
		cb.logger.Warn("circuit breaker opened",
			"failures", cb.failures,
			"threshold", cb.failureThreshold)
		return fmt.Errorf("circuit breaker is open")
	}

	return nil
}

// State returns the current circuit state.
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Allow checks if a request should proceed through the circuit.
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		return true
	case CircuitOpen:
		// Check if we should try a half-open state
		if time.Since(cb.lastFailureTime) > cb.resetTimeout {
			cb.state = CircuitHalfOpen
			cb.logger.Info("circuit breaker transitioning to half-open")
			return true
		}
		return false
	case CircuitHalfOpen:
		return true
	default:
		return false
	}
}

// Reset manually resets the circuit breaker to closed state.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.state = CircuitClosed
	cb.failures = 0
	cb.logger.Info("circuit breaker manually reset")
}
