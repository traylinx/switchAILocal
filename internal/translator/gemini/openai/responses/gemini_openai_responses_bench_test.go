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

func BenchmarkConvertGeminiResponseToOpenAIResponses(b *testing.B) {
	ctx := context.Background()
	var param any

	// Typical chunk with some text
	rawJSON := []byte(`{
	  "candidates": [
	    {
	      "content": {
	        "parts": [
	          { "text": "Hello world this is a test of performance" }
	        ]
	      }
	    }
	  ]
	}`)

	// Initialize param once
	ConvertGeminiResponseToOpenAIResponses(ctx, "gemini-pro", nil, nil, rawJSON, &param)
	st := param.(*geminiToResponsesState)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		st.TextBuf.Reset()
		ConvertGeminiResponseToOpenAIResponses(ctx, "gemini-pro", nil, nil, rawJSON, &param)
	}
}
