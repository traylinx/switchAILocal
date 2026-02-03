package memory

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

const (
	// Write queue configuration
	writeQueueSize     = 1000 // Buffer size for async writes
	writeFlushInterval = 5 * time.Second
	writeTimeout       = 10 * time.Second
)

// WriteOperation represents a pending write operation.
type WriteOperation struct {
	decision *RoutingDecision
	errChan  chan error
}

// RoutingHistoryStore manages persistent storage of routing decisions in JSONL format.
// It uses an async write queue with buffered channels to avoid blocking requests.
type RoutingHistoryStore struct {
	filePath   string
	writeQueue chan *WriteOperation
	wg         sync.WaitGroup
	ctx        context.Context
	cancel     context.CancelFunc
	mu         sync.RWMutex
	file       *os.File
}

// NewRoutingHistoryStore creates a new routing history store.
// It initializes the async write queue and starts the background writer.
func NewRoutingHistoryStore(filePath string) (*RoutingHistoryStore, error) {
	ctx, cancel := context.WithCancel(context.Background())

	store := &RoutingHistoryStore{
		filePath:   filePath,
		writeQueue: make(chan *WriteOperation, writeQueueSize),
		ctx:        ctx,
		cancel:     cancel,
	}

	// Open file for appending
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, filePermissions)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to open routing history file: %w", err)
	}
	store.file = file

	// Start background writer
	store.wg.Add(1)
	go store.writeWorker()

	return store, nil
}

// RecordRouting records a routing decision to persistent storage.
// This method is non-blocking and returns immediately after queuing the write.
// If the write queue is full, it returns an error without blocking.
func (rhs *RoutingHistoryStore) RecordRouting(decision *RoutingDecision) error {
	if decision == nil {
		return fmt.Errorf("decision cannot be nil")
	}

	// Comprehensive input validation
	if err := ValidateRoutingDecision(decision); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Check disk space before queuing write (estimate 10KB per decision)
	if err := CheckDiskSpace(rhs.filePath, 10*1024); err != nil {
		return fmt.Errorf("cannot write routing decision: %w", err)
	}

	// Create write operation with buffered error channel
	op := &WriteOperation{
		decision: decision,
		errChan:  make(chan error, 1),
	}

	// Get error channel reference before queuing to avoid race
	errChan := op.errChan

	// Try to queue the write (non-blocking)
	select {
	case rhs.writeQueue <- op:
		// Successfully queued, now safe to wait for result
		select {
		case err := <-errChan:
			return err
		case <-time.After(writeTimeout):
			return fmt.Errorf("write operation timed out")
		case <-rhs.ctx.Done():
			return fmt.Errorf("store is shutting down")
		}
	default:
		// Queue is full, graceful degradation
		return fmt.Errorf("write queue is full, dropping write (queue size: %d)", writeQueueSize)
	}
}

// GetHistory retrieves routing history for a specific API key.
// It reads the entire history file and filters by API key hash.
// The limit parameter controls the maximum number of results returned (most recent first).
func (rhs *RoutingHistoryStore) GetHistory(apiKeyHash string, limit int) ([]*RoutingDecision, error) {
	if apiKeyHash == "" {
		return nil, fmt.Errorf("api_key_hash cannot be empty")
	}

	if limit <= 0 {
		limit = 100 // Default limit
	}

	// Don't hold lock while reading file - only lock to get file path
	// This prevents blocking writes during long read operations
	rhs.mu.RLock()
	filePath := rhs.filePath
	rhs.mu.RUnlock()

	// Open file for reading (without holding lock)
	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist yet, return empty history
			return []*RoutingDecision{}, nil
		}
		return nil, fmt.Errorf("failed to open routing history file: %w", err)
	}
	defer file.Close()

	// Read all matching decisions
	var decisions []*RoutingDecision
	scanner := bufio.NewScanner(file)
	
	// Increase buffer size for large lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var decision RoutingDecision
		if err := json.Unmarshal(line, &decision); err != nil {
			// Log error but continue reading (graceful degradation)
			continue
		}

		// Filter by API key hash
		if decision.APIKeyHash == apiKeyHash {
			decisions = append(decisions, &decision)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading routing history: %w", err)
	}

	// Return most recent decisions first (reverse order)
	result := make([]*RoutingDecision, 0, limit)
	for i := len(decisions) - 1; i >= 0 && len(result) < limit; i-- {
		result = append(result, decisions[i])
	}

	return result, nil
}

// GetAllHistory retrieves all routing history without filtering.
// The limit parameter controls the maximum number of results returned (most recent first).
func (rhs *RoutingHistoryStore) GetAllHistory(limit int) ([]*RoutingDecision, error) {
	if limit <= 0 {
		limit = 100 // Default limit
	}

	// Don't hold lock while reading file - only lock to get file path
	// This prevents blocking writes during long read operations
	rhs.mu.RLock()
	filePath := rhs.filePath
	rhs.mu.RUnlock()

	// Open file for reading (without holding lock)
	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist yet, return empty history
			return []*RoutingDecision{}, nil
		}
		return nil, fmt.Errorf("failed to open routing history file: %w", err)
	}
	defer file.Close()

	// Read all decisions
	var decisions []*RoutingDecision
	scanner := bufio.NewScanner(file)
	
	// Increase buffer size for large lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var decision RoutingDecision
		if err := json.Unmarshal(line, &decision); err != nil {
			// Log error but continue reading (graceful degradation)
			continue
		}

		decisions = append(decisions, &decision)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading routing history: %w", err)
	}

	// Return most recent decisions first (reverse order)
	result := make([]*RoutingDecision, 0, limit)
	for i := len(decisions) - 1; i >= 0 && len(result) < limit; i-- {
		result = append(result, decisions[i])
	}

	return result, nil
}

// writeWorker is the background goroutine that processes write operations.
// It batches writes and flushes periodically to optimize disk I/O.
func (rhs *RoutingHistoryStore) writeWorker() {
	defer rhs.wg.Done()

	ticker := time.NewTicker(writeFlushInterval)
	defer ticker.Stop()

	for {
		select {
		case op := <-rhs.writeQueue:
			// Process write operation
			err := rhs.writeDecision(op.decision)
			
			// Send result back to caller
			select {
			case op.errChan <- err:
			default:
				// Caller not waiting, ignore
			}

		case <-ticker.C:
			// Periodic flush
			rhs.flush()

		case <-rhs.ctx.Done():
			// Shutdown: drain remaining writes
			rhs.drainQueue()
			return
		}
	}
}

// writeDecision writes a single routing decision to the file.
func (rhs *RoutingHistoryStore) writeDecision(decision *RoutingDecision) error {
	rhs.mu.Lock()
	defer rhs.mu.Unlock()

	// Marshal to JSON
	data, err := json.Marshal(decision)
	if err != nil {
		return fmt.Errorf("failed to marshal routing decision: %w", err)
	}

	// Append newline for JSONL format
	data = append(data, '\n')

	// Write to file
	if _, err := rhs.file.Write(data); err != nil {
		return fmt.Errorf("failed to write routing decision: %w", err)
	}

	return nil
}

// flush syncs the file to disk.
func (rhs *RoutingHistoryStore) flush() error {
	rhs.mu.Lock()
	defer rhs.mu.Unlock()

	if rhs.file != nil {
		return rhs.file.Sync()
	}
	return nil
}

// drainQueue processes all remaining write operations during shutdown.
func (rhs *RoutingHistoryStore) drainQueue() {
	for {
		select {
		case op := <-rhs.writeQueue:
			err := rhs.writeDecision(op.decision)
			select {
			case op.errChan <- err:
			default:
			}
		default:
			// Queue is empty
			rhs.flush()
			return
		}
	}
}

// Close gracefully shuts down the routing history store.
// It waits for all pending writes to complete before closing the file.
func (rhs *RoutingHistoryStore) Close() error {
	// Signal shutdown
	rhs.cancel()

	// Wait for write worker to finish
	rhs.wg.Wait()

	// Close file
	rhs.mu.Lock()
	defer rhs.mu.Unlock()

	if rhs.file != nil {
		if err := rhs.file.Close(); err != nil {
			return fmt.Errorf("failed to close routing history file: %w", err)
		}
		rhs.file = nil
	}

	return nil
}

// Count returns the total number of routing decisions in the history.
func (rhs *RoutingHistoryStore) Count() (int, error) {
	rhs.mu.RLock()
	defer rhs.mu.RUnlock()

	file, err := os.Open(rhs.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to open routing history file: %w", err)
	}
	defer file.Close()

	count := 0
	scanner := bufio.NewScanner(file)
	
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		if len(scanner.Bytes()) > 0 {
			count++
		}
	}

	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("error counting routing history: %w", err)
	}

	return count, nil
}
