package responses

import (
	"context"
	"testing"
)

func BenchmarkEmitEvent(b *testing.B) {
	evt := OutputTextDelta{
		Type:           "response.output_text.delta",
		SequenceNumber: 123,
		ItemID:         "msg_123",
		OutputIndex:    0,
		ContentIndex:   0,
		Delta:          "Hello World",
		Logprobs:       []any{},
	}

	st := &geminiToResponsesState{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = st.emit("response.output_text.delta", evt)
	}
}

func BenchmarkConvertGeminiResponseToOpenAIResponses_FuncCalls(b *testing.B) {
	// Simulate a Gemini response with multiple function calls
	rawJSON := []byte(`{
		"candidates": [{
			"content": {
				"parts": [
					{"functionCall": {"name": "func1", "args": {"arg1": "value1"}}},
					{"functionCall": {"name": "func2", "args": {"arg2": "value2"}}},
					{"functionCall": {"name": "func3", "args": {"arg3": "value3"}}},
					{"functionCall": {"name": "func4", "args": {"arg4": "value4"}}},
					{"functionCall": {"name": "func5", "args": {"arg5": "value5"}}}
				]
			}
		}],
		"responseId": "resp_1234567890",
		"createTime": "2024-01-01T00:00:00Z"
	}`)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		var state any
		ConvertGeminiResponseToOpenAIResponses(context.Background(), "gemini-1.5-pro", nil, nil, rawJSON, &state)
	}
}

func BenchmarkConvertGeminiResponseToOpenAIResponses_Streaming_FuncCalls(b *testing.B) {
	// Simulate a streaming scenario where we receive function call chunks
	chunk1 := []byte(`{
		"candidates": [{"content": {"parts": [{"functionCall": {"name": "func1", "args": {"arg1": "v"}}}]}}]
	}`)
	chunk2 := []byte(`{
		"candidates": [{"content": {"parts": [{"functionCall": {"name": "func1", "args": {"arg1": "alue1"}}}]}}]
	}`)
	chunk3 := []byte(`{
		"candidates": [{"content": {"parts": [{"functionCall": {"name": "func2", "args": {"arg2": "v"}}}]}}]
	}`)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		var state any
		ConvertGeminiResponseToOpenAIResponses(context.Background(), "gemini-1.5-pro", nil, nil, chunk1, &state)
		ConvertGeminiResponseToOpenAIResponses(context.Background(), "gemini-1.5-pro", nil, nil, chunk2, &state)
		ConvertGeminiResponseToOpenAIResponses(context.Background(), "gemini-1.5-pro", nil, nil, chunk3, &state)
	}
}
