package steering

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func BenchmarkSteeringEngine_RuleEvaluation(b *testing.B) {
	tmpDir, err := os.MkdirTemp("", "steering-perf-*")
	require.NoError(b, err)
	defer os.RemoveAll(tmpDir)

	// Create multiple rules to test performance
	rules := []string{
		`
name: "Coding Rule"
activation:
  condition: "Intent == 'coding'"
  priority: 100
preferences:
  primary_model: "claude-sonnet"
`,
		`
name: "Reasoning Rule"
activation:
  condition: "Intent == 'reasoning' && ContentLength > 500"
  priority: 90
preferences:
  primary_model: "gemini-pro"
`,
		`
name: "Chat Rule"
activation:
  condition: "Intent == 'chat' || Provider == 'ollama'"
  priority: 80
preferences:
  primary_model: "ollama-llama"
`,
		`
name: "Complex Rule"
activation:
  condition: "Intent == 'coding' && ContentLength > 1000 && Hour >= 9 && Hour <= 17"
  priority: 70
preferences:
  primary_model: "claude-opus"
`,
	}

	for i, rule := range rules {
		err = os.WriteFile(filepath.Join(tmpDir, "rule"+string(rune(i+48))+".yaml"), []byte(rule), 0644)
		require.NoError(b, err)
	}

	engine, err := NewSteeringEngine(tmpDir)
	require.NoError(b, err)
	err = engine.LoadRules()
	require.NoError(b, err)

	ctx := &RoutingContext{
		Intent:        "coding",
		Provider:      "claude",
		ContentLength: 1500,
		Hour:          14,
		Timestamp:     time.Now(),
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		matches, _ := engine.FindMatchingRules(ctx)
		engine.ApplySteering(ctx, nil, nil, matches)
	}
}

func TestSteeringEngine_PerformanceRequirement(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "steering-perf-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create a single rule for performance testing
	rule := `
name: "Performance Test Rule"
activation:
  condition: "Intent == 'coding' && ContentLength > 100"
  priority: 100
preferences:
  primary_model: "test-model"
  context_injection: "Test context"
`
	err = os.WriteFile(filepath.Join(tmpDir, "perf.yaml"), []byte(rule), 0644)
	require.NoError(t, err)

	engine, err := NewSteeringEngine(tmpDir)
	require.NoError(t, err)
	err = engine.LoadRules()
	require.NoError(t, err)

	ctx := &RoutingContext{
		Intent:        "coding",
		ContentLength: 500,
		Timestamp:     time.Now(),
	}

	// Test that rule evaluation takes less than 1ms
	start := time.Now()
	matches, _ := engine.FindMatchingRules(ctx)
	engine.ApplySteering(ctx, nil, nil, matches)
	elapsed := time.Since(start)

	// Rule evaluation should be under 1ms
	if elapsed > time.Millisecond {
		t.Errorf("Rule evaluation took %v, expected < 1ms", elapsed)
	}

	// Run multiple iterations to get average
	iterations := 1000
	start = time.Now()
	for i := 0; i < iterations; i++ {
		matches, _ := engine.FindMatchingRules(ctx)
		engine.ApplySteering(ctx, nil, nil, matches)
	}
	elapsed = time.Since(start)
	avgTime := elapsed / time.Duration(iterations)

	if avgTime > time.Millisecond {
		t.Errorf("Average rule evaluation took %v, expected < 1ms", avgTime)
	}

	t.Logf("Average rule evaluation time: %v", avgTime)
}
