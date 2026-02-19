package responses

import (
	"context"
	"testing"
)

func BenchmarkConvertGeminiResponseToOpenAIResponses_FunctionCall(b *testing.B) {
	ctx := context.Background()
	rawJSON := []byte(`{
	  "responseId": "resp-stream-123",
	  "candidates": [
	    {
	      "content": {
	        "parts": [
	          { "functionCall": { "name": "get_weather", "args": {"location": "San Francisco"} } }
	        ]
	      },
          "finishReason": "STOP"
	    }
	  ]
	}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var param any // New state every time
		ConvertGeminiResponseToOpenAIResponses(ctx, "gemini-pro", nil, nil, rawJSON, &param)
	}
}
