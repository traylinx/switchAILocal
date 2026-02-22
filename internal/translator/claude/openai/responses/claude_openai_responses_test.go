package responses

import (
	"context"
	"testing"

	"github.com/tidwall/gjson"
)

func TestConvertClaudeResponseToOpenAIResponsesNonStream_Correctness(t *testing.T) {
	// Sample data with mixed line endings and empty lines
	rawJSON := []byte(`
data: {"type": "message_start", "message": {"id": "msg_123", "usage": {"input_tokens": 10}}}

data: {"type": "content_block_start", "index": 0, "content_block": {"type": "text", "text": ""}}
data: {"type": "content_block_delta", "index": 0, "delta": {"type": "text_delta", "text": "Hello"}}
data: {"type": "content_block_delta", "index": 0, "delta": {"type": "text_delta", "text": ", world!"}}
data: {"type": "content_block_stop", "index": 0}

data: {"type": "message_delta", "usage": {"output_tokens": 5}}
data: {"type": "message_stop"}
`)
	ctx := context.Background()
	var param any

	outputJSON := ConvertClaudeResponseToOpenAIResponsesNonStream(ctx, "claude-3-opus-20240229", nil, nil, rawJSON, &param)

	// Verify output
	if !gjson.Valid(outputJSON) {
		t.Fatalf("Output is not valid JSON: %s", outputJSON)
	}

	id := gjson.Get(outputJSON, "id").String()
	if id != "msg_123" {
		t.Errorf("Expected id msg_123, got %s", id)
	}

	outputArr := gjson.Get(outputJSON, "output")
	if !outputArr.Exists() {
		t.Fatalf("Expected 'output' field in response")
	}

	msgs := outputArr.Array()
	foundText := false
	for _, item := range msgs {
		if item.Get("type").String() == "message" {
			// Check for content array
			contents := item.Get("content").Array()
			for _, content := range contents {
				if content.Get("type").String() == "output_text" {
					text := content.Get("text").String()
					if text == "Hello, world!" {
						foundText = true
					}
				}
			}
		}
	}

	if !foundText {
		t.Errorf("Did not find expected text 'Hello, world!' in output: %s", outputJSON)
	}

	// Check usage
	inputTokens := gjson.Get(outputJSON, "usage.input_tokens").Int()
	if inputTokens != 10 {
		t.Errorf("Expected input_tokens 10, got %d", inputTokens)
	}
	outputTokens := gjson.Get(outputJSON, "usage.output_tokens").Int()
	if outputTokens != 5 {
		t.Errorf("Expected output_tokens 5, got %d", outputTokens)
	}
}
