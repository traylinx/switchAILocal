package responses

import (
	"context"
	"testing"
)

func BenchmarkConvertGeminiResponseToOpenAIResponses_FunctionCall(b *testing.B) {
	ctx := context.Background()

	// Chunk 1: Function call start
	chunk1 := []byte(`{
	  "candidates": [
	    {
	      "content": {
	        "parts": [
	          {
	            "functionCall": {
	              "name": "get_weather",
	              "args": {"location": "Lon"}
	            }
	          }
	        ]
	      }
	    }
	  ]
	}`)

	// Chunk 2: Function call args continuation
	chunk2 := []byte(`{
	  "candidates": [
	    {
	      "content": {
	        "parts": [
	          {
	            "functionCall": {
	              "name": "get_weather",
	              "args": {"don"}
	            }
	          }
	        ]
	      }
	    }
	  ]
	}`)

	// Chunk 3: Function call args continuation + finish
	chunk3 := []byte(`{
	  "candidates": [
	    {
	      "content": {
	        "parts": [
	          {
	            "functionCall": {
	              "name": "get_weather",
	              "args": {""}
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
		// Simulate streaming sequence
		ConvertGeminiResponseToOpenAIResponses(ctx, "gemini-pro", nil, nil, chunk1, &param)
		ConvertGeminiResponseToOpenAIResponses(ctx, "gemini-pro", nil, nil, chunk2, &param)
		ConvertGeminiResponseToOpenAIResponses(ctx, "gemini-pro", nil, nil, chunk3, &param)
	}
}
