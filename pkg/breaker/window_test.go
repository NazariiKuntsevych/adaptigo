package breaker_test

import (
	"math"
	"sync"
	"testing"
	"time"

	"adaptigo/pkg/breaker"
)

func TestWindow_Empty(t *testing.T) {
	w := breaker.NewWindow(100*time.Millisecond, 5)

	gotTotal, gotFailures, gotFailureRate := w.Summary()
	if gotTotal != 0 || gotFailures != 0 || gotFailureRate != 0.0 {
		t.Errorf("Summary() = (total: %d, failures: %d, rate: %.2f), want (total: 0, failures: 0, rate: 0.00)",
			gotTotal, gotFailures, gotFailureRate)
	}
}

func TestWindow_Mixed(t *testing.T) {
	w := breaker.NewWindow(200*time.Millisecond, 5)

	for range 6 {
		w.Update(true)
	}
	for range 4 {
		w.Update(false)
	}

	gotTotal, gotFailures, gotFailureRate := w.Summary()
	if gotTotal != 10 || gotFailures != 4 || math.Abs(gotFailureRate-0.4) > 1e-9 {
		t.Errorf("Summary() = (total: %d, failures: %d, rate: %.4f), want (total: 10, failures: 4, rate: 0.4000)",
			gotTotal, gotFailures, gotFailureRate)
	}
}

func TestWindow_Expiry(t *testing.T) {
	windowDuration := 50 * time.Millisecond
	w := breaker.NewWindow(windowDuration, 5)

	w.Update(false)
	w.Update(false)

	gotTotal, gotFailures, _ := w.Summary()
	if gotTotal != 2 || gotFailures != 2 {
		t.Fatalf("Summary() = (total: %d, failures: %d), want (total: 2, failures: 2)", gotTotal, gotFailures)
	}

	// Sleep past the full window duration to allow all buckets to expire
	time.Sleep(windowDuration + 25*time.Millisecond)

	gotTotal, gotFailures, gotFailureRate := w.Summary()
	if gotTotal != 0 || gotFailures != 0 || gotFailureRate != 0.0 {
		t.Errorf("Summary() = (total: %d, failures: %d, rate: %.2f), want (total: 0, failures: 0, rate: 0.00)",
			gotTotal, gotFailures, gotFailureRate)
	}
}

func TestWindow_Reset(t *testing.T) {
	w := breaker.NewWindow(time.Second, 5)

	w.Update(true)
	w.Update(false)

	w.Reset()

	gotTotal, gotFailures, gotFailureRate := w.Summary()
	if gotTotal != 0 || gotFailures != 0 || gotFailureRate != 0.0 {
		t.Errorf("Summary() = (total: %d, failures: %d, rate: %.2f), want (total: 0, failures: 0, rate: 0.00)",
			gotTotal, gotFailures, gotFailureRate)
	}
}

func TestWindow_Concurrency(t *testing.T) {
	workers := 30
	iterations := 1000
	w := breaker.NewWindow(10*time.Second, 10)
	var wg sync.WaitGroup

	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for i := range iterations {
				w.Update(i%2 == 0)

				if i%10 == 0 {
					w.Summary()
				}
			}
		}()
	}

	wg.Wait()

	gotTotal, gotFailures, _ := w.Summary()
	wantTotal := workers * iterations
	wantFailures := wantTotal / 2
	if gotTotal != wantTotal || gotFailures != wantFailures {
		t.Errorf("Summary() = (total: %d, failures: %d), want (total: %d, failures: %d)",
			gotTotal, gotFailures, wantTotal, wantFailures)
	}
}
