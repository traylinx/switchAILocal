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

func BenchmarkConvertGeminiResponseToOpenAIResponses_FuncCall(b *testing.B) {
	ctx := context.Background()
	rawJSON := []byte(`{
      "responseId": "resp-123",
      "candidates": [
        {
          "content": {
            "parts": [
              {
                "functionCall": {
                  "name": "get_weather",
                  "args": {"location": "San Francisco", "unit": "celsius"}
                }
              },
              {
                 "functionCall": {
                    "name": "get_time",
                    "args": {"location": "San Francisco"}
                 }
              }
            ]
          },
          "finishReason": "STOP"
        }
      ]
    }`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var param any
		_ = ConvertGeminiResponseToOpenAIResponses(ctx, "gemini-pro", nil, nil, rawJSON, &param)
	}
}
