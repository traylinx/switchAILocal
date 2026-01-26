// Package router provides intelligent failover routing for the Superbrain system.
// It manages provider selection based on capabilities, availability, and historical
// success rates to ensure requests are routed to the most suitable alternative provider.
package router

import (
	"context"
	"errors"
	"time"

	"github.com/traylinx/switchAILocal/internal/config"
)

var (
	// ErrNoFallbackAvailable indicates no suitable fallback provider was found.
	ErrNoFallbackAvailable = errors.New("no fallback provider available")

	// ErrProviderNotConfigured indicates the provider is not in the configured list.
	ErrProviderNotConfigured = errors.New("provider not configured for fallback")
)

// FallbackDecision contains routing decision details.
type FallbackDecision struct {
	// OriginalProvider is the provider that failed.
	OriginalProvider string

	// FallbackProvider is the selected alternative provider.
	FallbackProvider string

	// Reason explains why this fallback was selected.
	Reason string

	// CapabilityMatch indicates how well the fallback matches requirements (0.0 to 1.0).
	CapabilityMatch float64

	// AdaptedRequest contains the request adapted for the fallback provider (if adaptation was performed).
	AdaptedRequest *AdaptedRequest
}

// RequestRequirements specifies what capabilities a request needs.
type RequestRequirements struct {
	// RequiresStream indicates the request needs streaming support.
	RequiresStream bool

	// RequiresCLI indicates the request needs CLI execution.
	RequiresCLI bool

	// MinContextSize is the minimum context size needed.
	MinContextSize int
}

// FallbackRouter manages intelligent failover between providers.
type FallbackRouter struct {
	config       *config.FallbackConfig
	capabilities *CapabilityRegistry
	stats        *StatsTracker
	adapter      *RequestAdapter
}

// NewFallbackRouter creates a new fallback router with the given configuration.
func NewFallbackRouter(cfg *config.FallbackConfig) *FallbackRouter {
	return &FallbackRouter{
		config:       cfg,
		capabilities: NewCapabilityRegistry(),
		stats:        NewStatsTracker(),
		adapter:      NewRequestAdapter(),
	}
}

// GetFallback finds an alternative provider for a failed request.
// It considers provider capabilities, availability, and success rate.
func (r *FallbackRouter) GetFallback(ctx context.Context, failedProvider string, requirements *RequestRequirements) (*FallbackDecision, error) {
	if !r.config.Enabled {
		return nil, ErrNoFallbackAvailable
	}

	// Find suitable providers from the configured list
	for _, provider := range r.config.Providers {
		// Skip the failed provider
		if provider == failedProvider {
			continue
		}

		// Check if provider meets requirements
		if r.isProviderSuitable(provider, requirements) {
			return &FallbackDecision{
				OriginalProvider: failedProvider,
				FallbackProvider: provider,
				Reason:           "Provider meets all requirements and has acceptable success rate",
				CapabilityMatch:  r.calculateCapabilityMatch(provider, requirements),
			}, nil
		}
	}

	return nil, ErrNoFallbackAvailable
}

// isProviderSuitable checks if a provider meets all requirements.
func (r *FallbackRouter) isProviderSuitable(provider string, requirements *RequestRequirements) bool {
	cap := r.capabilities.GetCapability(provider)
	if cap == nil {
		return false
	}

	// Check availability
	if !cap.IsAvailable {
		return false
	}

	// Check success rate against minimum threshold
	stats := r.stats.GetStats(provider)
	var successRate float64
	if stats != nil {
		successRate = stats.SuccessRate()
	} else {
		successRate = cap.SuccessRate // Use default if no stats
	}

	if successRate < r.config.MinSuccessRate {
		return false
	}

	// Check capability requirements
	if requirements != nil {
		if requirements.RequiresStream && !cap.SupportsStream {
			return false
		}
		if requirements.RequiresCLI && !cap.SupportsCLI {
			return false
		}
		if requirements.MinContextSize > 0 && cap.MaxContextSize < requirements.MinContextSize {
			return false
		}
	}

	return true
}

// calculateCapabilityMatch calculates how well a provider matches requirements.
func (r *FallbackRouter) calculateCapabilityMatch(provider string, requirements *RequestRequirements) float64 {
	cap := r.capabilities.GetCapability(provider)
	if cap == nil {
		return 0.0
	}

	// Start with base match
	match := 1.0

	// Adjust based on success rate
	stats := r.stats.GetStats(provider)
	if stats != nil {
		match *= stats.SuccessRate()
	}

	return match
}

// GetProviderCapabilities returns capabilities for all providers.
func (r *FallbackRouter) GetProviderCapabilities() []*ProviderCapability {
	return r.capabilities.GetAllCapabilities()
}

// UpdateProviderStats records success/failure for a provider.
func (r *FallbackRouter) UpdateProviderStats(provider string, success bool, latency time.Duration) {
	r.stats.UpdateStats(provider, success, latency, "")

	// Update capability registry with new success rate
	stats := r.stats.GetStats(provider)
	if stats != nil {
		r.capabilities.UpdateSuccessRate(provider, stats.SuccessRate())
	}
}

// IsProviderAvailable checks if a provider is currently usable.
func (r *FallbackRouter) IsProviderAvailable(provider string) bool {
	cap := r.capabilities.GetCapability(provider)
	if cap == nil {
		return false
	}
	return cap.IsAvailable
}

// IsProviderConfigured checks if a provider is in the configured fallback list.
func (r *FallbackRouter) IsProviderConfigured(provider string) bool {
	for _, p := range r.config.Providers {
		if p == provider {
			return true
		}
	}
	return false
}

// SetProviderAvailability updates the availability status for a provider.
func (r *FallbackRouter) SetProviderAvailability(provider string, available bool) {
	r.capabilities.UpdateAvailability(provider, available)
}

// GetCapabilityRegistry returns the capability registry for testing.
func (r *FallbackRouter) GetCapabilityRegistry() *CapabilityRegistry {
	return r.capabilities
}

// GetStatsTracker returns the stats tracker for testing.
func (r *FallbackRouter) GetStatsTracker() *StatsTracker {
	return r.stats
}

// AdaptRequest adapts a request for a fallback provider.
// It preserves the original request semantics (messages, streaming, capabilities)
// while adapting the model name and provider-specific fields.
func (r *FallbackRouter) AdaptRequest(originalPayload []byte, targetProvider string) (*AdaptedRequest, error) {
	return r.adapter.AdaptRequest(originalPayload, targetProvider)
}

// GetFallbackWithAdaptation finds an alternative provider and adapts the request.
// This is a convenience method that combines GetFallback and AdaptRequest.
func (r *FallbackRouter) GetFallbackWithAdaptation(ctx context.Context, failedProvider string, requirements *RequestRequirements, originalPayload []byte) (*FallbackDecision, error) {
	// First, find a suitable fallback provider
	decision, err := r.GetFallback(ctx, failedProvider, requirements)
	if err != nil {
		return nil, err
	}

	// Then, adapt the request for the fallback provider
	adaptedReq, err := r.AdaptRequest(originalPayload, decision.FallbackProvider)
	if err != nil {
		// If adaptation fails, still return the decision but without adapted request
		// The caller can decide how to handle this
		return decision, nil
	}

	decision.AdaptedRequest = adaptedReq
	return decision, nil
}

// GetRequestAdapter returns the request adapter for testing or custom configuration.
func (r *FallbackRouter) GetRequestAdapter() *RequestAdapter {
	return r.adapter
}
