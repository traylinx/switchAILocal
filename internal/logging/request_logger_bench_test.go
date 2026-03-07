package logging

import (
	"os"
	"testing"
)

func BenchmarkFileStreamingLogWriter_WriteChunkAsync(b *testing.B) {
	tempFile, err := os.CreateTemp("", "bench-log-*.tmp")
	if err != nil {
		b.Fatal(err)
	}
	defer os.Remove(tempFile.Name())

	writer := &FileStreamingLogWriter{
		chunkChan:        make(chan *[]byte, 100),
		closeChan:        make(chan struct{}),
		errorChan:        make(chan error, 1),
		responseBodyFile: tempFile,
	}

	go writer.asyncWriter()

	chunk := []byte("hello world this is a test chunk that we are writing to the log file repeatedly")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		writer.WriteChunkAsync(chunk)
	}

	b.StopTimer()
	writer.Close()
}
