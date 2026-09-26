// Package limiter implements an adaptive concurrency limiter based on the gradient
// algorithm and Little's Law to prevent latency degradation and downstream queue saturation.
package limiter

import (
	"errors"
	"math"
	"sync"
	"time"
)

// ErrLimitExceeded is returned when the number of concurrent in-flight requests
// reaches or exceeds the dynamically computed concurrency limit.
var ErrLimitExceeded = errors.New("concurrency limit exceeded")

// Limiter manages dynamic concurrency limits using measured round-trip latencies.
type Limiter struct {
	mu     sync.Mutex
	config Config

	limit             float64
	inFlight          int
	baseRTT           time.Duration
	lastBaseRTTChange time.Time
}

// New creates a new Limiter configured with given config.
func New(config Config) *Limiter {
	config.Normalize()

	return &Limiter{
		limit:             config.InitialLimit,
		lastBaseRTTChange: time.Now(),
		config:            config,
	}
}

// Default creates a new Limiter configured with default config.
func Default() *Limiter {
	return New(DefaultConfig())
}

// Acquire attempts to reserve an execution slot.
// On success, it returns an idempotent release function that must be called once the operation completes.
// If the current concurrency limit is exceeded, it returns ErrLimitExceeded.
func (l *Limiter) Acquire() (func(), error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if float64(l.inFlight) >= l.limit {
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

// Update recalibrates the concurrency limit based on request latency and invocation success.
func (l *Limiter) Update(rttSample time.Duration, success bool) {
	if rttSample <= 0 {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	var newLimit float64

	if !success {
		// Multiplicative decrease upon failure to shed load rapidly.
		newLimit = l.limit * 0.8
	} else {
		// Update baseline RTT only on successful responses to prevent poisoning.
		if now.Sub(l.lastBaseRTTChange) > l.config.BaselineResetInterval || l.baseRTT == 0 {
			l.baseRTT = rttSample
			l.lastBaseRTTChange = now
		} else if rttSample < l.baseRTT {
			l.baseRTT = rttSample
		}

		// gradient = RTT_base / RTT_sample.
		gradient := float64(l.baseRTT) / float64(rttSample)
		gradient = math.Max(0.5, math.Min(1.0, gradient))
		// newLimit = limit * gradient + queueHeadroom
		newLimit = l.limit*gradient + l.config.QueueHeadroom
	}

	// limit = (1 - smoothing) * limit + smoothing * newLimit
	smoothedLimit := (1-l.config.SmoothingFactor)*l.limit + l.config.SmoothingFactor*newLimit
	l.limit = math.Max(l.config.MinLimit, math.Min(l.config.MaxLimit, smoothedLimit))
}

// Limit returns the current calculated concurrency capacity.
func (l *Limiter) Limit() float64 {
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
