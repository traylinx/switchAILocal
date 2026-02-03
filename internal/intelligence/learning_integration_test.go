package intelligence

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/traylinx/switchAILocal/internal/config"
	"github.com/traylinx/switchAILocal/internal/memory"
)

// MockMemoryManager for testing
type MockMemoryManager struct {
	mock.Mock
}

func (m *MockMemoryManager) RecordRouting(decision *memory.RoutingDecision) error {
	args := m.Called(decision)
	return args.Error(0)
}

func (m *MockMemoryManager) GetUserPreferences(apiKeyHash string) (*memory.UserPreferences, error) {
	args := m.Called(apiKeyHash)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*memory.UserPreferences), args.Error(1)
}

func (m *MockMemoryManager) UpdateUserPreferences(prefs *memory.UserPreferences) error {
	args := m.Called(prefs)
	return args.Error(0)
}

func (m *MockMemoryManager) DeleteUserPreferences(apiKeyHash string) error {
	args := m.Called(apiKeyHash)
	return args.Error(0)
}

func (m *MockMemoryManager) AddQuirk(quirk *memory.Quirk) error {
	args := m.Called(quirk)
	return args.Error(0)
}

func (m *MockMemoryManager) GetProviderQuirks(provider string) ([]*memory.Quirk, error) {
	args := m.Called(provider)
	return args.Get(0).([]*memory.Quirk), args.Error(1)
}

func (m *MockMemoryManager) GetHistory(apiKeyHash string, limit int) ([]*memory.RoutingDecision, error) {
	args := m.Called(apiKeyHash, limit)
	return args.Get(0).([]*memory.RoutingDecision), args.Error(1)
}

func (m *MockMemoryManager) GetAllHistory(limit int) ([]*memory.RoutingDecision, error) {
	args := m.Called(limit)
	return args.Get(0).([]*memory.RoutingDecision), args.Error(1)
}

func (m *MockMemoryManager) LearnFromOutcome(decision *memory.RoutingDecision) error {
	args := m.Called(decision)
	return args.Error(0)
}

func (m *MockMemoryManager) GetStats() (*memory.MemoryStats, error) {
	args := m.Called()
	return args.Get(0).(*memory.MemoryStats), args.Error(1)
}

func (m *MockMemoryManager) GetAnalytics() (*memory.AnalyticsSummary, error) {
	args := m.Called()
	return args.Get(0).(*memory.AnalyticsSummary), args.Error(1)
}

func (m *MockMemoryManager) ComputeAnalytics() (*memory.AnalyticsSummary, error) {
	args := m.Called()
	return args.Get(0).(*memory.AnalyticsSummary), args.Error(1)
}

func (m *MockMemoryManager) Cleanup() error { return nil }
func (m *MockMemoryManager) Close() error   { return nil }

func TestLearningIntegration_ConfidenceAdjustment(t *testing.T) {
	mockMem := new(MockMemoryManager)
	cfg := &config.IntelligenceConfig{
		Enabled: true,
		Learning: config.LearningConfig{
			Enabled: true,
		},
	}

	router := NewCortexRouter(cfg, mockMem)
	defer router.Close()

	// Create test preferences with provider bias and model preferences
	prefs := &memory.UserPreferences{
		APIKeyHash: "test-user",
		ModelPreferences: map[string]string{
			"coding": "claude:3.5",
		},
		ProviderBias: map[string]float64{
			"claude": 0.3,  // Positive bias
			"openai": -0.5, // Negative bias
		},
	}

	mockMem.On("GetUserPreferences", "test-user").Return(prefs, nil)

	// Test 1: Confidence adjustment with learned preference and positive bias
	decision := &RoutingDecision{
		APIKeyHash:    "test-user",
		Intent:        "coding",
		SelectedModel: "claude:3.5",
		Confidence:    0.7,
		Timestamp:     time.Date(2024, 1, 1, 14, 0, 0, 0, time.UTC), // 2 PM - work hours
	}

	adjustedConfidence := router.adjustConfidenceWithMemory(decision)
	
	// Expected: 0.7 (base) + 0.15 (preference) + 0.03 (bias) + 0.1 (time) = 0.98
	assert.Greater(t, adjustedConfidence, 0.9, "Should boost confidence with positive factors")
	assert.LessOrEqual(t, adjustedConfidence, 1.0, "Should clamp to maximum 1.0")

	// Test 2: Confidence adjustment with negative bias
	decision.SelectedModel = "openai:gpt-4"
	decision.Intent = "general" // No learned preference
	decision.Timestamp = time.Date(2024, 1, 1, 23, 0, 0, 0, time.UTC) // 11 PM - off hours

	adjustedConfidence = router.adjustConfidenceWithMemory(decision)
	
	// Expected: 0.7 (base) + 0.0 (no preference) + (-0.05) (negative bias) + 0.0 (no time match) = 0.65
	assert.Less(t, adjustedConfidence, 0.7, "Should reduce confidence with negative bias")
	assert.Greater(t, adjustedConfidence, 0.6, "Should not reduce too much")
}

func TestLearningIntegration_TimePatternMatching(t *testing.T) {
	mockMem := new(MockMemoryManager)
	cfg := &config.IntelligenceConfig{
		Enabled: true,
		Learning: config.LearningConfig{
			Enabled: true,
		},
	}

	router := NewCortexRouter(cfg, mockMem)
	defer router.Close()

	prefs := &memory.UserPreferences{
		APIKeyHash: "test-user",
	}

	mockMem.On("GetUserPreferences", "test-user").Return(prefs, nil)

	testCases := []struct {
		intent   string
		hour     int
		expected bool
	}{
		{"coding", 14, true},   // 2 PM - work hours
		{"coding", 2, false},   // 2 AM - off hours
		{"chat", 20, true},     // 8 PM - evening
		{"chat", 10, false},    // 10 AM - work hours
		{"reasoning", 12, true}, // 12 PM - focused hours
		{"reasoning", 22, false}, // 10 PM - late
		{"unknown", 14, false},  // Unknown intent
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%s_at_%d", tc.intent, tc.hour), func(t *testing.T) {
			decision := &RoutingDecision{
				APIKeyHash: "test-user",
				Intent:     tc.intent,
				Timestamp:  time.Date(2024, 1, 1, tc.hour, 0, 0, 0, time.UTC),
			}

			isMatch := router.isTimePatternMatch(decision, prefs)
			assert.Equal(t, tc.expected, isMatch, 
				"Time pattern match for %s at %d:00 should be %t", tc.intent, tc.hour, tc.expected)
		})
	}
}

func TestLearningIntegration_EndToEndRouting(t *testing.T) {
	mockMem := new(MockMemoryManager)
	cfg := &config.IntelligenceConfig{
		Enabled: true,
		Learning: config.LearningConfig{
			Enabled: true,
		},
	}

	router := NewCortexRouter(cfg, mockMem)
	defer router.Close()

	// Setup preferences with learned model preference
	prefs := &memory.UserPreferences{
		APIKeyHash: "sha256:test-user",
		ModelPreferences: map[string]string{
			"coding": "claude:3.5",
		},
		ProviderBias: map[string]float64{
			"claude": 0.2, // Slight positive bias
		},
	}

	mockMem.On("GetUserPreferences", mock.AnythingOfType("string")).Return(prefs, nil)
	mockMem.On("RecordRouting", mock.AnythingOfType("*memory.RoutingDecision")).Return(nil)

	// Test routing a coding request
	request := &RoutingRequest{
		APIKey:  "test-api-key",
		Content: "def fibonacci(n): return n if n <= 1 else fibonacci(n-1) + fibonacci(n-2)",
	}

	ctx := context.Background()
	decision, err := router.Route(ctx, request)

	require.NoError(t, err)
	require.NotNil(t, decision)

	// Should use learned preference
	assert.Equal(t, "learned", decision.Tier, "Should use learned routing tier")
	assert.Equal(t, "coding", decision.Intent, "Should classify as coding")
	assert.Equal(t, "claude:3.5", decision.SelectedModel, "Should use learned preference")
	assert.True(t, decision.UsedMemory, "Should indicate memory was used")
	assert.Equal(t, "preferences", decision.MemorySource, "Should indicate preferences source")
	
	// Confidence should be adjusted by learning algorithm
	assert.Greater(t, decision.Confidence, 0.85, "Should have high confidence with learning adjustment")
}

func TestLearningIntegration_ReflexTierWithLearning(t *testing.T) {
	mockMem := new(MockMemoryManager)
	cfg := &config.IntelligenceConfig{
		Enabled: true,
		Learning: config.LearningConfig{
			Enabled: true,
		},
	}

	router := NewCortexRouter(cfg, mockMem)
	defer router.Close()

	// Setup preferences with provider bias
	prefs := &memory.UserPreferences{
		APIKeyHash: "sha256:test-user",
		ProviderBias: map[string]float64{
			"claudecli": 0.4, // Strong positive bias for Claude
		},
	}

	mockMem.On("GetUserPreferences", mock.AnythingOfType("string")).Return(prefs, nil)
	mockMem.On("RecordRouting", mock.AnythingOfType("*memory.RoutingDecision")).Return(nil)

	// Test routing a coding request that should hit reflex tier
	request := &RoutingRequest{
		APIKey:  "test-api-key",
		Content: "function calculateSum(a, b) { return a + b; }",
	}

	ctx := context.Background()
	decision, err := router.Route(ctx, request)

	require.NoError(t, err)
	require.NotNil(t, decision)

	// Should use reflex tier but with learning-adjusted confidence
	assert.Equal(t, "reflex", decision.Tier, "Should use reflex routing tier")
	assert.Equal(t, "coding", decision.Intent, "Should classify as coding")
	assert.Equal(t, "claudecli:claude-sonnet-4", decision.SelectedModel, "Should use reflex model choice")
	
	// Confidence should be higher than base due to positive provider bias
	assert.Greater(t, decision.Confidence, 0.90, "Should have boosted confidence due to positive bias")
}

func TestLearningIntegration_OutcomeRecording(t *testing.T) {
	mockMem := new(MockMemoryManager)
	cfg := &config.IntelligenceConfig{
		Enabled: true,
		Learning: config.LearningConfig{
			Enabled: true,
		},
	}

	router := NewCortexRouter(cfg, mockMem)
	defer router.Close()

	// Create a routing decision
	decision := &RoutingDecision{
		APIKeyHash:    "test-user",
		Intent:        "coding",
		SelectedModel: "claude:3.5",
		Confidence:    0.85,
		Timestamp:     time.Now(),
	}

	// Mock the memory manager calls
	mockMem.On("RecordRouting", mock.AnythingOfType("*memory.RoutingDecision")).Return(nil)
	mockMem.On("LearnFromOutcome", mock.AnythingOfType("*memory.RoutingDecision")).Return(nil)

	// Test successful outcome
	outcome := router.CreateOutcome(decision, true, 1500, nil)
	
	assert.NotNil(t, outcome)
	assert.Equal(t, decision, outcome.Decision)
	assert.True(t, outcome.Success)
	assert.Equal(t, int64(1500), outcome.ResponseTimeMs)
	assert.Greater(t, outcome.QualityScore, 0.0, "Should have positive quality score for success")

	// Test recording the outcome
	err := router.RecordOutcome(outcome)
	assert.NoError(t, err)

	// Verify memory manager was called
	mockMem.AssertCalled(t, "RecordRouting", mock.AnythingOfType("*memory.RoutingDecision"))
	mockMem.AssertCalled(t, "LearnFromOutcome", mock.AnythingOfType("*memory.RoutingDecision"))
}

// Benchmark tests
func BenchmarkLearningIntegration_ConfidenceAdjustment(b *testing.B) {
	mockMem := new(MockMemoryManager)
	cfg := &config.IntelligenceConfig{
		Enabled: true,
		Learning: config.LearningConfig{
			Enabled: true,
		},
	}

	router := NewCortexRouter(cfg, mockMem)
	defer router.Close()

	prefs := &memory.UserPreferences{
		APIKeyHash: "test-user",
		ModelPreferences: map[string]string{
			"coding": "claude:3.5",
		},
		ProviderBias: map[string]float64{
			"claude": 0.3,
		},
	}

	mockMem.On("GetUserPreferences", "test-user").Return(prefs, nil)

	decision := &RoutingDecision{
		APIKeyHash:    "test-user",
		Intent:        "coding",
		SelectedModel: "claude:3.5",
		Confidence:    0.7,
		Timestamp:     time.Now(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = router.adjustConfidenceWithMemory(decision)
	}
}

func BenchmarkLearningIntegration_EndToEndRouting(b *testing.B) {
	mockMem := new(MockMemoryManager)
	cfg := &config.IntelligenceConfig{
		Enabled: true,
		Learning: config.LearningConfig{
			Enabled: true,
		},
	}

	router := NewCortexRouter(cfg, mockMem)
	defer router.Close()

	prefs := &memory.UserPreferences{
		APIKeyHash: "sha256:test-user",
		ModelPreferences: map[string]string{
			"coding": "claude:3.5",
		},
	}

	mockMem.On("GetUserPreferences", mock.AnythingOfType("string")).Return(prefs, nil)
	mockMem.On("RecordRouting", mock.AnythingOfType("*memory.RoutingDecision")).Return(nil)

	request := &RoutingRequest{
		APIKey:  "test-api-key",
		Content: "def hello(): print('Hello, World!')",
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = router.Route(ctx, request)
	}
}