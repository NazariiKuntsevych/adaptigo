package limiter_test

import (
	"testing"
	"time"

	"adaptigo/pkg/limiter"
)

func TestDefaultConfig(t *testing.T) {
	got := limiter.DefaultConfig()
	want := limiter.Config{
		InitialLimit:          35.0,
		MinLimit:              5.0,
		MaxLimit:              50.0,
		QueueHeadroom:         5.0,
		SmoothingFactor:       0.15,
		BaselineResetInterval: 5 * time.Minute,
	}

	if got != want {
		t.Errorf("DefaultConfig() = %+v, want %+v", got, want)
	}
}

func TestConfig_Normalize(t *testing.T) {
	defaultConfig := limiter.DefaultConfig()

	tests := []struct {
		name string
		in   limiter.Config
		want limiter.Config
	}{
		{
			name: "zero value config populates all defaults",
			in:   limiter.Config{},
			want: defaultConfig,
		},
		{
			name: "negative values sanitized to defaults",
			in: limiter.Config{
				InitialLimit:          -10.0,
				MinLimit:              -2.0,
				MaxLimit:              -50.0,
				QueueHeadroom:         -1.0,
				SmoothingFactor:       -0.5,
				BaselineResetInterval: -1 * time.Minute,
			},
			want: defaultConfig,
		},
		{
			name: "preserves valid custom parameters",
			in: limiter.Config{
				InitialLimit:          20.0,
				MinLimit:              4.0,
				MaxLimit:              100.0,
				QueueHeadroom:         3.0,
				SmoothingFactor:       0.25,
				BaselineResetInterval: 10 * time.Minute,
			},
			want: limiter.Config{
				InitialLimit:          20.0,
				MinLimit:              4.0,
				MaxLimit:              100.0,
				QueueHeadroom:         3.0,
				SmoothingFactor:       0.25,
				BaselineResetInterval: 10 * time.Minute,
			},
		},
		{
			name: "smoothing factor above 1.0 resets to default",
			in: limiter.Config{
				SmoothingFactor: 1.5,
			},
			want: defaultConfig,
		},
		{
			name: "valid boundary smoothing factor of 1.0 is preserved",
			in: limiter.Config{
				InitialLimit:          35.0,
				MinLimit:              5.0,
				MaxLimit:              50.0,
				QueueHeadroom:         5.0,
				SmoothingFactor:       1.0,
				BaselineResetInterval: 5 * time.Minute,
			},
			want: limiter.Config{
				InitialLimit:          35.0,
				MinLimit:              5.0,
				MaxLimit:              50.0,
				QueueHeadroom:         5.0,
				SmoothingFactor:       1.0,
				BaselineResetInterval: 5 * time.Minute,
			},
		},
		{
			name: "max limit smaller than min limit scales automatically",
			in: limiter.Config{
				MinLimit: 8.0,
				MaxLimit: 3.0, // strictly less than MinLimit
			},
			want: func() limiter.Config {
				c := defaultConfig
				c.MinLimit = 8.0
				c.MaxLimit = 8.0 * 10 // expected 80.0
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
