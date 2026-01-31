// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package cascade

import (
	"sync"
	"sync/atomic"
	"time"
)

// Tier represents a model capability tier for cascading.
type Tier string

const (
	// TierFast is the cheapest, fastest tier (e.g., small models)
	TierFast Tier = "fast"
	// TierStandard is the balanced tier (e.g., medium models)
	TierStandard Tier = "standard"
	// TierReasoning is the most capable tier (e.g., large reasoning models)
	TierReasoning Tier = "reasoning"
)

// CascadeDecision represents the outcome of a cascade evaluation.
type CascadeDecision struct {
	// ShouldCascade indicates whether to retry with a higher tier
	ShouldCascade bool `json:"should_cascade"`
	// CurrentTier is the tier that produced the response
	CurrentTier Tier `json:"current_tier"`
	// NextTier is the recommended tier to try (if ShouldCascade is true)
	NextTier Tier `json:"next_tier,omitempty"`
	// Signals contains the detected quality issues
	Signals []QualitySignal `json:"signals,omitempty"`
	// QualityScore is the overall quality score (0.0-1.0)
	QualityScore float64 `json:"quality_score"`
	// Reason explains the decision
	Reason string `json:"reason"`
}

// CascadeResult tracks the outcome of a cascade operation.
type CascadeResult struct {
	// OriginalTier is the tier that was initially used
	OriginalTier Tier `json:"original_tier"`
	// FinalTier is the tier that produced the accepted response
	FinalTier Tier `json:"final_tier"`
	// CascadeCount is the number of cascade attempts
	CascadeCount int `json:"cascade_count"`
	// TotalLatencyMs is the total time spent across all attempts
	TotalLatencyMs int64 `json:"total_latency_ms"`
	// Success indicates whether a satisfactory response was obtained
	Success bool `json:"success"`
}

// Manager orchestrates model cascading based on response quality.
type Manager struct {
	mu sync.RWMutex

	// Configuration
	qualityThreshold float64
	maxCascades      int
	enabled          bool

	// Quality signal detector
	detector *QualitySignalDetector

	// Metrics (atomic for thread safety)
	totalRequests    int64
	cascadeCount     int64
	successCount     int64
	tierDistribution map[Tier]*int64
}

// Config holds configuration for the CascadeManager.
type Config struct {
	// Enabled toggles cascade functionality
	Enabled bool `yaml:"enabled" json:"enabled"`
	// QualityThreshold is the minimum quality score to accept (0.0-1.0)
	QualityThreshold float64 `yaml:"quality-threshold" json:"quality_threshold"`
	// MaxCascades is the maximum number of cascade attempts
	MaxCascades int `yaml:"max-cascades" json:"max_cascades"`
}

// NewManager creates a new CascadeManager with the given configuration.
func NewManager(cfg Config) *Manager {
	// Apply defaults
	if cfg.QualityThreshold == 0 {
		cfg.QualityThreshold = 0.70
	}
	if cfg.MaxCascades == 0 {
		cfg.MaxCascades = 2 // fast -> standard -> reasoning
	}

	// Initialize tier distribution counters
	fastCount := int64(0)
	standardCount := int64(0)
	reasoningCount := int64(0)

	return &Manager{
		qualityThreshold: cfg.QualityThreshold,
		maxCascades:      cfg.MaxCascades,
		enabled:          cfg.Enabled,
		detector:         NewQualitySignalDetector(),
		tierDistribution: map[Tier]*int64{
			TierFast:      &fastCount,
			TierStandard:  &standardCount,
			TierReasoning: &reasoningCount,
		},
	}
}

// IsEnabled returns whether cascade functionality is active.
func (m *Manager) IsEnabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.enabled
}

// DetectQualitySignals analyzes a response and returns detected quality issues.
func (m *Manager) DetectQualitySignals(response string) []QualitySignal {
	if m.detector == nil {
		return nil
	}
	return m.detector.DetectSignals(response)
}

// EvaluateResponse determines if a response should trigger a cascade.
func (m *Manager) EvaluateResponse(response string, currentTier Tier) *CascadeDecision {
	atomic.AddInt64(&m.totalRequests, 1)

	// Detect quality signals
	signals := m.DetectQualitySignals(response)
	qualityScore := CalculateOverallQuality(signals)

	decision := &CascadeDecision{
		CurrentTier:  currentTier,
		Signals:      signals,
		QualityScore: qualityScore,
	}

	// Check if quality is acceptable
	if qualityScore >= m.qualityThreshold && !HasCriticalSignals(signals) {
		decision.ShouldCascade = false
		decision.Reason = "Response quality is acceptable"
		m.recordTierUsage(currentTier)
		atomic.AddInt64(&m.successCount, 1)
		return decision
	}

	// Determine next tier
	nextTier := m.getNextTier(currentTier)
	if nextTier == "" {
		// Already at highest tier
		decision.ShouldCascade = false
		decision.Reason = "Already at highest tier, accepting response"
		m.recordTierUsage(currentTier)
		return decision
	}

	// Cascade needed
	decision.ShouldCascade = true
	decision.NextTier = nextTier
	decision.Reason = m.buildCascadeReason(signals, qualityScore)
	atomic.AddInt64(&m.cascadeCount, 1)

	return decision
}

// getNextTier returns the next higher tier, or empty string if at max.
func (m *Manager) getNextTier(current Tier) Tier {
	switch current {
	case TierFast:
		return TierStandard
	case TierStandard:
		return TierReasoning
	case TierReasoning:
		return "" // Already at highest
	default:
		return TierStandard // Unknown tier, try standard
	}
}

// buildCascadeReason creates a human-readable reason for cascading.
func (m *Manager) buildCascadeReason(signals []QualitySignal, score float64) string {
	if len(signals) == 0 {
		return "Quality score below threshold"
	}

	// Find the most severe signal
	var mostSevere *QualitySignal
	for i := range signals {
		if mostSevere == nil || signals[i].Severity > mostSevere.Severity {
			mostSevere = &signals[i]
		}
	}

	if mostSevere != nil {
		return mostSevere.Description
	}

	return "Multiple quality issues detected"
}

// recordTierUsage updates tier distribution metrics.
func (m *Manager) recordTierUsage(tier Tier) {
	m.mu.RLock()
	counter, ok := m.tierDistribution[tier]
	m.mu.RUnlock()

	if ok && counter != nil {
		atomic.AddInt64(counter, 1)
	}
}

// GetMetrics returns cascade performance metrics.
func (m *Manager) GetMetrics() map[string]interface{} {
	total := atomic.LoadInt64(&m.totalRequests)
	cascades := atomic.LoadInt64(&m.cascadeCount)
	successes := atomic.LoadInt64(&m.successCount)

	cascadeRate := 0.0
	successRate := 0.0
	if total > 0 {
		cascadeRate = float64(cascades) / float64(total)
		successRate = float64(successes) / float64(total)
	}

	m.mu.RLock()
	tierDist := make(map[string]int64)
	for tier, counter := range m.tierDistribution {
		if counter != nil {
			tierDist[string(tier)] = atomic.LoadInt64(counter)
		}
	}
	m.mu.RUnlock()

	return map[string]interface{}{
		"total_requests":    total,
		"cascade_count":     cascades,
		"success_count":     successes,
		"cascade_rate":      cascadeRate,
		"success_rate":      successRate,
		"tier_distribution": tierDist,
		"quality_threshold": m.qualityThreshold,
		"max_cascades":      m.maxCascades,
	}
}

// GetMetricsAsMap returns metrics in a format suitable for Lua.
func (m *Manager) GetMetricsAsMap() map[string]interface{} {
	return m.GetMetrics()
}

// TierToCapability maps a cascade tier to a capability slot name.
func TierToCapability(tier Tier) string {
	switch tier {
	case TierFast:
		return "fast"
	case TierStandard:
		return "chat"
	case TierReasoning:
		return "reasoning"
	default:
		return "fast"
	}
}

// CapabilityToTier maps a capability slot name to a cascade tier.
func CapabilityToTier(capability string) Tier {
	switch capability {
	case "fast":
		return TierFast
	case "chat", "creative", "coding":
		return TierStandard
	case "reasoning", "research":
		return TierReasoning
	default:
		return TierFast
	}
}

// CascadeTracker tracks an ongoing cascade operation.
type CascadeTracker struct {
	mu sync.Mutex

	originalTier Tier
	currentTier  Tier
	attempts     int
	maxAttempts  int
	startTime    time.Time
	decisions    []*CascadeDecision
}

// NewCascadeTracker creates a tracker for a cascade operation.
func NewCascadeTracker(startTier Tier, maxAttempts int) *CascadeTracker {
	return &CascadeTracker{
		originalTier: startTier,
		currentTier:  startTier,
		maxAttempts:  maxAttempts,
		startTime:    time.Now(),
		decisions:    make([]*CascadeDecision, 0),
	}
}

// RecordAttempt records a cascade attempt and its decision.
func (t *CascadeTracker) RecordAttempt(decision *CascadeDecision) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.attempts++
	t.decisions = append(t.decisions, decision)

	if decision.ShouldCascade && decision.NextTier != "" {
		t.currentTier = decision.NextTier
	}
}

// CanContinue returns whether more cascade attempts are allowed.
func (t *CascadeTracker) CanContinue() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.attempts < t.maxAttempts
}

// GetCurrentTier returns the current tier being used.
func (t *CascadeTracker) GetCurrentTier() Tier {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.currentTier
}

// GetResult returns the final cascade result.
func (t *CascadeTracker) GetResult(success bool) *CascadeResult {
	t.mu.Lock()
	defer t.mu.Unlock()

	return &CascadeResult{
		OriginalTier:   t.originalTier,
		FinalTier:      t.currentTier,
		CascadeCount:   t.attempts - 1, // First attempt is not a cascade
		TotalLatencyMs: time.Since(t.startTime).Milliseconds(),
		Success:        success,
	}
}
