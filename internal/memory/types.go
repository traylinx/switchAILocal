// Package memory provides persistent storage for routing decisions, provider quirks, and user preferences.
// This enables switchAILocal to learn from past decisions and improve routing over time.
package memory

import (
	"time"
)

// RoutingDecision represents a complete routing decision with its outcome.
// This is the core data structure for learning and analytics.
type RoutingDecision struct {
	Timestamp  time.Time   `json:"timestamp"`
	APIKeyHash string      `json:"api_key_hash"`
	Request    RequestInfo `json:"request"`
	Routing    RoutingInfo `json:"routing"`
	Outcome    OutcomeInfo `json:"outcome"`
}

// RequestInfo contains information about the incoming request.
type RequestInfo struct {
	Model         string `json:"model"`
	Intent        string `json:"intent"`
	ContentHash   string `json:"content_hash"`
	ContentLength int    `json:"content_length"`
}

// RoutingInfo contains information about the routing decision.
type RoutingInfo struct {
	Tier          string  `json:"tier"`           // reflex, semantic, cognitive, learned
	SelectedModel string  `json:"selected_model"` // e.g., "claudecli:claude-sonnet-4"
	Confidence    float64 `json:"confidence"`     // 0.0 to 1.0
	LatencyMs     int64   `json:"latency_ms"`     // routing decision latency
}

// OutcomeInfo contains information about the request outcome.
type OutcomeInfo struct {
	Success        bool    `json:"success"`
	ResponseTimeMs int64   `json:"response_time_ms"`
	Error          string  `json:"error,omitempty"`
	QualityScore   float64 `json:"quality_score"` // 0.0 to 1.0
}

// UserPreferences represents learned preferences for a specific API key.
type UserPreferences struct {
	APIKeyHash       string             `json:"api_key_hash"`
	LastUpdated      time.Time          `json:"last_updated"`
	LastAnalyzed     time.Time          `json:"last_analyzed,omitempty"`
	ModelPreferences map[string]string  `json:"model_preferences"` // intent -> model
	ModelConfidences map[string]float64 `json:"model_confidences"` // intent -> confidence
	ProviderBias     map[string]float64 `json:"provider_bias"`     // provider -> bias (-1.0 to 1.0)
	CustomRules      []PreferenceRule   `json:"custom_rules"`
}

// PreferenceRule represents a user-defined routing preference rule.
type PreferenceRule struct {
	Condition string `json:"condition"` // e.g., "intent == 'coding' && hour >= 9"
	Model     string `json:"model"`
	Priority  int    `json:"priority"`
}

// Quirk represents a known provider issue and its workaround.
type Quirk struct {
	Provider   string    `json:"provider"`
	Issue      string    `json:"issue"`
	Workaround string    `json:"workaround"`
	Discovered time.Time `json:"discovered"`
	Frequency  string    `json:"frequency"` // e.g., "3/10 startups", "daily during peak"
	Severity   string    `json:"severity"`  // low, medium, high, critical
}

// ProviderStats represents aggregated statistics for a provider.
type ProviderStats struct {
	Provider      string    `json:"provider"`
	TotalRequests int       `json:"total_requests"`
	SuccessRate   float64   `json:"success_rate"`
	AvgLatencyMs  float64   `json:"avg_latency_ms"`
	ErrorRate     float64   `json:"error_rate"`
	LastUpdated   time.Time `json:"last_updated"`
}

// ModelPerformance represents performance metrics for a specific model.
type ModelPerformance struct {
	Model           string  `json:"model"`
	TotalRequests   int     `json:"total_requests"`
	SuccessRate     float64 `json:"success_rate"`
	AvgQualityScore float64 `json:"avg_quality_score"`
	AvgCostPerReq   float64 `json:"avg_cost_per_req"`
}

// MemoryConfig represents configuration for the memory system.
type MemoryConfig struct {
	Enabled       bool   `yaml:"enabled"`
	BaseDir       string `yaml:"base_dir"`
	RetentionDays int    `yaml:"retention_days"`
	MaxLogSizeMB  int    `yaml:"max_log_size_mb"`
	Compression   bool   `yaml:"compression"`
}

// DefaultMemoryConfig returns the default memory configuration.
func DefaultMemoryConfig() *MemoryConfig {
	return &MemoryConfig{
		Enabled:       false, // Opt-in by default
		BaseDir:       ".switchailocal/memory",
		RetentionDays: 90,
		MaxLogSizeMB:  100,
		Compression:   true,
	}
}

// CalculateQualityScore computes a quality score for a request outcome.
// The score is based on success, response time, and error presence.
// Returns a value between 0.0 (worst) and 1.0 (best).
func CalculateQualityScore(success bool, responseTimeMs int64, hasError bool) float64 {
	// Base score starts at 1.0 for success, 0.0 for failure
	score := 0.0
	if success {
		score = 1.0
	}

	// Penalize for errors even if technically "successful"
	if hasError {
		score -= 0.3
	}

	// Penalize for slow response times
	// Target: < 2s = no penalty, > 10s = max penalty
	const (
		targetResponseMs = 2000  // 2 seconds
		maxResponseMs    = 10000 // 10 seconds
	)

	if responseTimeMs > targetResponseMs {
		// Calculate penalty based on how much over target
		overTime := float64(responseTimeMs - targetResponseMs)
		maxOverTime := float64(maxResponseMs - targetResponseMs)
		timePenalty := (overTime / maxOverTime) * 0.3 // Max 30% penalty for time

		// Cap the penalty at 0.3
		if timePenalty > 0.3 {
			timePenalty = 0.3
		}

		score -= timePenalty
	}

	// Clamp score between 0.0 and 1.0
	if score < 0.0 {
		score = 0.0
	} else if score > 1.0 {
		score = 1.0
	}

	return score
}
