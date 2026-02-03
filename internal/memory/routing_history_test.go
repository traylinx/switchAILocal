package memory

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewRoutingHistoryStore(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "routing-history.jsonl")

	store, err := NewRoutingHistoryStore(filePath)
	if err != nil {
		t.Fatalf("Failed to create routing history store: %v", err)
	}
	defer store.Close()

	// Verify file was created
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Errorf("Routing history file was not created")
	}
}

func TestRecordRouting_Success(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "routing-history.jsonl")

	store, err := NewRoutingHistoryStore(filePath)
	if err != nil {
		t.Fatalf("Failed to create routing history store: %v", err)
	}
	defer store.Close()

	decision := &RoutingDecision{
		Timestamp:  time.Now(),
		APIKeyHash: "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
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

	err = store.RecordRouting(decision)
	if err != nil {
		t.Errorf("Failed to record routing decision: %v", err)
	}

	// Wait for async write to complete
	time.Sleep(100 * time.Millisecond)

	// Verify decision was written
	count, err := store.Count()
	if err != nil {
		t.Errorf("Failed to count routing history: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 decision, got %d", count)
	}
}

func TestRecordRouting_NilDecision(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "routing-history.jsonl")

	store, err := NewRoutingHistoryStore(filePath)
	if err != nil {
		t.Fatalf("Failed to create routing history store: %v", err)
	}
	defer store.Close()

	err = store.RecordRouting(nil)
	if err == nil {
		t.Errorf("Expected error for nil decision, got nil")
	}
}

func TestRecordRouting_EmptyAPIKey(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "routing-history.jsonl")

	store, err := NewRoutingHistoryStore(filePath)
	if err != nil {
		t.Fatalf("Failed to create routing history store: %v", err)
	}
	defer store.Close()

	decision := &RoutingDecision{
		Timestamp:  time.Now(),
		APIKeyHash: "", // Empty API key
		Request: RequestInfo{
			Model: "auto",
		},
	}

	err = store.RecordRouting(decision)
	if err == nil {
		t.Errorf("Expected error for empty API key, got nil")
	}
}

func TestGetHistory_Success(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "routing-history.jsonl")

	store, err := NewRoutingHistoryStore(filePath)
	if err != nil {
		t.Fatalf("Failed to create routing history store: %v", err)
	}
	defer store.Close()

	// Record multiple decisions for different API keys
	apiKey1 := "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	apiKey2 := "sha256:def456"

	for i := 0; i < 5; i++ {
		decision := &RoutingDecision{
			Timestamp:  time.Now(),
			APIKeyHash: apiKey1,
			Request: RequestInfo{
				Model:  "auto",
				Intent: "coding",
			},
			Routing: RoutingInfo{
				SelectedModel: "claudecli:claude-sonnet-4",
			},
		}
		if err := store.RecordRouting(decision); err != nil {
			t.Fatalf("Failed to record decision: %v", err)
		}
	}

	for i := 0; i < 3; i++ {
		decision := &RoutingDecision{
			Timestamp:  time.Now(),
			APIKeyHash: apiKey2,
			Request: RequestInfo{
				Model:  "auto",
				Intent: "reasoning",
			},
			Routing: RoutingInfo{
				SelectedModel: "geminicli:gemini-2.5-pro",
			},
		}
		if err := store.RecordRouting(decision); err != nil {
			t.Fatalf("Failed to record decision: %v", err)
		}
	}

	// Wait for async writes to complete
	time.Sleep(200 * time.Millisecond)

	// Get history for apiKey1
	history, err := store.GetHistory(apiKey1, 10)
	if err != nil {
		t.Errorf("Failed to get history: %v", err)
	}
	if len(history) != 5 {
		t.Errorf("Expected 5 decisions for apiKey1, got %d", len(history))
	}

	// Verify all decisions are for apiKey1
	for _, decision := range history {
		if decision.APIKeyHash != apiKey1 {
			t.Errorf("Expected API key %s, got %s", apiKey1, decision.APIKeyHash)
		}
	}

	// Get history for apiKey2
	history, err = store.GetHistory(apiKey2, 10)
	if err != nil {
		t.Errorf("Failed to get history: %v", err)
	}
	if len(history) != 3 {
		t.Errorf("Expected 3 decisions for apiKey2, got %d", len(history))
	}
}

func TestGetHistory_WithLimit(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "routing-history.jsonl")

	store, err := NewRoutingHistoryStore(filePath)
	if err != nil {
		t.Fatalf("Failed to create routing history store: %v", err)
	}
	defer store.Close()

	apiKey := "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

	// Record 10 decisions
	for i := 0; i < 10; i++ {
		decision := &RoutingDecision{
			Timestamp:  time.Now(),
			APIKeyHash: apiKey,
			Request: RequestInfo{
				Model: "auto",
			},
		}
		if err := store.RecordRouting(decision); err != nil {
			t.Fatalf("Failed to record decision: %v", err)
		}
	}

	// Wait for async writes to complete
	time.Sleep(200 * time.Millisecond)

	// Get history with limit of 5
	history, err := store.GetHistory(apiKey, 5)
	if err != nil {
		t.Errorf("Failed to get history: %v", err)
	}
	if len(history) != 5 {
		t.Errorf("Expected 5 decisions (limit), got %d", len(history))
	}
}

func TestGetHistory_EmptyAPIKey(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "routing-history.jsonl")

	store, err := NewRoutingHistoryStore(filePath)
	if err != nil {
		t.Fatalf("Failed to create routing history store: %v", err)
	}
	defer store.Close()

	_, err = store.GetHistory("", 10)
	if err == nil {
		t.Errorf("Expected error for empty API key, got nil")
	}
}

func TestGetHistory_NonExistentFile(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "nonexistent.jsonl")

	store, err := NewRoutingHistoryStore(filePath)
	if err != nil {
		t.Fatalf("Failed to create routing history store: %v", err)
	}
	defer store.Close()

	// Close and delete the file
	store.Close()
	os.Remove(filePath)

	// Recreate store
	store, err = NewRoutingHistoryStore(filePath)
	if err != nil {
		t.Fatalf("Failed to create routing history store: %v", err)
	}
	defer store.Close()

	// Get history should return empty array, not error
	history, err := store.GetHistory("sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", 10)
	if err != nil {
		t.Errorf("Expected no error for non-existent file, got: %v", err)
	}
	if len(history) != 0 {
		t.Errorf("Expected empty history, got %d decisions", len(history))
	}
}

func TestGetAllHistory(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "routing-history.jsonl")

	store, err := NewRoutingHistoryStore(filePath)
	if err != nil {
		t.Fatalf("Failed to create routing history store: %v", err)
	}
	defer store.Close()

	// Record decisions for multiple API keys
	for i := 0; i < 5; i++ {
		decision := &RoutingDecision{
			Timestamp:  time.Now(),
			APIKeyHash: "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			Request: RequestInfo{
				Model: "auto",
			},
		}
		if err := store.RecordRouting(decision); err != nil {
			t.Fatalf("Failed to record decision: %v", err)
		}
	}

	for i := 0; i < 3; i++ {
		decision := &RoutingDecision{
			Timestamp:  time.Now(),
			APIKeyHash: "sha256:def456",
			Request: RequestInfo{
				Model: "auto",
			},
		}
		if err := store.RecordRouting(decision); err != nil {
			t.Fatalf("Failed to record decision: %v", err)
		}
	}

	// Wait for async writes to complete
	time.Sleep(200 * time.Millisecond)

	// Get all history
	history, err := store.GetAllHistory(100)
	if err != nil {
		t.Errorf("Failed to get all history: %v", err)
	}
	if len(history) != 8 {
		t.Errorf("Expected 8 total decisions, got %d", len(history))
	}
}

func TestRoutingHistoryCount(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "routing-history.jsonl")

	store, err := NewRoutingHistoryStore(filePath)
	if err != nil {
		t.Fatalf("Failed to create routing history store: %v", err)
	}
	defer store.Close()

	// Initial count should be 0
	count, err := store.Count()
	if err != nil {
		t.Errorf("Failed to count: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 initial count, got %d", count)
	}

	// Record 5 decisions
	for i := 0; i < 5; i++ {
		decision := &RoutingDecision{
			Timestamp:  time.Now(),
			APIKeyHash: "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			Request: RequestInfo{
				Model: "auto",
			},
		}
		if err := store.RecordRouting(decision); err != nil {
			t.Fatalf("Failed to record decision: %v", err)
		}
	}

	// Wait for async writes to complete
	time.Sleep(200 * time.Millisecond)

	// Count should be 5
	count, err = store.Count()
	if err != nil {
		t.Errorf("Failed to count: %v", err)
	}
	if count != 5 {
		t.Errorf("Expected 5 count, got %d", count)
	}
}

func TestClose(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "routing-history.jsonl")

	store, err := NewRoutingHistoryStore(filePath)
	if err != nil {
		t.Fatalf("Failed to create routing history store: %v", err)
	}

	// Record a decision
	decision := &RoutingDecision{
		Timestamp:  time.Now(),
		APIKeyHash: "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		Request: RequestInfo{
			Model: "auto",
		},
	}
	if err := store.RecordRouting(decision); err != nil {
		t.Fatalf("Failed to record decision: %v", err)
	}

	// Close should wait for pending writes
	err = store.Close()
	if err != nil {
		t.Errorf("Failed to close store: %v", err)
	}

	// Verify decision was written before close
	store2, err := NewRoutingHistoryStore(filePath)
	if err != nil {
		t.Fatalf("Failed to reopen store: %v", err)
	}
	defer store2.Close()

	count, err := store2.Count()
	if err != nil {
		t.Errorf("Failed to count: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 decision after close, got %d", count)
	}
}

func TestConcurrentWrites(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "routing-history.jsonl")

	store, err := NewRoutingHistoryStore(filePath)
	if err != nil {
		t.Fatalf("Failed to create routing history store: %v", err)
	}
	defer store.Close()

	// Concurrent writes from multiple goroutines
	numGoroutines := 10
	writesPerGoroutine := 10
	done := make(chan bool, numGoroutines)

	for g := 0; g < numGoroutines; g++ {
		go func(goroutineID int) {
			for i := 0; i < writesPerGoroutine; i++ {
				decision := &RoutingDecision{
					Timestamp:  time.Now(),
					APIKeyHash: "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
					Request: RequestInfo{
						Model: "auto",
					},
				}
				if err := store.RecordRouting(decision); err != nil {
					t.Errorf("Goroutine %d: Failed to record decision: %v", goroutineID, err)
				}
			}
			done <- true
		}(g)
	}

	// Wait for all goroutines to complete
	for g := 0; g < numGoroutines; g++ {
		<-done
	}

	// Wait for async writes to complete
	time.Sleep(500 * time.Millisecond)

	// Verify all writes succeeded
	count, err := store.Count()
	if err != nil {
		t.Errorf("Failed to count: %v", err)
	}
	expected := numGoroutines * writesPerGoroutine
	if count != expected {
		t.Errorf("Expected %d decisions, got %d", expected, count)
	}
}

func TestMostRecentFirst(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "routing-history.jsonl")

	store, err := NewRoutingHistoryStore(filePath)
	if err != nil {
		t.Fatalf("Failed to create routing history store: %v", err)
	}
	defer store.Close()

	apiKey := "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

	// Record decisions with increasing timestamps
	timestamps := make([]time.Time, 5)
	for i := 0; i < 5; i++ {
		timestamps[i] = time.Now().Add(time.Duration(i) * time.Second)
		decision := &RoutingDecision{
			Timestamp:  timestamps[i],
			APIKeyHash: apiKey,
			Request: RequestInfo{
				Model: "auto",
			},
		}
		if err := store.RecordRouting(decision); err != nil {
			t.Fatalf("Failed to record decision: %v", err)
		}
		time.Sleep(10 * time.Millisecond) // Small delay to ensure order
	}

	// Wait for async writes to complete
	time.Sleep(200 * time.Millisecond)

	// Get history
	history, err := store.GetHistory(apiKey, 10)
	if err != nil {
		t.Errorf("Failed to get history: %v", err)
	}

	// Verify most recent first (reverse chronological order)
	for i := 0; i < len(history)-1; i++ {
		if history[i].Timestamp.Before(history[i+1].Timestamp) {
			t.Errorf("History not in reverse chronological order: %v before %v",
				history[i].Timestamp, history[i+1].Timestamp)
		}
	}
}
