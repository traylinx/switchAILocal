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

func BenchmarkConvertGeminiResponseToOpenAIResponses_Init(b *testing.B) {
	ctx := context.Background()
	rawJSON := []byte(`{
	  "responseId": "resp-stream-123",
	  "candidates": [
	    {
	      "content": {
	        "parts": [
	          { "text": "Hello" }
	        ]
	      }
	    }
	  ]
	}`)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var param any
		_ = ConvertGeminiResponseToOpenAIResponses(ctx, "gemini-pro", nil, nil, rawJSON, &param)
	}
}

func BenchmarkConvertGeminiResponseToOpenAIResponses_FunctionCall(b *testing.B) {
	ctx := context.Background()
	rawJSON := []byte(`{
	  "responseId": "resp-fc-stream",
	  "candidates": [
	    {
	      "content": {
	        "parts": [
	          {
	            "functionCall": {
	              "name": "get_weather",
	              "args": {"location": "London"}
	            }
	          }
	        ]
	      },
          "finishReason": "STOP"
	    }
	  ]
	}`)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var param any
		_ = ConvertGeminiResponseToOpenAIResponses(ctx, "gemini-pro", nil, nil, rawJSON, &param)
	}
}
