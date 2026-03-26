package autoroute

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestOptimizationLab_EvaluateAndAdapt_Promote(t *testing.T) {
	cfg := Config{
		Lab: LabConfig{
			Enabled:            true,
			AdaptationInterval: 1 * time.Second,
			MaxWeightDrift:     0.1,
		},
		Weights: ScoringWeights{
			Availability: 0.25,
			Quota:        0.25,
			Latency:      0.25,
			SuccessRate:  0.25,
		},
	}

	scorer := NewProviderScorer(cfg)
	lab := NewLab(cfg, scorer, nil)

	// Inject a winning shadow hypothesis
	lab.mu.Lock()
	lab.shadowWeights = ScoringWeights{
		Availability: 0.1,
		Quota:        0.1,
		Latency:      0.1,
		SuccessRate:  0.7, // Heavy bias towards success
	}
	
	lab.windowReqCount = 20 // Must be >= 10 to evaluate
	
	// Set the shadow RQS to be significantly higher than production (Avg 0.60 vs Prod 0.50)
	lab.windowProdRQS = 20 * 0.50 
	lab.windowShadowRQS = 20 * 0.60 
	lab.mu.Unlock()

	// Force adaptation evaluation
	lab.evaluateAndAdapt()

	// Check if the scorer's weights were updated to the winning shadow configuration
	updatedWeights := scorer.weights
	assert.InDelta(t, 0.7, updatedWeights.SuccessRate, 0.001, "Scorer weights should be updated to the winning shadow weights")
	
	// Verify window was reset
	lab.mu.RLock()
	assert.Equal(t, 0, lab.windowReqCount)
	assert.Equal(t, float64(0), lab.windowProdRQS)
	lab.mu.RUnlock()
}

func TestOptimizationLab_EvaluateAndAdapt_Discard(t *testing.T) {
	cfg := Config{
		Lab: LabConfig{
			Enabled:            true,
			AdaptationInterval: 1 * time.Second,
		},
		Weights: ScoringWeights{
			Availability: 0.25,
			Quota:        0.25,
			Latency:      0.25,
			SuccessRate:  0.25,
		},
	}

	scorer := NewProviderScorer(cfg)
	lab := NewLab(cfg, scorer, nil)

	// Inject a losing shadow hypothesis
	lab.mu.Lock()
	lab.shadowWeights = ScoringWeights{
		Availability: 0.1,
		Quota:        0.1,
		Latency:      0.1,
		SuccessRate:  0.7,
	}
	
	lab.windowReqCount = 20
	
	// Set shadow RQS to be WORSE than production
	lab.windowProdRQS = 20 * 0.60
	lab.windowShadowRQS = 20 * 0.50
	lab.mu.Unlock()

	lab.evaluateAndAdapt()

	// Verify the original weights were kept (the hypothesis was discarded)
	assert.InDelta(t, 0.25, scorer.weights.SuccessRate, 0.001, "Scorer weights should NOT be updated if shadow hypothesis was worse")
}
