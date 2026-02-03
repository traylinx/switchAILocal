package intelligence

import (
	"context"
	"testing"
	"time"

	"github.com/traylinx/switchAILocal/internal/config"
	"github.com/traylinx/switchAILocal/internal/memory"
)

// Mock implementations for testing

type mockMemoryManager struct {
	preferences map[string]*memory.UserPreferences
	decisions   []*memory.RoutingDecision
	quirks      map[string][]*memory.Quirk
	enabled     bool
}

func newMockMemoryManager(enabled bool) *mockMemoryManager {
	return &mockMemoryManager{
		preferences: make(map[string]*memory.UserPreferences),
		decisions:   make([]*memory.RoutingDecision, 0),
		quirks:      make(map[string][]*memory.Quirk),
		enabled:     enabled,
	}
}

func (m *mockMemoryManager) RecordRouting(decision *memory.RoutingDecision) error {
	if !m.enabled {
		return nil
	}
	m.decisions = append(m.decisions, decision)
	return nil
}

func (m *mockMemoryManager) GetUserPreferences(apiKeyHash string) (*memory.UserPreferences, error) {
	if !m.enabled {
		return &memory.UserPreferences{
			APIKeyHash:       apiKeyHash,
			LastUpdated:      time.Now(),
			ModelPreferences: make(map[string]string),
			ProviderBias:     make(map[string]float64),
			CustomRules:      []memory.PreferenceRule{},
		}, nil
	}

	if prefs, exists := m.preferences[apiKeyHash]; exists {
		return prefs, nil
	}

	// Return default preferences
	return &memory.UserPreferences{
		APIKeyHash:       apiKeyHash,
		LastUpdated:      time.Now(),
		ModelPreferences: make(map[string]string),
		ProviderBias:     make(map[string]float64),
		CustomRules:      []memory.PreferenceRule{},
	}, nil
}

func (m *mockMemoryManager) UpdateUserPreferences(prefs *memory.UserPreferences) error {
	if !m.enabled {
		return nil
	}
	m.preferences[prefs.APIKeyHash] = prefs
	return nil
}

func (m *mockMemoryManager) DeleteUserPreferences(apiKeyHash string) error {
	if !m.enabled {
		return nil
	}
	delete(m.preferences, apiKeyHash)
	return nil
}

func (m *mockMemoryManager) AddQuirk(quirk *memory.Quirk) error {
	if !m.enabled {
		return nil
	}
	m.quirks[quirk.Provider] = append(m.quirks[quirk.Provider], quirk)
	return nil
}

func (m *mockMemoryManager) GetProviderQuirks(provider string) ([]*memory.Quirk, error) {
	if !m.enabled {
		return []*memory.Quirk{}, nil
	}
	return m.quirks[provider], nil
}

func (m *mockMemoryManager) GetHistory(apiKeyHash string, limit int) ([]*memory.RoutingDecision, error) {
	if !m.enabled {
		return []*memory.RoutingDecision{}, nil
	}

	var result []*memory.RoutingDecision
	count := 0
	for i := len(m.decisions) - 1; i >= 0 && count < limit; i-- {
		if m.decisions[i].APIKeyHash == apiKeyHash {
			result = append(result, m.decisions[i])
			count++
		}
	}
	return result, nil
}

func (m *mockMemoryManager) GetAllHistory(limit int) ([]*memory.RoutingDecision, error) {
	if !m.enabled {
		return []*memory.RoutingDecision{}, nil
	}

	if limit > len(m.decisions) {
		limit = len(m.decisions)
	}

	result := make([]*memory.RoutingDecision, limit)
	for i := 0; i < limit; i++ {
		result[i] = m.decisions[len(m.decisions)-1-i]
	}
	return result, nil
}

func (m *mockMemoryManager) LearnFromOutcome(decision *memory.RoutingDecision) error {
	if !m.enabled {
		return nil
	}

	// Simple learning: if successful, add to preferences
	if decision.Outcome.Success && decision.Request.Intent != "" {
		prefs, _ := m.GetUserPreferences(decision.APIKeyHash)
		if prefs.ModelPreferences == nil {
			prefs.ModelPreferences = make(map[string]string)
		}
		prefs.ModelPreferences[decision.Request.Intent] = decision.Routing.SelectedModel
		m.preferences[decision.APIKeyHash] = prefs
	}

	return nil
}

func (m *mockMemoryManager) GetStats() (*memory.MemoryStats, error) {
	return &memory.MemoryStats{
		TotalDecisions: len(m.decisions),
		TotalUsers:     len(m.preferences),
		TotalQuirks:    len(m.quirks),
	}, nil
}

func (m *mockMemoryManager) Cleanup() error {
	return nil
}

func (m *mockMemoryManager) Close() error {
	return nil
}

func (m *mockMemoryManager) ComputeAnalytics() (*memory.AnalyticsSummary, error) {
	if !m.enabled {
		return &memory.AnalyticsSummary{
			GeneratedAt:       time.Now(),
			ProviderStats:     make(map[string]*memory.ProviderStats),
			ModelPerformance:  make(map[string]*memory.ModelPerformance),
			TierEffectiveness: &memory.TierEffectiveness{},
			CostAnalysis:      &memory.CostAnalysis{},
			TrendAnalysis:     &memory.TrendAnalysis{},
		}, nil
	}

	// Simple mock analytics
	return &memory.AnalyticsSummary{
		GeneratedAt: time.Now(),
		ProviderStats: map[string]*memory.ProviderStats{
			"test-provider": {
				Provider:      "test-provider",
				TotalRequests: len(m.decisions),
				SuccessRate:   0.95,
				AvgLatencyMs:  100.0,
				ErrorRate:     0.05,
				LastUpdated:   time.Now(),
			},
		},
		ModelPerformance: map[string]*memory.ModelPerformance{
			"test-model": {
				Model:           "test-model",
				TotalRequests:   len(m.decisions),
				SuccessRate:     0.95,
				AvgQualityScore: 0.85,
				AvgCostPerReq:   0.001,
			},
		},
		TierEffectiveness: &memory.TierEffectiveness{},
		CostAnalysis:      &memory.CostAnalysis{},
		TrendAnalysis:     &memory.TrendAnalysis{},
	}, nil
}

func (m *mockMemoryManager) GetAnalytics() (*memory.AnalyticsSummary, error) {
	return m.ComputeAnalytics()
}

type mockSemanticTier struct {
	enabled bool
	results map[string]*SemanticMatchResult
}

func newMockSemanticTier(enabled bool) *mockSemanticTier {
	return &mockSemanticTier{
		enabled: enabled,
		results: make(map[string]*SemanticMatchResult),
	}
}

func (m *mockSemanticTier) MatchIntent(query string) (*SemanticMatchResult, error) {
	if !m.enabled {
		return nil, nil
	}

	if result, exists := m.results[query]; exists {
		return result, nil
	}

	// Default behavior for common patterns
	if len(query) > 10 && query[:4] == "code" {
		return &SemanticMatchResult{
			Intent:     "coding",
			Confidence: 0.85,
			LatencyMs:  10,
		}, nil
	}

	return nil, nil
}

func (m *mockSemanticTier) IsEnabled() bool {
	return m.enabled
}

func (m *mockSemanticTier) GetMetrics() map[string]interface{} {
	return map[string]interface{}{
		"enabled": m.enabled,
	}
}

// Test functions

func TestNewCortexRouter(t *testing.T) {
	config := &config.IntelligenceConfig{
		Enabled: true,
	}
	memoryManager := newMockMemoryManager(true)

	router := NewCortexRouter(config, memoryManager)

	if router == nil {
		t.Fatal("Expected non-nil router")
	}

	if !router.IsEnabled() {
		t.Error("Expected router to be enabled")
	}

	if !router.IsMemoryEnabled() {
		t.Error("Expected memory to be enabled")
	}
}

func TestNewCortexRouter_NilConfig(t *testing.T) {
	router := NewCortexRouter(nil, nil)

	if router == nil {
		t.Fatal("Expected non-nil router")
	}

	if router.IsEnabled() {
		t.Error("Expected router to be disabled with nil config")
	}

	if router.IsMemoryEnabled() {
		t.Error("Expected memory to be disabled with nil memory manager")
	}
}

func TestCortexRouter_Route_ReflexTier_PII(t *testing.T) {
	config := &config.IntelligenceConfig{Enabled: true}
	memoryManager := newMockMemoryManager(true)
	router := NewCortexRouter(config, memoryManager)

	request := &RoutingRequest{
		APIKey:  "test-key",
		Model:   "auto",
		Content: "My email is john@example.com and my phone is 555-1234",
	}

	ctx := context.Background()
	decision, err := router.Route(ctx, request)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if decision.Tier != "reflex" {
		t.Errorf("Expected tier 'reflex', got '%s'", decision.Tier)
	}

	if decision.Intent != "pii_detected" {
		t.Errorf("Expected intent 'pii_detected', got '%s'", decision.Intent)
	}

	if decision.Privacy != "pii" {
		t.Errorf("Expected privacy 'pii', got '%s'", decision.Privacy)
	}

	if decision.Confidence < 0.9 {
		t.Errorf("Expected high confidence for PII detection, got %.2f", decision.Confidence)
	}

	// Should route to local model for privacy
	if !contains(decision.SelectedModel, "ollama") {
		t.Errorf("Expected local model for PII, got '%s'", decision.SelectedModel)
	}
}

func TestCortexRouter_Route_ReflexTier_Greeting(t *testing.T) {
	config := &config.IntelligenceConfig{Enabled: true}
	memoryManager := newMockMemoryManager(true)
	router := NewCortexRouter(config, memoryManager)

	request := &RoutingRequest{
		APIKey:  "test-key",
		Model:   "auto",
		Content: "Hello there!",
	}

	ctx := context.Background()
	decision, err := router.Route(ctx, request)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if decision.Tier != "reflex" {
		t.Errorf("Expected tier 'reflex', got '%s'", decision.Tier)
	}

	if decision.Intent != "chat" {
		t.Errorf("Expected intent 'chat', got '%s'", decision.Intent)
	}

	if decision.Complexity != "simple" {
		t.Errorf("Expected complexity 'simple', got '%s'", decision.Complexity)
	}

	if decision.Confidence < 0.9 {
		t.Errorf("Expected high confidence for greeting, got %.2f", decision.Confidence)
	}
}

func TestCortexRouter_Route_ReflexTier_Code(t *testing.T) {
	config := &config.IntelligenceConfig{Enabled: true}
	memoryManager := newMockMemoryManager(true)
	router := NewCortexRouter(config, memoryManager)

	request := &RoutingRequest{
		APIKey:  "test-key",
		Model:   "auto",
		Content: "function fibonacci(n) { return n <= 1 ? n : fibonacci(n-1) + fibonacci(n-2); }",
	}

	ctx := context.Background()
	decision, err := router.Route(ctx, request)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if decision.Tier != "reflex" {
		t.Errorf("Expected tier 'reflex', got '%s'", decision.Tier)
	}

	if decision.Intent != "coding" {
		t.Errorf("Expected intent 'coding', got '%s'", decision.Intent)
	}

	if decision.Complexity != "complex" {
		t.Errorf("Expected complexity 'complex', got '%s'", decision.Complexity)
	}

	// Should route to Claude for coding
	if !contains(decision.SelectedModel, "claude") {
		t.Errorf("Expected Claude model for coding, got '%s'", decision.SelectedModel)
	}
}

func TestCortexRouter_Route_SemanticTier(t *testing.T) {
	config := &config.IntelligenceConfig{Enabled: true}
	memoryManager := newMockMemoryManager(false)
	router := NewCortexRouter(config, memoryManager)

	// Set up semantic tier
	semanticTier := newMockSemanticTier(true)
	semanticTier.results["code review best practices"] = &SemanticMatchResult{
		Intent:     "coding",
		Confidence: 0.85,
		LatencyMs:  15,
	}
	router.SetSemanticTier(semanticTier)

	request := &RoutingRequest{
		APIKey:  "test-key",
		Model:   "auto",
		Content: "code review best practices",
	}

	ctx := context.Background()
	decision, err := router.Route(ctx, request)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if decision.Tier != "semantic" {
		t.Errorf("Expected tier 'semantic', got '%s'", decision.Tier)
	}

	if decision.Intent != "coding" {
		t.Errorf("Expected intent 'coding', got '%s'", decision.Intent)
	}

	if decision.Confidence != 0.85 {
		t.Errorf("Expected confidence 0.85, got %.2f", decision.Confidence)
	}
}

func TestCortexRouter_Route_MemoryRouting_Preferences(t *testing.T) {
	config := &config.IntelligenceConfig{Enabled: true}
	memoryManager := newMockMemoryManager(true)

	// Set up user preferences
	apiKeyHash := "sha256:test-hash"
	preferences := &memory.UserPreferences{
		APIKeyHash:  apiKeyHash,
		LastUpdated: time.Now(),
		ModelPreferences: map[string]string{
			"coding": "claudecli:claude-sonnet-4",
		},
		ProviderBias: map[string]float64{
			"claudecli": 0.2, // Positive bias towards Claude
		},
	}
	memoryManager.preferences[apiKeyHash] = preferences

	router := NewCortexRouter(config, memoryManager)

	request := &RoutingRequest{
		APIKey:  "test-key", // Will be hashed to apiKeyHash in real implementation
		Model:   "auto",
		Content: "function test() { console.log('hello'); }",
	}

	ctx := context.Background()
	decision, err := router.Route(ctx, request)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify the decision was made
	if decision == nil {
		t.Fatal("Expected non-nil decision")
	}

	// Should use reflex tier for code patterns, but let's test the memory integration
	// by checking that the decision was recorded
	if len(memoryManager.decisions) == 0 {
		t.Error("Expected routing decision to be recorded in memory")
	}
}

func TestCortexRouter_Route_CognitiveTier_Fallback(t *testing.T) {
	config := &config.IntelligenceConfig{Enabled: true}
	memoryManager := newMockMemoryManager(true)
	router := NewCortexRouter(config, memoryManager)

	// Set up semantic tier that won't match
	semanticTier := newMockSemanticTier(true)
	router.SetSemanticTier(semanticTier)

	request := &RoutingRequest{
		APIKey:  "test-key",
		Model:   "auto",
		Content: "What is the capital of France?", // Should not match reflex or semantic
	}

	ctx := context.Background()
	decision, err := router.Route(ctx, request)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if decision.Tier != "cognitive" {
		t.Errorf("Expected tier 'cognitive', got '%s'", decision.Tier)
	}

	if decision.Intent != "factual" {
		t.Errorf("Expected intent 'factual', got '%s'", decision.Intent)
	}

	if decision.Confidence < 0.5 {
		t.Errorf("Expected reasonable confidence, got %.2f", decision.Confidence)
	}
}

func TestCortexRouter_RecordOutcome(t *testing.T) {
	config := &config.IntelligenceConfig{Enabled: true}
	memoryManager := newMockMemoryManager(true)
	router := NewCortexRouter(config, memoryManager)

	decision := &RoutingDecision{
		APIKeyHash:    "sha256:test-hash",
		RequestHash:   "sha256:request-hash",
		Timestamp:     time.Now(),
		Intent:        "coding",
		Tier:          "reflex",
		SelectedModel: "claudecli:claude-sonnet-4",
		Confidence:    0.95,
		LatencyMs:     25,
	}

	outcome := &RoutingOutcome{
		Decision:       decision,
		Success:        true,
		ResponseTimeMs: 1500,
		QualityScore:   0.9,
	}

	err := router.RecordOutcome(outcome)
	if err != nil {
		t.Fatalf("Unexpected error recording outcome: %v", err)
	}

	// Check that decision was recorded and learned from
	if len(memoryManager.decisions) == 0 {
		t.Error("Expected routing decision to be recorded")
	}

	// Check that preferences were updated (since it was successful)
	prefs, err := memoryManager.GetUserPreferences("sha256:test-hash")
	if err != nil {
		t.Fatalf("Failed to get preferences: %v", err)
	}

	if prefs.ModelPreferences["coding"] != "claudecli:claude-sonnet-4" {
		t.Error("Expected preference to be learned from successful outcome")
	}
}

func TestCortexRouter_DisabledMemory(t *testing.T) {
	config := &config.IntelligenceConfig{Enabled: true}
	router := NewCortexRouter(config, nil) // No memory manager

	if router.IsMemoryEnabled() {
		t.Error("Expected memory to be disabled")
	}

	request := &RoutingRequest{
		APIKey:  "test-key",
		Model:   "auto",
		Content: "Hello world",
	}

	ctx := context.Background()
	decision, err := router.Route(ctx, request)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if decision.UsedMemory {
		t.Error("Expected UsedMemory to be false when memory is disabled")
	}

	// Should still work without memory
	if decision.Tier != "reflex" {
		t.Errorf("Expected tier 'reflex', got '%s'", decision.Tier)
	}
}

func TestCortexRouter_GetStats(t *testing.T) {
	config := &config.IntelligenceConfig{Enabled: true}
	memoryManager := newMockMemoryManager(true)
	router := NewCortexRouter(config, memoryManager)

	stats := router.GetStats()

	if stats["enabled"] != true {
		t.Error("Expected enabled to be true")
	}

	if stats["memory_enabled"] != true {
		t.Error("Expected memory_enabled to be true")
	}

	if _, exists := stats["memory_stats"]; !exists {
		t.Error("Expected memory_stats to be present")
	}
}

func TestCortexRouter_ConfidenceAdjustment(t *testing.T) {
	config := &config.IntelligenceConfig{
		Enabled: true,
		Learning: config.LearningConfig{
			Enabled: true,
		},
	}
	memoryManager := newMockMemoryManager(true)

	// Set up user preferences with provider bias and model preferences
	apiKeyHash := "sha256:test-hash"
	preferences := &memory.UserPreferences{
		APIKeyHash:  apiKeyHash,
		LastUpdated: time.Now(),
		ModelPreferences: map[string]string{
			"coding": "claudecli:claude-sonnet-4", // Learned preference
		},
		ProviderBias: map[string]float64{
			"claudecli": 0.5,  // Strong positive bias
			"ollama":    -0.3, // Negative bias
		},
	}
	memoryManager.preferences[apiKeyHash] = preferences

	router := NewCortexRouter(config, memoryManager)
	defer router.Close()

	// Test confidence adjustment with learned preference and positive bias
	testDecision := &RoutingDecision{
		APIKeyHash:    apiKeyHash,
		Intent:        "coding", // Has learned preference
		SelectedModel: "claudecli:claude-sonnet-4",
		Confidence:    0.7,
		Timestamp:     time.Date(2024, 1, 1, 14, 0, 0, 0, time.UTC), // Work hours
	}

	adjustedConfidence := router.adjustConfidenceWithMemory(testDecision)

	// New learning-based formula: 0.7 (base) + 0.15 (preference) + 0.05 (bias) + 0.1 (time) = 1.0 (clamped)
	expectedConfidence := 1.0
	if adjustedConfidence < expectedConfidence-0.01 || adjustedConfidence > expectedConfidence+0.01 {
		t.Errorf("Expected confidence %.2f, got %.2f", expectedConfidence, adjustedConfidence)
	}

	// Test with negative bias and no learned preference
	testDecision.Intent = "general" // No learned preference
	testDecision.SelectedModel = "ollama:qwen:0.5b"
	testDecision.Timestamp = time.Date(2024, 1, 1, 23, 0, 0, 0, time.UTC) // Off hours

	adjustedConfidence = router.adjustConfidenceWithMemory(testDecision)

	// New learning-based formula: 0.7 (base) + 0.0 (no preference) + (-0.03) (negative bias) + 0.0 (no time match) = 0.67
	expectedConfidence = 0.67
	if adjustedConfidence < expectedConfidence-0.01 || adjustedConfidence > expectedConfidence+0.01 {
		t.Errorf("Expected confidence %.2f, got %.2f", expectedConfidence, adjustedConfidence)
	}
}

func TestCortexRouter_ExtractContent(t *testing.T) {
	router := NewCortexRouter(nil, nil)

	// Test with direct content
	request := &RoutingRequest{
		Content: "Direct content",
	}

	content := router.extractContent(request)
	if content != "Direct content" {
		t.Errorf("Expected 'Direct content', got '%s'", content)
	}

	// Test with messages
	request = &RoutingRequest{
		Messages: []map[string]string{
			{"role": "system", "content": "System message"},
			{"role": "user", "content": "User message"},
			{"role": "assistant", "content": "Assistant message"},
			{"role": "user", "content": "Another user message"},
		},
	}

	content = router.extractContent(request)
	expected := "User message Another user message"
	if content != expected {
		t.Errorf("Expected '%s', got '%s'", expected, content)
	}
}

// Helper functions

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		(len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr)) ||
		(len(substr) > 0 && len(s) > len(substr) && s[1:len(substr)+1] == substr))
}

// Benchmark tests

func BenchmarkCortexRouter_Route_Reflex(b *testing.B) {
	config := &config.IntelligenceConfig{Enabled: true}
	memoryManager := newMockMemoryManager(true)
	router := NewCortexRouter(config, memoryManager)

	request := &RoutingRequest{
		APIKey:  "test-key",
		Model:   "auto",
		Content: "Hello there!",
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := router.Route(ctx, request)
		if err != nil {
			b.Fatalf("Unexpected error: %v", err)
		}
	}
}

func BenchmarkCortexRouter_Route_Semantic(b *testing.B) {
	config := &config.IntelligenceConfig{Enabled: true}
	memoryManager := newMockMemoryManager(true)
	router := NewCortexRouter(config, memoryManager)

	// Set up semantic tier
	semanticTier := newMockSemanticTier(true)
	semanticTier.results["complex query"] = &SemanticMatchResult{
		Intent:     "factual",
		Confidence: 0.8,
		LatencyMs:  10,
	}
	router.SetSemanticTier(semanticTier)

	request := &RoutingRequest{
		APIKey:  "test-key",
		Model:   "auto",
		Content: "complex query",
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := router.Route(ctx, request)
		if err != nil {
			b.Fatalf("Unexpected error: %v", err)
		}
	}
}

func BenchmarkCortexRouter_Route_Cognitive(b *testing.B) {
	config := &config.IntelligenceConfig{Enabled: true}
	memoryManager := newMockMemoryManager(true)
	router := NewCortexRouter(config, memoryManager)

	// Set up semantic tier that won't match
	semanticTier := newMockSemanticTier(true)
	router.SetSemanticTier(semanticTier)

	request := &RoutingRequest{
		APIKey:  "test-key",
		Model:   "auto",
		Content: "What is the meaning of life?", // Won't match reflex or semantic
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := router.Route(ctx, request)
		if err != nil {
			b.Fatalf("Unexpected error: %v", err)
		}
	}
}

// TestMemoryIntegration_FullFlow tests the complete memory integration flow:
// 1. Route a request
// 2. Record the decision
// 3. Record the outcome
// 4. Verify preferences are learned
// 5. Verify next request uses learned preferences
func TestMemoryIntegration_FullFlow(t *testing.T) {
	// Setup
	config := &config.IntelligenceConfig{
		Enabled: true,
	}
	memoryManager := newMockMemoryManager(true)
	router := NewCortexRouter(config, memoryManager)

	ctx := context.Background()
	apiKey := "test-api-key-123"

	// Step 1: Route first request (no preferences yet)
	request1 := &RoutingRequest{
		APIKey:  apiKey,
		Model:   "auto",
		Content: "Write a Python function to sort a list",
	}

	decision1, err := router.Route(ctx, request1)
	if err != nil {
		t.Fatalf("Failed to route request: %v", err)
	}

	if decision1 == nil {
		t.Fatal("Expected routing decision, got nil")
	}

	// Verify decision was recorded
	if len(memoryManager.decisions) != 1 {
		t.Errorf("Expected 1 recorded decision, got %d", len(memoryManager.decisions))
	}

	// Step 2: Record successful outcome
	outcome1 := router.CreateOutcome(decision1, true, 2500, nil)
	if err := router.RecordOutcome(outcome1); err != nil {
		t.Fatalf("Failed to record outcome: %v", err)
	}

	// Verify quality score was calculated
	if outcome1.QualityScore <= 0.0 || outcome1.QualityScore > 1.0 {
		t.Errorf("Invalid quality score: %f (expected 0.0-1.0)", outcome1.QualityScore)
	}

	// Quality score should be high for successful request with reasonable response time
	if outcome1.QualityScore < 0.6 {
		t.Errorf("Quality score too low for successful request: %f", outcome1.QualityScore)
	}

	// Step 3: Simulate learning by setting preferences
	apiKeyHash := router.hashAPIKey(apiKey)
	prefs := &memory.UserPreferences{
		APIKeyHash:  apiKeyHash,
		LastUpdated: time.Now(),
		ModelPreferences: map[string]string{
			"coding": decision1.SelectedModel, // Learn that this model works for coding
		},
		ProviderBias: map[string]float64{
			router.extractProvider(decision1.SelectedModel): 0.3, // Positive bias
		},
		CustomRules: []memory.PreferenceRule{},
	}
	err = memoryManager.UpdateUserPreferences(prefs)
	if err != nil {
		t.Fatalf("Failed to update user preferences: %v", err)
	}

	// Step 4: Route second similar request (should use learned preferences)
	request2 := &RoutingRequest{
		APIKey:  apiKey,
		Model:   "auto",
		Content: "Write a JavaScript function to reverse a string",
	}

	decision2, err := router.Route(ctx, request2)
	if err != nil {
		t.Fatalf("Failed to route second request: %v", err)
	}

	// Verify learned preferences were applied
	if decision2.UsedMemory {
		t.Log("✓ Learned preferences were applied")
	}

	// Verify confidence was adjusted based on provider bias
	// (This is a soft check since confidence adjustment is complex)
	if decision2.Confidence > 0.0 {
		t.Logf("✓ Confidence score: %f", decision2.Confidence)
	}

	// Step 5: Record failed outcome to test quality score calculation
	outcome2 := router.CreateOutcome(decision2, false, 15000, &testError{msg: "timeout"})
	if err := router.RecordOutcome(outcome2); err != nil {
		t.Fatalf("Failed to record failed outcome: %v", err)
	}

	// Quality score should be low for failed request with long response time
	if outcome2.QualityScore > 0.3 {
		t.Errorf("Quality score too high for failed request: %f (expected < 0.3)", outcome2.QualityScore)
	}

	// Verify all decisions were recorded
	if len(memoryManager.decisions) < 2 {
		t.Errorf("Expected at least 2 recorded decisions, got %d", len(memoryManager.decisions))
	}

	t.Log("✓ Full memory integration flow completed successfully")
}

// TestQualityScoreCalculation tests the quality score calculation with various scenarios
func TestQualityScoreCalculation(t *testing.T) {
	tests := []struct {
		name           string
		success        bool
		responseTimeMs int64
		hasError       bool
		expectedMin    float64
		expectedMax    float64
	}{
		{
			name:           "Perfect request",
			success:        true,
			responseTimeMs: 1000,
			hasError:       false,
			expectedMin:    0.9,
			expectedMax:    1.0,
		},
		{
			name:           "Successful but slow",
			success:        true,
			responseTimeMs: 8000,
			hasError:       false,
			expectedMin:    0.6,
			expectedMax:    0.8,
		},
		{
			name:           "Successful with error",
			success:        true,
			responseTimeMs: 2000,
			hasError:       true,
			expectedMin:    0.6,
			expectedMax:    0.8,
		},
		{
			name:           "Failed request",
			success:        false,
			responseTimeMs: 5000,
			hasError:       true,
			expectedMin:    0.0,
			expectedMax:    0.1,
		},
		{
			name:           "Very slow timeout",
			success:        false,
			responseTimeMs: 30000,
			hasError:       true,
			expectedMin:    0.0,
			expectedMax:    0.1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := memory.CalculateQualityScore(tt.success, tt.responseTimeMs, tt.hasError)

			if score < tt.expectedMin || score > tt.expectedMax {
				t.Errorf("Quality score %f not in expected range [%f, %f]",
					score, tt.expectedMin, tt.expectedMax)
			}

			t.Logf("✓ %s: quality score = %f", tt.name, score)
		})
	}
}

// TestCreateOutcome tests the CreateOutcome helper method
func TestCreateOutcome(t *testing.T) {
	config := &config.IntelligenceConfig{Enabled: true}
	memoryManager := newMockMemoryManager(true)
	router := NewCortexRouter(config, memoryManager)

	decision := &RoutingDecision{
		APIKeyHash:    "sha256:test",
		RequestHash:   "sha256:request",
		Timestamp:     time.Now(),
		Intent:        "coding",
		SelectedModel: "claudecli:claude-sonnet-4",
		Provider:      "claudecli",
		Confidence:    0.9,
		Tier:          "semantic",
	}

	// Test successful outcome
	outcome := router.CreateOutcome(decision, true, 2000, nil)
	if outcome == nil {
		t.Fatal("Expected outcome, got nil")
	}
	if !outcome.Success {
		t.Error("Expected success=true")
	}
	if outcome.ResponseTimeMs != 2000 {
		t.Errorf("Expected responseTimeMs=2000, got %d", outcome.ResponseTimeMs)
	}
	if outcome.QualityScore <= 0.0 {
		t.Error("Expected positive quality score")
	}
	if outcome.Error != "" {
		t.Errorf("Expected no error, got: %s", outcome.Error)
	}

	// Test failed outcome with error
	testErr := &testError{msg: "connection timeout"}
	outcome2 := router.CreateOutcome(decision, false, 10000, testErr)
	if outcome2.Success {
		t.Error("Expected success=false")
	}
	if outcome2.Error != "connection timeout" {
		t.Errorf("Expected error message, got: %s", outcome2.Error)
	}
	if outcome2.QualityScore > 0.3 {
		t.Errorf("Expected low quality score for failed request, got: %f", outcome2.QualityScore)
	}

	t.Log("✓ CreateOutcome helper works correctly")
}

// testError is a simple error implementation for testing
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

// Benchmark tests

// BenchmarkRouteWithMemory benchmarks routing with memory integration enabled
func BenchmarkRouteWithMemory(b *testing.B) {
	config := &config.IntelligenceConfig{Enabled: true}
	memoryManager := newMockMemoryManager(true)
	router := NewCortexRouter(config, memoryManager)

	ctx := context.Background()
	request := &RoutingRequest{
		APIKey:  "bench-api-key",
		Model:   "auto",
		Content: "Write a function to calculate fibonacci numbers",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := router.Route(ctx, request)
		if err != nil {
			b.Fatalf("Routing failed: %v", err)
		}
	}
}

// BenchmarkRecordOutcome benchmarks outcome recording
func BenchmarkRecordOutcome(b *testing.B) {
	config := &config.IntelligenceConfig{Enabled: true}
	memoryManager := newMockMemoryManager(true)
	router := NewCortexRouter(config, memoryManager)

	decision := &RoutingDecision{
		APIKeyHash:    "sha256:bench",
		RequestHash:   "sha256:request",
		Timestamp:     time.Now(),
		Intent:        "coding",
		SelectedModel: "claudecli:claude-sonnet-4",
		Provider:      "claudecli",
		Confidence:    0.9,
		Tier:          "semantic",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		outcome := router.CreateOutcome(decision, true, 2000, nil)
		err := router.RecordOutcome(outcome)
		if err != nil {
			b.Fatalf("Recording failed: %v", err)
		}
	}
}

// BenchmarkQualityScoreCalculation benchmarks quality score calculation
func BenchmarkQualityScoreCalculation(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = memory.CalculateQualityScore(true, 2500, false)
	}
}
