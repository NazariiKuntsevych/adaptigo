package main

import (
	"context"
	"io"
	"net/http"
	"time"
)

// staticClient represents a baseline HTTP client implementation using a fixed timeout
// and a traditional consecutive-failure circuit breaker without adaptive capabilities.
type staticClient struct {
	base    *http.Client
	breaker *staticBreaker
}

// newStaticClient constructs a staticClient configured with a static deadline,
// fixed breaker cooldown interval, and consecutive failure threshold.
func newStaticClient(timeout time.Duration, cooldown time.Duration, failureThreshold int) *staticClient {
	customTransport := http.DefaultTransport.(*http.Transport).Clone()
	customTransport.MaxIdleConnsPerHost = 100
	customTransport.MaxIdleConns = 100

	return &staticClient{
		base: &http.Client{
			Transport: customTransport,
			Timeout:   timeout,
		},
		breaker: newStaticBreaker(cooldown, failureThreshold),
	}
}

// do executes an outbound HTTP GET request, checking the breaker and classifying the outcome.
func (sc *staticClient) do(ctx context.Context, targetURL string) (time.Duration, status) {
	if err := sc.breaker.allow(); err != nil {
		return 0, statusThrottled
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return 0, statusFailure
	}

	start := time.Now()
	resp, reqErr := sc.base.Do(req)
	latency := time.Since(start)

	success := (reqErr == nil && resp.StatusCode < http.StatusInternalServerError && resp.StatusCode != http.StatusTooManyRequests)
	sc.breaker.update(success)

	if reqErr != nil {
		return latency, statusFailure
	}

	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)

	if !success {
		return latency, statusFailure
	}

	return latency, statusSuccess
}

// stats returns the static client's current breaker state and fixed timeout for reporting.
func (sc *staticClient) stats() (state string, limit float64, inFlight int, timeoutMs float64) {
	state = sc.breaker.State().String()
	limit = 0
	inFlight = 0
	timeoutMs = float64(sc.base.Timeout.Milliseconds())
	return state, limit, inFlight, timeoutMs
}
