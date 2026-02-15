package responses

import (
	"context"
	"testing"
)

func BenchmarkFunctionCallAggregation(b *testing.B) {
	ctx := context.Background()
	// Simulate a chunk with 5 function calls
	rawJSON := []byte(`{
		"responseId": "resp-123",
        "candidates": [
            {
                "content": {
                    "parts": [
                        { "functionCall": { "name": "f1", "args": {"a": 1} } },
                        { "functionCall": { "name": "f2", "args": {"b": 2} } },
                        { "functionCall": { "name": "f3", "args": {"c": 3} } },
                        { "functionCall": { "name": "f4", "args": {"d": 4} } },
                        { "functionCall": { "name": "f5", "args": {"e": 5} } }
                    ]
                }
            }
        ]
    }`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var param any
		// Simulate a stream of 5 chunks (accumulating calls)
		// Note: The function implementation appends calls.
		// If we pass the same JSON 5 times, it will generate 5x5=25 function calls in the state.
		for j := 0; j < 5; j++ {
			ConvertGeminiResponseToOpenAIResponses(ctx, "model", nil, nil, rawJSON, &param)
		}
	}
}
