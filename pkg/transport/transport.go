// Package transport provides an HTTP client decorator combining circuit breaking,
// adaptive concurrency limiting, and dynamic timeouts into an integrated pipeline.
package transport

import (
	"context"
	"io"
	"net/http"
	"time"

	"adaptigo/pkg/breaker"
	"adaptigo/pkg/limiter"
	"adaptigo/pkg/timeout"
)

// bodyWithCancel wraps an io.ReadCloser to ensure the associated context is canceled upon closure.
type bodyWithCancel struct {
	io.ReadCloser
	cancel context.CancelFunc
}

// Close implements the io.Closer interface, closing the underlying body and triggering context cancellation.
func (b *bodyWithCancel) Close() error {
	err := b.ReadCloser.Close()
	b.cancel()
	return err
}

// Transport decorates an http.RoundTripper with adaptive resilience mechanisms.
type Transport struct {
	base http.RoundTripper

	breaker      *breaker.Breaker
	limiter      *limiter.Limiter
	tracker      *timeout.Tracker
}

// Option represents a configuration option to be applied to transport during initialization.
type Option func(*Transport)

// WithBreaker attaches a circuit breaker to the transport pipeline.
func WithBreaker(breaker *breaker.Breaker) Option {
	return func(t *Transport) {
		t.breaker = breaker
	}
}

// WithLimiter attaches an adaptive concurrency limiter to the transport pipeline.
func WithLimiter(limiter *limiter.Limiter) Option {
	return func(t *Transport) {
		t.limiter = limiter
	}
}

// WithTimeout attaches a dynamic RTT timeout tracker to the transport pipeline.
func WithTimeout(tracker *timeout.Tracker) Option {
	return func(t *Transport) {
		t.tracker = tracker
	}
}

// New creates a new Transport using provided options and base transport.
func New(base http.RoundTripper, options ...Option) *Transport {
	if base == nil {
		base = http.DefaultTransport
	}

	transport := &Transport{base: base}
	for _, option := range options {
		option(transport)
	}
	return transport
}

// Default creates a new Transport using resilience components with default configs and default base transport.
func Default() *Transport {
	return New(
		http.DefaultTransport,
		WithBreaker(breaker.Default()),
		WithLimiter(limiter.Default()),
		WithTimeout(timeout.Default()),
	)
}

// RoundTrip executes outbound HTTP calls protected by the adaptive resilience chain:
// Breaker -> Limiter -> Dynamic Timeout.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Breaker check
	if t.breaker != nil {
		if err := t.breaker.Allow(); err != nil {
			return nil, err
		}
	}

	// Limiter reservation
	if t.limiter != nil {
		release, err := t.limiter.Acquire()
		if err != nil {
			if t.breaker != nil {
				t.breaker.RollbackProbe()
			}
			return nil, err
		}
		defer release()
	}

	// Deadline context
	var ctx context.Context
	var cancel context.CancelFunc
	if t.tracker != nil {
		ctx, cancel = context.WithTimeout(req.Context(), t.tracker.Timeout())
	} else {
		ctx, cancel = context.WithCancel(req.Context())
	}
	clonedReq := req.Clone(ctx)

	// Request
	start := time.Now()
	resp, reqErr := t.base.RoundTrip(clonedReq)
	latency := time.Since(start)

	// Result update for errors
	if reqErr != nil {
		cancel()
		if t.breaker != nil {
			t.breaker.Update(false)
		}
		if t.limiter != nil {
			t.limiter.Update(latency, false)
		}
		return nil, reqErr
	}

	// Result update for status
	success := resp.StatusCode < http.StatusInternalServerError
	if t.breaker != nil {
		t.breaker.Update(success)
	}
	if t.limiter != nil {
		t.limiter.Update(latency, success)
	}
	if t.tracker != nil && success {
		t.tracker.Update(latency)
	}

	// Body check
	if resp.Body == nil {
		cancel()
		return resp, nil
	}

	// Body proper cancellation
	resp.Body = &bodyWithCancel{
		ReadCloser: resp.Body,
		cancel:     cancel,
	}
	return resp, nil
}
