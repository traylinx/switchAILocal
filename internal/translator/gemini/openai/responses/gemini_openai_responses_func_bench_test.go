package responses

import (
	"context"
	"testing"
)

func BenchmarkConvertGeminiResponseToOpenAIResponses_FunctionCalls(b *testing.B) {
	// Simulate a function call streaming response
	chunks := [][]byte{
		[]byte(`{"responseId": "resp_123", "createTime": "2024-01-01T00:00:00Z"}`),
		[]byte(`{"candidates": [{"content": {"parts": [{"functionCall": {"name": "get_weather", "args": {"location": "San Francisco"}}}]}}]}`),
		[]byte(`{"candidates": [{"content": {"parts": [{"functionCall": {"args": {"unit": "celsius"}}}]}}]}`),
		[]byte(`{"candidates": [{"finishReason": "STOP"}]}`),
	}

	ctx := context.Background()
	model := "gemini-pro"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var state any
		for _, chunk := range chunks {
			_ = ConvertGeminiResponseToOpenAIResponses(ctx, model, nil, nil, chunk, &state)
		}
	}
}

func BenchmarkConvertGeminiResponseToOpenAIResponses_MultipleFunctionCalls(b *testing.B) {
	// Simulate multiple parallel function calls
	chunks := [][]byte{
		[]byte(`{"responseId": "resp_parallel", "createTime": "2024-01-01T00:00:00Z"}`),
		// First function call
		[]byte(`{"candidates": [{"content": {"parts": [{"functionCall": {"name": "func1", "args": {"arg1": "val1"}}}]}}]}`),
		// Second function call
		[]byte(`{"candidates": [{"content": {"parts": [{"functionCall": {"name": "func2", "args": {"arg2": "val2"}}}]}}]}`),
		// More args for func1
		[]byte(`{"candidates": [{"content": {"parts": [{"functionCall": {"args": {"arg1_cont": "ued"}}}]}}]}`), // Note: Gemini usually sends updates to the last one or by index, but here we simulate simplistic appending if the logic allows or just sequential calls.
		// Wait, the current logic relies on order. If multiple parts come in one chunk or sequential chunks...
		// The current implementation appends to the *last* function call if no name is present?
		// Actually, let's look at the implementation.
		// It checks `if fc := part.Get("functionCall"); fc.Exists()`.
		// If `name` is present, it starts a new function call (increments NextIndex).
		// If `name` is NOT present? The current implementation assumes name is always present for a new call?
		// Wait, `name := fc.Get("name").String()`. If name is empty...
		// `st.FuncNames[idx] = name`.
		// It seems it treats every `functionCall` part as a NEW function call if it appears in `parts`.
		// Does Gemini stream arguments across multiple chunks for the SAME function call?
		// Yes, but the structure in `parts` would be different?
		// The current implementation seems to assume each `functionCall` in `parts` is a distinct item in the output list.
		// Let's stick to the single function call streaming for now, as that's the most common case to optimize.
		[]byte(`{"candidates": [{"finishReason": "STOP"}]}`),
	}

	ctx := context.Background()
	model := "gemini-pro"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var state any
		for _, chunk := range chunks {
			_ = ConvertGeminiResponseToOpenAIResponses(ctx, model, nil, nil, chunk, &state)
		}
	}
}
