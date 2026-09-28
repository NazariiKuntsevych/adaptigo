package main

import (
	"strconv"
)

// metricSnapshot captures system performance and resilience telemetry aggregated over a single time interval.
type metricSnapshot struct {
	// Second indicates the elapsed execution time in seconds since the scenario began.
	Second int `json:"second"`

	// Strategy denotes the client configuration tested ("static" or "adaptive").
	Strategy string `json:"strategy"`

	// Requests is the total number of invocations initiated during the interval.
	Requests int `json:"requests"`

	// Successes is the count of requests that completed with HTTP status < 500.
	Successes int `json:"successes"`

	// Throttled is the count of requests shed locally by circuit breaking or concurrency limits.
	Throttled int `json:"throttled"`

	// Failures is the count of requests that resulted in transport errors or 5xx responses.
	Failures int `json:"failures"`

	// AvgLatencyMs represents the arithmetic mean round-trip latency in milliseconds.
	AvgLatencyMs float64 `json:"avg_latency_ms"`

	// P95LatencyMs represents the 95th percentile latency in milliseconds.
	P95LatencyMs float64 `json:"p95_latency_ms"`

	// State reflects the circuit breaker operational state at the time of sampling.
	State string `json:"state"`

	// Limit is the allocated concurrency ceiling (0 for static clients).
	Limit float64 `json:"limit"`

	// InFlight is the number of requests currently executing concurrently.
	InFlight int `json:"in_flight"`

	// TimeoutMs is the effective deadline applied to outbound calls in milliseconds.
	TimeoutMs float64 `json:"timeout_ms"`
}

// csvHeaders returns the column definitions for the benchmark CSV telemetry export.
func csvHeaders() []string {
	return []string{
		"second",
		"strategy",
		"requests",
		"successes",
		"throttled",
		"failures",
		"avg_latency_ms",
		"p95_latency_ms",
		"state",
		"limit",
		"in_flight",
		"timeout_ms",
	}
}

// csvRow serializes the metric snapshot into a string slice formatted for CSV writers.
func (m metricSnapshot) csvRow() []string {
	return []string{
		strconv.Itoa(m.Second),
		m.Strategy,
		strconv.Itoa(m.Requests),
		strconv.Itoa(m.Successes),
		strconv.Itoa(m.Throttled),
		strconv.Itoa(m.Failures),
		strconv.FormatFloat(m.AvgLatencyMs, 'f', 2, 64),
		strconv.FormatFloat(m.P95LatencyMs, 'f', 2, 64),
		m.State,
		strconv.FormatFloat(m.Limit, 'f', 2, 64),
		strconv.Itoa(m.InFlight),
		strconv.FormatFloat(m.TimeoutMs, 'f', 2, 64),
	}
}
