package responses

import (
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
