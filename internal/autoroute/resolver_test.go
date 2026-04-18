package autoroute

import (
	"context"
	"testing"
	"time"
)

func TestResolver_DisabledConfig(t *testing.T) {
	cfg := Config{Enabled: false}
	resolver := NewAutoResolver(cfg, t.TempDir())

	_, err := resolver.Resolve(context.Background(), &RoutingRequest{Content: "test"})
	if err != ErrAutoRoutingDisabled {
		t.Errorf("Expected ErrAutoRoutingDisabled, got %v", err)
	}
}

func TestResolver_NoAvailableProviders(t *testing.T) {
	cfg := newTestConfig()
	resolver := NewAutoResolver(cfg, t.TempDir())

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
	resolver := NewAutoResolver(cfg, t.TempDir())

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
	resolver := NewAutoResolver(cfg, t.TempDir())

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
	resolver := NewAutoResolver(cfg, t.TempDir())

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
	resolver := NewAutoResolver(cfg, t.TempDir())

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

func TestResolver_DisabledProviders(t *testing.T) {
	cfg := newTestConfig()
	cfg.DisabledProviders = []string{"anthropic"}
	resolver := NewAutoResolver(cfg, t.TempDir())

	req := &RoutingRequest{
		Content: "Hello",
		AvailableModels: []CandidateInput{
			{Model: "anthropic:claude-3-5-sonnet", Provider: "anthropic", Available: true, QuotaHealth: 1.0, Latency: 50 * time.Millisecond, SuccessRate: 0.99},
			{Model: "gemini:gemini-2.5-pro", Provider: "gemini", Available: true, QuotaHealth: 0.8, Latency: 200 * time.Millisecond, SuccessRate: 0.9},
			{Model: "ollama:llama3:latest", Provider: "ollama", Available: true, QuotaHealth: 0.7, Latency: 500 * time.Millisecond, SuccessRate: 0.85},
		},
	}

	decision, err := resolver.Resolve(context.Background(), req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Anthropic scored highest (1.0 quota, 50ms latency, 0.99 success) but is blacklisted.
	// Winner must be gemini or ollama, never anthropic.
	if decision.SelectedModel == "anthropic:claude-3-5-sonnet" {
		t.Errorf("Disabled provider anthropic was selected: %s", decision.SelectedModel)
	}
	for _, fb := range decision.FallbackChain {
		if fb.Provider == "anthropic" {
			t.Errorf("Disabled provider anthropic appeared in fallback chain: %+v", fb)
		}
	}
}

func TestResolver_DisabledProviders_AllBlocked(t *testing.T) {
	cfg := newTestConfig()
	cfg.DisabledProviders = []string{"anthropic", "gemini"}
	resolver := NewAutoResolver(cfg, t.TempDir())

	req := &RoutingRequest{
		Content: "Hello",
		AvailableModels: []CandidateInput{
			{Model: "anthropic:claude-3-5-sonnet", Provider: "anthropic", Available: true, QuotaHealth: 1.0, SuccessRate: 0.99},
			{Model: "gemini:gemini-2.5-pro", Provider: "gemini", Available: true, QuotaHealth: 0.9, SuccessRate: 0.95},
		},
	}

	// Every candidate is blacklisted — must fail, never silently bypass.
	_, err := resolver.Resolve(context.Background(), req)
	if err != ErrNoAvailableProviders {
		t.Errorf("Expected ErrNoAvailableProviders when all providers disabled, got %v", err)
	}
}

func TestFilterDisabledProviders(t *testing.T) {
	candidates := []CandidateInput{
		{Model: "a:m", Provider: "a"},
		{Model: "b:m", Provider: "b"},
		{Model: "c:m", Provider: "c"},
	}

	// Empty disabled list → unchanged
	if got := filterDisabledProviders(candidates, nil); len(got) != 3 {
		t.Errorf("nil disabled list should return all 3 candidates, got %d", len(got))
	}

	// Block one
	got := filterDisabledProviders(candidates, []string{"b"})
	if len(got) != 2 {
		t.Fatalf("blocking 'b' should leave 2 candidates, got %d", len(got))
	}
	for _, c := range got {
		if c.Provider == "b" {
			t.Errorf("provider 'b' was not filtered out")
		}
	}

	// Block all
	if got := filterDisabledProviders(candidates, []string{"a", "b", "c"}); len(got) != 0 {
		t.Errorf("blocking all should leave 0 candidates, got %d", len(got))
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
	resolver := NewAutoResolver(cfg, b.TempDir())

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
		_, _ = resolver.Resolve(context.Background(), req)
	}
}
