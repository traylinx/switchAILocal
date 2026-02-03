// Package integration provides coordination and lifecycle management for intelligent systems.
package integration

import (
	"fmt"
	"time"

	"github.com/traylinx/switchAILocal/internal/heartbeat"
	"github.com/traylinx/switchAILocal/internal/hooks"
	"github.com/traylinx/switchAILocal/internal/memory"

	log "github.com/sirupsen/logrus"
)

// EventBusIntegrator connects system events to the hooks event bus.
// It bridges heartbeat events, routing events, and provider events to the hooks system,
// allowing hooks to react to these events.
type EventBusIntegrator struct {
	eventBus  *hooks.EventBus
	hooks     *hooks.HookManager
	heartbeat heartbeat.HeartbeatMonitor
}

// NewEventBusIntegrator creates a new event bus integrator.
// All parameters are optional - if nil, the corresponding functionality is disabled.
func NewEventBusIntegrator(
	eventBus *hooks.EventBus,
	hooksMgr *hooks.HookManager,
	hbMonitor heartbeat.HeartbeatMonitor,
) *EventBusIntegrator {
	return &EventBusIntegrator{
		eventBus:  eventBus,
		hooks:     hooksMgr,
		heartbeat: hbMonitor,
	}
}

// ConnectHeartbeatEvents connects heartbeat monitor events to the hooks event bus.
// It registers a heartbeat event handler that translates heartbeat events to hook events.
// Returns an error if the heartbeat monitor or event bus is not available.
func (ebi *EventBusIntegrator) ConnectHeartbeatEvents() error {
	if ebi.heartbeat == nil {
		log.Debug("Heartbeat monitor not available, skipping heartbeat event connection")
		return nil
	}

	if ebi.eventBus == nil {
		return fmt.Errorf("event bus is required to connect heartbeat events")
	}

	// Create a heartbeat event handler that bridges to the hooks event bus
	handler := &heartbeatEventBridge{
		eventBus: ebi.eventBus,
	}

	// Register the handler with the heartbeat monitor
	ebi.heartbeat.AddEventHandler(handler)

	log.Info("Connected heartbeat events to hooks event bus")
	return nil
}

// ConnectRoutingEvents connects routing decision events to the hooks event bus.
// This is a no-op since routing events are emitted directly by the RequestPipelineIntegrator.
// This method exists for API completeness and future extensibility.
func (ebi *EventBusIntegrator) ConnectRoutingEvents() error {
	if ebi.eventBus == nil {
		log.Debug("Event bus not available, skipping routing event connection")
		return nil
	}

	// Routing events are emitted directly by RequestPipelineIntegrator.EmitRoutingEvent()
	// No additional connection is needed here.
	log.Debug("Routing events are connected via RequestPipelineIntegrator")
	return nil
}

// ConnectProviderEvents connects provider failure events to the hooks event bus.
// This is a no-op since provider events are emitted through heartbeat events.
// This method exists for API completeness and future extensibility.
func (ebi *EventBusIntegrator) ConnectProviderEvents() error {
	if ebi.eventBus == nil {
		log.Debug("Event bus not available, skipping provider event connection")
		return nil
	}

	// Provider events (unavailable, degraded) are emitted through heartbeat events
	// No additional connection is needed here.
	log.Debug("Provider events are connected via heartbeat event bridge")
	return nil
}

// EmitEvent emits a custom event to the hooks event bus.
// This is a convenience method for emitting events from other parts of the system.
// If the event bus is not available, it logs a debug message and returns nil.
func (ebi *EventBusIntegrator) EmitEvent(event *hooks.EventContext) error {
	if ebi.eventBus == nil {
		log.Debug("Event bus not available, skipping event emission")
		return nil
	}

	if event == nil {
		return fmt.Errorf("event context cannot be nil")
	}

	// Emit the event asynchronously
	ebi.eventBus.PublishAsync(event)

	log.Debugf("Emitted custom event: %s", event.Event)
	return nil
}

// heartbeatEventBridge implements heartbeat.HeartbeatEventHandler to bridge
// heartbeat events to the hooks event bus.
type heartbeatEventBridge struct {
	eventBus *hooks.EventBus
}

// HandleEvent processes heartbeat events and translates them to hook events.
func (heb *heartbeatEventBridge) HandleEvent(event *heartbeat.HeartbeatEvent) error {
	if event == nil {
		return fmt.Errorf("heartbeat event cannot be nil")
	}

	// Translate heartbeat event to hook event
	hookEvent := heb.translateEvent(event)
	if hookEvent == nil {
		// Event type not mapped to hooks, skip
		return nil
	}

	// Emit to event bus asynchronously
	heb.eventBus.PublishAsync(hookEvent)

	return nil
}

// translateEvent translates a heartbeat event to a hook event context.
// Returns nil if the event type should not be forwarded to hooks.
func (heb *heartbeatEventBridge) translateEvent(hbEvent *heartbeat.HeartbeatEvent) *hooks.EventContext {
	// Map heartbeat event types to hook event types
	var hookEventType hooks.HookEvent
	var includeEvent bool

	switch hbEvent.Type {
	case heartbeat.EventProviderUnavailable:
		hookEventType = hooks.EventProviderUnavailable
		includeEvent = true

	case heartbeat.EventHealthCheckFailed:
		hookEventType = hooks.EventHealthCheckFailed
		includeEvent = true

	case heartbeat.EventQuotaWarning:
		hookEventType = hooks.EventQuotaWarning
		includeEvent = true

	case heartbeat.EventQuotaCritical:
		hookEventType = hooks.EventQuotaExceeded
		includeEvent = true

	case heartbeat.EventModelDiscovered:
		hookEventType = hooks.EventModelDiscovered
		includeEvent = true

	case heartbeat.EventProviderHealthy, heartbeat.EventProviderDegraded:
		// These events are informational and typically don't trigger hooks
		// But we'll include them for completeness
		includeEvent = false

	case heartbeat.EventHeartbeatStarted, heartbeat.EventHeartbeatStopped:
		// System lifecycle events, not forwarded to hooks
		includeEvent = false

	default:
		// Unknown event type, skip
		return nil
	}

	if !includeEvent {
		return nil
	}

	// Create hook event context
	eventCtx := &hooks.EventContext{
		Event:     hookEventType,
		Timestamp: hbEvent.Timestamp,
		Provider:  hbEvent.Provider,
		Data:      make(map[string]interface{}),
	}

	// Copy data from heartbeat event
	if hbEvent.Data != nil {
		for k, v := range hbEvent.Data {
			eventCtx.Data[k] = v
		}
	}

	// Add status information if available
	if hbEvent.Status != nil {
		eventCtx.Data["status"] = string(hbEvent.Status.Status)
		eventCtx.Data["last_check"] = hbEvent.Status.LastCheck
		eventCtx.Data["response_time"] = hbEvent.Status.ResponseTime
		eventCtx.Data["models_count"] = hbEvent.Status.ModelsCount
		eventCtx.Data["quota_used"] = hbEvent.Status.QuotaUsed
		eventCtx.Data["quota_limit"] = hbEvent.Status.QuotaLimit

		if hbEvent.Status.ErrorMessage != "" {
			eventCtx.ErrorMessage = hbEvent.Status.ErrorMessage
		}
	}

	// Add previous status information if available
	if hbEvent.PreviousStatus != nil {
		eventCtx.Data["previous_status"] = string(hbEvent.PreviousStatus.Status)
	}

	return eventCtx
}

// EmitRequestFailedEvent is a convenience method to emit a request failed event.
// This can be called from API handlers when a request fails.
func (ebi *EventBusIntegrator) EmitRequestFailedEvent(
	provider string,
	model string,
	errorMsg string,
	decision *memory.RoutingDecision,
) error {
	if ebi.eventBus == nil {
		log.Debug("Event bus not available, skipping request failed event")
		return nil
	}

	eventCtx := &hooks.EventContext{
		Event:        hooks.EventRequestFailed,
		Timestamp:    time.Now(),
		Provider:     provider,
		Model:        model,
		ErrorMessage: errorMsg,
		Data:         make(map[string]interface{}),
	}

	// Add routing decision data if available
	if decision != nil {
		eventCtx.Data["api_key_hash"] = decision.APIKeyHash
		eventCtx.Data["intent"] = decision.Request.Intent
		eventCtx.Data["tier"] = decision.Routing.Tier
		eventCtx.Data["selected_model"] = decision.Routing.SelectedModel
		eventCtx.Data["confidence"] = decision.Routing.Confidence
		eventCtx.Data["latency_ms"] = decision.Routing.LatencyMs
	}

	// Emit the event asynchronously
	ebi.eventBus.PublishAsync(eventCtx)

	log.Debugf("Emitted request failed event for provider: %s, model: %s", provider, model)
	return nil
}

// EmitRequestReceivedEvent is a convenience method to emit a request received event.
// This can be called from API handlers when a request is received.
func (ebi *EventBusIntegrator) EmitRequestReceivedEvent(
	provider string,
	model string,
	apiKeyHash string,
) error {
	if ebi.eventBus == nil {
		log.Debug("Event bus not available, skipping request received event")
		return nil
	}

	eventCtx := &hooks.EventContext{
		Event:     hooks.EventRequestReceived,
		Timestamp: time.Now(),
		Provider:  provider,
		Model:     model,
		Data: map[string]interface{}{
			"api_key_hash": apiKeyHash,
		},
	}

	// Emit the event asynchronously
	ebi.eventBus.PublishAsync(eventCtx)

	log.Debugf("Emitted request received event for provider: %s, model: %s", provider, model)
	return nil
}
