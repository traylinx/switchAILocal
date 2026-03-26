package autoroute

import "strings"

// Tier constants define the provider subscription levels.
const (
	TierPremium  = "premium"
	TierStandard = "standard"
	TierFree     = "free"
	TierLocal    = "local"
	TierReserve  = "reserve"
)

// tierBoostMap maps tier names to their scoring boost values.
// These additive boosts ensure paid providers are preferred for complex tasks.
var tierBoostMap = map[string]float64{
	TierPremium:  0.30,
	TierStandard: 0.15,
	TierFree:     0.00,
	TierLocal:    0.05,
	TierReserve:  -0.10,
}

// TierBoost returns the additive score boost for a given tier.
func TierBoost(tier string) float64 {
	if boost, ok := tierBoostMap[tier]; ok {
		return boost
	}
	return 0.0
}

// GetEffectiveTier resolves the effective tier for a model.
// Model-level overrides (e.g., ollama model-tiers) take precedence
// over provider-level tiers.
func GetEffectiveTier(model, provider string, cfg Config) string {
	// Check for model-level override first
	if provCfg, ok := cfg.Providers[provider]; ok {
		// Extract the model name part (after "provider:") for model-tier lookup
		modelName := model
		if idx := strings.Index(model, ":"); idx >= 0 {
			modelName = model[idx+1:]
		}
		if modelTier, ok := provCfg.ModelTiers[modelName]; ok {
			return modelTier
		}
		// Fall back to provider-level tier
		if provCfg.Tier != "" {
			return provCfg.Tier
		}
	}

	// Auto-detect tier if not configured
	return AutoDetectTier(provider)
}

// AutoDetectTier infers a provider's tier from its name when no explicit config exists.
func AutoDetectTier(provider string) string {
	if strings.HasSuffix(provider, "cli") {
		return TierStandard // CLI-based = OAuth login = likely paid
	}

	switch provider {
	case "ollama-local", "lmstudio":
		return TierLocal
	case "switchai":
		return TierFree
	case "gemini", "claude", "codex":
		return TierStandard // API key = user went through setup
	default:
		return TierFree
	}
}

// PreferenceBoost returns the additive preference boost for a specific model ID.
// The raw preference (0.0-1.0) is scaled by 0.20 to limit its influence to 20%.
func PreferenceBoost(modelID string, preferences []ModelPreference) float64 {
	for _, pref := range preferences {
		if pref.Model == modelID {
			return pref.Preference * 0.20
		}
	}
	return 0.0
}

// ConservationMultiplier adjusts the final score based on task complexity
// and provider tier. This prevents wasting premium tokens on trivial tasks.
func ConservationMultiplier(tier string, complexity float64, cfg ConservationConfig) float64 {
	if !cfg.Enabled {
		return 1.0
	}

	threshold := float64(cfg.SimpleThreshold) / 1000.0 // normalize to 0.0-1.0 range
	if threshold <= 0 {
		threshold = 0.30 // default: 30% complexity threshold
	}

	// Simple task + premium provider = penalize
	if complexity < threshold && tier == TierPremium {
		return 0.3
	}

	// Simple task + free/local provider = bonus
	if complexity < threshold && (tier == TierFree || tier == TierLocal) {
		return 1.5
	}

	// Complex task + premium = extra boost
	if complexity > 0.7 && tier == TierPremium {
		return 1.2
	}

	return 1.0
}
