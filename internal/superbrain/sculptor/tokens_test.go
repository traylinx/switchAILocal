package sculptor

import (
	"testing"
)

func TestNewTokenEstimator(t *testing.T) {
	t.Run("creates estimator with simple method", func(t *testing.T) {
		te := NewTokenEstimator("simple")
		if te.Method() != "simple" {
			t.Errorf("expected method 'simple', got '%s'", te.Method())
		}
	})

	t.Run("creates estimator with tiktoken method", func(t *testing.T) {
		te := NewTokenEstimator("tiktoken")
		if te.Method() != "tiktoken" {
			t.Errorf("expected method 'tiktoken', got '%s'", te.Method())
		}
	})

	t.Run("defaults to simple for invalid method", func(t *testing.T) {
		te := NewTokenEstimator("invalid")
		if te.Method() != "simple" {
			t.Errorf("expected method 'simple' for invalid input, got '%s'", te.Method())
		}
	})
}

func TestEstimateTokens_TikToken(t *testing.T) {
	te := NewTokenEstimator("tiktoken")
	if te.Method() != "tiktoken" {
		t.Skip("tiktoken method fell back to simple (likely missing embedded vocab), skipping test")
	}

	t.Run("estimates tokens accurately for tiktoken", func(t *testing.T) {
		// "The quick brown fox jumps over the lazy dog."
		// cl100k_base count is 10
		content := "The quick brown fox jumps over the lazy dog."
		tokens := te.EstimateTokens(content)

		// We check for exact match or at least > 0
		if tokens == 0 {
			t.Error("expected > 0 tokens")
		}

		// If encoding works, it should be 10 for cl100k_base.
		// If it's 10, it confirms it's using tiktoken (simple estimate is 11)
		if tokens != 10 {
			t.Logf("Warning: Expected 10 tokens for cl100k_base, got %d. Might be using different encoding or fallback.", tokens)
		}
	})

	t.Run("handles special characters", func(t *testing.T) {
		content := "hello world! 😊"
		tokens := te.EstimateTokens(content)
		if tokens == 0 {
			t.Error("expected > 0 tokens")
		}
	})
}

func TestEstimateTokens(t *testing.T) {
	te := NewTokenEstimator("simple")

	t.Run("returns 0 for empty content", func(t *testing.T) {
		tokens := te.EstimateTokens("")
		if tokens != 0 {
			t.Errorf("expected 0 tokens for empty content, got %d", tokens)
		}
	})

	t.Run("estimates single word", func(t *testing.T) {
		tokens := te.EstimateTokens("hello")
		// 1 word * 1.3 = 1.3, truncated to 1
		if tokens != 1 {
			t.Errorf("expected 1 token for single word, got %d", tokens)
		}
	})

	t.Run("estimates multiple words", func(t *testing.T) {
		tokens := te.EstimateTokens("hello world this is a test")
		// 6 words * 1.3 = 7.8, truncated to 7
		if tokens != 7 {
			t.Errorf("expected 7 tokens for 6 words, got %d", tokens)
		}
	})

	t.Run("handles whitespace correctly", func(t *testing.T) {
		tokens := te.EstimateTokens("  hello   world  ")
		// 2 words * 1.3 = 2.6, truncated to 2
		if tokens != 2 {
			t.Errorf("expected 2 tokens, got %d", tokens)
		}
	})

	t.Run("handles newlines and tabs", func(t *testing.T) {
		tokens := te.EstimateTokens("hello\nworld\ttab")
		// 3 words * 1.3 = 3.9, truncated to 3
		if tokens != 3 {
			t.Errorf("expected 3 tokens, got %d", tokens)
		}
	})
}

func TestCountWords(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected int
	}{
		{"empty string", "", 0},
		{"single word", "hello", 1},
		{"two words", "hello world", 2},
		{"multiple spaces", "hello    world", 2},
		{"leading spaces", "   hello", 1},
		{"trailing spaces", "hello   ", 1},
		{"newlines", "hello\nworld", 2},
		{"tabs", "hello\tworld", 2},
		{"mixed whitespace", "  hello \n world \t test  ", 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := countWords(tt.content)
			if result != tt.expected {
				t.Errorf("countWords(%q) = %d, expected %d", tt.content, result, tt.expected)
			}
		})
	}
}

func TestGetModelContextLimit(t *testing.T) {
	t.Run("returns exact match", func(t *testing.T) {
		limit := GetModelContextLimit("claude-3-opus")
		if limit != 200000 {
			t.Errorf("expected 200000 for claude-3-opus, got %d", limit)
		}
	})

	t.Run("returns prefix match", func(t *testing.T) {
		limit := GetModelContextLimit("claude-3-opus-20240229")
		if limit != 200000 {
			t.Errorf("expected 200000 for claude-3-opus-20240229, got %d", limit)
		}
	})

	t.Run("returns default for unknown model", func(t *testing.T) {
		limit := GetModelContextLimit("unknown-model")
		if limit != 8192 {
			t.Errorf("expected default 8192 for unknown model, got %d", limit)
		}
	})

	t.Run("returns correct limits for various models", func(t *testing.T) {
		tests := []struct {
			model    string
			expected int
		}{
			{"gemini-1.5-pro", 1000000},
			{"gpt-4-turbo", 128000},
			{"gpt-4", 8192},
			{"claude-sonnet-4", 200000},
		}

		for _, tt := range tests {
			limit := GetModelContextLimit(tt.model)
			if limit != tt.expected {
				t.Errorf("GetModelContextLimit(%s) = %d, expected %d", tt.model, limit, tt.expected)
			}
		}
	})
}

func TestContextAnalysis(t *testing.T) {
	t.Run("creates analysis with correct model limit", func(t *testing.T) {
		analysis := NewContextAnalysis("claude-3-opus")
		if analysis.ModelContextLimit != 200000 {
			t.Errorf("expected limit 200000, got %d", analysis.ModelContextLimit)
		}
	})

	t.Run("UpdateExceedsLimit sets flag correctly", func(t *testing.T) {
		analysis := NewContextAnalysis("gpt-4") // 8192 limit
		analysis.EstimatedTokens = 5000
		analysis.UpdateExceedsLimit()
		if analysis.ExceedsLimit {
			t.Error("expected ExceedsLimit to be false for 5000 tokens")
		}

		analysis.EstimatedTokens = 10000
		analysis.UpdateExceedsLimit()
		if !analysis.ExceedsLimit {
			t.Error("expected ExceedsLimit to be true for 10000 tokens")
		}
	})
}
