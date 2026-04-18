package autoroute

import (
	"fmt"
	"time"
)

// Config represents the master configuration for the Auto-Routing subsystem.
type Config struct {
	Enabled           bool                          `yaml:"enabled" json:"enabled"`
	MaxResolution     time.Duration                 `yaml:"max-resolution-ms" json:"max_resolution_ms"`
	Providers         map[string]ProviderTierConfig `yaml:"providers" json:"providers"`
	Preferences       []ModelPreference             `yaml:"preferences" json:"preferences"`
	Conservation      ConservationConfig            `yaml:"conservation" json:"conservation"`
	Discovery         DiscoveryConfig               `yaml:"discovery" json:"discovery"`
	Weights           ScoringWeights                `yaml:"weights" json:"weights"`
	IntentMatrix      map[string][]string           `yaml:"intent-matrix" json:"intent_matrix"`
	Lab               LabConfig                     `yaml:"lab" json:"lab"`
	// DisabledProviders lists provider names that should never be selected, even if
	// they score highest. Use when credits are exhausted or a provider is known-bad.
	// Example: [anthropic] skips all Anthropic models until the list is cleared.
	DisabledProviders []string `yaml:"disabled-providers" json:"disabled_providers"`
}

// DefaultConfig returns the default safe configuration for Auto-Routing (opt-in).
func DefaultConfig() Config {
	return Config{
		Enabled:       false,
		MaxResolution: 5 * time.Millisecond,
		Weights: ScoringWeights{
			Availability: 0.35,
			Quota:        0.25,
			Latency:      0.20,
			SuccessRate:  0.20,
		},
		Conservation: ConservationConfig{
			Enabled:               true,
			SimpleThreshold:       500,
			PremiumConservationAt: 0.20, // 20% quota remaining
		},
		Discovery: DiscoveryConfig{
			ProbeOnStartup:    true,
			ProbeInterval:     15 * time.Minute,
			ProbeTimeout:      5 * time.Second,
			PassiveMonitoring: true,
			CacheTTL:          24 * time.Hour,
		},
		Lab: LabConfig{
			Enabled:            false,
			AdaptationInterval: 24 * time.Hour,
			MaxWeightDrift:     0.10, // 10% max change per cycle
		},
	}
}

// Validate ensures the configuration is mathematically sound and safe to execute.
func (c *Config) Validate() error {
	sum := c.Weights.Availability + c.Weights.Quota + c.Weights.Latency + c.Weights.SuccessRate
	// allow tiny float variations
	if sum < 0.99 || sum > 1.01 {
		return fmt.Errorf("scoring weights must sum to 1.0, got %f", sum)
	}

	if c.MaxResolution <= 0 || c.MaxResolution > 50*time.Millisecond {
		return fmt.Errorf("max-resolution-ms must be between 1ms and 50ms, got %v", c.MaxResolution)
	}

	for _, pref := range c.Preferences {
		if pref.Preference < 0.0 || pref.Preference > 1.0 {
			return fmt.Errorf("model preference must be between 0.0 and 1.0, got %f for %s", pref.Preference, pref.Model)
		}
	}

	return nil
}

// ProviderTierConfig defines fixed constraints and tier allocations for a provider.
type ProviderTierConfig struct {
	Tier          string            `yaml:"tier" json:"tier"`
	MonthlyBudget float64           `yaml:"monthly-budget,omitempty" json:"monthly_budget,omitempty"`
	ModelTiers    map[string]string `yaml:"model-tiers,omitempty" json:"model_tiers,omitempty"`
}

// ModelPreference allows soft-steering of specific models.
type ModelPreference struct {
	Model      string  `yaml:"model" json:"model"`
	Preference float64 `yaml:"preference" json:"preference"` // 0.0 to 1.0 multiplier boost
	Reason     string  `yaml:"reason,omitempty" json:"reason,omitempty"`
}

// ConservationConfig dictates how aggressively the router should hoard premium tokens.
type ConservationConfig struct {
	Enabled               bool `yaml:"enabled" json:"enabled"`
	SimpleThreshold       int  `yaml:"simple-threshold-tokens" json:"simple_threshold_tokens"`
	PremiumConservationAt float64 `yaml:"premium-conservation-at" json:"premium_conservation_at"` // percentage (0.0 to 1.0)
}

// DiscoveryConfig controls active and passive intelligence gathering.
type DiscoveryConfig struct {
	Enabled           bool          `yaml:"enabled" json:"enabled"`
	ProbeOnStartup    bool          `yaml:"probe-on-startup" json:"probe_on_startup"`
	ProbeInterval     time.Duration `yaml:"probe-interval" json:"probe_interval"`
	ProbeTimeout      time.Duration `yaml:"probe-timeout" json:"probe_timeout"`
	PassiveMonitoring bool          `yaml:"passive-monitoring" json:"passive_monitoring"`
	CacheTTL          time.Duration `yaml:"cache-ttl" json:"cache_ttl"`
}

// LabConfig controls the autonomous self-optimization engine (autoresearch plugin equivalent).
type LabConfig struct {
	Enabled            bool          `yaml:"enabled" json:"enabled"`
	AdaptationInterval time.Duration `yaml:"adaptation-interval" json:"adaptation_interval"`
	MaxWeightDrift     float64       `yaml:"max-weight-drift" json:"max_weight_drift"`
}

// ScoringWeights represent the importance of different health metrics.
type ScoringWeights struct {
	Availability float64 `yaml:"availability" json:"availability"`
	Quota        float64 `yaml:"quota" json:"quota"`
	Latency      float64 `yaml:"latency" json:"latency"`
	SuccessRate  float64 `yaml:"success-rate" json:"success_rate"`
}
