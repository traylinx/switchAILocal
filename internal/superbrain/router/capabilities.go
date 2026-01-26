// Package router provides intelligent failover routing between AI providers.
// It tracks provider capabilities and statistics to make informed routing decisions.
package router

import (
	"time"
)

// ProviderCapability describes what a provider can handle.
type ProviderCapability struct {
	// Provider is the unique identifier for this provider (e.g., "claudecli", "gemini").
	Provider string

	// MaxContextSize is the maximum number of tokens the provider can handle.
	MaxContextSize int

	// SupportsStream indicates whether the provider supports streaming responses.
	SupportsStream bool

	// SupportsCLI indicates whether the provider is a CLI-based tool.
	SupportsCLI bool

	// SuccessRate is the historical success rate (0.0 to 1.0).
	SuccessRate float64

	// AverageLatency is the average response time for this provider.
	AverageLatency time.Duration

	// IsAvailable indicates whether the provider is currently usable.
	IsAvailable bool
}

// DefaultProviderCapabilities returns the default capabilities for known providers.
var DefaultProviderCapabilities = map[string]*ProviderCapability{
	"claudecli": {
		Provider:       "claudecli",
		MaxContextSize: 200000,
		SupportsStream: true,
		SupportsCLI:    true,
		SuccessRate:    1.0,
		IsAvailable:    true,
	},
	"geminicli": {
		Provider:       "geminicli",
		MaxContextSize: 1000000,
		SupportsStream: true,
		SupportsCLI:    true,
		SuccessRate:    1.0,
		IsAvailable:    true,
	},
	"gemini": {
		Provider:       "gemini",
		MaxContextSize: 1000000,
		SupportsStream: true,
		SupportsCLI:    false,
		SuccessRate:    1.0,
		IsAvailable:    true,
	},
	"ollama": {
		Provider:       "ollama",
		MaxContextSize: 128000,
		SupportsStream: true,
		SupportsCLI:    false,
		SuccessRate:    1.0,
		IsAvailable:    true,
	},
	"openai": {
		Provider:       "openai",
		MaxContextSize: 128000,
		SupportsStream: true,
		SupportsCLI:    false,
		SuccessRate:    1.0,
		IsAvailable:    true,
	},
	"lmstudio": {
		Provider:       "lmstudio",
		MaxContextSize: 32000,
		SupportsStream: true,
		SupportsCLI:    false,
		SuccessRate:    1.0,
		IsAvailable:    true,
	},
}

// CapabilityRegistry manages provider capabilities.
type CapabilityRegistry struct {
	capabilities map[string]*ProviderCapability
}

// NewCapabilityRegistry creates a new registry with default capabilities.
func NewCapabilityRegistry() *CapabilityRegistry {
	caps := make(map[string]*ProviderCapability)
	for k, v := range DefaultProviderCapabilities {
		// Deep copy to avoid modifying defaults
		capCopy := *v
		caps[k] = &capCopy
	}
	return &CapabilityRegistry{capabilities: caps}
}

// GetCapability returns the capability for a provider, or nil if not found.
func (r *CapabilityRegistry) GetCapability(provider string) *ProviderCapability {
	return r.capabilities[provider]
}

// SetCapability sets or updates the capability for a provider.
func (r *CapabilityRegistry) SetCapability(cap *ProviderCapability) {
	r.capabilities[cap.Provider] = cap
}

// GetAllCapabilities returns all registered capabilities.
func (r *CapabilityRegistry) GetAllCapabilities() []*ProviderCapability {
	caps := make([]*ProviderCapability, 0, len(r.capabilities))
	for _, cap := range r.capabilities {
		caps = append(caps, cap)
	}
	return caps
}

// UpdateSuccessRate updates the success rate for a provider.
func (r *CapabilityRegistry) UpdateSuccessRate(provider string, rate float64) {
	if cap, ok := r.capabilities[provider]; ok {
		cap.SuccessRate = rate
	}
}

// UpdateAvailability updates the availability status for a provider.
func (r *CapabilityRegistry) UpdateAvailability(provider string, available bool) {
	if cap, ok := r.capabilities[provider]; ok {
		cap.IsAvailable = available
	}
}

// UpdateLatency updates the average latency for a provider.
func (r *CapabilityRegistry) UpdateLatency(provider string, latency time.Duration) {
	if cap, ok := r.capabilities[provider]; ok {
		cap.AverageLatency = latency
	}
}
