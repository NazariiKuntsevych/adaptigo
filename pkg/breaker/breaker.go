// Package breaker implements a basic, three-state circuit breaker
// with consecutive failure tracking and fixed cooldown recovery.
package breaker

import (
	"errors"
	"sync"
	"time"
)

const (
	// defaultFailureThreshold is the number of consecutive failures needed to trip the breaker.
	defaultFailureThreshold = 5

	// defaultSuccessThreshold is the number of consecutive successful probes required to close the breaker.
	defaultSuccessThreshold = 2

	// defaultCooldown is the sleep duration before attempting recovery in half-open state.
	defaultCooldown = 5 * time.Second
)

// ErrCircuitOpen indicates that the circuit is currently open and requests are blocked.
var ErrCircuitOpen = errors.New("circuit breaker is open")

// State represents the operational state of the circuit breaker.
type State int

const (
	// StateClosed allows requests through and counts consecutive failures.
	StateClosed State = iota

	// StateHalfOpen permits trial requests to test downstream service recovery.
	StateHalfOpen

	// StateOpen fails requests fast without touching the network.
	StateOpen
)

// String returns the human-readable representation of the circuit state.
func (s State) String() string {
	switch s {
	case StateClosed:
		return "CLOSED"
	case StateHalfOpen:
		return "HALF-OPEN"
	case StateOpen:
		return "OPEN"
	default:
		return "UNKNOWN"
	}
}

// Breaker coordinates state transitions based on consecutive call outcomes.
type Breaker struct {
	mu        sync.Mutex
	state     State
	failures  int
	successes int
	openUntil time.Time
}

// New creates a new Breaker initialized in the CLOSED state.
func New() *Breaker {
	return &Breaker{
		state: StateClosed,
	}
}

// Allow determines if an outbound request should proceed.
// Returns nil if allowed, or ErrCircuitOpen if rejected.
func (b *Breaker) Allow() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()

	if b.state == StateOpen && now.After(b.openUntil) {
		b.toHalfOpen()
	}

	switch b.state {
	case StateClosed, StateHalfOpen:
		return nil
	case StateOpen:
		return ErrCircuitOpen
	default:
		return ErrCircuitOpen
	}
}

// Update registers the outcome of a request and executes state transitions.
func (b *Breaker) Update(success bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()

	switch b.state {
	case StateClosed:
		if success {
			b.failures = 0
		} else {
			b.failures++
			if b.failures >= defaultFailureThreshold {
				b.toOpen(now)
			}
		}

	case StateHalfOpen:
		if !success {
			b.toOpen(now)
		} else {
			b.successes++
			if b.successes >= defaultSuccessThreshold {
				b.toClosed()
			}
		}
	}
}

// State returns the current operational state, evaluating cooldown transitions if applicable.
func (b *Breaker) State() State {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.state == StateOpen && time.Now().After(b.openUntil) {
		b.toHalfOpen()
	}
	return b.state
}

// toClosed transitions the circuit to CLOSED and resets tracking counters.
func (b *Breaker) toClosed() {
	b.state = StateClosed
	b.failures = 0
	b.successes = 0
}

// toHalfOpen transitions the circuit to HALF-OPEN to probe service recovery.
func (b *Breaker) toHalfOpen() {
	b.state = StateHalfOpen
	b.failures = 0
	b.successes = 0
}

// toOpen transitions the circuit to OPEN and schedules the cooldown window.
func (b *Breaker) toOpen(now time.Time) {
	b.state = StateOpen
	b.failures = 0
	b.successes = 0
	b.openUntil = now.Add(defaultCooldown)
}
