// Package main provides a demonstration of adaptive client-side resilience patterns.
package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"sync"
	"time"
)

const (
	// experimentDuration defines the total execution duration for each test scenario.
	experimentDuration = 60 * time.Second

	// concurrencyWorkers is the number of concurrent goroutines dispatching requests.
	concurrencyWorkers = 30

	// outputCSVFile specifies the target destination for exported benchmark telemetry.
	outputCSVFile = "demo_results.csv"
)

// status represents the operational outcome classification of an HTTP request invocation.
type status int

const (
	// statusSuccess denotes an operation that returned an HTTP status below 500.
	statusSuccess status = iota

	// statusThrottled indicates client-side shedding via circuit breaking or rate-limiting.
	statusThrottled

	// statusFailure denotes transport errors, context timeouts, or 5xx server responses.
	statusFailure
)

// result encapsulates the measured latency and categorical outcome of a single request.
type result struct {
	latency time.Duration
	status  status
}

// client abstracts the execution interface shared by static and adaptive resilience clients.
type client interface {
	do(ctx context.Context, targetURL string) (time.Duration, status)
	stats() (state string, limit float64, inFlight int, timeoutMs float64)
}

// calculateLatencyStats calculates the arithmetic average and 95th percentile latency from raw samples.
func calculateLatencyStats(latencies []float64) (avg, p95 float64) {
	if len(latencies) == 0 {
		return 0, 0
	}

	sort.Float64s(latencies)

	sum := 0.0
	for _, v := range latencies {
		sum += v
	}
	avg = sum / float64(len(latencies))

	p95Idx := int(float64(len(latencies)-1) * 0.95)
	p95 = latencies[p95Idx]

	return avg, p95
}

// updateChaosProfile adjusts upstream fault injection parameters according to the experimental timeline.
func updateChaosProfile(chaosServer *chaosServer, elapsed time.Duration) {
	warmupEnd := experimentDuration * 25 / 100
	latencyEnd := experimentDuration * 50 / 100
	faultEnd := experimentDuration * 75 / 100

	switch {
	case elapsed < warmupEnd:
		// Stage 1: Healthy baseline (15ms RTT, 0% error)
		chaosServer.setFaults(15*time.Millisecond, 0.0)

	case elapsed < latencyEnd:
		// Stage 2: Backend slowdown (120ms RTT, 0% error) -> Concurrency Limiter contraction
		chaosServer.setFaults(120*time.Millisecond, 0.0)

	case elapsed < faultEnd:
		// Stage 3: Error storm (20ms RTT, 50% failure rate) -> Circuit Breaker tripping
		chaosServer.setFaults(20*time.Millisecond, 0.5)

	default:
		// Stage 4: Downstream recovery (15ms RTT, 0% error) -> Half-Open recovery
		chaosServer.setFaults(15*time.Millisecond, 0.0)
	}
}

// executeScenario runs a designated benchmark scenario, collecting and logging second-by-second metrics.
func executeScenario(
	client client,
	output *csv.Writer,
	strategy string,
) error {
	slog.Info("starting benchmark scenario", slog.String("strategy", strategy))

	chaosServer := newChaosServer(15*time.Millisecond, 0.0)
	defer chaosServer.close()
	ctx, cancel := context.WithTimeout(context.Background(), experimentDuration)
	defer cancel()
	resultsChan := make(chan result, 20000)
	var wg sync.WaitGroup

	for range concurrencyWorkers {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for {
				select {
				case <-ctx.Done():
					return
				default:
					latency, category := client.do(ctx, chaosServer.url())
					resultsChan <- result{latency: latency, status: category}
					time.Sleep(10 * time.Millisecond)
				}
			}
		}()
	}

	start := time.Now()
	seconds := 0
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			wg.Wait()
			return nil
		case <-ticker.C:
			seconds++
			elapsed := time.Since(start)
			updateChaosProfile(chaosServer, elapsed)

			var latencies []float64
			successes, throttled, failures := 0, 0, 0

		drainLoop:
			for {
				select {
				case result := <-resultsChan:
					switch result.status {
					case statusSuccess:
						successes++
						latencies = append(latencies, float64(result.latency.Milliseconds()))
					case statusThrottled:
						throttled++
					case statusFailure:
						failures++
						latencies = append(latencies, float64(result.latency.Milliseconds()))
					}
				default:
					break drainLoop
				}
			}

			requests := successes + throttled + failures
			avgLatencyMs, p95LatencyMs := calculateLatencyStats(latencies)
			state, limit, inFlight, timeoutMs := client.stats()

			snapshot := metricSnapshot{
				Second:       seconds,
				Strategy:     strategy,
				Requests:     requests,
				Successes:    successes,
				Throttled:    throttled,
				Failures:     failures,
				AvgLatencyMs: avgLatencyMs,
				P95LatencyMs: p95LatencyMs,
				State:        state,
				Limit:        limit,
				InFlight:     inFlight,
				TimeoutMs:    timeoutMs,
			}

			if err := output.Write(snapshot.csvRow()); err != nil {
				return fmt.Errorf("failed to append record to CSV file: %w", err)
			}
			output.Flush()

			slog.Info("metric generated", slog.Any("snapshot", snapshot))
		}
	}
}

// run sets up the simulation environment, coordinates the benchmark workloads
// under varying network fault profiles, and records comparative telemetry results.
func run() error {
	csvFile, err := os.Create(outputCSVFile)
	if err != nil {
		return fmt.Errorf("failed to create CSV file: %w", err)
	}
	defer func() { _ = csvFile.Close() }()

	writer := csv.NewWriter(csvFile)
	defer writer.Flush()

	if err := writer.Write(csvHeaders()); err != nil {
		return fmt.Errorf("failed to write header to CSV file: %w", err)
	}

	staticClient := newStaticClient(1500*time.Millisecond, 5*time.Second, 5)
	if err := executeScenario(staticClient, writer, "static"); err != nil {
		return err
	}

	time.Sleep(5 * time.Second)

	adaptiveClient := newAdaptiveClient()
	if err := executeScenario(adaptiveClient, writer, "adaptive"); err != nil {
		return err
	}

	return nil
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	if err := run(); err != nil {
		slog.Error("fatal error", slog.String("error", err.Error()))
		os.Exit(1)
	}
}
