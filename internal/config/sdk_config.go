// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package config provides configuration management for the switchAILocal server.
// It handles loading and parsing YAML configuration files, and provides structured
// access to application settings including server port, authentication directory,
// debug settings, proxy configuration, and API keys.
package config

import "strings"

// SDKConfig represents the application's configuration, loaded from a YAML file.
type SDKConfig struct {
	// ProxyURL is the URL of an optional proxy server to use for outbound requests.
	ProxyURL string `yaml:"proxy-url" json:"proxy-url"`

	// ForceModelPrefix requires explicit model prefixes (e.g., "teamA/gemini-3-pro-preview")
	// to target prefixed credentials. When false, unprefixed model requests may use prefixed
	// credentials as well.
	ForceModelPrefix bool `yaml:"force-model-prefix" json:"force-model-prefix"`

	// RequestLog enables or disables detailed request logging functionality.
	RequestLog bool `yaml:"request-log" json:"request-log"`

	// APIKeys is a list of keys for authenticating clients to this proxy server.
	APIKeys []string `yaml:"api-keys" json:"api-keys"`

	// Access holds request authentication provider configuration.
	Access AccessConfig `yaml:"auth,omitempty" json:"auth,omitempty"`

	// Streaming configures server-side streaming behavior (keep-alives and safe bootstrap retries).
	Streaming StreamingConfig `yaml:"streaming" json:"streaming"`

	// Intelligence configures the "Cortex" engine for model: "auto" routing.
	// It relies on the Plugin system and a specialized Router Model.
	Intelligence IntelligenceConfig `yaml:"intelligence" json:"intelligence"`

	// Routing controls credential selection behavior and auto-resolver priority.
	Routing RoutingConfig `yaml:"routing,omitempty" json:"routing,omitempty"`
}

// RoutingConfig configures how credentials are selected for requests.
type RoutingConfig struct {
	// Strategy selects the credential selection strategy.
	// Supported values: "round-robin" (default), "fill-first".
	Strategy string `yaml:"strategy,omitempty" json:"strategy,omitempty"`

	// AutoModelPriority defines a prioritized list of model IDs to use when "auto" is requested.
	// The server will pick the first available model from this list.
	// If none are available, it falls back to the default timestamp-based selection.
	AutoModelPriority []string `yaml:"auto-model-priority,omitempty" json:"auto-model-priority,omitempty"`
}

// StreamingConfig holds server streaming behavior configuration.
type StreamingConfig struct {
	// KeepAliveSeconds controls how often the server emits SSE heartbeats (": keep-alive\n\n").
	// nil means default (15 seconds). <= 0 disables keep-alives.
	KeepAliveSeconds *int `yaml:"keepalive-seconds,omitempty" json:"keepalive-seconds,omitempty"`

	// BootstrapRetries controls how many times the server may retry a streaming request before any bytes are sent,
	// to allow auth rotation / transient recovery.
	// nil means default (2). 0 disables bootstrap retries.
	BootstrapRetries *int `yaml:"bootstrap-retries,omitempty" json:"bootstrap-retries,omitempty"`
}

// AccessConfig groups request authentication providers.
type AccessConfig struct {
	// Providers lists configured authentication providers.
	Providers []AccessProvider `yaml:"providers,omitempty" json:"providers,omitempty"`
}

// AccessProvider describes a request authentication provider entry.
type AccessProvider struct {
	// Name is the instance identifier for the provider.
	Name string `yaml:"name" json:"name"`

	// Type selects the provider implementation registered via the SDK.
	Type string `yaml:"type" json:"type"`

	// SDK optionally names a third-party SDK module providing this provider.
	SDK string `yaml:"sdk,omitempty" json:"sdk,omitempty"`

	// APIKeys lists inline keys for providers that require them.
	APIKeys []string `yaml:"api-keys,omitempty" json:"api-keys,omitempty"`

	// Config passes provider-specific options to the implementation.
	Config map[string]any `yaml:"config,omitempty" json:"config,omitempty"`
}

const (
	// AccessProviderTypeConfigAPIKey is the built-in provider validating inline API keys.
	AccessProviderTypeConfigAPIKey = "config-api-key"

	// DefaultAccessProviderName is applied when no provider name is supplied.
	DefaultAccessProviderName = "config-inline"
)

// ConfigAPIKeyProvider returns the first inline API key provider if present.
func (c *SDKConfig) ConfigAPIKeyProvider() *AccessProvider {
	if c == nil {
		return nil
	}
	for i := range c.Access.Providers {
		if c.Access.Providers[i].Type == AccessProviderTypeConfigAPIKey {
			if c.Access.Providers[i].Name == "" {
				c.Access.Providers[i].Name = DefaultAccessProviderName
			}
			return &c.Access.Providers[i]
		}
	}
	return nil
}

// MakeInlineAPIKeyProvider constructs an inline API key provider configuration.
// It returns nil when no keys are supplied.
func MakeInlineAPIKeyProvider(keys []string) *AccessProvider {
	if len(keys) == 0 {
		return nil
	}
	provider := &AccessProvider{
		Name:    DefaultAccessProviderName,
		Type:    AccessProviderTypeConfigAPIKey,
		APIKeys: append([]string(nil), keys...),
	}
	return provider
}

// IntelligenceConfig defines settings for the Intelligent Routing system.
type IntelligenceConfig struct {
	// Enabled toggles the intelligent routing feature (master switch).
	// When false, all Phase 2 features are disabled.
	Enabled bool `yaml:"enabled" json:"enabled"`

	// RouterModel is the identifier of the LLM used for classification (e.g., "ollama:qwen:0.5b").
	RouterModel string `yaml:"router-model" json:"router-model"`

	// RouterFallback is the model used if the primary RouterModel fails or returns invalid result.
	RouterFallback string `yaml:"router-fallback" json:"router-fallback"`

	// Matrix defines the intent-to-model mapping patterns.
	// It is used by the Lua scripts to resolve abstract intents to specific models.
	Matrix map[string]string `yaml:"matrix,omitempty" json:"matrix,omitempty"`

	// SkillsPath defines the directory where Agent Skills are stored (SKILL.md).
	SkillsPath string `yaml:"skills-path" json:"skills-path"`

	// Discovery configures model discovery settings.
	Discovery DiscoveryConfig `yaml:"discovery,omitempty" json:"discovery,omitempty"`

	// CapabilityAnalysis enables automatic capability inference from model metadata.
	CapabilityAnalysis FeatureFlag `yaml:"capability-analysis,omitempty" json:"capability-analysis,omitempty"`

	// AutoAssign configures automatic model-to-capability slot assignment.
	AutoAssign AutoAssignConfig `yaml:"auto-assign,omitempty" json:"auto-assign,omitempty"`

	// Skills configures the skill registry.
	Skills SkillsConfig `yaml:"skills,omitempty" json:"skills,omitempty"`

	// Embedding configures the embedding engine.
	Embedding EmbeddingConfig `yaml:"embedding,omitempty" json:"embedding,omitempty"`

	// SemanticTier configures semantic intent matching.
	SemanticTier SemanticTierConfig `yaml:"semantic-tier,omitempty" json:"semantic-tier,omitempty"`

	// SkillMatching configures skill matching behavior.
	SkillMatching SkillMatchingConfig `yaml:"skill-matching,omitempty" json:"skill-matching,omitempty"`

	// SemanticCache configures semantic caching.
	SemanticCache SemanticCacheConfig `yaml:"semantic-cache,omitempty" json:"semantic-cache,omitempty"`

	// Confidence enables confidence scoring for classifications.
	Confidence FeatureFlag `yaml:"confidence,omitempty" json:"confidence,omitempty"`

	// Verification configures classification verification.
	Verification VerificationConfig `yaml:"verification,omitempty" json:"verification,omitempty"`

	// Cascade configures model cascading behavior.
	Cascade CascadeConfig `yaml:"cascade,omitempty" json:"cascade,omitempty"`

	// Feedback configures feedback collection.
	Feedback FeedbackConfig `yaml:"feedback,omitempty" json:"feedback,omitempty"`

	// Steering configures context-aware routing rules.
	Steering SteeringConfig `yaml:"steering,omitempty" json:"steering,omitempty"`

	// Hooks configures the event-driven automation system.
	Hooks HooksConfig `yaml:"hooks,omitempty" json:"hooks,omitempty"`

	// Learning configures the adaptive learning engine.
	Learning LearningConfig `yaml:"learning,omitempty" json:"learning,omitempty"`
}

// FeatureFlag represents a simple on/off toggle for a feature.
type FeatureFlag struct {
	Enabled bool `yaml:"enabled" json:"enabled"`
}

// DiscoveryConfig configures model discovery behavior.
type DiscoveryConfig struct {
	Enabled         bool   `yaml:"enabled" json:"enabled"`
	RefreshInterval int    `yaml:"refresh-interval,omitempty" json:"refresh-interval,omitempty"` // seconds
	CacheDir        string `yaml:"cache-dir,omitempty" json:"cache-dir,omitempty"`
}

// AutoAssignConfig configures automatic model assignment.
type AutoAssignConfig struct {
	Enabled          bool              `yaml:"enabled" json:"enabled"`
	PreferLocal      bool              `yaml:"prefer-local,omitempty" json:"prefer-local,omitempty"`
	CostOptimization bool              `yaml:"cost-optimization,omitempty" json:"cost-optimization,omitempty"`
	Overrides        map[string]string `yaml:"overrides,omitempty" json:"overrides,omitempty"`
}

// SkillsConfig configures the skill registry.
type SkillsConfig struct {
	Enabled   bool   `yaml:"enabled" json:"enabled"`
	Directory string `yaml:"directory,omitempty" json:"directory,omitempty"`
}

// EmbeddingConfig configures the embedding engine.
type EmbeddingConfig struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	Model   string `yaml:"model,omitempty" json:"model,omitempty"`
}

// SemanticTierConfig configures semantic intent matching.
type SemanticTierConfig struct {
	Enabled             bool    `yaml:"enabled" json:"enabled"`
	ConfidenceThreshold float64 `yaml:"confidence-threshold,omitempty" json:"confidence-threshold,omitempty"`
}

// SkillMatchingConfig configures skill matching behavior.
type SkillMatchingConfig struct {
	Enabled             bool    `yaml:"enabled" json:"enabled"`
	ConfidenceThreshold float64 `yaml:"confidence-threshold,omitempty" json:"confidence-threshold,omitempty"`
}

// SemanticCacheConfig configures semantic caching.
type SemanticCacheConfig struct {
	Enabled             bool    `yaml:"enabled" json:"enabled"`
	SimilarityThreshold float64 `yaml:"similarity-threshold,omitempty" json:"similarity-threshold,omitempty"`
	MaxSize             int     `yaml:"max-size,omitempty" json:"max-size,omitempty"`
}

// VerificationConfig configures classification verification.
type VerificationConfig struct {
	Enabled                 bool    `yaml:"enabled" json:"enabled"`
	ConfidenceThresholdLow  float64 `yaml:"confidence-threshold-low,omitempty" json:"confidence-threshold-low,omitempty"`
	ConfidenceThresholdHigh float64 `yaml:"confidence-threshold-high,omitempty" json:"confidence-threshold-high,omitempty"`
}

// CascadeConfig configures model cascading.
type CascadeConfig struct {
	Enabled          bool    `yaml:"enabled" json:"enabled"`
	QualityThreshold float64 `yaml:"quality-threshold,omitempty" json:"quality-threshold,omitempty"`
}

// FeedbackConfig configures feedback collection.
type FeedbackConfig struct {
	Enabled       bool `yaml:"enabled" json:"enabled"`
	RetentionDays int  `yaml:"retention-days,omitempty" json:"retention-days,omitempty"`
}

// SteeringConfig configures the steering engine.
type SteeringConfig struct {
	Enabled     bool   `yaml:"enabled" json:"enabled"`
	SteeringDir string `yaml:"steering-dir,omitempty" json:"steering-dir,omitempty"`
	RulesDir    string `yaml:"rules-dir,omitempty" json:"rules-dir,omitempty"` // Alias for SteeringDir
	HotReload   bool   `yaml:"hot-reload,omitempty" json:"hot-reload,omitempty"`
}

// HooksConfig configures the hook system.
type HooksConfig struct {
	Enabled   bool   `yaml:"enabled" json:"enabled"`
	HooksDir  string `yaml:"hooks-dir,omitempty" json:"hooks-dir,omitempty"`
	HotReload bool   `yaml:"hot-reload,omitempty" json:"hot-reload,omitempty"`
}

// LearningConfig configures the learning engine.
type LearningConfig struct {
	Enabled             bool    `yaml:"enabled" json:"enabled"`
	MinSampleSize       int     `yaml:"min-sample-size,omitempty" json:"min-sample-size,omitempty"`
	ConfidenceThreshold float64 `yaml:"confidence-threshold,omitempty" json:"confidence-threshold,omitempty"`
	AutoApply           bool    `yaml:"auto-apply,omitempty" json:"auto-apply,omitempty"`
	AnalysisInterval    string  `yaml:"analysis-interval,omitempty" json:"analysis-interval,omitempty"`
}

// SanitizeIntelligence normalizes the intelligence routing configuration.
func (c *SDKConfig) SanitizeIntelligence() {
	if c == nil {
		return
	}

	// Existing v1.0 fields
	c.Intelligence.RouterModel = strings.TrimSpace(c.Intelligence.RouterModel)
	c.Intelligence.RouterFallback = strings.TrimSpace(c.Intelligence.RouterFallback)
	if c.Intelligence.RouterModel == "" {
		c.Intelligence.RouterModel = "ollama:qwen:0.5b"
	}
	if c.Intelligence.RouterFallback == "" {
		c.Intelligence.RouterFallback = "openai:gpt-4o-mini"
	}
	if c.Intelligence.Matrix == nil {
		c.Intelligence.Matrix = make(map[string]string)
	}
	for k, v := range c.Intelligence.Matrix {
		cleanK := strings.TrimSpace(k)
		cleanV := strings.TrimSpace(v)
		if cleanK != "" {
			delete(c.Intelligence.Matrix, k)
			c.Intelligence.Matrix[cleanK] = cleanV
		}
	}

	// Phase 2 defaults (all disabled by default when master switch is off)
	// Discovery defaults
	if c.Intelligence.Discovery.RefreshInterval == 0 {
		c.Intelligence.Discovery.RefreshInterval = 3600 // 1 hour
	}
	if c.Intelligence.Discovery.CacheDir == "" {
		c.Intelligence.Discovery.CacheDir = "~/.switchailocal/cache/discovery"
	}

	// Auto-assign defaults
	if c.Intelligence.AutoAssign.Overrides == nil {
		c.Intelligence.AutoAssign.Overrides = make(map[string]string)
	}

	// Skills defaults
	if c.Intelligence.Skills.Directory == "" {
		c.Intelligence.Skills.Directory = "plugins/cortex-router/skills"
	}

	// Embedding defaults
	if c.Intelligence.Embedding.Model == "" {
		c.Intelligence.Embedding.Model = "all-MiniLM-L6-v2"
	}

	// Semantic tier defaults
	if c.Intelligence.SemanticTier.ConfidenceThreshold == 0 {
		c.Intelligence.SemanticTier.ConfidenceThreshold = 0.85
	}

	// Skill matching defaults
	if c.Intelligence.SkillMatching.ConfidenceThreshold == 0 {
		c.Intelligence.SkillMatching.ConfidenceThreshold = 0.80
	}

	// Semantic cache defaults
	if c.Intelligence.SemanticCache.SimilarityThreshold == 0 {
		c.Intelligence.SemanticCache.SimilarityThreshold = 0.95
	}
	if c.Intelligence.SemanticCache.MaxSize == 0 {
		c.Intelligence.SemanticCache.MaxSize = 10000
	}

	// Verification defaults
	if c.Intelligence.Verification.ConfidenceThresholdLow == 0 {
		c.Intelligence.Verification.ConfidenceThresholdLow = 0.60
	}
	if c.Intelligence.Verification.ConfidenceThresholdHigh == 0 {
		c.Intelligence.Verification.ConfidenceThresholdHigh = 0.90
	}

	// Cascade defaults
	if c.Intelligence.Cascade.QualityThreshold == 0 {
		c.Intelligence.Cascade.QualityThreshold = 0.70
	}

	// Feedback defaults
	if c.Intelligence.Feedback.RetentionDays == 0 {
		c.Intelligence.Feedback.RetentionDays = 90
	}

	// Steering defaults
	if c.Intelligence.Steering.SteeringDir == "" && c.Intelligence.Steering.RulesDir == "" {
		c.Intelligence.Steering.RulesDir = ".switchailocal/steering"
		c.Intelligence.Steering.SteeringDir = ".switchailocal/steering"
	} else if c.Intelligence.Steering.RulesDir == "" {
		c.Intelligence.Steering.RulesDir = c.Intelligence.Steering.SteeringDir
	} else if c.Intelligence.Steering.SteeringDir == "" {
		c.Intelligence.Steering.SteeringDir = c.Intelligence.Steering.RulesDir
	}

	// Hooks defaults
	if c.Intelligence.Hooks.HooksDir == "" {
		c.Intelligence.Hooks.HooksDir = ".switchailocal/hooks"
	}

	// Learning defaults
	if c.Intelligence.Learning.MinSampleSize == 0 {
		c.Intelligence.Learning.MinSampleSize = 100
	}
	if c.Intelligence.Learning.ConfidenceThreshold == 0 {
		c.Intelligence.Learning.ConfidenceThreshold = 0.85
	}
	if c.Intelligence.Learning.AnalysisInterval == "" {
		c.Intelligence.Learning.AnalysisInterval = "24h"
	}
}
