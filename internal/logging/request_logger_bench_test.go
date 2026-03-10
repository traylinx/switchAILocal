package logging

import (
	"os"
	"testing"
)

func BenchmarkFileStreamingLogWriter_WriteChunkAsync(b *testing.B) {
	tmpFile, err := os.CreateTemp("", "bench-log-*.tmp")
	if err != nil {
		b.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	writer := &FileStreamingLogWriter{
		responseBodyFile: tmpFile,
		chunkChan:        make(chan *[]byte, 1000),
		closeChan:        make(chan struct{}),
		errorChan:        make(chan error, 1),
	}

	go writer.asyncWriter()

	chunk := []byte("hello world chunk data")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		writer.WriteChunkAsync(chunk)
	}

	b.StopTimer()
	writer.Close()
}
