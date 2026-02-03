package config

import (
	"strings"
	"time"
)

// HeartbeatConfig holds the heartbeat monitoring configuration.
// The heartbeat system provides proactive background monitoring of provider health,
// quota usage, and automatic model discovery to prevent failures before they impact users.
type HeartbeatConfig struct {
	// Enabled toggles the entire heartbeat monitoring system.
	// When false, no background monitoring occurs.
	Enabled bool `yaml:"enabled" json:"enabled"`

	// Interval is the time between heartbeat cycles.
	// Default: "5m" (5 minutes). Minimum: "30s" (30 seconds).
	Interval string `yaml:"interval" json:"interval"`

	// Timeout is the maximum time to wait for a single provider check.
	// Default: "5s" (5 seconds). Minimum: "1s" (1 second).
	Timeout string `yaml:"timeout" json:"timeout"`

	// AutoDiscovery enables automatic model discovery for supported providers.
	// When true, new models are discovered without requiring configuration reload.
	AutoDiscovery bool `yaml:"auto-discovery" json:"auto-discovery"`

	// QuotaWarningThreshold triggers warnings when quota usage exceeds this ratio (0.0-1.0).
	// Default: 0.80 (80%). Set to 0 to disable quota monitoring.
	QuotaWarningThreshold float64 `yaml:"quota-warning-threshold" json:"quota-warning-threshold"`

	// QuotaCriticalThreshold triggers critical alerts when quota usage exceeds this ratio (0.0-1.0).
	// Default: 0.95 (95%). Must be greater than QuotaWarningThreshold.
	QuotaCriticalThreshold float64 `yaml:"quota-critical-threshold" json:"quota-critical-threshold"`

	// MaxConcurrentChecks limits the number of simultaneous health checks.
	// Default: 10. Minimum: 1. Maximum: 50.
	MaxConcurrentChecks int `yaml:"max-concurrent-checks" json:"max-concurrent-checks"`

	// RetryAttempts is the number of retries for failed health checks.
	// Default: 2. Minimum: 0. Maximum: 5.
	RetryAttempts int `yaml:"retry-attempts" json:"retry-attempts"`

	// RetryDelay is the delay between retry attempts.
	// Default: "1s" (1 second). Minimum: "100ms".
	RetryDelay string `yaml:"retry-delay" json:"retry-delay"`

	// Providers configures provider-specific heartbeat settings.
	// If not specified, default settings are used for all providers.
	Providers map[string]ProviderHeartbeatConfig `yaml:"providers,omitempty" json:"providers,omitempty"`
}

// ProviderHeartbeatConfig holds provider-specific heartbeat settings.
type ProviderHeartbeatConfig struct {
	// Enabled toggles heartbeat monitoring for this specific provider.
	// Default: true (inherits from global Enabled setting).
	Enabled *bool `yaml:"enabled,omitempty" json:"enabled,omitempty"`

	// Interval overrides the global interval for this provider.
	// Some providers may benefit from more or less frequent checks.
	Interval string `yaml:"interval,omitempty" json:"interval,omitempty"`

	// Timeout overrides the global timeout for this provider.
	// Some providers may be slower and need longer timeouts.
	Timeout string `yaml:"timeout,omitempty" json:"timeout,omitempty"`

	// QuotaMonitoring enables quota monitoring for this provider.
	// Default: true for providers that support it.
	QuotaMonitoring *bool `yaml:"quota-monitoring,omitempty" json:"quota-monitoring,omitempty"`

	// AutoDiscovery enables model auto-discovery for this provider.
	// Default: true for providers that support it (Ollama, LM Studio).
	AutoDiscovery *bool `yaml:"auto-discovery,omitempty" json:"auto-discovery,omitempty"`

	// HealthEndpoint overrides the default health check endpoint for this provider.
	// Only applicable for providers that support custom health endpoints.
	HealthEndpoint string `yaml:"health-endpoint,omitempty" json:"health-endpoint,omitempty"`

	// Headers adds custom headers for health check requests to this provider.
	Headers map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`
}

// SanitizeHeartbeat validates and normalizes heartbeat configuration.
// It ensures intervals and timeouts are valid durations and applies sensible constraints.
func (cfg *Config) SanitizeHeartbeat() {
	if cfg == nil {
		return
	}

	hb := &cfg.Heartbeat

	// Validate and normalize interval
	if hb.Interval == "" {
		hb.Interval = "5m"
	}
	if interval, err := time.ParseDuration(hb.Interval); err != nil || interval < 30*time.Second {
		hb.Interval = "5m" // Default to 5 minutes for invalid or too short intervals
	}

	// Validate and normalize timeout
	if hb.Timeout == "" {
		hb.Timeout = "5s"
	}
	if timeout, err := time.ParseDuration(hb.Timeout); err != nil || timeout < time.Second {
		hb.Timeout = "5s" // Default to 5 seconds for invalid or too short timeouts
	}

	// Validate quota thresholds
	if hb.QuotaWarningThreshold < 0.0 || hb.QuotaWarningThreshold > 1.0 {
		hb.QuotaWarningThreshold = 0.80 // Default to 80%
	}
	if hb.QuotaCriticalThreshold < 0.0 || hb.QuotaCriticalThreshold > 1.0 {
		hb.QuotaCriticalThreshold = 0.95 // Default to 95%
	}
	// Ensure critical threshold is higher than warning threshold
	if hb.QuotaCriticalThreshold <= hb.QuotaWarningThreshold {
		hb.QuotaCriticalThreshold = hb.QuotaWarningThreshold + 0.10
		if hb.QuotaCriticalThreshold > 1.0 {
			hb.QuotaCriticalThreshold = 1.0
			hb.QuotaWarningThreshold = 0.90
		}
	}

	// Validate concurrent checks
	if hb.MaxConcurrentChecks < 1 {
		hb.MaxConcurrentChecks = 1
	}
	if hb.MaxConcurrentChecks > 50 {
		hb.MaxConcurrentChecks = 50
	}

	// Validate retry attempts
	if hb.RetryAttempts < 0 {
		hb.RetryAttempts = 0
	}
	if hb.RetryAttempts > 5 {
		hb.RetryAttempts = 5
	}

	// Validate and normalize retry delay
	if hb.RetryDelay == "" {
		hb.RetryDelay = "1s"
	}
	if delay, err := time.ParseDuration(hb.RetryDelay); err != nil || delay < 100*time.Millisecond {
		hb.RetryDelay = "1s" // Default to 1 second for invalid or too short delays
	}

	// Sanitize provider-specific configurations
	if hb.Providers != nil {
		for provider, providerConfig := range hb.Providers {
			// Normalize provider name
			normalizedProvider := strings.ToLower(strings.TrimSpace(provider))
			if normalizedProvider == "" {
				delete(hb.Providers, provider)
				continue
			}

			// If provider name changed, update the map
			if normalizedProvider != provider {
				delete(hb.Providers, provider)
				hb.Providers[normalizedProvider] = providerConfig
			}

			// Validate provider-specific interval
			if providerConfig.Interval != "" {
				if interval, err := time.ParseDuration(providerConfig.Interval); err != nil || interval < 30*time.Second {
					providerConfig.Interval = "" // Reset to use global default
				}
			}

			// Validate provider-specific timeout
			if providerConfig.Timeout != "" {
				if timeout, err := time.ParseDuration(providerConfig.Timeout); err != nil || timeout < time.Second {
					providerConfig.Timeout = "" // Reset to use global default
				}
			}

			// Normalize headers
			if len(providerConfig.Headers) > 0 {
				normalizedHeaders := make(map[string]string)
				for key, value := range providerConfig.Headers {
					normalizedKey := strings.TrimSpace(key)
					normalizedValue := strings.TrimSpace(value)
					if normalizedKey != "" && normalizedValue != "" {
						normalizedHeaders[normalizedKey] = normalizedValue
					}
				}
				if len(normalizedHeaders) > 0 {
					providerConfig.Headers = normalizedHeaders
				} else {
					providerConfig.Headers = nil
				}
			}

			// Update the configuration
			hb.Providers[normalizedProvider] = providerConfig
		}
	}
}

// GetHeartbeatInterval returns the heartbeat interval as a time.Duration.
func (cfg *Config) GetHeartbeatInterval() time.Duration {
	if cfg == nil {
		return 5 * time.Minute
	}
	
	interval, err := time.ParseDuration(cfg.Heartbeat.Interval)
	if err != nil {
		return 5 * time.Minute // Default fallback
	}
	
	return interval
}

// GetHeartbeatTimeout returns the heartbeat timeout as a time.Duration.
func (cfg *Config) GetHeartbeatTimeout() time.Duration {
	if cfg == nil {
		return 5 * time.Second
	}
	
	timeout, err := time.ParseDuration(cfg.Heartbeat.Timeout)
	if err != nil {
		return 5 * time.Second // Default fallback
	}
	
	return timeout
}

// GetHeartbeatRetryDelay returns the heartbeat retry delay as a time.Duration.
func (cfg *Config) GetHeartbeatRetryDelay() time.Duration {
	if cfg == nil {
		return time.Second
	}
	
	delay, err := time.ParseDuration(cfg.Heartbeat.RetryDelay)
	if err != nil {
		return time.Second // Default fallback
	}
	
	return delay
}

// IsProviderHeartbeatEnabled returns whether heartbeat monitoring is enabled for a specific provider.
func (cfg *Config) IsProviderHeartbeatEnabled(provider string) bool {
	if cfg == nil || !cfg.Heartbeat.Enabled {
		return false
	}
	
	normalizedProvider := strings.ToLower(strings.TrimSpace(provider))
	if providerConfig, exists := cfg.Heartbeat.Providers[normalizedProvider]; exists {
		if providerConfig.Enabled != nil {
			return *providerConfig.Enabled
		}
	}
	
	// Default to enabled if no provider-specific setting
	return true
}

// GetProviderHeartbeatInterval returns the heartbeat interval for a specific provider.
func (cfg *Config) GetProviderHeartbeatInterval(provider string) time.Duration {
	if cfg == nil {
		return 5 * time.Minute
	}
	
	normalizedProvider := strings.ToLower(strings.TrimSpace(provider))
	if providerConfig, exists := cfg.Heartbeat.Providers[normalizedProvider]; exists {
		if providerConfig.Interval != "" {
			if interval, err := time.ParseDuration(providerConfig.Interval); err == nil {
				return interval
			}
		}
	}
	
	// Fall back to global interval
	return cfg.GetHeartbeatInterval()
}

// GetProviderHeartbeatTimeout returns the heartbeat timeout for a specific provider.
func (cfg *Config) GetProviderHeartbeatTimeout(provider string) time.Duration {
	if cfg == nil {
		return 5 * time.Second
	}
	
	normalizedProvider := strings.ToLower(strings.TrimSpace(provider))
	if providerConfig, exists := cfg.Heartbeat.Providers[normalizedProvider]; exists {
		if providerConfig.Timeout != "" {
			if timeout, err := time.ParseDuration(providerConfig.Timeout); err == nil {
				return timeout
			}
		}
	}
	
	// Fall back to global timeout
	return cfg.GetHeartbeatTimeout()
}