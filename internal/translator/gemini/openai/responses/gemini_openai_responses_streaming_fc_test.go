package responses

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestConvertGeminiResponseToOpenAIResponses_FunctionCallStreaming(t *testing.T) {
	ctx := context.Background()
	var param any

	// Chunk 1: Complete Function call
	chunk1 := []byte(`{
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
	      },
          "finishReason": "STOP"
	    }
	  ]
	}`)

	events1 := ConvertGeminiResponseToOpenAIResponses(ctx, "gemini-pro", nil, nil, chunk1, &param)

	foundAdded := false
	foundDelta := false
	foundDone := false
	foundItemDone := false

	for _, event := range events1 {
		if strings.Contains(event, "response.output_item.added") && strings.Contains(event, "get_weather") {
			foundAdded = true
		}
		if strings.Contains(event, "response.function_call_arguments.delta") && strings.Contains(event, "location") {
			foundDelta = true
		}
		if strings.Contains(event, "response.function_call_arguments.done") {
			foundDone = true

			// Extract data
			lines := strings.Split(event, "\n")
			var dataStr string
			for _, line := range lines {
				if strings.HasPrefix(line, "data: ") {
					dataStr = strings.TrimPrefix(line, "data: ")
				}
			}

			var data map[string]interface{}
			if err := json.Unmarshal([]byte(dataStr), &data); err != nil {
				t.Errorf("Failed to parse event data: %v", err)
			}

			args, ok := data["arguments"].(string)
			if !ok {
				t.Errorf("Arguments not string in done event")
			}
			if !strings.Contains(args, "London") {
				t.Errorf("Expected arguments to contain 'London', got: %s", args)
			}
		}
		if strings.Contains(event, "response.output_item.done") {
			foundItemDone = true
		}
	}

	if !foundAdded {
		t.Error("Did not find response.output_item.added")
	}
	if !foundDelta {
		t.Error("Did not find response.function_call_arguments.delta")
	}
	if !foundDone {
		t.Error("Did not find response.function_call_arguments.done")
	}
	if !foundItemDone {
		t.Error("Did not find response.output_item.done")
	}
}
