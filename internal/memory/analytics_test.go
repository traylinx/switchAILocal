package memory

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAnalyticsEngine_ComputeAnalytics_Empty(t *testing.T) {
	tempDir := t.TempDir()
	
	engine := NewAnalyticsEngine(tempDir)
	
	// Compute analytics with no decisions
	summary, err := engine.ComputeAnalytics([]*RoutingDecision{})
	if err != nil {
		t.Fatalf("Failed to compute analytics: %v", err)
	}
	
	// Verify empty summary
	if summary.GeneratedAt.IsZero() {
		t.Error("Expected non-zero generated timestamp")
	}
	
	if summary.TimeRange.Days != 0 {
		t.Errorf("Expected 0 days, got %d", summary.TimeRange.Days)
	}
	
	if len(summary.ProviderStats) != 0 {
		t.Errorf("Expected 0 provider stats, got %d", len(summary.ProviderStats))
	}
	
	if len(summary.ModelPerformance) != 0 {
		t.Errorf("Expected 0 model performance entries, got %d", len(summary.ModelPerformance))
	}
}

func TestAnalyticsEngine_ComputeAnalytics_WithData(t *testing.T) {
	tempDir := t.TempDir()
	
	engine := NewAnalyticsEngine(tempDir)
	
	// Create test routing decisions
	baseTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
	decisions := []*RoutingDecision{
		{
			Timestamp:  baseTime,
			APIKeyHash: "user1",
			Request: RequestInfo{
				Model:         "auto",
				Intent:        "coding",
				ContentLength: 100,
			},
			Routing: RoutingInfo{
				Tier:          "semantic",
				SelectedModel: "claudecli:claude-sonnet-4",
				Confidence:    0.9,
				LatencyMs:     15,
			},
			Outcome: OutcomeInfo{
				Success:        true,
				ResponseTimeMs: 2000,
				QualityScore:   0.85,
			},
		},
		{
			Timestamp:  baseTime.Add(time.Hour),
			APIKeyHash: "user1",
			Request: RequestInfo{
				Model:         "auto",
				Intent:        "reasoning",
				ContentLength: 200,
			},
			Routing: RoutingInfo{
				Tier:          "cognitive",
				SelectedModel: "geminicli:gemini-2.5-pro",
				Confidence:    0.8,
				LatencyMs:     25,
			},
			Outcome: OutcomeInfo{
				Success:        false,
				ResponseTimeMs: 5000,
				Error:          "timeout",
				QualityScore:   0.0,
			},
		},
		{
			Timestamp:  baseTime.Add(2 * time.Hour),
			APIKeyHash: "user2",
			Request: RequestInfo{
				Model:         "auto",
				Intent:        "coding",
				ContentLength: 150,
			},
			Routing: RoutingInfo{
				Tier:          "semantic",
				SelectedModel: "ollama:codellama",
				Confidence:    0.7,
				LatencyMs:     10,
			},
			Outcome: OutcomeInfo{
				Success:        true,
				ResponseTimeMs: 1500,
				QualityScore:   0.9,
			},
		},
	}
	
	// Compute analytics
	summary, err := engine.ComputeAnalytics(decisions)
	if err != nil {
		t.Fatalf("Failed to compute analytics: %v", err)
	}
	
	// Verify time range
	if summary.TimeRange.Days != 1 {
		t.Errorf("Expected 1 day, got %d", summary.TimeRange.Days)
	}
	
	// Verify provider stats
	if len(summary.ProviderStats) != 3 {
		t.Errorf("Expected 3 providers, got %d", len(summary.ProviderStats))
	}
	
	// Check Claude stats
	claudeStats, exists := summary.ProviderStats["claudecli"]
	if !exists {
		t.Fatal("Expected claudecli provider stats")
	}
	if claudeStats.TotalRequests != 1 {
		t.Errorf("Expected 1 Claude request, got %d", claudeStats.TotalRequests)
	}
	if claudeStats.SuccessRate != 1.0 {
		t.Errorf("Expected 100%% Claude success rate, got %.2f", claudeStats.SuccessRate)
	}
	
	// Check Gemini stats
	geminiStats, exists := summary.ProviderStats["geminicli"]
	if !exists {
		t.Fatal("Expected geminicli provider stats")
	}
	if geminiStats.TotalRequests != 1 {
		t.Errorf("Expected 1 Gemini request, got %d", geminiStats.TotalRequests)
	}
	if geminiStats.SuccessRate != 0.0 {
		t.Errorf("Expected 0%% Gemini success rate, got %.2f", geminiStats.SuccessRate)
	}
	
	// Check Ollama stats
	ollamaStats, exists := summary.ProviderStats["ollama"]
	if !exists {
		t.Fatal("Expected ollama provider stats")
	}
	if ollamaStats.TotalRequests != 1 {
		t.Errorf("Expected 1 Ollama request, got %d", ollamaStats.TotalRequests)
	}
	if ollamaStats.SuccessRate != 1.0 {
		t.Errorf("Expected 100%% Ollama success rate, got %.2f", ollamaStats.SuccessRate)
	}
	
	// Verify model performance
	if len(summary.ModelPerformance) != 3 {
		t.Errorf("Expected 3 models, got %d", len(summary.ModelPerformance))
	}
	
	claudeModel, exists := summary.ModelPerformance["claudecli:claude-sonnet-4"]
	if !exists {
		t.Fatal("Expected Claude model performance")
	}
	if claudeModel.TotalRequests != 1 {
		t.Errorf("Expected 1 Claude model request, got %d", claudeModel.TotalRequests)
	}
	if claudeModel.AvgQualityScore != 0.85 {
		t.Errorf("Expected 0.85 Claude quality score, got %.2f", claudeModel.AvgQualityScore)
	}
	
	// Verify tier effectiveness
	if summary.TierEffectiveness == nil {
		t.Fatal("Expected tier effectiveness data")
	}
	
	if summary.TierEffectiveness.SemanticTier.TotalRequests != 2 {
		t.Errorf("Expected 2 semantic tier requests, got %d", summary.TierEffectiveness.SemanticTier.TotalRequests)
	}
	
	if summary.TierEffectiveness.CognitiveTier.TotalRequests != 1 {
		t.Errorf("Expected 1 cognitive tier request, got %d", summary.TierEffectiveness.CognitiveTier.TotalRequests)
	}
	
	// Verify cost analysis
	if summary.CostAnalysis == nil {
		t.Fatal("Expected cost analysis data")
	}
	
	if summary.CostAnalysis.TotalCost <= 0 {
		t.Error("Expected positive total cost")
	}
	
	if len(summary.CostAnalysis.CostByProvider) != 3 {
		t.Errorf("Expected 3 providers in cost analysis, got %d", len(summary.CostAnalysis.CostByProvider))
	}
	
	// Ollama should have 0 cost (local provider)
	if summary.CostAnalysis.CostByProvider["ollama"] != 0.0 {
		t.Errorf("Expected 0 cost for Ollama, got %.4f", summary.CostAnalysis.CostByProvider["ollama"])
	}
	
	// Verify trend analysis
	if summary.TrendAnalysis == nil {
		t.Fatal("Expected trend analysis data")
	}
	
	if len(summary.TrendAnalysis.RequestVolumeTrend) != 1 {
		t.Errorf("Expected 1 day in volume trend, got %d", len(summary.TrendAnalysis.RequestVolumeTrend))
	}
	
	if summary.TrendAnalysis.RequestVolumeTrend[0].Requests != 3 {
		t.Errorf("Expected 3 requests in volume trend, got %d", summary.TrendAnalysis.RequestVolumeTrend[0].Requests)
	}
	
	if len(summary.TrendAnalysis.PopularModels) != 3 {
		t.Errorf("Expected 3 popular models, got %d", len(summary.TrendAnalysis.PopularModels))
	}
}

func TestAnalyticsEngine_ProviderExtraction(t *testing.T) {
	engine := NewAnalyticsEngine("")
	
	testCases := []struct {
		model    string
		expected string
	}{
		{"claudecli:claude-sonnet-4", "claudecli"},
		{"geminicli:gemini-2.5-pro", "geminicli"},
		{"ollama:codellama", "ollama"},
		{"openai:gpt-4", "openai"},
		{"simple-model", "simple-model"},
	}
	
	for _, tc := range testCases {
		result := engine.extractProvider(tc.model)
		if result != tc.expected {
			t.Errorf("extractProvider(%s): expected %s, got %s", tc.model, tc.expected, result)
		}
	}
}

func TestAnalyticsEngine_LocalProviderDetection(t *testing.T) {
	engine := NewAnalyticsEngine("")
	
	testCases := []struct {
		provider string
		expected bool
	}{
		{"ollama", true},
		{"lmstudio", true},
		{"localai", true},
		{"claudecli", false},
		{"geminicli", false},
		{"openai", false},
	}
	
	for _, tc := range testCases {
		result := engine.isLocalProvider(tc.provider)
		if result != tc.expected {
			t.Errorf("isLocalProvider(%s): expected %t, got %t", tc.provider, tc.expected, result)
		}
	}
}

func TestAnalyticsEngine_CostEstimation(t *testing.T) {
	engine := NewAnalyticsEngine("")
	
	decision := &RoutingDecision{
		Request: RequestInfo{
			ContentLength: 100,
		},
	}
	
	testCases := []struct {
		model        string
		expectedCost float64
	}{
		{"ollama:codellama", 0.0},        // Local provider
		{"lmstudio:model", 0.0},          // Local provider
		{"claudecli:claude", 0.002},      // 2x base cost
		{"geminicli:gemini", 0.0015},     // 1.5x base cost
		{"openai:gpt-4", 0.0018},         // 1.8x base cost
		{"unknown:model", 0.001},         // Base cost
	}
	
	for _, tc := range testCases {
		result := engine.estimateRequestCost(tc.model, decision)
		if abs(result - tc.expectedCost) > 0.0001 {
			t.Errorf("estimateRequestCost(%s): expected %.4f, got %.4f", tc.model, tc.expectedCost, result)
		}
	}
}

// abs returns the absolute value of a float64
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func TestAnalyticsEngine_StoreAndLoadAnalytics(t *testing.T) {
	tempDir := t.TempDir()
	
	engine := NewAnalyticsEngine(tempDir)
	
	// Create test data
	decisions := []*RoutingDecision{
		{
			Timestamp:  time.Now(),
			APIKeyHash: "user1",
			Request: RequestInfo{
				Model:  "auto",
				Intent: "coding",
			},
			Routing: RoutingInfo{
				Tier:          "semantic",
				SelectedModel: "claudecli:claude-sonnet-4",
				Confidence:    0.9,
			},
			Outcome: OutcomeInfo{
				Success:      true,
				QualityScore: 0.85,
			},
		},
	}
	
	// Compute and store analytics
	originalSummary, err := engine.ComputeAnalytics(decisions)
	if err != nil {
		t.Fatalf("Failed to compute analytics: %v", err)
	}
	
	// Verify files were created
	expectedFiles := []string{
		"provider-stats.json",
		"model-performance.json",
		"analytics-summary.json",
	}
	
	for _, fileName := range expectedFiles {
		filePath := filepath.Join(tempDir, fileName)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			t.Errorf("Expected file %s was not created", fileName)
		}
	}
	
	// Load analytics back
	loadedSummary, err := engine.LoadAnalytics()
	if err != nil {
		t.Fatalf("Failed to load analytics: %v", err)
	}
	
	// Verify loaded data matches original
	if len(loadedSummary.ProviderStats) != len(originalSummary.ProviderStats) {
		t.Errorf("Provider stats count mismatch: expected %d, got %d",
			len(originalSummary.ProviderStats), len(loadedSummary.ProviderStats))
	}
	
	if len(loadedSummary.ModelPerformance) != len(originalSummary.ModelPerformance) {
		t.Errorf("Model performance count mismatch: expected %d, got %d",
			len(originalSummary.ModelPerformance), len(loadedSummary.ModelPerformance))
	}
	
	// Verify specific provider stats
	originalClaude := originalSummary.ProviderStats["claudecli"]
	loadedClaude := loadedSummary.ProviderStats["claudecli"]
	
	if originalClaude.TotalRequests != loadedClaude.TotalRequests {
		t.Errorf("Claude total requests mismatch: expected %d, got %d",
			originalClaude.TotalRequests, loadedClaude.TotalRequests)
	}
	
	if originalClaude.SuccessRate != loadedClaude.SuccessRate {
		t.Errorf("Claude success rate mismatch: expected %.2f, got %.2f",
			originalClaude.SuccessRate, loadedClaude.SuccessRate)
	}
}

func TestAnalyticsEngine_LoadNonexistentAnalytics(t *testing.T) {
	tempDir := t.TempDir()
	
	engine := NewAnalyticsEngine(tempDir)
	
	// Load analytics when no file exists
	summary, err := engine.LoadAnalytics()
	if err != nil {
		t.Fatalf("Failed to load analytics: %v", err)
	}
	
	// Should return empty analytics
	if len(summary.ProviderStats) != 0 {
		t.Errorf("Expected 0 provider stats, got %d", len(summary.ProviderStats))
	}
	
	if len(summary.ModelPerformance) != 0 {
		t.Errorf("Expected 0 model performance entries, got %d", len(summary.ModelPerformance))
	}
}

func TestAnalyticsEngine_TrendAnalysis(t *testing.T) {
	tempDir := t.TempDir()
	
	engine := NewAnalyticsEngine(tempDir)
	
	// Create decisions across multiple days and hours
	baseTime := time.Date(2023, 1, 1, 9, 0, 0, 0, time.UTC)
	var decisions []*RoutingDecision
	
	// Day 1: 5 requests
	for i := 0; i < 5; i++ {
		decisions = append(decisions, &RoutingDecision{
			Timestamp:  baseTime.Add(time.Duration(i) * time.Hour),
			APIKeyHash: "user1",
			Request: RequestInfo{
				Model:  "auto",
				Intent: "coding",
			},
			Routing: RoutingInfo{
				Tier:          "semantic",
				SelectedModel: "claudecli:claude-sonnet-4",
				Confidence:    0.9,
			},
			Outcome: OutcomeInfo{
				Success:        true,
				ResponseTimeMs: 2000,
				QualityScore:   0.85,
			},
		})
	}
	
	// Day 2: 3 requests
	day2 := baseTime.AddDate(0, 0, 1)
	for i := 0; i < 3; i++ {
		decisions = append(decisions, &RoutingDecision{
			Timestamp:  day2.Add(time.Duration(i) * time.Hour),
			APIKeyHash: "user1",
			Request: RequestInfo{
				Model:  "auto",
				Intent: "reasoning",
			},
			Routing: RoutingInfo{
				Tier:          "cognitive",
				SelectedModel: "geminicli:gemini-2.5-pro",
				Confidence:    0.8,
			},
			Outcome: OutcomeInfo{
				Success:        true,
				ResponseTimeMs: 3000,
				QualityScore:   0.9,
			},
		})
	}
	
	// Compute analytics
	summary, err := engine.ComputeAnalytics(decisions)
	if err != nil {
		t.Fatalf("Failed to compute analytics: %v", err)
	}
	
	// Verify request volume trend
	volumeTrend := summary.TrendAnalysis.RequestVolumeTrend
	if len(volumeTrend) != 2 {
		t.Fatalf("Expected 2 days in volume trend, got %d", len(volumeTrend))
	}
	
	// Should be sorted by date
	if volumeTrend[0].Date != "2023-01-01" {
		t.Errorf("Expected first date to be 2023-01-01, got %s", volumeTrend[0].Date)
	}
	if volumeTrend[0].Requests != 5 {
		t.Errorf("Expected 5 requests on first day, got %d", volumeTrend[0].Requests)
	}
	
	if volumeTrend[1].Date != "2023-01-02" {
		t.Errorf("Expected second date to be 2023-01-02, got %s", volumeTrend[1].Date)
	}
	if volumeTrend[1].Requests != 3 {
		t.Errorf("Expected 3 requests on second day, got %d", volumeTrend[1].Requests)
	}
	
	// Verify success rate trend
	successTrend := summary.TrendAnalysis.SuccessRateTrend
	if len(successTrend) != 2 {
		t.Fatalf("Expected 2 days in success trend, got %d", len(successTrend))
	}
	
	// All requests were successful
	for i, trend := range successTrend {
		if trend.Value != 1.0 {
			t.Errorf("Day %d: expected 100%% success rate, got %.2f", i, trend.Value)
		}
	}
	
	// Verify popular models
	popularModels := summary.TrendAnalysis.PopularModels
	if len(popularModels) != 2 {
		t.Fatalf("Expected 2 popular models, got %d", len(popularModels))
	}
	
	// Should be sorted by request count (descending)
	if popularModels[0].Model != "claudecli:claude-sonnet-4" {
		t.Errorf("Expected most popular model to be Claude, got %s", popularModels[0].Model)
	}
	if popularModels[0].Requests != 5 {
		t.Errorf("Expected Claude to have 5 requests, got %d", popularModels[0].Requests)
	}
	if popularModels[0].Percentage != 62.5 { // 5/8 * 100
		t.Errorf("Expected Claude percentage to be 62.5, got %.1f", popularModels[0].Percentage)
	}
	
	// Verify peak hours
	peakHours := summary.TrendAnalysis.PeakHours
	if len(peakHours) != 24 {
		t.Fatalf("Expected 24 hours in peak hours, got %d", len(peakHours))
	}
	
	// Should be sorted by request count (descending)
	// Hours 9, 10, 11 should have the most requests (2 each)
	maxRequests := peakHours[0].Requests
	if maxRequests != 2 {
		t.Errorf("Expected max requests per hour to be 2, got %d", maxRequests)
	}
}

func TestAnalyticsEngine_CachedData(t *testing.T) {
	tempDir := t.TempDir()
	
	engine := NewAnalyticsEngine(tempDir)
	
	// Create test data
	decisions := []*RoutingDecision{
		{
			Timestamp:  time.Now(),
			APIKeyHash: "user1",
			Request: RequestInfo{
				Model:  "auto",
				Intent: "coding",
			},
			Routing: RoutingInfo{
				Tier:          "semantic",
				SelectedModel: "claudecli:claude-sonnet-4",
				Confidence:    0.9,
			},
			Outcome: OutcomeInfo{
				Success:      true,
				QualityScore: 0.85,
			},
		},
	}
	
	// Compute analytics
	_, err := engine.ComputeAnalytics(decisions)
	if err != nil {
		t.Fatalf("Failed to compute analytics: %v", err)
	}
	
	// Get cached provider stats
	providerStats := engine.GetProviderStats()
	if len(providerStats) != 1 {
		t.Errorf("Expected 1 cached provider stat, got %d", len(providerStats))
	}
	
	claudeStats, exists := providerStats["claudecli"]
	if !exists {
		t.Fatal("Expected cached Claude stats")
	}
	if claudeStats.TotalRequests != 1 {
		t.Errorf("Expected 1 cached request, got %d", claudeStats.TotalRequests)
	}
	
	// Get cached model performance
	modelPerformance := engine.GetModelPerformance()
	if len(modelPerformance) != 1 {
		t.Errorf("Expected 1 cached model performance, got %d", len(modelPerformance))
	}
	
	claudeModel, exists := modelPerformance["claudecli:claude-sonnet-4"]
	if !exists {
		t.Fatal("Expected cached Claude model performance")
	}
	if claudeModel.TotalRequests != 1 {
		t.Errorf("Expected 1 cached model request, got %d", claudeModel.TotalRequests)
	}
	
	// Verify last update time
	lastUpdate := engine.GetLastUpdate()
	if lastUpdate.IsZero() {
		t.Error("Expected non-zero last update time")
	}
}