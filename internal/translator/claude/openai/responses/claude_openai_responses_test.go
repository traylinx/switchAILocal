package responses

import (
	"context"
	"testing"

	"github.com/tidwall/gjson"
)

func TestConvertClaudeResponseToOpenAIResponsesNonStream(t *testing.T) {
	// Sample Claude SSE response
	rawJSON := []byte(`event: message_start
data: {"type":"message_start","message":{"id":"msg_01","type":"message","role":"assistant","content":[],"model":"claude-3-opus-20240229","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":1}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" World"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":5}}

event: message_stop
data: {"type":"message_stop"}
`)

	ctx := context.Background()
	var param any
	out := ConvertClaudeResponseToOpenAIResponsesNonStream(ctx, "claude-3-opus", nil, nil, rawJSON, &param)

	// Verify output
	if !gjson.Valid(out) {
		t.Fatalf("Output is not valid JSON: %s", out)
	}

	id := gjson.Get(out, "id").String()
	if id != "msg_01" {
		t.Errorf("Expected id 'msg_01', got '%s'", id)
	}

	content := gjson.Get(out, "output.0.content.0.text").String()
	if content != "Hello World" {
		t.Errorf("Expected content 'Hello World', got '%s'", content)
	}

	usageTotal := gjson.Get(out, "usage.total_tokens").Int()
	if usageTotal != 15 { // 10 input + 5 output
		t.Errorf("Expected total_tokens 15, got %d", usageTotal)
	}
}
