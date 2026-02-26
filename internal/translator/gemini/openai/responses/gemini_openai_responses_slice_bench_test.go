package responses

import (
	"context"
	"testing"
)

// BenchmarkFuncCallStreaming simulates a response with multiple function calls
// to stress the map overhead and sorting logic.
func BenchmarkFuncCallStreaming(b *testing.B) {
	// A chunk that contains a function call part
	// We'll simulate 5 function calls being streamed.

	// Create chunks for 5 function calls
	chunks := make([][]byte, 0, 5)
	for i := 0; i < 5; i++ {
		// Each chunk introduces a new function call
		jsonStr := `{
			"candidates": [{
				"content": {
					"parts": [{
						"functionCall": {
							"name": "search_web",
							"args": {"query": "something"}
						}
					}]
				}
			}]
		}`
		chunks = append(chunks, []byte(jsonStr))
	}

	// Also a final chunk to trigger finalization
	finalChunk := []byte(`{
		"candidates": [{
			"finishReason": "STOP"
		}]
	}`)

	ctx := context.Background()
	model := "gemini-pro"

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var param any
		// Process chunks
		for _, chunk := range chunks {
			_ = ConvertGeminiResponseToOpenAIResponses(ctx, model, nil, nil, chunk, &param)
		}
		// Finalize
		_ = ConvertGeminiResponseToOpenAIResponses(ctx, model, nil, nil, finalChunk, &param)
	}
}
