package learning

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/traylinx/switchAILocal/internal/config"
	"github.com/traylinx/switchAILocal/internal/memory"
)

// MockMemoryManager is a mock implementation of MemoryManager
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

func TestAnalyzeUser_InsufficientData(t *testing.T) {
	mockMem := new(MockMemoryManager)
	cfg := &config.LearningConfig{
		Enabled:       true,
		MinSampleSize: 10,
	}

	engine, err := NewLearningEngine(cfg, mockMem)
	assert.NoError(t, err)

	history := []*memory.RoutingDecision{{Timestamp: time.Now()}} // Only 1 item
	mockMem.On("GetHistory", "user1", 1000).Return(history, nil)

	ctx := context.Background()
	result, err := engine.AnalyzeUser(ctx, "user1")
	assert.NoError(t, err)
	assert.Equal(t, 1, result.RequestsAnalyzed)
	assert.Nil(t, result.NewPreferences) // Should be nil as analysis skipped
	assert.Contains(t, result.Suggestions[0], "Insufficient data")
}

func TestAnalyzeUser_SuccessAnalysis(t *testing.T) {
	mockMem := new(MockMemoryManager)
	cfg := &config.LearningConfig{
		Enabled:       true,
		MinSampleSize: 5,
	}

	engine, err := NewLearningEngine(cfg, mockMem)
	assert.NoError(t, err)

	// Create 10 items, 8 success for "coding" -> "claude"
	history := make([]*memory.RoutingDecision, 0)
	for i := 0; i < 10; i++ {
		outcome := true
		if i >= 8 {
			outcome = false
		} // 80% success

		history = append(history, &memory.RoutingDecision{
			APIKeyHash: "user1",
			Timestamp:  time.Now().Add(time.Duration(i) * time.Hour),
			Request:    memory.RequestInfo{Intent: "coding"},
			Routing:    memory.RoutingInfo{SelectedModel: "claude"},
			Outcome:    memory.OutcomeInfo{Success: outcome, QualityScore: 1.0},
		})
	}

	mockMem.On("GetHistory", "user1", 1000).Return(history, nil)

	ctx := context.Background()
	result, err := engine.AnalyzeUser(ctx, "user1")
	assert.NoError(t, err)
	assert.NotNil(t, result.NewPreferences)

	// Check learned preference
	pref := result.NewPreferences.ModelPreferences["coding"]
	assert.NotNil(t, pref)
	assert.Equal(t, "claude", pref.Model)
	assert.Greater(t, pref.SuccessRate, 0.7)

	// Check confidence
	// 8/10 success, quality 1.0.
	// Base = 0.8*0.7 + 1.0*0.3 = 0.56 + 0.3 = 0.86
	// Penalty for size < 100: log(11)/log(101) approx 2.39/4.61 = 0.51
	// Total approx 0.44
	// Assert it's calculated (>0)
	assert.Greater(t, pref.Confidence, 0.0)
}

func TestCalculatePreferenceConfidence(t *testing.T) {
	// Perfect short run
	c1 := CalculatePreferenceConfidence(10, 10, 1.0)
	assert.Less(t, c1, 1.0) // Penalized for size
	assert.Greater(t, c1, 0.4)

	// Perfect long run
	c2 := CalculatePreferenceConfidence(100, 100, 1.0)
	assert.InDelta(t, 1.0, c2, 0.05) // close to 1.0

	// Poor run
	c3 := CalculatePreferenceConfidence(50, 100, 0.5)
	assert.Less(t, c3, 0.6)
}
