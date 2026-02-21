package responses

import (
	"context"
	"testing"
)

func BenchmarkConvertGeminiResponseToOpenAIResponses_FuncCallStreaming(b *testing.B) {
	// Sample input chunks for function calls
	chunk1 := []byte(`{"candidates": [{"content": {"parts": [{"functionCall": {"name": "func1", "args": {"arg1": "value1"}}}]}}]}`)
	chunk2 := []byte(`{"candidates": [{"content": {"parts": [{"functionCall": {"name": "func2", "args": {"arg2": "value2"}}}]}}]}`)
	chunk3 := []byte(`{"candidates": [{"finishReason": "STOP"}]}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var state any
		ConvertGeminiResponseToOpenAIResponses(context.Background(), "gemini-pro", nil, nil, chunk1, &state)
		ConvertGeminiResponseToOpenAIResponses(context.Background(), "gemini-pro", nil, nil, chunk2, &state)
		ConvertGeminiResponseToOpenAIResponses(context.Background(), "gemini-pro", nil, nil, chunk3, &state)
	}
}
