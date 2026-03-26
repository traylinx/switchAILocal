package autoroute

import (
	"context"
	"testing"
	"time"
)

// TestE2EFailover_QuotaExhaustion simulates a realistic scenario where a preferred
// premium model has exhausted its quota, and the router must successfully failover
// to the next best candidate based on RQS (Routing Quality Score) and intent restrictions.
func TestE2EFailover_QuotaExhaustion(t *testing.T) {
	cfg := newTestConfig()
	// Add custom intent constraints
	cfg.IntentMatrix = map[string][]string{
		"reasoning": {"openai:o1-preview", "claude:claude-3-opus", "switchai:deepseek-reasoner"},
	}
	
	resolver := NewAutoResolver(cfg, t.TempDir())

	req := &RoutingRequest{
		Content:    "Solve this complex logical paradox",
		IntentHint: "reasoning",
		AvailableModels: []CandidateInput{
			// Preferred but exhausted quota (Health = 0.05, near zero)
			{
				Model:       "openai:o1-preview",
				Provider:    "openai",
				Available:   true,
				QuotaHealth: 0.05, // Almost exhausted
				Latency:     500 * time.Millisecond,
				SuccessRate: 0.99,
			},
			// Healthy fallback
			{
				Model:       "switchai:deepseek-reasoner",
				Provider:    "switchai",
				Available:   true,
				QuotaHealth: 0.90,
				Latency:     800 * time.Millisecond,
				SuccessRate: 0.95,
			},
			// Another healthy fallback but slightly slower
			{
				Model:       "claude:claude-3-opus",
				Provider:    "claude",
				Available:   true,
				QuotaHealth: 0.85,
				Latency:     1200 * time.Millisecond,
				SuccessRate: 0.90,
			},
			// Healthy but WRONG INTENT (should be filtered out entirely)
			{
				Model:       "ollama:llama3",
				Provider:    "ollama",
				Available:   true,
				QuotaHealth: 1.0,
				Latency:     300 * time.Millisecond,
				SuccessRate: 0.99,
			},
		},
	}

	decision, err := resolver.Resolve(context.Background(), req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if decision.SelectedModel == "openai:o1-preview" {
		t.Errorf("Router erroneously selected the quota-exhausted model instead of failing over.")
	}
	
	if decision.SelectedModel == "ollama:llama3" {
		t.Errorf("Router selected a model that violates the intent matrix restrictions.")
	}

	// Expected winner is deepseek-reasoner due to high quota + good intent match + better latency than opus
	if decision.SelectedModel != "switchai:deepseek-reasoner" {
		t.Errorf("Expected failover to switchai:deepseek-reasoner, got: %s", decision.SelectedModel)
	}

	// The exhausted model should still be in the fallback chain in case the primary fails, 
	// but relegated to the back.
	foundExhausted := false
	for _, fallback := range decision.FallbackChain {
		if fallback.Model == "openai:o1-preview" {
			foundExhausted = true
		}
	}
	if !foundExhausted {
		t.Errorf("Exhausted model should still be present in the fallback chain")
	}

	t.Logf("Successful E2E Fallover routing decision: Selected=%s, Fallbacks=%v", decision.SelectedModel, decision.FallbackChain)
}
