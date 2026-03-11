package logging

import (
	"os"
	"testing"
)

func BenchmarkFileStreamingLogWriter_AsyncWrites(b *testing.B) {
	file, err := os.CreateTemp("", "bench-log-*.tmp")
	if err != nil {
		b.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(file.Name())

	writer := &FileStreamingLogWriter{
		responseBodyFile: file,
		chunkChan:        make(chan *[]byte, 100),
		closeChan:        make(chan struct{}),
		errorChan:        make(chan error, 1),
	}

	go writer.asyncWriter()

	chunk := make([]byte, 1024) // 1KB chunk
	for i := range chunk {
		chunk[i] = 'A'
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		writer.WriteChunkAsync(chunk)
	}

	// Wait for processing to finish
	writer.Close()
}
