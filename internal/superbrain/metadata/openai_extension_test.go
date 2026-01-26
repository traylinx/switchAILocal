package metadata

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/traylinx/switchAILocal/internal/superbrain/types"
)

func TestToOpenAIExtension_NoHealing(t *testing.T) {
	agg := NewAggregator("req-1", "claudecli")

	metadata := agg.GetMetadata()
	extension := metadata.ToOpenAIExtension()

	// Verify structure
	superbrainData, ok := extension["superbrain"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected superbrain key in extension")
	}

	// Should not be healed if no actions
	healed, ok := superbrainData["healed"].(bool)
	if !ok {
		t.Fatal("Expected healed field to be bool")
	}

	if healed {
		t.Error("Expected healed to be false when no actions recorded")
	}

	// Verify provider fields
	if superbrainData["original_provider"] != "claudecli" {
		t.Errorf("Expected original_provider to be claudecli, got %v", superbrainData["original_provider"])
	}

	if superbrainData["final_provider"] != "claudecli" {
		t.Errorf("Expected final_provider to be claudecli, got %v", superbrainData["final_provider"])
	}
}

func TestToOpenAIExtension_WithHealing(t *testing.T) {
	agg := NewAggregator("req-2", "claudecli")

	// Record some healing actions
	agg.RecordAction("stdin_injection", "Injected permission response", true, map[string]interface{}{
		"pattern": "file_permission",
	})

	agg.RecordAction("restart_with_flags", "Restarted with corrective flags", true, map[string]interface{}{
		"flags": []string{"--skip-permissions"},
	})

	metadata := agg.GetMetadata()
	extension := metadata.ToOpenAIExtension()

	superbrainData, ok := extension["superbrain"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected superbrain key in extension")
	}

	// Should be healed when actions exist
	healed, ok := superbrainData["healed"].(bool)
	if !ok {
		t.Fatal("Expected healed field to be bool")
	}

	if !healed {
		t.Error("Expected healed to be true when actions recorded")
	}

	// Verify healing actions
	actions, ok := superbrainData["healing_actions"].([]map[string]interface{})
	if !ok {
		t.Fatal("Expected healing_actions to be array of maps")
	}

	if len(actions) != 2 {
		t.Errorf("Expected 2 healing actions, got %d", len(actions))
	}

	// Verify first action
	if actions[0]["type"] != "stdin_injection" {
		t.Errorf("Expected first action type to be stdin_injection, got %v", actions[0]["type"])
	}

	if actions[0]["description"] != "Injected permission response" {
		t.Errorf("Expected first action description, got %v", actions[0]["description"])
	}

	if actions[0]["success"] != true {
		t.Error("Expected first action to be successful")
	}

	// Verify second action
	if actions[1]["type"] != "restart_with_flags" {
		t.Errorf("Expected second action type to be restart_with_flags, got %v", actions[1]["type"])
	}
}

func TestToOpenAIExtension_WithFallback(t *testing.T) {
	agg := NewAggregator("req-3", "claudecli")

	// Simulate a fallback scenario
	agg.RecordAction("fallback_routing", "Routed to alternative provider", true, map[string]interface{}{
		"target_provider": "geminicli",
	})

	agg.SetFinalProvider("geminicli")

	metadata := agg.GetMetadata()
	extension := metadata.ToOpenAIExtension()

	superbrainData := extension["superbrain"].(map[string]interface{})

	if superbrainData["original_provider"] != "claudecli" {
		t.Errorf("Expected original_provider to be claudecli, got %v", superbrainData["original_provider"])
	}

	if superbrainData["final_provider"] != "geminicli" {
		t.Errorf("Expected final_provider to be geminicli, got %v", superbrainData["final_provider"])
	}
}

func TestToOpenAIExtension_WithContextOptimization(t *testing.T) {
	agg := NewAggregator("req-4", "claudecli")

	highDensityMap := &types.HighDensityMap{
		TotalFiles:    100,
		IncludedFiles: 20,
		ExcludedFiles: 80,
		TokensSaved:   50000,
	}

	agg.SetContextOptimization(highDensityMap)

	metadata := agg.GetMetadata()
	extension := metadata.ToOpenAIExtension()

	superbrainData := extension["superbrain"].(map[string]interface{})

	contextOptimized, ok := superbrainData["context_optimized"].(bool)
	if !ok {
		t.Fatal("Expected context_optimized field to be bool")
	}

	if !contextOptimized {
		t.Error("Expected context_optimized to be true")
	}
}

func TestToOpenAIExtension_JSONSerialization(t *testing.T) {
	agg := NewAggregator("req-5", "claudecli")

	// Add various data
	agg.RecordAction("stdin_injection", "Test action", true, map[string]interface{}{
		"key": "value",
	})

	agg.SetFinalProvider("geminicli")

	highDensityMap := &types.HighDensityMap{
		TotalFiles:    50,
		IncludedFiles: 10,
		ExcludedFiles: 40,
		TokensSaved:   25000,
	}
	agg.SetContextOptimization(highDensityMap)

	metadata := agg.GetMetadata()
	extension := metadata.ToOpenAIExtension()

	// Verify it can be serialized to JSON
	jsonData, err := json.Marshal(extension)
	if err != nil {
		t.Fatalf("Failed to serialize extension to JSON: %v", err)
	}

	// Verify it can be deserialized
	var deserialized map[string]interface{}
	err = json.Unmarshal(jsonData, &deserialized)
	if err != nil {
		t.Fatalf("Failed to deserialize JSON: %v", err)
	}

	// Verify structure is preserved
	superbrainData, ok := deserialized["superbrain"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected superbrain key after deserialization")
	}

	if superbrainData["healed"] != true {
		t.Error("Expected healed to be true after deserialization")
	}
}

func TestToOpenAIExtension_EmptyActions(t *testing.T) {
	agg := NewAggregator("req-6", "claudecli")

	metadata := agg.GetMetadata()
	extension := metadata.ToOpenAIExtension()

	superbrainData := extension["superbrain"].(map[string]interface{})

	actions, ok := superbrainData["healing_actions"].([]map[string]interface{})
	if !ok {
		t.Fatal("Expected healing_actions to be array")
	}

	if len(actions) != 0 {
		t.Errorf("Expected 0 healing actions, got %d", len(actions))
	}
}

func TestToOpenAIExtension_MultipleActions(t *testing.T) {
	agg := NewAggregator("req-7", "claudecli")

	// Record multiple actions with different outcomes
	agg.RecordAction("stdin_injection", "First attempt", false, nil)
	agg.RecordAction("restart_with_flags", "Second attempt", true, nil)
	agg.RecordAction("fallback_routing", "Final attempt", true, nil)

	metadata := agg.GetMetadata()
	extension := metadata.ToOpenAIExtension()

	superbrainData := extension["superbrain"].(map[string]interface{})
	actions := superbrainData["healing_actions"].([]map[string]interface{})

	if len(actions) != 3 {
		t.Errorf("Expected 3 healing actions, got %d", len(actions))
	}

	// Verify first action (failed)
	if actions[0]["success"] != false {
		t.Error("Expected first action to have failed")
	}

	// Verify subsequent actions (successful)
	if actions[1]["success"] != true {
		t.Error("Expected second action to be successful")
	}

	if actions[2]["success"] != true {
		t.Error("Expected third action to be successful")
	}
}

func TestToOpenAIExtension_CompatibilityWithOpenAIResponse(t *testing.T) {
	// Simulate a complete OpenAI-style response with superbrain extension
	agg := NewAggregator("req-8", "claudecli")
	agg.RecordAction("stdin_injection", "Auto-approved permission", true, nil)

	metadata := agg.GetMetadata()
	extension := metadata.ToOpenAIExtension()

	// Create a mock OpenAI response structure
	openAIResponse := map[string]interface{}{
		"id":      "chatcmpl-123",
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   "claude-3-5-sonnet-20241022",
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": "Hello, world!",
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     10,
			"completion_tokens": 20,
			"total_tokens":      30,
		},
	}

	// Add superbrain extension
	for k, v := range extension {
		openAIResponse[k] = v
	}

	// Verify it can be serialized
	jsonData, err := json.Marshal(openAIResponse)
	if err != nil {
		t.Fatalf("Failed to serialize OpenAI response with extension: %v", err)
	}

	// Verify it can be deserialized
	var deserialized map[string]interface{}
	err = json.Unmarshal(jsonData, &deserialized)
	if err != nil {
		t.Fatalf("Failed to deserialize OpenAI response: %v", err)
	}

	// Verify all standard OpenAI fields are present
	if deserialized["id"] != "chatcmpl-123" {
		t.Error("OpenAI response structure corrupted")
	}

	// Verify superbrain extension is present
	if _, ok := deserialized["superbrain"]; !ok {
		t.Error("Superbrain extension missing from response")
	}
}

func TestToOpenAIExtension_NoContextOptimization(t *testing.T) {
	agg := NewAggregator("req-9", "claudecli")

	metadata := agg.GetMetadata()
	extension := metadata.ToOpenAIExtension()

	superbrainData := extension["superbrain"].(map[string]interface{})

	contextOptimized, ok := superbrainData["context_optimized"].(bool)
	if !ok {
		t.Fatal("Expected context_optimized field to be bool")
	}

	if contextOptimized {
		t.Error("Expected context_optimized to be false when not set")
	}
}
