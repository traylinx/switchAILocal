// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package config

import (
	"testing"
	"time"
)

func TestProviderTimeouts_Resolve(t *testing.T) {
	cfg := ProviderTimeoutsConfig{
		Default: 30 * time.Second,
		PerProvider: map[string]time.Duration{
			"openai":    20 * time.Second,
			"anthropic": 60 * time.Second,
			"ollama":    180 * time.Second,
		},
	}

	tests := []struct {
		provider string
		want     time.Duration
	}{
		{"openai", 20 * time.Second},
		{"anthropic", 60 * time.Second},
		{"ollama", 180 * time.Second},
		{"gemini", 30 * time.Second},    // unknown → default
		{"", 30 * time.Second},          // empty → default
		{"unknown", 30 * time.Second},   // unknown → default
	}

	for _, tt := range tests {
		if got := cfg.Resolve(tt.provider); got != tt.want {
			t.Errorf("Resolve(%q) = %v, want %v", tt.provider, got, tt.want)
		}
	}
}

func TestProviderTimeouts_Resolve_ZeroEntryFallsThrough(t *testing.T) {
	// A zero-value entry in PerProvider must fall through to Default, not return 0.
	cfg := ProviderTimeoutsConfig{
		Default: 30 * time.Second,
		PerProvider: map[string]time.Duration{
			"openai": 0, // explicitly zero
		},
	}
	if got := cfg.Resolve("openai"); got != 30*time.Second {
		t.Errorf("zero entry should fall through to default; got %v", got)
	}
}

func TestProviderTimeouts_Resolve_NoDefault(t *testing.T) {
	// If neither per-provider entry nor default exist, Resolve returns 0 and the
	// caller must apply its own hard default.
	cfg := ProviderTimeoutsConfig{}
	if got := cfg.Resolve("anything"); got != 0 {
		t.Errorf("empty config should return 0; got %v", got)
	}
}

func TestPerformanceConfig_SanitizeDefaults(t *testing.T) {
	var c PerformanceConfig
	c.SanitizePerformance()

	if c.ProviderTimeouts.Default != 120*time.Second {
		t.Errorf("ProviderTimeouts.Default = %v, want 120s", c.ProviderTimeouts.Default)
	}
	if c.ProviderTimeouts.PerProvider == nil {
		t.Error("ProviderTimeouts.PerProvider should be non-nil after sanitize")
	}
	if c.Streaming.FirstByteTimeout != 15*time.Second {
		t.Errorf("Streaming.FirstByteTimeout = %v, want 15s", c.Streaming.FirstByteTimeout)
	}
	if c.Streaming.StallTimeout != 60*time.Second {
		t.Errorf("Streaming.StallTimeout = %v, want 60s", c.Streaming.StallTimeout)
	}
	// Pre-existing defaults still apply
	if c.CircuitBreaker.FailureThreshold != 5 {
		t.Errorf("CircuitBreaker.FailureThreshold = %d, want 5", c.CircuitBreaker.FailureThreshold)
	}
}

func TestPerformanceConfig_SanitizePreservesUserValues(t *testing.T) {
	c := PerformanceConfig{
		ProviderTimeouts: ProviderTimeoutsConfig{
			Default: 45 * time.Second,
			PerProvider: map[string]time.Duration{
				"openai": 25 * time.Second,
			},
		},
		Streaming: StreamHealthConfig{
			FirstByteTimeout: 5 * time.Second,
			StallTimeout:     90 * time.Second,
		},
	}
	c.SanitizePerformance()

	if c.ProviderTimeouts.Default != 45*time.Second {
		t.Errorf("user-set default overwritten: got %v", c.ProviderTimeouts.Default)
	}
	if c.ProviderTimeouts.PerProvider["openai"] != 25*time.Second {
		t.Errorf("user-set per-provider overwritten: got %v", c.ProviderTimeouts.PerProvider["openai"])
	}
	if c.Streaming.FirstByteTimeout != 5*time.Second {
		t.Errorf("user-set FirstByteTimeout overwritten: got %v", c.Streaming.FirstByteTimeout)
	}
	if c.Streaming.StallTimeout != 90*time.Second {
		t.Errorf("user-set StallTimeout overwritten: got %v", c.Streaming.StallTimeout)
	}
}
