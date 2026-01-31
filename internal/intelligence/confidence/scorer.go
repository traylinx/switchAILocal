// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package confidence

import (
	"encoding/json"
	"fmt"
	"sync"
)

// ConfidenceScorer handles extracting and tracking confidence scores from LLM responses.
type Scorer struct {
	mu sync.RWMutex
	// Metrics
	totalClassifications int
	confidenceSum        float64
	lowConfidenceCount   int // < 0.60
	highConfidenceCount  int // > 0.90
}

// Result represents the parsed confidence result.
type Result struct {
	Confidence float64 `json:"confidence"`
	Intent     string  `json:"intent"`
	Complexity string  `json:"complexity"`
}

// NewScorer creates a new ConfidenceScorer instance.
func NewScorer() *Scorer {
	return &Scorer{}
}

// Parse extracts the confidence and intent from a JSON response string.
func (s *Scorer) Parse(jsonStr string) (*Result, error) {
	var res Result
	if err := json.Unmarshal([]byte(jsonStr), &res); err != nil {
		return nil, fmt.Errorf("failed to parse classification JSON: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.totalClassifications++
	s.confidenceSum += res.Confidence
	if res.Confidence < 0.60 {
		s.lowConfidenceCount++
	} else if res.Confidence > 0.90 {
		s.highConfidenceCount++
	}

	return &res, nil
}

// GetMetrics returns confidence distribution metrics.
func (s *Scorer) GetMetrics() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	avg := 0.0
	if s.totalClassifications > 0 {
		avg = s.confidenceSum / float64(s.totalClassifications)
	}

	return map[string]interface{}{
		"total_classifications": s.totalClassifications,
		"average_confidence":    avg,
		"low_confidence_count":  s.lowConfidenceCount,
		"high_confidence_count": s.highConfidenceCount,
	}
}
