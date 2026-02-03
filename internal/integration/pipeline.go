// Package integration provides coordination and lifecycle management for intelligent systems.
package integration

import (
	"fmt"

	"github.com/traylinx/switchAILocal/internal/hooks"
	"github.com/traylinx/switchAILocal/internal/memory"
	"github.com/traylinx/switchAILocal/internal/steering"

	log "github.com/sirupsen/logrus"
)

// RequestPipelineIntegrator integrates steering and memory into request processing.
// It applies steering rules before routing, records routing decisions to memory,
// and emits routing events to the event bus.
type RequestPipelineIntegrator struct {
	steering *steering.SteeringEngine
	memory   memory.MemoryManager
	eventBus *hooks.EventBus
}

// NewRequestPipelineIntegrator creates a new request pipeline integrator.
// All parameters are optional - if nil, the corresponding functionality is disabled.
func NewRequestPipelineIntegrator(
	steeringEngine *steering.SteeringEngine,
	memoryManager memory.MemoryManager,
	eventBus *hooks.EventBus,
) *RequestPipelineIntegrator {
	return &RequestPipelineIntegrator{
		steering: steeringEngine,
		memory:   memoryManager,
		eventBus: eventBus,
	}
}

// ApplySteering evaluates steering rules and modifies the request if rules match.
// It returns the selected model (or empty string if no override) and modified messages.
// If steering is disabled or evaluation fails, it returns empty model and original messages.
//
// Parameters:
//   - ctx: The routing context containing request metadata (should be *steering.RoutingContext)
//   - messages: The original request messages
//
// Returns:
//   - selectedModel: The model selected by steering rules (empty if no override)
//   - modifiedMessages: The messages after context injection (same as input if no injection)
//   - error: Any error that occurred during evaluation (non-fatal, caller should continue)
func (rpi *RequestPipelineIntegrator) ApplySteering(
	ctx interface{},
	messages []map[string]string,
) (string, []map[string]string, error) {
	// If steering is not available, return original values
	if rpi.steering == nil {
		log.Debug("Steering engine not available, skipping steering evaluation")
		return "", messages, nil
	}

	// Type assert the context to *steering.RoutingContext
	routingCtx, ok := ctx.(*steering.RoutingContext)
	if !ok {
		log.Warnf("Invalid context type for steering evaluation: %T", ctx)
		return "", messages, fmt.Errorf("invalid context type: expected *steering.RoutingContext, got %T", ctx)
	}

	// Find matching rules
	matchedRules, err := rpi.steering.FindMatchingRules(routingCtx)
	if err != nil {
		log.Warnf("Failed to find matching steering rules: %v", err)
		// Return original values on error (fail-safe behavior)
		return "", messages, fmt.Errorf("steering evaluation failed: %w", err)
	}

	// If no rules matched, return original values
	if len(matchedRules) == 0 {
		log.Debug("No steering rules matched for this request")
		return "", messages, nil
	}

	log.Debugf("Found %d matching steering rules", len(matchedRules))

	// Apply steering using the engine's ApplySteering method
	// This handles priority, time-based rules, context injection, etc.
	selectedModel, modifiedMessages, _ := rpi.steering.ApplySteering(routingCtx, messages, nil, matchedRules)

	return selectedModel, modifiedMessages, nil
}

// RecordRouting records a routing decision to the memory system.
// If memory is disabled or recording fails, it logs the error and continues.
// This method is non-blocking and fail-safe.
//
// Parameters:
//   - decision: The routing decision to record (should be *memory.RoutingDecision)
//
// Returns:
//   - error: Any error that occurred during recording (non-fatal, logged internally)
func (rpi *RequestPipelineIntegrator) RecordRouting(decision interface{}) error {
	// If memory is not available, skip recording
	if rpi.memory == nil {
		log.Debug("Memory manager not available, skipping routing decision recording")
		return nil
	}

	// Type assert the decision to *memory.RoutingDecision
	routingDecision, ok := decision.(*memory.RoutingDecision)
	if !ok {
		log.Warnf("Invalid decision type for memory recording: %T", decision)
		return fmt.Errorf("invalid decision type: expected *memory.RoutingDecision, got %T", decision)
	}

	// Record the decision
	if err := rpi.memory.RecordRouting(routingDecision); err != nil {
		log.Errorf("Failed to record routing decision: %v", err)
		return fmt.Errorf("memory recording failed: %w", err)
	}

	log.Debugf("Recorded routing decision for API key hash: %s", routingDecision.APIKeyHash)
	return nil
}

// UpdateOutcome updates a routing decision with its outcome.
// This should be called after the request completes (success or failure).
// If memory is disabled or update fails, it logs the error and continues.
// This method is non-blocking and fail-safe.
//
// Parameters:
//   - decision: The routing decision with updated outcome information (should be *memory.RoutingDecision)
//
// Returns:
//   - error: Any error that occurred during update (non-fatal, logged internally)
func (rpi *RequestPipelineIntegrator) UpdateOutcome(decision interface{}) error {
	// If memory is not available, skip update
	if rpi.memory == nil {
		log.Debug("Memory manager not available, skipping outcome update")
		return nil
	}

	// Type assert the decision to *memory.RoutingDecision
	routingDecision, ok := decision.(*memory.RoutingDecision)
	if !ok {
		log.Warnf("Invalid decision type for outcome update: %T", decision)
		return fmt.Errorf("invalid decision type: expected *memory.RoutingDecision, got %T", decision)
	}

	// Update the decision with outcome
	// Note: The memory manager's RecordRouting method handles both initial recording
	// and updates. For a more sophisticated implementation, you might want a separate
	// UpdateDecision method, but for now we'll use RecordRouting.
	if err := rpi.memory.RecordRouting(routingDecision); err != nil {
		log.Errorf("Failed to update routing decision outcome: %v", err)
		return fmt.Errorf("outcome update failed: %w", err)
	}

	log.Debugf("Updated routing decision outcome for API key hash: %s (success: %v)",
		routingDecision.APIKeyHash, routingDecision.Outcome.Success)
	return nil
}

// EmitRoutingEvent emits a routing decision event to the event bus.
// This allows hooks to react to routing decisions.
// If the event bus is not available or emission fails, it logs the error and continues.
// This method is non-blocking and fail-safe.
//
// Parameters:
//   - decision: The routing decision to emit as an event (should be *memory.RoutingDecision)
//
// Returns:
//   - error: Any error that occurred during emission (non-fatal, logged internally)
func (rpi *RequestPipelineIntegrator) EmitRoutingEvent(decision interface{}) error {
	// If event bus is not available, skip emission
	if rpi.eventBus == nil {
		log.Debug("Event bus not available, skipping routing event emission")
		return nil
	}

	// Type assert the decision to *memory.RoutingDecision
	routingDecision, ok := decision.(*memory.RoutingDecision)
	if !ok {
		log.Warnf("Invalid decision type for event emission: %T", decision)
		return fmt.Errorf("invalid decision type: expected *memory.RoutingDecision, got %T", decision)
	}

	// Create event context
	eventCtx := &hooks.EventContext{
		Event:     hooks.EventRoutingDecision,
		Timestamp: routingDecision.Timestamp,
		Data: map[string]interface{}{
			"api_key_hash":     routingDecision.APIKeyHash,
			"model":            routingDecision.Request.Model,
			"intent":           routingDecision.Request.Intent,
			"tier":             routingDecision.Routing.Tier,
			"selected_model":   routingDecision.Routing.SelectedModel,
			"confidence":       routingDecision.Routing.Confidence,
			"latency_ms":       routingDecision.Routing.LatencyMs,
			"success":          routingDecision.Outcome.Success,
			"response_time_ms": routingDecision.Outcome.ResponseTimeMs,
			"quality_score":    routingDecision.Outcome.QualityScore,
		},
		Model:    routingDecision.Routing.SelectedModel,
		Provider: extractProvider(routingDecision.Routing.SelectedModel),
	}

	// Add error message if present
	if routingDecision.Outcome.Error != "" {
		eventCtx.ErrorMessage = routingDecision.Outcome.Error
	}

	// Emit the event asynchronously
	rpi.eventBus.PublishAsync(eventCtx)

	log.Debugf("Emitted routing decision event for model: %s", routingDecision.Routing.SelectedModel)
	return nil
}

// extractProvider extracts the provider name from a model string.
// Model strings are typically in the format "provider:model" (e.g., "claudecli:claude-sonnet-4").
// If no colon is present, returns the entire string.
func extractProvider(model string) string {
	for i, c := range model {
		if c == ':' {
			return model[:i]
		}
	}
	return model
}
