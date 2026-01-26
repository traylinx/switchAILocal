package router

import (
	"sync"
	"time"
)

// ProviderStats tracks historical performance for a provider.
type ProviderStats struct {
	// Provider is the unique identifier for this provider.
	Provider string

	// TotalRequests is the total number of requests made to this provider.
	TotalRequests int64

	// SuccessCount is the number of successful requests.
	SuccessCount int64

	// FailureCount is the number of failed requests.
	FailureCount int64

	// TotalLatencyMs is the cumulative latency in milliseconds.
	TotalLatencyMs int64

	// LastSuccess is the timestamp of the last successful request.
	LastSuccess time.Time

	// LastFailure is the timestamp of the last failed request.
	LastFailure time.Time

	// FailureReasons maps failure reasons to their occurrence counts.
	FailureReasons map[string]int64
}

// SuccessRate calculates the success rate for this provider.
func (s *ProviderStats) SuccessRate() float64 {
	if s.TotalRequests == 0 {
		return 1.0 // No data, assume success
	}
	return float64(s.SuccessCount) / float64(s.TotalRequests)
}

// AverageLatency calculates the average latency for this provider.
func (s *ProviderStats) AverageLatency() time.Duration {
	if s.TotalRequests == 0 {
		return 0
	}
	avgMs := s.TotalLatencyMs / s.TotalRequests
	return time.Duration(avgMs) * time.Millisecond
}

// StatsTracker manages provider statistics with thread-safe access.
type StatsTracker struct {
	mu    sync.RWMutex
	stats map[string]*ProviderStats
}

// NewStatsTracker creates a new statistics tracker.
func NewStatsTracker() *StatsTracker {
	return &StatsTracker{
		stats: make(map[string]*ProviderStats),
	}
}

// GetStats returns the statistics for a provider.
// Returns nil if no stats exist for the provider.
func (t *StatsTracker) GetStats(provider string) *ProviderStats {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.stats[provider]
}

// GetOrCreateStats returns existing stats or creates new ones for a provider.
func (t *StatsTracker) GetOrCreateStats(provider string) *ProviderStats {
	t.mu.Lock()
	defer t.mu.Unlock()

	if stats, ok := t.stats[provider]; ok {
		return stats
	}

	stats := &ProviderStats{
		Provider:       provider,
		FailureReasons: make(map[string]int64),
	}
	t.stats[provider] = stats
	return stats
}

// UpdateStats records a request outcome for a provider.
func (t *StatsTracker) UpdateStats(provider string, success bool, latency time.Duration, failureReason string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	stats, ok := t.stats[provider]
	if !ok {
		stats = &ProviderStats{
			Provider:       provider,
			FailureReasons: make(map[string]int64),
		}
		t.stats[provider] = stats
	}

	stats.TotalRequests++
	stats.TotalLatencyMs += latency.Milliseconds()

	if success {
		stats.SuccessCount++
		stats.LastSuccess = time.Now()
	} else {
		stats.FailureCount++
		stats.LastFailure = time.Now()
		if failureReason != "" {
			stats.FailureReasons[failureReason]++
		}
	}
}

// GetAllStats returns statistics for all tracked providers.
func (t *StatsTracker) GetAllStats() map[string]*ProviderStats {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make(map[string]*ProviderStats, len(t.stats))
	for k, v := range t.stats {
		// Return a copy to avoid race conditions
		statsCopy := *v
		statsCopy.FailureReasons = make(map[string]int64)
		for reason, count := range v.FailureReasons {
			statsCopy.FailureReasons[reason] = count
		}
		result[k] = &statsCopy
	}
	return result
}

// Reset clears all statistics.
func (t *StatsTracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.stats = make(map[string]*ProviderStats)
}
