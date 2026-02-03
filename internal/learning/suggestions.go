package learning

import (
	"fmt"
)

// generateSuggestions creates recommendations based on analysis.
func (le *LearningEngine) generateSuggestions(model *PreferenceModel) []string {
	var suggestions []string

	if model == nil {
		return suggestions
	}

	// 1. Suggest stronger models for intents
	for intent, pref := range model.ModelPreferences {
		if pref.Confidence > 0.9 && pref.UsageCount > 20 {
			// If we have very high confidence, verify against current implicit behavior?
			// Since we don't know the "current" baseline without looking at recent failures of OTHER models,
			// we frame this as a positive reinforcement or change suggestion.

			// Simple suggestion:
			msg := fmt.Sprintf("Strong performance detected for model '%s' on intent '%s' (Confidence: %.2f)",
				pref.Model, intent, pref.Confidence)
			suggestions = append(suggestions, msg)
		}
	}

	// 2. Warn about weak providers
	for provider, bias := range model.ProviderBias {
		if bias < -0.5 {
			msg := fmt.Sprintf("Performance issues detected with provider '%s' (Negative bias: %.2f). Consider checking quotas or network.",
				provider, bias)
			suggestions = append(suggestions, msg)
		}
	}

	// 3. Time-based suggestions
	if generalTime, ok := model.TimePatterns["general"]; ok {
		for hour, intent := range generalTime.PeakIntents {
			msg := fmt.Sprintf("High volume of '%s' intent detected at hour %d:00.", intent, hour)
			suggestions = append(suggestions, msg)
		}
	}

	return suggestions
}
