package integration

import (
	"testing"
	"time"

	"github.com/traylinx/switchAILocal/internal/config"
	"github.com/traylinx/switchAILocal/internal/heartbeat"
)

func TestConvertHeartbeatConfig(t *testing.T) {
	tests := []struct {
		name     string
		input    *config.HeartbeatConfig
		expected *heartbeat.HeartbeatConfig
	}{
		{
			name:  "nil config returns defaults",
			input: nil,
			expected: &heartbeat.HeartbeatConfig{
				Enabled:                true,
				Interval:               5 * time.Minute,
				Timeout:                5 * time.Second,
				AutoDiscovery:          true,
				QuotaWarningThreshold:  0.80,
				QuotaCriticalThreshold: 0.95,
				MaxConcurrentChecks:    10,
				RetryAttempts:          2,
				RetryDelay:             time.Second,
			},
		},
		{
			name: "valid config with all fields",
			input: &config.HeartbeatConfig{
				Enabled:                true,
				Interval:               "10m",
				Timeout:                "10s",
				AutoDiscovery:          false,
				QuotaWarningThreshold:  0.70,
				QuotaCriticalThreshold: 0.90,
				MaxConcurrentChecks:    20,
				RetryAttempts:          3,
				RetryDelay:             "2s",
			},
			expected: &heartbeat.HeartbeatConfig{
				Enabled:                true,
				Interval:               10 * time.Minute,
				Timeout:                10 * time.Second,
				AutoDiscovery:          false,
				QuotaWarningThreshold:  0.70,
				QuotaCriticalThreshold: 0.90,
				MaxConcurrentChecks:    20,
				RetryAttempts:          3,
				RetryDelay:             2 * time.Second,
			},
		},
		{
			name: "invalid interval falls back to default",
			input: &config.HeartbeatConfig{
				Enabled:  true,
				Interval: "invalid",
			},
			expected: &heartbeat.HeartbeatConfig{
				Enabled:                true,
				Interval:               5 * time.Minute,
				Timeout:                5 * time.Second,
				AutoDiscovery:          false,
				QuotaWarningThreshold:  0.80,
				QuotaCriticalThreshold: 0.95,
				MaxConcurrentChecks:    10,
				RetryAttempts:          0,
				RetryDelay:             time.Second,
			},
		},
		{
			name: "empty strings use defaults",
			input: &config.HeartbeatConfig{
				Enabled:  true,
				Interval: "",
				Timeout:  "",
			},
			expected: &heartbeat.HeartbeatConfig{
				Enabled:                true,
				Interval:               5 * time.Minute,
				Timeout:                5 * time.Second,
				AutoDiscovery:          false,
				QuotaWarningThreshold:  0.80,
				QuotaCriticalThreshold: 0.95,
				MaxConcurrentChecks:    10,
				RetryAttempts:          0,
				RetryDelay:             time.Second,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertHeartbeatConfig(tt.input)

			if result.Enabled != tt.expected.Enabled {
				t.Errorf("Enabled: got %v, want %v", result.Enabled, tt.expected.Enabled)
			}
			if result.Interval != tt.expected.Interval {
				t.Errorf("Interval: got %v, want %v", result.Interval, tt.expected.Interval)
			}
			if result.Timeout != tt.expected.Timeout {
				t.Errorf("Timeout: got %v, want %v", result.Timeout, tt.expected.Timeout)
			}
			if result.AutoDiscovery != tt.expected.AutoDiscovery {
				t.Errorf("AutoDiscovery: got %v, want %v", result.AutoDiscovery, tt.expected.AutoDiscovery)
			}
			if result.QuotaWarningThreshold != tt.expected.QuotaWarningThreshold {
				t.Errorf("QuotaWarningThreshold: got %v, want %v", result.QuotaWarningThreshold, tt.expected.QuotaWarningThreshold)
			}
			if result.QuotaCriticalThreshold != tt.expected.QuotaCriticalThreshold {
				t.Errorf("QuotaCriticalThreshold: got %v, want %v", result.QuotaCriticalThreshold, tt.expected.QuotaCriticalThreshold)
			}
			if result.MaxConcurrentChecks != tt.expected.MaxConcurrentChecks {
				t.Errorf("MaxConcurrentChecks: got %v, want %v", result.MaxConcurrentChecks, tt.expected.MaxConcurrentChecks)
			}
			if result.RetryAttempts != tt.expected.RetryAttempts {
				t.Errorf("RetryAttempts: got %v, want %v", result.RetryAttempts, tt.expected.RetryAttempts)
			}
			if result.RetryDelay != tt.expected.RetryDelay {
				t.Errorf("RetryDelay: got %v, want %v", result.RetryDelay, tt.expected.RetryDelay)
			}
		})
	}
}

func TestRegisterProviderCheckers(t *testing.T) {
	tests := []struct {
		name          string
		config        *config.Config
		expectedCount int
	}{
		{
			name:          "nil config registers nothing",
			config:        nil,
			expectedCount: 0,
		},
		{
			name: "ollama enabled registers checker",
			config: &config.Config{
				Ollama: config.OllamaConfig{
					Enabled: true,
					BaseURL: "http://localhost:11434",
				},
			},
			expectedCount: 1,
		},
		{
			name: "multiple providers register multiple checkers",
			config: &config.Config{
				Ollama: config.OllamaConfig{
					Enabled: true,
					BaseURL: "http://localhost:11434",
				},
				LMStudio: config.LMStudioConfig{
					Enabled: true,
					BaseURL: "http://localhost:1234/v1",
				},
				GeminiKey: []config.GeminiKey{
					{APIKey: "test-key"},
				},
			},
			expectedCount: 3,
		},
		{
			name: "disabled providers don't register",
			config: &config.Config{
				Ollama: config.OllamaConfig{
					Enabled: false,
				},
				LMStudio: config.LMStudioConfig{
					Enabled: false,
				},
			},
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a heartbeat monitor
			hbConfig := heartbeat.DefaultHeartbeatConfig()
			hbConfig.Enabled = true
			monitor := heartbeat.NewHeartbeatMonitor(hbConfig)

			// Register provider checkers
			err := RegisterProviderCheckers(monitor, tt.config)
			if err != nil {
				t.Errorf("RegisterProviderCheckers() error = %v", err)
			}

			// Check the number of registered checkers by casting to concrete type
			// This is acceptable in tests to verify internal state
			if monitorImpl, ok := monitor.(*heartbeat.HeartbeatMonitorImpl); ok {
				stats := monitorImpl.GetStats()
				if stats.ProvidersMonitored != tt.expectedCount {
					t.Errorf("Expected %d registered checkers, got %d", tt.expectedCount, stats.ProvidersMonitored)
				}
			} else {
				t.Error("Failed to cast monitor to concrete type")
			}
		})
	}
}

func TestRegisterProviderCheckers_NilMonitor(t *testing.T) {
	cfg := &config.Config{
		Ollama: config.OllamaConfig{
			Enabled: true,
		},
	}

	// Should not panic with nil monitor
	err := RegisterProviderCheckers(nil, cfg)
	if err != nil {
		t.Errorf("Expected no error with nil monitor, got %v", err)
	}
}
