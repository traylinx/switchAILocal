package main

import (
	"fmt"
	"testing"
)

func BenchmarkSprintf(b *testing.B) {
	template := `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`
	for i := 0; i < b.N; i++ {
		_ = fmt.Sprintf("data: %s\n\n", template)
	}
}

func BenchmarkConcat(b *testing.B) {
	template := `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`
	for i := 0; i < b.N; i++ {
		_ = "data: " + template + "\n\n"
	}
}
