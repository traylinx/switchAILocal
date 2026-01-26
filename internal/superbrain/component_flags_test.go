package superbrain

import (
	"testing"

	"github.com/traylinx/switchAILocal/internal/config"
)

// TestComponentFlags verifies that component flags control individual Superbrain components.
func TestComponentFlags(t *testing.T) {
	tests := []struct {
		name      string
		flags     config.ComponentFlags
		component string
		want      bool
	}{
		{
			name: "overwatch enabled",
			flags: config.ComponentFlags{
				OverwatchEnabled: true,
				DoctorEnabled:    false,
			},
			component: "overwatch",
			want:      true,
		},
		{
			name: "overwatch disabled",
			flags: config.ComponentFlags{
				OverwatchEnabled: false,
			},
			component: "overwatch",
			want:      false,
		},
		{
			name: "doctor enabled",
			flags: config.ComponentFlags{
				DoctorEnabled: true,
			},
			component: "doctor",
			want:      true,
		},
		{
			name: "injector enabled",
			flags: config.ComponentFlags{
				InjectorEnabled: true,
			},
			component: "injector",
			want:      true,
		},
		{
			name: "recovery enabled",
			flags: config.ComponentFlags{
				RecoveryEnabled: true,
			},
			component: "recovery",
			want:      true,
		},
		{
			name: "fallback enabled",
			flags: config.ComponentFlags{
				FallbackEnabled: true,
			},
			component: "fallback",
			want:      true,
		},
		{
			name: "sculptor enabled",
			flags: config.ComponentFlags{
				SculptorEnabled: true,
			},
			component: "sculptor",
			want:      true,
		},
		{
			name: "unknown component",
			flags: config.ComponentFlags{
				OverwatchEnabled: true,
			},
			component: "unknown",
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.SuperbrainConfig{
				Enabled:        true,
				Mode:           "autopilot",
				ComponentFlags: tt.flags,
			}

			mock := &mockExecutor{identifier: "test"}
			se := NewSuperbrainExecutor(mock, cfg)

			got := se.isComponentEnabled(tt.component)
			if got != tt.want {
				t.Errorf("isComponentEnabled(%q) = %v, want %v", tt.component, got, tt.want)
			}
		})
	}
}

// TestComponentFlagsNilConfig verifies behavior when config is nil.
func TestComponentFlagsNilConfig(t *testing.T) {
	mock := &mockExecutor{identifier: "test"}
	se := NewSuperbrainExecutor(mock, nil)

	components := []string{"overwatch", "doctor", "injector", "recovery", "fallback", "sculptor"}
	for _, component := range components {
		if se.isComponentEnabled(component) {
			t.Errorf("isComponentEnabled(%q) should return false when config is nil", component)
		}
	}
}

// TestShouldPerformPreFlightWithComponentFlags verifies that shouldPerformPreFlight
// respects component flags.
func TestShouldPerformPreFlightWithComponentFlags(t *testing.T) {
	tests := []struct {
		name            string
		sculptorEnabled bool
		componentFlag   bool
		want            bool
	}{
		{
			name:            "both enabled",
			sculptorEnabled: true,
			componentFlag:   true,
			want:            true,
		},
		{
			name:            "sculptor enabled, component flag disabled",
			sculptorEnabled: true,
			componentFlag:   false,
			want:            false,
		},
		{
			name:            "sculptor disabled, component flag enabled",
			sculptorEnabled: false,
			componentFlag:   true,
			want:            false,
		},
		{
			name:            "both disabled",
			sculptorEnabled: false,
			componentFlag:   false,
			want:            false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.SuperbrainConfig{
				Enabled: true,
				Mode:    "autopilot",
				ComponentFlags: config.ComponentFlags{
					SculptorEnabled: tt.componentFlag,
				},
				ContextSculptor: config.ContextSculptorConfig{
					Enabled: tt.sculptorEnabled,
				},
			}

			mock := &mockExecutor{identifier: "test"}
			se := NewSuperbrainExecutor(mock, cfg)

			got := se.shouldPerformPreFlight()
			if got != tt.want {
				t.Errorf("shouldPerformPreFlight() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestComponentFlagsDefaults verifies that component flags default to true.
func TestComponentFlagsDefaults(t *testing.T) {
	cfg := &config.SuperbrainConfig{
		Enabled: true,
		Mode:    "autopilot",
		// ComponentFlags not explicitly set, should use zero values
	}

	mock := &mockExecutor{identifier: "test"}
	se := NewSuperbrainExecutor(mock, cfg)

	// With zero values (false), all components should be disabled
	components := []string{"overwatch", "doctor", "injector", "recovery", "fallback", "sculptor"}
	for _, component := range components {
		if se.isComponentEnabled(component) {
			t.Errorf("isComponentEnabled(%q) should return false with zero-value ComponentFlags", component)
		}
	}
}
