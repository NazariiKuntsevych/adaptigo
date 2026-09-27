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
	mu         sync.RWMutex
	minTimeout time.Duration
	maxTimeout time.Duration
	alpha      float64
	beta       float64
	k          float64

	initialized bool
	smoothedRTT time.Duration
	rttVar      time.Duration
}

// New creates a new Tracker configured with given params.
func New(minTimeout, maxTimeout time.Duration, alpha, beta, k float64) *Tracker {
	return &Tracker{
		minTimeout: minTimeout,
		maxTimeout: maxTimeout,
		alpha:      alpha,
		beta:       beta,
		k:          k,
	}
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
	t.rttVar = time.Duration((1-t.beta)*float64(t.rttVar) + t.beta*float64(diff))
	// SRTT = (1 - alpha) * SRTT + alpha * RTT_sample
	t.smoothedRTT = time.Duration((1-t.alpha)*float64(t.smoothedRTT) + t.alpha*float64(rttSample))
}

// Timeout returns the current dynamic deadline duration clamped to configured boundaries.
func (t *Tracker) Timeout() time.Duration {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if !t.initialized {
		return t.maxTimeout
	}

	// timeout = SRTT + K * RTTVAR
	timeout := time.Duration(float64(t.smoothedRTT) + t.k*float64(t.rttVar))
	return min(max(t.minTimeout, timeout), t.maxTimeout)
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
