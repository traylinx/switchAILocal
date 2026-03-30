package router

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/traylinx/switchAILocal/internal/config"
)

func TestRequestAdapter_AdaptRequest_PreservesMessages(t *testing.T) {
	adapter := NewRequestAdapter()

	originalReq := ChatRequest{
		Model: "claude-3-5-sonnet-20241022",
		Messages: []ChatMessage{
			{Role: "system", Content: "You are a helpful assistant."},
			{Role: "user", Content: "Hello, how are you?"},
			{Role: "assistant", Content: "I'm doing well, thank you!"},
			{Role: "user", Content: "What's the weather like?"},
		},
		Stream: true,
	}

	payload, _ := json.Marshal(originalReq)

	adapted, err := adapter.AdaptRequest(payload, "gemini")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Parse the adapted request
	var adaptedReq ChatRequest
	if err := json.Unmarshal(adapted.Payload, &adaptedReq); err != nil {
		t.Fatalf("failed to parse adapted request: %v", err)
	}

	// Verify messages are preserved exactly
	if len(adaptedReq.Messages) != len(originalReq.Messages) {
		t.Errorf("expected %d messages, got %d", len(originalReq.Messages), len(adaptedReq.Messages))
	}

	for i, msg := range adaptedReq.Messages {
		if msg.Role != originalReq.Messages[i].Role {
			t.Errorf("message %d: expected role %s, got %s", i, originalReq.Messages[i].Role, msg.Role)
		}
		if msg.Content != originalReq.Messages[i].Content {
			t.Errorf("message %d: expected content %s, got %s", i, originalReq.Messages[i].Content, msg.Content)
		}
	}

	// Verify "messages" is in preserved semantics
	found := false
	for _, s := range adapted.PreservedSemantics {
		if s == "messages" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'messages' in preserved semantics")
	}
}

func TestRequestAdapter_AdaptRequest_PreservesStreamingPreference(t *testing.T) {
	adapter := NewRequestAdapter()

	tests := []struct {
		name   string
		stream bool
	}{
		{"streaming enabled", true},
		{"streaming disabled", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalReq := ChatRequest{
				Model: "claude-3-5-sonnet-20241022",
				Messages: []ChatMessage{
					{Role: "user", Content: "Hello"},
				},
				Stream: tt.stream,
			}

			payload, _ := json.Marshal(originalReq)

			adapted, err := adapter.AdaptRequest(payload, "gemini")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var adaptedReq ChatRequest
			if err := json.Unmarshal(adapted.Payload, &adaptedReq); err != nil {
				t.Fatalf("failed to parse adapted request: %v", err)
			}

			if adaptedReq.Stream != tt.stream {
				t.Errorf("expected stream=%v, got %v", tt.stream, adaptedReq.Stream)
			}

			// Verify "stream" is in preserved semantics
			found := false
			for _, s := range adapted.PreservedSemantics {
				if s == "stream" {
					found = true
					break
				}
			}
			if !found {
				t.Error("expected 'stream' in preserved semantics")
			}
		})
	}
}

func TestRequestAdapter_AdaptRequest_AdaptsModelName(t *testing.T) {
	adapter := NewRequestAdapter()

	tests := []struct {
		name           string
		originalModel  string
		targetProvider string
		expectedModel  string
	}{
		{
			name:           "Claude to Gemini",
			originalModel:  "claude-3-5-sonnet-20241022",
			targetProvider: "gemini",
			expectedModel:  "gemini-2.0-flash",
		},
		{
			name:           "Claude to GeminiCLI",
			originalModel:  "claude-3-5-sonnet-20241022",
			targetProvider: "geminicli",
			expectedModel:  "gemini-2.0-flash",
		},
		{
			name:           "Claude Opus to Gemini",
			originalModel:  "claude-3-opus-20240229",
			targetProvider: "gemini",
			expectedModel:  "gemini-2.0-pro",
		},
		{
			name:           "Gemini to Claude",
			originalModel:  "gemini-2.0-flash",
			targetProvider: "claudecli",
			expectedModel:  "claude-3-5-sonnet-20241022",
		},
		{
			name:           "GPT-4o to Gemini",
			originalModel:  "gpt-4o",
			targetProvider: "gemini",
			expectedModel:  "gemini-2.0-pro",
		},
		{
			name:           "Unknown model uses default",
			originalModel:  "unknown-model",
			targetProvider: "gemini",
			expectedModel:  "gemini-2.0-flash",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalReq := ChatRequest{
				Model: tt.originalModel,
				Messages: []ChatMessage{
					{Role: "user", Content: "Hello"},
				},
			}

			payload, _ := json.Marshal(originalReq)

			adapted, err := adapter.AdaptRequest(payload, tt.targetProvider)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if adapted.OriginalModel != tt.originalModel {
				t.Errorf("expected original model %s, got %s", tt.originalModel, adapted.OriginalModel)
			}

			if adapted.AdaptedModel != tt.expectedModel {
				t.Errorf("expected adapted model %s, got %s", tt.expectedModel, adapted.AdaptedModel)
			}

			var adaptedReq ChatRequest
			if err := json.Unmarshal(adapted.Payload, &adaptedReq); err != nil {
				t.Fatalf("failed to parse adapted request: %v", err)
			}

			if adaptedReq.Model != tt.expectedModel {
				t.Errorf("expected model in payload %s, got %s", tt.expectedModel, adaptedReq.Model)
			}
		})
	}
}

func TestRequestAdapter_AdaptRequest_PreservesCapabilities(t *testing.T) {
	adapter := NewRequestAdapter()

	temp := 0.7
	maxTokens := 1000
	topP := 0.9

	originalReq := ChatRequest{
		Model: "claude-3-5-sonnet-20241022",
		Messages: []ChatMessage{
			{Role: "user", Content: "Hello"},
		},
		Temperature: &temp,
		MaxTokens:   &maxTokens,
		TopP:        &topP,
		Stop:        []string{"END", "STOP"},
	}

	payload, _ := json.Marshal(originalReq)

	adapted, err := adapter.AdaptRequest(payload, "gemini")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var adaptedReq ChatRequest
	if err := json.Unmarshal(adapted.Payload, &adaptedReq); err != nil {
		t.Fatalf("failed to parse adapted request: %v", err)
	}

	// Verify capabilities are preserved
	if adaptedReq.Temperature == nil || *adaptedReq.Temperature != temp {
		t.Errorf("expected temperature %v, got %v", temp, adaptedReq.Temperature)
	}

	if adaptedReq.MaxTokens == nil || *adaptedReq.MaxTokens != maxTokens {
		t.Errorf("expected max_tokens %v, got %v", maxTokens, adaptedReq.MaxTokens)
	}

	if adaptedReq.TopP == nil || *adaptedReq.TopP != topP {
		t.Errorf("expected top_p %v, got %v", topP, adaptedReq.TopP)
	}

	if len(adaptedReq.Stop) != 2 || adaptedReq.Stop[0] != "END" || adaptedReq.Stop[1] != "STOP" {
		t.Errorf("expected stop sequences [END, STOP], got %v", adaptedReq.Stop)
	}

	// Verify preserved semantics includes these
	expectedSemantics := []string{"temperature", "max_tokens", "top_p", "stop"}
	for _, expected := range expectedSemantics {
		found := false
		for _, s := range adapted.PreservedSemantics {
			if s == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected '%s' in preserved semantics", expected)
		}
	}
}

func TestRequestAdapter_AdaptRequest_AdaptsExtraBody(t *testing.T) {
	adapter := NewRequestAdapter()

	tests := []struct {
		name           string
		extraBody      map[string]interface{}
		targetProvider string
		expectCLI      bool
	}{
		{
			name: "CLI fields preserved for CLI provider",
			extraBody: map[string]interface{}{
				"cli":    map[string]interface{}{"some": "option"},
				"custom": "value",
			},
			targetProvider: "geminicli",
			expectCLI:      true,
		},
		{
			name: "CLI fields removed for non-CLI provider",
			extraBody: map[string]interface{}{
				"cli":    map[string]interface{}{"some": "option"},
				"custom": "value",
			},
			targetProvider: "gemini",
			expectCLI:      false,
		},
		{
			name:           "nil extra_body stays nil",
			extraBody:      nil,
			targetProvider: "gemini",
			expectCLI:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalReq := ChatRequest{
				Model: "claude-3-5-sonnet-20241022",
				Messages: []ChatMessage{
					{Role: "user", Content: "Hello"},
				},
				ExtraBody: tt.extraBody,
			}

			payload, _ := json.Marshal(originalReq)

			adapted, err := adapter.AdaptRequest(payload, tt.targetProvider)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var adaptedReq ChatRequest
			if err := json.Unmarshal(adapted.Payload, &adaptedReq); err != nil {
				t.Fatalf("failed to parse adapted request: %v", err)
			}

			if tt.extraBody == nil {
				if adaptedReq.ExtraBody != nil {
					t.Error("expected nil extra_body")
				}
				return
			}

			hasCLI := adaptedReq.ExtraBody != nil && adaptedReq.ExtraBody["cli"] != nil
			if hasCLI != tt.expectCLI {
				t.Errorf("expected CLI fields present=%v, got %v", tt.expectCLI, hasCLI)
			}

			// Custom fields should always be preserved
			if adaptedReq.ExtraBody != nil {
				if adaptedReq.ExtraBody["custom"] != "value" {
					t.Error("expected custom field to be preserved")
				}
			}
		})
	}
}

func TestRequestAdapter_AdaptRequest_InvalidJSON(t *testing.T) {
	adapter := NewRequestAdapter()

	_, err := adapter.AdaptRequest([]byte("invalid json"), "gemini")
	if err != ErrInvalidRequest {
		t.Errorf("expected ErrInvalidRequest, got %v", err)
	}
}

func TestRequestAdapter_SetModelMapping(t *testing.T) {
	adapter := NewRequestAdapter()

	// Set a custom mapping
	adapter.SetModelMapping("custom-model", "gemini", "gemini-custom")

	// Verify the mapping works
	mapped := adapter.GetModelMapping("custom-model", "gemini")
	if mapped != "gemini-custom" {
		t.Errorf("expected gemini-custom, got %s", mapped)
	}

	// Verify it's used in adaptation
	originalReq := ChatRequest{
		Model: "custom-model",
		Messages: []ChatMessage{
			{Role: "user", Content: "Hello"},
		},
	}

	payload, _ := json.Marshal(originalReq)

	adapted, err := adapter.AdaptRequest(payload, "gemini")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if adapted.AdaptedModel != "gemini-custom" {
		t.Errorf("expected adapted model gemini-custom, got %s", adapted.AdaptedModel)
	}
}

func TestIsCLIProvider(t *testing.T) {
	tests := []struct {
		provider string
		expected bool
	}{
		{"claudecli", true},
		{"geminicli", true},
		{"gemini", false},
		{"openai", false},
		{"ollama", false},
		{"unknown", false},
	}

	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			if got := isCLIProvider(tt.provider); got != tt.expected {
				t.Errorf("isCLIProvider(%s) = %v, want %v", tt.provider, got, tt.expected)
			}
		})
	}
}


func TestFallbackRouter_GetFallbackWithAdaptation(t *testing.T) {
	cfg := &config.FallbackConfig{
		Enabled:        true,
		Providers:      []string{"geminicli", "gemini", "ollama"},
		MinSuccessRate: 0.5,
	}
	router := NewFallbackRouter(cfg)

	originalReq := ChatRequest{
		Model: "claude-3-5-sonnet-20241022",
		Messages: []ChatMessage{
			{Role: "system", Content: "You are a helpful assistant."},
			{Role: "user", Content: "Hello"},
		},
		Stream: true,
	}

	payload, _ := json.Marshal(originalReq)

	decision, err := router.GetFallbackWithAdaptation(
		context.Background(),
		"claudecli",
		&RequestRequirements{RequiresStream: true},
		payload,
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify fallback decision
	if decision.OriginalProvider != "claudecli" {
		t.Errorf("expected original provider claudecli, got %s", decision.OriginalProvider)
	}

	if decision.FallbackProvider != "geminicli" {
		t.Errorf("expected fallback provider geminicli, got %s", decision.FallbackProvider)
	}

	// Verify adapted request
	if decision.AdaptedRequest == nil {
		t.Fatal("expected adapted request to be present")
	}

	if decision.AdaptedRequest.OriginalModel != "claude-3-5-sonnet-20241022" {
		t.Errorf("expected original model claude-3-5-sonnet-20241022, got %s", decision.AdaptedRequest.OriginalModel)
	}

	if decision.AdaptedRequest.AdaptedModel != "gemini-2.0-flash" {
		t.Errorf("expected adapted model gemini-2.0-flash, got %s", decision.AdaptedRequest.AdaptedModel)
	}

	// Verify the adapted payload preserves semantics
	var adaptedReq ChatRequest
	if err := json.Unmarshal(decision.AdaptedRequest.Payload, &adaptedReq); err != nil {
		t.Fatalf("failed to parse adapted request: %v", err)
	}

	if len(adaptedReq.Messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(adaptedReq.Messages))
	}

	if !adaptedReq.Stream {
		t.Error("expected streaming to be preserved")
	}
}

func TestFallbackRouter_AdaptRequest(t *testing.T) {
	cfg := &config.FallbackConfig{
		Enabled:        true,
		Providers:      []string{"geminicli"},
		MinSuccessRate: 0.5,
	}
	router := NewFallbackRouter(cfg)

	originalReq := ChatRequest{
		Model: "claude-3-5-sonnet-20241022",
		Messages: []ChatMessage{
			{Role: "user", Content: "Hello"},
		},
	}

	payload, _ := json.Marshal(originalReq)

	adapted, err := router.AdaptRequest(payload, "geminicli")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if adapted.OriginalModel != "claude-3-5-sonnet-20241022" {
		t.Errorf("expected original model claude-3-5-sonnet-20241022, got %s", adapted.OriginalModel)
	}

	if adapted.AdaptedModel != "gemini-2.0-flash" {
		t.Errorf("expected adapted model gemini-2.0-flash, got %s", adapted.AdaptedModel)
	}
}

func TestFallbackRouter_GetRequestAdapter(t *testing.T) {
	cfg := &config.FallbackConfig{
		Enabled:        true,
		Providers:      []string{"geminicli"},
		MinSuccessRate: 0.5,
	}
	router := NewFallbackRouter(cfg)

	adapter := router.GetRequestAdapter()
	if adapter == nil {
		t.Fatal("expected request adapter to be present")
	}

	// Verify we can use the adapter directly
	adapter.SetModelMapping("test-model", "gemini", "gemini-test")
	mapped := adapter.GetModelMapping("test-model", "gemini")
	if mapped != "gemini-test" {
		t.Errorf("expected gemini-test, got %s", mapped)
	}
}
