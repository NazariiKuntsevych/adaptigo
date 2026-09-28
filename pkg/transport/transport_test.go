package transport_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"adaptigo/pkg/breaker"
	"adaptigo/pkg/limiter"
	"adaptigo/pkg/timeout"
	"adaptigo/pkg/transport"
	"adaptigo/pkg/zerotrust"
)

const testURL = "http://example.internal/api"

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	time.Sleep(5 * time.Millisecond)
	return f(req)
}

func TestTransport_Success(t *testing.T) {
	mockBody := `{"status":"ok"}`
	base := roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewBufferString(mockBody)),
			Header:     make(http.Header),
		}, nil
	})
	l := limiter.Default()
	tr := timeout.Default()
	tp := transport.New(
		base,
		transport.WithBreaker(breaker.Default()),
		transport.WithLimiter(l),
		transport.WithTimeout(tr),
	)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, testURL, nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext() = %v, want nil", err)
	}

	resp, err := tp.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip() = %v, want nil", err)
	}

	if got := resp.StatusCode; got != http.StatusOK {
		t.Errorf("StatusCode = %d, want %d", got, http.StatusOK)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll() = %v, want nil", err)
	}

	if got := string(bodyBytes); got != mockBody {
		t.Errorf("Body = %q, want %q", got, mockBody)
	}

	if err := resp.Body.Close(); err != nil {
		t.Errorf("Body.Close() = %v, want nil", err)
	}

	// Execution slot must be released back to limiter
	if got, want := l.InFlight(), 0; got != want {
		t.Errorf("InFlight() = %d, want %d", got, want)
	}

	// Telemetry must be recorded in timeout tracker on success
	if got := tr.SmoothedRTT(); got <= 0 {
		t.Errorf("SmoothedRTT() = %v, want > 0", got)
	}
}

func TestTransport_CircuitBreakerRejection(t *testing.T) {
	base := roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		t.Fatalf("unexpected call to inner RoundTrip()")
		return nil, errors.New("unexpected call")
	})
	b := breaker.New(breaker.Config{
		MinRequests:          1,
		FailureRateThreshold: 0.1,
	})
	tp := transport.New(base, transport.WithBreaker(b))

	// Force circuit breaker into OPEN state
	b.Update(false)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, testURL, nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext() = %v, want nil", err)
	}

	resp, err := tp.RoundTrip(req)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if !errors.Is(err, breaker.ErrCircuitOpen) {
		t.Errorf("RoundTrip() = %v, want %v", err, breaker.ErrCircuitOpen)
	}
}

func TestTransport_LimiterRejectionAndRollback(t *testing.T) {
	base := roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		t.Fatalf("unexpected call to inner RoundTrip()")
		return nil, errors.New("unexpected call")
	})
	l := limiter.New(limiter.Config{
		InitialLimit: 1.0,
		MinLimit:     1.0,
		MaxLimit:     1.0,
	})
	tp := transport.New(base, transport.WithLimiter(l))

	// Reach concurrency limit
	release, err := l.Acquire()
	if err != nil {
		t.Fatalf("Acquire() = %v, want nil", err)
	}
	defer release()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, testURL, nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext() = %v, want nil", err)
	}

	resp, err := tp.RoundTrip(req)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if !errors.Is(err, limiter.ErrLimitExceeded) {
		t.Errorf("RoundTrip() = %v, want %v", err, limiter.ErrLimitExceeded)
	}
}

func TestTransport_ZeroTrustInjection(t *testing.T) {
	secret := []byte("crypto-test-secret-key")
	tm := zerotrust.NewTokenManager(secret, 5*time.Minute)

	var capturedAuthHeader string
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		capturedAuthHeader = req.Header.Get("Authorization")
		return &http.Response{StatusCode: http.StatusOK}, nil
	})

	tp := transport.New(
		base,
		transport.WithZeroTrust(tm, "service-client", "service-backend"),
	)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, testURL, nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext() = %v, want nil", err)
	}

	resp, err := tp.RoundTrip(req)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		t.Fatalf("RoundTrip() = %v, want nil", err)
	}

	if !strings.HasPrefix(capturedAuthHeader, "Bearer ") {
		t.Fatalf("Header = %q, want prefix 'Bearer '", capturedAuthHeader)
	}

	rawToken := strings.TrimPrefix(capturedAuthHeader, "Bearer ")
	claims, err := tm.Verify(rawToken, "service-backend")
	if err != nil {
		t.Fatalf("Verify() = %v, want nil", err)
	}

	if claims != nil && claims.Issuer != "service-client" {
		t.Errorf("claims.Issuer = %q, want %q", claims.Issuer, "service-client")
	}
}

func TestTransport_ServerError(t *testing.T) {
	base := roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(bytes.NewBufferString(`{"status":"error","message":"upstream error"}`)),
		}, nil
	})
	l := limiter.Default()
	tr := timeout.Default()
	tp := transport.New(
		base,
		transport.WithBreaker(breaker.Default()),
		transport.WithLimiter(l),
		transport.WithTimeout(tr),
	)

	wantLimit := l.Limit()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, testURL, nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext() = %v, want nil", err)
	}

	resp, err := tp.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip() = %v, want nil", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)

	if got := resp.StatusCode; got != http.StatusInternalServerError {
		t.Errorf("StatusCode = %d, want %d", got, http.StatusInternalServerError)
	}

	// Concurrency slot must be cleanly returned
	if got, want := l.InFlight(), 0; got != want {
		t.Errorf("InFlight() = %d, want %d", got, want)
	}

	// Limit should experience multiplicative decrease
	if got := l.Limit(); got >= wantLimit {
		t.Errorf("Limit() = %.2f, want < %.2f", got, wantLimit)
	}

	// Tracker must not update on 5xx failures (Karn's rule)
	if got, want := tr.SmoothedRTT(), time.Duration(0); got != want {
		t.Errorf("SmoothedRTT() = %v, want %v", got, want)
	}
}

func TestTransport_NetworkError(t *testing.T) {
	networkErr := errors.New("network connection reset by peer")
	base := roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return nil, networkErr
	})
	l := limiter.Default()
	tr := timeout.Default()
	tp := transport.New(
		base,
		transport.WithBreaker(breaker.Default()),
		transport.WithLimiter(l),
		transport.WithTimeout(tr),
	)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, testURL, nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext() = %v, want nil", err)
	}

	resp, err := tp.RoundTrip(req)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
		t.Errorf("Response = %v, want nil", resp)
	}
	if !errors.Is(err, networkErr) {
		t.Errorf("RoundTrip() = %v, want %v", err, networkErr)
	}

	if got, want := l.InFlight(), 0; got != want {
		t.Errorf("InFlight() = %d, want %d", got, want)
	}

	if got, want := tr.SmoothedRTT(), time.Duration(0); got != want {
		t.Errorf("SmoothedRTT() = %v, want %v", got, want)
	}
}

func TestTransport_Timeout(t *testing.T) {
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		select {
		case <-req.Context().Done():
			return nil, req.Context().Err()
		case <-time.After(200 * time.Millisecond):
			return &http.Response{StatusCode: http.StatusOK}, nil
		}
	})
	tr := timeout.New(timeout.Config{
		MinTimeout: 20 * time.Millisecond,
		MaxTimeout: 40 * time.Millisecond,
	})
	tp := transport.New(
		base,
		transport.WithBreaker(breaker.Default()),
		transport.WithLimiter(limiter.Default()),
		transport.WithTimeout(tr),
	)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, testURL, nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext() = %v, want nil", err)
	}

	resp, err := tp.RoundTrip(req)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("RoundTrip() = %v, want %v", err, context.DeadlineExceeded)
	}
}

func TestTransport_Concurrency(t *testing.T) {
	workers := 25
	iterations := 200
	base := roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		time.Sleep(1 * time.Millisecond)
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewBufferString(`{"status":"ok"}`)),
		}, nil
	})
	l := limiter.Default()
	tp := transport.New(
		base,
		transport.WithBreaker(breaker.Default()),
		transport.WithLimiter(l),
		transport.WithTimeout(timeout.Default()),
	)
	var wg sync.WaitGroup

	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for range iterations {
				req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, testURL, nil)
				resp, err := tp.RoundTrip(req)
				if err == nil && resp != nil {
					_, _ = io.Copy(io.Discard, resp.Body)
					_ = resp.Body.Close()
				}
			}
		}()
	}

	wg.Wait()

	if got, want := l.InFlight(), 0; got != want {
		t.Errorf("InFlight() = %d, want %d", got, want)
	}
}
