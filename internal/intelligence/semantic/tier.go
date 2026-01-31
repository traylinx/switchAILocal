// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package semantic provides semantic intent matching for Phase 2 intelligent routing.
// It uses embedding similarity to match user queries to predefined intents.
package semantic

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/goccy/go-yaml"
	log "github.com/sirupsen/logrus"
)

// IntentDefinition represents a single intent with its metadata and examples.
type IntentDefinition struct {
	// Name is the unique identifier for the intent
	Name string `yaml:"name" json:"name"`

	// Description explains what the intent represents
	Description string `yaml:"description" json:"description"`

	// Examples are sample queries that match this intent
	Examples []string `yaml:"examples" json:"examples"`

	// Embedding is the pre-computed embedding vector for the intent
	// This is computed from the description and examples
	Embedding []float32 `yaml:"-" json:"-"`
}

// IntentsFile represents the structure of the intents.yaml file.
type IntentsFile struct {
	Intents []IntentDefinition `yaml:"intents"`
}

// MatchResult represents the result of semantic intent matching.
type MatchResult struct {
	// Intent is the matched intent name
	Intent string `json:"intent"`

	// Confidence is the similarity score (0.0-1.0)
	Confidence float64 `json:"confidence"`

	// LatencyMs is the time taken for matching in milliseconds
	LatencyMs int64 `json:"latency_ms"`
}


// EmbeddingEngine defines the interface for computing embeddings.
// This allows the semantic tier to work with the embedding engine.
type EmbeddingEngine interface {
	// Embed computes the embedding vector for a text
	Embed(text string) ([]float32, error)

	// CosineSimilarity computes the cosine similarity between two vectors
	CosineSimilarity(a, b []float32) float64

	// IsEnabled returns whether the engine is ready
	IsEnabled() bool
}

// Tier provides semantic intent matching using embedding similarity.
// It pre-computes intent embeddings at startup and matches queries against them.
type Tier struct {
	// engine is the embedding engine for computing embeddings
	engine EmbeddingEngine

	// intents holds all loaded intent definitions
	intents []*IntentDefinition

	// threshold is the minimum confidence score for a match
	threshold float64

	// enabled indicates whether the tier is ready
	enabled bool

	// mu protects concurrent access
	mu sync.RWMutex

	// metrics for tracking
	matchCount    int64
	hitCount      int64
	totalLatencyMs int64
}

// NewTier creates a new semantic tier instance.
//
// Parameters:
//   - engine: The embedding engine for computing embeddings
//   - threshold: Minimum confidence score for a match (default: 0.85)
//
// Returns:
//   - *Tier: A new tier instance
func NewTier(engine EmbeddingEngine, threshold float64) *Tier {
	if threshold <= 0 {
		threshold = 0.85 // Default threshold
	}

	return &Tier{
		engine:    engine,
		intents:   make([]*IntentDefinition, 0),
		threshold: threshold,
		enabled:   false,
	}
}

// Initialize loads intents from the YAML file and pre-computes embeddings.
//
// Parameters:
//   - intentsPath: Path to the intents.yaml file
//
// Returns:
//   - error: Any error encountered during initialization
func (t *Tier) Initialize(intentsPath string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.engine == nil || !t.engine.IsEnabled() {
		return fmt.Errorf("embedding engine not available")
	}

	// Read intents file
	data, err := os.ReadFile(intentsPath)
	if err != nil {
		return fmt.Errorf("failed to read intents file: %w", err)
	}

	// Parse YAML
	var intentsFile IntentsFile
	if err := yaml.Unmarshal(data, &intentsFile); err != nil {
		return fmt.Errorf("failed to parse intents file: %w", err)
	}

	if len(intentsFile.Intents) == 0 {
		return fmt.Errorf("no intents found in file")
	}

	log.Infof("Loading %d intents for semantic tier...", len(intentsFile.Intents))

	// Pre-compute embeddings for each intent
	t.intents = make([]*IntentDefinition, 0, len(intentsFile.Intents))
	for i := range intentsFile.Intents {
		intent := &intentsFile.Intents[i]

		// Create combined text for embedding (description + examples)
		combinedText := intent.Description
		for _, example := range intent.Examples {
			combinedText += " " + example
		}

		// Compute embedding
		embedding, err := t.engine.Embed(combinedText)
		if err != nil {
			log.Warnf("Failed to compute embedding for intent %s: %v", intent.Name, err)
			continue
		}

		intent.Embedding = embedding
		t.intents = append(t.intents, intent)
	}

	if len(t.intents) == 0 {
		return fmt.Errorf("failed to compute embeddings for any intents")
	}

	t.enabled = true
	log.Infof("Semantic tier initialized with %d intents", len(t.intents))

	return nil
}

// IsEnabled returns whether the semantic tier is ready for matching.
//
// Returns:
//   - bool: true if the tier is initialized and ready
func (t *Tier) IsEnabled() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.enabled
}

// MatchIntent finds the best matching intent for a query.
// Returns nil if no intent matches above the confidence threshold.
//
// Parameters:
//   - query: The user query to match
//
// Returns:
//   - *MatchResult: The match result, or nil if no match
//   - error: Any error encountered during matching
func (t *Tier) MatchIntent(query string) (*MatchResult, error) {
	start := time.Now()

	t.mu.RLock()
	defer t.mu.RUnlock()

	if !t.enabled {
		return nil, fmt.Errorf("semantic tier not initialized")
	}

	if t.engine == nil || !t.engine.IsEnabled() {
		return nil, fmt.Errorf("embedding engine not available")
	}

	// Compute query embedding
	queryEmbedding, err := t.engine.Embed(query)
	if err != nil {
		return nil, fmt.Errorf("failed to compute query embedding: %w", err)
	}

	// Find best matching intent
	var bestIntent *IntentDefinition
	var bestScore float64

	for _, intent := range t.intents {
		if len(intent.Embedding) == 0 {
			continue
		}

		similarity := t.engine.CosineSimilarity(queryEmbedding, intent.Embedding)
		if similarity > bestScore {
			bestScore = similarity
			bestIntent = intent
		}
	}

	latencyMs := time.Since(start).Milliseconds()

	// Update metrics (need write lock)
	t.mu.RUnlock()
	t.mu.Lock()
	t.matchCount++
	t.totalLatencyMs += latencyMs
	if bestIntent != nil && bestScore >= t.threshold {
		t.hitCount++
	}
	t.mu.Unlock()
	t.mu.RLock()

	// Check if best match exceeds threshold
	if bestIntent == nil || bestScore < t.threshold {
		return nil, nil // No match above threshold
	}

	return &MatchResult{
		Intent:     bestIntent.Name,
		Confidence: bestScore,
		LatencyMs:  latencyMs,
	}, nil
}


// GetIntents returns all loaded intents.
//
// Returns:
//   - []*IntentDefinition: A slice of all intents
func (t *Tier) GetIntents() []*IntentDefinition {
	t.mu.RLock()
	defer t.mu.RUnlock()

	// Return a copy to prevent external modification
	result := make([]*IntentDefinition, len(t.intents))
	copy(result, t.intents)
	return result
}

// GetIntentCount returns the number of loaded intents.
//
// Returns:
//   - int: The number of intents
func (t *Tier) GetIntentCount() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.intents)
}

// GetThreshold returns the confidence threshold.
//
// Returns:
//   - float64: The confidence threshold
func (t *Tier) GetThreshold() float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.threshold
}

// SetThreshold updates the confidence threshold.
//
// Parameters:
//   - threshold: The new threshold value
func (t *Tier) SetThreshold(threshold float64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.threshold = threshold
}

// GetMetrics returns metrics about semantic tier usage.
//
// Returns:
//   - map[string]interface{}: Metrics including match count, hit rate, avg latency
func (t *Tier) GetMetrics() map[string]interface{} {
	t.mu.RLock()
	defer t.mu.RUnlock()

	hitRate := float64(0)
	avgLatency := float64(0)

	if t.matchCount > 0 {
		hitRate = float64(t.hitCount) / float64(t.matchCount)
		avgLatency = float64(t.totalLatencyMs) / float64(t.matchCount)
	}

	return map[string]interface{}{
		"match_count":      t.matchCount,
		"hit_count":        t.hitCount,
		"hit_rate":         hitRate,
		"avg_latency_ms":   avgLatency,
		"intent_count":     len(t.intents),
		"threshold":        t.threshold,
		"enabled":          t.enabled,
	}
}

// Shutdown gracefully shuts down the semantic tier.
//
// Returns:
//   - error: Any error encountered during shutdown
func (t *Tier) Shutdown() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.enabled {
		return nil
	}

	t.enabled = false
	t.intents = nil
	log.Info("Semantic tier shut down")

	return nil
}


// MatchIntentInterface matches an intent and returns the result as an interface type.
// This is used by the intelligence service to avoid circular imports.
func (t *Tier) MatchIntentInterface(query string) (interface{}, error) {
	return t.MatchIntent(query)
}
