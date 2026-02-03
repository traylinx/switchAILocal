package heartbeat

import (
	"context"
	"fmt"
	"sync"
)

// RecoveryEventHandler implements HeartbeatEventHandler to trigger recovery actions.
type RecoveryEventHandler struct {
	mu sync.RWMutex

	// recoveryManager handles recovery actions
	recoveryManager *RecoveryManager

	// ctx is the context for recovery operations
	ctx context.Context
}

// NewRecoveryEventHandler creates a new recovery event handler.
func NewRecoveryEventHandler(ctx context.Context, config *RecoveryConfig) *RecoveryEventHandler {
	return &RecoveryEventHandler{
		recoveryManager: NewRecoveryManager(config),
		ctx:             ctx,
	}
}

// HandleEvent processes heartbeat events and triggers recovery actions.
func (h *RecoveryEventHandler) HandleEvent(event *HeartbeatEvent) error {
	if event == nil {
		return fmt.Errorf("event cannot be nil")
	}

	switch event.Type {
	case EventProviderHealthy:
		return h.handleProviderHealthy(event)

	case EventProviderDegraded:
		return h.handleProviderDegraded(event)

	case EventProviderUnavailable:
		return h.handleProviderUnavailable(event)

	case EventHealthCheckFailed:
		return h.handleHealthCheckFailed(event)

	case EventQuotaWarning:
		return h.handleQuotaWarning(event)

	case EventQuotaCritical:
		return h.handleQuotaCritical(event)

	case EventModelDiscovered:
		return h.handleModelDiscovered(event)

	case EventHeartbeatStarted, EventHeartbeatStopped:
		// No recovery action needed for these events
		return nil

	default:
		return fmt.Errorf("unknown event type: %s", event.Type)
	}
}

// handleProviderHealthy handles a provider becoming healthy.
func (h *RecoveryEventHandler) handleProviderHealthy(event *HeartbeatEvent) error {
	if event.Status == nil {
		return fmt.Errorf("event status is nil")
	}

	return h.recoveryManager.HandleProviderHealthy(h.ctx, event.Provider, event.Status)
}

// handleProviderDegraded handles a provider becoming degraded.
func (h *RecoveryEventHandler) handleProviderDegraded(event *HeartbeatEvent) error {
	if event.Status == nil {
		return fmt.Errorf("event status is nil")
	}

	return h.recoveryManager.HandleProviderDegraded(h.ctx, event.Provider, event.Status)
}

// handleProviderUnavailable handles a provider becoming unavailable.
func (h *RecoveryEventHandler) handleProviderUnavailable(event *HeartbeatEvent) error {
	if event.Status == nil {
		return fmt.Errorf("event status is nil")
	}

	return h.recoveryManager.HandleProviderUnavailable(h.ctx, event.Provider, event.Status)
}

// handleHealthCheckFailed handles a health check failure.
func (h *RecoveryEventHandler) handleHealthCheckFailed(event *HeartbeatEvent) error {
	// Health check failures are already handled by the unavailable event
	// This is just for logging/metrics
	return nil
}

// handleQuotaWarning handles a quota warning.
func (h *RecoveryEventHandler) handleQuotaWarning(event *HeartbeatEvent) error {
	// Log quota warning
	// In a real implementation, this might trigger notifications or throttling
	return nil
}

// handleQuotaCritical handles a critical quota threshold.
func (h *RecoveryEventHandler) handleQuotaCritical(event *HeartbeatEvent) error {
	// Log critical quota
	// In a real implementation, this might trigger emergency throttling or provider switching
	return nil
}

// handleModelDiscovered handles a new model discovery.
func (h *RecoveryEventHandler) handleModelDiscovered(event *HeartbeatEvent) error {
	// Log model discovery
	// In a real implementation, this might trigger model cache updates
	return nil
}

// GetRecoveryManager returns the recovery manager.
func (h *RecoveryEventHandler) GetRecoveryManager() *RecoveryManager {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.recoveryManager
}

// IsProviderDisabled returns whether a provider is currently disabled.
func (h *RecoveryEventHandler) IsProviderDisabled(provider string) bool {
	return h.recoveryManager.IsProviderDisabled(provider)
}

// IsFallbackEnabled returns whether fallback routing is enabled for a provider.
func (h *RecoveryEventHandler) IsFallbackEnabled(provider string) bool {
	return h.recoveryManager.IsFallbackEnabled(provider)
}

// GetRecoveryActions returns all recovery actions.
func (h *RecoveryEventHandler) GetRecoveryActions() []RecoveryAction {
	return h.recoveryManager.GetActions()
}

// GetRecoveryActionsForProvider returns recovery actions for a specific provider.
func (h *RecoveryEventHandler) GetRecoveryActionsForProvider(provider string) []RecoveryAction {
	return h.recoveryManager.GetActionsForProvider(provider)
}

// GetRecoveryStats returns recovery statistics.
func (h *RecoveryEventHandler) GetRecoveryStats() *RecoveryStats {
	return h.recoveryManager.GetStats()
}

// LoggingEventHandler implements HeartbeatEventHandler to log events.
type LoggingEventHandler struct {
	// In a real implementation, this would use a proper logger
	// For now, we'll just track events in memory
	mu     sync.RWMutex
	events []HeartbeatEvent
}

// NewLoggingEventHandler creates a new logging event handler.
func NewLoggingEventHandler() *LoggingEventHandler {
	return &LoggingEventHandler{
		events: make([]HeartbeatEvent, 0),
	}
}

// HandleEvent logs heartbeat events.
func (h *LoggingEventHandler) HandleEvent(event *HeartbeatEvent) error {
	if event == nil {
		return fmt.Errorf("event cannot be nil")
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	// Store event (keep last 1000 events)
	h.events = append(h.events, *event)
	if len(h.events) > 1000 {
		h.events = h.events[len(h.events)-1000:]
	}

	// In a real implementation, this would log to a proper logger
	// fmt.Printf("[HEARTBEAT] %s: %s\n", event.Type, event.Provider)

	return nil
}

// GetEvents returns all logged events.
func (h *LoggingEventHandler) GetEvents() []HeartbeatEvent {
	h.mu.RLock()
	defer h.mu.RUnlock()

	events := make([]HeartbeatEvent, len(h.events))
	copy(events, h.events)
	return events
}

// GetEventsForProvider returns events for a specific provider.
func (h *LoggingEventHandler) GetEventsForProvider(provider string) []HeartbeatEvent {
	h.mu.RLock()
	defer h.mu.RUnlock()

	events := make([]HeartbeatEvent, 0)
	for _, event := range h.events {
		if event.Provider == provider {
			events = append(events, event)
		}
	}
	return events
}

// GetEventsByType returns events of a specific type.
func (h *LoggingEventHandler) GetEventsByType(eventType HeartbeatEventType) []HeartbeatEvent {
	h.mu.RLock()
	defer h.mu.RUnlock()

	events := make([]HeartbeatEvent, 0)
	for _, event := range h.events {
		if event.Type == eventType {
			events = append(events, event)
		}
	}
	return events
}

// Clear clears all logged events.
func (h *LoggingEventHandler) Clear() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = make([]HeartbeatEvent, 0)
}
