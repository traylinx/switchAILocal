package responses

import (
	"context"
	"strings"
	"testing"
)

func BenchmarkConvertGeminiResponseToOpenAIResponses_FunctionCalls(b *testing.B) {
	ctx := context.Background()

	// Simulate a finish reason which triggers the function call sorting logic
	rawJSON := []byte(`{
	  "candidates": [
	    {
	      "finishReason": "STOP"
	    }
	  ]
	}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		st := &geminiToResponsesState{
			FuncArgsBuf: make([]*strings.Builder, 10),
			FuncNames:   make([]string, 10),
			FuncCallIDs: make([]string, 10),
			Started:     true,
		}

		// Add 10 function calls
		for k := 0; k < 10; k++ {
			sb := &strings.Builder{}
			sb.WriteString("{}")
			st.FuncArgsBuf[k] = sb
			st.FuncNames[k] = "test_func"
			st.FuncCallIDs[k] = "call_123"
		}

		var param any = st
		ConvertGeminiResponseToOpenAIResponses(ctx, "gemini-pro", nil, nil, rawJSON, &param)
	}
}
