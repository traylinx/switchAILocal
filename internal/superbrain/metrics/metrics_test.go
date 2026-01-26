package metrics

import (
	"sync"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	t.Run("creates metrics with specified max samples", func(t *testing.T) {
		m := New(500)
		if m == nil {
			t.Fatal("expected non-nil metrics")
		}
		if m.maxSamples != 500 {
			t.Errorf("expected maxSamples=500, got %d", m.maxSamples)
		}
	})

	t.Run("uses default max samples when zero", func(t *testing.T) {
		m := New(0)
		if m.maxSamples != 1000 {
			t.Errorf("expected default maxSamples=1000, got %d", m.maxSamples)
		}
	})

	t.Run("uses default max samples when negative", func(t *testing.T) {
		m := New(-10)
		if m.maxSamples != 1000 {
			t.Errorf("expected default maxSamples=1000, got %d", m.maxSamples)
		}
	})
}

func TestCounters(t *testing.T) {
	m := New(100)

	t.Run("healing attempts counter", func(t *testing.T) {
		m.RecordHealingAttempt()
		m.RecordHealingAttempt()
		m.RecordHealingAttempt()

		snapshot := m.Snapshot()
		if snapshot.HealingAttempts != 3 {
			t.Errorf("expected 3 healing attempts, got %d", snapshot.HealingAttempts)
		}
	})

	t.Run("successful healings counter", func(t *testing.T) {
		m.RecordHealingSuccess(100)
		m.RecordHealingSuccess(200)

		snapshot := m.Snapshot()
		if snapshot.SuccessfulHealings != 2 {
			t.Errorf("expected 2 successful healings, got %d", snapshot.SuccessfulHealings)
		}
	})

	t.Run("failed healings counter", func(t *testing.T) {
		m.RecordHealingFailure()

		snapshot := m.Snapshot()
		if snapshot.FailedHealings != 1 {
			t.Errorf("expected 1 failed healing, got %d", snapshot.FailedHealings)
		}
	})

	t.Run("silence detections counter", func(t *testing.T) {
		m.RecordSilenceDetection()
		m.RecordSilenceDetection()

		snapshot := m.Snapshot()
		if snapshot.SilenceDetections != 2 {
			t.Errorf("expected 2 silence detections, got %d", snapshot.SilenceDetections)
		}
	})

	t.Run("diagnoses performed counter", func(t *testing.T) {
		m.RecordDiagnosis()

		snapshot := m.Snapshot()
		if snapshot.DiagnosesPerformed != 1 {
			t.Errorf("expected 1 diagnosis, got %d", snapshot.DiagnosesPerformed)
		}
	})

	t.Run("fallbacks triggered counter", func(t *testing.T) {
		m.RecordFallback()

		snapshot := m.Snapshot()
		if snapshot.FallbacksTriggered != 1 {
			t.Errorf("expected 1 fallback, got %d", snapshot.FallbacksTriggered)
		}
	})

	t.Run("stdin injections counter", func(t *testing.T) {
		m.RecordStdinInjection()
		m.RecordStdinInjection()

		snapshot := m.Snapshot()
		if snapshot.StdinInjectionsTotal != 2 {
			t.Errorf("expected 2 stdin injections, got %d", snapshot.StdinInjectionsTotal)
		}
	})

	t.Run("restarts counter", func(t *testing.T) {
		m.RecordRestart()

		snapshot := m.Snapshot()
		if snapshot.RestartsTotal != 1 {
			t.Errorf("expected 1 restart, got %d", snapshot.RestartsTotal)
		}
	})

	t.Run("context optimizations counter", func(t *testing.T) {
		m.RecordContextOptimization()

		snapshot := m.Snapshot()
		if snapshot.ContextOptimizations != 1 {
			t.Errorf("expected 1 context optimization, got %d", snapshot.ContextOptimizations)
		}
	})
}

func TestHealingByType(t *testing.T) {
	m := New(100)

	m.RecordHealingByType("stdin_injection")
	m.RecordHealingByType("stdin_injection")
	m.RecordHealingByType("restart_with_flags")
	m.RecordHealingByType("fallback_routing")
	m.RecordHealingByType("fallback_routing")
	m.RecordHealingByType("fallback_routing")

	snapshot := m.Snapshot()

	expected := map[string]int64{
		"stdin_injection":    2,
		"restart_with_flags": 1,
		"fallback_routing":   3,
	}

	for healingType, expectedCount := range expected {
		actualCount, exists := snapshot.HealingByType[healingType]
		if !exists {
			t.Errorf("expected healing type %s to exist in snapshot", healingType)
			continue
		}
		if actualCount != expectedCount {
			t.Errorf("healing type %s: expected count %d, got %d", healingType, expectedCount, actualCount)
		}
	}
}

func TestFailureByType(t *testing.T) {
	m := New(100)

	m.RecordFailureByType("permission_prompt")
	m.RecordFailureByType("permission_prompt")
	m.RecordFailureByType("auth_error")
	m.RecordFailureByType("context_exceeded")
	m.RecordFailureByType("rate_limit")
	m.RecordFailureByType("rate_limit")

	snapshot := m.Snapshot()

	expected := map[string]int64{
		"permission_prompt": 2,
		"auth_error":        1,
		"context_exceeded":  1,
		"rate_limit":        2,
	}

	for failureType, expectedCount := range expected {
		actualCount, exists := snapshot.FailureByType[failureType]
		if !exists {
			t.Errorf("expected failure type %s to exist in snapshot", failureType)
			continue
		}
		if actualCount != expectedCount {
			t.Errorf("failure type %s: expected count %d, got %d", failureType, expectedCount, actualCount)
		}
	}
}

func TestLatencyTracking(t *testing.T) {
	m := New(5) // Small buffer for testing

	t.Run("tracks latency samples", func(t *testing.T) {
		m.RecordHealingSuccess(100)
		m.RecordHealingSuccess(200)
		m.RecordHealingSuccess(150)

		snapshot := m.Snapshot()
		stats := snapshot.LatencyStats

		if stats.Samples != 3 {
			t.Errorf("expected 3 samples, got %d", stats.Samples)
		}
		if stats.MinMs != 100 {
			t.Errorf("expected min 100ms, got %d", stats.MinMs)
		}
		if stats.MaxMs != 200 {
			t.Errorf("expected max 200ms, got %d", stats.MaxMs)
		}
		if stats.AverageMs != 150 {
			t.Errorf("expected average 150ms, got %d", stats.AverageMs)
		}
	})

	t.Run("limits samples to maxSamples", func(t *testing.T) {
		m2 := New(3)
		m2.RecordHealingSuccess(100)
		m2.RecordHealingSuccess(200)
		m2.RecordHealingSuccess(300)
		m2.RecordHealingSuccess(400) // Should evict 100
		m2.RecordHealingSuccess(500) // Should evict 200

		snapshot := m2.Snapshot()
		stats := snapshot.LatencyStats

		if stats.Samples != 3 {
			t.Errorf("expected 3 samples (max), got %d", stats.Samples)
		}
		if stats.MinMs != 300 {
			t.Errorf("expected min 300ms (oldest evicted), got %d", stats.MinMs)
		}
		if stats.MaxMs != 500 {
			t.Errorf("expected max 500ms, got %d", stats.MaxMs)
		}
	})

	t.Run("handles empty latency samples", func(t *testing.T) {
		m3 := New(100)
		snapshot := m3.Snapshot()
		stats := snapshot.LatencyStats

		if stats.Samples != 0 {
			t.Errorf("expected 0 samples, got %d", stats.Samples)
		}
		if stats.AverageMs != 0 {
			t.Errorf("expected average 0ms, got %d", stats.AverageMs)
		}
	})
}

func TestGauges(t *testing.T) {
	m := New(100)

	t.Run("active monitoring contexts gauge", func(t *testing.T) {
		m.IncrementActiveMonitoring()
		m.IncrementActiveMonitoring()
		m.IncrementActiveMonitoring()

		snapshot := m.Snapshot()
		if snapshot.ActiveMonitoringContexts != 3 {
			t.Errorf("expected 3 active contexts, got %d", snapshot.ActiveMonitoringContexts)
		}

		m.DecrementActiveMonitoring()

		snapshot = m.Snapshot()
		if snapshot.ActiveMonitoringContexts != 2 {
			t.Errorf("expected 2 active contexts after decrement, got %d", snapshot.ActiveMonitoringContexts)
		}
	})

	t.Run("queued healing actions gauge", func(t *testing.T) {
		m.IncrementQueuedActions()
		m.IncrementQueuedActions()

		snapshot := m.Snapshot()
		if snapshot.QueuedHealingActions != 2 {
			t.Errorf("expected 2 queued actions, got %d", snapshot.QueuedHealingActions)
		}

		m.DecrementQueuedActions()
		m.DecrementQueuedActions()

		snapshot = m.Snapshot()
		if snapshot.QueuedHealingActions != 0 {
			t.Errorf("expected 0 queued actions after decrement, got %d", snapshot.QueuedHealingActions)
		}
	})
}

func TestSnapshot(t *testing.T) {
	m := New(100)

	// Record various metrics
	m.RecordHealingAttempt()
	m.RecordHealingAttempt()
	m.RecordHealingSuccess(100)
	m.RecordHealingFailure()
	m.RecordSilenceDetection()
	m.RecordDiagnosis()
	m.RecordFallback()
	m.RecordHealingByType("stdin_injection")
	m.RecordFailureByType("permission_prompt")

	snapshot := m.Snapshot()

	t.Run("snapshot contains all counters", func(t *testing.T) {
		if snapshot.HealingAttempts != 2 {
			t.Errorf("expected 2 healing attempts, got %d", snapshot.HealingAttempts)
		}
		if snapshot.SuccessfulHealings != 1 {
			t.Errorf("expected 1 successful healing, got %d", snapshot.SuccessfulHealings)
		}
		if snapshot.FailedHealings != 1 {
			t.Errorf("expected 1 failed healing, got %d", snapshot.FailedHealings)
		}
	})

	t.Run("snapshot contains by-type breakdowns", func(t *testing.T) {
		if len(snapshot.HealingByType) != 1 {
			t.Errorf("expected 1 healing type, got %d", len(snapshot.HealingByType))
		}
		if len(snapshot.FailureByType) != 1 {
			t.Errorf("expected 1 failure type, got %d", len(snapshot.FailureByType))
		}
	})

	t.Run("snapshot contains metadata", func(t *testing.T) {
		if snapshot.UptimeSeconds < 0 {
			t.Errorf("expected non-negative uptime, got %d", snapshot.UptimeSeconds)
		}
		if snapshot.Timestamp.IsZero() {
			t.Error("expected non-zero timestamp")
		}
	})

	t.Run("snapshot is independent copy", func(t *testing.T) {
		// Modify original metrics
		m.RecordHealingAttempt()

		// Snapshot should not change
		if snapshot.HealingAttempts != 2 {
			t.Errorf("snapshot should be immutable, expected 2, got %d", snapshot.HealingAttempts)
		}

		// New snapshot should reflect changes
		newSnapshot := m.Snapshot()
		if newSnapshot.HealingAttempts != 3 {
			t.Errorf("new snapshot should reflect changes, expected 3, got %d", newSnapshot.HealingAttempts)
		}
	})
}

func TestSnapshotRates(t *testing.T) {
	t.Run("success rate calculation", func(t *testing.T) {
		m := New(100)
		m.RecordHealingAttempt()
		m.RecordHealingAttempt()
		m.RecordHealingAttempt()
		m.RecordHealingAttempt()
		m.RecordHealingSuccess(100)
		m.RecordHealingSuccess(100)
		m.RecordHealingSuccess(100)

		snapshot := m.Snapshot()
		successRate := snapshot.SuccessRate()

		expected := 75.0 // 3 successes out of 4 attempts
		if successRate != expected {
			t.Errorf("expected success rate %.2f%%, got %.2f%%", expected, successRate)
		}
	})

	t.Run("failure rate calculation", func(t *testing.T) {
		m := New(100)
		m.RecordHealingAttempt()
		m.RecordHealingAttempt()
		m.RecordHealingAttempt()
		m.RecordHealingAttempt()
		m.RecordHealingFailure()

		snapshot := m.Snapshot()
		failureRate := snapshot.FailureRate()

		expected := 25.0 // 1 failure out of 4 attempts
		if failureRate != expected {
			t.Errorf("expected failure rate %.2f%%, got %.2f%%", expected, failureRate)
		}
	})

	t.Run("rates with no attempts", func(t *testing.T) {
		m := New(100)
		snapshot := m.Snapshot()

		if snapshot.SuccessRate() != 0.0 {
			t.Errorf("expected 0%% success rate with no attempts, got %.2f%%", snapshot.SuccessRate())
		}
		if snapshot.FailureRate() != 0.0 {
			t.Errorf("expected 0%% failure rate with no attempts, got %.2f%%", snapshot.FailureRate())
		}
	})
}

func TestReset(t *testing.T) {
	m := New(100)

	// Record various metrics
	m.RecordHealingAttempt()
	m.RecordHealingSuccess(100)
	m.RecordHealingFailure()
	m.RecordSilenceDetection()
	m.RecordDiagnosis()
	m.RecordFallback()
	m.RecordStdinInjection()
	m.RecordRestart()
	m.RecordContextOptimization()
	m.RecordHealingByType("stdin_injection")
	m.RecordFailureByType("permission_prompt")
	m.IncrementActiveMonitoring()
	m.IncrementQueuedActions()

	// Reset
	m.Reset()

	snapshot := m.Snapshot()

	// Verify all counters are zero
	if snapshot.HealingAttempts != 0 {
		t.Errorf("expected 0 healing attempts after reset, got %d", snapshot.HealingAttempts)
	}
	if snapshot.SuccessfulHealings != 0 {
		t.Errorf("expected 0 successful healings after reset, got %d", snapshot.SuccessfulHealings)
	}
	if snapshot.FailedHealings != 0 {
		t.Errorf("expected 0 failed healings after reset, got %d", snapshot.FailedHealings)
	}
	if snapshot.SilenceDetections != 0 {
		t.Errorf("expected 0 silence detections after reset, got %d", snapshot.SilenceDetections)
	}
	if snapshot.DiagnosesPerformed != 0 {
		t.Errorf("expected 0 diagnoses after reset, got %d", snapshot.DiagnosesPerformed)
	}
	if snapshot.FallbacksTriggered != 0 {
		t.Errorf("expected 0 fallbacks after reset, got %d", snapshot.FallbacksTriggered)
	}
	if snapshot.StdinInjectionsTotal != 0 {
		t.Errorf("expected 0 stdin injections after reset, got %d", snapshot.StdinInjectionsTotal)
	}
	if snapshot.RestartsTotal != 0 {
		t.Errorf("expected 0 restarts after reset, got %d", snapshot.RestartsTotal)
	}
	if snapshot.ContextOptimizations != 0 {
		t.Errorf("expected 0 context optimizations after reset, got %d", snapshot.ContextOptimizations)
	}
	if snapshot.ActiveMonitoringContexts != 0 {
		t.Errorf("expected 0 active monitoring contexts after reset, got %d", snapshot.ActiveMonitoringContexts)
	}
	if snapshot.QueuedHealingActions != 0 {
		t.Errorf("expected 0 queued actions after reset, got %d", snapshot.QueuedHealingActions)
	}

	// Verify maps are empty
	if len(snapshot.HealingByType) != 0 {
		t.Errorf("expected empty healing by type map after reset, got %d entries", len(snapshot.HealingByType))
	}
	if len(snapshot.FailureByType) != 0 {
		t.Errorf("expected empty failure by type map after reset, got %d entries", len(snapshot.FailureByType))
	}

	// Verify latency samples are cleared
	if snapshot.LatencyStats.Samples != 0 {
		t.Errorf("expected 0 latency samples after reset, got %d", snapshot.LatencyStats.Samples)
	}
}

func TestConcurrency(t *testing.T) {
	m := New(1000)
	iterations := 1000
	goroutines := 10

	var wg sync.WaitGroup
	wg.Add(goroutines)

	// Concurrent writes
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				m.RecordHealingAttempt()
				m.RecordHealingSuccess(100)
				m.RecordHealingByType("test_type")
				m.RecordFailureByType("test_failure")
				m.IncrementActiveMonitoring()
				m.DecrementActiveMonitoring()
			}
		}()
	}

	// Concurrent reads
	for i := 0; i < goroutines; i++ {
		go func() {
			for j := 0; j < iterations; j++ {
				_ = m.Snapshot()
			}
		}()
	}

	wg.Wait()

	snapshot := m.Snapshot()

	expectedAttempts := int64(goroutines * iterations)
	if snapshot.HealingAttempts != expectedAttempts {
		t.Errorf("expected %d healing attempts, got %d", expectedAttempts, snapshot.HealingAttempts)
	}

	expectedSuccesses := int64(goroutines * iterations)
	if snapshot.SuccessfulHealings != expectedSuccesses {
		t.Errorf("expected %d successful healings, got %d", expectedSuccesses, snapshot.SuccessfulHealings)
	}
}

func TestGlobal(t *testing.T) {
	t.Run("returns same instance", func(t *testing.T) {
		m1 := Global()
		m2 := Global()

		if m1 != m2 {
			t.Error("Global() should return the same instance")
		}
	})

	t.Run("is initialized with defaults", func(t *testing.T) {
		m := Global()
		if m == nil {
			t.Fatal("expected non-nil global metrics")
		}
		if m.maxSamples != 1000 {
			t.Errorf("expected maxSamples=1000, got %d", m.maxSamples)
		}
	})
}

func TestUptimeTracking(t *testing.T) {
	m := New(100)

	// Wait a bit to ensure uptime is measurable
	time.Sleep(100 * time.Millisecond)

	snapshot := m.Snapshot()

	if snapshot.UptimeSeconds < 0 {
		t.Errorf("expected non-negative uptime, got %d", snapshot.UptimeSeconds)
	}

	// Uptime should be at least 0 seconds (may be 0 if very fast)
	if snapshot.UptimeSeconds < 0 {
		t.Errorf("uptime should be >= 0, got %d", snapshot.UptimeSeconds)
	}
}
