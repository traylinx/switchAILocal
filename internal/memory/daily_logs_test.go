package memory

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDailyLogsManager_WriteEntry(t *testing.T) {
	tempDir := t.TempDir()

	manager, err := NewDailyLogsManager(tempDir, 90, false)
	if err != nil {
		t.Fatalf("Failed to create daily logs manager: %v", err)
	}
	defer manager.Close()

	// Write test entries
	testData := map[string]interface{}{
		"test_key": "test_value",
		"number":   42,
	}

	err = manager.WriteEntry("test", testData)
	if err != nil {
		t.Fatalf("Failed to write entry: %v", err)
	}

	// Verify file was created
	today := time.Now().Format("2006-01-02")
	logFile := filepath.Join(tempDir, today+".jsonl")

	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		t.Fatalf("Log file was not created: %s", logFile)
	}

	// Read and verify content
	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	if len(lines) != 1 {
		t.Fatalf("Expected 1 line, got %d", len(lines))
	}

	var entry DailyLogEntry
	if err := json.Unmarshal([]byte(lines[0]), &entry); err != nil {
		t.Fatalf("Failed to unmarshal entry: %v", err)
	}

	if entry.Type != "test" {
		t.Errorf("Expected type 'test', got '%s'", entry.Type)
	}

	if entry.Timestamp.IsZero() {
		t.Error("Expected non-zero timestamp")
	}
}

func TestDailyLogsManager_MultipleEntries(t *testing.T) {
	tempDir := t.TempDir()

	manager, err := NewDailyLogsManager(tempDir, 90, false)
	if err != nil {
		t.Fatalf("Failed to create daily logs manager: %v", err)
	}
	defer manager.Close()

	// Write multiple entries
	for i := 0; i < 5; i++ {
		testData := map[string]interface{}{
			"entry_number": i,
			"timestamp":    time.Now(),
		}

		err = manager.WriteEntry("test", testData)
		if err != nil {
			t.Fatalf("Failed to write entry %d: %v", i, err)
		}
	}

	// Read entries back
	today := time.Now().Format("2006-01-02")
	entries, err := manager.ReadLogFile(today+".jsonl", -1)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	if len(entries) != 5 {
		t.Fatalf("Expected 5 entries, got %d", len(entries))
	}

	// Verify entries are in order
	for i, entry := range entries {
		if entry.Type != "test" {
			t.Errorf("Entry %d: expected type 'test', got '%s'", i, entry.Type)
		}

		data, ok := entry.Data.(map[string]interface{})
		if !ok {
			t.Errorf("Entry %d: expected map data", i)
			continue
		}

		entryNum, ok := data["entry_number"].(float64)
		if !ok || int(entryNum) != i {
			t.Errorf("Entry %d: expected entry_number %d, got %v", i, i, entryNum)
		}
	}
}

func TestDailyLogsManager_Compression(t *testing.T) {
	tempDir := t.TempDir()

	manager, err := NewDailyLogsManager(tempDir, 90, true)
	if err != nil {
		t.Fatalf("Failed to create daily logs manager: %v", err)
	}
	defer manager.Close()

	// Create a test log file
	testFile := filepath.Join(tempDir, "2023-01-01.jsonl")
	testContent := `{"timestamp":"2023-01-01T12:00:00Z","type":"test","data":{"key":"value"}}
{"timestamp":"2023-01-01T12:01:00Z","type":"test","data":{"key":"value2"}}`

	err = os.WriteFile(testFile, []byte(testContent), 0600)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Compress the file
	err = manager.compressLogFile(testFile)
	if err != nil {
		t.Fatalf("Failed to compress file: %v", err)
	}

	// Verify original file is gone
	if _, err := os.Stat(testFile); !os.IsNotExist(err) {
		t.Error("Original file should be deleted after compression")
	}

	// Verify compressed file exists
	compressedFile := testFile + ".gz"
	if _, err := os.Stat(compressedFile); os.IsNotExist(err) {
		t.Fatal("Compressed file should exist")
	}

	// Verify compressed content
	file, err := os.Open(compressedFile)
	if err != nil {
		t.Fatalf("Failed to open compressed file: %v", err)
	}
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatalf("Failed to create gzip reader: %v", err)
	}
	defer gzipReader.Close()

	decompressed, err := io.ReadAll(gzipReader)
	if err != nil {
		t.Fatalf("Failed to read compressed content: %v", err)
	}

	if string(decompressed) != testContent {
		t.Error("Decompressed content doesn't match original")
	}
}

func TestDailyLogsManager_ReadCompressedFile(t *testing.T) {
	tempDir := t.TempDir()

	manager, err := NewDailyLogsManager(tempDir, 90, true)
	if err != nil {
		t.Fatalf("Failed to create daily logs manager: %v", err)
	}
	defer manager.Close()

	// Create and compress a test log file
	testFile := filepath.Join(tempDir, "2023-01-01.jsonl")
	testEntries := []DailyLogEntry{
		{
			Timestamp: time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
			Type:      "test",
			Data:      map[string]interface{}{"key": "value1"},
		},
		{
			Timestamp: time.Date(2023, 1, 1, 12, 1, 0, 0, time.UTC),
			Type:      "test",
			Data:      map[string]interface{}{"key": "value2"},
		},
	}

	// Write test entries to file
	file, err := os.Create(testFile)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	for _, entry := range testEntries {
		jsonData, _ := json.Marshal(entry)
		_, _ = file.WriteString(string(jsonData) + "\n")
	}
	file.Close()

	// Compress the file
	err = manager.compressLogFile(testFile)
	if err != nil {
		t.Fatalf("Failed to compress file: %v", err)
	}

	// Read compressed file
	entries, err := manager.ReadLogFile("2023-01-01.jsonl.gz", -1)
	if err != nil {
		t.Fatalf("Failed to read compressed file: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("Expected 2 entries, got %d", len(entries))
	}

	// Verify entries
	for i, entry := range entries {
		if entry.Type != "test" {
			t.Errorf("Entry %d: expected type 'test', got '%s'", i, entry.Type)
		}

		expectedKey := "value" + string(rune('1'+i))
		data, ok := entry.Data.(map[string]interface{})
		if !ok {
			t.Errorf("Entry %d: expected map data", i)
			continue
		}

		if data["key"] != expectedKey {
			t.Errorf("Entry %d: expected key '%s', got '%v'", i, expectedKey, data["key"])
		}
	}
}

func TestDailyLogsManager_CleanupOldLogs(t *testing.T) {
	tempDir := t.TempDir()

	manager, err := NewDailyLogsManager(tempDir, 7, false) // 7 days retention
	if err != nil {
		t.Fatalf("Failed to create daily logs manager: %v", err)
	}
	defer manager.Close()

	// Create test log files with different dates
	testFiles := []string{
		"2023-01-01.jsonl",    // Old (should be deleted)
		"2023-01-02.jsonl.gz", // Old compressed (should be deleted)
		time.Now().AddDate(0, 0, -5).Format("2006-01-02") + ".jsonl", // Recent (should be kept)
		time.Now().Format("2006-01-02") + ".jsonl",                   // Today (should be kept)
	}

	for _, fileName := range testFiles {
		filePath := filepath.Join(tempDir, fileName)
		err := os.WriteFile(filePath, []byte("test content"), 0600)
		if err != nil {
			t.Fatalf("Failed to create test file %s: %v", fileName, err)
		}
	}

	// Run cleanup
	err = manager.CleanupOldLogs()
	if err != nil {
		t.Fatalf("Failed to cleanup old logs: %v", err)
	}

	// Check which files remain
	entries, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatalf("Failed to read directory: %v", err)
	}

	remainingFiles := make(map[string]bool)
	for _, entry := range entries {
		remainingFiles[entry.Name()] = true
	}

	// Old files should be deleted
	if remainingFiles["2023-01-01.jsonl"] {
		t.Error("Old file 2023-01-01.jsonl should be deleted")
	}
	if remainingFiles["2023-01-02.jsonl.gz"] {
		t.Error("Old compressed file 2023-01-02.jsonl.gz should be deleted")
	}

	// Recent files should remain
	recentFile := time.Now().AddDate(0, 0, -5).Format("2006-01-02") + ".jsonl"
	if !remainingFiles[recentFile] {
		t.Errorf("Recent file %s should be kept", recentFile)
	}

	todayFile := time.Now().Format("2006-01-02") + ".jsonl"
	if !remainingFiles[todayFile] {
		t.Errorf("Today's file %s should be kept", todayFile)
	}
}

func TestDailyLogsManager_GetLogFiles(t *testing.T) {
	tempDir := t.TempDir()

	manager, err := NewDailyLogsManager(tempDir, 90, false)
	if err != nil {
		t.Fatalf("Failed to create daily logs manager: %v", err)
	}
	defer manager.Close()

	// Create test log files
	testFiles := []string{
		"2023-01-01.jsonl",
		"2023-01-02.jsonl.gz",
		"2023-01-03.jsonl",
		"not-a-log-file.txt", // Should be ignored
		"invalid-date.jsonl", // Should be ignored
	}

	for _, fileName := range testFiles {
		filePath := filepath.Join(tempDir, fileName)
		err := os.WriteFile(filePath, []byte("test content"), 0600)
		if err != nil {
			t.Fatalf("Failed to create test file %s: %v", fileName, err)
		}
	}

	// Get log files
	logFiles, err := manager.GetLogFiles()
	if err != nil {
		t.Fatalf("Failed to get log files: %v", err)
	}

	// Should return 3 valid log files plus today's file, sorted by date (newest first)
	// The manager creates today's log file automatically
	expectedMinFiles := 3

	if len(logFiles) < expectedMinFiles {
		t.Fatalf("Expected at least %d log files, got %d", expectedMinFiles, len(logFiles))
	}

	// Check that the expected test files are present
	expectedTestFiles := []string{
		"2023-01-03.jsonl",
		"2023-01-02.jsonl.gz",
		"2023-01-01.jsonl",
	}

	foundTestFiles := 0
	for _, logFile := range logFiles {
		for _, expectedFile := range expectedTestFiles {
			if logFile == expectedFile {
				foundTestFiles++
				break
			}
		}
	}

	if foundTestFiles != len(expectedTestFiles) {
		t.Errorf("Expected to find all %d test files, found %d", len(expectedTestFiles), foundTestFiles)
	}
}

func TestDailyLogsManager_GetStats(t *testing.T) {
	tempDir := t.TempDir()

	manager, err := NewDailyLogsManager(tempDir, 90, true)
	if err != nil {
		t.Fatalf("Failed to create daily logs manager: %v", err)
	}
	defer manager.Close()

	// Write some test entries
	for i := 0; i < 10; i++ {
		testData := map[string]interface{}{
			"entry": i,
		}
		err = manager.WriteEntry("test", testData)
		if err != nil {
			t.Fatalf("Failed to write entry %d: %v", i, err)
		}
	}

	// Get stats
	stats, err := manager.GetStats()
	if err != nil {
		t.Fatalf("Failed to get stats: %v", err)
	}

	// Verify stats
	if stats.TotalLogFiles != 1 {
		t.Errorf("Expected 1 log file, got %d", stats.TotalLogFiles)
	}

	if stats.TotalEntries != 10 {
		t.Errorf("Expected 10 entries, got %d", stats.TotalEntries)
	}

	if stats.RetentionDays != 90 {
		t.Errorf("Expected retention days 90, got %d", stats.RetentionDays)
	}

	if !stats.CompressionEnabled {
		t.Error("Expected compression to be enabled")
	}

	if stats.DiskUsageBytes <= 0 {
		t.Error("Expected positive disk usage")
	}

	// Verify date fields
	today := time.Now().Format("2006-01-02")
	if stats.NewestLogDate != today {
		t.Errorf("Expected newest log date '%s', got '%s'", today, stats.NewestLogDate)
	}

	if stats.OldestLogDate != today {
		t.Errorf("Expected oldest log date '%s', got '%s'", today, stats.OldestLogDate)
	}
}

func TestDailyLogsManager_ConcurrentWrites(t *testing.T) {
	tempDir := t.TempDir()

	manager, err := NewDailyLogsManager(tempDir, 90, false)
	if err != nil {
		t.Fatalf("Failed to create daily logs manager: %v", err)
	}
	defer manager.Close()

	// Write entries concurrently
	const numGoroutines = 10
	const entriesPerGoroutine = 10

	done := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(goroutineID int) {
			for j := 0; j < entriesPerGoroutine; j++ {
				testData := map[string]interface{}{
					"goroutine": goroutineID,
					"entry":     j,
				}

				if err := manager.WriteEntry("concurrent_test", testData); err != nil {
					done <- err
					return
				}
			}
			done <- nil
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		if err := <-done; err != nil {
			t.Fatalf("Concurrent write failed: %v", err)
		}
	}

	// Verify all entries were written
	today := time.Now().Format("2006-01-02")
	entries, err := manager.ReadLogFile(today+".jsonl", -1)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	expectedEntries := numGoroutines * entriesPerGoroutine
	if len(entries) != expectedEntries {
		t.Fatalf("Expected %d entries, got %d", expectedEntries, len(entries))
	}

	// Verify all entries are valid
	for i, entry := range entries {
		if entry.Type != "concurrent_test" {
			t.Errorf("Entry %d: expected type 'concurrent_test', got '%s'", i, entry.Type)
		}

		data, ok := entry.Data.(map[string]interface{})
		if !ok {
			t.Errorf("Entry %d: expected map data", i)
			continue
		}

		if _, hasGoroutine := data["goroutine"]; !hasGoroutine {
			t.Errorf("Entry %d: missing goroutine field", i)
		}

		if _, hasEntry := data["entry"]; !hasEntry {
			t.Errorf("Entry %d: missing entry field", i)
		}
	}
}

func TestDailyLogsManager_ReadWithLimit(t *testing.T) {
	tempDir := t.TempDir()

	manager, err := NewDailyLogsManager(tempDir, 90, false)
	if err != nil {
		t.Fatalf("Failed to create daily logs manager: %v", err)
	}
	defer manager.Close()

	// Write 20 entries
	for i := 0; i < 20; i++ {
		testData := map[string]interface{}{
			"entry": i,
		}
		err = manager.WriteEntry("test", testData)
		if err != nil {
			t.Fatalf("Failed to write entry %d: %v", i, err)
		}
	}

	// Read with limit
	today := time.Now().Format("2006-01-02")
	entries, err := manager.ReadLogFile(today+".jsonl", 5)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	if len(entries) != 5 {
		t.Fatalf("Expected 5 entries, got %d", len(entries))
	}

	// Verify we got the first 5 entries
	for i, entry := range entries {
		data, ok := entry.Data.(map[string]interface{})
		if !ok {
			t.Errorf("Entry %d: expected map data", i)
			continue
		}

		entryNum, ok := data["entry"].(float64)
		if !ok || int(entryNum) != i {
			t.Errorf("Entry %d: expected entry number %d, got %v", i, i, entryNum)
		}
	}
}
