// Package integration provides coordination and lifecycle management for intelligent systems.
package integration

import (
	"time"

	"github.com/traylinx/switchAILocal/internal/memory"
	"github.com/traylinx/switchAILocal/internal/steering"
)

// HandlerAdapter adapts RequestPipelineIntegrator to the interface expected by BaseAPIHandler.
// This avoids circular dependencies between sdk/api/handlers and internal/integration.
type HandlerAdapter struct {
	integrator *RequestPipelineIntegrator
}

// NewHandlerAdapter creates a new handler adapter.
func NewHandlerAdapter(integrator *RequestPipelineIntegrator) *HandlerAdapter {
	return &HandlerAdapter{
		integrator: integrator,
	}
}

// ApplySteering evaluates steering rules and modifies the request if rules match.
// The ctx parameter should be a *steering.RoutingContext.
func (ha *HandlerAdapter) ApplySteering(ctx interface{}, messages []map[string]string) (string, []map[string]string, error) {
	if ha.integrator == nil {
		return "", messages, nil
	}
	
	routingCtx, ok := ctx.(*steering.RoutingContext)
	if !ok {
		return "", messages, nil
	}
	
	return ha.integrator.ApplySteering(routingCtx, messages)
}

// RecordRouting records a routing decision to the memory system.
// The decision parameter can be either a *memory.RoutingDecision or a map[string]interface{}.
func (ha *HandlerAdapter) RecordRouting(decision interface{}) error {
	if ha.integrator == nil {
		return nil
	}
	
	// Handle *memory.RoutingDecision directly
	if routingDecision, ok := decision.(*memory.RoutingDecision); ok {
		return ha.integrator.RecordRouting(routingDecision)
	}
	
	// Handle map[string]interface{} by converting to RoutingDecision
	if decisionMap, ok := decision.(map[string]interface{}); ok {
		routingDecision := ha.mapToRoutingDecision(decisionMap)
		return ha.integrator.RecordRouting(routingDecision)
	}
	
	return nil
}

// UpdateOutcome updates a routing decision with its outcome.
// The decision parameter can be either a *memory.RoutingDecision or a map[string]interface{}.
func (ha *HandlerAdapter) UpdateOutcome(decision interface{}) error {
	if ha.integrator == nil {
		return nil
	}
	
	// Handle *memory.RoutingDecision directly
	if routingDecision, ok := decision.(*memory.RoutingDecision); ok {
		return ha.integrator.UpdateOutcome(routingDecision)
	}
	
	// Handle map[string]interface{} by converting to RoutingDecision
	if decisionMap, ok := decision.(map[string]interface{}); ok {
		routingDecision := ha.mapToRoutingDecision(decisionMap)
		return ha.integrator.UpdateOutcome(routingDecision)
	}
	
	return nil
}

// EmitRoutingEvent emits a routing decision event to the event bus.
// The decision parameter can be either a *memory.RoutingDecision or a map[string]interface{}.
func (ha *HandlerAdapter) EmitRoutingEvent(decision interface{}) error {
	if ha.integrator == nil {
		return nil
	}
	
	// Handle *memory.RoutingDecision directly
	if routingDecision, ok := decision.(*memory.RoutingDecision); ok {
		return ha.integrator.EmitRoutingEvent(routingDecision)
	}
	
	// Handle map[string]interface{} by converting to RoutingDecision
	if decisionMap, ok := decision.(map[string]interface{}); ok {
		routingDecision := ha.mapToRoutingDecision(decisionMap)
		return ha.integrator.EmitRoutingEvent(routingDecision)
	}
	
	return nil
}

// mapToRoutingDecision converts a map[string]interface{} to a *memory.RoutingDecision.
func (ha *HandlerAdapter) mapToRoutingDecision(m map[string]interface{}) *memory.RoutingDecision {
	decision := &memory.RoutingDecision{
		Timestamp: time.Now(),
	}
	
	// Extract API key hash
	if apiKeyHash, ok := m["api_key_hash"].(string); ok {
		decision.APIKeyHash = apiKeyHash
	}
	
	// Extract timestamp
	if timestamp, ok := m["timestamp"].(time.Time); ok {
		decision.Timestamp = timestamp
	}
	
	// Extract request info
	if requestMap, ok := m["request"].(map[string]interface{}); ok {
		if model, ok := requestMap["model"].(string); ok {
			decision.Request.Model = model
		}
		if intent, ok := requestMap["intent"].(string); ok {
			decision.Request.Intent = intent
		}
		if contentHash, ok := requestMap["content_hash"].(string); ok {
			decision.Request.ContentHash = contentHash
		}
		if contentLength, ok := requestMap["content_length"].(int); ok {
			decision.Request.ContentLength = contentLength
		}
	}
	
	// Extract routing info
	if routingMap, ok := m["routing"].(map[string]interface{}); ok {
		if selectedModel, ok := routingMap["selected_model"].(string); ok {
			decision.Routing.SelectedModel = selectedModel
		}
		if tier, ok := routingMap["tier"].(string); ok {
			decision.Routing.Tier = tier
		}
		if confidence, ok := routingMap["confidence"].(float64); ok {
			decision.Routing.Confidence = confidence
		}
		if latencyMs, ok := routingMap["latency_ms"].(int64); ok {
			decision.Routing.LatencyMs = latencyMs
		}
	}
	
	// Extract outcome info
	if outcomeMap, ok := m["outcome"].(map[string]interface{}); ok {
		if success, ok := outcomeMap["success"].(bool); ok {
			decision.Outcome.Success = success
		}
		if responseTimeMs, ok := outcomeMap["response_time_ms"].(int64); ok {
			decision.Outcome.ResponseTimeMs = responseTimeMs
		}
		if errorMsg, ok := outcomeMap["error"].(string); ok {
			decision.Outcome.Error = errorMsg
		}
		if qualityScore, ok := outcomeMap["quality_score"].(float64); ok {
			decision.Outcome.QualityScore = qualityScore
		}
	}
	
	return decision
}

// RoutingDecisionBuilder helps build routing decisions from handler context.
type RoutingDecisionBuilder struct {
	decision *memory.RoutingDecision
}

// NewRoutingDecisionBuilder creates a new routing decision builder.
func NewRoutingDecisionBuilder() *RoutingDecisionBuilder {
	return &RoutingDecisionBuilder{
		decision: &memory.RoutingDecision{
			Timestamp: time.Now(),
			Request:   memory.RequestInfo{},
			Routing:   memory.RoutingInfo{},
			Outcome:   memory.OutcomeInfo{},
		},
	}
}

// WithAPIKeyHash sets the API key hash.
func (b *RoutingDecisionBuilder) WithAPIKeyHash(hash string) *RoutingDecisionBuilder {
	b.decision.APIKeyHash = hash
	return b
}

// WithModel sets the requested model.
func (b *RoutingDecisionBuilder) WithModel(model string) *RoutingDecisionBuilder {
	b.decision.Request.Model = model
	return b
}

// WithIntent sets the request intent.
func (b *RoutingDecisionBuilder) WithIntent(intent string) *RoutingDecisionBuilder {
	b.decision.Request.Intent = intent
	return b
}

// WithContentHash sets the content hash.
func (b *RoutingDecisionBuilder) WithContentHash(hash string) *RoutingDecisionBuilder {
	b.decision.Request.ContentHash = hash
	return b
}

// WithContentLength sets the content length.
func (b *RoutingDecisionBuilder) WithContentLength(length int) *RoutingDecisionBuilder {
	b.decision.Request.ContentLength = length
	return b
}

// WithTier sets the routing tier.
func (b *RoutingDecisionBuilder) WithTier(tier string) *RoutingDecisionBuilder {
	b.decision.Routing.Tier = tier
	return b
}

// WithSelectedModel sets the selected model.
func (b *RoutingDecisionBuilder) WithSelectedModel(model string) *RoutingDecisionBuilder {
	b.decision.Routing.SelectedModel = model
	return b
}

// WithConfidence sets the routing confidence.
func (b *RoutingDecisionBuilder) WithConfidence(confidence float64) *RoutingDecisionBuilder {
	b.decision.Routing.Confidence = confidence
	return b
}

// WithLatency sets the routing latency in milliseconds.
func (b *RoutingDecisionBuilder) WithLatency(latencyMs int64) *RoutingDecisionBuilder {
	b.decision.Routing.LatencyMs = latencyMs
	return b
}

// WithSuccess sets the outcome success status.
func (b *RoutingDecisionBuilder) WithSuccess(success bool) *RoutingDecisionBuilder {
	b.decision.Outcome.Success = success
	return b
}

// WithResponseTime sets the response time in milliseconds.
func (b *RoutingDecisionBuilder) WithResponseTime(responseTimeMs int64) *RoutingDecisionBuilder {
	b.decision.Outcome.ResponseTimeMs = responseTimeMs
	return b
}

// WithError sets the error message.
func (b *RoutingDecisionBuilder) WithError(err string) *RoutingDecisionBuilder {
	b.decision.Outcome.Error = err
	return b
}

// WithQualityScore sets the quality score.
func (b *RoutingDecisionBuilder) WithQualityScore(score float64) *RoutingDecisionBuilder {
	b.decision.Outcome.QualityScore = score
	return b
}

// Build returns the constructed routing decision.
func (b *RoutingDecisionBuilder) Build() *memory.RoutingDecision {
	return b.decision
}
