package autoroute

import (
	"testing"
	"time"
)

// newTestConfig returns a Config with default scoring weights for testing.
func newTestConfig() Config {
	return Config{
		Enabled:       true,
		MaxResolution: 5 * time.Millisecond,
		Weights: ScoringWeights{
			Availability: 0.35,
			Quota:        0.25,
			Latency:      0.20,
			SuccessRate:  0.20,
		},
		Providers: map[string]ProviderTierConfig{
			"claudecli": {Tier: TierPremium},
			"geminicli": {Tier: TierStandard},
			"ollama": {Tier: TierFree, ModelTiers: map[string]string{
				"kimi-k2.5:cloud":    TierStandard,
				"gpt-oss:120b-cloud": TierStandard,
			}},
			"switchai":     {Tier: TierFree},
			"ollama-local": {Tier: TierLocal},
		},
		Preferences: []ModelPreference{
			{Model: "claudecli:claude-sonnet-4", Preference: 0.9},
			{Model: "geminicli:gemini-3.1-pro-preview", Preference: 0.8},
			{Model: "ollama:kimi-k2.5:cloud", Preference: 0.7},
			{Model: "ollama:qwen3.5:cloud", Preference: 0.5},
		},
		Conservation: ConservationConfig{
			Enabled:               true,
			SimpleThreshold:       300, // 0.30 after normalization
			PremiumConservationAt: 0.30,
		},
	}
}

// --- Tier Tests ---

func TestTierBoost(t *testing.T) {
	tests := []struct {
		tier string
		want float64
	}{
		{TierPremium, 0.30},
		{TierStandard, 0.15},
		{TierFree, 0.00},
		{TierLocal, 0.05},
		{TierReserve, -0.10},
		{"unknown", 0.00},
	}
	for _, tt := range tests {
		got := TierBoost(tt.tier)
		if got != tt.want {
			t.Errorf("TierBoost(%q) = %f, want %f", tt.tier, got, tt.want)
		}
	}
}

func TestGetEffectiveTier_ModelOverride(t *testing.T) {
	cfg := newTestConfig()

	// Provider-level tier
	if tier := GetEffectiveTier("claudecli:claude-sonnet-4", "claudecli", cfg); tier != TierPremium {
		t.Errorf("Expected premium for claudecli, got %s", tier)
	}

	// Model-level override: free provider, but model promoted to standard
	if tier := GetEffectiveTier("ollama:kimi-k2.5:cloud", "ollama", cfg); tier != TierStandard {
		t.Errorf("Expected standard for kimi-k2.5 (model override), got %s", tier)
	}

	// No override: plain free
	if tier := GetEffectiveTier("ollama:qwen3.5:cloud", "ollama", cfg); tier != TierFree {
		t.Errorf("Expected free for qwen3.5, got %s", tier)
	}
}

func TestAutoDetectTier(t *testing.T) {
	tests := []struct {
		provider string
		want     string
	}{
		{"claudecli", TierStandard},
		{"geminicli", TierStandard},
		{"ollama-local", TierLocal},
		{"lmstudio", TierLocal},
		{"switchai", TierFree},
		{"gemini", TierStandard},
		{"randomcloud", TierFree},
	}
	for _, tt := range tests {
		got := AutoDetectTier(tt.provider)
		if got != tt.want {
			t.Errorf("AutoDetectTier(%q) = %q, want %q", tt.provider, got, tt.want)
		}
	}
}

func TestPreferenceBoost(t *testing.T) {
	prefs := []ModelPreference{
		{Model: "claudecli:claude-sonnet-4", Preference: 0.9},
		{Model: "ollama:qwen3.5:cloud", Preference: 0.5},
	}

	// Known model: 0.9 * 0.20 = 0.18
	got := PreferenceBoost("claudecli:claude-sonnet-4", prefs)
	if abs(got-0.18) > 0.001 {
		t.Errorf("Expected ~0.18, got %f", got)
	}

	// Unknown model: 0.0
	got = PreferenceBoost("unknown:model", prefs)
	if abs(got-0.0) > 0.001 {
		t.Errorf("Expected ~0.0, got %f", got)
	}
}

func TestConservationMultiplier(t *testing.T) {
	cfg := ConservationConfig{Enabled: true, SimpleThreshold: 300}

	// Simple task + premium = penalize
	got := ConservationMultiplier(TierPremium, 0.05, cfg)
	if got != 0.3 {
		t.Errorf("Simple+Premium: expected 0.3, got %f", got)
	}

	// Simple task + free = boost
	got = ConservationMultiplier(TierFree, 0.05, cfg)
	if got != 1.5 {
		t.Errorf("Simple+Free: expected 1.5, got %f", got)
	}

	// Complex task + premium = boost
	got = ConservationMultiplier(TierPremium, 0.95, cfg)
	if got != 1.2 {
		t.Errorf("Complex+Premium: expected 1.2, got %f", got)
	}

	// Medium task + standard = neutral
	got = ConservationMultiplier(TierStandard, 0.50, cfg)
	if got != 1.0 {
		t.Errorf("Medium+Standard: expected 1.0, got %f", got)
	}

	// Conservation disabled
	got = ConservationMultiplier(TierPremium, 0.05, ConservationConfig{Enabled: false})
	if got != 1.0 {
		t.Errorf("Disabled: expected 1.0, got %f", got)
	}
}

// --- Scorer Tests: Spec Examples A-D ---

func TestScorerExampleA_ComplexCodingTask(t *testing.T) {
	// Example A from 08_PROVIDER_PRIORITY_INTELLIGENCE.md
	// Complex coding task (complexity = 0.95)
	// Expected winner: claudecli:claude-sonnet-4

	cfg := newTestConfig()
	scorer := NewProviderScorer(cfg)

	candidates := []CandidateInput{
		{Model: "claudecli:claude-sonnet-4", Provider: "claudecli", Available: true, QuotaHealth: 0.8, Latency: 1200 * time.Millisecond, SuccessRate: 0.92},
		{Model: "ollama:kimi-k2.5:cloud", Provider: "ollama", Available: true, QuotaHealth: 0.7, Latency: 1500 * time.Millisecond, SuccessRate: 0.88},
		{Model: "geminicli:gemini-3.1-pro-preview", Provider: "geminicli", Available: true, QuotaHealth: 0.75, Latency: 1000 * time.Millisecond, SuccessRate: 0.90},
		{Model: "ollama:qwen3.5:cloud", Provider: "ollama", Available: true, QuotaHealth: 0.6, Latency: 800 * time.Millisecond, SuccessRate: 0.85},
	}

	scored := scorer.ScoreAll(candidates, 0.95)

	if len(scored) == 0 {
		t.Fatal("Expected scored candidates, got 0")
	}

	// Claude should win for complex coding tasks (premium tier + high preference + conservation boost)
	if scored[0].Model != "claudecli:claude-sonnet-4" {
		t.Errorf("Expected claudecli:claude-sonnet-4 as winner, got %s (score: %.4f)", scored[0].Model, scored[0].FinalScore)
		for _, s := range scored {
			t.Logf("  %s: final=%.4f base=%.4f tier=%.4f pref=%.4f conserv=%.4f", s.Model, s.FinalScore, s.BaseScore, s.TierBoost, s.PreferenceBoost, s.ConservationMult)
		}
	}

	// Conservation multiplier should be 1.2 for complex + premium
	if scored[0].ConservationMult != 1.2 {
		t.Errorf("Expected conservation multiplier 1.2 for complex+premium, got %f", scored[0].ConservationMult)
	}
}

func TestScorerExampleB_SimpleChatQuestion(t *testing.T) {
	// Example B from 08_PROVIDER_PRIORITY_INTELLIGENCE.md
	// Simple chat question (complexity = 0.05)
	// Expected: Claude MUST NOT win (conservation penalizes premium for simple tasks)

	cfg := newTestConfig()
	scorer := NewProviderScorer(cfg)

	candidates := []CandidateInput{
		{Model: "claudecli:claude-sonnet-4", Provider: "claudecli", Available: true, QuotaHealth: 0.8, Latency: 1200 * time.Millisecond, SuccessRate: 0.92},
		{Model: "ollama:qwen3.5:cloud", Provider: "ollama", Available: true, QuotaHealth: 0.7, Latency: 500 * time.Millisecond, SuccessRate: 0.85},
		{Model: "geminicli:gemini-3.1-pro-preview", Provider: "geminicli", Available: true, QuotaHealth: 0.8, Latency: 800 * time.Millisecond, SuccessRate: 0.90},
	}

	scored := scorer.ScoreAll(candidates, 0.05)

	// Claude should NOT win for simple tasks
	if scored[0].Model == "claudecli:claude-sonnet-4" {
		t.Errorf("Claude should NOT win for simple tasks! Conservation should penalize premium. Scores:")
		for _, s := range scored {
			t.Logf("  %s: final=%.4f conserv=%.4f", s.Model, s.FinalScore, s.ConservationMult)
		}
	}

	// Claude's conservation multiplier should be 0.3 (heavy penalty)
	for _, s := range scored {
		if s.Model == "claudecli:claude-sonnet-4" && s.ConservationMult != 0.3 {
			t.Errorf("Expected conservation 0.3 for simple+premium, got %f", s.ConservationMult)
		}
	}
}

func TestScorerExampleC_AllProvidersDown(t *testing.T) {
	// Edge case: all providers unavailable
	cfg := newTestConfig()
	scorer := NewProviderScorer(cfg)

	candidates := []CandidateInput{
		{Model: "claudecli:claude-sonnet-4", Provider: "claudecli", Available: false, QuotaHealth: 0.0},
		{Model: "ollama:qwen3.5:cloud", Provider: "ollama", Available: false, QuotaHealth: 0.0},
	}

	scored := scorer.ScoreAll(candidates, 0.5)

	// All candidates should have Available=false
	for _, s := range scored {
		if s.Available {
			t.Errorf("Expected all candidates unavailable, but %s is available", s.Model)
		}
	}
}

func TestScorerColdStart(t *testing.T) {
	// Cold start: SuccessRate=-1 means unknown → use 0.7 optimistic default
	cfg := newTestConfig()
	scorer := NewProviderScorer(cfg)

	candidates := []CandidateInput{
		{Model: "newprovider:new-model", Provider: "newprovider", Available: true, QuotaHealth: 1.0, SuccessRate: -1}, // Cold start
	}

	scored := scorer.ScoreAll(candidates, 0.5)
	if len(scored) == 0 {
		t.Fatal("Expected 1 scored candidate")
	}

	// Success rate component should use 0.7 default, not -1
	// BaseScore should be positive (0.35*1.0 + 0.25*1.0 + 0.20*0.5 + 0.20*0.7) = 0.35 + 0.25 + 0.10 + 0.14 = 0.84
	expectedBase := 0.35*1.0 + 0.25*1.0 + 0.20*0.5 + 0.20*0.7
	if abs(scored[0].BaseScore-expectedBase) > 0.01 {
		t.Errorf("Cold start base score: expected ~%.4f, got %.4f", expectedBase, scored[0].BaseScore)
	}
}

func TestScorerTieBreaking(t *testing.T) {
	// Two candidates with identical scores → lower latency wins
	cfg := Config{
		Enabled:      true,
		Weights:      ScoringWeights{Availability: 0.25, Quota: 0.25, Latency: 0.25, SuccessRate: 0.25},
		Conservation: ConservationConfig{Enabled: false},
	}
	scorer := NewProviderScorer(cfg)

	candidates := []CandidateInput{
		{Model: "b:model", Provider: "b", Available: true, QuotaHealth: 1.0, Latency: 2000 * time.Millisecond, SuccessRate: 0.9},
		{Model: "a:model", Provider: "a", Available: true, QuotaHealth: 1.0, Latency: 1000 * time.Millisecond, SuccessRate: 0.9},
	}

	scored := scorer.ScoreAll(candidates, 0.5)

	// "a:model" should win due to lower latency
	if scored[0].Model != "a:model" {
		t.Errorf("Expected a:model to win tie-break (lower latency), got %s", scored[0].Model)
	}
}

func TestScorerTieBreaking_Alphabetical(t *testing.T) {
	// Completely identical scores + latency → alphabetical wins
	cfg := Config{
		Enabled:      true,
		Weights:      ScoringWeights{Availability: 0.25, Quota: 0.25, Latency: 0.25, SuccessRate: 0.25},
		Conservation: ConservationConfig{Enabled: false},
	}
	scorer := NewProviderScorer(cfg)

	candidates := []CandidateInput{
		{Model: "z:model", Provider: "z", Available: true, QuotaHealth: 1.0, Latency: 1000 * time.Millisecond, SuccessRate: 0.9},
		{Model: "a:model", Provider: "a", Available: true, QuotaHealth: 1.0, Latency: 1000 * time.Millisecond, SuccessRate: 0.9},
	}

	scored := scorer.ScoreAll(candidates, 0.5)

	if scored[0].Model != "a:model" {
		t.Errorf("Expected a:model to win alphabetical tie-break, got %s", scored[0].Model)
	}
}

// --- Complexity Estimation Tests ---

func TestEstimateComplexity(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    float64
	}{
		{"empty", "", 0.1},
		{"trivial", "Hi", 0.1},
		{"short", "What is the capital of France?", 0.1},
		{"medium", string(make([]byte, 400)), 0.3},
		{"long", string(make([]byte, 1200)), 0.5},
		{"complex", string(make([]byte, 4000)), 0.7},
		{"expert", string(make([]byte, 20000)), 0.9},
	}
	for _, tt := range tests {
		got := EstimateComplexity(tt.content)
		if got != tt.want {
			t.Errorf("EstimateComplexity(%s): got %f, want %f", tt.name, got, tt.want)
		}
	}
}

// --- Latency Score Tests ---

func TestLatencyToScore(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want float64
	}{
		{0, 0.5},                        // Unknown → neutral
		{1 * time.Millisecond, 1.0},     // Almost instant
		{2500 * time.Millisecond, 0.5},  // Half max
		{5000 * time.Millisecond, 0.0},  // At max
		{10000 * time.Millisecond, 0.0}, // Beyond max
	}
	for _, tt := range tests {
		got := latencyToScore(tt.d)
		if abs(got-tt.want) > 0.01 {
			t.Errorf("latencyToScore(%v) = %f, want %f", tt.d, got, tt.want)
		}
	}
}

// --- Benchmark ---

func BenchmarkScoreAll_20Candidates(b *testing.B) {
	cfg := newTestConfig()
	scorer := NewProviderScorer(cfg)

	candidates := make([]CandidateInput, 20)
	for i := 0; i < 20; i++ {
		candidates[i] = CandidateInput{
			Model:       "provider:model-" + string(rune('a'+i)),
			Provider:    "provider",
			Available:   true,
			QuotaHealth: 0.8,
			Latency:     time.Duration(500+i*100) * time.Millisecond,
			SuccessRate: 0.9,
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		scorer.ScoreAll(candidates, 0.5)
	}
}

// abs returns the absolute value of a float64.
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
