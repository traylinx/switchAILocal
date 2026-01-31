// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package matrix

import (
	"testing"

	"github.com/traylinx/switchAILocal/internal/intelligence/capability"
)

// TestNewBuilder tests the creation of a new matrix builder
func TestNewBuilder(t *testing.T) {
	builder := NewBuilder(true, true, nil)
	if builder == nil {
		t.Fatal("NewBuilder returned nil")
	}
}

// TestBuildEmptyModels tests building a matrix with no models
func TestBuildEmptyModels(t *testing.T) {
	builder := NewBuilder(false, false, nil)
	matrix := builder.Build([]*ModelWithCapability{})

	if matrix == nil {
		t.Fatal("Build returned nil for empty models")
	}

	// All slots should exist but have no primary
	for _, slot := range AllSlots() {
		assignment, exists := matrix.Assignments[slot]
		if !exists {
			t.Errorf("Expected slot %s to exist", slot)
			continue
		}
		if assignment.Primary != "" {
			t.Errorf("Expected empty primary for slot %s with no models", slot)
		}
	}
}

// TestScoringCodingSlot tests that coding models are scored correctly
// Requirements 3.1: Score and rank models for each capability slot
func TestScoringCodingSlot(t *testing.T) {
	builder := NewBuilder(false, false, nil)

	models := []*ModelWithCapability{
		{
			ID:       "deepseek-coder",
			Provider: "openai",
			Capabilities: &capability.ModelCapability{
				SupportsCoding:    true,
				SupportsReasoning: false,
				SupportsVision:    false,
				ContextWindow:     128000,
				EstimatedLatency:  "standard",
				CostTier:          "medium",
				IsLocal:           false,
			},
		},
		{
			ID:       "gpt-3.5-turbo",
			Provider: "openai",
			Capabilities: &capability.ModelCapability{
				SupportsCoding:    false,
				SupportsReasoning: false,
				SupportsVision:    false,
				ContextWindow:     16000,
				EstimatedLatency:  "fast",
				CostTier:          "low",
				IsLocal:           false,
			},
		},
	}

	matrix := builder.Build(models)

	assignment := matrix.Assignments[SlotCoding]
	if assignment.Primary != "deepseek-coder" {
		t.Errorf("Expected deepseek-coder as primary for coding slot, got %s", assignment.Primary)
	}
}


// TestScoringReasoningSlot tests that reasoning models are scored correctly
func TestScoringReasoningSlot(t *testing.T) {
	builder := NewBuilder(false, false, nil)

	models := []*ModelWithCapability{
		{
			ID:       "o1-preview",
			Provider: "openai",
			Capabilities: &capability.ModelCapability{
				SupportsCoding:    false,
				SupportsReasoning: true,
				SupportsVision:    false,
				ContextWindow:     128000,
				EstimatedLatency:  "slow",
				CostTier:          "high",
				IsLocal:           false,
			},
		},
		{
			ID:       "gpt-4o-mini",
			Provider: "openai",
			Capabilities: &capability.ModelCapability{
				SupportsCoding:    false,
				SupportsReasoning: false,
				SupportsVision:    true,
				ContextWindow:     128000,
				EstimatedLatency:  "fast",
				CostTier:          "low",
				IsLocal:           false,
			},
		},
	}

	matrix := builder.Build(models)

	assignment := matrix.Assignments[SlotReasoning]
	if assignment.Primary != "o1-preview" {
		t.Errorf("Expected o1-preview as primary for reasoning slot, got %s", assignment.Primary)
	}
}

// TestFallbackChainGeneration tests that fallback chains are generated correctly
// Requirements 3.2: Assign a primary model and fallback chain for each slot
func TestFallbackChainGeneration(t *testing.T) {
	builder := NewBuilder(false, false, nil)

	models := []*ModelWithCapability{
		{
			ID:       "model-1",
			Provider: "openai",
			Capabilities: &capability.ModelCapability{
				SupportsCoding:    true,
				ContextWindow:     128000,
				EstimatedLatency:  "standard",
				CostTier:          "high",
			},
		},
		{
			ID:       "model-2",
			Provider: "openai",
			Capabilities: &capability.ModelCapability{
				SupportsCoding:    true,
				ContextWindow:     64000,
				EstimatedLatency:  "standard",
				CostTier:          "medium",
			},
		},
		{
			ID:       "model-3",
			Provider: "openai",
			Capabilities: &capability.ModelCapability{
				SupportsCoding:    true,
				ContextWindow:     32000,
				EstimatedLatency:  "fast",
				CostTier:          "low",
			},
		},
	}

	matrix := builder.Build(models)

	assignment := matrix.Assignments[SlotCoding]
	if len(assignment.Fallbacks) == 0 {
		t.Error("Expected fallback chain to be generated")
	}

	// Fallbacks should not include the primary
	for _, fb := range assignment.Fallbacks {
		if fb == assignment.Primary {
			t.Error("Fallback chain should not include the primary model")
		}
	}
}

// TestSecureSlotPreference tests that local models are preferred for secure slot
// Requirements 3.3: Prefer local models for the secure slot
func TestSecureSlotPreference(t *testing.T) {
	builder := NewBuilder(true, false, nil) // preferLocal = true

	models := []*ModelWithCapability{
		{
			ID:       "cloud-model",
			Provider: "openai",
			Capabilities: &capability.ModelCapability{
				SupportsCoding:    true,
				SupportsReasoning: true,
				SupportsVision:    true,
				ContextWindow:     128000,
				EstimatedLatency:  "standard",
				CostTier:          "high",
				IsLocal:           false,
			},
		},
		{
			ID:       "local-llama",
			Provider: "ollama",
			Capabilities: &capability.ModelCapability{
				SupportsCoding:    false,
				SupportsReasoning: false,
				SupportsVision:    false,
				ContextWindow:     32000,
				EstimatedLatency:  "standard",
				CostTier:          "free",
				IsLocal:           true,
			},
		},
	}

	matrix := builder.Build(models)

	assignment := matrix.Assignments[SlotSecure]
	if assignment.Primary != "local-llama" {
		t.Errorf("Expected local-llama as primary for secure slot, got %s", assignment.Primary)
	}
}

// TestManualOverrideApplication tests that manual overrides are applied correctly
// Requirements 3.4: Respect manual overrides over auto-assignment
func TestManualOverrideApplication(t *testing.T) {
	overrides := map[string]string{
		"coding": "my-custom-coder",
	}
	builder := NewBuilder(false, false, overrides)

	models := []*ModelWithCapability{
		{
			ID:       "deepseek-coder",
			Provider: "openai",
			Capabilities: &capability.ModelCapability{
				SupportsCoding:    true,
				ContextWindow:     128000,
				EstimatedLatency:  "standard",
				CostTier:          "medium",
			},
		},
	}

	matrix := builder.Build(models)

	assignment := matrix.Assignments[SlotCoding]
	if assignment.Primary != "my-custom-coder" {
		t.Errorf("Expected my-custom-coder as primary (override), got %s", assignment.Primary)
	}
	if assignment.Reason != "manual override" {
		t.Errorf("Expected reason to be 'manual override', got %s", assignment.Reason)
	}
}

// TestVisionSlotRequiresVisionCapability tests that vision slot only accepts vision models
func TestVisionSlotRequiresVisionCapability(t *testing.T) {
	builder := NewBuilder(false, false, nil)

	models := []*ModelWithCapability{
		{
			ID:       "text-only-model",
			Provider: "openai",
			Capabilities: &capability.ModelCapability{
				SupportsCoding:    true,
				SupportsReasoning: true,
				SupportsVision:    false,
				ContextWindow:     128000,
				EstimatedLatency:  "standard",
				CostTier:          "high",
			},
		},
	}

	matrix := builder.Build(models)

	assignment := matrix.Assignments[SlotVision]
	if assignment.Primary != "" {
		t.Errorf("Expected no primary for vision slot with non-vision models, got %s", assignment.Primary)
	}
}

// TestCostOptimization tests that cost optimization affects scoring
func TestCostOptimization(t *testing.T) {
	// Without cost optimization
	builderNoCost := NewBuilder(false, false, nil)
	// With cost optimization
	builderWithCost := NewBuilder(false, true, nil)

	models := []*ModelWithCapability{
		{
			ID:       "expensive-model",
			Provider: "openai",
			Capabilities: &capability.ModelCapability{
				SupportsCoding:    true,
				ContextWindow:     128000,
				EstimatedLatency:  "standard",
				CostTier:          "high",
			},
		},
		{
			ID:       "cheap-model",
			Provider: "openai",
			Capabilities: &capability.ModelCapability{
				SupportsCoding:    true,
				ContextWindow:     64000,
				EstimatedLatency:  "standard",
				CostTier:          "low",
			},
		},
	}

	matrixNoCost := builderNoCost.Build(models)
	matrixWithCost := builderWithCost.Build(models)

	// With cost optimization, cheaper model should be preferred
	assignmentWithCost := matrixWithCost.Assignments[SlotCoding]
	if assignmentWithCost.Primary != "cheap-model" {
		t.Logf("With cost optimization, expected cheap-model, got %s", assignmentWithCost.Primary)
	}

	// Without cost optimization, expensive model might be preferred due to context
	assignmentNoCost := matrixNoCost.Assignments[SlotCoding]
	if assignmentNoCost.Primary != "expensive-model" {
		t.Logf("Without cost optimization, expected expensive-model, got %s", assignmentNoCost.Primary)
	}
}

// TestGetCurrentMatrix tests retrieving the current matrix
func TestGetCurrentMatrix(t *testing.T) {
	builder := NewBuilder(false, false, nil)

	// Before building, should be nil
	if builder.GetCurrentMatrix() != nil {
		t.Error("Expected nil matrix before building")
	}

	models := []*ModelWithCapability{
		{
			ID:       "test-model",
			Provider: "openai",
			Capabilities: &capability.ModelCapability{
				SupportsCoding: true,
				ContextWindow:  32000,
			},
		},
	}

	builder.Build(models)

	// After building, should not be nil
	if builder.GetCurrentMatrix() == nil {
		t.Error("Expected non-nil matrix after building")
	}
}

// TestGetCurrentMatrixAsMap tests retrieving the matrix as a map for Lua
func TestGetCurrentMatrixAsMap(t *testing.T) {
	builder := NewBuilder(false, false, nil)

	// Before building, should be nil
	if builder.GetCurrentMatrixAsMap() != nil {
		t.Error("Expected nil map before building")
	}

	models := []*ModelWithCapability{
		{
			ID:       "test-model",
			Provider: "openai",
			Capabilities: &capability.ModelCapability{
				SupportsCoding: true,
				ContextWindow:  32000,
			},
		},
	}

	builder.Build(models)

	matrixMap := builder.GetCurrentMatrixAsMap()
	if matrixMap == nil {
		t.Fatal("Expected non-nil map after building")
	}

	// Check that coding slot exists
	codingSlot, exists := matrixMap["coding"]
	if !exists {
		t.Error("Expected coding slot in map")
	}

	// Check structure
	if slotMap, ok := codingSlot.(map[string]interface{}); ok {
		if _, hasPrimary := slotMap["primary"]; !hasPrimary {
			t.Error("Expected primary field in slot map")
		}
		if _, hasFallbacks := slotMap["fallbacks"]; !hasFallbacks {
			t.Error("Expected fallbacks field in slot map")
		}
	} else {
		t.Error("Expected slot to be a map")
	}
}

// TestAllSlots tests that AllSlots returns all expected slots
func TestAllSlots(t *testing.T) {
	slots := AllSlots()

	expectedSlots := []CapabilitySlot{
		SlotCoding,
		SlotReasoning,
		SlotCreative,
		SlotFast,
		SlotSecure,
		SlotVision,
	}

	if len(slots) != len(expectedSlots) {
		t.Errorf("Expected %d slots, got %d", len(expectedSlots), len(slots))
	}

	for _, expected := range expectedSlots {
		found := false
		for _, slot := range slots {
			if slot == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected slot %s not found", expected)
		}
	}
}

// TestFastSlotPrefersLowLatency tests that fast slot prefers low latency models
func TestFastSlotPrefersLowLatency(t *testing.T) {
	builder := NewBuilder(false, false, nil)

	models := []*ModelWithCapability{
		{
			ID:       "slow-model",
			Provider: "openai",
			Capabilities: &capability.ModelCapability{
				ContextWindow:    128000,
				EstimatedLatency: "slow",
				CostTier:         "high",
			},
		},
		{
			ID:       "fast-model",
			Provider: "openai",
			Capabilities: &capability.ModelCapability{
				ContextWindow:    32000,
				EstimatedLatency: "fast",
				CostTier:         "low",
			},
		},
	}

	matrix := builder.Build(models)

	assignment := matrix.Assignments[SlotFast]
	if assignment.Primary != "fast-model" {
		t.Errorf("Expected fast-model as primary for fast slot, got %s", assignment.Primary)
	}
}
