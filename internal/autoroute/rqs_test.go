package autoroute

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCalculateRQS(t *testing.T) {
	// Baseline config for calculation:
	weights := RQSWeightConfig{
		Success:      0.4,
		Latency:      0.3,
		Efficiency:   0.2,
		Conservation: 0.1,
	}

	t.Run("Perfect Request", func(t *testing.T) {
		outcome := RequestOutcome{
			Success:             true,
			Latency:             150 * time.Millisecond,
			EstimatedComplexity: 0.2, // Low complexity
			Tier:                TierStandard,
		}

		score := CalculateRQS(outcome, weights)
		assert.Greater(t, score, 0.8) // Should be very high
	})

	t.Run("Failed Request", func(t *testing.T) {
		outcome := RequestOutcome{
			Success: false,
			Latency: 5 * time.Second,
			Tier:    TierPremium,
		}

		score := CalculateRQS(outcome, weights)
		assert.Less(t, score, 0.4) // Should strictly penalize the 0.4 success weight
	})

	t.Run("Slow Request Penalized", func(t *testing.T) {
		outcomeFast := RequestOutcome{
			Success: true,
			Latency: 200 * time.Millisecond,
			Tier:    TierStandard,
		}
		outcomeSlow := RequestOutcome{
			Success: true,
			Latency: 8 * time.Second,
			Tier:    TierStandard,
		}

		scoreFast := CalculateRQS(outcomeFast, weights)
		scoreSlow := CalculateRQS(outcomeSlow, weights)

		assert.Greater(t, scoreFast, scoreSlow, "Fast request should have higher RQS than slow request")
	})

	t.Run("Conservation Penalty for Premium on Simple Intent", func(t *testing.T) {
		outcomePremium := RequestOutcome{
			Success:             true,
			Latency:             500 * time.Millisecond,
			Tier:                TierPremium,
			EstimatedComplexity: 0.1, // Very simple task
		}
		outcomeLocal := RequestOutcome{
			Success:             true,
			Latency:             500 * time.Millisecond,
			Tier:                TierLocal,
			EstimatedComplexity: 0.1,
		}

		scorePremium := CalculateRQS(outcomePremium, weights)
		scoreLocal := CalculateRQS(outcomeLocal, weights)

		assert.Greater(t, scoreLocal, scorePremium, "Local should win over premium for simple tasks due to conservation")
	})
}
