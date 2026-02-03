package learning

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traylinx/switchAILocal/internal/config"
	"github.com/traylinx/switchAILocal/internal/memory"
)

func TestPerformStatisticalAnalysis_EmptyHistory(t *testing.T) {
	mockMem := new(MockMemoryManager)
	cfg := &config.LearningConfig{MinSampleSize: 10}
	engine, err := NewLearningEngine(cfg, mockMem)
	require.NoError(t, err)

	model := engine.performStatisticalAnalysis([]*memory.RoutingDecision{})
	
	assert.NotNil(t, model)
	assert.Equal(t, 0, model.TotalRequests)
	assert.Empty(t, model.ModelPreferences)
	assert.Empty(t, model.ProviderBias)
}

func TestPerformStatisticalAnalysis_ModelPreferences(t *testing.T) {
	mockMem := new(MockMemoryManager)
	cfg := &config.LearningConfig{MinSampleSize: 5}
	engine, err := NewLearningEngine(cfg, mockMem)
	require.NoError(t, err)

	// Create test data: "coding" intent with different models
	history := []*memory.RoutingDecision{
		// Claude: 8/10 success (80%)
		{APIKeyHash: "user1", Timestamp: time.Now(), Request: memory.RequestInfo{Intent: "coding"}, Routing: memory.RoutingInfo{SelectedModel: "claude:3.5"}, Outcome: memory.OutcomeInfo{Success: true, QualityScore: 0.9}},
		{APIKeyHash: "user1", Timestamp: time.Now(), Request: memory.RequestInfo{Intent: "coding"}, Routing: memory.RoutingInfo{SelectedModel: "claude:3.5"}, Outcome: memory.OutcomeInfo{Success: true, QualityScore: 0.8}},
		{APIKeyHash: "user1", Timestamp: time.Now(), Request: memory.RequestInfo{Intent: "coding"}, Routing: memory.RoutingInfo{SelectedModel: "claude:3.5"}, Outcome: memory.OutcomeInfo{Success: true, QualityScore: 0.9}},
		{APIKeyHash: "user1", Timestamp: time.Now(), Request: memory.RequestInfo{Intent: "coding"}, Routing: memory.RoutingInfo{SelectedModel: "claude:3.5"}, Outcome: memory.OutcomeInfo{Success: true, QualityScore: 0.7}},
		{APIKeyHash: "user1", Timestamp: time.Now(), Request: memory.RequestInfo{Intent: "coding"}, Routing: memory.RoutingInfo{SelectedModel: "claude:3.5"}, Outcome: memory.OutcomeInfo{Success: true, QualityScore: 0.8}},
		{APIKeyHash: "user1", Timestamp: time.Now(), Request: memory.RequestInfo{Intent: "coding"}, Routing: memory.RoutingInfo{SelectedModel: "claude:3.5"}, Outcome: memory.OutcomeInfo{Success: true, QualityScore: 0.9}},
		{APIKeyHash: "user1", Timestamp: time.Now(), Request: memory.RequestInfo{Intent: "coding"}, Routing: memory.RoutingInfo{SelectedModel: "claude:3.5"}, Outcome: memory.OutcomeInfo{Success: true, QualityScore: 0.8}},
		{APIKeyHash: "user1", Timestamp: time.Now(), Request: memory.RequestInfo{Intent: "coding"}, Routing: memory.RoutingInfo{SelectedModel: "claude:3.5"}, Outcome: memory.OutcomeInfo{Success: true, QualityScore: 0.9}},
		{APIKeyHash: "user1", Timestamp: time.Now(), Request: memory.RequestInfo{Intent: "coding"}, Routing: memory.RoutingInfo{SelectedModel: "claude:3.5"}, Outcome: memory.OutcomeInfo{Success: false, QualityScore: 0.0}},
		{APIKeyHash: "user1", Timestamp: time.Now(), Request: memory.RequestInfo{Intent: "coding"}, Routing: memory.RoutingInfo{SelectedModel: "claude:3.5"}, Outcome: memory.OutcomeInfo{Success: false, QualityScore: 0.0}},
		
		// GPT: 3/5 success (60%)
		{APIKeyHash: "user1", Timestamp: time.Now(), Request: memory.RequestInfo{Intent: "coding"}, Routing: memory.RoutingInfo{SelectedModel: "gpt:4"}, Outcome: memory.OutcomeInfo{Success: true, QualityScore: 0.7}},
		{APIKeyHash: "user1", Timestamp: time.Now(), Request: memory.RequestInfo{Intent: "coding"}, Routing: memory.RoutingInfo{SelectedModel: "gpt:4"}, Outcome: memory.OutcomeInfo{Success: true, QualityScore: 0.6}},
		{APIKeyHash: "user1", Timestamp: time.Now(), Request: memory.RequestInfo{Intent: "coding"}, Routing: memory.RoutingInfo{SelectedModel: "gpt:4"}, Outcome: memory.OutcomeInfo{Success: true, QualityScore: 0.8}},
		{APIKeyHash: "user1", Timestamp: time.Now(), Request: memory.RequestInfo{Intent: "coding"}, Routing: memory.RoutingInfo{SelectedModel: "gpt:4"}, Outcome: memory.OutcomeInfo{Success: false, QualityScore: 0.0}},
		{APIKeyHash: "user1", Timestamp: time.Now(), Request: memory.RequestInfo{Intent: "coding"}, Routing: memory.RoutingInfo{SelectedModel: "gpt:4"}, Outcome: memory.OutcomeInfo{Success: false, QualityScore: 0.0}},
	}

	model := engine.performStatisticalAnalysis(history)
	
	require.NotNil(t, model)
	assert.Equal(t, "user1", model.UserID)
	assert.Equal(t, 15, model.TotalRequests)
	
	// Should prefer Claude for coding (higher success rate and quality)
	codingPref, exists := model.ModelPreferences["coding"]
	require.True(t, exists)
	assert.Equal(t, "claude:3.5", codingPref.Model)
	assert.Equal(t, 10, codingPref.UsageCount)
	assert.InDelta(t, 0.8, codingPref.SuccessRate, 0.01) // 8/10 = 0.8
	assert.Greater(t, codingPref.Confidence, 0.0)
}

func TestPerformStatisticalAnalysis_ProviderBias(t *testing.T) {
	mockMem := new(MockMemoryManager)
	cfg := &config.LearningConfig{MinSampleSize: 5}
	engine, err := NewLearningEngine(cfg, mockMem)
	require.NoError(t, err)

	// Create test data with different provider performance
	history := []*memory.RoutingDecision{}
	
	// Claude provider: 9/10 success (90%)
	for i := 0; i < 10; i++ {
		success := i < 9
		history = append(history, &memory.RoutingDecision{
			APIKeyHash: "user1",
			Timestamp:  time.Now(),
			Request:    memory.RequestInfo{Intent: "general"},
			Routing:    memory.RoutingInfo{SelectedModel: "claude:3.5"},
			Outcome:    memory.OutcomeInfo{Success: success, QualityScore: 0.8},
		})
	}
	
	// OpenAI provider: 5/10 success (50%)
	for i := 0; i < 10; i++ {
		success := i < 5
		history = append(history, &memory.RoutingDecision{
			APIKeyHash: "user1",
			Timestamp:  time.Now(),
			Request:    memory.RequestInfo{Intent: "general"},
			Routing:    memory.RoutingInfo{SelectedModel: "openai:gpt-4"},
			Outcome:    memory.OutcomeInfo{Success: success, QualityScore: 0.6},
		})
	}

	model := engine.performStatisticalAnalysis(history)
	
	require.NotNil(t, model)
	
	// Global success rate should be 70% (14/20)
	// Claude: 90% vs 70% global = +20% = +0.4 bias (after 2x amplification)
	// OpenAI: 50% vs 70% global = -20% = -0.4 bias (after 2x amplification)
	
	claudeBias, exists := model.ProviderBias["claude"]
	if exists {
		assert.Greater(t, claudeBias, 0.0, "Claude should have positive bias")
	}
	
	openAIBias, exists := model.ProviderBias["openai"]
	if exists {
		assert.Less(t, openAIBias, 0.0, "OpenAI should have negative bias")
	}
}

func TestPerformStatisticalAnalysis_TimePatterns(t *testing.T) {
	mockMem := new(MockMemoryManager)
	cfg := &config.LearningConfig{MinSampleSize: 5}
	engine, err := NewLearningEngine(cfg, mockMem)
	require.NoError(t, err)

	// Create test data with time patterns
	history := []*memory.RoutingDecision{}
	
	// Morning coding (9 AM): 8 requests
	for i := 0; i < 8; i++ {
		timestamp := time.Date(2024, 1, 1, 9, 0, 0, 0, time.UTC)
		history = append(history, &memory.RoutingDecision{
			APIKeyHash: "user1",
			Timestamp:  timestamp,
			Request:    memory.RequestInfo{Intent: "coding"},
			Routing:    memory.RoutingInfo{SelectedModel: "claude:3.5"},
			Outcome:    memory.OutcomeInfo{Success: true, QualityScore: 0.8},
		})
	}
	
	// Afternoon writing (2 PM): 6 requests
	for i := 0; i < 6; i++ {
		timestamp := time.Date(2024, 1, 1, 14, 0, 0, 0, time.UTC)
		history = append(history, &memory.RoutingDecision{
			APIKeyHash: "user1",
			Timestamp:  timestamp,
			Request:    memory.RequestInfo{Intent: "writing"},
			Routing:    memory.RoutingInfo{SelectedModel: "gpt:4"},
			Outcome:    memory.OutcomeInfo{Success: true, QualityScore: 0.7},
		})
	}

	model := engine.performStatisticalAnalysis(history)
	
	require.NotNil(t, model)
	
	timePattern, exists := model.TimePatterns["general"]
	require.True(t, exists)
	
	// Check hourly usage
	assert.Equal(t, 8, timePattern.HourlyUsage[9])  // 9 AM
	assert.Equal(t, 6, timePattern.HourlyUsage[14]) // 2 PM
	
	// Check peak intents (should be dominant with >50% and >=5 samples)
	assert.Equal(t, "coding", timePattern.PeakIntents[9])   // 8/8 = 100% coding at 9 AM
	assert.Equal(t, "writing", timePattern.PeakIntents[14]) // 6/6 = 100% writing at 2 PM
}

func TestPerformStatisticalAnalysis_MinimumSamples(t *testing.T) {
	mockMem := new(MockMemoryManager)
	cfg := &config.LearningConfig{MinSampleSize: 5}
	engine, err := NewLearningEngine(cfg, mockMem)
	require.NoError(t, err)

	// Create test data with insufficient samples per model
	history := []*memory.RoutingDecision{
		// Only 3 samples for claude (below 5 minimum)
		{APIKeyHash: "user1", Request: memory.RequestInfo{Intent: "coding"}, Routing: memory.RoutingInfo{SelectedModel: "claude:3.5"}, Outcome: memory.OutcomeInfo{Success: true}},
		{APIKeyHash: "user1", Request: memory.RequestInfo{Intent: "coding"}, Routing: memory.RoutingInfo{SelectedModel: "claude:3.5"}, Outcome: memory.OutcomeInfo{Success: true}},
		{APIKeyHash: "user1", Request: memory.RequestInfo{Intent: "coding"}, Routing: memory.RoutingInfo{SelectedModel: "claude:3.5"}, Outcome: memory.OutcomeInfo{Success: true}},
		
		// Only 2 samples for gpt (below 5 minimum)
		{APIKeyHash: "user1", Request: memory.RequestInfo{Intent: "coding"}, Routing: memory.RoutingInfo{SelectedModel: "gpt:4"}, Outcome: memory.OutcomeInfo{Success: false}},
		{APIKeyHash: "user1", Request: memory.RequestInfo{Intent: "coding"}, Routing: memory.RoutingInfo{SelectedModel: "gpt:4"}, Outcome: memory.OutcomeInfo{Success: false}},
	}

	model := engine.performStatisticalAnalysis(history)
	
	require.NotNil(t, model)
	
	// Should not create preferences due to insufficient samples
	_, exists := model.ModelPreferences["coding"]
	assert.False(t, exists, "Should not create preference with insufficient samples")
}

func TestExtractProvider(t *testing.T) {
	tests := []struct {
		model    string
		expected string
	}{
		{"claude:3.5", "claude"},
		{"openai:gpt-4", "openai"},
		{"gemini:pro", "gemini"},
		{"local-model", "local-model"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			result := extractProvider(tt.model)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Benchmark tests
func BenchmarkPerformStatisticalAnalysis_SmallDataset(b *testing.B) {
	mockMem := new(MockMemoryManager)
	cfg := &config.LearningConfig{MinSampleSize: 5}
	engine, _ := NewLearningEngine(cfg, mockMem)

	// Create 100 routing decisions
	history := make([]*memory.RoutingDecision, 100)
	for i := 0; i < 100; i++ {
		history[i] = &memory.RoutingDecision{
			APIKeyHash: "user1",
			Timestamp:  time.Now().Add(time.Duration(i) * time.Minute),
			Request:    memory.RequestInfo{Intent: "coding"},
			Routing:    memory.RoutingInfo{SelectedModel: "claude:3.5"},
			Outcome:    memory.OutcomeInfo{Success: i%4 != 0, QualityScore: 0.8}, // 75% success
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = engine.performStatisticalAnalysis(history)
	}
}

func BenchmarkPerformStatisticalAnalysis_LargeDataset(b *testing.B) {
	mockMem := new(MockMemoryManager)
	cfg := &config.LearningConfig{MinSampleSize: 5}
	engine, _ := NewLearningEngine(cfg, mockMem)

	// Create 10,000 routing decisions with variety
	history := make([]*memory.RoutingDecision, 10000)
	intents := []string{"coding", "writing", "analysis", "general"}
	models := []string{"claude:3.5", "gpt:4", "gemini:pro"}
	
	for i := 0; i < 10000; i++ {
		history[i] = &memory.RoutingDecision{
			APIKeyHash: "user1",
			Timestamp:  time.Now().Add(time.Duration(i) * time.Minute),
			Request:    memory.RequestInfo{Intent: intents[i%len(intents)]},
			Routing:    memory.RoutingInfo{SelectedModel: models[i%len(models)]},
			Outcome:    memory.OutcomeInfo{Success: i%3 != 0, QualityScore: 0.7}, // 67% success
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = engine.performStatisticalAnalysis(history)
	}
}

func BenchmarkCalculatePreferenceConfidence(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = CalculatePreferenceConfidence(80, 100, 0.85)
	}
}