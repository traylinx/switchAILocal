// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package cascade

import (
	"testing"
)

func TestNewQualitySignalDetector(t *testing.T) {
	detector := NewQualitySignalDetector()

	if detector == nil {
		t.Fatal("Expected non-nil detector")
	}
	if len(detector.abruptEndingPatterns) == 0 {
		t.Error("Expected abrupt ending patterns to be initialized")
	}
	if len(detector.refusalPatterns) == 0 {
		t.Error("Expected refusal patterns to be initialized")
	}
}

func TestQualitySignalDetector_DetectRefusal(t *testing.T) {
	detector := NewQualitySignalDetector()

	tests := []struct {
		name     string
		response string
		wantType SignalType
	}{
		{
			name:     "cannot refusal",
			response: "I cannot help with that request.",
			wantType: SignalRefusal,
		},
		{
			name:     "sorry refusal",
			response: "Sorry, I can't assist with that.",
			wantType: SignalRefusal,
		},
		{
			name:     "as an AI refusal",
			response: "As an AI, I cannot provide that information.",
			wantType: SignalRefusal,
		},
		{
			name:     "normal response",
			response: "Here is the information you requested about weather patterns.",
			wantType: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			signals := detector.DetectSignals(tt.response)

			if tt.wantType == "" {
				// Should not detect refusal
				for _, s := range signals {
					if s.Type == SignalRefusal {
						t.Errorf("Unexpected refusal signal detected")
					}
				}
			} else {
				// Should detect refusal
				found := false
				for _, s := range signals {
					if s.Type == tt.wantType {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected signal type %s not found", tt.wantType)
				}
			}
		})
	}
}

func TestQualitySignalDetector_DetectAbruptEnding(t *testing.T) {
	detector := NewQualitySignalDetector()

	tests := []struct {
		name     string
		response string
		wantType SignalType
	}{
		{
			name:     "ends with ellipsis",
			response: "The process involves several steps including...",
			wantType: SignalAbruptEnding,
		},
		{
			name:     "ends with conjunction",
			response: "You should consider this option and",
			wantType: SignalAbruptEnding,
		},
		{
			name:     "ends with preposition",
			response: "This is important for",
			wantType: SignalAbruptEnding,
		},
		{
			name:     "proper ending",
			response: "This completes the explanation of the process.",
			wantType: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			signals := detector.DetectSignals(tt.response)

			if tt.wantType == "" {
				for _, s := range signals {
					if s.Type == SignalAbruptEnding {
						t.Errorf("Unexpected abrupt ending signal detected")
					}
				}
			} else {
				found := false
				for _, s := range signals {
					if s.Type == tt.wantType {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected signal type %s not found", tt.wantType)
				}
			}
		})
	}
}

func TestQualitySignalDetector_DetectIncompleteCode(t *testing.T) {
	detector := NewQualitySignalDetector()

	tests := []struct {
		name     string
		response string
		wantType SignalType
	}{
		{
			name:     "unclosed code block",
			response: "Here is the code:\n```python\ndef hello():\n    print('hello')",
			wantType: SignalIncompleteCode,
		},
		{
			name:     "closed code block",
			response: "Here is the code:\n```python\ndef hello():\n    print('hello')\n```",
			wantType: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			signals := detector.DetectSignals(tt.response)

			if tt.wantType == "" {
				for _, s := range signals {
					if s.Type == SignalIncompleteCode {
						t.Errorf("Unexpected incomplete code signal detected")
					}
				}
			} else {
				found := false
				for _, s := range signals {
					if s.Type == tt.wantType {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected signal type %s not found", tt.wantType)
				}
			}
		})
	}
}

func TestQualitySignalDetector_DetectTruncation(t *testing.T) {
	detector := NewQualitySignalDetector()

	tests := []struct {
		name     string
		response string
		wantType SignalType
	}{
		{
			name:     "truncated marker",
			response: "Here is the response [truncated]",
			wantType: SignalTruncated,
		},
		{
			name:     "max length reached",
			response: "The output was cut because maximum length reached.",
			wantType: SignalTruncated,
		},
		{
			name:     "normal response",
			response: "This is a complete response without any truncation.",
			wantType: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			signals := detector.DetectSignals(tt.response)

			if tt.wantType == "" {
				for _, s := range signals {
					if s.Type == SignalTruncated {
						t.Errorf("Unexpected truncation signal detected")
					}
				}
			} else {
				found := false
				for _, s := range signals {
					if s.Type == tt.wantType {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected signal type %s not found", tt.wantType)
				}
			}
		})
	}
}

func TestQualitySignalDetector_ShortResponse(t *testing.T) {
	detector := NewQualitySignalDetector()

	signals := detector.DetectSignals("OK")

	found := false
	for _, s := range signals {
		if s.Type == SignalLowQuality {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected low quality signal for short response")
	}
}

func TestCalculateOverallQuality(t *testing.T) {
	tests := []struct {
		name    string
		signals []QualitySignal
		want    float64
	}{
		{
			name:    "no signals",
			signals: nil,
			want:    1.0,
		},
		{
			name: "single low severity",
			signals: []QualitySignal{
				{Type: SignalAbruptEnding, Severity: 0.3},
			},
			want: 0.7,
		},
		{
			name: "single high severity",
			signals: []QualitySignal{
				{Type: SignalRefusal, Severity: 0.9},
			},
			want: 0.1,
		},
		{
			name: "multiple signals capped",
			signals: []QualitySignal{
				{Type: SignalAbruptEnding, Severity: 0.7},
				{Type: SignalIncompleteCode, Severity: 0.8},
			},
			want: 0.0, // Capped at 1.0 penalty
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateOverallQuality(tt.signals)
			if got < tt.want-0.01 || got > tt.want+0.01 {
				t.Errorf("CalculateOverallQuality() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHasCriticalSignals(t *testing.T) {
	tests := []struct {
		name    string
		signals []QualitySignal
		want    bool
	}{
		{
			name:    "no signals",
			signals: nil,
			want:    false,
		},
		{
			name: "low severity",
			signals: []QualitySignal{
				{Type: SignalAbruptEnding, Severity: 0.5},
			},
			want: false,
		},
		{
			name: "high severity",
			signals: []QualitySignal{
				{Type: SignalAbruptEnding, Severity: 0.85},
			},
			want: true,
		},
		{
			name: "critical type refusal",
			signals: []QualitySignal{
				{Type: SignalRefusal, Severity: 0.5},
			},
			want: true,
		},
		{
			name: "critical type truncated",
			signals: []QualitySignal{
				{Type: SignalTruncated, Severity: 0.5},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HasCriticalSignals(tt.signals)
			if got != tt.want {
				t.Errorf("HasCriticalSignals() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewManager(t *testing.T) {
	tests := []struct {
		name          string
		cfg           Config
		wantThreshold float64
		wantMax       int
	}{
		{
			name:          "default values",
			cfg:           Config{Enabled: true},
			wantThreshold: 0.70,
			wantMax:       2,
		},
		{
			name: "custom values",
			cfg: Config{
				Enabled:          true,
				QualityThreshold: 0.80,
				MaxCascades:      3,
			},
			wantThreshold: 0.80,
			wantMax:       3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewManager(tt.cfg)

			if m.qualityThreshold != tt.wantThreshold {
				t.Errorf("qualityThreshold = %v, want %v", m.qualityThreshold, tt.wantThreshold)
			}
			if m.maxCascades != tt.wantMax {
				t.Errorf("maxCascades = %v, want %v", m.maxCascades, tt.wantMax)
			}
		})
	}
}

func TestManager_IsEnabled(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
		want    bool
	}{
		{
			name:    "enabled",
			enabled: true,
			want:    true,
		},
		{
			name:    "disabled",
			enabled: false,
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewManager(Config{Enabled: tt.enabled})
			if m.IsEnabled() != tt.want {
				t.Errorf("IsEnabled() = %v, want %v", m.IsEnabled(), tt.want)
			}
		})
	}
}

func TestManager_EvaluateResponse_AcceptableQuality(t *testing.T) {
	m := NewManager(Config{
		Enabled:          true,
		QualityThreshold: 0.70,
	})

	// Good quality response
	response := "This is a complete and well-formed response that provides all the necessary information about the topic you asked about."
	decision := m.EvaluateResponse(response, TierFast)

	if decision.ShouldCascade {
		t.Error("Expected no cascade for acceptable quality response")
	}
	if decision.QualityScore < 0.70 {
		t.Errorf("QualityScore = %v, expected >= 0.70", decision.QualityScore)
	}
}

func TestManager_EvaluateResponse_PoorQuality(t *testing.T) {
	m := NewManager(Config{
		Enabled:          true,
		QualityThreshold: 0.70,
	})

	// Poor quality response (refusal)
	response := "I cannot help with that request."
	decision := m.EvaluateResponse(response, TierFast)

	if !decision.ShouldCascade {
		t.Error("Expected cascade for poor quality response")
	}
	if decision.NextTier != TierStandard {
		t.Errorf("NextTier = %v, want %v", decision.NextTier, TierStandard)
	}
}

func TestManager_EvaluateResponse_TierEscalation(t *testing.T) {
	m := NewManager(Config{
		Enabled:          true,
		QualityThreshold: 0.70,
	})

	// Test tier escalation path
	tests := []struct {
		currentTier Tier
		wantNext    Tier
	}{
		{TierFast, TierStandard},
		{TierStandard, TierReasoning},
		{TierReasoning, ""}, // Already at highest
	}

	for _, tt := range tests {
		t.Run(string(tt.currentTier), func(t *testing.T) {
			// Use a refusal to trigger cascade
			response := "I cannot help with that."
			decision := m.EvaluateResponse(response, tt.currentTier)

			if tt.wantNext == "" {
				if decision.ShouldCascade {
					t.Error("Should not cascade from highest tier")
				}
			} else {
				if !decision.ShouldCascade {
					t.Error("Expected cascade")
				}
				if decision.NextTier != tt.wantNext {
					t.Errorf("NextTier = %v, want %v", decision.NextTier, tt.wantNext)
				}
			}
		})
	}
}

func TestManager_GetMetrics(t *testing.T) {
	m := NewManager(Config{
		Enabled:          true,
		QualityThreshold: 0.70,
	})

	// Generate some metrics
	m.EvaluateResponse("Good response with enough content to pass quality checks.", TierFast)
	m.EvaluateResponse("I cannot help.", TierFast)

	metrics := m.GetMetrics()

	if metrics["total_requests"].(int64) != 2 {
		t.Errorf("total_requests = %v, want 2", metrics["total_requests"])
	}
	if metrics["quality_threshold"].(float64) != 0.70 {
		t.Errorf("quality_threshold = %v, want 0.70", metrics["quality_threshold"])
	}
}

func TestTierToCapability(t *testing.T) {
	tests := []struct {
		tier Tier
		want string
	}{
		{TierFast, "fast"},
		{TierStandard, "chat"},
		{TierReasoning, "reasoning"},
		{Tier("unknown"), "fast"},
	}

	for _, tt := range tests {
		t.Run(string(tt.tier), func(t *testing.T) {
			got := TierToCapability(tt.tier)
			if got != tt.want {
				t.Errorf("TierToCapability(%v) = %v, want %v", tt.tier, got, tt.want)
			}
		})
	}
}

func TestCapabilityToTier(t *testing.T) {
	tests := []struct {
		capability string
		want       Tier
	}{
		{"fast", TierFast},
		{"chat", TierStandard},
		{"coding", TierStandard},
		{"reasoning", TierReasoning},
		{"research", TierReasoning},
		{"unknown", TierFast},
	}

	for _, tt := range tests {
		t.Run(tt.capability, func(t *testing.T) {
			got := CapabilityToTier(tt.capability)
			if got != tt.want {
				t.Errorf("CapabilityToTier(%v) = %v, want %v", tt.capability, got, tt.want)
			}
		})
	}
}

func TestCascadeTracker(t *testing.T) {
	tracker := NewCascadeTracker(TierFast, 3)

	if tracker.GetCurrentTier() != TierFast {
		t.Errorf("Initial tier = %v, want %v", tracker.GetCurrentTier(), TierFast)
	}

	// Record first attempt (cascade needed)
	tracker.RecordAttempt(&CascadeDecision{
		ShouldCascade: true,
		CurrentTier:   TierFast,
		NextTier:      TierStandard,
	})

	if tracker.GetCurrentTier() != TierStandard {
		t.Errorf("After cascade tier = %v, want %v", tracker.GetCurrentTier(), TierStandard)
	}
	if !tracker.CanContinue() {
		t.Error("Should be able to continue after 1 attempt")
	}

	// Record second attempt (cascade needed)
	tracker.RecordAttempt(&CascadeDecision{
		ShouldCascade: true,
		CurrentTier:   TierStandard,
		NextTier:      TierReasoning,
	})

	if tracker.GetCurrentTier() != TierReasoning {
		t.Errorf("After second cascade tier = %v, want %v", tracker.GetCurrentTier(), TierReasoning)
	}

	// Record third attempt (no cascade)
	tracker.RecordAttempt(&CascadeDecision{
		ShouldCascade: false,
		CurrentTier:   TierReasoning,
	})

	if tracker.CanContinue() {
		t.Error("Should not be able to continue after 3 attempts")
	}

	// Get result
	result := tracker.GetResult(true)
	if result.OriginalTier != TierFast {
		t.Errorf("OriginalTier = %v, want %v", result.OriginalTier, TierFast)
	}
	if result.FinalTier != TierReasoning {
		t.Errorf("FinalTier = %v, want %v", result.FinalTier, TierReasoning)
	}
	if result.CascadeCount != 2 {
		t.Errorf("CascadeCount = %v, want 2", result.CascadeCount)
	}
	if !result.Success {
		t.Error("Expected success = true")
	}
}
