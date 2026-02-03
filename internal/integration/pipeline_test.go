package integration

import (
	"sync"
	"testing"
	"time"

	"github.com/traylinx/switchAILocal/internal/hooks"
	"github.com/traylinx/switchAILocal/internal/memory"
	"github.com/traylinx/switchAILocal/internal/steering"
)

// TestNewRequestPipelineIntegrator verifies that the constructor works with nil parameters.
func TestNewRequestPipelineIntegrator(t *testing.T) {
	// Test with all nil parameters
	rpi := NewRequestPipelineIntegrator(nil, nil, nil)
	if rpi == nil {
		t.Fatal("Expected non-nil RequestPipelineIntegrator")
	}

	// Test with non-nil parameters
	eventBus := hooks.NewEventBus()
	rpi2 := NewRequestPipelineIntegrator(nil, nil, eventBus)
	if rpi2 == nil {
		t.Fatal("Expected non-nil RequestPipelineIntegrator")
	}
	if rpi2.eventBus != eventBus {
		t.Error("Expected event bus to be set")
	}
}

// TestApplySteering_NoSteeringEngine verifies behavior when steering engine is nil.
func TestApplySteering_NoSteeringEngine(t *testing.T) {
	rpi := NewRequestPipelineIntegrator(nil, nil, nil)

	ctx := &steering.RoutingContext{
		Intent: "coding",
		Model:  "gpt-4",
	}
	messages := []map[string]string{
		{"role": "user", "content": "Hello"},
	}

	model, modifiedMessages, err := rpi.ApplySteering(ctx, messages)

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if model != "" {
		t.Errorf("Expected empty model, got: %s", model)
	}
	if len(modifiedMessages) != len(messages) {
		t.Errorf("Expected messages unchanged, got different length")
	}
}

// TestInjectContext_NoSystemMessage verifies context injection when no system message exists.
func TestInjectContext_NoSystemMessage(t *testing.T) {
	// Context injection is now handled by the steering engine's ApplySteering method
	// This test is no longer applicable
	t.Skip("Context injection is handled by steering engine")
}

// TestInjectContext_WithSystemMessage verifies context injection when system message exists.
func TestInjectContext_WithSystemMessage(t *testing.T) {
	// Context injection is now handled by the steering engine's ApplySteering method
	// This test is no longer applicable
	t.Skip("Context injection is handled by steering engine")
}

// TestInjectContext_EmptyInjection verifies that empty injection returns original messages.
func TestInjectContext_EmptyInjection(t *testing.T) {
	// Context injection is now handled by the steering engine's ApplySteering method
	// This test is no longer applicable
	t.Skip("Context injection is handled by steering engine")
}

// TestRecordRouting_NoMemory verifies behavior when memory manager is nil.
func TestRecordRouting_NoMemory(t *testing.T) {
	rpi := NewRequestPipelineIntegrator(nil, nil, nil)

	decision := &memory.RoutingDecision{
		Timestamp:  time.Now(),
		APIKeyHash: "test-hash",
		Request: memory.RequestInfo{
			Model:  "gpt-4",
			Intent: "coding",
		},
		Routing: memory.RoutingInfo{
			Tier:          "semantic",
			SelectedModel: "gpt-4",
			Confidence:    0.9,
		},
	}

	err := rpi.RecordRouting(decision)

	// Should not error when memory is nil
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
}

// TestUpdateOutcome_NoMemory verifies behavior when memory manager is nil.
func TestUpdateOutcome_NoMemory(t *testing.T) {
	rpi := NewRequestPipelineIntegrator(nil, nil, nil)

	decision := &memory.RoutingDecision{
		Timestamp:  time.Now(),
		APIKeyHash: "test-hash",
		Outcome: memory.OutcomeInfo{
			Success:        true,
			ResponseTimeMs: 1500,
		},
	}

	err := rpi.UpdateOutcome(decision)

	// Should not error when memory is nil
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
}

// TestEmitRoutingEvent_NoEventBus verifies behavior when event bus is nil.
func TestEmitRoutingEvent_NoEventBus(t *testing.T) {
	rpi := NewRequestPipelineIntegrator(nil, nil, nil)

	decision := &memory.RoutingDecision{
		Timestamp:  time.Now(),
		APIKeyHash: "test-hash",
		Request: memory.RequestInfo{
			Model:  "gpt-4",
			Intent: "coding",
		},
		Routing: memory.RoutingInfo{
			Tier:          "semantic",
			SelectedModel: "claudecli:claude-sonnet-4",
			Confidence:    0.9,
		},
		Outcome: memory.OutcomeInfo{
			Success:        true,
			ResponseTimeMs: 1500,
			QualityScore:   0.95,
		},
	}

	err := rpi.EmitRoutingEvent(decision)

	// Should not error when event bus is nil
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
}

// TestEmitRoutingEvent_WithEventBus verifies event emission with a real event bus.
func TestEmitRoutingEvent_WithEventBus(t *testing.T) {
	eventBus := hooks.NewEventBus()
	defer eventBus.Shutdown()
	
	rpi := NewRequestPipelineIntegrator(nil, nil, eventBus)

	// Subscribe to routing events
	var eventReceived sync.WaitGroup
	eventReceived.Add(1)
	
	eventBus.Subscribe(hooks.EventRoutingDecision, func(ctx *hooks.EventContext) {
		defer eventReceived.Done()

		// Verify event data
		if ctx.Event != hooks.EventRoutingDecision {
			t.Errorf("Expected EventRoutingDecision, got: %v", ctx.Event)
		}
		if ctx.Model != "claudecli:claude-sonnet-4" {
			t.Errorf("Expected model 'claudecli:claude-sonnet-4', got: %s", ctx.Model)
		}
		if ctx.Provider != "claudecli" {
			t.Errorf("Expected provider 'claudecli', got: %s", ctx.Provider)
		}

		// Verify data fields
		if apiKeyHash, ok := ctx.Data["api_key_hash"].(string); !ok || apiKeyHash != "test-hash" {
			t.Errorf("Expected api_key_hash 'test-hash', got: %v", ctx.Data["api_key_hash"])
		}
	})

	decision := &memory.RoutingDecision{
		Timestamp:  time.Now(),
		APIKeyHash: "test-hash",
		Request: memory.RequestInfo{
			Model:  "gpt-4",
			Intent: "coding",
		},
		Routing: memory.RoutingInfo{
			Tier:          "semantic",
			SelectedModel: "claudecli:claude-sonnet-4",
			Confidence:    0.9,
		},
		Outcome: memory.OutcomeInfo{
			Success:        true,
			ResponseTimeMs: 1500,
			QualityScore:   0.95,
		},
	}

	err := rpi.EmitRoutingEvent(decision)

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	// Wait for event to be processed
	done := make(chan struct{})
	go func() {
		eventReceived.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Event was received
	case <-time.After(time.Second):
		t.Error("Timeout waiting for event to be received by subscriber")
	}
}

// TestExtractProvider verifies provider extraction from model strings.
func TestExtractProvider(t *testing.T) {
	tests := []struct {
		model    string
		expected string
	}{
		{"claudecli:claude-sonnet-4", "claudecli"},
		{"openai:gpt-4", "openai"},
		{"gemini:gemini-pro", "gemini"},
		{"gpt-4", "gpt-4"}, // No colon, returns entire string
		{"", ""},           // Empty string
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			result := extractProvider(tt.model)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

// TestEmitRoutingEvent_WithError verifies event emission includes error information.
func TestEmitRoutingEvent_WithError(t *testing.T) {
	eventBus := hooks.NewEventBus()
	defer eventBus.Shutdown()
	
	rpi := NewRequestPipelineIntegrator(nil, nil, eventBus)

	// Subscribe to routing events
	var eventReceived sync.WaitGroup
	eventReceived.Add(1)
	
	eventBus.Subscribe(hooks.EventRoutingDecision, func(ctx *hooks.EventContext) {
		defer eventReceived.Done()

		// Verify error message is included
		if ctx.ErrorMessage != "Provider timeout" {
			t.Errorf("Expected error message 'Provider timeout', got: %s", ctx.ErrorMessage)
		}
	})

	decision := &memory.RoutingDecision{
		Timestamp:  time.Now(),
		APIKeyHash: "test-hash",
		Request: memory.RequestInfo{
			Model:  "gpt-4",
			Intent: "coding",
		},
		Routing: memory.RoutingInfo{
			Tier:          "semantic",
			SelectedModel: "openai:gpt-4",
			Confidence:    0.9,
		},
		Outcome: memory.OutcomeInfo{
			Success:        false,
			ResponseTimeMs: 30000,
			Error:          "Provider timeout",
		},
	}

	err := rpi.EmitRoutingEvent(decision)

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	// Wait for event to be processed
	done := make(chan struct{})
	go func() {
		eventReceived.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Event was received
	case <-time.After(time.Second):
		t.Error("Timeout waiting for event to be received by subscriber")
	}
}
