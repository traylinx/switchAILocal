package logging_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/traylinx/switchAILocal/internal/logging"
)

func BenchmarkFileStreamingLogWriter_WriteChunkAsync(b *testing.B) {
	// Create a temporary directory for logs
	logsDir := b.TempDir()

	logger := logging.NewFileRequestLogger(true, logsDir, "")
	writer, err := logger.LogStreamingRequest(
		"http://example.com/api/v1/stream",
		"POST",
		map[string][]string{"Content-Type": {"application/json"}},
		[]byte(`{"prompt": "hello"}`),
		"test-req-id",
	)
	if err != nil {
		b.Fatalf("failed to create streaming writer: %v", err)
	}

	// Prepare a chunk
	chunk := bytes.Repeat([]byte("data: {\"test\":\"value\"}\n\n"), 10)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		writer.WriteChunkAsync(chunk)
		// Small sleep to simulate real streaming and avoid filling the channel instantly
		time.Sleep(1 * time.Microsecond)
	}

	b.StopTimer()

	// Clean up
	err = writer.Close()
	if err != nil {
		b.Fatalf("failed to close writer: %v", err)
	}
}
