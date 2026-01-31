// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package cache

import (
	"testing"
	"time"
)

// mockEmbeddingEngine provides a simple mock for testing
type mockEmbeddingEngine struct {
	enabled bool
}

func (m *mockEmbeddingEngine) Embed(text string) ([]float32, error) {
	if len(text) == 0 {
		return []float32{0, 0, 0}, nil
	}

	// Create a unique embedding by using the first 3 characters or padding
	embedding := make([]float32, 3)
	for i := 0; i < 3; i++ {
		if i < len(text) {
			embedding[i] = float32(text[i])
		} else {
			embedding[i] = 0
		}
	}
	return embedding, nil
}

func (m *mockEmbeddingEngine) CosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0.0
	}

	// Exact match check for testing
	match := true
	for i := range a {
		if a[i] != b[i] {
			match = false
			break
		}
	}
	if match {
		return 1.0
	}

	// For distinct queries in tests, return low similarity
	return 0.1
}

func (m *mockEmbeddingEngine) IsEnabled() bool {
	return m.enabled
}

func TestNewSemanticCache(t *testing.T) {
	engine := &mockEmbeddingEngine{enabled: true}

	tests := []struct {
		name                string
		similarityThreshold float64
		maxSize             int
		wantThreshold       float64
		wantMaxSize         int
	}{
		{
			name:                "default values",
			similarityThreshold: 0,
			maxSize:             0,
			wantThreshold:       0.95,
			wantMaxSize:         10000,
		},
		{
			name:                "custom values",
			similarityThreshold: 0.90,
			maxSize:             5000,
			wantThreshold:       0.90,
			wantMaxSize:         5000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := NewSemanticCache(engine, tt.similarityThreshold, tt.maxSize)

			if cache.similarityThreshold != tt.wantThreshold {
				t.Errorf("similarityThreshold = %v, want %v", cache.similarityThreshold, tt.wantThreshold)
			}
			if cache.maxSize != tt.wantMaxSize {
				t.Errorf("maxSize = %v, want %v", cache.maxSize, tt.wantMaxSize)
			}
			if !cache.IsEnabled() {
				t.Error("cache should be enabled")
			}
		})
	}
}

func TestSemanticCache_StoreAndLookup(t *testing.T) {
	engine := &mockEmbeddingEngine{enabled: true}
	cache := NewSemanticCache(engine, 0.95, 100)

	// Store a decision
	query := "What is the weather today?"
	embedding, _ := engine.Embed(query)
	decision := "openai:gpt-4"
	metadata := map[string]interface{}{
		"intent":     "weather",
		"confidence": 0.95,
	}

	err := cache.Store(query, embedding, decision, metadata)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	// Lookup exact match
	entry, err := cache.Lookup(query)
	if err != nil {
		t.Fatalf("Lookup failed: %v", err)
	}
	if entry == nil {
		t.Fatal("Expected cache hit, got miss")
	}
	ce := entry.(*CacheEntry)
	if ce.Decision != decision {
		t.Errorf("Decision = %v, want %v", ce.Decision, decision)
	}
	if ce.Metadata["intent"] != "weather" {
		t.Errorf("Metadata intent = %v, want weather", ce.Metadata["intent"])
	}

	// Check metrics
	metrics := cache.GetMetrics()
	if metrics.Hits != 1 {
		t.Errorf("Hits = %v, want 1", metrics.Hits)
	}
	if metrics.Size != 1 {
		t.Errorf("Size = %v, want 1", metrics.Size)
	}
}

func TestSemanticCache_Miss(t *testing.T) {
	engine := &mockEmbeddingEngine{enabled: true}
	cache := NewSemanticCache(engine, 0.95, 100)

	// Lookup without storing
	entry, err := cache.Lookup("What is the weather?")
	if err != nil {
		t.Fatalf("Lookup failed: %v", err)
	}
	if entry != nil {
		t.Error("Expected cache miss, got hit")
	}

	// Check metrics
	metrics := cache.GetMetrics()
	if metrics.Misses != 1 {
		t.Errorf("Misses = %v, want 1", metrics.Misses)
	}
}

func TestSemanticCache_SimilarityMatch(t *testing.T) {
	engine := &mockEmbeddingEngine{enabled: true}
	cache := NewSemanticCache(engine, 0.80, 100)

	// Store a decision
	query1 := "What is the weather today?"
	embedding1, _ := engine.Embed(query1)
	decision := "openai:gpt-4"

	err := cache.Store(query1, embedding1, decision, nil)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	// Lookup similar query (same length = high similarity in our mock)
	query2 := "What is the weather today!" // Same length
	entry, err := cache.Lookup(query2)
	if err != nil {
		t.Fatalf("Lookup failed: %v", err)
	}

	// With our mock, same length queries have high similarity
	// So we should get a hit
	if entry == nil {
		t.Error("Expected cache hit for similar query")
	}
}

func TestSemanticCache_Eviction(t *testing.T) {
	engine := &mockEmbeddingEngine{enabled: true}
	cache := NewSemanticCache(engine, 0.95, 3) // Small cache

	// Store 4 entries with different first characters
	queries := []string{"apple", "banana", "cherry", "date"}
	for i, query := range queries {
		embedding, _ := engine.Embed(query)
		decision := "model-" + query

		err := cache.Store(query, embedding, decision, nil)
		if err != nil {
			t.Fatalf("Store %d failed: %v", i, err)
		}
	}

	// Cache should have 3 entries (one evicted)
	if cache.GetSize() != 3 {
		t.Errorf("Size = %v, want 3", cache.GetSize())
	}

	// Check evictions
	metrics := cache.GetMetrics()
	if metrics.Evictions != 1 {
		t.Errorf("Evictions = %v, want 1", metrics.Evictions)
	}
}

func TestSemanticCache_Clear(t *testing.T) {
	engine := &mockEmbeddingEngine{enabled: true}
	cache := NewSemanticCache(engine, 0.95, 100)

	// Store some entries
	for i := 0; i < 5; i++ {
		query := string(rune('a' + i))
		embedding, _ := engine.Embed(query)
		_ = cache.Store(query, embedding, "model", nil)
	}

	if cache.GetSize() != 5 {
		t.Errorf("Size = %v, want 5", cache.GetSize())
	}

	// Clear cache
	cache.Clear()

	if cache.GetSize() != 0 {
		t.Errorf("Size after clear = %v, want 0", cache.GetSize())
	}

	// All entries should be gone
	entry, _ := cache.Lookup("a")
	if entry != nil {
		t.Error("Cache should be empty after clear")
	}
}

func TestSemanticCache_HitRate(t *testing.T) {
	engine := &mockEmbeddingEngine{enabled: true}
	cache := NewSemanticCache(engine, 0.95, 100)

	// Store two distinct entries
	query1 := "test query"
	embedding1, _ := engine.Embed(query1)
	_ = cache.Store(query1, embedding1, "model1", nil)

	query2 := "apple"
	embedding2, _ := engine.Embed(query2)
	_ = cache.Store(query2, embedding2, "model2", nil)

	// 2 hits (exact matches), 1 miss (new query)
	_, _ = cache.Lookup(query1)
	_, _ = cache.Lookup(query2)
	_, _ = cache.Lookup("zebra") // Different from both

	hitRate := cache.GetHitRate()
	expected := 2.0 / 3.0
	if hitRate < expected-0.01 || hitRate > expected+0.01 {
		t.Errorf("HitRate = %v, want ~%v", hitRate, expected)
	}
}

func TestSemanticCache_Latency(t *testing.T) {
	engine := &mockEmbeddingEngine{enabled: true}
	cache := NewSemanticCache(engine, 0.95, 100)

	// Store an entry
	query := "test query"
	embedding, _ := engine.Embed(query)
	_ = cache.Store(query, embedding, "model", nil)

	// Lookup (should be fast)
	start := time.Now()
	entry, err := cache.Lookup(query)
	latency := time.Since(start)

	if err != nil {
		t.Fatalf("Lookup failed: %v", err)
	}
	if entry == nil {
		t.Fatal("Expected cache hit")
	}

	// Cache hit should be < 1ms (requirement from spec)
	if latency > time.Millisecond {
		t.Errorf("Cache hit latency = %v, want < 1ms", latency)
	}
}

func TestSemanticCache_IsEnabled(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
		want    bool
	}{
		{
			name:    "enabled engine",
			enabled: true,
			want:    true,
		},
		{
			name:    "disabled engine",
			enabled: false,
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := &mockEmbeddingEngine{enabled: tt.enabled}
			cache := NewSemanticCache(engine, 0.95, 100)

			if cache.IsEnabled() != tt.want {
				t.Errorf("IsEnabled() = %v, want %v", cache.IsEnabled(), tt.want)
			}
		})
	}
}

func TestSemanticCache_GetMetricsAsMap(t *testing.T) {
	engine := &mockEmbeddingEngine{enabled: true}
	cache := NewSemanticCache(engine, 0.95, 100)

	// Store and lookup to generate metrics
	query := "test"
	embedding, _ := engine.Embed(query)
	_ = cache.Store(query, embedding, "model", nil)
	_, _ = cache.Lookup(query)   // Hit (exact match)
	_, _ = cache.Lookup("zebra") // Miss (different first character)

	metricsMap := cache.GetMetricsAsMap()

	// Check that all expected fields are present
	expectedFields := []string{"hits", "misses", "evictions", "size", "avg_hit_latency", "avg_lookup", "hit_rate"}
	for _, field := range expectedFields {
		if _, ok := metricsMap[field]; !ok {
			t.Errorf("Missing field in metrics map: %s", field)
		}
	}

	// Check values
	if metricsMap["hits"].(int64) != 1 {
		t.Errorf("hits = %v, want 1", metricsMap["hits"])
	}
	if metricsMap["misses"].(int64) != 1 {
		t.Errorf("misses = %v, want 1", metricsMap["misses"])
	}
	if metricsMap["size"].(int) != 1 {
		t.Errorf("size = %v, want 1", metricsMap["size"])
	}
}
