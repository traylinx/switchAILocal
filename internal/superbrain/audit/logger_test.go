package audit

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestNewLogger_Disabled(t *testing.T) {
	logger, err := NewLogger(Config{Enabled: false})
	if err != nil {
		t.Fatalf("NewLogger failed: %v", err)
	}

	if logger.enabled {
		t.Error("Expected logger to be disabled")
	}

	// Should be safe to call methods on disabled logger
	logger.LogAction(AuditLogEntry{
		RequestID:  "test-123",
		ActionType: "test_action",
		Provider:   "test",
		Model:      "test-model",
		Outcome:    "success",
	})

	if err := logger.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

func TestNewLogger_Enabled(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "audit.log")

	logger, err := NewLogger(Config{
		Enabled:    true,
		LogPath:    logPath,
		MaxSizeMB:  10,
		MaxBackups: 5,
		MaxAgeDays: 7,
		Compress:   false,
	})
	if err != nil {
		t.Fatalf("NewLogger failed: %v", err)
	}
	defer logger.Close()

	if !logger.enabled {
		t.Error("Expected logger to be enabled")
	}

	if logger.logPath != logPath {
		t.Errorf("Expected logPath %s, got %s", logPath, logger.logPath)
	}
}

func TestNewLogger_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "nested", "dir", "audit.log")

	logger, err := NewLogger(Config{
		Enabled: true,
		LogPath: logPath,
	})
	if err != nil {
		t.Fatalf("NewLogger failed: %v", err)
	}
	defer logger.Close()

	// Verify directory was created
	logDir := filepath.Dir(logPath)
	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		t.Errorf("Expected directory %s to be created", logDir)
	}
}

func TestLogAction_WritesJSONEntry(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "audit.log")

	logger, err := NewLogger(Config{
		Enabled: true,
		LogPath: logPath,
	})
	if err != nil {
		t.Fatalf("NewLogger failed: %v", err)
	}

	// Log an action
	entry := AuditLogEntry{
		Timestamp:  time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC),
		RequestID:  "req-12345",
		ActionType: "stdin_injection",
		Provider:   "claudecli",
		Model:      "claude-sonnet-4",
		ActionDetails: map[string]interface{}{
			"pattern":  "permission_prompt",
			"response": "y\n",
		},
		Outcome:        "success",
		UserIdentifier: "user@example.com",
	}

	logger.LogAction(entry)
	logger.Close()

	// Read and verify the log file
	file, err := os.Open(logPath)
	if err != nil {
		t.Fatalf("Failed to open log file: %v", err)
	}
	defer file.Close()

	var loggedEntry AuditLogEntry
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&loggedEntry); err != nil {
		t.Fatalf("Failed to decode log entry: %v", err)
	}

	// Verify fields
	if loggedEntry.RequestID != entry.RequestID {
		t.Errorf("Expected RequestID %s, got %s", entry.RequestID, loggedEntry.RequestID)
	}
	if loggedEntry.ActionType != entry.ActionType {
		t.Errorf("Expected ActionType %s, got %s", entry.ActionType, loggedEntry.ActionType)
	}
	if loggedEntry.Provider != entry.Provider {
		t.Errorf("Expected Provider %s, got %s", entry.Provider, loggedEntry.Provider)
	}
	if loggedEntry.Model != entry.Model {
		t.Errorf("Expected Model %s, got %s", entry.Model, loggedEntry.Model)
	}
	if loggedEntry.Outcome != entry.Outcome {
		t.Errorf("Expected Outcome %s, got %s", entry.Outcome, loggedEntry.Outcome)
	}
	if loggedEntry.UserIdentifier != entry.UserIdentifier {
		t.Errorf("Expected UserIdentifier %s, got %s", entry.UserIdentifier, loggedEntry.UserIdentifier)
	}

	// Verify action details
	if pattern, ok := loggedEntry.ActionDetails["pattern"].(string); !ok || pattern != "permission_prompt" {
		t.Errorf("Expected pattern 'permission_prompt', got %v", loggedEntry.ActionDetails["pattern"])
	}
	if response, ok := loggedEntry.ActionDetails["response"].(string); !ok || response != "y\n" {
		t.Errorf("Expected response 'y\\n', got %v", loggedEntry.ActionDetails["response"])
	}
}

func TestLogAction_SetsTimestamp(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "audit.log")

	logger, err := NewLogger(Config{
		Enabled: true,
		LogPath: logPath,
	})
	if err != nil {
		t.Fatalf("NewLogger failed: %v", err)
	}

	// Log an action without timestamp
	before := time.Now()
	logger.LogAction(AuditLogEntry{
		RequestID:  "req-12345",
		ActionType: "test_action",
		Provider:   "test",
		Model:      "test-model",
		Outcome:    "success",
	})
	after := time.Now()
	logger.Close()

	// Read and verify timestamp was set
	file, err := os.Open(logPath)
	if err != nil {
		t.Fatalf("Failed to open log file: %v", err)
	}
	defer file.Close()

	var loggedEntry AuditLogEntry
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&loggedEntry); err != nil {
		t.Fatalf("Failed to decode log entry: %v", err)
	}

	if loggedEntry.Timestamp.IsZero() {
		t.Error("Expected timestamp to be set")
	}

	if loggedEntry.Timestamp.Before(before) || loggedEntry.Timestamp.After(after) {
		t.Errorf("Timestamp %v is outside expected range [%v, %v]", loggedEntry.Timestamp, before, after)
	}
}

func TestLogAction_MultipleEntries(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "audit.log")

	logger, err := NewLogger(Config{
		Enabled: true,
		LogPath: logPath,
	})
	if err != nil {
		t.Fatalf("NewLogger failed: %v", err)
	}

	// Log multiple actions
	entries := []AuditLogEntry{
		{
			RequestID:  "req-1",
			ActionType: "stdin_injection",
			Provider:   "claudecli",
			Model:      "claude-sonnet-4",
			Outcome:    "success",
		},
		{
			RequestID:  "req-2",
			ActionType: "restart_with_flags",
			Provider:   "geminicli",
			Model:      "gemini-2.0-flash",
			Outcome:    "success",
		},
		{
			RequestID:  "req-3",
			ActionType: "fallback_routing",
			Provider:   "claudecli",
			Model:      "claude-opus-4",
			Outcome:    "failed",
		},
	}

	for _, entry := range entries {
		logger.LogAction(entry)
	}
	logger.Close()

	// Read and verify all entries
	file, err := os.Open(logPath)
	if err != nil {
		t.Fatalf("Failed to open log file: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineCount := 0
	for scanner.Scan() {
		lineCount++
		var loggedEntry AuditLogEntry
		if err := json.Unmarshal(scanner.Bytes(), &loggedEntry); err != nil {
			t.Fatalf("Failed to decode log entry %d: %v", lineCount, err)
		}

		expectedEntry := entries[lineCount-1]
		if loggedEntry.RequestID != expectedEntry.RequestID {
			t.Errorf("Entry %d: Expected RequestID %s, got %s", lineCount, expectedEntry.RequestID, loggedEntry.RequestID)
		}
		if loggedEntry.ActionType != expectedEntry.ActionType {
			t.Errorf("Entry %d: Expected ActionType %s, got %s", lineCount, expectedEntry.ActionType, loggedEntry.ActionType)
		}
	}

	if lineCount != len(entries) {
		t.Errorf("Expected %d log entries, got %d", len(entries), lineCount)
	}
}

func TestLogStdinInjection(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "audit.log")

	logger, err := NewLogger(Config{
		Enabled: true,
		LogPath: logPath,
	})
	if err != nil {
		t.Fatalf("NewLogger failed: %v", err)
	}

	logger.LogStdinInjection("req-123", "claudecli", "claude-sonnet-4", "permission_prompt", "y\n", "success")
	logger.Close()

	// Verify entry
	file, err := os.Open(logPath)
	if err != nil {
		t.Fatalf("Failed to open log file: %v", err)
	}
	defer file.Close()

	var entry AuditLogEntry
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&entry); err != nil {
		t.Fatalf("Failed to decode log entry: %v", err)
	}

	if entry.ActionType != "stdin_injection" {
		t.Errorf("Expected ActionType 'stdin_injection', got %s", entry.ActionType)
	}
	if entry.RequestID != "req-123" {
		t.Errorf("Expected RequestID 'req-123', got %s", entry.RequestID)
	}
	if entry.Provider != "claudecli" {
		t.Errorf("Expected Provider 'claudecli', got %s", entry.Provider)
	}
	if entry.Model != "claude-sonnet-4" {
		t.Errorf("Expected Model 'claude-sonnet-4', got %s", entry.Model)
	}
	if entry.Outcome != "success" {
		t.Errorf("Expected Outcome 'success', got %s", entry.Outcome)
	}

	pattern, ok := entry.ActionDetails["pattern"].(string)
	if !ok || pattern != "permission_prompt" {
		t.Errorf("Expected pattern 'permission_prompt', got %v", entry.ActionDetails["pattern"])
	}

	response, ok := entry.ActionDetails["response"].(string)
	if !ok || response != "y\n" {
		t.Errorf("Expected response 'y\\n', got %v", entry.ActionDetails["response"])
	}
}

func TestLogRestart(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "audit.log")

	logger, err := NewLogger(Config{
		Enabled: true,
		LogPath: logPath,
	})
	if err != nil {
		t.Fatalf("NewLogger failed: %v", err)
	}

	flags := []string{"--dangerously-skip-permissions", "--no-interactive"}
	logger.LogRestart("req-456", "claudecli", "claude-opus-4", flags, "success")
	logger.Close()

	// Verify entry
	file, err := os.Open(logPath)
	if err != nil {
		t.Fatalf("Failed to open log file: %v", err)
	}
	defer file.Close()

	var entry AuditLogEntry
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&entry); err != nil {
		t.Fatalf("Failed to decode log entry: %v", err)
	}

	if entry.ActionType != "restart_with_flags" {
		t.Errorf("Expected ActionType 'restart_with_flags', got %s", entry.ActionType)
	}

	flagsInterface, ok := entry.ActionDetails["flags"].([]interface{})
	if !ok {
		t.Fatalf("Expected flags to be []interface{}, got %T", entry.ActionDetails["flags"])
	}

	if len(flagsInterface) != len(flags) {
		t.Errorf("Expected %d flags, got %d", len(flags), len(flagsInterface))
	}

	for i, flag := range flags {
		if flagStr, ok := flagsInterface[i].(string); !ok || flagStr != flag {
			t.Errorf("Expected flag %d to be %s, got %v", i, flag, flagsInterface[i])
		}
	}
}

func TestLogFallback(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "audit.log")

	logger, err := NewLogger(Config{
		Enabled: true,
		LogPath: logPath,
	})
	if err != nil {
		t.Fatalf("NewLogger failed: %v", err)
	}

	logger.LogFallback("req-789", "claudecli", "geminicli", "claude-opus-4", "max_retries_exceeded", "success")
	logger.Close()

	// Verify entry
	file, err := os.Open(logPath)
	if err != nil {
		t.Fatalf("Failed to open log file: %v", err)
	}
	defer file.Close()

	var entry AuditLogEntry
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&entry); err != nil {
		t.Fatalf("Failed to decode log entry: %v", err)
	}

	if entry.ActionType != "fallback_routing" {
		t.Errorf("Expected ActionType 'fallback_routing', got %s", entry.ActionType)
	}
	if entry.Provider != "claudecli" {
		t.Errorf("Expected Provider 'claudecli', got %s", entry.Provider)
	}

	fallbackProvider, ok := entry.ActionDetails["fallback_provider"].(string)
	if !ok || fallbackProvider != "geminicli" {
		t.Errorf("Expected fallback_provider 'geminicli', got %v", entry.ActionDetails["fallback_provider"])
	}

	reason, ok := entry.ActionDetails["reason"].(string)
	if !ok || reason != "max_retries_exceeded" {
		t.Errorf("Expected reason 'max_retries_exceeded', got %v", entry.ActionDetails["reason"])
	}
}

func TestLogContextOptimization(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "audit.log")

	logger, err := NewLogger(Config{
		Enabled: true,
		LogPath: logPath,
	})
	if err != nil {
		t.Fatalf("NewLogger failed: %v", err)
	}

	logger.LogContextOptimization("req-999", "geminicli", "gemini-2.0-flash", 100000, 32000, "success")
	logger.Close()

	// Verify entry
	file, err := os.Open(logPath)
	if err != nil {
		t.Fatalf("Failed to open log file: %v", err)
	}
	defer file.Close()

	var entry AuditLogEntry
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&entry); err != nil {
		t.Fatalf("Failed to decode log entry: %v", err)
	}

	if entry.ActionType != "context_optimization" {
		t.Errorf("Expected ActionType 'context_optimization', got %s", entry.ActionType)
	}

	originalTokens, ok := entry.ActionDetails["original_tokens"].(float64)
	if !ok || int(originalTokens) != 100000 {
		t.Errorf("Expected original_tokens 100000, got %v", entry.ActionDetails["original_tokens"])
	}

	optimizedTokens, ok := entry.ActionDetails["optimized_tokens"].(float64)
	if !ok || int(optimizedTokens) != 32000 {
		t.Errorf("Expected optimized_tokens 32000, got %v", entry.ActionDetails["optimized_tokens"])
	}

	tokensSaved, ok := entry.ActionDetails["tokens_saved"].(float64)
	if !ok || int(tokensSaved) != 68000 {
		t.Errorf("Expected tokens_saved 68000, got %v", entry.ActionDetails["tokens_saved"])
	}
}

func TestLogDiagnosis(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "audit.log")

	logger, err := NewLogger(Config{
		Enabled: true,
		LogPath: logPath,
	})
	if err != nil {
		t.Fatalf("NewLogger failed: %v", err)
	}

	logger.LogDiagnosis("req-111", "claudecli", "claude-sonnet-4", "permission_prompt", "stdin_inject", 0.95)
	logger.Close()

	// Verify entry
	file, err := os.Open(logPath)
	if err != nil {
		t.Fatalf("Failed to open log file: %v", err)
	}
	defer file.Close()

	var entry AuditLogEntry
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&entry); err != nil {
		t.Fatalf("Failed to decode log entry: %v", err)
	}

	if entry.ActionType != "diagnosis" {
		t.Errorf("Expected ActionType 'diagnosis', got %s", entry.ActionType)
	}

	failureType, ok := entry.ActionDetails["failure_type"].(string)
	if !ok || failureType != "permission_prompt" {
		t.Errorf("Expected failure_type 'permission_prompt', got %v", entry.ActionDetails["failure_type"])
	}

	remediation, ok := entry.ActionDetails["remediation"].(string)
	if !ok || remediation != "stdin_inject" {
		t.Errorf("Expected remediation 'stdin_inject', got %v", entry.ActionDetails["remediation"])
	}

	confidence, ok := entry.ActionDetails["confidence"].(float64)
	if !ok || confidence != 0.95 {
		t.Errorf("Expected confidence 0.95, got %v", entry.ActionDetails["confidence"])
	}
}

func TestLogSilenceDetection(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "audit.log")

	logger, err := NewLogger(Config{
		Enabled: true,
		LogPath: logPath,
	})
	if err != nil {
		t.Fatalf("NewLogger failed: %v", err)
	}

	logger.LogSilenceDetection("req-222", "claudecli", "claude-opus-4", 30000)
	logger.Close()

	// Verify entry
	file, err := os.Open(logPath)
	if err != nil {
		t.Fatalf("Failed to open log file: %v", err)
	}
	defer file.Close()

	var entry AuditLogEntry
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&entry); err != nil {
		t.Fatalf("Failed to decode log entry: %v", err)
	}

	if entry.ActionType != "silence_detection" {
		t.Errorf("Expected ActionType 'silence_detection', got %s", entry.ActionType)
	}

	silenceDuration, ok := entry.ActionDetails["silence_duration_ms"].(float64)
	if !ok || int64(silenceDuration) != 30000 {
		t.Errorf("Expected silence_duration_ms 30000, got %v", entry.ActionDetails["silence_duration_ms"])
	}
}

func TestGlobal_DefaultDisabled(t *testing.T) {
	// Reset global state for test
	once = sync.Once{}
	globalLogger = nil

	logger := Global()
	if logger.enabled {
		t.Error("Expected global logger to be disabled by default")
	}
}

func TestInitGlobal(t *testing.T) {
	// Reset global state for test
	once = sync.Once{}
	globalLogger = nil

	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "audit.log")

	err := InitGlobal(Config{
		Enabled: true,
		LogPath: logPath,
	})
	if err != nil {
		t.Fatalf("InitGlobal failed: %v", err)
	}

	logger := Global()
	if !logger.enabled {
		t.Error("Expected global logger to be enabled")
	}

	// Test logging with global logger
	logger.LogAction(AuditLogEntry{
		RequestID:  "global-test",
		ActionType: "test_action",
		Provider:   "test",
		Model:      "test-model",
		Outcome:    "success",
	})

	logger.Close()

	// Verify log was written
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		t.Error("Expected log file to exist")
	}
}

func TestLogger_ThreadSafety(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "audit.log")

	logger, err := NewLogger(Config{
		Enabled: true,
		LogPath: logPath,
	})
	if err != nil {
		t.Fatalf("NewLogger failed: %v", err)
	}
	defer logger.Close()

	// Spawn multiple goroutines writing concurrently
	const numGoroutines = 10
	const entriesPerGoroutine = 100

	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			for j := 0; j < entriesPerGoroutine; j++ {
				logger.LogAction(AuditLogEntry{
					RequestID:  "concurrent-test",
					ActionType: "test_action",
					Provider:   "test",
					Model:      "test-model",
					Outcome:    "success",
				})
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	logger.Close()

	// Count entries in log file
	file, err := os.Open(logPath)
	if err != nil {
		t.Fatalf("Failed to open log file: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineCount := 0
	for scanner.Scan() {
		lineCount++
	}

	expectedCount := numGoroutines * entriesPerGoroutine
	if lineCount != expectedCount {
		t.Errorf("Expected %d log entries, got %d", expectedCount, lineCount)
	}
}
