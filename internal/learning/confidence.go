package learning

import (
	"math"
)

// CalculatePreferenceConfidence computes the confidence score for a learned preference.
// It considers success rate, sample size, and consistency.
func CalculatePreferenceConfidence(successCount, totalCount int, avgQuality float64) float64 {
	if totalCount == 0 {
		return 0.0
	}

	successRate := float64(successCount) / float64(totalCount)

	// 1. Base score from success and quality
	// Weight success rate higher (70%) than quality (30%)?
	// If avgQuality is 0 (not tracked), rely solely on success rate.
	baseScore := successRate
	if avgQuality > 0 {
		baseScore = (successRate * 0.7) + (avgQuality * 0.3)
	}

	// 2. Penalize small sample sizes (Wilson score interval approach simplified or just a penalty)
	// We want high confidence only if we have sufficient samples.
	// Using a logarithmic boost up to a threshold.
	// 5 samples -> 0.5 factor
	// 20 samples -> 0.9 factor
	// 100 samples -> 1.0 factor

	samplePenalty := 1.0
	if totalCount < 100 {
		// Logarithmic curve from 0 to 100
		samplePenalty = math.Log(float64(totalCount)+1) / math.Log(101)
	}

	confidence := baseScore * samplePenalty

	// Clamp
	if confidence < 0.0 {
		return 0.0
	}
	if confidence > 1.0 {
		return 1.0
	}

	return confidence
}

// AdjustRuntimeConfidence adjusts a hypothetical runtime confidence based on learned factors.
// This implements the design doc formula: Base + Pref Boost + Bias + Time
func AdjustRuntimeConfidence(baseConf float64, hasPreference bool, providerBias float64, isTimeMatch bool) float64 {
	score := baseConf

	// Preference Boost
	if hasPreference {
		score += 0.15
	}

	// Provider Bias (range -1.0 to 1.0, scaled to small influence)
	score += (providerBias * 0.1)

	// Time Pattern Boost
	if isTimeMatch {
		score += 0.1
	}

	// Clamp
	if score < 0.0 {
		score = 0.0
	}
	if score > 1.0 {
		score = 1.0
	}

	return score
}
