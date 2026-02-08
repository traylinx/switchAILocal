package responses

import (
	"context"
	"strings"
	"testing"
)

// TestConvertGeminiResponseToOpenAIResponses verifies that the streaming conversion
// correctly handles a Gemini response chunk and produces OpenAI SSE events.
func TestConvertGeminiResponseToOpenAIResponses(t *testing.T) {
	ctx := context.Background()
	var param any

	// Sample Gemini Response Chunk
	rawJSON := []byte(`{
	  "responseId": "resp-stream-123",
	  "candidates": [
	    {
	      "content": {
	        "parts": [
	          { "text": "Hello" }
	        ]
	      }
	    }
	  ]
	}`)

	// First chunk
	events := ConvertGeminiResponseToOpenAIResponses(ctx, "gemini-pro", nil, nil, rawJSON, &param)

	if len(events) == 0 {
		t.Fatal("Expected events, got none")
	}

	// Check for specific events
	foundDelta := false
	for _, event := range events {
		if strings.Contains(event, "response.output_text.delta") {
			foundDelta = true
			if !strings.Contains(event, "Hello") {
				t.Errorf("Expected delta to contain 'Hello', got: %s", event)
			}
		}
	}

	if !foundDelta {
		t.Error("Did not find response.output_text.delta event")
	}

	// Second chunk
	rawJSON2 := []byte(`{
	  "candidates": [
	    {
	      "content": {
	        "parts": [
	          { "text": " World" }
	        ]
	      },
          "finishReason": "STOP"
	    }
	  ]
	}`)

	events2 := ConvertGeminiResponseToOpenAIResponses(ctx, "gemini-pro", nil, nil, rawJSON2, &param)
	if len(events2) == 0 {
		t.Fatal("Expected events for second chunk, got none")
	}

	foundDone := false
	for _, event := range events2 {
		if strings.Contains(event, "response.output_text.done") {
			foundDone = true
		}
	}
	if !foundDone {
		t.Error("Did not find response.output_text.done event")
	}
}

func TestStreamingFunctionCalls(t *testing.T) {
	ctx := context.Background()
	var param any

	// Chunk 1: Function call start
	rawJSON1 := []byte(`{
	  "responseId": "resp-fc-stream",
	  "candidates": [
	    {
	      "content": {
	        "parts": [
	          {
	            "functionCall": {
	              "name": "get_weather",
	              "args": {"location": "London"}
	            }
	          }
	        ]
	      }
	    }
	  ]
	}`)

	events1 := ConvertGeminiResponseToOpenAIResponses(ctx, "gemini-pro", nil, nil, rawJSON1, &param)
	if len(events1) == 0 {
		t.Fatal("Expected events for chunk 1")
	}

	foundAdded := false
	foundArgsDelta := false
	for _, event := range events1 {
		if strings.Contains(event, "response.output_item.added") && strings.Contains(event, "function_call") {
			foundAdded = true
		}
		if strings.Contains(event, "response.function_call_arguments.delta") && strings.Contains(event, "London") {
			foundArgsDelta = true
		}
	}

	if !foundAdded {
		t.Error("Missing response.output_item.added for function_call")
	}
	if !foundArgsDelta {
		t.Error("Missing response.function_call_arguments.delta")
	}

	// Chunk 2: Finish
	rawJSON2 := []byte(`{
	  "candidates": [
	    {
	      "finishReason": "STOP"
	    }
	  ]
	}`)

	events2 := ConvertGeminiResponseToOpenAIResponses(ctx, "gemini-pro", nil, nil, rawJSON2, &param)

	foundDone := false
	foundItemDone := false
	for _, event := range events2 {
		if strings.Contains(event, "response.function_call_arguments.done") && strings.Contains(event, "London") {
			foundDone = true
		}
		if strings.Contains(event, "response.output_item.done") && strings.Contains(event, "function_call") {
			foundItemDone = true
		}
	}

	if !foundDone {
		t.Error("Missing response.function_call_arguments.done")
	}
	if !foundItemDone {
		t.Error("Missing response.output_item.done for function_call")
	}
}
