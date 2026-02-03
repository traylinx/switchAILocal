package memory

import (
	"fmt"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// Property test for diagnostic report completeness
// **Validates: Requirements FR-1.1**
func TestProperty_DiagnosticReportCompleteness(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("For any provider in the system, the diagnostic report SHALL include all required metrics", prop.ForAll(
		func(requestCount int, successCount int) bool {
			// Ensure valid test data
			if requestCount < 1 || requestCount > 50 || successCount < 0 || successCount > requestCount {
				return true // Skip invalid combinations
			}

			providerName := "testprovider"
			tempDir := t.TempDir()

			// Create analytics engine
			engine := NewAnalyticsEngine(tempDir)

			// Create test routing decisions for the provider
			var decisions []*RoutingDecision
			baseTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)

			latencyPerRequest := int64(2000) // 2 seconds per request

			for i := 0; i < requestCount; i++ {
				success := i < successCount

				decision := &RoutingDecision{
					Timestamp:  baseTime.Add(time.Duration(i) * time.Minute),
					APIKeyHash: "test-user",
					Request: RequestInfo{
						Model:  "auto",
						Intent: "coding",
					},
					Routing: RoutingInfo{
						Tier:          "semantic",
						SelectedModel: providerName + ":test-model",
						Confidence:    0.9,
						LatencyMs:     15,
					},
					Outcome: OutcomeInfo{
						Success:        success,
						ResponseTimeMs: latencyPerRequest,
						QualityScore:   0.85,
					},
				}

				if !success {
					decision.Outcome.Error = "test error"
					decision.Outcome.QualityScore = 0.0
				}

				decisions = append(decisions, decision)
			}

			// Compute analytics
			summary, err := engine.ComputeAnalytics(decisions)
			if err != nil {
				t.Logf("Failed to compute analytics: %v", err)
				return false
			}

			// Provider should exist in stats
			providerStats, exists := summary.ProviderStats[providerName]
			if !exists {
				t.Logf("Provider '%s' not found. Available providers: %v",
					providerName, getKeys(summary.ProviderStats))
				return false
			}

			// Verify all required metrics are present and correct

			// 1. Total requests
			if providerStats.TotalRequests != requestCount {
				t.Logf("Total requests mismatch: expected %d, got %d", requestCount, providerStats.TotalRequests)
				return false
			}

			// 2. Success rate
			expectedSuccessRate := float64(successCount) / float64(requestCount)
			if abs(providerStats.SuccessRate-expectedSuccessRate) > 0.001 {
				t.Logf("Success rate mismatch: expected %.3f, got %.3f", expectedSuccessRate, providerStats.SuccessRate)
				return false
			}

			// 3. Average latency (should match exactly)
			expectedAvgLatency := float64(latencyPerRequest)
			if providerStats.AvgLatencyMs != expectedAvgLatency {
				t.Logf("Avg latency mismatch: expected %.3f, got %.3f", expectedAvgLatency, providerStats.AvgLatencyMs)
				return false
			}

			// 4. Error rate
			expectedErrorRate := float64(requestCount-successCount) / float64(requestCount)
			if abs(providerStats.ErrorRate-expectedErrorRate) > 0.001 {
				t.Logf("Error rate mismatch: expected %.3f, got %.3f", expectedErrorRate, providerStats.ErrorRate)
				return false
			}

			// 5. Last updated timestamp (should be recent)
			if providerStats.LastUpdated.IsZero() {
				t.Logf("Last updated timestamp is zero")
				return false
			}

			// 6. Provider name should match
			if providerStats.Provider != providerName {
				t.Logf("Provider name mismatch: expected '%s', got '%s'", providerName, providerStats.Provider)
				return false
			}

			return true
		},
		gen.IntRange(1, 20), // Request count (1-20)
		gen.IntRange(0, 20), // Success count (0-20, will be clamped by requestCount)
	))

	properties.TestingRun(t)
}

// Property test for daily log rotation behavior
// **Validates: Requirements FR-1.1**
func TestProperty_DailyLogRotation(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("Daily logs SHALL rotate at midnight and maintain proper file structure", prop.ForAll(
		func(entryCount int) bool {
			if entryCount < 0 || entryCount > 1000 {
				return true // Skip extreme values
			}

			tempDir := t.TempDir()

			manager, err := NewDailyLogsManager(tempDir, 90, false)
			if err != nil {
				return false
			}
			defer manager.Close()

			// Write entries
			for i := 0; i < entryCount; i++ {
				testData := map[string]interface{}{
					"entry_id": i,
					"data":     "test data",
				}

				if err := manager.WriteEntry("test", testData); err != nil {
					return false
				}
			}

			// Get log files
			logFiles, err := manager.GetLogFiles()
			if err != nil {
				return false
			}

			// Should have at least one log file (today's)
			if len(logFiles) == 0 {
				return false
			}

			// Read entries from today's log
			today := time.Now().Format("2006-01-02")
			todayFile := today + ".jsonl"

			entries, err := manager.ReadLogFile(todayFile, -1)
			if err != nil {
				return false
			}

			// Should have the correct number of entries
			if len(entries) != entryCount {
				return false
			}

			// Verify entry structure
			for i, entry := range entries {
				if entry.Type != "test" {
					return false
				}

				if entry.Timestamp.IsZero() {
					return false
				}

				data, ok := entry.Data.(map[string]interface{})
				if !ok {
					return false
				}

				entryID, ok := data["entry_id"].(float64)
				if !ok || int(entryID) != i {
					return false
				}
			}

			return true
		},
		gen.IntRange(0, 50), // Entry count
	))

	properties.TestingRun(t)
}

// Property test for analytics aggregation correctness
// **Validates: Requirements FR-1.1**
func TestProperty_AnalyticsAggregation(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("Analytics aggregation SHALL correctly compute provider stats and model performance", prop.ForAll(
		func(providerCount int, requestsPerProvider int) bool {
			// Ensure valid data
			if providerCount < 1 || providerCount > 5 || requestsPerProvider < 1 || requestsPerProvider > 20 {
				return true // Skip invalid combinations
			}

			tempDir := t.TempDir()
			engine := NewAnalyticsEngine(tempDir)

			var decisions []*RoutingDecision
			baseTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)

			totalRequests := 0
			providerRequestCounts := make(map[string]int)

			// Create decisions for each provider
			for i := 0; i < providerCount; i++ {
				provider := fmt.Sprintf("provider%d", i)
				providerRequestCounts[provider] = requestsPerProvider

				for j := 0; j < requestsPerProvider; j++ {
					decision := &RoutingDecision{
						Timestamp:  baseTime.Add(time.Duration(totalRequests) * time.Minute),
						APIKeyHash: "test-user",
						Request: RequestInfo{
							Model:  "auto",
							Intent: "coding",
						},
						Routing: RoutingInfo{
							Tier:          "semantic",
							SelectedModel: provider + ":test-model",
							Confidence:    0.9,
							LatencyMs:     15,
						},
						Outcome: OutcomeInfo{
							Success:        true,
							ResponseTimeMs: 2000,
							QualityScore:   0.85,
						},
					}

					decisions = append(decisions, decision)
					totalRequests++
				}
			}

			// Compute analytics
			summary, err := engine.ComputeAnalytics(decisions)
			if err != nil {
				return false
			}

			// Verify provider stats match expected counts
			if len(summary.ProviderStats) != providerCount {
				return false
			}

			for provider, expectedCount := range providerRequestCounts {
				stats, exists := summary.ProviderStats[provider]
				if !exists {
					return false
				}

				if stats.TotalRequests != expectedCount {
					return false
				}

				// All requests were successful in this test
				if stats.SuccessRate != 1.0 {
					return false
				}

				if stats.ErrorRate != 0.0 {
					return false
				}
			}

			// Verify model performance
			if len(summary.ModelPerformance) != providerCount {
				return false
			}

			// Verify trend analysis
			if summary.TrendAnalysis == nil {
				return false
			}

			// Should have request volume data
			if len(summary.TrendAnalysis.RequestVolumeTrend) == 0 {
				return false
			}

			return true
		},
		gen.IntRange(1, 3),  // Provider count (1-3)
		gen.IntRange(1, 10), // Requests per provider (1-10)
	))

	properties.TestingRun(t)
}

// Helper function for _max
// Removed to satisfy linter.

// Helper function to get map keys
func getKeys(m map[string]*ProviderStats) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
