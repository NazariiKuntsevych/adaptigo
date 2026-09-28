package timeout_test

import (
	"testing"
	"time"

	"adaptigo/pkg/timeout"
)

func TestDefaultConfig(t *testing.T) {
	got := timeout.DefaultConfig()
	want := timeout.Config{
		MinTimeout: 200 * time.Millisecond,
		MaxTimeout: 1500 * time.Millisecond,
		Alpha:      0.125,
		Beta:       0.25,
		K:          4.0,
	}

	if got != want {
		t.Errorf("DefaultConfig() = %+v, want %+v", got, want)
	}
}

func TestConfig_Normalize(t *testing.T) {
	defaultConfig := timeout.DefaultConfig()

	tests := []struct {
		name string
		in   timeout.Config
		want timeout.Config
	}{
		{
			name: "zero value config populates all defaults",
			in:   timeout.Config{},
			want: defaultConfig,
		},
		{
			name: "negative values sanitized to defaults",
			in: timeout.Config{
				MinTimeout: -100 * time.Millisecond,
				MaxTimeout: -1 * time.Second,
				Alpha:      -0.2,
				Beta:       -0.5,
				K:          -2.0,
			},
			want: defaultConfig,
		},
		{
			name: "preserves valid custom parameters",
			in: timeout.Config{
				MinTimeout: 50 * time.Millisecond,
				MaxTimeout: 3 * time.Second,
				Alpha:      0.2,
				Beta:       0.3,
				K:          3.0,
			},
			want: timeout.Config{
				MinTimeout: 50 * time.Millisecond,
				MaxTimeout: 3 * time.Second,
				Alpha:      0.2,
				Beta:       0.3,
				K:          3.0,
			},
		},
		{
			name: "alpha upper boundary at or above 1.0 resets to default",
			in: timeout.Config{
				Alpha: 1.0,
			},
			want: defaultConfig,
		},
		{
			name: "beta upper boundary at or above 1.0 resets to default",
			in: timeout.Config{
				Beta: 1.2,
			},
			want: defaultConfig,
		},
		{
			name: "max timeout smaller than min timeout scales automatically",
			in: timeout.Config{
				MinTimeout: 100 * time.Millisecond,
				MaxTimeout: 50 * time.Millisecond, // strictly less than MinTimeout
			},
			want: func() timeout.Config {
				c := defaultConfig
				c.MinTimeout = 100 * time.Millisecond
				c.MaxTimeout = 100 * time.Millisecond * 40 // expected 4s
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
