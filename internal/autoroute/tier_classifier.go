package autoroute

import (
	"strings"
)

// ClassifyTier analyzes the probe result to determine the provider's subscription tier.
// Returns "free", "standard", or "premium".
func ClassifyTier(probe ProbeResult) string {
	// 1. Explicit subscription info from CLI (e.g. claude account)
	if probe.SubscriptionInfo != nil && probe.SubscriptionInfo.Tier != "" && probe.SubscriptionInfo.Tier != AuthTypeUnknown {
		return mapSubscriptionToTier(probe.SubscriptionInfo.Tier)
	}

	// 2. Rate-limit header inference (API providers)
	if probe.RateLimits != nil {
		return inferTierFromRateLimits(probe.Provider, probe.RateLimits)
	}

	// 3. Auth type heuristics
	switch probe.AuthType {
	case AuthTypeOAuth:
		// OAuth-based CLI usually means a human user actively authenticated
		return "standard"
	case AuthTypeAPIKey:
		// API key alone doesn't tell us much without rate limits, assume standard to be safe
		return "standard"
	case AuthTypeLocal:
		// Local models are effectively "free" regarding budget, but "local" tier handles them
		return "local"
	}

	// Conservative default
	return "free"
}

// mapSubscriptionToTier standardizes vendor-specific tier names into our internal tiers
func mapSubscriptionToTier(rawTier string) string {
	lower := strings.ToLower(strings.TrimSpace(rawTier))

	switch {
	case strings.Contains(lower, "pro"), strings.Contains(lower, "premium"), strings.Contains(lower, "enterprise"), strings.Contains(lower, "team"), strings.Contains(lower, "paid"):
		return "premium"
	case strings.Contains(lower, "free"), strings.Contains(lower, "hobby"):
		return "free"
	}

	return "standard"
}

// inferTierFromRateLimits guesses the subscription tier based on quota headers
func inferTierFromRateLimits(provider string, rl *RateLimits) string {
	provider = strings.ToLower(provider)

	// Claude rules
	if strings.Contains(provider, "claude") {
		// Anthropic limits: Free (5 RPM), Pro (50 RPM), Team (100 RPM)
		if rl.RequestsPerMinute >= 100 {
			return "premium"
		}
		if rl.RequestsPerMinute >= 40 {
			return "standard"
		}
		return "free"
	}

	// OpenAI / Groq / Generic rules
	// High-tier accounts usually have >500 RPM
	if rl.RequestsPerMinute >= 500 {
		return "premium"
	}
	if rl.RequestsPerMinute >= 50 {
		return "standard"
	}

	return "free"
}
