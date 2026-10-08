package main

import (
	"errors"
	"sync"
	"time"
)

// errStaticCircuitOpen indicates that the static circuit breaker is currently in OPEN state.
var errStaticCircuitOpen = errors.New("static circuit breaker is open")

// state represents the discrete operational state of the static circuit breaker.
type state int

const (
	// stateClosed allows requests through and counts consecutive failures.
	stateClosed state = iota

	// stateHalfOpen permits trial traffic after the cooldown period elapses.
	stateHalfOpen

	// stateOpen sheds incoming traffic fast until the fixed cooldown expires.
	stateOpen
)

// String returns the string representation of the static circuit breaker state.
func (s state) String() string {
	switch s {
	case stateClosed:
		return "CLOSED"
	case stateHalfOpen:
		return "HALF-OPEN"
	case stateOpen:
		return "OPEN"
	default:
		return "UNKNOWN"
	}
}

// staticBreaker implements a traditional, non-adaptive circuit breaker using
// consecutive failure counts and fixed cooldown intervals.
type staticBreaker struct {
	mu    sync.Mutex
	state state

	cooldown         time.Duration
	openUntil        time.Time
	failures         int
	failureThreshold int
}

// newStaticBreaker initializes a static breaker with a fixed cooldown duration and failure threshold.
func newStaticBreaker(cooldown time.Duration, failureThreshold int) *staticBreaker {
	return &staticBreaker{
		state:            stateClosed,
		cooldown:         cooldown,
		failureThreshold: failureThreshold,
	}
}

// allow evaluates if a request can proceed, transitioning from OPEN to HALF-OPEN if cooldown expired.
func (sb *staticBreaker) allow() error {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	now := time.Now()
	if sb.state == stateOpen && now.After(sb.openUntil) {
		sb.state = stateHalfOpen
	}

	switch sb.state {
	case stateClosed:
		return nil

	case stateHalfOpen:
		return nil

	case stateOpen:
		return errStaticCircuitOpen

	default:
		return errStaticCircuitOpen
	}
}

// update records invocation outcome, incrementing consecutive failures or closing the circuit upon success.
func (sb *staticBreaker) update(success bool) {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	now := time.Now()
	switch sb.state {
	case stateClosed:
		if !success {
			sb.failures++

			if sb.failures >= sb.failureThreshold {
				sb.state = stateOpen
				sb.openUntil = now.Add(sb.cooldown)
			}
		} else {
			sb.failures = 0
		}

	case stateHalfOpen:
		if !success {
			sb.state = stateOpen
			sb.openUntil = now.Add(sb.cooldown)
			sb.failures++
		} else {
			sb.state = stateClosed
			sb.failures = 0
		}
	}
}

// State returns the current operational state, evaluating cooldown transitions if applicable.
func (sb *staticBreaker) State() state {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	now := time.Now()
	if sb.state == stateOpen && now.After(sb.openUntil) {
		sb.state = stateHalfOpen
	}
	return sb.state
}
