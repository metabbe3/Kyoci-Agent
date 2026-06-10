package gateway

import (
	"errors"
	"sync"
	"time"
)

// State represents the circuit breaker state
type State int

const (
	StateClosed   State = 0 // Circuit is closed, requests flow through
	StateHalfOpen State = 1 // Circuit is half-open, testing if service recovered
	StateOpen     State = 2 // Circuit is open, requests are blocked
)

// String returns string representation of State
func (s State) String() string {
	switch s {
	case StateClosed:
		return "Closed"
	case StateHalfOpen:
		return "HalfOpen"
	case StateOpen:
		return "Open"
	default:
		return "Unknown"
	}
}

// ErrCircuitOpen is returned when the circuit breaker is open
var ErrCircuitOpen = errors.New("circuit breaker is open")

// CircuitBreaker implements a thread-safe circuit breaker pattern
type CircuitBreaker struct {
	name              string
	state             State
	failures          int
	successes         int
	failureThreshold  int           // default 5
	successThreshold  int           // default 3
	timeout           time.Duration // default 30s (time to try again after opening)
	lastFailure       time.Time
	mu                sync.Mutex
	onStateChange     func(name string, from, to State)
}

// CBOption is a functional option for configuring CircuitBreaker
type CBOption func(*CircuitBreaker)

// WithFailureThreshold sets the failure threshold
func WithFailureThreshold(n int) CBOption {
	return func(cb *CircuitBreaker) {
		if n > 0 {
			cb.failureThreshold = n
		}
	}
}

// WithSuccessThreshold sets the success threshold for half-open state
func WithSuccessThreshold(n int) CBOption {
	return func(cb *CircuitBreaker) {
		if n > 0 {
			cb.successThreshold = n
		}
	}
}

// WithTimeout sets the timeout before attempting to close an open circuit
func WithTimeout(d time.Duration) CBOption {
	return func(cb *CircuitBreaker) {
		if d > 0 {
			cb.timeout = d
		}
	}
}

// WithStateChangeCallback sets a callback for state transitions
func WithStateChangeCallback(fn func(name string, from, to State)) CBOption {
	return func(cb *CircuitBreaker) {
		cb.onStateChange = fn
	}
}

// NewCircuitBreaker creates a new circuit breaker with default values
func NewCircuitBreaker(name string, opts ...CBOption) *CircuitBreaker {
	cb := &CircuitBreaker{
		name:             name,
		state:            StateClosed,
		failureThreshold: 5,
		successThreshold: 3,
		timeout:          30 * time.Second,
	}

	for _, opt := range opts {
		opt(cb)
	}

	return cb
}

// Execute runs the given function, applying circuit breaker logic
func (cb *CircuitBreaker) Execute(fn func() (interface{}, error)) (interface{}, error) {
	cb.mu.Lock()

	// Check if circuit is open
	if cb.state == StateOpen {
		// Check if timeout has elapsed to try half-open
		if time.Since(cb.lastFailure) < cb.timeout {
			cb.mu.Unlock()
			return nil, ErrCircuitOpen
		}
		// Transition to half-open
		cb.transitionTo(StateHalfOpen)
		cb.mu.Unlock()
		return cb.executeHalfOpen(fn)
	}

	currentState := cb.state
	cb.mu.Unlock()

	if currentState == StateHalfOpen {
		return cb.executeHalfOpen(fn)
	}

	// StateClosed: execute normally
	result, err := fn()
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		cb.failures++
		cb.lastFailure = time.Now()
		if cb.failures >= cb.failureThreshold {
			cb.transitionTo(StateOpen)
		}
		return nil, err
	}

	// Success in closed state - can reset failures optionally
	cb.successes++
	return result, nil
}

// executeHalfOpen handles execution in half-open state
func (cb *CircuitBreaker) executeHalfOpen(fn func() (interface{}, error)) (interface{}, error) {
	result, err := fn()

	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		// Failure in half-open -> open circuit
		cb.failures++
		cb.lastFailure = time.Now()
		cb.transitionTo(StateOpen)
		return nil, err
	}

	// Success in half-open
	cb.successes++
	if cb.successes >= cb.successThreshold {
		// Reached success threshold -> close circuit
		cb.failures = 0
		cb.successes = 0
		cb.transitionTo(StateClosed)
	}
	return result, nil
}

// transitionTo changes the circuit breaker state and calls callback if set
func (cb *CircuitBreaker) transitionTo(to State) {
	if cb.state != to {
		from := cb.state
		cb.state = to
		if cb.onStateChange != nil {
			go cb.onStateChange(cb.name, from, to)
		}
	}
}

// State returns the current state of the circuit breaker
func (cb *CircuitBreaker) State() State {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}

// Stats returns current statistics about the circuit breaker
func (cb *CircuitBreaker) Stats() (state State, failures, successes int) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state, cb.failures, cb.successes
}

// Name returns the circuit breaker name
func (cb *CircuitBreaker) Name() string {
	return cb.name
}

// Reset manually resets the circuit breaker to closed state
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.state = StateClosed
	cb.failures = 0
	cb.successes = 0
}

// RecordFailure manually records a failure without executing a function
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures++
	cb.lastFailure = time.Now()
	if cb.failures >= cb.failureThreshold {
		cb.transitionTo(StateOpen)
	}
}

// RecordSuccess manually records a success without executing a function
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.successes++
}

// ForceOpen forces the circuit breaker to open state
func (cb *CircuitBreaker) ForceOpen() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if cb.state != StateOpen {
		cb.transitionTo(StateOpen)
		cb.lastFailure = time.Now()
	}
}

// ForceClosed forces the circuit breaker to closed state
func (cb *CircuitBreaker) ForceClosed() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if cb.state != StateClosed {
		cb.failures = 0
		cb.successes = 0
		cb.transitionTo(StateClosed)
	}
}