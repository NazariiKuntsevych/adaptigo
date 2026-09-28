package breaker_test

import (
	"testing"
	"time"

	"adaptigo/pkg/breaker"
)

func TestDefaultConfig(t *testing.T) {
	got := breaker.DefaultConfig()
	want := breaker.Config{
		MinRequests:          20,
		FailureRateThreshold: 0.5,
		BaseCooldown:         1 * time.Second,
		MaxCooldown:          30 * time.Second,
		MaxProbes:            5,
		WindowDuration:       10 * time.Second,
		WindowBuckets:        10,
	}

	if got != want {
		t.Errorf("DefaultConfig() = %+v, want %+v", got, want)
	}
}

func TestConfig_Normalize(t *testing.T) {
	defaultConfig := breaker.DefaultConfig()

	tests := []struct {
		name string
		in   breaker.Config
		want breaker.Config
	}{
		{
			name: "zero value config populates all defaults",
			in:   breaker.Config{},
			want: defaultConfig,
		},
		{
			name: "negative values sanitized to defaults",
			in: breaker.Config{
				MinRequests:          -10,
				FailureRateThreshold: -0.1,
				BaseCooldown:         -5 * time.Second,
				MaxCooldown:          -10 * time.Second,
				MaxProbes:            -2,
				WindowDuration:       -1 * time.Second,
				WindowBuckets:        -5,
			},
			want: defaultConfig,
		},
		{
			name: "preserves valid custom parameters",
			in: breaker.Config{
				MinRequests:          50,
				FailureRateThreshold: 0.3,
				BaseCooldown:         2 * time.Second,
				MaxCooldown:          60 * time.Second,
				MaxProbes:            10,
				WindowDuration:       30 * time.Second,
				WindowBuckets:        15,
			},
			want: breaker.Config{
				MinRequests:          50,
				FailureRateThreshold: 0.3,
				BaseCooldown:         2 * time.Second,
				MaxCooldown:          60 * time.Second,
				MaxProbes:            10,
				WindowDuration:       30 * time.Second,
				WindowBuckets:        15,
			},
		},
		{
			name: "threshold boundary above 1.0 resets to default",
			in: breaker.Config{
				FailureRateThreshold: 1.5,
			},
			want: defaultConfig,
		},
		{
			name: "max cooldown smaller than base cooldown scales automatically",
			in: breaker.Config{
				BaseCooldown: 3 * time.Second,
				MaxCooldown:  1 * time.Second,
			},
			want: func() breaker.Config {
				c := defaultConfig
				c.BaseCooldown = 3 * time.Second
				c.MaxCooldown = 3 * time.Second * 30
				return c
			}(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.in
			got.Normalize()

			if got != tc.want {
				t.Errorf("Normalize() = %+v, want %+v", got, tc.want)
			}
		})
	}
}
