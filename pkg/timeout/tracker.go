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
func (tr *Tracker) Update(rttSample time.Duration) {
	if rttSample <= 0 {
		return
	}

	tr.mu.Lock()
	defer tr.mu.Unlock()

	if !tr.initialized {
		tr.smoothedRTT = rttSample
		tr.rttVar = rttSample / 2
		tr.initialized = true
		return
	}

	// diff = |SRTT - RTT_sample|
	diff := math.Abs(float64(tr.smoothedRTT - rttSample))
	// RTTVAR = (1 - beta) * RTTVAR + beta * diff
	tr.rttVar = time.Duration((1-tr.config.Beta)*float64(tr.rttVar) + tr.config.Beta*float64(diff))
	// SRTT = (1 - alpha) * SRTT + alpha * RTT_sample
	tr.smoothedRTT = time.Duration((1-tr.config.Alpha)*float64(tr.smoothedRTT) + tr.config.Alpha*float64(rttSample))
}

// Timeout returns the current dynamic deadline duration clamped to configured boundaries.
func (tr *Tracker) Timeout() time.Duration {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	if !tr.initialized {
		return tr.config.MaxTimeout
	}

	// timeout = SRTT + K * RTTVAR
	timeout := time.Duration(float64(tr.smoothedRTT) + tr.config.K*float64(tr.rttVar))
	return min(max(tr.config.MinTimeout, timeout), tr.config.MaxTimeout)
}

// SmoothedRTT returns the current SRTT.
func (tr *Tracker) SmoothedRTT() time.Duration {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	return tr.smoothedRTT
}

// RTTVar returns the current RTTVAR.
func (tr *Tracker) RTTVar() time.Duration {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	return tr.rttVar
}
