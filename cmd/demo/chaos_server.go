package main

import (
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"
)

// chaosServer simulates an upstream microservice with dynamically configurable
// artificial latency injection and randomized HTTP error rates.
type chaosServer struct {
	mu        sync.RWMutex
	server    *httptest.Server
	delay     time.Duration
	errorRate float64
}

// newChaosServer spins up an in-process HTTP test server with initial fault profiles.
func newChaosServer(initialDelay time.Duration, initialErrorRate float64) *chaosServer {
	chaosServer := &chaosServer{
		delay:     initialDelay,
		errorRate: initialErrorRate,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/work", chaosServer.handleWork)

	chaosServer.server = httptest.NewServer(mux)
	return chaosServer
}

// url returns the fully qualified target URL pointing to the work endpoint.
func (cs *chaosServer) url() string {
	return cs.server.URL + "/api/work"
}

// setFaults dynamically adjusts artificial processing latency and failure probability.
func (cs *chaosServer) setFaults(delay time.Duration, errorRate float64) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	cs.delay = delay
	cs.errorRate = errorRate
}

// close shuts down the underlying test server and frees allocated network ports.
func (cs *chaosServer) close() {
	cs.server.Close()
}

// handleWork handles incoming test requests by evaluating active delays and error distributions.
func (cs *chaosServer) handleWork(w http.ResponseWriter, req *http.Request) {
	cs.mu.RLock()
	delay := cs.delay
	errorRate := cs.errorRate
	cs.mu.RUnlock()

	// Artificial latency simulation
	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-req.Context().Done():
			return
		}
	}

	// Fault injection simulation
	//nolint:gosec // G404: weak random generator is safe for chaos engineering
	if errorRate > 0 && rand.Float64() < errorRate {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"status":"error","message":"internal failure"}`))
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}
