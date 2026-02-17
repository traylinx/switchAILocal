package responses

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestConvertGeminiResponseToOpenAIResponses_StreamingFunctionCall(t *testing.T) {
	ctx := context.Background()
	var param any

	// 1. Initial chunk with function call start
	chunk1 := []byte(`{
		"responseId": "resp-func-1",
		"candidates": [{
			"content": {
				"parts": [{
					"functionCall": {
						"name": "get_weather",
						"args": {"location": "San Francisco"}
					}
				}]
			}
		}]
	}`)

	events1 := ConvertGeminiResponseToOpenAIResponses(ctx, "gemini-pro", nil, nil, chunk1, &param)

	foundItemAdded := false
	foundArgsDelta := false

	for _, event := range events1 {
		if strings.Contains(event, "response.output_item.added") && strings.Contains(event, "function_call") {
			foundItemAdded = true
		}
		if strings.Contains(event, "response.function_call_arguments.delta") && strings.Contains(event, "San Francisco") {
			foundArgsDelta = true
		}
	}

	if !foundItemAdded {
		t.Error("Did not find response.output_item.added for function call")
	}
	if !foundArgsDelta {
		t.Error("Did not find response.function_call_arguments.delta")
	}

	// 2. Second chunk with more args (simulated appending)
	chunk2 := []byte(`{
		"candidates": [{
			"content": {
				"parts": [{
					"functionCall": {
						"args": {"unit": "celsius"}
					}
				}]
			}
		}]
	}`)

	events2 := ConvertGeminiResponseToOpenAIResponses(ctx, "gemini-pro", nil, nil, chunk2, &param)

	foundArgsDelta2 := false
	for _, event := range events2 {
		if strings.Contains(event, "response.function_call_arguments.delta") && strings.Contains(event, "celsius") {
			foundArgsDelta2 = true
		}
	}
	if !foundArgsDelta2 {
		t.Error("Did not find second response.function_call_arguments.delta")
	}

	// 3. Final chunk
	chunk3 := []byte(`{
		"candidates": [{
			"finishReason": "STOP"
		}]
	}`)

	events3 := ConvertGeminiResponseToOpenAIResponses(ctx, "gemini-pro", nil, nil, chunk3, &param)

	foundItemDone := false
	foundArgsDone := false
	var finalArgs string

	for _, event := range events3 {
		lines := strings.Split(event, "\n")
		var dataLine string
		for _, line := range lines {
			if strings.HasPrefix(line, "data: ") {
				dataLine = strings.TrimPrefix(line, "data: ")
				break
			}
		}

		if dataLine != "" {
			var obj map[string]any
			if err := json.Unmarshal([]byte(dataLine), &obj); err == nil {
				if typ, ok := obj["type"].(string); ok {
					if typ == "response.output_item.done" {
						item := obj["item"].(map[string]any)
						if item["type"] == "function_call" {
							foundItemDone = true
							finalArgs = item["arguments"].(string)
						}
					}
					if typ == "response.function_call_arguments.done" {
						foundArgsDone = true
					}
				}
			}
		}
	}

	if !foundItemDone {
		t.Error("Did not find response.output_item.done")
	}
	if !foundArgsDone {
		t.Error("Did not find response.function_call_arguments.done")
	}

	// Check if arguments were aggregated correctly
	if !strings.Contains(finalArgs, "San Francisco") || !strings.Contains(finalArgs, "celsius") {
		t.Errorf("Final arguments incomplete: %s", finalArgs)
	}
}
