// Package timeout provides dynamic request deadline calculation using Jacobson's
// round-trip time estimation algorithm (RFC 6298).
package timeout

import (
	"math"
	"sync"
	"time"
)

// Tracker dynamically computes request timeouts based on observed network latencies.
type Tracker struct {
	mu     sync.RWMutex
	config Config

	initialized bool
	smoothedRTT time.Duration
	rttVar      time.Duration
}

// New creates a new Tracker configured with given config.
func New(config Config) *Tracker {
	config.Normalize()

	return &Tracker{
		config: config,
	}
}

// Default creates a new Tracker configured with default config.
func Default() *Tracker {
	return New(DefaultConfig())
}

// Update records a new latency sample and updates SRTT and RTTVAR calculations.
func (t *Tracker) Update(rttSample time.Duration) {
	if rttSample <= 0 {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.initialized {
		t.smoothedRTT = rttSample
		t.rttVar = rttSample / 2
		t.initialized = true
		return
	}

	// diff = |SRTT - RTT_sample|
	diff := math.Abs(float64(t.smoothedRTT - rttSample))
	// RTTVAR = (1 - beta) * RTTVAR + beta * diff
	t.rttVar = time.Duration((1-t.config.Beta)*float64(t.rttVar) + t.config.Beta*float64(diff))
	// SRTT = (1 - alpha) * SRTT + alpha * RTT_sample
	t.smoothedRTT = time.Duration((1-t.config.Alpha)*float64(t.smoothedRTT) + t.config.Alpha*float64(rttSample))
}

// Timeout returns the current dynamic deadline duration clamped to configured boundaries.
func (t *Tracker) Timeout() time.Duration {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if !t.initialized {
		return t.config.MaxTimeout
	}

	// timeout = SRTT + K * RTTVAR
	timeout := time.Duration(float64(t.smoothedRTT) + t.config.K*float64(t.rttVar))
	return min(max(t.config.MinTimeout, timeout), t.config.MaxTimeout)
}

// SmoothedRTT returns the current SRTT.
func (t *Tracker) SmoothedRTT() time.Duration {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return t.smoothedRTT
}

// RTTVar returns the current RTTVAR.
func (t *Tracker) RTTVar() time.Duration {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return t.rttVar
}
