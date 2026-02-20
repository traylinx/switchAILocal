package logging

import (
	"testing"
)

func BenchmarkWriteChunkAsync(b *testing.B) {
	// Create a dummy writer
	w := &FileStreamingLogWriter{
		chunkChan: make(chan *[]byte, 100),
	}

	// Start a consumer that mimics asyncWriter's recycling behavior
	go func() {
		for chunkPtr := range w.chunkChan {
			chunkPool.Put(chunkPtr)
		}
	}()

	chunk := []byte("some data chunk")

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		w.WriteChunkAsync(chunk)
	}
	close(w.chunkChan)
}
