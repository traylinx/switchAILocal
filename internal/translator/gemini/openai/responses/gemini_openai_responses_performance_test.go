package responses

import (
	"context"
	"testing"
)

func BenchmarkConvertGeminiResponseToOpenAIResponses_FunctionCalls(b *testing.B) {
	// Simulate 5 concurrent function calls
	chunks := [][]byte{
		// Chunk 1: Start
		[]byte(`{"responseId": "resp-123", "createTime": "2023-10-27T10:00:00Z"}`),
		// Chunk 2: 5 Function Calls start
		[]byte(`{"candidates": [{"content": {"parts": [{"functionCall": {"name": "func1", "args": {"arg": "1"}}}, {"functionCall": {"name": "func2", "args": {"arg": "2"}}}, {"functionCall": {"name": "func3", "args": {"arg": "3"}}}, {"functionCall": {"name": "func4", "args": {"arg": "4"}}}, {"functionCall": {"name": "func5", "args": {"arg": "5"}}}]}}]}`),
		// Chunk 3: More args
		[]byte(`{"candidates": [{"content": {"parts": [{"functionCall": {"name": "func1", "args": {"more": "1"}}}, {"functionCall": {"name": "func2", "args": {"more": "2"}}}]}}]}`),
		// Chunk 4: Finish
		[]byte(`{"candidates": [{"finishReason": "STOP"}]}`),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var state any
		for _, chunk := range chunks {
			_ = ConvertGeminiResponseToOpenAIResponses(context.Background(), "gemini-pro", nil, nil, chunk, &state)
		}
	}
}

func BenchmarkConvertGeminiResponseToOpenAIResponses_SimpleMessage(b *testing.B) {
	// Simulate a simple message response
	chunks := [][]byte{
		// Chunk 1: Start
		[]byte(`{"responseId": "resp-123", "createTime": "2023-10-27T10:00:00Z"}`),
		// Chunk 2: Text
		[]byte(`{"candidates": [{"content": {"parts": [{"text": "Hello"}]}}]}`),
		// Chunk 3: Text
		[]byte(`{"candidates": [{"content": {"parts": [{"text": " World"}]}}]}`),
		// Chunk 4: Finish
		[]byte(`{"candidates": [{"finishReason": "STOP"}]}`),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var state any
		for _, chunk := range chunks {
			_ = ConvertGeminiResponseToOpenAIResponses(context.Background(), "gemini-pro", nil, nil, chunk, &state)
		}
	}
}
