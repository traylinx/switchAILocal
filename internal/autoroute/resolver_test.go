package autoroute

import (
	"context"
	"testing"
	"time"
)

func TestResolver_DisabledConfig(t *testing.T) {
	cfg := Config{Enabled: false}
	resolver := NewAutoResolver(cfg)

	_, err := resolver.Resolve(context.Background(), &RoutingRequest{Content: "test"})
	if err != ErrAutoRoutingDisabled {
		t.Errorf("Expected ErrAutoRoutingDisabled, got %v", err)
	}
}

func TestResolver_NoAvailableProviders(t *testing.T) {
	cfg := newTestConfig()
	resolver := NewAutoResolver(cfg)

	req := &RoutingRequest{
		Content: "Hello",
		AvailableModels: []CandidateInput{
			{Model: "a:model", Provider: "a", Available: false, QuotaHealth: 0.0},
			{Model: "b:model", Provider: "b", Available: false, QuotaHealth: 0.0},
		},
	}

	_, err := resolver.Resolve(context.Background(), req)
	if err != ErrNoAvailableProviders {
		t.Errorf("Expected ErrNoAvailableProviders, got %v", err)
	}
}

func TestResolver_EmptyCandidates(t *testing.T) {
	cfg := newTestConfig()
	resolver := NewAutoResolver(cfg)

	req := &RoutingRequest{
		Content:         "Hello",
		AvailableModels: nil,
	}

	_, err := resolver.Resolve(context.Background(), req)
	if err != ErrNoAvailableProviders {
		t.Errorf("Expected ErrNoAvailableProviders, got %v", err)
	}
}

func TestResolver_HappyPath(t *testing.T) {
	cfg := newTestConfig()
	resolver := NewAutoResolver(cfg)

	req := &RoutingRequest{
		Content: "Implement a distributed consensus algorithm in Rust",
		AvailableModels: []CandidateInput{
			{Model: "claudecli:claude-sonnet-4", Provider: "claudecli", Available: true, QuotaHealth: 0.8, Latency: 1200 * time.Millisecond, SuccessRate: 0.92},
			{Model: "ollama:kimi-k2.5:cloud", Provider: "ollama", Available: true, QuotaHealth: 0.7, Latency: 1500 * time.Millisecond, SuccessRate: 0.88},
			{Model: "geminicli:gemini-3.1-pro-preview", Provider: "geminicli", Available: true, QuotaHealth: 0.75, Latency: 1000 * time.Millisecond, SuccessRate: 0.90},
		},
	}

	decision, err := resolver.Resolve(context.Background(), req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if decision.SelectedModel == "" {
		t.Fatal("Expected a selected model, got empty")
	}

	if len(decision.FallbackChain) == 0 {
		t.Error("Expected at least one fallback entry")
	}

	if decision.ResolutionLatency > 5*time.Millisecond {
		t.Errorf("Resolution exceeded 5ms budget: %v", decision.ResolutionLatency)
	}

	if decision.EstimatedComplexity == 0 {
		t.Error("Expected non-zero complexity estimate")
	}

	t.Logf("Decision: %s", decision)
}

func TestResolver_FallbackChainOrdering(t *testing.T) {
	cfg := newTestConfig()
	resolver := NewAutoResolver(cfg)

	req := &RoutingRequest{
		Content: "Simple hello",
		AvailableModels: []CandidateInput{
			{Model: "a:model", Provider: "a", Available: true, QuotaHealth: 1.0, Latency: 100 * time.Millisecond, SuccessRate: 0.9},
			{Model: "b:model", Provider: "b", Available: true, QuotaHealth: 0.8, Latency: 200 * time.Millisecond, SuccessRate: 0.8},
			{Model: "c:model", Provider: "c", Available: true, QuotaHealth: 0.5, Latency: 500 * time.Millisecond, SuccessRate: 0.7},
		},
	}

	decision, err := resolver.Resolve(context.Background(), req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Fallback chain should have exactly 2 entries (everything except the winner)
	if len(decision.FallbackChain) != 2 {
		t.Errorf("Expected 2 fallback entries, got %d", len(decision.FallbackChain))
	}
}

func TestResolver_IntentFilter(t *testing.T) {
	cfg := newTestConfig()
	cfg.IntentMatrix = map[string][]string{
		"coding": {"ollama:kimi-k2.5:cloud", "claudecli:claude-sonnet-4"},
	}
	resolver := NewAutoResolver(cfg)

	req := &RoutingRequest{
		Content:    "Write a Go function",
		IntentHint: "coding",
		AvailableModels: []CandidateInput{
			{Model: "claudecli:claude-sonnet-4", Provider: "claudecli", Available: true, QuotaHealth: 0.8, SuccessRate: 0.9},
			{Model: "ollama:kimi-k2.5:cloud", Provider: "ollama", Available: true, QuotaHealth: 0.7, SuccessRate: 0.85},
			{Model: "ollama:qwen3.5:cloud", Provider: "ollama", Available: true, QuotaHealth: 0.9, SuccessRate: 0.8},
		},
	}

	decision, err := resolver.Resolve(context.Background(), req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// qwen3.5 should NOT be the winner (it's not in the coding intent matrix)
	if decision.SelectedModel == "ollama:qwen3.5:cloud" {
		t.Error("qwen3.5 should not win when intent=coding filters it out")
	}
}

func TestParseAutoModelHint(t *testing.T) {
	tests := []struct {
		input    string
		wantBase string
		wantHint string
	}{
		{"auto", "auto", ""},
		{"", "auto", ""},
		{"auto:coding", "auto", "coding"},
		{"auto:fast", "auto", "fast"},
		{"gpt-4", "gpt-4", ""},
		{"claude-sonnet-4", "claude-sonnet-4", ""},
	}

	for _, tt := range tests {
		base, hint := ParseAutoModelHint(tt.input)
		if base != tt.wantBase || hint != tt.wantHint {
			t.Errorf("ParseAutoModelHint(%q) = (%q, %q), want (%q, %q)", tt.input, base, hint, tt.wantBase, tt.wantHint)
		}
	}
}

func BenchmarkResolve(b *testing.B) {
	cfg := newTestConfig()
	resolver := NewAutoResolver(cfg)

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

	req := &RoutingRequest{
		Content:         "Test content for benchmarking the routing resolution.",
		AvailableModels: candidates,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resolver.Resolve(context.Background(), req)
	}
}
