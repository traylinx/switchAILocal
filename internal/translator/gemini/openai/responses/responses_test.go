package responses

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestConvertGeminiResponseToOpenAIResponses_Streaming(t *testing.T) {
	// Sample minimal Gemini 1.5 streaming response chunk
	rawJSON := []byte(`{
		"responseId": "test-resp-123",
		"candidates": [{
			"content": {
				"parts": [{ "text": "Hello world" }],
				"role": "model"
			},
			"finishReason": "",
			"index": 0
		}],
		"usageMetadata": {
			"promptTokenCount": 10,
			"candidatesTokenCount": 5,
			"totalTokenCount": 15
		}
	}`)

	var state any // opaque param for state

	// First chunk (start + content)
	ctx := context.Background()
	events := ConvertGeminiResponseToOpenAIResponses(ctx, "gemini-1.5-flash", nil, nil, rawJSON, &state)

	if len(events) == 0 {
		t.Fatal("Expected events, got 0")
	}

	// Helper to parse SSE event
	parseEvent := func(evt string) (string, map[string]any) {
		lines := strings.Split(evt, "\n")
		var eventType string
		var data map[string]any
		for _, line := range lines {
			if strings.HasPrefix(line, "event: ") {
				eventType = strings.TrimPrefix(line, "event: ")
			} else if strings.HasPrefix(line, "data: ") {
				json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &data)
			}
		}
		return eventType, data
	}

	// Verify we got created, in_progress, and output items (since it's the first chunk)
	gotCreated := false
	gotContent := false
	for _, e := range events {
		typ, _ := parseEvent(e)
		if typ == "response.created" {
			gotCreated = true
		} else if typ == "response.output_text.delta" {
			gotContent = true
		}
	}

	if !gotCreated {
		t.Error("Missing response.created event in first chunk")
	}
	if !gotContent {
		t.Error("Missing response.output_text.delta event in first chunk")
	}

	// Second chunk (finish)
	rawJSONFinal := []byte(`{
		"responseId": "test-resp-123",
		"candidates": [{
			"content": {
				"parts": [{ "text": "" }],
				"role": "model"
			},
			"finishReason": "STOP",
			"index": 0
		}]
	}`)

	eventsFinal := ConvertGeminiResponseToOpenAIResponses(ctx, "gemini-1.5-flash", nil, nil, rawJSONFinal, &state)

	gotCompleted := false
	for _, e := range eventsFinal {
		typ, data := parseEvent(e)
		if typ == "response.completed" {
			gotCompleted = true
			resp, ok := data["response"].(map[string]any)
			if !ok {
				t.Error("response.completed missing response object")
				continue
			}
			if resp["status"] != "completed" {
				t.Errorf("Expected status completed, got %v", resp["status"])
			}
			// Verify Struct Zero-Allocation logic didn't break JSON keys
			// The PR changed ResponseUsage fields, check if they are present if usage was sent
			// (Usage was sent in first chunk, logic aggregates it? actually ConvertGemini logic keeps state in 'st')
		}
	}

	if !gotCompleted {
		t.Error("Missing response.completed event in final chunk")
	}
}

func TestResponseCompleted_Marshal(t *testing.T) {
	// Verify struct tags for new ResponseCompleted struct (PR #12 core change)
	rc := ResponseCompleted{
		Type:           "response.completed",
		SequenceNumber: 1,
		Response: ResponseInfo{
			ID:     "resp_1",
			Status: "completed",
			Usage: &ResponseUsage{
				InputTokens:  10,
				OutputTokens: 20,
			},
		},
	}

	b, err := json.Marshal(rc)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var m map[string]any
	json.Unmarshal(b, &m)

	if m["type"] != "response.completed" {
		t.Errorf("JSON type mismatch: %v", m["type"])
	}
	resp := m["response"].(map[string]any)
	usage := resp["usage"].(map[string]any)
	if usage["input_tokens"].(float64) != 10 { // JSON numbers are floats in map[string]any
		t.Errorf("input_tokens mismatch")
	}
}
