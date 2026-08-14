package test

import (
	"fmt"
	"strings"
	"testing"
)

func emitEventSprintf(event string, payload string) string {
	return fmt.Sprintf("event: %s\ndata: %s", event, payload)
}

func emitEventConcat(event string, payload string) string {
	return "event: " + event + "\ndata: " + payload
}

func emitEventBuilder(event string, payload string) string {
	var b strings.Builder
	b.Grow(14 + len(event) + len(payload))
	b.WriteString("event: ")
	b.WriteString(event)
	b.WriteString("\ndata: ")
	b.WriteString(payload)
	return b.String()
}

func BenchmarkEmitEventSprintf(b *testing.B) {
	event := "response.output_text.delta"
	payload := `{"type":"response.output_text.delta","sequence_number":123,"item_id":"msg_123","output_index":0,"content_index":0,"delta":"Hello","logprobs":[]}`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		emitEventSprintf(event, payload)
	}
}

func BenchmarkEmitEventConcat(b *testing.B) {
	event := "response.output_text.delta"
	payload := `{"type":"response.output_text.delta","sequence_number":123,"item_id":"msg_123","output_index":0,"content_index":0,"delta":"Hello","logprobs":[]}`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		emitEventConcat(event, payload)
	}
}

func BenchmarkEmitEventBuilder(b *testing.B) {
	event := "response.output_text.delta"
	payload := `{"type":"response.output_text.delta","sequence_number":123,"item_id":"msg_123","output_index":0,"content_index":0,"delta":"Hello","logprobs":[]}`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		emitEventBuilder(event, payload)
	}
}
