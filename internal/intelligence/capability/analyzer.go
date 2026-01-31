// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package capability provides model capability analysis for the intelligence system.
// It infers model capabilities from model names and metadata to enable intelligent
// routing decisions.
package capability

import (
	"strings"

	log "github.com/sirupsen/logrus"
)

// ModelCapability represents inferred capabilities for a model.
type ModelCapability struct {
	SupportsCoding    bool   `json:"supports_coding"`
	SupportsReasoning bool   `json:"supports_reasoning"`
	SupportsVision    bool   `json:"supports_vision"`
	ContextWindow     int    `json:"context_window"`
	EstimatedLatency  string `json:"estimated_latency"`
	CostTier          string `json:"cost_tier"`
	IsLocal           bool   `json:"is_local"`
}

// DiscoveredModel represents a minimal model structure for analysis.
// This avoids import cycles with the discovery package.
type DiscoveredModel struct {
	ID          string
	Provider    string
	DisplayName string
}

// Analyzer infers model capabilities from model names and metadata.
type Analyzer struct{}

// NewAnalyzer creates a new CapabilityAnalyzer instance.
//
// Returns:
//   - *Analyzer: A new capability analyzer instance
func NewAnalyzer() *Analyzer {
	return &Analyzer{}
}

// Analyze infers capabilities for a single discovered model.
// It uses name-based pattern matching and metadata analysis to determine
// what the model is capable of.
//
// Parameters:
//   - model: The discovered model to analyze
//
// Returns:
//   - *ModelCapability: The inferred capabilities
func (a *Analyzer) Analyze(model *DiscoveredModel) *ModelCapability {
	if model == nil {
		return &ModelCapability{}
	}

	capability := &ModelCapability{}
	
	// Normalize model name for pattern matching
	modelName := strings.ToLower(model.ID)
	displayName := strings.ToLower(model.DisplayName)
	
	// Detect coding capability
	capability.SupportsCoding = a.detectCoding(modelName, displayName)
	
	// Detect reasoning capability
	capability.SupportsReasoning = a.detectReasoning(modelName, displayName)
	
	// Detect vision capability
	capability.SupportsVision = a.detectVision(modelName, displayName)
	
	// Detect if model is local
	capability.IsLocal = a.detectLocal(model.Provider)
	
	// Infer latency tier based on model characteristics
	capability.EstimatedLatency = a.inferLatency(modelName, displayName)
	
	// Infer cost tier
	capability.CostTier = a.inferCostTier(modelName, displayName, capability.IsLocal)
	
	// Context window inference would require metadata from providers
	// For now, we set reasonable defaults based on model patterns
	capability.ContextWindow = a.inferContextWindow(modelName, displayName)
	
	log.Debugf("Analyzed model %s: coding=%v, reasoning=%v, vision=%v, local=%v",
		model.ID, capability.SupportsCoding, capability.SupportsReasoning,
		capability.SupportsVision, capability.IsLocal)
	
	return capability
}

// AnalyzeBatch infers capabilities for multiple models in batch.
//
// Parameters:
//   - models: List of discovered models to analyze
//
// Returns:
//   - []*ModelCapability: List of inferred capabilities
func (a *Analyzer) AnalyzeBatch(models []*DiscoveredModel) []*ModelCapability {
	capabilities := make([]*ModelCapability, len(models))
	for i, model := range models {
		capabilities[i] = a.Analyze(model)
	}
	return capabilities
}

// detectCoding checks if a model supports coding tasks.
// Requirements 2.2: Detect coding capability from model names containing
// "code", "codex", "deepseek", "kimi"
func (a *Analyzer) detectCoding(modelName, displayName string) bool {
	codingPatterns := []string{
		"code",
		"codex",
		"deepseek",
		"kimi",
		"coder",
		"starcoder",
		"codellama",
	}
	
	for _, pattern := range codingPatterns {
		if strings.Contains(modelName, pattern) || strings.Contains(displayName, pattern) {
			return true
		}
	}
	
	return false
}

// detectReasoning checks if a model supports reasoning tasks.
// Requirements 2.3: Detect reasoning capability from model names containing
// "reasoner", "o1", "o3", "thinking", "pro"
func (a *Analyzer) detectReasoning(modelName, displayName string) bool {
	reasoningPatterns := []string{
		"reasoner",
		"o1",
		"o3",
		"thinking",
		"pro",
		"reasoning",
		"think",
	}
	
	for _, pattern := range reasoningPatterns {
		if strings.Contains(modelName, pattern) || strings.Contains(displayName, pattern) {
			return true
		}
	}
	
	return false
}

// detectVision checks if a model supports vision/image tasks.
// Requirements 2.4: Detect vision capability from model names containing
// "vision", "4o", "gpt-4", "gemini", "claude-3"
func (a *Analyzer) detectVision(modelName, displayName string) bool {
	visionPatterns := []string{
		"vision",
		"4o",
		"gpt-4",
		"gemini",
		"claude-3",
		"claude-4",
		"llava",
		"multimodal",
	}
	
	for _, pattern := range visionPatterns {
		if strings.Contains(modelName, pattern) || strings.Contains(displayName, pattern) {
			return true
		}
	}
	
	return false
}

// detectLocal checks if a model is running locally.
func (a *Analyzer) detectLocal(provider string) bool {
	localProviders := []string{
		"ollama",
		"lmstudio",
		"localai",
		"llamacpp",
	}
	
	providerLower := strings.ToLower(provider)
	for _, local := range localProviders {
		if strings.Contains(providerLower, local) {
			return true
		}
	}
	
	return false
}

// inferLatency estimates the latency tier for a model.
func (a *Analyzer) inferLatency(modelName, displayName string) string {
	// Fast models (small, optimized)
	fastPatterns := []string{
		"mini",
		"fast",
		"turbo",
		"flash",
		"haiku",
		"0.5b",
		"1b",
		"3b",
	}
	
	for _, pattern := range fastPatterns {
		if strings.Contains(modelName, pattern) || strings.Contains(displayName, pattern) {
			return "fast"
		}
	}
	
	// Slow models (reasoning, large)
	slowPatterns := []string{
		"reasoner",
		"o1",
		"o3",
		"thinking",
		"opus",
		"70b",
		"405b",
	}
	
	for _, pattern := range slowPatterns {
		if strings.Contains(modelName, pattern) || strings.Contains(displayName, pattern) {
			return "slow"
		}
	}
	
	// Default to standard
	return "standard"
}

// inferCostTier estimates the cost tier for a model.
func (a *Analyzer) inferCostTier(modelName, displayName string, isLocal bool) string {
	// Local models are free
	if isLocal {
		return "free"
	}
	
	// Cheap models (check before expensive patterns)
	cheapPatterns := []string{
		"mini",
		"haiku",
		"flash",
		"3.5",
	}
	
	for _, pattern := range cheapPatterns {
		if strings.Contains(modelName, pattern) || strings.Contains(displayName, pattern) {
			return "low"
		}
	}
	
	// Expensive models
	expensivePatterns := []string{
		"opus",
		"o1",
		"o3",
		"claude-4",
	}
	
	for _, pattern := range expensivePatterns {
		if strings.Contains(modelName, pattern) || strings.Contains(displayName, pattern) {
			return "high"
		}
	}
	
	// Default to medium
	return "medium"
}

// inferContextWindow estimates the context window size.
// Requirements 2.5: Extract context window size from model metadata when available
func (a *Analyzer) inferContextWindow(modelName, displayName string) int {
	// Check for explicit size markers first
	if strings.Contains(modelName, "128k") || strings.Contains(displayName, "128k") {
		return 128000
	}
	if strings.Contains(modelName, "200k") || strings.Contains(displayName, "200k") {
		return 200000
	}
	
	// Gemini models (check early since "gemini" contains "mini")
	if strings.Contains(modelName, "gemini") {
		return 128000
	}
	
	// Check for small model indicators (more specific patterns)
	// These need to be checked before broader patterns like "gpt-4" or "claude"
	if strings.Contains(modelName, "-mini") || strings.Contains(modelName, "-haiku") {
		return 8000
	}
	
	// GPT-4 and Claude models typically have large contexts
	if strings.Contains(modelName, "gpt-4") || strings.Contains(modelName, "claude") {
		return 128000
	}
	
	// Default context window
	return 32000
}
