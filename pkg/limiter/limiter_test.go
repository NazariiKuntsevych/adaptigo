package limiter_test

import (
	"errors"
	"math"
	"sync"
	"testing"
	"time"

	"adaptigo/pkg/limiter"
)

func TestLimiter_InitialState(t *testing.T) {
	l := limiter.Default()

	if got, want := l.Limit(), 35.0; got != want {
		t.Errorf("Limit() = %.2f, want %.2f", got, want)
	}

	if got, want := l.InFlight(), 0; got != want {
		t.Errorf("InFlight() = %d, want %d", got, want)
	}
}

func TestLimiter_AcquireAndRelease(t *testing.T) {
	l := limiter.Default()

	release, err := l.Acquire()
	if err != nil {
		t.Fatalf("Acquire() = %v, want nil", err)
	}

	if got, want := l.InFlight(), 1; got != want {
		t.Fatalf("InFlight() = %d, want %d", got, want)
	}

	release()

	if got, want := l.InFlight(), 0; got != want {
		t.Errorf("InFlight() = %d, want %d", got, want)
	}

	// Ensure release closure is idempotent via sync.Once
	release()
	if got, want := l.InFlight(), 0; got != want {
		t.Errorf("InFlight() = %d, want %d", got, want)
	}
}

func TestLimiter_CapacityExceeded(t *testing.T) {
	config := limiter.Config{
		InitialLimit: 2.0,
		MinLimit:     1.0,
		MaxLimit:     10.0,
	}
	l := limiter.New(config)

	release1, err := l.Acquire()
	if err != nil {
		t.Fatalf("Acquire() = %v, want nil", err)
	}

	release2, err := l.Acquire()
	if err != nil {
		t.Fatalf("Acquire() = %v, want nil", err)
	}

	// 3rd acquisition must be rejected as limit == 2.0
	_, err = l.Acquire()
	if !errors.Is(err, limiter.ErrLimitExceeded) {
		t.Fatalf("Acquire() = %v, want %v", err, limiter.ErrLimitExceeded)
	}

	release1()

	releaseRetry, err := l.Acquire()
	if err != nil {
		t.Fatalf("Acquire() = %v, want nil", err)
	}

	release2()
	releaseRetry()

	if got, want := l.InFlight(), 0; got != want {
		t.Errorf("InFlight() = %d, want %d", got, want)
	}
}

func TestLimiter_UpdateSuccessGradient(t *testing.T) {
	// Set SmoothingFactor = 1.0 to test pure deterministic formula without historical lag
	config := limiter.Config{
		InitialLimit:          20.0,
		MinLimit:              5.0,
		MaxLimit:              50.0,
		QueueHeadroom:         2.0,
		SmoothingFactor:       1.0,
		BaselineResetInterval: 5 * time.Minute,
	}
	l := limiter.New(config)

	// Baseline established at 50ms (gradient = 50/50 = 1.0)
	// Expected limit: 20 * 1.0 + 2.0 = 22.0
	l.Update(50*time.Millisecond, true)
	if got, want := l.Limit(), 22.0; math.Abs(got-want) > 1e-9 {
		t.Errorf("Limit() = %.4f, want %.4f", got, want)
	}

	// Latency doubles to 100ms (gradient = 50/100 = 0.5)
	// Expected limit: 22 * 0.5 + 2.0 = 13.0
	l.Update(100*time.Millisecond, true)
	if got, want := l.Limit(), 13.0; math.Abs(got-want) > 1e-9 {
		t.Errorf("Limit() = %.4f, want %.4f", got, want)
	}

	// Severe congestion with 500ms latency
	// Expected limit: 13 * 0.5 + 2.0 = 8.5
	l.Update(500*time.Millisecond, true)
	if got, want := l.Limit(), 8.5; math.Abs(got-want) > 1e-9 {
		t.Errorf("Limit() = %.4f, want %.4f", got, want)
	}
}

func TestLimiter_UpdateFailureMultiplicativeDecrease(t *testing.T) {
	// SmoothingFactor = 1.0 for instant drop verification
	config := limiter.Config{
		InitialLimit:    20.0,
		MinLimit:        5.0,
		MaxLimit:        50.0,
		SmoothingFactor: 1.0,
	}
	l := limiter.New(config)

	// Expected limit: 20 * 0.8 = 16.0
	l.Update(100*time.Millisecond, false)
	if got, want := l.Limit(), 16.0; math.Abs(got-want) > 1e-9 {
		t.Errorf("Limit() = %.2f, want %.2f", got, want)
	}

	// Subsequent failures must not drop limit below configured MinLimit
	for range 10 {
		l.Update(100*time.Millisecond, false)
	}

	if got := l.Limit(); got != config.MinLimit {
		t.Errorf("Limit() = %.2f, want %.2f", got, config.MinLimit)
	}
}

func TestLimiter_UpdateBaselinePoisoningProtection(t *testing.T) {
	config := limiter.Config{
		InitialLimit:          20.0,
		MinLimit:              5.0,
		MaxLimit:              50.0,
		QueueHeadroom:         2.0,
		SmoothingFactor:       1.0,
		BaselineResetInterval: 5 * time.Minute,
	}
	l := limiter.New(config)

	// Establish low latency baseline of 20ms
	l.Update(20*time.Millisecond, true)

	// High latency failure (e.g., downstream panic after 2000ms)
	// BaseRTT must not be overwritten by this failed sample
	l.Update(2000*time.Millisecond, false)

	// Next successful request returns in 40ms
	// If baseRTT was corrupted to 2000ms, gradient would be 2000/40 = 50 -> clamped to 1.0
	// Since baseRTT is still 20ms, gradient is 20/40 = 0.5
	limitBefore := l.Limit()
	l.Update(40*time.Millisecond, true)

	want := limitBefore*0.5 + config.QueueHeadroom
	if got := l.Limit(); math.Abs(got-want) > 1e-9 {
		t.Errorf("Limit() = %.4f, want %.4f", got, want)
	}
}

func TestLimiter_UpdateIgnoreNonPositiveSample(t *testing.T) {
	l := limiter.Default()
	initialLimit := l.Limit()

	// Zero and negative durations should be ignored as telemetry noise
	l.Update(0, true)
	l.Update(-15*time.Millisecond, true)
	l.Update(0, false)

	if got := l.Limit(); got != initialLimit {
		t.Errorf("Limit() = %.2f, want %.2f", got, initialLimit)
	}
}

func TestLimiter_Concurrency(t *testing.T) {
	workers := 25
	iterations := 500
	config := limiter.DefaultConfig()
	config.MinLimit = 5.0
	config.MaxLimit = 40.0
	l := limiter.New(config)
	var wg sync.WaitGroup

	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for i := range iterations {
				release, err := l.Acquire()
				if err == nil {
					sample := time.Duration(10+i%30) * time.Millisecond
					success := i%5 != 0

					l.Update(sample, success)
					release()
				}

				if i%10 == 0 {
					l.Limit()
					l.InFlight()
				}
			}
		}()
	}

	wg.Wait()

	if got, want := l.InFlight(), 0; got != want {
		t.Errorf("InFlight() = %d, want %d", got, want)
	}

	if got := l.Limit(); got < config.MinLimit || got > config.MaxLimit {
		t.Errorf("Limit() = %.2f, want within [%.2f, %.2f]", got, config.MinLimit, config.MaxLimit)
	}
}
