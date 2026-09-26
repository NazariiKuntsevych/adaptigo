package breaker

import "time"

// Config defines operational parameters for the adaptive circuit breaker.
type Config struct {
	// MinRequests is the minimum request volume required in the evaluation window
	// before any failure-rate-based decisions are made.
	MinRequests int

	// FailureRateThreshold is the failure ratio [0.0, 1.0] that triggers the breaker to open.
	FailureRateThreshold float64

	// BaseCooldown is the minimum duration the breaker remains open before probing recovery.
	BaseCooldown time.Duration

	// MaxCooldown is the upper ceiling for exponential backoff duration.
	MaxCooldown time.Duration

	// MaxProbes is the number of consecutive successful requests required in Half-Open to close the circuit.
	MaxProbes int

	// WindowDuration is the rolling time window used for statistical sampling.
	WindowDuration time.Duration

	// WindowBuckets is the number of discrete time buckets within the rolling window.
	WindowBuckets int
}

// DefaultConfig creates production-ready defaults.
func DefaultConfig() Config {
	return Config{
		MinRequests:          20,
		FailureRateThreshold: 0.5,
		BaseCooldown:         1 * time.Second,
		MaxCooldown:          30 * time.Second,
		MaxProbes:            5,
		WindowDuration:       10 * time.Second,
		WindowBuckets:        10,
	}
}

// Normalize sanitizes missing or invalid configuration values using standard defaults.
func (c *Config) Normalize() {
	defaultConfig := DefaultConfig()
	if c.MinRequests <= 0 {
		c.MinRequests = defaultConfig.MinRequests
	}
	if c.FailureRateThreshold <= 0 || c.FailureRateThreshold > 1 {
		c.FailureRateThreshold = defaultConfig.FailureRateThreshold
	}
	if c.BaseCooldown <= 0 {
		c.BaseCooldown = defaultConfig.BaseCooldown
	}
	if c.BaseCooldown <= 0 {
		c.BaseCooldown = defaultConfig.BaseCooldown
	}
	if c.MaxCooldown < c.BaseCooldown {
		c.MaxCooldown = c.BaseCooldown * 30
	}
	if c.MaxProbes <= 0 {
		c.MaxProbes = defaultConfig.MaxProbes
	}
	if c.WindowDuration <= 0 {
		c.WindowDuration = defaultConfig.WindowDuration
	}
	if c.WindowBuckets <= 0 {
		c.WindowBuckets = defaultConfig.WindowBuckets
	}
}
