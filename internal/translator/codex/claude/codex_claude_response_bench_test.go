package claude

import (
	"context"
	"testing"
)

func BenchmarkConvertCodexResponseToClaude(b *testing.B) {
	// The gjson parser requires a valid JSON object after "data: ".
	rawJSON := []byte(`data: {"type": "response.output_text.delta", "output_index": 0, "delta": "hello"}`)
	var param any = nil
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ConvertCodexResponseToClaude(ctx, "model", nil, nil, rawJSON, &param)
	}
}
