// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package capability

import (
	"testing"
)

// TestNewAnalyzer tests the creation of a new capability analyzer
func TestNewAnalyzer(t *testing.T) {
	analyzer := NewAnalyzer()
	if analyzer == nil {
		t.Fatal("NewAnalyzer returned nil")
	}
}

// TestAnalyzeNilModel tests that Analyze handles nil models gracefully
func TestAnalyzeNilModel(t *testing.T) {
	analyzer := NewAnalyzer()
	capability := analyzer.Analyze(nil)
	
	if capability == nil {
		t.Fatal("Analyze returned nil for nil model")
	}
	
	// Should return empty capability
	if capability.SupportsCoding || capability.SupportsReasoning || capability.SupportsVision {
		t.Error("Expected empty capability for nil model")
	}
}

// TestDetectCoding tests coding capability detection
// Requirements 2.2: Detect coding capability from model names containing
// "code", "codex", "deepseek", "kimi"
func TestDetectCoding(t *testing.T) {
	analyzer := NewAnalyzer()
	
	testCases := []struct {
		name           string
		modelID        string
		displayName    string
		expectCoding   bool
	}{
		{"GPT-4 with code", "gpt-4-code", "GPT-4 Code", true},
		{"Codex model", "codex-001", "Codex", true},
		{"DeepSeek Coder", "deepseek-coder", "DeepSeek Coder", true},
		{"Kimi model", "kimi-chat", "Kimi", true},
		{"StarCoder", "starcoder", "StarCoder", true},
		{"CodeLlama", "codellama-7b", "Code Llama", true},
		{"Regular chat model", "gpt-3.5-turbo", "GPT-3.5 Turbo", false},
		{"Claude Sonnet", "claude-sonnet", "Claude Sonnet", false},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			model := &DiscoveredModel{
				ID:          tc.modelID,
				DisplayName: tc.displayName,
				Provider:    "test",
			}
			
			capability := analyzer.Analyze(model)
			
			if capability.SupportsCoding != tc.expectCoding {
				t.Errorf("Expected SupportsCoding=%v for %s, got %v",
					tc.expectCoding, tc.modelID, capability.SupportsCoding)
			}
		})
	}
}

// TestDetectReasoning tests reasoning capability detection
// Requirements 2.3: Detect reasoning capability from model names containing
// "reasoner", "o1", "o3", "thinking", "pro"
func TestDetectReasoning(t *testing.T) {
	analyzer := NewAnalyzer()
	
	testCases := []struct {
		name             string
		modelID          string
		displayName      string
		expectReasoning  bool
	}{
		{"O1 model", "o1-preview", "O1 Preview", true},
		{"O3 model", "o3-mini", "O3 Mini", true},
		{"Reasoner model", "claude-reasoner", "Claude Reasoner", true},
		{"Thinking model", "thinking-claude", "Thinking Claude", true},
		{"Pro model", "gemini-pro", "Gemini Pro", true},
		{"Regular chat", "gpt-3.5-turbo", "GPT-3.5 Turbo", false},
		{"Fast model", "gpt-4o-mini", "GPT-4o Mini", false},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			model := &DiscoveredModel{
				ID:          tc.modelID,
				DisplayName: tc.displayName,
				Provider:    "test",
			}
			
			capability := analyzer.Analyze(model)
			
			if capability.SupportsReasoning != tc.expectReasoning {
				t.Errorf("Expected SupportsReasoning=%v for %s, got %v",
					tc.expectReasoning, tc.modelID, capability.SupportsReasoning)
			}
		})
	}
}

// TestDetectVision tests vision capability detection
// Requirements 2.4: Detect vision capability from model names containing
// "vision", "4o", "gpt-4", "gemini", "claude-3"
func TestDetectVision(t *testing.T) {
	analyzer := NewAnalyzer()
	
	testCases := []struct {
		name          string
		modelID       string
		displayName   string
		expectVision  bool
	}{
		{"GPT-4o", "gpt-4o", "GPT-4o", true},
		{"GPT-4 Turbo", "gpt-4-turbo", "GPT-4 Turbo", true},
		{"Claude 3 Opus", "claude-3-opus", "Claude 3 Opus", true},
		{"Claude 4", "claude-4-sonnet", "Claude 4 Sonnet", true},
		{"Gemini Pro", "gemini-pro", "Gemini Pro", true},
		{"LLaVA", "llava-v1.5", "LLaVA", true},
		{"Vision model", "vision-model", "Vision Model", true},
		{"Text-only model", "gpt-3.5-turbo", "GPT-3.5 Turbo", false},
		{"Llama 3", "llama-3-8b", "Llama 3", false},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			model := &DiscoveredModel{
				ID:          tc.modelID,
				DisplayName: tc.displayName,
				Provider:    "test",
			}
			
			capability := analyzer.Analyze(model)
			
			if capability.SupportsVision != tc.expectVision {
				t.Errorf("Expected SupportsVision=%v for %s, got %v",
					tc.expectVision, tc.modelID, capability.SupportsVision)
			}
		})
	}
}

// TestDetectLocal tests local model detection
func TestDetectLocal(t *testing.T) {
	analyzer := NewAnalyzer()
	
	testCases := []struct {
		name        string
		provider    string
		expectLocal bool
	}{
		{"Ollama", "ollama", true},
		{"LM Studio", "lmstudio", true},
		{"LocalAI", "localai", true},
		{"LlamaCpp", "llamacpp", true},
		{"OpenAI", "openai", false},
		{"Anthropic", "anthropic", false},
		{"Google", "google", false},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			model := &DiscoveredModel{
				ID:          "test-model",
				DisplayName: "Test Model",
				Provider:    tc.provider,
			}
			
			capability := analyzer.Analyze(model)
			
			if capability.IsLocal != tc.expectLocal {
				t.Errorf("Expected IsLocal=%v for provider %s, got %v",
					tc.expectLocal, tc.provider, capability.IsLocal)
			}
		})
	}
}

// TestInferLatency tests latency tier inference
func TestInferLatency(t *testing.T) {
	analyzer := NewAnalyzer()
	
	testCases := []struct {
		name            string
		modelID         string
		expectLatency   string
	}{
		{"Fast mini model", "gpt-4o-mini", "fast"},
		{"Fast turbo model", "gpt-3.5-turbo", "fast"},
		{"Fast flash model", "gemini-flash", "fast"},
		{"Fast haiku model", "claude-haiku", "fast"},
		{"Slow reasoner", "o1-preview", "slow"},
		{"Slow opus", "claude-opus", "slow"},
		{"Standard model", "gpt-4", "standard"},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			model := &DiscoveredModel{
				ID:          tc.modelID,
				DisplayName: tc.modelID,
				Provider:    "test",
			}
			
			capability := analyzer.Analyze(model)
			
			if capability.EstimatedLatency != tc.expectLatency {
				t.Errorf("Expected EstimatedLatency=%s for %s, got %s",
					tc.expectLatency, tc.modelID, capability.EstimatedLatency)
			}
		})
	}
}

// TestInferCostTier tests cost tier inference
func TestInferCostTier(t *testing.T) {
	analyzer := NewAnalyzer()
	
	testCases := []struct {
		name        string
		modelID     string
		provider    string
		expectCost  string
	}{
		{"Local model", "llama-3", "ollama", "free"},
		{"Expensive opus", "claude-opus", "anthropic", "high"},
		{"Expensive o1", "o1-preview", "openai", "high"},
		{"Cheap mini", "gpt-4o-mini", "openai", "low"},
		{"Cheap haiku", "claude-haiku", "anthropic", "low"},
		{"Medium model", "gpt-4", "openai", "medium"},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			model := &DiscoveredModel{
				ID:          tc.modelID,
				DisplayName: tc.modelID,
				Provider:    tc.provider,
			}
			
			capability := analyzer.Analyze(model)
			
			if capability.CostTier != tc.expectCost {
				t.Errorf("Expected CostTier=%s for %s, got %s",
					tc.expectCost, tc.modelID, capability.CostTier)
			}
		})
	}
}

// TestInferContextWindow tests context window inference
// Requirements 2.5: Extract context window size from model metadata when available
func TestInferContextWindow(t *testing.T) {
	analyzer := NewAnalyzer()
	
	testCases := []struct {
		name          string
		modelID       string
		expectWindow  int
	}{
		{"128k model", "gpt-4-128k", 128000},
		{"200k model", "claude-200k", 200000},
		{"GPT-4", "gpt-4", 128000},
		{"Claude", "claude-3-opus", 128000},
		{"Gemini", "gemini-pro", 128000},
		{"Mini model", "gpt-4o-mini", 8000},
		{"Haiku model", "claude-haiku", 8000},
		{"Default model", "llama-3", 32000},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			model := &DiscoveredModel{
				ID:          tc.modelID,
				DisplayName: tc.modelID,
				Provider:    "test",
			}
			
			capability := analyzer.Analyze(model)
			
			if capability.ContextWindow != tc.expectWindow {
				t.Errorf("Expected ContextWindow=%d for %s, got %d",
					tc.expectWindow, tc.modelID, capability.ContextWindow)
			}
		})
	}
}

// TestAnalyzeBatch tests batch analysis of multiple models
func TestAnalyzeBatch(t *testing.T) {
	analyzer := NewAnalyzer()
	
	models := []*DiscoveredModel{
		{
			ID:          "gpt-4-code",
			DisplayName: "GPT-4 Code",
			Provider:    "openai",
		},
		{
			ID:          "claude-3-opus",
			DisplayName: "Claude 3 Opus",
			Provider:    "anthropic",
		},
		{
			ID:          "llama-3",
			DisplayName: "Llama 3",
			Provider:    "ollama",
		},
	}
	
	capabilities := analyzer.AnalyzeBatch(models)
	
	if len(capabilities) != len(models) {
		t.Errorf("Expected %d capabilities, got %d", len(models), len(capabilities))
	}
	
	// Verify first model (coding)
	if !capabilities[0].SupportsCoding {
		t.Error("Expected first model to support coding")
	}
	
	// Verify second model (vision)
	if !capabilities[1].SupportsVision {
		t.Error("Expected second model to support vision")
	}
	
	// Verify third model (local)
	if !capabilities[2].IsLocal {
		t.Error("Expected third model to be local")
	}
}

// TestMultipleCapabilities tests models with multiple capabilities
func TestMultipleCapabilities(t *testing.T) {
	analyzer := NewAnalyzer()
	
	// GPT-4o should have coding, reasoning, and vision
	model := &DiscoveredModel{
		ID:          "gpt-4o",
		DisplayName: "GPT-4o",
		Provider:    "openai",
	}
	
	capability := analyzer.Analyze(model)
	
	if !capability.SupportsVision {
		t.Error("Expected GPT-4o to support vision")
	}
	
	// Claude 3 Opus should have vision
	model = &DiscoveredModel{
		ID:          "claude-3-opus",
		DisplayName: "Claude 3 Opus",
		Provider:    "anthropic",
	}
	
	capability = analyzer.Analyze(model)
	
	if !capability.SupportsVision {
		t.Error("Expected Claude 3 Opus to support vision")
	}
}

// TestCaseInsensitivity tests that pattern matching is case-insensitive
func TestCaseInsensitivity(t *testing.T) {
	analyzer := NewAnalyzer()
	
	testCases := []struct {
		modelID     string
		displayName string
	}{
		{"GPT-4-CODE", "GPT-4 Code"},
		{"gpt-4-code", "gpt-4 code"},
		{"GpT-4-CoDe", "GpT-4 CoDe"},
	}
	
	for _, tc := range testCases {
		model := &DiscoveredModel{
			ID:          tc.modelID,
			DisplayName: tc.displayName,
			Provider:    "test",
		}
		
		capability := analyzer.Analyze(model)
		
		if !capability.SupportsCoding {
			t.Errorf("Expected coding support for %s (case-insensitive)", tc.modelID)
		}
	}
}
