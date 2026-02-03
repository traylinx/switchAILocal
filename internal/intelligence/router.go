// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package intelligence provides the Cortex Router for intelligent request routing.
// The Cortex Router implements multi-tier classification (Reflex → Semantic → Cognitive)
// with memory integration for learning and preference-based routing.
package intelligence

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/traylinx/switchAILocal/internal/config"
	"github.com/traylinx/switchAILocal/internal/hooks"
	"github.com/traylinx/switchAILocal/internal/learning"
	"github.com/traylinx/switchAILocal/internal/memory"
	"github.com/traylinx/switchAILocal/internal/steering"
)

// CortexRouter implements intelligent multi-tier routing with memory integration.
// It provides three routing tiers: Reflex (fast rules), Semantic (intent matching),
// and Cognitive (LLM classification), with learned preferences from memory.
type CortexRouter struct {
	config        *config.IntelligenceConfig
	memoryManager memory.MemoryManager
	learningEngine *learning.LearningEngine

	// Services from intelligence service
	semanticTier SemanticTierInterface
	cache        SemanticCacheInterface
	steering     *steering.SteeringEngine
	eventBus     *hooks.EventBus

	// Configuration
	enabled       bool
	memoryEnabled bool
}

// RoutingRequest represents an incoming request to be routed.
type RoutingRequest struct {
	APIKey   string                 `json:"api_key"`
	Model    string                 `json:"model"`
	Messages []map[string]string    `json:"messages"`
	Content  string                 `json:"content"` // Extracted content for analysis
	Metadata map[string]interface{} `json:"metadata"`
}

// RoutingDecision represents the router's decision with confidence and reasoning.
type RoutingDecision struct {
	// Request information
	APIKeyHash  string    `json:"api_key_hash"`
	RequestHash string    `json:"request_hash"`
	Timestamp   time.Time `json:"timestamp"`

	// Classification results
	Intent     string `json:"intent"`
	Complexity string `json:"complexity"`
	Privacy    string `json:"privacy"`
	Tier       string `json:"tier"` // reflex, semantic, cognitive, learned

	// Routing decision
	SelectedModel string  `json:"selected_model"`
	Provider      string  `json:"provider"`
	Confidence    float64 `json:"confidence"`

	// Performance metrics
	LatencyMs int64 `json:"latency_ms"`

	// Memory integration
	UsedMemory   bool   `json:"used_memory"`
	MemorySource string `json:"memory_source,omitempty"` // preferences, cache, quirks

	// Reasoning
	Reason string `json:"reason"`
}

// RoutingOutcome represents the outcome of a routing decision for learning.
type RoutingOutcome struct {
	Decision       *RoutingDecision `json:"decision"`
	Success        bool             `json:"success"`
	ResponseTimeMs int64            `json:"response_time_ms"`
	Error          string           `json:"error,omitempty"`
	QualityScore   float64          `json:"quality_score"` // 0.0 to 1.0
	UserFeedback   string           `json:"user_feedback,omitempty"`
}

// NewCortexRouter creates a new Cortex Router with optional memory integration.
func NewCortexRouter(cfg *config.IntelligenceConfig, memoryManager memory.MemoryManager) *CortexRouter {
	if cfg == nil {
		cfg = &config.IntelligenceConfig{
			Enabled: false,
		}
	}

	router := &CortexRouter{
		config:        cfg,
		memoryManager: memoryManager,
		enabled:       cfg.Enabled,
		memoryEnabled: memoryManager != nil,
	}

	// Initialize learning engine if memory is available
	if memoryManager != nil && cfg.Learning.Enabled {
		if learningEngine, err := learning.NewLearningEngine(&cfg.Learning, memoryManager); err == nil {
			router.learningEngine = learningEngine
			learningEngine.Start() // Start background analysis
		} else {
			log.Printf("Failed to initialize learning engine: %v", err)
		}
	}

	return router
}

// SetSemanticTier sets the semantic tier for intent matching.
func (cr *CortexRouter) SetSemanticTier(tier SemanticTierInterface) {
	cr.semanticTier = tier
}

// SetSemanticCache sets the semantic cache for caching routing decisions.
func (cr *CortexRouter) SetSemanticCache(cache SemanticCacheInterface) {
	cr.cache = cache
}

func (cr *CortexRouter) SetSteeringEngine(engine *steering.SteeringEngine) {
	cr.steering = engine
}

// SetEventBus sets the event bus for publishing routing events.
func (cr *CortexRouter) SetEventBus(bus *hooks.EventBus) {
	cr.eventBus = bus
}

// Close shuts down the router and its components.
func (cr *CortexRouter) Close() {
	if cr.learningEngine != nil {
		cr.learningEngine.Stop()
	}
}

// Route performs intelligent routing using the multi-tier approach.
// It tries tiers in order: Reflex → Semantic → Cognitive, with memory integration.
func (cr *CortexRouter) Route(ctx context.Context, request *RoutingRequest) (*RoutingDecision, error) {
	startTime := time.Now()

	// Publish request received event
	if cr.eventBus != nil {
		cr.eventBus.PublishAsync(&hooks.EventContext{
			Event:     hooks.EventRequestReceived,
			Timestamp: startTime,
			Data: map[string]interface{}{
				"content_length": len(request.Content),
				"model":          request.Model,
			},
			Provider: "user", // Source
		})
	}

	// Hash API key for privacy
	apiKeyHash := cr.hashAPIKey(request.APIKey)

	// Extract content for analysis
	content := cr.extractContent(request)
	contentHash := cr.hashContent(content)

	decision := &RoutingDecision{
		APIKeyHash:  apiKeyHash,
		RequestHash: contentHash,
		Timestamp:   startTime,
		UsedMemory:  false,
	}

	// Step 0: Apply Steering Rules
	if cr.steering != nil && cr.config.Steering.Enabled {
		// Build routing context for steering (simplified for now)
		routingCtx := &steering.RoutingContext{
			Intent:        "", // Will be classified below
			APIKeyHash:    apiKeyHash,
			ContentLength: len(content),
			Hour:          startTime.Hour(),
			DayOfWeek:     startTime.Weekday().String(),
			Timestamp:     startTime,
			Metadata:      request.Metadata,
		}

		// First pass intent classification for steering (reflex-like)
		if routingCtx.Intent == "" {
			routingCtx.Intent = cr.classifyIntentFromContent(content)
		}

		rules, _ := cr.steering.FindMatchingRules(routingCtx)
		if len(rules) > 0 {
			steeringModel, newMessages, newMetadata := cr.steering.ApplySteering(routingCtx, request.Messages, request.Metadata, rules)

			// Update the request with steered modifications
			request.Messages = newMessages
			request.Metadata = newMetadata

			// If a model was explicitly chosen by steering and we should override
			if steeringModel != "" {
				// We still need to decide if we stop here or continue.
				// For now, if steering selected a model, we consider it a "reflex" or "steered" tier.
				decision.Tier = "steered"
				decision.SelectedModel = steeringModel
				decision.Confidence = 1.0 // High confidence for explicit steering
				decision.LatencyMs = time.Since(startTime).Milliseconds()
				decision.Reason = "Applied steering rules: "
				for i, r := range rules {
					if i > 0 {
						decision.Reason += ", "
					}
					decision.Reason += r.Name
				}

				// Important: Extract provider from steered model
				decision.Provider = cr.extractProvider(steeringModel)

				cr.recordDecision(decision)
				return decision, nil
			}
		}
	}

	// Step 1: Check memory for learned preferences (if enabled)
	if cr.memoryEnabled {
		if memoryDecision := cr.tryMemoryRouting(ctx, request, decision); memoryDecision != nil {
			memoryDecision.LatencyMs = time.Since(startTime).Milliseconds()
			return memoryDecision, nil
		}
	}

	// Step 2: Try Reflex tier (fast rule-based routing)
	if reflexDecision := cr.tryReflexTier(ctx, request, decision); reflexDecision != nil {
		reflexDecision.LatencyMs = time.Since(startTime).Milliseconds()
		cr.recordDecision(reflexDecision)
		return reflexDecision, nil
	}

	// Step 3: Try Semantic tier (intent matching with embeddings)
	if cr.semanticTier != nil && cr.semanticTier.IsEnabled() {
		if semanticDecision := cr.trySemanticTier(ctx, request, decision); semanticDecision != nil {
			semanticDecision.LatencyMs = time.Since(startTime).Milliseconds()
			cr.recordDecision(semanticDecision)
			return semanticDecision, nil
		}
	}

	// Step 4: Fall back to Cognitive tier (LLM classification)
	cognitiveDecision := cr.tryCognitiveTier(ctx, request, decision)
	cognitiveDecision.LatencyMs = time.Since(startTime).Milliseconds()
	cr.recordDecision(cognitiveDecision)

	return cognitiveDecision, nil
}

// tryMemoryRouting attempts to route using learned preferences and cached decisions.
func (cr *CortexRouter) tryMemoryRouting(ctx context.Context, request *RoutingRequest, decision *RoutingDecision) *RoutingDecision {
	// Check semantic cache first
	if cr.cache != nil && cr.cache.IsEnabled() {
		content := cr.extractContent(request)
		if cached, err := cr.cache.Lookup(content); err == nil && cached != nil {
			// Use cached decision
			decision.Tier = "learned"
			decision.SelectedModel = "cached-model" // Would extract from cached decision
			decision.Confidence = 0.95              // High confidence for cached results
			decision.UsedMemory = true
			decision.MemorySource = "cache"
			decision.Reason = "Found cached routing decision"
			
			// Apply learning-based confidence adjustment
			decision.Confidence = cr.adjustConfidenceWithMemory(decision)
			
			return decision
		}
	}

	// Check user preferences
	if prefs, err := cr.memoryManager.GetUserPreferences(decision.APIKeyHash); err == nil && prefs != nil {
		// Try to match intent from preferences
		content := cr.extractContent(request)
		if intent := cr.classifyIntentFromContent(content); intent != "" {
			if preferredModel, exists := prefs.ModelPreferences[intent]; exists {
				decision.Tier = "learned"
				decision.Intent = intent
				decision.SelectedModel = preferredModel
				decision.Confidence = 0.85 // Good confidence for learned preferences
				decision.UsedMemory = true
				decision.MemorySource = "preferences"
				decision.Reason = fmt.Sprintf("Using learned preference for intent '%s'", intent)
				
				// Apply learning-based confidence adjustment
				decision.Confidence = cr.adjustConfidenceWithMemory(decision)
				
				return decision
			}
		}
	}

	return nil
}

// tryReflexTier implements fast rule-based routing for common patterns.
func (cr *CortexRouter) tryReflexTier(ctx context.Context, request *RoutingRequest, decision *RoutingDecision) *RoutingDecision {
	content := strings.ToLower(cr.extractContent(request))

	// PII Detection (security-sensitive routing)
	if cr.containsPII(content) {
		decision.Tier = "reflex"
		decision.Intent = "pii_detected"
		decision.Privacy = "pii"
		decision.SelectedModel = "ollama:qwen:0.5b" // Route to local model for privacy
		decision.Confidence = 0.99
		decision.Reason = "PII detected, routing to local model for privacy"
		
		// Apply learning-based confidence adjustment
		if cr.memoryEnabled {
			decision.Confidence = cr.adjustConfidenceWithMemory(decision)
		}
		
		return decision
	}

	// Simple greetings
	if cr.isSimpleGreeting(content) {
		decision.Tier = "reflex"
		decision.Intent = "chat"
		decision.Complexity = "simple"
		decision.Privacy = "public"
		decision.SelectedModel = "ollama:qwen:0.5b" // Fast local model for simple queries
		decision.Confidence = 0.95
		decision.Reason = "Simple greeting detected"
		
		// Apply learning-based confidence adjustment
		if cr.memoryEnabled {
			decision.Confidence = cr.adjustConfidenceWithMemory(decision)
		}
		
		return decision
	}

	// Code patterns
	if cr.containsCodePatterns(content) {
		decision.Tier = "reflex"
		decision.Intent = "coding"
		decision.Complexity = "complex"
		decision.Privacy = "public"
		decision.SelectedModel = "claudecli:claude-sonnet-4" // Best for coding
		decision.Confidence = 0.90
		decision.Reason = "Code patterns detected"
		
		// Apply learning-based confidence adjustment
		if cr.memoryEnabled {
			decision.Confidence = cr.adjustConfidenceWithMemory(decision)
		}
		
		return decision
	}

	// Math/reasoning patterns
	if cr.containsMathPatterns(content) {
		decision.Tier = "reflex"
		decision.Intent = "reasoning"
		decision.Complexity = "complex"
		decision.Privacy = "public"
		decision.SelectedModel = "geminicli:gemini-2.5-pro" // Good for reasoning
		decision.Confidence = 0.85
		decision.Reason = "Mathematical reasoning patterns detected"
		
		// Apply learning-based confidence adjustment
		if cr.memoryEnabled {
			decision.Confidence = cr.adjustConfidenceWithMemory(decision)
		}
		
		return decision
	}

	return nil
}

// trySemanticTier uses embedding-based intent matching.
func (cr *CortexRouter) trySemanticTier(ctx context.Context, request *RoutingRequest, decision *RoutingDecision) *RoutingDecision {
	content := cr.extractContent(request)

	result, err := cr.semanticTier.MatchIntent(content)
	if err != nil || result == nil {
		return nil
	}

	// Only use semantic result if confidence is high enough
	if result.Confidence < 0.7 {
		return nil
	}

	decision.Tier = "semantic"
	decision.Intent = result.Intent
	decision.Confidence = result.Confidence

	// Map intent to model based on configuration or defaults
	decision.SelectedModel = cr.mapIntentToModel(result.Intent)
	decision.Reason = fmt.Sprintf("Semantic intent matching: %s (confidence: %.2f)", result.Intent, result.Confidence)

	// Adjust confidence based on memory if available
	if cr.memoryEnabled {
		decision.Confidence = cr.adjustConfidenceWithMemory(decision)
	}

	return decision
}

// tryCognitiveTier uses LLM classification as the final fallback.
func (cr *CortexRouter) tryCognitiveTier(ctx context.Context, request *RoutingRequest, decision *RoutingDecision) *RoutingDecision {
	// This would integrate with the existing classification system in handlers.go
	// For now, we'll implement a simplified version

	content := cr.extractContent(request)

	// Simulate LLM classification (in real implementation, this would call the LLM)
	classification := cr.simulateLLMClassification(content)

	decision.Tier = "cognitive"
	decision.Intent = classification.Intent
	decision.Complexity = classification.Complexity
	decision.Privacy = classification.Privacy
	decision.Confidence = classification.Confidence
	decision.SelectedModel = cr.mapIntentToModel(classification.Intent)
	decision.Reason = "LLM-based classification"

	// Apply learning-based confidence adjustment
	if cr.memoryEnabled {
		decision.Confidence = cr.adjustConfidenceWithMemory(decision)
	}

	return decision
}

// RecordOutcome records the outcome of a routing decision for learning.
func (cr *CortexRouter) RecordOutcome(outcome *RoutingOutcome) error {
	if !cr.memoryEnabled {
		return nil
	}

	// Convert to memory format
	memoryDecision := &memory.RoutingDecision{
		Timestamp:  outcome.Decision.Timestamp,
		APIKeyHash: outcome.Decision.APIKeyHash,
		Request: memory.RequestInfo{
			Model:         outcome.Decision.SelectedModel,
			Intent:        outcome.Decision.Intent,
			ContentHash:   outcome.Decision.RequestHash,
			ContentLength: len(outcome.Decision.Reason), // Approximate
		},
		Routing: memory.RoutingInfo{
			Tier:          outcome.Decision.Tier,
			SelectedModel: outcome.Decision.SelectedModel,
			Confidence:    outcome.Decision.Confidence,
			LatencyMs:     outcome.Decision.LatencyMs,
		},
		Outcome: memory.OutcomeInfo{
			Success:        outcome.Success,
			ResponseTimeMs: outcome.ResponseTimeMs,
			Error:          outcome.Error,
			QualityScore:   outcome.QualityScore,
		},
	}

	// Record the decision
	if err := cr.memoryManager.RecordRouting(memoryDecision); err != nil {
		log.Printf("Failed to record routing decision: %v", err)
		return err
	}

	// Learn from the outcome
	if err := cr.memoryManager.LearnFromOutcome(memoryDecision); err != nil {
		log.Printf("Failed to learn from outcome: %v", err)
		return err
	}

	return nil
}

// CreateOutcome creates a RoutingOutcome from a routing decision and execution result.
// This is a helper method to make it easier to record outcomes from executors.
func (cr *CortexRouter) CreateOutcome(decision *RoutingDecision, success bool, responseTimeMs int64, err error) *RoutingOutcome {
	outcome := &RoutingOutcome{
		Decision:       decision,
		Success:        success,
		ResponseTimeMs: responseTimeMs,
	}

	// Add error message if present
	if err != nil {
		outcome.Error = err.Error()
	}

	// Calculate quality score
	outcome.QualityScore = memory.CalculateQualityScore(success, responseTimeMs, err != nil)

	return outcome
}

// Helper methods

func (cr *CortexRouter) hashAPIKey(apiKey string) string {
	hash := sha256.Sum256([]byte(apiKey))
	return fmt.Sprintf("sha256:%x", hash)
}

func (cr *CortexRouter) hashContent(content string) string {
	hash := sha256.Sum256([]byte(content))
	return fmt.Sprintf("sha256:%x", hash)
}

func (cr *CortexRouter) extractContent(request *RoutingRequest) string {
	if request.Content != "" {
		return request.Content
	}

	// Extract from messages
	var content strings.Builder
	for _, msg := range request.Messages {
		if msg["role"] == "user" {
			content.WriteString(msg["content"])
			content.WriteString(" ")
		}
	}

	return strings.TrimSpace(content.String())
}

func (cr *CortexRouter) containsPII(content string) bool {
	// Simple PII detection patterns
	piiPatterns := []string{
		"@", // Email patterns
		"phone", "tel:", "+1", "(555)",
		"ssn", "social security",
		"credit card", "visa", "mastercard",
		"api_key", "secret", "token",
	}

	for _, pattern := range piiPatterns {
		if strings.Contains(content, pattern) {
			return true
		}
	}

	return false
}

func (cr *CortexRouter) isSimpleGreeting(content string) bool {
	greetings := []string{"hello", "hi", "hey", "good morning", "good afternoon", "good evening"}
	for _, greeting := range greetings {
		if strings.Contains(content, greeting) && len(content) < 50 {
			return true
		}
	}
	return false
}

func (cr *CortexRouter) containsCodePatterns(content string) bool {
	codePatterns := []string{
		"function", "def ", "class ", "import ", "from ",
		"console.log", "print(", "printf", "echo ",
		"if (", "for (", "while (", "switch (",
		"```", "```python", "```javascript", "```go",
		"git ", "npm ", "pip ", "cargo ",
		"coding", "programming", "software", "development",
		"algorithm", "data structure", "binary tree",
	}

	for _, pattern := range codePatterns {
		if strings.Contains(content, pattern) {
			return true
		}
	}

	return false
}

func (cr *CortexRouter) containsMathPatterns(content string) bool {
	mathPatterns := []string{
		"calculate", "solve", "equation", "formula",
		"derivative", "integral", "matrix", "probability",
		"statistics", "algebra", "geometry", "calculus",
		"x =", "y =", "f(x)", "∫", "∑", "√",
	}

	for _, pattern := range mathPatterns {
		if strings.Contains(content, pattern) {
			return true
		}
	}

	return false
}

func (cr *CortexRouter) classifyIntentFromContent(content string) string {
	content = strings.ToLower(content)

	if cr.containsCodePatterns(content) {
		return "coding"
	}
	if cr.containsMathPatterns(content) {
		return "reasoning"
	}
	if cr.isSimpleGreeting(content) {
		return "chat"
	}

	// More sophisticated classification would go here
	return ""
}

func (cr *CortexRouter) mapIntentToModel(intent string) string {
	// Default model mapping based on intent
	mapping := map[string]string{
		"coding":    "claudecli:claude-sonnet-4",
		"reasoning": "geminicli:gemini-2.5-pro",
		"creative":  "claudecli:claude-sonnet-4",
		"factual":   "geminicli:gemini-2.5-pro",
		"chat":      "ollama:qwen:0.5b",
	}

	if model, exists := mapping[intent]; exists {
		return model
	}

	// Default fallback
	return "ollama:qwen:0.5b"
}

func (cr *CortexRouter) adjustConfidenceWithMemory(decision *RoutingDecision) float64 {
	if !cr.memoryEnabled {
		return decision.Confidence
	}

	// Get user preferences for learning-based adjustment
	prefs, err := cr.memoryManager.GetUserPreferences(decision.APIKeyHash)
	if err != nil || prefs == nil {
		return decision.Confidence
	}

	// Check if we have a learned preference for this intent
	hasPreference := false
	if decision.Intent != "" {
		_, hasPreference = prefs.ModelPreferences[decision.Intent]
	}

	// Get provider bias
	provider := cr.extractProvider(decision.SelectedModel)
	providerBias := 0.0
	if bias, exists := prefs.ProviderBias[provider]; exists {
		providerBias = bias
	}

	// Check time pattern match
	isTimeMatch := cr.isTimePatternMatch(decision, prefs)

	// Apply sophisticated learning-based confidence adjustment
	adjustedConfidence := learning.AdjustRuntimeConfidence(
		decision.Confidence,
		hasPreference,
		providerBias,
		isTimeMatch,
	)

	return adjustedConfidence
}

// isTimePatternMatch checks if the current time matches learned usage patterns
func (cr *CortexRouter) isTimePatternMatch(decision *RoutingDecision, prefs *memory.UserPreferences) bool {
	if decision.Intent == "" {
		return false
	}

	// This is a simplified implementation - in a full implementation,
	// we would check the time patterns from the learning system
	// For now, we'll use a basic heuristic based on time of day
	
	hour := decision.Timestamp.Hour()
	
	// Simple time-based patterns (could be enhanced with actual learned patterns)
	switch decision.Intent {
	case "coding":
		// Coding typically peaks during work hours
		return hour >= 9 && hour <= 17
	case "chat":
		// Chat can happen anytime, but peaks in evening
		return hour >= 18 && hour <= 22
	case "reasoning":
		// Complex reasoning typically during focused work hours
		return hour >= 10 && hour <= 16
	default:
		return false
	}
}

func (cr *CortexRouter) extractProvider(model string) string {
	parts := strings.Split(model, ":")
	if len(parts) > 0 {
		return parts[0]
	}
	return "unknown"
}

func (cr *CortexRouter) recordDecision(decision *RoutingDecision) {
	if !cr.memoryEnabled {
		return
	}

	// Convert to memory format for recording
	memoryDecision := &memory.RoutingDecision{
		Timestamp:  decision.Timestamp,
		APIKeyHash: decision.APIKeyHash,
		Request: memory.RequestInfo{
			Model:         decision.SelectedModel,
			Intent:        decision.Intent,
			ContentHash:   decision.RequestHash,
			ContentLength: 0, // Will be filled when outcome is recorded
		},
		Routing: memory.RoutingInfo{
			Tier:          decision.Tier,
			SelectedModel: decision.SelectedModel,
			Confidence:    decision.Confidence,
			LatencyMs:     decision.LatencyMs,
		},
		Outcome: memory.OutcomeInfo{
			Success: true, // Will be updated when outcome is recorded
		},
	}

	// Record synchronously for now (in production, this could be async)
	if err := cr.memoryManager.RecordRouting(memoryDecision); err != nil {
		log.Printf("Failed to record routing decision: %v", err)
	}

	// Publish routing decision event
	if cr.eventBus != nil {
		// Asynchronous publication to avoid strict dependency on hook execution time
		cr.eventBus.PublishAsync(&hooks.EventContext{
			Event:     hooks.EventRoutingDecision,
			Timestamp: decision.Timestamp,
			Data: map[string]interface{}{
				"decision": decision,
				"intent":   decision.Intent,
			},
			Provider: decision.Provider,
			Model:    decision.SelectedModel,
		})
	}
}

// Classification represents the result of LLM classification
type Classification struct {
	Intent     string  `json:"intent"`
	Complexity string  `json:"complexity"`
	Privacy    string  `json:"privacy"`
	Confidence float64 `json:"confidence"`
}

func (cr *CortexRouter) simulateLLMClassification(content string) Classification {
	// This is a simplified simulation of LLM classification
	// In the real implementation, this would call the actual LLM

	content = strings.ToLower(content)

	if cr.containsCodePatterns(content) {
		return Classification{
			Intent:     "coding",
			Complexity: "complex",
			Privacy:    "public",
			Confidence: 0.85,
		}
	}

	if cr.containsMathPatterns(content) {
		return Classification{
			Intent:     "reasoning",
			Complexity: "complex",
			Privacy:    "public",
			Confidence: 0.80,
		}
	}

	if cr.isSimpleGreeting(content) {
		return Classification{
			Intent:     "chat",
			Complexity: "simple",
			Privacy:    "public",
			Confidence: 0.95,
		}
	}

	// Default classification
	return Classification{
		Intent:     "factual",
		Complexity: "simple",
		Privacy:    "public",
		Confidence: 0.60,
	}
}

// IsEnabled returns whether the Cortex Router is enabled.
func (cr *CortexRouter) IsEnabled() bool {
	return cr.enabled
}

// IsMemoryEnabled returns whether memory integration is enabled.
func (cr *CortexRouter) IsMemoryEnabled() bool {
	return cr.memoryEnabled
}

// GetStats returns routing statistics.
func (cr *CortexRouter) GetStats() map[string]interface{} {
	stats := map[string]interface{}{
		"enabled":        cr.enabled,
		"memory_enabled": cr.memoryEnabled,
	}

	if cr.memoryEnabled {
		if memStats, err := cr.memoryManager.GetStats(); err == nil {
			stats["memory_stats"] = memStats
		}
	}

	return stats
}
