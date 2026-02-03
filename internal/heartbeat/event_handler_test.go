package heartbeat

import (
	"context"
	"testing"
	"time"
)

func TestRecoveryEventHandler_HandleProviderHealthy(t *testing.T) {
	ctx := context.Background()
	config := DefaultRecoveryConfig()
	handler := NewRecoveryEventHandler(ctx, config)

	event := &HeartbeatEvent{
		Type:      EventProviderHealthy,
		Provider:  "test-provider",
		Timestamp: time.Now(),
		Status: &HealthStatus{
			Provider: "test-provider",
			Status:   StatusHealthy,
		},
	}

	err := handler.HandleEvent(event)
	if err != nil {
		t.Fatalf("HandleEvent failed: %v", err)
	}

	// Check that recovery attempts were reset
	if handler.GetRecoveryManager().GetRecoveryAttempts("test-provider") != 0 {
		t.Error("Recovery attempts should be reset for healthy provider")
	}
}

func TestRecoveryEventHandler_HandleProviderUnavailable(t *testing.T) {
	ctx := context.Background()
	config := DefaultRecoveryConfig()
	handler := NewRecoveryEventHandler(ctx, config)

	event := &HeartbeatEvent{
		Type:      EventProviderUnavailable,
		Provider:  "test-provider",
		Timestamp: time.Now(),
		Status: &HealthStatus{
			Provider:     "test-provider",
			Status:       StatusUnavailable,
			ErrorMessage: "connection refused",
		},
	}

	err := handler.HandleEvent(event)
	if err != nil {
		t.Fatalf("HandleEvent failed: %v", err)
	}

	// Check that fallback was enabled
	if !handler.IsFallbackEnabled("test-provider") {
		t.Error("Fallback should be enabled for unavailable provider")
	}

	// Check that recovery actions were recorded
	actions := handler.GetRecoveryActionsForProvider("test-provider")
	if len(actions) == 0 {
		t.Error("Expected recovery actions to be recorded")
	}
}

func TestRecoveryEventHandler_HandleProviderDegraded(t *testing.T) {
	ctx := context.Background()
	config := DefaultRecoveryConfig()
	handler := NewRecoveryEventHandler(ctx, config)

	event := &HeartbeatEvent{
		Type:      EventProviderDegraded,
		Provider:  "test-provider",
		Timestamp: time.Now(),
		Status: &HealthStatus{
			Provider:     "test-provider",
			Status:       StatusDegraded,
			ErrorMessage: "slow response",
		},
	}

	err := handler.HandleEvent(event)
	if err != nil {
		t.Fatalf("HandleEvent failed: %v", err)
	}

	// Check that fallback was enabled
	if !handler.IsFallbackEnabled("test-provider") {
		t.Error("Fallback should be enabled for degraded provider")
	}
}

func TestRecoveryEventHandler_HandleNilEvent(t *testing.T) {
	ctx := context.Background()
	config := DefaultRecoveryConfig()
	handler := NewRecoveryEventHandler(ctx, config)

	err := handler.HandleEvent(nil)
	if err == nil {
		t.Error("Expected error for nil event")
	}
}

func TestRecoveryEventHandler_HandleNilStatus(t *testing.T) {
	ctx := context.Background()
	config := DefaultRecoveryConfig()
	handler := NewRecoveryEventHandler(ctx, config)

	event := &HeartbeatEvent{
		Type:      EventProviderUnavailable,
		Provider:  "test-provider",
		Timestamp: time.Now(),
		Status:    nil, // Nil status
	}

	err := handler.HandleEvent(event)
	if err == nil {
		t.Error("Expected error for nil status")
	}
}

func TestRecoveryEventHandler_HandleUnknownEventType(t *testing.T) {
	ctx := context.Background()
	config := DefaultRecoveryConfig()
	handler := NewRecoveryEventHandler(ctx, config)

	event := &HeartbeatEvent{
		Type:      HeartbeatEventType("unknown"),
		Provider:  "test-provider",
		Timestamp: time.Now(),
	}

	err := handler.HandleEvent(event)
	if err == nil {
		t.Error("Expected error for unknown event type")
	}
}

func TestRecoveryEventHandler_HandleHeartbeatStarted(t *testing.T) {
	ctx := context.Background()
	config := DefaultRecoveryConfig()
	handler := NewRecoveryEventHandler(ctx, config)

	event := &HeartbeatEvent{
		Type:      EventHeartbeatStarted,
		Timestamp: time.Now(),
	}

	err := handler.HandleEvent(event)
	if err != nil {
		t.Fatalf("HandleEvent failed: %v", err)
	}

	// No recovery actions should be taken for heartbeat started
	actions := handler.GetRecoveryActions()
	if len(actions) != 0 {
		t.Error("No recovery actions should be taken for heartbeat started event")
	}
}

func TestRecoveryEventHandler_GetRecoveryStats(t *testing.T) {
	ctx := context.Background()
	config := DefaultRecoveryConfig()
	handler := NewRecoveryEventHandler(ctx, config)

	// Trigger some events
	event1 := &HeartbeatEvent{
		Type:      EventProviderUnavailable,
		Provider:  "provider1",
		Timestamp: time.Now(),
		Status: &HealthStatus{
			Provider:     "provider1",
			Status:       StatusUnavailable,
			ErrorMessage: "connection refused",
		},
	}

	event2 := &HeartbeatEvent{
		Type:      EventProviderDegraded,
		Provider:  "provider2",
		Timestamp: time.Now(),
		Status: &HealthStatus{
			Provider:     "provider2",
			Status:       StatusDegraded,
			ErrorMessage: "slow response",
		},
	}

	_ = handler.HandleEvent(event1)
	_ = handler.HandleEvent(event2)

	// Get stats
	stats := handler.GetRecoveryStats()

	if stats.TotalActions == 0 {
		t.Error("Expected some actions to be recorded")
	}

	if len(stats.ActionsByProvider) != 2 {
		t.Errorf("Expected 2 providers in stats, got %d", len(stats.ActionsByProvider))
	}
}

// --- LoggingEventHandler Tests ---

func TestLoggingEventHandler_HandleEvent(t *testing.T) {
	handler := NewLoggingEventHandler()

	event := &HeartbeatEvent{
		Type:      EventProviderHealthy,
		Provider:  "test-provider",
		Timestamp: time.Now(),
		Status: &HealthStatus{
			Provider: "test-provider",
			Status:   StatusHealthy,
		},
	}

	err := handler.HandleEvent(event)
	if err != nil {
		t.Fatalf("HandleEvent failed: %v", err)
	}

	// Check that event was logged
	events := handler.GetEvents()
	if len(events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(events))
	}

	if events[0].Type != EventProviderHealthy {
		t.Errorf("Expected EventProviderHealthy, got %s", events[0].Type)
	}
}

func TestLoggingEventHandler_GetEventsForProvider(t *testing.T) {
	handler := NewLoggingEventHandler()

	// Log events for multiple providers
	event1 := &HeartbeatEvent{
		Type:      EventProviderHealthy,
		Provider:  "provider1",
		Timestamp: time.Now(),
	}

	event2 := &HeartbeatEvent{
		Type:      EventProviderUnavailable,
		Provider:  "provider2",
		Timestamp: time.Now(),
	}

	event3 := &HeartbeatEvent{
		Type:      EventProviderDegraded,
		Provider:  "provider1",
		Timestamp: time.Now(),
	}

	_ = handler.HandleEvent(event1)
	_ = handler.HandleEvent(event2)
	_ = handler.HandleEvent(event3)

	// Get events for provider1
	events := handler.GetEventsForProvider("provider1")
	if len(events) != 2 {
		t.Errorf("Expected 2 events for provider1, got %d", len(events))
	}

	// Get events for provider2
	events = handler.GetEventsForProvider("provider2")
	if len(events) != 1 {
		t.Errorf("Expected 1 event for provider2, got %d", len(events))
	}
}

func TestLoggingEventHandler_GetEventsByType(t *testing.T) {
	handler := NewLoggingEventHandler()

	// Log events of different types
	event1 := &HeartbeatEvent{
		Type:      EventProviderHealthy,
		Provider:  "provider1",
		Timestamp: time.Now(),
	}

	event2 := &HeartbeatEvent{
		Type:      EventProviderHealthy,
		Provider:  "provider2",
		Timestamp: time.Now(),
	}

	event3 := &HeartbeatEvent{
		Type:      EventProviderUnavailable,
		Provider:  "provider3",
		Timestamp: time.Now(),
	}

	_ = handler.HandleEvent(event1)
	_ = handler.HandleEvent(event2)
	_ = handler.HandleEvent(event3)

	// Get healthy events
	events := handler.GetEventsByType(EventProviderHealthy)
	if len(events) != 2 {
		t.Errorf("Expected 2 healthy events, got %d", len(events))
	}

	// Get unavailable events
	events = handler.GetEventsByType(EventProviderUnavailable)
	if len(events) != 1 {
		t.Errorf("Expected 1 unavailable event, got %d", len(events))
	}
}

func TestLoggingEventHandler_Clear(t *testing.T) {
	handler := NewLoggingEventHandler()

	// Log some events
	event := &HeartbeatEvent{
		Type:      EventProviderHealthy,
		Provider:  "test-provider",
		Timestamp: time.Now(),
	}

	_ = handler.HandleEvent(event)
	_ = handler.HandleEvent(event)

	// Check events were logged
	if len(handler.GetEvents()) != 2 {
		t.Error("Expected 2 events before clear")
	}

	// Clear events
	handler.Clear()

	// Check events were cleared
	if len(handler.GetEvents()) != 0 {
		t.Error("Expected 0 events after clear")
	}
}

func TestLoggingEventHandler_HandleNilEvent(t *testing.T) {
	handler := NewLoggingEventHandler()

	err := handler.HandleEvent(nil)
	if err == nil {
		t.Error("Expected error for nil event")
	}
}

func TestLoggingEventHandler_MaxEvents(t *testing.T) {
	handler := NewLoggingEventHandler()

	// Log more than 1000 events
	for i := 0; i < 1500; i++ {
		event := &HeartbeatEvent{
			Type:      EventProviderHealthy,
			Provider:  "test-provider",
			Timestamp: time.Now(),
		}
		_ = handler.HandleEvent(event)
	}

	// Check that only last 1000 events are kept
	events := handler.GetEvents()
	if len(events) != 1000 {
		t.Errorf("Expected 1000 events (max), got %d", len(events))
	}
}

// --- Integration Tests ---

func TestHeartbeatMonitor_WithRecoveryHandler(t *testing.T) {
	config := &HeartbeatConfig{
		Enabled:                true,
		Interval:               100 * time.Millisecond,
		Timeout:                time.Second,
		AutoDiscovery:          false,
		QuotaWarningThreshold:  0.80,
		QuotaCriticalThreshold: 0.95,
		MaxConcurrentChecks:    5,
		RetryAttempts:          1,
		RetryDelay:             10 * time.Millisecond,
	}

	monitor := NewHeartbeatMonitor(config).(*HeartbeatMonitorImpl)

	// Create recovery handler
	ctx := context.Background()
	recoveryConfig := DefaultRecoveryConfig()
	recoveryHandler := NewRecoveryEventHandler(ctx, recoveryConfig)

	// Register recovery handler
	monitor.AddEventHandler(recoveryHandler)

	// Create a mock checker that fails
	checker := NewMockHealthChecker("test-provider")
	checker.SetStatus(StatusUnavailable)

	_ = monitor.RegisterChecker(checker)

	// Perform manual check
	status, err := monitor.CheckProvider(ctx, "test-provider")
	if err != nil {
		// Error is expected, but status should still be recorded
		t.Logf("CheckProvider returned error (expected): %v", err)
	}

	// Check that status was recorded as unavailable
	if status != nil && status.Status != StatusUnavailable {
		t.Errorf("Expected unavailable status, got %s", status.Status)
	}

	// Give event handlers time to process
	time.Sleep(50 * time.Millisecond)

	// Check that recovery actions were taken
	if !recoveryHandler.IsFallbackEnabled("test-provider") {
		t.Error("Fallback should be enabled for unavailable provider")
	}

	actions := recoveryHandler.GetRecoveryActionsForProvider("test-provider")
	if len(actions) == 0 {
		t.Error("Expected recovery actions to be recorded")
	}
}

func BenchmarkRecoveryEventHandler_HandleEvent(b *testing.B) {
	ctx := context.Background()
	config := DefaultRecoveryConfig()
	config.RecoveryBackoff = 0 // Disable backoff for benchmarking
	config.MaxRecoveryAttempts = 1000000

	handler := NewRecoveryEventHandler(ctx, config)

	event := &HeartbeatEvent{
		Type:      EventProviderUnavailable,
		Provider:  "test-provider",
		Timestamp: time.Now(),
		Status: &HealthStatus{
			Provider:     "test-provider",
			Status:       StatusUnavailable,
			ErrorMessage: "connection refused",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = handler.HandleEvent(event)
	}
}

func BenchmarkLoggingEventHandler_HandleEvent(b *testing.B) {
	handler := NewLoggingEventHandler()

	event := &HeartbeatEvent{
		Type:      EventProviderHealthy,
		Provider:  "test-provider",
		Timestamp: time.Now(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = handler.HandleEvent(event)
	}
}
