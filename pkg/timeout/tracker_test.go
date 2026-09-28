package timeout_test

import (
	"sync"
	"testing"
	"time"

	"adaptigo/pkg/timeout"
)

func TestTracker_InitialState(t *testing.T) {
	tr := timeout.Default()

	if got, want := tr.Timeout(), 1500*time.Millisecond; got != want {
		t.Errorf("Timeout() = %v, want %v", got, want)
	}

	if got, want := tr.SmoothedRTT(), time.Duration(0); got != want {
		t.Errorf("SmoothedRTT() = %v, want %v", got, want)
	}

	if got, want := tr.RTTVar(), time.Duration(0); got != want {
		t.Errorf("RTTVar() = %v, want %v", got, want)
	}
}

func TestTracker_FirstSampleInitialization(t *testing.T) {
	tr := timeout.Default()

	// Sample: 100ms -> SRTT = 100ms, RTTVAR = 50ms
	tr.Update(100 * time.Millisecond)

	if got, want := tr.SmoothedRTT(), 100*time.Millisecond; got != want {
		t.Errorf("SmoothedRTT() = %v, want %v", got, want)
	}

	if got, want := tr.RTTVar(), 50*time.Millisecond; got != want {
		t.Errorf("RTTVar() = %v, want %v", got, want)
	}

	// Expected timeout: SRTT + 4 * RTTVAR = 100ms + 4 * 50ms = 300ms
	if got, want := tr.Timeout(), 300*time.Millisecond; got != want {
		t.Errorf("Timeout() = %v, want %v", got, want)
	}
}

func TestTracker_UpdateRFC6298(t *testing.T) {
	tr := timeout.Default()

	// Sample: 100ms -> SRTT = 100ms, RTTVAR = 50ms
	tr.Update(100 * time.Millisecond)

	// Sample: 140ms ->
	// SRTT = (1 - 0.125) * 100ms + 0.125 * 140ms = 87.5ms + 17.5ms = 105ms
	// RTTVAR = (1 - 0.25) * 50ms + 0.25 * |100ms - 140ms| = 37.5ms + 10ms = 47.5ms
	// Timeout = 105ms + 4 * 47.5ms = 295ms
	tr.Update(140 * time.Millisecond)

	if got, want := tr.SmoothedRTT(), 105*time.Millisecond; got != want {
		t.Errorf("SmoothedRTT() = %v, want %v", got, want)
	}

	if got, want := tr.RTTVar(), 47500*time.Microsecond; got != want {
		t.Errorf("RTTVar() = %v, want %v", got, want)
	}

	if got, want := tr.Timeout(), 295*time.Millisecond; got != want {
		t.Errorf("Timeout() = %v, want %v", got, want)
	}
}

func TestTracker_ClampingBoundaries(t *testing.T) {
	config := timeout.Config{
		MinTimeout: 100 * time.Millisecond,
		MaxTimeout: 500 * time.Millisecond,
		Alpha:      0.125,
		Beta:       0.25,
		K:          4.0,
	}
	tr := timeout.New(config)

	// Very fast sample: 10ms -> timeout must be clamped upwards to MinTimeout
	tr.Update(10 * time.Millisecond)

	if got := tr.Timeout(); got != config.MinTimeout {
		t.Errorf("Timeout() = %v, want %v", got, config.MinTimeout)
	}

	// Spike sample: 2s -> timeout must be clamped downwards to MaxTimeout
	tr.Update(2 * time.Second)

	if got := tr.Timeout(); got != config.MaxTimeout {
		t.Errorf("Timeout() = %v, want %v", got, config.MaxTimeout)
	}
}

func TestTracker_IgnoreNonPositiveSample(t *testing.T) {
	tr := timeout.Default()
	initialTimeout := tr.Timeout()

	// Zero and negative durations should be ignored as invalid telemetry
	tr.Update(0)
	tr.Update(-20 * time.Millisecond)

	if got := tr.Timeout(); got != initialTimeout {
		t.Errorf("Timeout() = %v, want %v", got, initialTimeout)
	}

	if got, want := tr.SmoothedRTT(), time.Duration(0); got != want {
		t.Errorf("SmoothedRTT() = %v, want %v", got, want)
	}
}

func TestTracker_Concurrency(t *testing.T) {
	workers := 25
	iterations := 500
	config := timeout.DefaultConfig()
	tr := timeout.New(config)
	var wg sync.WaitGroup

	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for i := range iterations {
				sample := time.Duration(10+i%100) * time.Millisecond
				tr.Update(sample)

				if i%10 == 0 {
					tr.Timeout()
					tr.SmoothedRTT()
					tr.RTTVar()
				}
			}
		}()
	}

	wg.Wait()

	if got := tr.Timeout(); got < config.MinTimeout || got > config.MaxTimeout {
		t.Errorf("Timeout() = %v, want within [%v, %v]", got, config.MinTimeout, config.MaxTimeout)
	}
}
