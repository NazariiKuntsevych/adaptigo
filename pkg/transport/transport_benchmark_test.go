package transport_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"adaptigo/pkg/breaker"
	"adaptigo/pkg/limiter"
	"adaptigo/pkg/timeout"
	"adaptigo/pkg/transport"
	"adaptigo/pkg/zerotrust"
)

const benchmarkURL = "http://example.internal/benchmark"

type zeroTrustTransport struct {
	tokenManager *zerotrust.TokenManager
	policyEngine *zerotrust.PolicyEngine
	serviceID    string
}

func newZeroTrustTransport(serviceID string, tokenManager *zerotrust.TokenManager, policyEngine *zerotrust.PolicyEngine) *zeroTrustTransport {
	return &zeroTrustTransport{
		tokenManager: tokenManager,
		policyEngine: policyEngine,
		serviceID:    serviceID,
	}
}

func (tp *zeroTrustTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if tp.tokenManager != nil && tp.policyEngine != nil {
		authHeader := req.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Body:       io.NopCloser(bytes.NewReader([]byte(`{"error":"missing token"}`))),
			}, nil
		}

		rawToken := strings.TrimPrefix(authHeader, "Bearer ")
		claims, err := tp.tokenManager.Verify(rawToken, tp.serviceID)
		if err != nil {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Body:       io.NopCloser(bytes.NewReader([]byte(`{"error":"invalid token"}`))),
			}, nil
		}

		if err := tp.policyEngine.Authorize(claims.Issuer, tp.serviceID, req.Method, req.URL.Path); err != nil {
			return &http.Response{
				StatusCode: http.StatusForbidden,
				Body:       io.NopCloser(bytes.NewReader([]byte(`{"error":"access denied"}`))),
			}, nil
		}
	}

	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader([]byte(`{"status":"ok"}`))),
		Header:     make(http.Header),
	}, nil
}

func BenchmarkPipeline_Baseline(b *testing.B) {
	tp := newZeroTrustTransport("service-backend", nil, nil)
	client := &http.Client{Transport: tp}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, benchmarkURL, nil)
	if err != nil {
		b.Fatalf("NewRequestWithContext() = %v, want nil", err)
	}

	b.ReportAllocs()
	for b.Loop() {
		resp, err := client.Do(req)
		if err != nil {
			b.Fatalf("Do() = %v, want nil", err)
		}
		if resp.StatusCode != http.StatusOK {
			b.Fatalf("StatusCode = %d, want %d", resp.StatusCode, http.StatusOK)
		}

		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
}

func BenchmarkPipeline_ResilienceOnly(b *testing.B) {
	tp := transport.New(
		newZeroTrustTransport("service-backend", nil, nil),
		transport.WithBreaker(breaker.Default()),
		transport.WithLimiter(limiter.Default()),
		transport.WithTimeout(timeout.Default()),
	)
	client := &http.Client{Transport: tp}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, benchmarkURL, nil)
	if err != nil {
		b.Fatalf("NewRequestWithContext() = %v, want nil", err)
	}

	b.ReportAllocs()
	for b.Loop() {
		resp, err := client.Do(req)
		if err != nil {
			b.Fatalf("Do() = %v, want nil", err)
		}
		if resp.StatusCode != http.StatusOK {
			b.Fatalf("StatusCode = %d, want %d", resp.StatusCode, http.StatusOK)
		}

		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
}

func BenchmarkPipeline_ResilienceWithZeroTrust(b *testing.B) {
	secret := []byte("crypto-benchmark-secret-key")
	tm := zerotrust.NewTokenManager(secret, 5*time.Minute)
	defer tm.Stop()
	pe := zerotrust.NewPolicyEngine()
	pe.AddRule("service-client", "service-backend", "GET", "/benchmark")

	tp := transport.New(
		newZeroTrustTransport("service-backend", tm, pe),
		transport.WithBreaker(breaker.Default()),
		transport.WithLimiter(limiter.Default()),
		transport.WithTimeout(timeout.Default()),
		transport.WithZeroTrust(tm, "service-client", "service-backend"),
	)
	client := &http.Client{Transport: tp}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, benchmarkURL, nil)
	if err != nil {
		b.Fatalf("NewRequestWithContext() = %v, want nil", err)
	}

	b.ReportAllocs()
	for b.Loop() {
		resp, err := client.Do(req)
		if err != nil {
			b.Fatalf("Do() = %v, want nil", err)
		}
		if resp.StatusCode != http.StatusOK {
			b.Fatalf("StatusCode = %d, want %d", resp.StatusCode, http.StatusOK)
		}

		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
}
