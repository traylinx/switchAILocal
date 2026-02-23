package responses

import (
	"bytes"
	"context"
	"testing"
)

func BenchmarkConvertClaudeResponseToOpenAIResponsesNonStream(b *testing.B) {
	// Setup a rawJSON input with many SSE events
	var buf bytes.Buffer
	for i := 0; i < 1000; i++ {
		buf.WriteString("data: {\"type\": \"content_block_delta\", \"index\": 0, \"delta\": {\"type\": \"text_delta\", \"text\": \"hello\"}}\n")
	}
	rawJSON := buf.Bytes()
	ctx := context.Background()
	// param is ignored in non-stream but strict signature requires it
	var param any

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ConvertClaudeResponseToOpenAIResponsesNonStream(ctx, "claude-3-opus", nil, nil, rawJSON, &param)
	}
}
