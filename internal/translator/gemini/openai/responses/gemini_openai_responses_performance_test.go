package responses

import (
	"context"
	"testing"
)

// BenchmarkConvertGeminiResponseToOpenAIResponses benchmarks the conversion of a Gemini text response chunk.
// It resets the state parameter in each iteration to simulate processing a new chunk/request,
// avoiding unrealistic growth of internal buffers that would occur in an infinite loop.
func BenchmarkConvertGeminiResponseToOpenAIResponses(b *testing.B) {
	rawJSON := []byte(`{
		"candidates": [{
			"content": {
				"parts": [{
					"text": "Hello world, this is a test response."
				}]
			}
		}],
		"responseId": "resp-123",
		"createTime": "2024-01-01T00:00:00Z"
	}`)

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var param any
		ConvertGeminiResponseToOpenAIResponses(ctx, "gemini-pro", nil, nil, rawJSON, &param)
	}
}

// BenchmarkConvertGeminiResponseToOpenAIResponses_FuncCall benchmarks the conversion of a Gemini function call chunk.
func BenchmarkConvertGeminiResponseToOpenAIResponses_FuncCall(b *testing.B) {
	rawJSON := []byte(`{
		"candidates": [{
			"content": {
				"parts": [{
					"functionCall": {
						"name": "get_weather",
						"args": {"location": "San Francisco, CA"}
					}
				}]
			}
		}],
		"responseId": "resp-123"
	}`)

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var param any
		ConvertGeminiResponseToOpenAIResponses(ctx, "gemini-pro", nil, nil, rawJSON, &param)
	}
}
