// Package breaker implements an adaptive, three-state circuit breaker with
// statistical failure tracking, exponential backoff, and trial probe rate-limiting.
package breaker

import (
	"errors"
	"math"
	"math/rand/v2"
	"sync"
	"time"
)

// ErrCircuitOpen indicates that the circuit is currently open or probe capacity is exhausted.
var ErrCircuitOpen = errors.New("circuit breaker is open")

// State represents the operational state of the circuit breaker.
type State int

const (
	// StateClosed allows requests through and monitors failure rates.
	StateClosed State = iota

	// StateHalfOpen permits a limited number of trial probes to test downstream recovery.
	StateHalfOpen

	// StateOpen sheds incoming traffic fast without reaching the network.
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

// Breaker coordinates state transitions and request admission based on downstream health telemetry.
type Breaker struct {
	mu                   sync.Mutex
	state                State
	window               *Window
	minRequests          int
	failureRateThreshold float64
	baseCooldown         time.Duration
	maxCooldown          time.Duration
	maxProbes            int

	consecutiveTrips int
	openUntil        time.Time
	inFlightProbes   int
	successfulProbes int
	lastStateChange  time.Time
}

// New creates a new Breaker configured with given settings.
func New(minRequests int, failureRateThreshold float64, baseCooldown, maxCooldown time.Duration,
	maxProbes int, windowDuration time.Duration, windowBuckets int) *Breaker {
	return &Breaker{
		state:                StateClosed,
		lastStateChange:      time.Now(),
		minRequests:          minRequests,
		failureRateThreshold: failureRateThreshold,
		baseCooldown:         baseCooldown,
		maxCooldown:          maxCooldown,
		maxProbes:            maxProbes,
		window:               NewWindow(windowDuration, windowBuckets),
	}
}

// Allow determines if an outbound request should proceed.
// Returns nil if allowed, or ErrCircuitOpen if rejected.
func (b *Breaker) Allow() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()

	if b.state == StateOpen && now.After(b.openUntil) {
		b.toHalfOpen(now)
	}

	switch b.state {
	case StateClosed:
		return nil

	case StateHalfOpen:
		if b.inFlightProbes < b.maxProbes {
			b.inFlightProbes++
			return nil
		}
		return ErrCircuitOpen

	case StateOpen:
		return ErrCircuitOpen

	default:
		return ErrCircuitOpen
	}
}

// RollbackProbe releases an allocated probe slot if the request was aborted
// before reaching the downstream dependency (e.g., dropped by a concurrency limiter).
func (b *Breaker) RollbackProbe() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.state == StateHalfOpen && b.inFlightProbes > 0 {
		b.inFlightProbes--
	}
}

// Update recalculates circuit metrics and transitions breaker state based on invocation outcome.
func (b *Breaker) Update(success bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()

	switch b.state {
	case StateClosed:
		b.window.Update(success)
		totalRequests, _, failureRate := b.window.Summary()

		if totalRequests >= b.minRequests && failureRate >= b.failureRateThreshold {
			b.toOpen(now)
		}

	case StateHalfOpen:
		if !success {
			b.toOpen(now)
		} else {
			b.successfulProbes++
			if b.successfulProbes >= b.maxProbes {
				b.toClosed(now)
			}
		}
	}
}

// State returns the current operational state, evaluating cooldown transitions if applicable.
func (b *Breaker) State() State {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	if b.state == StateOpen && now.After(b.openUntil) {
		b.toHalfOpen(now)
	}
	return b.state
}

// toClosed transitions the circuit to the CLOSED state, resetting metrics and probe counters.
func (b *Breaker) toClosed(now time.Time) {
	b.state = StateClosed
	b.lastStateChange = now
	b.inFlightProbes = 0
	b.successfulProbes = 0
	b.consecutiveTrips = 0
	b.window.Reset()
}

// toHalfOpen transitions the circuit to the HALF-OPEN state to begin trial probing.
func (b *Breaker) toHalfOpen(now time.Time) {
	b.state = StateHalfOpen
	b.lastStateChange = now
	b.inFlightProbes = 0
	b.successfulProbes = 0
	b.window.Reset()
}

// toOpen transitions the circuit to the OPEN state and calculates the exponential backoff cooldown.
func (b *Breaker) toOpen(now time.Time) {
	b.state = StateOpen
	b.lastStateChange = now
	b.inFlightProbes = 0
	b.successfulProbes = 0
	b.window.Reset()

	factor := math.Pow(2, float64(b.consecutiveTrips))
	cooldown := float64(b.baseCooldown) * factor
	backoff := min(cooldown, float64(b.maxCooldown))
	//nolint:gosec // G404: weak random generator is safe for calculating retry jitter
	jitter := rand.Float64() * 0.25 * backoff

	b.openUntil = now.Add(time.Duration(backoff + jitter))
	b.consecutiveTrips++
}
