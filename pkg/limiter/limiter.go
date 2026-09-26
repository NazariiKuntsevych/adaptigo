// Package limiter provides concurrency limiting mechanisms to prevent downstream service
// saturation by strictly controlling the number of concurrent in-flight executions.
package limiter

import (
	"errors"
	"sync"
)

// ErrLimitExceeded is returned when the number of concurrent in-flight requests
// reaches or exceeds the configured concurrency limit.
var ErrLimitExceeded = errors.New("concurrency limit exceeded")

// Limiter manages fixed concurrency thresholds using in-flight execution counters.
type Limiter struct {
	mu       sync.Mutex
	limit    int

	inFlight int
}

// New creates a new Limiter with the specified maximum concurrency limit.
// If maxConcurrency is less than 1, it falls back to a safe minimum of 1.
func New(limit int) *Limiter {
	if limit < 1 {
		limit = 1
	}

	return &Limiter{
		limit: limit,
	}
}

// Acquire attempts to reserve an execution slot.
// On success, it returns an idempotent release function that must be called once the operation completes.
// If the maximum concurrency limit is reached, it returns ErrLimitExceeded.
func (l *Limiter) Acquire() (func(), error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.inFlight >= l.limit {
		return nil, ErrLimitExceeded
	}

	l.inFlight++

	var once sync.Once
	release := func() {
		once.Do(func() {
			l.mu.Lock()
			defer l.mu.Unlock()
			l.inFlight--
		})
	}
	return release, nil
}

// Limit returns the configured maximum concurrency capacity.
func (l *Limiter) Limit() int {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.limit
}

// InFlight returns the number of active requests currently occupying execution slots.
func (l *Limiter) InFlight() int {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.inFlight
}
