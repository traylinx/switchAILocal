package responses

import (
	"context"
	"testing"
)

func BenchmarkConvertClaudeResponseToOpenAIResponsesNonStream(b *testing.B) {
	// Sample data simulating a small but complete response
	rawJSON := []byte(`
data: {"type": "message_start", "message": {"id": "msg_123", "usage": {"input_tokens": 10}}}
data: {"type": "content_block_start", "index": 0, "content_block": {"type": "text", "text": ""}}
data: {"type": "content_block_delta", "index": 0, "delta": {"type": "text_delta", "text": "Hello, world!"}}
data: {"type": "content_block_stop", "index": 0}
data: {"type": "message_delta", "usage": {"output_tokens": 5}}
data: {"type": "message_stop"}
`)
	ctx := context.Background()
	var param any

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ConvertClaudeResponseToOpenAIResponsesNonStream(ctx, "claude-3-opus-20240229", nil, nil, rawJSON, &param)
	}
}
