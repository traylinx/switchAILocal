// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package intelligence

// DiscoveryServiceInterface defines the interface for model discovery operations.
// This interface is used by the Lua plugin engine to access discovery functionality.
type DiscoveryServiceInterface interface {
	GetAvailableModelsAsMap() []map[string]interface{}
}

// MatrixBuilderInterface defines the interface for dynamic matrix operations.
// This interface is used by the Lua plugin engine to access matrix functionality.
type MatrixBuilderInterface interface {
	GetCurrentMatrixAsMap() map[string]interface{}
}

// SemanticMatchResult represents the result of semantic intent matching.
// This is a copy of semantic.MatchResult to avoid circular imports.
type SemanticMatchResult struct {
	Intent     string  `json:"intent"`
	Confidence float64 `json:"confidence"`
	LatencyMs  int64   `json:"latency_ms"`
}

// SemanticTierInterface defines the interface for semantic intent matching.
// This interface is used by the Lua plugin engine to access semantic tier functionality.
type SemanticTierInterface interface {
	MatchIntent(query string) (*SemanticMatchResult, error)
	IsEnabled() bool
	GetMetrics() map[string]interface{}
}

// CacheEntry represents a cached routing decision.
// This is a copy of cache.CacheEntry to avoid circular imports.
type CacheEntry struct {
	Query     string
	Decision  string
	Metadata  map[string]interface{}
	Timestamp string
}

// SemanticCacheInterface defines the interface for semantic caching operations.
// This interface is used by the Lua plugin engine to access cache functionality.
type SemanticCacheInterface interface {
	Lookup(query string) (interface{}, error)
	Store(query string, embedding []float32, decision string, metadata map[string]interface{}) error
	IsEnabled() bool
	GetMetricsAsMap() map[string]interface{}
}

// CascadeDecision represents the outcome of a cascade evaluation.
// This is a copy of cascade.CascadeDecision to avoid circular imports.
type CascadeDecision struct {
	ShouldCascade bool                   `json:"should_cascade"`
	CurrentTier   string                 `json:"current_tier"`
	NextTier      string                 `json:"next_tier,omitempty"`
	Signals       []CascadeQualitySignal `json:"signals,omitempty"`
	QualityScore  float64                `json:"quality_score"`
	Reason        string                 `json:"reason"`
}

// CascadeQualitySignal represents a detected quality issue.
type CascadeQualitySignal struct {
	Type        string  `json:"type"`
	Severity    float64 `json:"severity"`
	Description string  `json:"description"`
}

// CascadeManagerInterface defines the interface for model cascading operations.
// This interface is used by the Lua plugin engine to access cascade functionality.
type CascadeManagerInterface interface {
	IsEnabled() bool
	EvaluateResponse(response string, currentTier string) *CascadeDecision
	GetMetricsAsMap() map[string]interface{}
}

// FeedbackRecord represents a single routing feedback entry.
// This is a copy of feedback.FeedbackRecord to avoid circular imports.
type FeedbackRecord struct {
	ID              int64                  `json:"id"`
	Timestamp       string                 `json:"timestamp"`
	Query           string                 `json:"query"`
	Intent          string                 `json:"intent"`
	SelectedModel   string                 `json:"selected_model"`
	RoutingTier     string                 `json:"routing_tier"`
	Confidence      float64                `json:"confidence"`
	MatchedSkill    string                 `json:"matched_skill,omitempty"`
	CascadeOccurred bool                   `json:"cascade_occurred"`
	ResponseQuality float64                `json:"response_quality,omitempty"`
	LatencyMs       int64                  `json:"latency_ms"`
	Success         bool                   `json:"success"`
	ErrorMessage    string                 `json:"error_message,omitempty"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

// FeedbackCollectorInterface defines the interface for feedback collection operations.
// This interface is used by the Lua plugin engine to access feedback functionality.
type FeedbackCollectorInterface interface {
	IsEnabled() bool
	Record(ctx interface{}, record *FeedbackRecord) error
	GetStats(ctx interface{}) (map[string]interface{}, error)
	GetRecent(ctx interface{}, limit int) (interface{}, error)
}
