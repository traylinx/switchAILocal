// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package autoroute

import (
	"net/http"
	"testing"
	"time"
)

// TestComputeCooldownBackoff_BoundedAndMonotonicUntilCap verifies that the
// jittered backoff stays within ±10% of the deterministic value, is
// non-decreasing for the first few attempts, and is capped at the max.
func TestComputeCooldownBackoff_BoundedAndMonotonicUntilCap(t *testing.T) {
	cases := []struct {
		attempts    int
		idealMillis int64
	}{
		{0, 5_000},
		{1, 10_000},
		{2, 20_000},
		{3, 40_000},
		{4, 80_000},
		{5, 160_000},
		{6, 300_000}, // 320s → capped to 300s
		{7, 300_000}, // capped
		{99, 300_000}, // capped
	}
	for _, tt := range cases {
		// Sample 50 times to flush out jitter outliers.
		for s := 0; s < 50; s++ {
			got := computeCooldownBackoff(tt.attempts)
			lo := time.Duration(float64(tt.idealMillis*1_000_000) * 0.9)
			hi := time.Duration(float64(tt.idealMillis*1_000_000) * 1.1)
			if got < lo || got > hi {
				t.Errorf("attempts=%d: got=%s, want within ±10%% of %dms", tt.attempts, got, tt.idealMillis)
				break
			}
		}
	}
}

// TestComputeCooldownBackoff_NegativeAttempts treats negative as 0.
func TestComputeCooldownBackoff_NegativeAttempts(t *testing.T) {
	got := computeCooldownBackoff(-5)
	lo := time.Duration(float64(5*time.Second) * 0.9)
	hi := time.Duration(float64(5*time.Second) * 1.1)
	if got < lo || got > hi {
		t.Errorf("negative attempts: got=%s, want within ±10%% of 5s", got)
	}
}

// TestComputeCooldownBackoff_NeverNegative defends against jitter making
// the duration negative.
func TestComputeCooldownBackoff_NeverNegative(t *testing.T) {
	for s := 0; s < 1000; s++ {
		if got := computeCooldownBackoff(0); got < 0 {
			t.Fatalf("computed negative duration: %s", got)
		}
	}
}

// TestRecordRequestOutcome_FirstCooldownTripIsBaseTier verifies that the
// first time a provider's circuit trips, the cooldown is ~5s (base), not
// the legacy 5min.
func TestRecordRequestOutcome_FirstCooldownTripIsBaseTier(t *testing.T) {
	m := NewProviderHealthMonitor(nil, time.Minute)

	// 10 consecutive failures triggers the breaker.
	for i := 0; i < 10; i++ {
		m.RecordRequestOutcome("openai", 100*time.Millisecond, false, http.StatusInternalServerError, nil)
	}

	m.mu.RLock()
	state := m.providers["openai"]
	m.mu.RUnlock()

	if state == nil || !state.CoolingDown {
		t.Fatal("expected provider in cooldown after 10 failures")
	}
	if state.CooldownAttempts != 1 {
		t.Errorf("CooldownAttempts=%d, want 1 after first trip", state.CooldownAttempts)
	}
	remaining := time.Until(state.CooldownUntil)
	if remaining > 6*time.Second || remaining < 4*time.Second {
		t.Errorf("first-trip cooldown remaining=%s, want ~5s ±10%%", remaining)
	}
}

// TestRecordRequestOutcome_SuccessAfterRecoveryResetsBackoff verifies that
// a successful request after the provider has come out of cooldown resets
// CooldownAttempts to 0, so the NEXT outage starts at base again.
func TestRecordRequestOutcome_SuccessAfterRecoveryResetsBackoff(t *testing.T) {
	m := NewProviderHealthMonitor(nil, time.Minute)

	// Trip the breaker.
	for i := 0; i < 10; i++ {
		m.RecordRequestOutcome("gemini", 100*time.Millisecond, false, http.StatusBadGateway, nil)
	}
	// Manually expire cooldown (avoid sleeping in test).
	m.mu.Lock()
	state := m.providers["gemini"]
	state.CooldownUntil = time.Now().Add(-time.Second)
	m.activeProbeCycleLocked()
	m.mu.Unlock()

	// One success → status flips to healthy → CooldownAttempts resets.
	m.RecordRequestOutcome("gemini", 50*time.Millisecond, true, 200, nil)

	m.mu.RLock()
	defer m.mu.RUnlock()
	if state.CooldownAttempts != 0 {
		t.Errorf("CooldownAttempts=%d after recovery success, want 0", state.CooldownAttempts)
	}
	if state.Status != "healthy" {
		t.Errorf("Status=%q after recovery, want healthy", state.Status)
	}
}

// TestRecordRequestOutcome_SecondTripUsesLargerBackoff verifies that two
// consecutive trip cycles produce roughly doubled backoffs (5s → 10s),
// proving the exponential growth is wired through the live state machine.
func TestRecordRequestOutcome_SecondTripUsesLargerBackoff(t *testing.T) {
	m := NewProviderHealthMonitor(nil, time.Minute)

	// First trip.
	for i := 0; i < 10; i++ {
		m.RecordRequestOutcome("claude", 100*time.Millisecond, false, http.StatusInternalServerError, nil)
	}
	m.mu.RLock()
	first := time.Until(m.providers["claude"].CooldownUntil)
	m.mu.RUnlock()

	// Force expiry; do NOT record a success → CooldownAttempts stays at 1.
	m.mu.Lock()
	m.providers["claude"].CooldownUntil = time.Now().Add(-time.Second)
	m.providers["claude"].CoolingDown = false
	m.providers["claude"].ConsecutiveFail = 0
	m.mu.Unlock()

	// Trip again.
	for i := 0; i < 10; i++ {
		m.RecordRequestOutcome("claude", 100*time.Millisecond, false, http.StatusInternalServerError, nil)
	}
	m.mu.RLock()
	second := time.Until(m.providers["claude"].CooldownUntil)
	attempts := m.providers["claude"].CooldownAttempts
	m.mu.RUnlock()

	if attempts != 2 {
		t.Errorf("CooldownAttempts=%d after second trip, want 2", attempts)
	}
	// Second trip is attempts=1 → ideal 10s; first was attempts=0 → ideal 5s.
	// With ±10% jitter, second should reliably be > first when they differ
	// by 2× — leave some slack but enforce growth.
	if second <= first {
		t.Errorf("second cooldown (%s) should be > first (%s)", second, first)
	}
}
