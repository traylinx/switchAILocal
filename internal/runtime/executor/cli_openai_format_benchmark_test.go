package executor

import (
	"testing"
	"time"
)

func BenchmarkBuildOpenAIStreamChunkFast(b *testing.B) {
	id := streamChunkID()
	created := time.Now().Unix()
	model := "gpt-4-test-model"
	content := "This is a test content string that simulates a typical streaming chunk from an AI model."

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = BuildOpenAIStreamChunkFast(id, created, model, content, false)
	}
}

func BenchmarkBuildOpenAIStreamFinishChunkFast(b *testing.B) {
	id := streamChunkID()
	created := time.Now().Unix()
	model := "gpt-4-test-model"

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = BuildOpenAIStreamFinishChunkFast(id, created, model)
	}
}
