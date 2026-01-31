// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package semantic

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// mockEmbeddingEngine is a mock implementation of EmbeddingEngine for testing.
type mockEmbeddingEngine struct {
	embeddings map[string][]float32
	enabled    bool
}

func newMockEmbeddingEngine() *mockEmbeddingEngine {
	return &mockEmbeddingEngine{
		embeddings: make(map[string][]float32),
		enabled:    true,
	}
}

func (m *mockEmbeddingEngine) Embed(text string) ([]float32, error) {
	// Return pre-computed embedding if available
	if emb, ok := m.embeddings[text]; ok {
		return emb, nil
	}
	// Generate a simple deterministic embedding based on text hash
	embedding := make([]float32, 384)
	for i := 0; i < 384; i++ {
		embedding[i] = float32(len(text)%10) / 10.0
	}
	return embedding, nil
}

func (m *mockEmbeddingEngine) CosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0.0
	}
	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0.0
	}
	return dotProduct / (normA * normB)
}

func (m *mockEmbeddingEngine) IsEnabled() bool {
	return m.enabled
}

// createTestIntentsFile creates a temporary intents.yaml file for testing.
func createTestIntentsFile(t *testing.T) string {
	t.Helper()

	content := `intents:
  - name: coding
    description: "Writing, debugging, or explaining code"
    examples:
      - "Write a Python function"
      - "Debug this code"
  - name: reasoning
    description: "Complex logical analysis or problem solving"
    examples:
      - "Solve this math problem"
      - "Analyze the pros and cons"
  - name: creative
    description: "Creative writing or brainstorming"
    examples:
      - "Write a short story"
      - "Generate creative names"
  - name: fast
    description: "Quick factual questions"
    examples:
      - "What is the capital of France"
      - "How many days in a year"
`

	tmpDir := t.TempDir()
	intentsPath := filepath.Join(tmpDir, "intents.yaml")
	if err := os.WriteFile(intentsPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test intents file: %v", err)
	}
	return intentsPath
}

func TestNewTier(t *testing.T) {
	engine := newMockEmbeddingEngine()

	// Test with default threshold
	tier := NewTier(engine, 0)
	if tier.threshold != 0.85 {
		t.Errorf("Expected default threshold 0.85, got %f", tier.threshold)
	}

	// Test with custom threshold
	tier = NewTier(engine, 0.90)
	if tier.threshold != 0.90 {
		t.Errorf("Expected threshold 0.90, got %f", tier.threshold)
	}

	// Test initial state
	if tier.IsEnabled() {
		t.Error("Expected tier to be disabled before initialization")
	}
}

func TestTierInitialize(t *testing.T) {
	engine := newMockEmbeddingEngine()
	tier := NewTier(engine, 0.85)
	intentsPath := createTestIntentsFile(t)

	// Test successful initialization
	err := tier.Initialize(intentsPath)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if !tier.IsEnabled() {
		t.Error("Expected tier to be enabled after initialization")
	}

	if tier.GetIntentCount() != 4 {
		t.Errorf("Expected 4 intents, got %d", tier.GetIntentCount())
	}
}

func TestTierInitializeWithDisabledEngine(t *testing.T) {
	engine := newMockEmbeddingEngine()
	engine.enabled = false
	tier := NewTier(engine, 0.85)
	intentsPath := createTestIntentsFile(t)

	err := tier.Initialize(intentsPath)
	if err == nil {
		t.Error("Expected error when engine is disabled")
	}
}

func TestTierInitializeWithMissingFile(t *testing.T) {
	engine := newMockEmbeddingEngine()
	tier := NewTier(engine, 0.85)

	err := tier.Initialize("/nonexistent/path/intents.yaml")
	if err == nil {
		t.Error("Expected error for missing file")
	}
}

func TestMatchIntent(t *testing.T) {
	engine := newMockEmbeddingEngine()
	tier := NewTier(engine, 0.50) // Lower threshold for testing
	intentsPath := createTestIntentsFile(t)

	if err := tier.Initialize(intentsPath); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Test matching
	result, err := tier.MatchIntent("Write a Python function to sort a list")
	if err != nil {
		t.Fatalf("MatchIntent failed: %v", err)
	}

	// With mock engine, we should get a result (confidence depends on mock implementation)
	if result != nil {
		if result.Intent == "" {
			t.Error("Expected non-empty intent")
		}
		if result.Confidence < 0 || result.Confidence > 1 {
			t.Errorf("Confidence out of range: %f", result.Confidence)
		}
		if result.LatencyMs < 0 {
			t.Errorf("Latency should be non-negative: %d", result.LatencyMs)
		}
	}
}

func TestMatchIntentNotInitialized(t *testing.T) {
	engine := newMockEmbeddingEngine()
	tier := NewTier(engine, 0.85)

	// Try to match without initialization
	_, err := tier.MatchIntent("test query")
	if err == nil {
		t.Error("Expected error when tier not initialized")
	}
}

func TestMatchIntentLatency(t *testing.T) {
	engine := newMockEmbeddingEngine()
	tier := NewTier(engine, 0.50)
	intentsPath := createTestIntentsFile(t)

	if err := tier.Initialize(intentsPath); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Test that latency is reasonable (<20ms for mock engine)
	start := time.Now()
	result, err := tier.MatchIntent("What is the capital of France")
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("MatchIntent failed: %v", err)
	}

	// With mock engine, should be very fast
	if elapsed > 20*time.Millisecond {
		t.Errorf("Latency too high: %v (expected <20ms)", elapsed)
	}

	if result != nil && result.LatencyMs > 20 {
		t.Errorf("Reported latency too high: %dms", result.LatencyMs)
	}
}

func TestGetIntents(t *testing.T) {
	engine := newMockEmbeddingEngine()
	tier := NewTier(engine, 0.85)
	intentsPath := createTestIntentsFile(t)

	if err := tier.Initialize(intentsPath); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	intents := tier.GetIntents()
	if len(intents) != 4 {
		t.Errorf("Expected 4 intents, got %d", len(intents))
	}

	// Verify intent names
	intentNames := make(map[string]bool)
	for _, intent := range intents {
		intentNames[intent.Name] = true
	}

	expectedNames := []string{"coding", "reasoning", "creative", "fast"}
	for _, name := range expectedNames {
		if !intentNames[name] {
			t.Errorf("Missing expected intent: %s", name)
		}
	}
}

func TestSetThreshold(t *testing.T) {
	engine := newMockEmbeddingEngine()
	tier := NewTier(engine, 0.85)

	if tier.GetThreshold() != 0.85 {
		t.Errorf("Expected threshold 0.85, got %f", tier.GetThreshold())
	}

	tier.SetThreshold(0.90)
	if tier.GetThreshold() != 0.90 {
		t.Errorf("Expected threshold 0.90, got %f", tier.GetThreshold())
	}
}

func TestGetMetrics(t *testing.T) {
	engine := newMockEmbeddingEngine()
	tier := NewTier(engine, 0.50)
	intentsPath := createTestIntentsFile(t)

	if err := tier.Initialize(intentsPath); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Make some matches
	_, _ = tier.MatchIntent("Write code")
	_, _ = tier.MatchIntent("Solve math")

	metrics := tier.GetMetrics()

	if metrics["match_count"].(int64) != 2 {
		t.Errorf("Expected match_count 2, got %v", metrics["match_count"])
	}

	if metrics["intent_count"].(int) != 4 {
		t.Errorf("Expected intent_count 4, got %v", metrics["intent_count"])
	}

	if metrics["enabled"].(bool) != true {
		t.Error("Expected enabled to be true")
	}
}

func TestShutdown(t *testing.T) {
	engine := newMockEmbeddingEngine()
	tier := NewTier(engine, 0.85)
	intentsPath := createTestIntentsFile(t)

	if err := tier.Initialize(intentsPath); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if !tier.IsEnabled() {
		t.Error("Expected tier to be enabled")
	}

	if err := tier.Shutdown(); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}

	if tier.IsEnabled() {
		t.Error("Expected tier to be disabled after shutdown")
	}

	// Shutdown again should be no-op
	if err := tier.Shutdown(); err != nil {
		t.Fatalf("Second shutdown failed: %v", err)
	}
}
