package learning

import (
	"strings"
	"time"

	"github.com/traylinx/switchAILocal/internal/memory"
)

// performStatisticalAnalysis analyzes history to find patterns.
func (le *LearningEngine) performStatisticalAnalysis(history []*memory.RoutingDecision) *PreferenceModel {
	model := &PreferenceModel{
		LastUpdated:      time.Now(),
		TotalRequests:    len(history),
		ModelPreferences: make(map[string]*ModelPreference),
		ProviderBias:     make(map[string]float64),
		TimePatterns:     make(map[string]*TimePattern),
	}

	if len(history) == 0 {
		return model
	}

	model.UserID = history[0].APIKeyHash

	// 1. Analyze Intent-Model Performance
	intentStats := make(map[string]map[string]*stats) // Intent -> Model -> Stats

	for _, decision := range history {
		if decision.Request.Intent == "" {
			continue
		}

		intent := decision.Request.Intent
		modelName := decision.Routing.SelectedModel

		if intentStats[intent] == nil {
			intentStats[intent] = make(map[string]*stats)
		}
		if intentStats[intent][modelName] == nil {
			intentStats[intent][modelName] = &stats{}
		}

		s := intentStats[intent][modelName]
		s.count++
		if decision.Outcome.Success {
			s.success++
			s.qualitySum += decision.Outcome.QualityScore
		}
	}

	// Determine best model for each intent
	for intent, models := range intentStats {
		var bestModel string
		var bestScore float64
		var bestStats *stats

		for name, s := range models {
			if s.count < 5 { // Minimum samples per model-intent pair
				continue
			}

			avgQuality := 0.0
			if s.success > 0 {
				avgQuality = s.qualitySum / float64(s.success)
			}

			score := CalculatePreferenceConfidence(s.success, s.count, avgQuality)

			if score > bestScore {
				bestScore = score
				bestModel = name
				bestStats = s
			}
		}

		if bestModel != "" {
			model.ModelPreferences[intent] = &ModelPreference{
				Model:       bestModel,
				Confidence:  bestScore, // Use calculated score as base confidence
				UsageCount:  bestStats.count,
				SuccessRate: float64(bestStats.success) / float64(bestStats.count),
			}
		}
	}

	// 2. Analyze Time Patterns
	// Group by hour (0-23)
	hourlyUsage := make(map[int]int)
	hourlyIntents := make(map[int]map[string]int) // Hour -> Intent -> Count

	for _, decision := range history {
		hour := decision.Timestamp.Hour()
		hourlyUsage[hour]++

		if decision.Request.Intent != "" {
			if hourlyIntents[hour] == nil {
				hourlyIntents[hour] = make(map[string]int)
			}
			hourlyIntents[hour][decision.Request.Intent]++
		}
	}

	// Create general time pattern
	tp := &TimePattern{
		HourlyUsage: hourlyUsage,
		PeakIntents: make(map[int]string),
	}

	// Identify peak intents per hour
	for h := 0; h < 24; h++ {
		if intents, ok := hourlyIntents[h]; ok {
			var peakIntent string
			var maxCount int
			var totalHourCount int

			for intent, count := range intents {
				totalHourCount += count
				if count > maxCount {
					maxCount = count
					peakIntent = intent
				}
			}

			// Only mark as peak if it's dominant (> 50% of requests in that hour, with min samples)
			if totalHourCount >= 5 && float64(maxCount)/float64(totalHourCount) > 0.5 {
				tp.PeakIntents[h] = peakIntent
			}
		}
	}

	model.TimePatterns["general"] = tp

	// 3. Analyze Provider Bias
	providerStats := make(map[string]*stats)

	for _, decision := range history {
		provider := extractProvider(decision.Routing.SelectedModel)
		if provider == "" {
			continue
		}

		if providerStats[provider] == nil {
			providerStats[provider] = &stats{}
		}

		s := providerStats[provider]
		s.count++
		if decision.Outcome.Success {
			s.success++
		}
	}

	// Calculate global success rate
	var totalSuccess, totalCount int
	for _, s := range providerStats {
		totalSuccess += s.success
		totalCount += s.count
	}
	globalRate := 0.0
	if totalCount > 0 {
		globalRate = float64(totalSuccess) / float64(totalCount)
	}

	// Calculate bias relative to global average
	for provider, s := range providerStats {
		if s.count < 10 {
			continue
		}

		rate := float64(s.success) / float64(s.count)
		// Bias: difference from global average, clamped
		bias := (rate - globalRate) * 2.0 // Amplify difference

		if bias < -1.0 {
			bias = -1.0
		}
		if bias > 1.0 {
			bias = 1.0
		}

		if bias != 0 {
			model.ProviderBias[provider] = bias
		}
	}

	return model
}

type stats struct {
	count      int
	success    int
	qualitySum float64
}

func extractProvider(model string) string {
	parts := strings.SplitN(model, ":", 2)
	if len(parts) >= 1 {
		return parts[0]
	}
	return ""
}
