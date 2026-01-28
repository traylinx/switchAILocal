package responses

import (
	"context"
	"encoding/json"
	"testing"
)

func TestConvertGeminiResponseToOpenAIResponsesNonStream(t *testing.T) {
	ctx := context.Background()
	var param any

	// Sample Gemini Response
	rawJSON := []byte(`{
	  "responseId": "resp-123",
	  "createTime": "2023-10-27T10:00:00Z",
	  "candidates": [
	    {
	      "content": {
	        "parts": [
	          { "text": "Hello, world!" }
	        ]
	      },
	      "finishReason": "STOP"
	    }
	  ],
	  "usageMetadata": {
	    "promptTokenCount": 10,
	    "candidatesTokenCount": 20,
	    "totalTokenCount": 30
	  }
	}`)

	// Call the function
	output := ConvertGeminiResponseToOpenAIResponsesNonStream(ctx, "gemini-pro", nil, nil, rawJSON, &param)

	// Verify Output
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("Failed to unmarshal output: %v", err)
	}

	if result["id"] != "resp_resp-123" {
		t.Errorf("Expected id to be resp_resp-123, got %v", result["id"])
	}
	if result["object"] != "response" {
		t.Errorf("Expected object to be response, got %v", result["object"])
	}
	if result["status"] != "completed" {
		t.Errorf("Expected status to be completed, got %v", result["status"])
	}

	usage, ok := result["usage"].(map[string]interface{})
	if !ok {
		t.Fatalf("Usage missing or invalid")
	}
	if usage["input_tokens"].(float64) != 10 {
		t.Errorf("Expected input_tokens 10, got %v", usage["input_tokens"])
	}
	if usage["output_tokens"].(float64) != 20 {
		t.Errorf("Expected output_tokens 20, got %v", usage["output_tokens"])
	}

	outputList, ok := result["output"].([]interface{})
	if !ok {
		t.Fatalf("Output missing or invalid")
	}
	if len(outputList) != 1 {
		t.Fatalf("Expected 1 output item, got %d", len(outputList))
	}

	item := outputList[0].(map[string]interface{})
	if item["type"] != "message" {
		t.Errorf("Expected item type message, got %v", item["type"])
	}

	contentList := item["content"].([]interface{})
	contentItem := contentList[0].(map[string]interface{})
	if contentItem["text"] != "Hello, world!" {
		t.Errorf("Expected text 'Hello, world!', got %v", contentItem["text"])
	}
}

func TestConvertGeminiResponseToOpenAIResponsesNonStream_FunctionCall(t *testing.T) {
	ctx := context.Background()
	var param any

	rawJSON := []byte(`{
	  "responseId": "resp-fc",
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

	output := ConvertGeminiResponseToOpenAIResponsesNonStream(ctx, "gemini-pro", nil, nil, rawJSON, &param)

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("Failed to unmarshal output: %v", err)
	}

	outputList := result["output"].([]interface{})
	item := outputList[0].(map[string]interface{})
	if item["type"] != "function_call" {
		t.Errorf("Expected type function_call, got %v", item["type"])
	}
	if item["name"] != "get_weather" {
		t.Errorf("Expected name get_weather, got %v", item["name"])
	}
}
