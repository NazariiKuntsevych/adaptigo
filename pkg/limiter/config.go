package limiter

import "time"

// Config defines the tuning parameters for the gradient concurrency limiter.
type Config struct {
	// InitialLimit is the starting concurrency allocation before any runtime telemetry is gathered.
	InitialLimit float64

	// MinLimit is the absolute lower boundary for concurrency capacity to prevent starvation.
	MinLimit float64

	// MaxLimit is the upper ceiling for concurrency capacity to protect system boundaries.
	MaxLimit float64

	// QueueHeadroom represents the additive probe headroom used to discover available capacity.
	QueueHeadroom float64

	// SmoothingFactor determines the weight [0.0, 1.0] of new observations in the exponential moving average.
	SmoothingFactor float64

	// BaselineResetInterval defines how often the minimum observed RTT reference is refreshed.
	BaselineResetInterval time.Duration
}

// DefaultConfig creates production-ready defaults.
func DefaultConfig() Config {
	return Config{
		InitialLimit:          35.0,
		MinLimit:              5.0,
		MaxLimit:              50.0,
		QueueHeadroom:         5.0,
		SmoothingFactor:       0.15,
		BaselineResetInterval: 5 * time.Minute,
	}
}

// Normalize ensures that all configuration fields hold safe and valid values.
func (c *Config) Normalize() {
	defaultConfig := DefaultConfig()
	if c.InitialLimit <= 0 {
		c.InitialLimit = defaultConfig.InitialLimit
	}
	if c.MinLimit <= 0 {
		c.MinLimit = defaultConfig.MinLimit
	}
	if c.MaxLimit <= 0 {
		c.MaxLimit = defaultConfig.MaxLimit
	}
	if c.MaxLimit < c.MinLimit {
		c.MaxLimit = c.MinLimit * 10
	}
	if c.QueueHeadroom <= 0 {
		c.QueueHeadroom = defaultConfig.QueueHeadroom
	}
	if c.SmoothingFactor <= 0 || c.SmoothingFactor > 1 {
		c.SmoothingFactor = defaultConfig.SmoothingFactor
	}
	if c.BaselineResetInterval <= 0 {
		c.BaselineResetInterval = defaultConfig.BaselineResetInterval
	}
}
