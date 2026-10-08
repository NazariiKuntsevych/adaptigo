# AdaptiGo

AdaptiGo is an adaptive HTTP client resilience and Zero-Trust security library for Go, designed to protect service-to-service communication from cascading failures, network congestion, and unauthorized access.

## Features

* **Adaptive Concurrency Limiter**: Gradient-based capacity limits using Little's Law to prevent latency degradation and downstream queue saturation.
* **Dynamic Timeouts**: Automatically calculates request deadlines using Jacobson's round-trip time (RTT) estimation algorithm (RFC 6298).
* **Circuit Breaker**: A sophisticated 3-state (Closed, Half-Open, Open) breaker with exponential backoff, jitter, and trial probe rate-limiting.
* **Zero-Trust Security**: Ephemeral HMAC-signed token issuance, constant-time validation, and a Default-Deny Policy Engine (PDP) for strict inter-service authentication.
* **Integrated Transport**: A drop-in `http.RoundTripper` decorator that combines all resilience and security mechanisms into a seamless pipeline.
* **Zero Dependencies**: Built entirely using the Go standard library.
* **Chaos Testing Suite**: Includes a built-in tool to simulate latency spikes and error storms.

## Installation

```shell
$ go get github.com/NazariiKuntsevych/adaptigo
```

## Overview

### Client-Side: Adaptive Transport Pipeline

Easily decorate the standard HTTP client to protect outbound requests with resilience and security mechanisms.

```go
package main

import (
	"net/http"
	"time"

	"adaptigo/pkg/breaker"
	"adaptigo/pkg/limiter"
	"adaptigo/pkg/timeout"
	"adaptigo/pkg/transport"
	"adaptigo/pkg/zerotrust"
)

func main() {
	// 1. Initialize Security (Zero-Trust)
	secret := []byte("crypto-secret-key")
	tokenManager := zerotrust.NewTokenManager(secret, 5*time.Minute)
	defer tokenManager.Stop()

	// 2. Build the resilient and secure Transport
	resilientTransport := transport.New(
		http.DefaultTransport,
		transport.WithBreaker(breaker.Default()),
		transport.WithLimiter(limiter.Default()),
		transport.WithTimeout(timeout.Default()),
		transport.WithZeroTrust(tokenManager, "service-client", "service-backend"),
	)

	// 3. Inject into standard HTTP Client
	client := &http.Client{
		Transport: resilientTransport,
	}

	// 4. Use safely
	resp, err := client.Get("http://internal-backend.local/api/data")
	// ...
	
}
```

### Server-Side: Zero-Trust Policy Enforcement

AdaptiGo implements a strict Default-Deny authorization model. Protect your endpoints by wrapping handlers with a Zero-Trust middleware that validates cryptographic tokens and evaluates access policies.

```go
package main

import (
	"net/http"
	"strings"
	"time"

	"adaptigo/pkg/zerotrust"
)

// ZeroTrustMiddleware verifies incoming caller tokens and enforces the policy engine.
func ZeroTrustMiddleware(tm *zerotrust.TokenManager, pe *zerotrust.PolicyEngine, serviceID string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 1. Extract Bearer token
			authHeader := r.Header.Get("Authorization")
			if !strings.HasPrefix(authHeader, "Bearer ") {
				http.Error(w, `{"error":"missing token"}`, http.StatusUnauthorized)
				return
			}
			rawToken := strings.TrimPrefix(authHeader, "Bearer ")

			// 2. Cryptographically verify signature, TTL, and audience
			claims, err := tm.Verify(rawToken, serviceID)
			if err != nil {
				http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
				return
			}

			// 3. Enforce Default-Deny policy
			if err := pe.Authorize(claims.Issuer, serviceID, r.Method, r.URL.Path); err != nil {
				http.Error(w, `{"error":"access denied"}`, http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func main() {
	// 1. Initialize Security (Zero-Trust)
	secret := []byte("crypto-secret-key")
	tokenManager := zerotrust.NewTokenManager(secret, 5*time.Minute)
	defer tokenManager.Stop()

	// 2. Define explicit access rules
	policyEngine := zerotrust.NewPolicyEngine()
	policyEngine.AddRule("service-client", "service-backend", "GET", "/api/data")
	policyEngine.AddRule("*", "service-backend", "GET", "/public")

	// 3. Setup router and handlers
	mux := http.NewServeMux()
	mux.HandleFunc("/api/data", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	// 4. Wrap entire router (or individual routes) with the middleware
	withZeroTrust := ZeroTrustMiddleware(tokenManager, policyEngine, "service-backend")
	
	http.ListenAndServe(":8080", withZeroTrust(mux))
}
```

## Getting Started

#### 1. Clone the repository:

```shell
$ git clone git@github.com:NazariiKuntsevych/adaptigo.git
$ cd adaptigo/
```

#### 2. To run linter:

```shell
$ golangci-lint run ./...
```

#### 3. To run tests:

```shell
$ go test -v -race -count=1 -cover ./pkg/...
```

#### 4. To run benchmark:

```shell
$ go test -run=^$ -bench=BenchmarkPipeline -benchtime=1000x -benchmem ./pkg/transport/...
```

#### 5. Run the chaos simulation:

This runs a testing suite that compares a static client against the adaptive client under varying fault conditions.

```shell
$ go run ./cmd/demo/...
```

This will generate a `demo_results.csv` file containing second-by-second telemetry comparing the two strategies.

## Code Structure

```
adaptigo/
|-- cmd/demo/
|---- main.go            # Entry point for the benchmark scenario
|---- chaos_server.go    # Simulated faulty upstream service
|---- metrics.go         # CSV telemetry extraction
|---- adaptive_client.go # Benchmark client using AdaptiGo
|---- static_client.go   # Benchmark client using static timeouts/breakers
|---- static_breaker.go  # 3-state static circuit breaker
|-- pkg/
|---- breaker/           # 3-state circuit breaker with exponential backoff
|---- limiter/           # Gradient concurrency limiter based on Little's Law
|---- timeout/           # Dynamic RTT-based timeout tracker (RFC 6298)
|---- transport/         # HTTP RoundTripper orchestrating the pipeline
|---- zerotrust/         # TokenManager and Default-Deny PolicyEngine
```

## Licensing

The code in this project is licensed under MIT license.
