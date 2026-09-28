package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"adaptigo/pkg/breaker"
	"adaptigo/pkg/limiter"
	"adaptigo/pkg/timeout"
	"adaptigo/pkg/transport"
)

// adaptiveClient encapsulates an HTTP client decorated with the full adaptive
// resilience stack: circuit breaker, concurrency limiter, and dynamic timeout tracker.
type adaptiveClient struct {
	base    *http.Client
	breaker *breaker.Breaker
	limiter *limiter.Limiter
	tracker *timeout.Tracker
}

// newAdaptiveClient initializes an HTTP client using the resilient Transport pipeline with default configs.
func newAdaptiveClient() *adaptiveClient {
	breaker := breaker.Default()
	limiter := limiter.Default()
	tracker := timeout.Default()
	customTransport := http.DefaultTransport.(*http.Transport).Clone()
	customTransport.MaxIdleConnsPerHost = 100
	customTransport.MaxIdleConns = 100

	return &adaptiveClient{
		base: &http.Client{
			Transport: transport.New(
				customTransport,
				transport.WithBreaker(breaker),
				transport.WithLimiter(limiter),
				transport.WithTimeout(tracker),
			),
		},
		breaker: breaker,
		limiter: limiter,
		tracker: tracker,
	}
}

// do executes an outbound HTTP request through the adaptive transport pipeline and classifies the outcome.
func (ac *adaptiveClient) do(ctx context.Context, targetURL string) (time.Duration, status) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return 0, statusFailure
	}

	start := time.Now()
	resp, reqErr := ac.base.Do(req)
	latency := time.Since(start)

	if reqErr != nil {
		if errors.Is(reqErr, limiter.ErrLimitExceeded) || errors.Is(reqErr, breaker.ErrCircuitOpen) {
			return latency, statusThrottled
		}

		return latency, statusFailure
	}

	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= http.StatusInternalServerError {
		return latency, statusFailure
	}

	return latency, statusSuccess
}

// stats extracts real-time telemetry from the underlying resilience components for metrics reporting.
func (ac *adaptiveClient) stats() (state string, limit float64, inFlight int, timeoutMs float64) {
	state = ac.breaker.State().String()
	limit = ac.limiter.Limit()
	inFlight = ac.limiter.InFlight()
	timeoutMs = float64(ac.tracker.Timeout().Milliseconds())
	return state, limit, inFlight, timeoutMs
}
