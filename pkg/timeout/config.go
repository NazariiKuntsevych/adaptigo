package timeout

import "time"

// Config defines the tuning parameters for the dynamic RTT-based timeout tracker.
type Config struct {
	// MinTimeout is the lower bound for request deadlines to prevent premature cancellation.
	MinTimeout time.Duration

	// MaxTimeout is the upper safety ceiling for request deadlines.
	MaxTimeout time.Duration

	// Alpha is the smoothing factor for the smoothed round-trip time (SRTT).
	Alpha float64

	// Beta is the smoothing factor for the round-trip time variation (RTTVAR).
	Beta float64

	// K is the multiplier for the delay variance used in deadline estimation.
	K float64
}

// DefaultConfig creates production-ready defaults.
func DefaultConfig() Config {
	return Config{
		MinTimeout: 200 * time.Millisecond,
		MaxTimeout: 1500 * time.Millisecond,
		Alpha:      0.125,
		Beta:       0.25,
		K:          4.0,
	}
}

// Normalize sanitizes configuration fields and substitutes valid defaults for invalid inputs.
func (c *Config) Normalize() {
	defaultConfig := DefaultConfig()
	if c.MinTimeout <= 0 {
		c.MinTimeout = defaultConfig.MinTimeout
	}
	if c.MaxTimeout <= 0 {
		c.MaxTimeout = defaultConfig.MaxTimeout
	}
	if c.MaxTimeout < c.MinTimeout {
		c.MaxTimeout = c.MinTimeout * 40
	}
	if c.Alpha <= 0 || c.Alpha >= 1.0 {
		c.Alpha = defaultConfig.Alpha
	}
	if c.Beta <= 0 || c.Beta >= 1.0 {
		c.Beta = defaultConfig.Beta
	}
	if c.K <= 0 {
		c.K = defaultConfig.K
	}
}
