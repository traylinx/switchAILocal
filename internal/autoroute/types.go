package autoroute

import "time"

// ScoredCandidate represents a model that has been evaluated and scored by the ProviderScorer.
type ScoredCandidate struct {
	// Model is the canonical internal model identifier (e.g., "geminicli:gemini-3.1-pro")
	Model string `json:"model"`

	// Provider is the extracted provider name (e.g., "geminicli")
	Provider string `json:"provider"`

	// Score Breakdown
	FinalScore       float64 `json:"final_score"`
	BaseScore        float64 `json:"base_score"`
	TierBoost        float64 `json:"tier_boost"`
	PreferenceBoost  float64 `json:"preference_boost"`
	ConservationMult float64 `json:"conservation_multiplier"`

	// EffectiveTier is the final assigned tier (e.g., "premium", "free") after resolving overrides
	EffectiveTier string `json:"effective_tier"`

	// Available tracks if the health monitor considers this candidate routable
	Available bool `json:"available"`

	// EstimatedLatency via health monitor EMAs
	EstimatedLatency time.Duration `json:"estimated_latency_ms"`
}

// RoutingDecision represents the final selected strategy determined by the AutoResolver.
type RoutingDecision struct {
	// SelectedModel is the primary model chosen for the request
	SelectedModel string `json:"selected_model"`

	// FallbackChain is an ordered list of models to try if the primary fails
	FallbackChain []FallbackEntry `json:"fallback_chain"`

	// Intent hints or classifications (e.g., "coding", "creative")
	Intent string `json:"intent"`

	// EstimatedComplexity of the prompt (0.0 to 1.0)
	EstimatedComplexity float64 `json:"estimated_complexity"`

	// Candidates evaluated (useful for debugging/dashboard)
	Candidates []ScoredCandidate `json:"candidates,omitempty"`

	// OriginalInputs preserves the raw CandidateInput values at decision time,
	// enabling the Lab to replay exact conditions for shadow scoring (H1 fix).
	OriginalInputs []CandidateInput `json:"-"`

	// ResolutionLatency tracks how long the scoring process took
	ResolutionLatency time.Duration `json:"resolution_latency_ms"`
}

// FallbackEntry represents a selected provider/model combination in the execution chain.
type FallbackEntry struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Tier     string `json:"tier"`
}

