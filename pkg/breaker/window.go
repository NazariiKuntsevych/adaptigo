package breaker

import (
	"sync"
	"time"
)

// bucket represents a discrete time interval holding aggregated success and failure counts.
type bucket struct {
	timestamp time.Time
	successes int
	failures  int
}

// reset clears the bucket's telemetry and updates its timestamp to the current interval.
func (b *bucket) reset(now time.Time) {
	b.timestamp = now
	b.successes = 0
	b.failures = 0
}

// Window maintains a ring buffer of time-bucketed telemetry to calculate rolling failure rates.
type Window struct {
	mu      sync.Mutex
	buckets []bucket

	bucketDuration time.Duration
	windowDuration time.Duration
	head           int
}

// NewWindow initializes a window partitioned into buckets discrete intervals.
func NewWindow(duration time.Duration, buckets int) *Window {
	if duration <= 0 {
		duration = 10 * time.Second
	}
	if buckets <= 0 {
		buckets = 10
	}

	w := &Window{
		buckets:        make([]bucket, buckets),
		bucketDuration: duration / time.Duration(buckets),
		windowDuration: duration,
	}
	now := time.Now()
	w.reset(now)
	return w
}

// Update tallies an operation result in the current active bucket.
func (w *Window) Update(success bool) {
	w.mu.Lock()
	defer w.mu.Unlock()

	now := time.Now()
	w.sync(now)

	if !success {
		w.buckets[w.head].failures++
	} else {
		w.buckets[w.head].successes++
	}
}

// Summary calculates the aggregated request count, failure count, and failure rate across the active window.
func (w *Window) Summary() (total, failures int, failureRate float64) {
	w.mu.Lock()
	defer w.mu.Unlock()

	now := time.Now()
	w.sync(now)

	cutoff := now.Add(-w.windowDuration)

	for _, bucket := range w.buckets {
		if !bucket.timestamp.Before(cutoff) {
			total += bucket.successes + bucket.failures
			failures += bucket.failures
		}
	}

	if total == 0 {
		return 0, 0, 0.0
	}

	failureRate = float64(failures) / float64(total)
	return total, failures, failureRate
}

// Reset clears all historical data across all buckets.
func (w *Window) Reset() {
	w.mu.Lock()
	defer w.mu.Unlock()

	now := time.Now()
	w.reset(now)
}

// reset forcefully clears all historical buckets and resets the head pointer.
func (w *Window) reset(now time.Time) {
	for i := range w.buckets {
		w.buckets[i].reset(now)
	}
	w.head = 0
}

// sync advances the ring buffer to the current time, clearing expired buckets along the way.
func (w *Window) sync(now time.Time) {
	lastTime := w.buckets[w.head].timestamp
	diff := now.Sub(lastTime)

	if diff < w.bucketDuration {
		return
	}

	if diff >= w.windowDuration {
		w.reset(now)
		return
	}

	steps := int(diff / w.bucketDuration)
	for range steps {
		w.head = (w.head + 1) % len(w.buckets)
		lastTime = lastTime.Add(w.bucketDuration)
		w.buckets[w.head].reset(lastTime)
	}
}
