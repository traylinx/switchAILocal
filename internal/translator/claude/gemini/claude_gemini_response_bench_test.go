package gemini

import (
	"context"
	"strings"
	"testing"
)

var benchmarkRawJSON = []byte(strings.Repeat("data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":null,\"stop_sequence\":null},\"usage\":{\"output_tokens\":1}}\n\n", 10000))

func BenchmarkConvertClaudeResponseToGeminiNonStream(b *testing.B) {
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ConvertClaudeResponseToGeminiNonStream(ctx, "gemini-pro", nil, nil, benchmarkRawJSON, nil)
	}
}
