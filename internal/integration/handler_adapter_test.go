package integration

import (
	"testing"
	"time"

	"github.com/traylinx/switchAILocal/internal/memory"
)

func TestHandlerAdapter_RecordRouting_WithMap(t *testing.T) {
	// Create a mock integrator
	recordCalled := false
	mockIntegrator := &RequestPipelineIntegrator{
		memory: &mockMemoryManager{
			recordFunc: func(decision *memory.RoutingDecision) error {
				recordCalled = true
				if decision.APIKeyHash != "test-hash" {
					t.Errorf("Expected APIKeyHash 'test-hash', got '%s'", decision.APIKeyHash)
				}
				if decision.Request.Model != "test-model" {
					t.Errorf("Expected Model 'test-model', got '%s'", decision.Request.Model)
				}
				if decision.Routing.SelectedModel != "selected-model" {
					t.Errorf("Expected SelectedModel 'selected-model', got '%s'", decision.Routing.SelectedModel)
				}
				if !decision.Outcome.Success {
					t.Error("Expected Success to be true")
				}
				return nil
			},
		},
	}

	adapter := NewHandlerAdapter(mockIntegrator)

	// Create a decision map
	decisionMap := map[string]interface{}{
		"api_key_hash": "test-hash",
		"timestamp":    time.Now(),
		"request": map[string]interface{}{
			"model": "test-model",
		},
		"routing": map[string]interface{}{
			"selected_model": "selected-model",
			"tier":           "premium",
			"confidence":     0.95,
			"latency_ms":     int64(100),
		},
		"outcome": map[string]interface{}{
			"success":          true,
			"response_time_ms": int64(500),
		},
	}

	// Record the routing decision
	err := adapter.RecordRouting(decisionMap)
	if err != nil {
		t.Fatalf("RecordRouting failed: %v", err)
	}

	if !recordCalled {
		t.Error("Expected RecordRouting to be called on the integrator")
	}
}

func TestHandlerAdapter_RecordRouting_WithNilIntegrator(t *testing.T) {
	adapter := NewHandlerAdapter(nil)

	decisionMap := map[string]interface{}{
		"api_key_hash": "test-hash",
	}

	// Should not panic with nil integrator
	err := adapter.RecordRouting(decisionMap)
	if err != nil {
		t.Fatalf("RecordRouting should not fail with nil integrator: %v", err)
	}
}

func TestHandlerAdapter_EmitRoutingEvent_WithMap(t *testing.T) {
	// Create a mock integrator with a real event bus
	emitCalled := false
	
	// We can't easily mock the event bus, so we'll just verify the method doesn't panic
	mockIntegrator := &RequestPipelineIntegrator{
		eventBus: nil, // Event bus is optional
	}

	adapter := NewHandlerAdapter(mockIntegrator)

	decisionMap := map[string]interface{}{
		"api_key_hash": "test-hash",
		"timestamp":    time.Now(),
		"request": map[string]interface{}{
			"model": "test-model",
		},
		"routing": map[string]interface{}{
			"selected_model": "selected-model",
		},
		"outcome": map[string]interface{}{
			"success": true,
		},
	}

	// Emit the routing event - should not panic even with nil event bus
	err := adapter.EmitRoutingEvent(decisionMap)
	if err != nil {
		t.Fatalf("EmitRoutingEvent failed: %v", err)
	}

	// Since we can't easily verify the event was emitted, we just verify no panic
	_ = emitCalled
}

// Mock implementations for testing

type mockMemoryManager struct {
	recordFunc func(*memory.RoutingDecision) error
}

func (m *mockMemoryManager) RecordRouting(decision *memory.RoutingDecision) error {
	if m.recordFunc != nil {
		return m.recordFunc(decision)
	}
	return nil
}

func (m *mockMemoryManager) GetAnalytics() (*memory.AnalyticsSummary, error) {
	return nil, nil
}

func (m *mockMemoryManager) ComputeAnalytics() (*memory.AnalyticsSummary, error) {
	return nil, nil
}

func (m *mockMemoryManager) GetUserPreferences(apiKeyHash string) (*memory.UserPreferences, error) {
	return nil, nil
}

func (m *mockMemoryManager) UpdateUserPreferences(prefs *memory.UserPreferences) error {
	return nil
}

func (m *mockMemoryManager) DeleteUserPreferences(apiKeyHash string) error {
	return nil
}

func (m *mockMemoryManager) AddQuirk(quirk *memory.Quirk) error {
	return nil
}

func (m *mockMemoryManager) GetProviderQuirks(provider string) ([]*memory.Quirk, error) {
	return nil, nil
}

func (m *mockMemoryManager) GetHistory(apiKeyHash string, limit int) ([]*memory.RoutingDecision, error) {
	return nil, nil
}

func (m *mockMemoryManager) GetAllHistory(limit int) ([]*memory.RoutingDecision, error) {
	return nil, nil
}

func (m *mockMemoryManager) LearnFromOutcome(decision *memory.RoutingDecision) error {
	return nil
}

func (m *mockMemoryManager) GetStats() (*memory.MemoryStats, error) {
	return nil, nil
}

func (m *mockMemoryManager) Cleanup() error {
	return nil
}

func (m *mockMemoryManager) Close() error {
	return nil
}
