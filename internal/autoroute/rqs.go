package autoroute

import (
	"time"
)

// RequestOutcome represents the real-world result of a routing decision.
type RequestOutcome struct {
	Timestamp          time.Time
	Model              string
	Provider           string
	Tier               string
	Latency            time.Duration
	Success            bool
	StatusCode         int
	EstimatedComplexity float64
	TokensUsed         int // If available
}

// RQSWeightConfig defines the importance of different factors in the
// Routing Quality Score (RQS). This is the master "val_bpb" metric.
type RQSWeightConfig struct {
	Success     float64
	Latency     float64
	Efficiency  float64
	Conservation float64
}

// DefaultRQSWeights provides the standard baseline for evaluating routing quality.
var DefaultRQSWeights = RQSWeightConfig{
	Success:      0.40, // Most important: did it work?
	Latency:      0.20, // Next: was it fast?
	Efficiency:   0.20, // Did we use an appropriate model for the task?
	Conservation: 0.20, // Did we save premium tokens when possible?
}

// CalculateRQS computes the Routing Quality Score (0.0 to 1.0) for a single request outcome.
// Higher is always better. This is the single metric the Lab optimizes for.
func CalculateRQS(outcome RequestOutcome, weights RQSWeightConfig) float64 {
	// 1. Success Component
	successScore := 0.0
	if outcome.Success {
		successScore = 1.0
	}

	// 2. Latency Component (0=5000ms+, 1.0=0ms)
	latencyScore := latencyToScore(outcome.Latency)

	// 3. Efficiency Component
	// Did we use a heavy model for a simple task, or vice versa?
	efficiencyScore := calculateEfficiency(outcome.Tier, outcome.EstimatedComplexity)

	// 4. Conservation Component
	// Did we successfully route a simple task to a free/local provider?
	conservationScore := calculateConservation(outcome.Tier, outcome.EstimatedComplexity)

	// Composite Score
	score := (successScore * weights.Success) +
		(latencyScore * weights.Latency) +
		(efficiencyScore * weights.Efficiency) +
		(conservationScore * weights.Conservation)

	// Clamp to 0.0 - 1.0
	if score < 0 {
		return 0.0
	}
	if score > 1 {
		return 1.0
	}

	return score
}

// calculateEfficiency measures if the chosen tier matches the task complexity.
// 1.0 = perfect match, decreasing as mismatch grows.
func calculateEfficiency(tier string, complexity float64) float64 {
	// Expected complexity bands per tier
	var expectedMin, expectedMax float64
	switch tier {
	case TierPremium:
		expectedMin, expectedMax = 0.6, 1.0
	case TierStandard:
		expectedMin, expectedMax = 0.3, 0.8
	case TierFree, TierLocal:
		expectedMin, expectedMax = 0.0, 0.5
	default: // Reserve or unknown
		expectedMin, expectedMax = 0.0, 1.0
	}

	if complexity >= expectedMin && complexity <= expectedMax {
		return 1.0 // Perfect match
	}

	// Calculate distance from the appropriate band
	var distance float64
	if complexity < expectedMin {
		distance = expectedMin - complexity
	} else {
		distance = complexity - expectedMax
	}

	// Deduct 1.0 per full point of distance (distance is max 1.0)
	score := 1.0 - distance
	if score < 0 {
		return 0.0
	}
	return score
}

// calculateConservation specifically rewards saving premium tokens on simple tasks.
func calculateConservation(tier string, complexity float64) float64 {
	isSimple := complexity < 0.4

	if isSimple {
		if tier == TierFree || tier == TierLocal {
			return 1.0 // Maximum conservation achieved
		}
		if tier == TierPremium {
			return 0.0 // Failed to conserve on simple task
		}
		return 0.5 // Standard tier is neutral
	}

	// For complex tasks, conservation is less relevant, so we don't penalize
	// using premium, and we slightly reward using free (if it succeeded).
	if tier == TierFree || tier == TierLocal {
		return 1.0 // Over-performed: solved complex task with free model
	}
	return 0.8 // Using premium/standard for complex tasks is normal
}
