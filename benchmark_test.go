package main

import (
	"fmt"
	"testing"
)

func emitEventSprintf(event string, payload string) string {
	return fmt.Sprintf("event: %s\ndata: %s", event, payload)
}

func emitEventConcat(event string, payload string) string {
	return "event: " + event + "\ndata: " + payload
}

func BenchmarkEmitEventSprintf(b *testing.B) {
	for i := 0; i < b.N; i++ {
		emitEventSprintf("response.created", `{"id":"resp_123","object":"response.created"}`)
	}
}

func BenchmarkEmitEventConcat(b *testing.B) {
	for i := 0; i < b.N; i++ {
		emitEventConcat("response.created", `{"id":"resp_123","object":"response.created"}`)
	}
}
