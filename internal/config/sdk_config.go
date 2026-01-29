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
	// Enabled toggles the intelligent routing feature.
	Enabled bool `yaml:"enabled" json:"enabled"`

	// RouterModel is the identifier of the LLM used for classification (e.g., "ollama:qwen:0.5b").
	RouterModel string `yaml:"router-model" json:"router-model"`

	// RouterFallback is the model used if the primary RouterModel fails or returns invalid result.
	RouterFallback string `yaml:"router-fallback" json:"router-fallback"`

	// Matrix defines the intent-to-model mapping patterns.
	// It is used by the Lua scripts to resolve abstract intents to specific models.
	Matrix map[string]string `yaml:"matrix,omitempty" json:"matrix,omitempty"`
}

// SanitizeIntelligence normalizes the intelligence routing configuration.
func (c *SDKConfig) SanitizeIntelligence() {
	if c == nil {
		return
	}
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
}
