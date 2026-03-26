package autoroute

import (
	"testing"
)

// ─── ClassifyIntent Tests ───

func TestClassifyIntent_ExplicitHint(t *testing.T) {
	c := ClassifyIntent("random content", "coding")
	if c.Intent != IntentCoding {
		t.Errorf("Expected Intent=%q, got %q", IntentCoding, c.Intent)
	}
	if c.Confidence != 1.0 {
		t.Errorf("Expected Confidence=1.0 for explicit hint, got %f", c.Confidence)
	}
	if c.Method != "hint" {
		t.Errorf("Expected Method=hint, got %q", c.Method)
	}
}

func TestClassifyIntent_HintNormalization(t *testing.T) {
	tests := map[string]string{
		"code":    IntentCoding,
		"program": IntentCoding,
		"dev":     IntentCoding,
		"reason":  IntentReasoning,
		"think":   IntentReasoning,
		"math":    IntentReasoning,
		"write":   IntentCreative,
		"story":   IntentCreative,
		"quick":   IntentFast,
		"chat":    IntentFast,
		"image":   IntentVision,
		"private": IntentSecure,
		"local":   IntentSecure,
	}
	for hint, expected := range tests {
		c := ClassifyIntent("", hint)
		if c.Intent != expected {
			t.Errorf("Hint %q: expected %q, got %q", hint, expected, c.Intent)
		}
	}
}

func TestClassifyIntent_CodingCodeBlock(t *testing.T) {
	content := "Can you fix this?\n```go\nfunc main() {}\n```"
	c := ClassifyIntent(content, "")
	if c.Intent != IntentCoding {
		t.Errorf("Expected coding for code block, got %q", c.Intent)
	}
	if c.Method != "heuristic" {
		t.Errorf("Expected method=heuristic, got %q", c.Method)
	}
}

func TestClassifyIntent_CodingKeywords(t *testing.T) {
	tests := []string{
		"Write a function that sorts an array",
		"Please refactor this code to use interfaces",
		"I have a compile error in my Go project",
		"Can you write code for a REST API endpoint?",
	}
	for _, content := range tests {
		c := ClassifyIntent(content, "")
		if c.Intent != IntentCoding {
			t.Errorf("Expected coding for %q, got %q", content, c.Intent)
		}
	}
}

func TestClassifyIntent_Reasoning(t *testing.T) {
	tests := []string{
		"Explain why quicksort has O(n log n) average complexity",
		"Analyze the pros and cons of microservices vs monolith",
		"Step by step solve this equation: 2x + 5 = 13",
		"Compare and contrast React and Vue frameworks",
	}
	for _, content := range tests {
		c := ClassifyIntent(content, "")
		if c.Intent != IntentReasoning {
			t.Errorf("Expected reasoning for %q, got %q", content, c.Intent)
		}
	}
}

func TestClassifyIntent_Creative(t *testing.T) {
	tests := []string{
		"Write a story about a robot learning to love",
		"Write a poem about the ocean",
		"Brainstorm 10 startup ideas for AI",
		"Draft a blog post about climate change",
	}
	for _, content := range tests {
		c := ClassifyIntent(content, "")
		if c.Intent != IntentCreative {
			t.Errorf("Expected creative for %q, got %q", content, c.Intent)
		}
	}
}

func TestClassifyIntent_Fast(t *testing.T) {
	tests := []string{
		"What is the capital of France?",
		"How many days in a year?",
		"Define entropy",
	}
	for _, content := range tests {
		c := ClassifyIntent(content, "")
		if c.Intent != IntentFast {
			t.Errorf("Expected fast for %q, got %q", content, c.Intent)
		}
	}
}

func TestClassifyIntent_Vision(t *testing.T) {
	tests := []string{
		"What do you see in this image?",
		"Describe the image attached",
		"Please read the text in this screenshot",
	}
	for _, content := range tests {
		c := ClassifyIntent(content, "")
		if c.Intent != IntentVision {
			t.Errorf("Expected vision for %q, got %q", content, c.Intent)
		}
	}
}

func TestClassifyIntent_Secure(t *testing.T) {
	tests := []string{
		"Generate a password for my database",
		"Analyze this api key rotation strategy",
		"This contains confidential employee data",
	}
	for _, content := range tests {
		c := ClassifyIntent(content, "")
		if c.Intent != IntentSecure {
			t.Errorf("Expected secure for %q, got %q", content, c.Intent)
		}
	}
}

func TestClassifyIntent_General(t *testing.T) {
	content := "Tell me about the history of basketball and its evolution over the decades"
	c := ClassifyIntent(content, "")
	if c.Intent != IntentGeneral {
		t.Errorf("Expected general for ambiguous content, got %q", c.Intent)
	}
}

func TestClassifyIntent_EmptyContent(t *testing.T) {
	c := ClassifyIntent("", "")
	if c.Intent != IntentGeneral {
		t.Errorf("Expected general for empty content, got %q", c.Intent)
	}
}

// ─── normalizeIntent Tests ───

func TestNormalizeIntent_Unknown(t *testing.T) {
	result := normalizeIntent("mystery-intent")
	if result != "mystery-intent" {
		t.Errorf("Expected unknown intent to pass through, got %q", result)
	}
}

// ─── Benchmark ───

func BenchmarkClassifyIntent(b *testing.B) {
	content := "Write a function in Go that implements a binary search tree with insert, delete, and search operations. Include proper error handling and unit tests."
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ClassifyIntent(content, "")
	}
}

func BenchmarkClassifyIntent_Short(b *testing.B) {
	content := "What is 2+2?"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ClassifyIntent(content, "")
	}
}
