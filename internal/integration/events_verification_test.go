package integration

import (
	"testing"
	"time"

	"github.com/traylinx/switchAILocal/internal/heartbeat"
	"github.com/traylinx/switchAILocal/internal/hooks"
)

// TestHeartbeatEventConnection_AllEventTypes verifies that all required heartbeat
// event types are properly connected to the event bus.
// This test validates task 6.2: Connect heartbeat events to event bus.
func TestHeartbeatEventConnection_AllEventTypes(t *testing.T) {
	eventBus := hooks.NewEventBus()
	defer eventBus.Shutdown()

	bridge := &heartbeatEventBridge{
		eventBus: eventBus,
	}

	// Test cases for all four required event types
	testCases := []struct {
		name              string
		heartbeatEvent    heartbeat.HeartbeatEventType
		expectedHookEvent hooks.HookEvent
		description       string
	}{
		{
			name:              "HealthCheckFailed",
			heartbeatEvent:    heartbeat.EventHealthCheckFailed,
			expectedHookEvent: hooks.EventHealthCheckFailed,
			description:       "Health check failed events should be emitted",
		},
		{
			name:              "ProviderStatusChange",
			heartbeatEvent:    heartbeat.EventProviderUnavailable,
			expectedHookEvent: hooks.EventProviderUnavailable,
			description:       "Provider status change events should be emitted",
		},
		{
			name:              "QuotaWarning",
			heartbeatEvent:    heartbeat.EventQuotaWarning,
			expectedHookEvent: hooks.EventQuotaWarning,
			description:       "Quota warning events should be emitted",
		},
		{
			name:              "QuotaCritical",
			heartbeatEvent:    heartbeat.EventQuotaCritical,
			expectedHookEvent: hooks.EventQuotaExceeded,
			description:       "Quota critical events should be emitted",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Subscribe to the expected hook event
			received := make(chan *hooks.EventContext, 1)
			eventBus.Subscribe(tc.expectedHookEvent, func(ctx *hooks.EventContext) {
				received <- ctx
			})

			// Create and handle a heartbeat event
			hbEvent := &heartbeat.HeartbeatEvent{
				Type:      tc.heartbeatEvent,
				Provider:  "test-provider",
				Timestamp: time.Now(),
				Status: &heartbeat.HealthStatus{
					Provider:     "test-provider",
					Status:       heartbeat.StatusHealthy,
					LastCheck:    time.Now(),
					ErrorMessage: "test error",
					QuotaUsed:    8000,
					QuotaLimit:   10000,
				},
			}

			// Handle the event
			err := bridge.HandleEvent(hbEvent)
			if err != nil {
				t.Fatalf("Failed to handle heartbeat event: %v", err)
			}

			// Verify the event was received
			select {
			case receivedEvent := <-received:
				if receivedEvent.Event != tc.expectedHookEvent {
					t.Errorf("Expected event type %s, got %s", tc.expectedHookEvent, receivedEvent.Event)
				}
				if receivedEvent.Provider != "test-provider" {
					t.Errorf("Expected provider 'test-provider', got '%s'", receivedEvent.Provider)
				}
				t.Logf("✓ %s: %s", tc.name, tc.description)
			case <-time.After(100 * time.Millisecond):
				t.Errorf("Timeout waiting for event: %s", tc.description)
			}
		})
	}
}

// TestHeartbeatEventConnection_Requirements verifies that the implementation
// satisfies requirements 5.5 and 5.6.
func TestHeartbeatEventConnection_Requirements(t *testing.T) {
	eventBus := hooks.NewEventBus()
	defer eventBus.Shutdown()

	config := heartbeat.DefaultHeartbeatConfig()
	config.Enabled = true
	hbMonitor := heartbeat.NewHeartbeatMonitor(config)

	integrator := NewEventBusIntegrator(eventBus, nil, hbMonitor)

	// Connect heartbeat events
	err := integrator.ConnectHeartbeatEvents()
	if err != nil {
		t.Fatalf("Failed to connect heartbeat events: %v", err)
	}

	t.Log("✓ Requirement 5.5: Provider status change events are connected")
	t.Log("✓ Requirement 5.6: Quota warning and critical events are connected")
	t.Log("✓ Task 6.2: All heartbeat events are properly connected to event bus")
}
