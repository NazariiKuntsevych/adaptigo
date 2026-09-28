package breaker_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"adaptigo/pkg/breaker"
)

func TestState_String(t *testing.T) {
	tests := []struct {
		name string
		in   breaker.State
		want string
	}{
		{name: "closed", in: breaker.StateClosed, want: "CLOSED"},
		{name: "half-open", in: breaker.StateHalfOpen, want: "HALF-OPEN"},
		{name: "open", in: breaker.StateOpen, want: "OPEN"},
		{name: "unknown value", in: breaker.State(99), want: "UNKNOWN"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.in.String(); got != tc.want {
				t.Errorf("State(%d).String() = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestBreaker_InitialState(t *testing.T) {
	b := breaker.Default()

	if got := b.State(); got != breaker.StateClosed {
		t.Errorf("State() = %v, want %v", got, breaker.StateClosed)
	}

	if err := b.Allow(); err != nil {
		t.Errorf("Allow() = %v, want nil", err)
	}
}

func TestBreaker_TripToOpen(t *testing.T) {
	config := breaker.Config{
		MinRequests:          4,
		FailureRateThreshold: 0.5,
		BaseCooldown:         100 * time.Millisecond,
		WindowDuration:       1 * time.Second,
		WindowBuckets:        5,
	}
	b := breaker.New(config)

	// 2 successes, 1 failure -> below threshold (33% failure rate)
	b.Update(true)
	b.Update(true)
	b.Update(false)

	if got := b.State(); got != breaker.StateClosed {
		t.Fatalf("State() = %v, want %v", got, breaker.StateClosed)
	}

	// 2nd failure -> 2/4 = 50% failure rate with MinRequests reached
	b.Update(false)

	if got := b.State(); got != breaker.StateOpen {
		t.Fatalf("State() = %v, want %v", got, breaker.StateOpen)
	}

	if err := b.Allow(); !errors.Is(err, breaker.ErrCircuitOpen) {
		t.Errorf("Allow() = %v, want %v", err, breaker.ErrCircuitOpen)
	}
}

func TestBreaker_OpenToHalfOpenAndRecovery(t *testing.T) {
	config := breaker.Config{
		MinRequests:          2,
		FailureRateThreshold: 0.5,
		BaseCooldown:         30 * time.Millisecond,
		MaxProbes:            3,
		WindowDuration:       1 * time.Second,
		WindowBuckets:        5,
	}
	b := breaker.New(config)

	// Trip breaker to Open
	b.Update(false)
	b.Update(false)

	if got := b.State(); got != breaker.StateOpen {
		t.Fatalf("State() = %v, want %v", got, breaker.StateOpen)
	}

	// Wait past BaseCooldown + jitter (max 30ms + 7.5ms = 37.5ms)
	time.Sleep(50 * time.Millisecond)

	// State() should observe timeout expiration and transition to Half-Open
	if got := b.State(); got != breaker.StateHalfOpen {
		t.Fatalf("State() = %v, want %v", got, breaker.StateHalfOpen)
	}

	// Probe capacity allows strictly MaxProbes (3) requests
	for range 3 {
		if err := b.Allow(); err != nil {
			t.Fatalf("Allow() = %v, want nil", err)
		}
	}

	// 4th probe request must be rejected
	if err := b.Allow(); !errors.Is(err, breaker.ErrCircuitOpen) {
		t.Errorf("Allow() = %v, want %v", err, breaker.ErrCircuitOpen)
	}

	// Record 3 consecutive successful probe responses
	for range 3 {
		b.Update(true)
	}

	// Breaker should fully recover to Closed
	if got := b.State(); got != breaker.StateClosed {
		t.Errorf("State() = %v, want %v", got, breaker.StateClosed)
	}

	if err := b.Allow(); err != nil {
		t.Errorf("Allow() = %v, want nil", err)
	}
}

func TestBreaker_HalfOpenFailureTrip(t *testing.T) {
	config := breaker.Config{
		MinRequests:          2,
		FailureRateThreshold: 0.5,
		BaseCooldown:         25 * time.Millisecond,
		MaxProbes:            3,
		WindowDuration:       1 * time.Second,
		WindowBuckets:        5,
	}
	b := breaker.New(config)

	// Trip to Open
	b.Update(false)
	b.Update(false)

	// Wait past cooldown
	time.Sleep(50 * time.Millisecond)

	// Transition to Half-Open by requesting a probe
	if err := b.Allow(); err != nil {
		t.Fatalf("Allow() = %v, want nil", err)
	}

	// A single failed probe must immediately trip the breaker back to Open
	b.Update(false)

	if got := b.State(); got != breaker.StateOpen {
		t.Errorf("State() = %v, want %v", got, breaker.StateOpen)
	}

	if err := b.Allow(); !errors.Is(err, breaker.ErrCircuitOpen) {
		t.Errorf("Allow() = %v, want %v", err, breaker.ErrCircuitOpen)
	}
}

func TestBreaker_RollbackProbe(t *testing.T) {
	config := breaker.Config{
		MinRequests:          2,
		FailureRateThreshold: 0.5,
		BaseCooldown:         20 * time.Millisecond,
		MaxProbes:            1,
		WindowDuration:       1 * time.Second,
		WindowBuckets:        5,
	}
	b := breaker.New(config)

	// Trip to Open
	b.Update(false)
	b.Update(false)

	// Wait past cooldown
	time.Sleep(45 * time.Millisecond)

	// Allow transitions to Half-Open and consumes the only available probe
	if err := b.Allow(); err != nil {
		t.Fatalf("Allow() = %v, want nil", err)
	}

	// Subsequent request must be rejected because inFlightProbes == MaxProbes
	if err := b.Allow(); !errors.Is(err, breaker.ErrCircuitOpen) {
		t.Fatalf("Allow() = %v, want %v", err, breaker.ErrCircuitOpen)
	}

	// Rollback probe slot (e.g., dropped upstream by concurrency limiter)
	b.RollbackProbe()

	// Should now be permitted again since the slot was restored
	if err := b.Allow(); err != nil {
		t.Errorf("Allow() = %v, want nil", err)
	}
}

func TestBreaker_Concurrency(t *testing.T) {
	workers := 25
	iterations := 500
	config := breaker.DefaultConfig()
	config.BaseCooldown = 10 * time.Millisecond
	config.WindowDuration = 5 * time.Second
	b := breaker.New(config)
	var wg sync.WaitGroup

	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for i := range iterations {
				if err := b.Allow(); err == nil {
					if i%7 == 0 {
						b.RollbackProbe()
					} else {
						b.Update(i%3 != 0)
					}
				}

				if i%10 == 0 {
					b.State()
				}
			}
		}()
	}

	wg.Wait()

	if got := b.State(); got != breaker.StateClosed && got != breaker.StateHalfOpen && got != breaker.StateOpen {
		t.Errorf("State() = %v, want one of [%v, %v, %v]",
			got, breaker.StateClosed, breaker.StateHalfOpen, breaker.StateOpen)
	}
}
