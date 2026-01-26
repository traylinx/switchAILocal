// Package metrics provides observability infrastructure for the Superbrain system.
// It tracks healing attempts, diagnoses, fallbacks, and other autonomous actions
// to enable monitoring and performance analysis of the self-healing capabilities.
package metrics

import (
	"sync"
	"sync/atomic"
	"time"
)

// Metrics tracks all Superbrain operations for observability.
// This is a Day-0 requirement for autonomous systems - we need comprehensive
// metrics to monitor healing effectiveness and system behavior.
type Metrics struct {
	// Counters track cumulative counts of events
	healingAttempts      atomic.Int64
	successfulHealings   atomic.Int64
	failedHealings       atomic.Int64
	silenceDetections    atomic.Int64
	diagnosesPerformed   atomic.Int64
	fallbacksTriggered   atomic.Int64
	stdinInjectionsTotal atomic.Int64
	restartsTotal        atomic.Int64
	contextOptimizations atomic.Int64

	// Healing by type counters
	healingByTypeMu sync.RWMutex
	healingByType   map[string]int64

	// Healing by failure type counters
	failureByTypeMu sync.RWMutex
	failureByType   map[string]int64

	// Latency tracking (in milliseconds)
	latencyMu      sync.RWMutex
	latencySamples []int64
	maxSamples     int

	// Gauges track current state
	activeMonitoringContexts atomic.Int64
	queuedHealingActions     atomic.Int64

	// Timestamps for rate calculations
	startTime time.Time
}

// New creates a new Metrics instance with the specified maximum latency samples.
// The maxSamples parameter controls how many latency measurements are kept for
// calculating percentiles and averages.
func New(maxSamples int) *Metrics {
	if maxSamples <= 0 {
		maxSamples = 1000 // Default to keeping last 1000 samples
	}

	return &Metrics{
		healingByType:  make(map[string]int64),
		failureByType:  make(map[string]int64),
		latencySamples: make([]int64, 0, maxSamples),
		maxSamples:     maxSamples,
		startTime:      time.Now(),
	}
}

// RecordHealingAttempt increments the total healing attempts counter.
func (m *Metrics) RecordHealingAttempt() {
	m.healingAttempts.Add(1)
}

// RecordHealingSuccess increments the successful healings counter and records latency.
// The latencyMs parameter is the time taken for the healing action to complete.
func (m *Metrics) RecordHealingSuccess(latencyMs int64) {
	m.successfulHealings.Add(1)
	m.recordLatency(latencyMs)
}

// RecordHealingFailure increments the failed healings counter.
func (m *Metrics) RecordHealingFailure() {
	m.failedHealings.Add(1)
}

// RecordHealingByType increments the counter for a specific healing action type.
// Common types include: "stdin_injection", "restart_with_flags", "fallback_routing", "context_optimization".
func (m *Metrics) RecordHealingByType(healingType string) {
	m.healingByTypeMu.Lock()
	defer m.healingByTypeMu.Unlock()
	m.healingByType[healingType]++
}

// RecordFailureByType increments the counter for a specific failure type.
// Common types include: "permission_prompt", "auth_error", "context_exceeded", "rate_limit", etc.
func (m *Metrics) RecordFailureByType(failureType string) {
	m.failureByTypeMu.Lock()
	defer m.failureByTypeMu.Unlock()
	m.failureByType[failureType]++
}

// RecordSilenceDetection increments the counter for silence threshold detections.
func (m *Metrics) RecordSilenceDetection() {
	m.silenceDetections.Add(1)
}

// RecordDiagnosis increments the counter for diagnoses performed by the Internal Doctor.
func (m *Metrics) RecordDiagnosis() {
	m.diagnosesPerformed.Add(1)
}

// RecordFallback increments the counter for fallback routing events.
func (m *Metrics) RecordFallback() {
	m.fallbacksTriggered.Add(1)
}

// RecordStdinInjection increments the counter for stdin injection attempts.
func (m *Metrics) RecordStdinInjection() {
	m.stdinInjectionsTotal.Add(1)
}

// RecordRestart increments the counter for process restart attempts.
func (m *Metrics) RecordRestart() {
	m.restartsTotal.Add(1)
}

// RecordContextOptimization increments the counter for context sculpting operations.
func (m *Metrics) RecordContextOptimization() {
	m.contextOptimizations.Add(1)
}

// IncrementActiveMonitoring increments the gauge for active monitoring contexts.
// This should be called when a new Overwatch monitoring context is created.
func (m *Metrics) IncrementActiveMonitoring() {
	m.activeMonitoringContexts.Add(1)
}

// DecrementActiveMonitoring decrements the gauge for active monitoring contexts.
// This should be called when an Overwatch monitoring context completes.
func (m *Metrics) DecrementActiveMonitoring() {
	m.activeMonitoringContexts.Add(-1)
}

// IncrementQueuedActions increments the gauge for queued healing actions.
// This is used in human-in-the-loop mode when actions await approval.
func (m *Metrics) IncrementQueuedActions() {
	m.queuedHealingActions.Add(1)
}

// DecrementQueuedActions decrements the gauge for queued healing actions.
func (m *Metrics) DecrementQueuedActions() {
	m.queuedHealingActions.Add(-1)
}

// recordLatency adds a latency sample to the histogram.
// Keeps only the most recent maxSamples measurements.
func (m *Metrics) recordLatency(latencyMs int64) {
	m.latencyMu.Lock()
	defer m.latencyMu.Unlock()

	m.latencySamples = append(m.latencySamples, latencyMs)

	// Keep only the most recent samples
	if len(m.latencySamples) > m.maxSamples {
		m.latencySamples = m.latencySamples[len(m.latencySamples)-m.maxSamples:]
	}
}

// Snapshot returns a point-in-time view of all metrics.
// This is safe to call concurrently and returns a copy of the current state.
func (m *Metrics) Snapshot() *Snapshot {
	m.healingByTypeMu.RLock()
	healingByTypeCopy := make(map[string]int64, len(m.healingByType))
	for k, v := range m.healingByType {
		healingByTypeCopy[k] = v
	}
	m.healingByTypeMu.RUnlock()

	m.failureByTypeMu.RLock()
	failureByTypeCopy := make(map[string]int64, len(m.failureByType))
	for k, v := range m.failureByType {
		failureByTypeCopy[k] = v
	}
	m.failureByTypeMu.RUnlock()

	m.latencyMu.RLock()
	latencyStats := m.calculateLatencyStats()
	m.latencyMu.RUnlock()

	uptime := time.Since(m.startTime)

	return &Snapshot{
		// Counters
		HealingAttempts:      m.healingAttempts.Load(),
		SuccessfulHealings:   m.successfulHealings.Load(),
		FailedHealings:       m.failedHealings.Load(),
		SilenceDetections:    m.silenceDetections.Load(),
		DiagnosesPerformed:   m.diagnosesPerformed.Load(),
		FallbacksTriggered:   m.fallbacksTriggered.Load(),
		StdinInjectionsTotal: m.stdinInjectionsTotal.Load(),
		RestartsTotal:        m.restartsTotal.Load(),
		ContextOptimizations: m.contextOptimizations.Load(),

		// By-type breakdowns
		HealingByType: healingByTypeCopy,
		FailureByType: failureByTypeCopy,

		// Latency statistics
		LatencyStats: latencyStats,

		// Gauges
		ActiveMonitoringContexts: m.activeMonitoringContexts.Load(),
		QueuedHealingActions:     m.queuedHealingActions.Load(),

		// Metadata
		UptimeSeconds: int64(uptime.Seconds()),
		Timestamp:     time.Now(),
	}
}

// calculateLatencyStats computes statistics from the latency samples.
// Must be called with latencyMu held.
func (m *Metrics) calculateLatencyStats() LatencyStats {
	if len(m.latencySamples) == 0 {
		return LatencyStats{}
	}

	// Calculate average
	var sum int64
	min := m.latencySamples[0]
	max := m.latencySamples[0]

	for _, sample := range m.latencySamples {
		sum += sample
		if sample < min {
			min = sample
		}
		if sample > max {
			max = sample
		}
	}

	avg := sum / int64(len(m.latencySamples))

	return LatencyStats{
		AverageMs: avg,
		MinMs:     min,
		MaxMs:     max,
		Samples:   int64(len(m.latencySamples)),
	}
}

// Reset clears all metrics. This is primarily useful for testing.
func (m *Metrics) Reset() {
	m.healingAttempts.Store(0)
	m.successfulHealings.Store(0)
	m.failedHealings.Store(0)
	m.silenceDetections.Store(0)
	m.diagnosesPerformed.Store(0)
	m.fallbacksTriggered.Store(0)
	m.stdinInjectionsTotal.Store(0)
	m.restartsTotal.Store(0)
	m.contextOptimizations.Store(0)
	m.activeMonitoringContexts.Store(0)
	m.queuedHealingActions.Store(0)

	m.healingByTypeMu.Lock()
	m.healingByType = make(map[string]int64)
	m.healingByTypeMu.Unlock()

	m.failureByTypeMu.Lock()
	m.failureByType = make(map[string]int64)
	m.failureByTypeMu.Unlock()

	m.latencyMu.Lock()
	m.latencySamples = make([]int64, 0, m.maxSamples)
	m.latencyMu.Unlock()

	m.startTime = time.Now()
}

// Snapshot represents a point-in-time view of all Superbrain metrics.
// This structure is safe to serialize and expose via API endpoints.
type Snapshot struct {
	// Counters
	HealingAttempts      int64 `json:"healing_attempts"`
	SuccessfulHealings   int64 `json:"successful_healings"`
	FailedHealings       int64 `json:"failed_healings"`
	SilenceDetections    int64 `json:"silence_detections"`
	DiagnosesPerformed   int64 `json:"diagnoses_performed"`
	FallbacksTriggered   int64 `json:"fallbacks_triggered"`
	StdinInjectionsTotal int64 `json:"stdin_injections_total"`
	RestartsTotal        int64 `json:"restarts_total"`
	ContextOptimizations int64 `json:"context_optimizations"`

	// By-type breakdowns
	HealingByType map[string]int64 `json:"healing_by_type"`
	FailureByType map[string]int64 `json:"failure_by_type"`

	// Latency statistics
	LatencyStats LatencyStats `json:"latency_stats"`

	// Gauges
	ActiveMonitoringContexts int64 `json:"active_monitoring_contexts"`
	QueuedHealingActions     int64 `json:"queued_healing_actions"`

	// Metadata
	UptimeSeconds int64     `json:"uptime_seconds"`
	Timestamp     time.Time `json:"timestamp"`
}

// LatencyStats contains statistical information about healing latencies.
type LatencyStats struct {
	AverageMs int64 `json:"average_ms"`
	MinMs     int64 `json:"min_ms"`
	MaxMs     int64 `json:"max_ms"`
	Samples   int64 `json:"samples"`
}

// SuccessRate calculates the healing success rate as a percentage (0-100).
// Returns 0 if no healing attempts have been made.
func (s *Snapshot) SuccessRate() float64 {
	if s.HealingAttempts == 0 {
		return 0.0
	}
	return (float64(s.SuccessfulHealings) / float64(s.HealingAttempts)) * 100.0
}

// FailureRate calculates the healing failure rate as a percentage (0-100).
// Returns 0 if no healing attempts have been made.
func (s *Snapshot) FailureRate() float64 {
	if s.HealingAttempts == 0 {
		return 0.0
	}
	return (float64(s.FailedHealings) / float64(s.HealingAttempts)) * 100.0
}

// Global metrics instance for the Superbrain system.
// This is initialized once and shared across all components.
var globalMetrics *Metrics
var once sync.Once

// Global returns the global Metrics instance, initializing it if necessary.
func Global() *Metrics {
	once.Do(func() {
		globalMetrics = New(1000) // Keep last 1000 latency samples
	})
	return globalMetrics
}
