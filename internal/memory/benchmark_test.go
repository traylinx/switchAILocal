package memory

import (
	"path/filepath"
	"testing"
	"time"
)

// BenchmarkRecordRouting benchmarks the RecordRouting method
// Target: < 5ms per operation (design requirement)
func BenchmarkRecordRouting(b *testing.B) {
	tempDir := b.TempDir()
	filePath := filepath.Join(tempDir, "routing-history.jsonl")

	store, err := NewRoutingHistoryStore(filePath)
	if err != nil {
		b.Fatalf("Failed to create routing history store: %v", err)
	}
	defer store.Close()

	decision := &RoutingDecision{
		Timestamp:  time.Now(),
		APIKeyHash: "sha256:abc123",
		Request: RequestInfo{
			Model:         "auto",
			Intent:        "coding",
			ContentHash:   "sha256:def456",
			ContentLength: 1234,
		},
		Routing: RoutingInfo{
			Tier:          "semantic",
			SelectedModel: "claudecli:claude-sonnet-4",
			Confidence:    0.92,
			LatencyMs:     15,
		},
		Outcome: OutcomeInfo{
			Success:        true,
			ResponseTimeMs: 2340,
			QualityScore:   0.88,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := store.RecordRouting(decision); err != nil {
			b.Fatalf("Failed to record routing decision: %v", err)
		}
	}
}

// BenchmarkGetHistory benchmarks the GetHistory method
// Target: < 10ms per operation (design requirement)
func BenchmarkGetHistory(b *testing.B) {
	tempDir := b.TempDir()
	filePath := filepath.Join(tempDir, "routing-history.jsonl")

	store, err := NewRoutingHistoryStore(filePath)
	if err != nil {
		b.Fatalf("Failed to create routing history store: %v", err)
	}
	defer store.Close()

	// Pre-populate with 1000 decisions
	apiKey := "sha256:abc123"
	for i := 0; i < 1000; i++ {
		decision := &RoutingDecision{
			Timestamp:  time.Now(),
			APIKeyHash: apiKey,
			Request: RequestInfo{
				Model: "auto",
			},
		}
		if err := store.RecordRouting(decision); err != nil {
			b.Fatalf("Failed to record decision: %v", err)
		}
	}

	// Wait for writes to complete
	time.Sleep(100 * time.Millisecond)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := store.GetHistory(apiKey, 100)
		if err != nil {
			b.Fatalf("Failed to get history: %v", err)
		}
	}
}