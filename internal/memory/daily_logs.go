package memory

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// DailyLogsManager handles daily log rotation and archival.
// It automatically rotates logs at midnight and manages log retention.
type DailyLogsManager struct {
	baseDir       string
	retentionDays int
	compression   bool

	// Current log file management
	currentDate   string
	currentFile   *os.File
	currentWriter *bufio.Writer

	// Synchronization
	mu sync.RWMutex

	// Rotation management
	rotationTicker *time.Ticker
	rotationDone   chan struct{}
}

// DailyLogEntry represents a single entry in the daily log.
type DailyLogEntry struct {
	Timestamp time.Time   `json:"timestamp"`
	Type      string      `json:"type"` // "routing", "quirk", "preference_update"
	Data      interface{} `json:"data"`
}

// NewDailyLogsManager creates a new daily logs manager.
func NewDailyLogsManager(baseDir string, retentionDays int, compression bool) (*DailyLogsManager, error) {
	manager := &DailyLogsManager{
		baseDir:       baseDir,
		retentionDays: retentionDays,
		compression:   compression,
		rotationDone:  make(chan struct{}),
	}

	// Initialize current log file
	if err := manager.initializeCurrentLog(); err != nil {
		return nil, fmt.Errorf("failed to initialize current log: %w", err)
	}

	// Start rotation routine
	manager.startRotationRoutine()

	return manager, nil
}

// initializeCurrentLog opens or creates the current day's log file.
func (dlm *DailyLogsManager) initializeCurrentLog() error {
	dlm.mu.Lock()
	defer dlm.mu.Unlock()

	// Close existing file if open
	if dlm.currentFile != nil {
		dlm.flushAndCloseCurrentFile()
	}

	// Get current date
	now := time.Now()
	dlm.currentDate = now.Format("2006-01-02")

	// Open or create today's log file
	logPath := filepath.Join(dlm.baseDir, fmt.Sprintf("%s.jsonl", dlm.currentDate))

	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("failed to open daily log file %s: %w", logPath, err)
	}

	dlm.currentFile = file
	dlm.currentWriter = bufio.NewWriter(file)

	return nil
}

// WriteEntry writes an entry to the current daily log.
// This is thread-safe and non-blocking.
func (dlm *DailyLogsManager) WriteEntry(entryType string, data interface{}) error {
	entry := DailyLogEntry{
		Timestamp: time.Now(),
		Type:      entryType,
		Data:      data,
	}

	// Serialize entry to JSON
	jsonData, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal log entry: %w", err)
	}

	dlm.mu.Lock()
	defer dlm.mu.Unlock()

	// Check if we need to rotate (date changed)
	currentDate := time.Now().Format("2006-01-02")
	if currentDate != dlm.currentDate {
		if err := dlm.rotateLogFile(); err != nil {
			return fmt.Errorf("failed to rotate log file: %w", err)
		}
	}

	// Write entry to current log
	if dlm.currentWriter == nil {
		return fmt.Errorf("no current log writer available")
	}

	if _, err := dlm.currentWriter.Write(jsonData); err != nil {
		return fmt.Errorf("failed to write log entry: %w", err)
	}

	if _, err := dlm.currentWriter.WriteString("\n"); err != nil {
		return fmt.Errorf("failed to write newline: %w", err)
	}

	// Flush periodically for safety
	if err := dlm.currentWriter.Flush(); err != nil {
		return fmt.Errorf("failed to flush log writer: %w", err)
	}

	return nil
}

// rotateLogFile rotates the current log file (called when date changes).
// Must be called with dlm.mu locked.
func (dlm *DailyLogsManager) rotateLogFile() error {
	// Close current file
	dlm.flushAndCloseCurrentFile()

	// Compress previous day's log if enabled
	if dlm.compression {
		previousLogPath := filepath.Join(dlm.baseDir, fmt.Sprintf("%s.jsonl", dlm.currentDate))
		if err := dlm.compressLogFile(previousLogPath); err != nil {
			fmt.Fprintf(os.Stderr, "failed to compress log file %s: %v\n", previousLogPath, err)
		}
	}

	// Initialize new log file for current date
	now := time.Now()
	dlm.currentDate = now.Format("2006-01-02")

	logPath := filepath.Join(dlm.baseDir, fmt.Sprintf("%s.jsonl", dlm.currentDate))

	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("failed to open new daily log file %s: %w", logPath, err)
	}

	dlm.currentFile = file
	dlm.currentWriter = bufio.NewWriter(file)

	return nil
}

// flushAndCloseCurrentFile safely closes the current log file.
// Must be called with dlm.mu locked.
func (dlm *DailyLogsManager) flushAndCloseCurrentFile() {
	if dlm.currentWriter != nil {
		dlm.currentWriter.Flush()
		dlm.currentWriter = nil
	}

	if dlm.currentFile != nil {
		dlm.currentFile.Close()
		dlm.currentFile = nil
	}
}

// compressLogFile compresses a log file using gzip.
func (dlm *DailyLogsManager) compressLogFile(logPath string) error {
	// Check if file exists
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		return nil // File doesn't exist, nothing to compress
	}

	// Open source file
	sourceFile, err := os.Open(logPath)
	if err != nil {
		return fmt.Errorf("failed to open source file %s: %w", logPath, err)
	}
	defer sourceFile.Close()

	// Create compressed file
	compressedPath := logPath + ".gz"
	compressedFile, err := os.Create(compressedPath)
	if err != nil {
		return fmt.Errorf("failed to create compressed file %s: %w", compressedPath, err)
	}
	defer compressedFile.Close()

	// Create gzip writer
	gzipWriter := gzip.NewWriter(compressedFile)
	defer gzipWriter.Close()

	// Copy data
	if _, err := io.Copy(gzipWriter, sourceFile); err != nil {
		return fmt.Errorf("failed to compress file: %w", err)
	}

	// Close gzip writer to flush
	if err := gzipWriter.Close(); err != nil {
		return fmt.Errorf("failed to close gzip writer: %w", err)
	}

	// Remove original file
	if err := os.Remove(logPath); err != nil {
		return fmt.Errorf("failed to remove original file: %w", err)
	}

	return nil
}

// CleanupOldLogs removes log files older than the retention period.
func (dlm *DailyLogsManager) CleanupOldLogs() error {
	dlm.mu.RLock()
	defer dlm.mu.RUnlock()

	// Calculate cutoff date
	cutoffTime := time.Now().AddDate(0, 0, -dlm.retentionDays)
	cutoffDate := cutoffTime.Format("2006-01-02")

	// Read directory entries
	entries, err := os.ReadDir(dlm.baseDir)
	if err != nil {
		return fmt.Errorf("failed to read daily logs directory: %w", err)
	}

	// Process each file
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		fileName := entry.Name()

		// Check if it's a daily log file (YYYY-MM-DD.jsonl or YYYY-MM-DD.jsonl.gz)
		var fileDate string
		if strings.HasSuffix(fileName, ".jsonl") {
			fileDate = strings.TrimSuffix(fileName, ".jsonl")
		} else if strings.HasSuffix(fileName, ".jsonl.gz") {
			fileDate = strings.TrimSuffix(fileName, ".jsonl.gz")
		} else {
			continue // Not a daily log file
		}

		// Validate date format
		if _, err := time.Parse("2006-01-02", fileDate); err != nil {
			continue // Invalid date format
		}

		// Check if file is older than retention period
		if fileDate < cutoffDate {
			filePath := filepath.Join(dlm.baseDir, fileName)
			if err := os.Remove(filePath); err != nil {
				// Log error but continue with other files
				continue
			}
		}
	}

	return nil
}

// GetLogFiles returns a list of all daily log files, sorted by date.
func (dlm *DailyLogsManager) GetLogFiles() ([]string, error) {
	entries, err := os.ReadDir(dlm.baseDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read daily logs directory: %w", err)
	}

	var logFiles []string

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		fileName := entry.Name()

		// Check if it's a daily log file
		if strings.HasSuffix(fileName, ".jsonl") || strings.HasSuffix(fileName, ".jsonl.gz") {
			// Extract date part
			var fileDate string
			if strings.HasSuffix(fileName, ".jsonl") {
				fileDate = strings.TrimSuffix(fileName, ".jsonl")
			} else {
				fileDate = strings.TrimSuffix(fileName, ".jsonl.gz")
			}

			// Validate date format
			if _, err := time.Parse("2006-01-02", fileDate); err == nil {
				logFiles = append(logFiles, fileName)
			}
		}
	}

	// Sort by date (newest first)
	sort.Slice(logFiles, func(i, j int) bool {
		return logFiles[i] > logFiles[j]
	})

	return logFiles, nil
}

// ReadLogFile reads entries from a specific daily log file.
func (dlm *DailyLogsManager) ReadLogFile(fileName string, limit int) ([]*DailyLogEntry, error) {
	filePath := filepath.Join(dlm.baseDir, fileName)

	// Open file (handle both compressed and uncompressed)
	var reader io.Reader

	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file %s: %w", filePath, err)
	}
	defer file.Close()

	if strings.HasSuffix(fileName, ".gz") {
		// Compressed file
		gzipReader, err := gzip.NewReader(file)
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer gzipReader.Close()
		reader = gzipReader
	} else {
		// Uncompressed file
		reader = file
	}

	// Read entries
	var entries []*DailyLogEntry
	scanner := bufio.NewScanner(reader)
	count := 0

	for scanner.Scan() && (limit <= 0 || count < limit) {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		var entry DailyLogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			// Skip invalid entries
			continue
		}

		entries = append(entries, &entry)
		count++
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading log file: %w", err)
	}

	return entries, nil
}

// startRotationRoutine starts the background rotation routine.
func (dlm *DailyLogsManager) startRotationRoutine() {
	// Calculate time until next midnight
	now := time.Now()
	nextMidnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
	timeUntilMidnight := nextMidnight.Sub(now)

	// Start ticker for daily rotation at midnight
	go func() {
		defer close(dlm.rotationDone)

		// Wait until first midnight
		time.Sleep(timeUntilMidnight)

		// Create ticker for daily rotation
		dlm.rotationTicker = time.NewTicker(24 * time.Hour)
		defer dlm.rotationTicker.Stop()

		for {
			select {
			case <-dlm.rotationTicker.C:
				// Perform daily rotation and cleanup
				dlm.mu.Lock()
				if err := dlm.rotateLogFile(); err != nil {
					fmt.Fprintf(os.Stderr, "failed to rotate log file: %v\n", err)
				}
				dlm.mu.Unlock()

				// Cleanup old logs
				if err := dlm.CleanupOldLogs(); err != nil {
					fmt.Fprintf(os.Stderr, "failed to cleanup old logs: %v\n", err)
				}

			case <-dlm.rotationDone:
				return
			}
		}
	}()
}

// Close gracefully shuts down the daily logs manager.
func (dlm *DailyLogsManager) Close() error {
	dlm.mu.Lock()
	defer dlm.mu.Unlock()

	// Stop rotation routine
	if dlm.rotationTicker != nil {
		dlm.rotationTicker.Stop()
		close(dlm.rotationDone)
	}

	// Close current file
	dlm.flushAndCloseCurrentFile()

	return nil
}

// GetStats returns statistics about the daily logs.
func (dlm *DailyLogsManager) GetStats() (*DailyLogsStats, error) {
	logFiles, err := dlm.GetLogFiles()
	if err != nil {
		return nil, fmt.Errorf("failed to get log files: %w", err)
	}

	stats := &DailyLogsStats{
		TotalLogFiles:      len(logFiles),
		RetentionDays:      dlm.retentionDays,
		CompressionEnabled: dlm.compression,
	}

	// Calculate total entries and disk usage
	var totalEntries int
	var totalDiskUsage int64

	for _, fileName := range logFiles {
		filePath := filepath.Join(dlm.baseDir, fileName)

		// Get file size
		if info, err := os.Stat(filePath); err == nil {
			totalDiskUsage += info.Size()
		}

		// Count entries (sample first few files to avoid performance impact)
		if len(logFiles) <= 5 || totalEntries == 0 {
			if entries, err := dlm.ReadLogFile(fileName, -1); err == nil {
				totalEntries += len(entries)
			}
		}
	}

	stats.TotalEntries = totalEntries
	stats.DiskUsageBytes = totalDiskUsage

	// Set oldest and newest dates
	if len(logFiles) > 0 {
		// Extract dates from filenames
		newestFile := logFiles[0]
		oldestFile := logFiles[len(logFiles)-1]

		if strings.HasSuffix(newestFile, ".jsonl") {
			stats.NewestLogDate = strings.TrimSuffix(newestFile, ".jsonl")
		} else if strings.HasSuffix(newestFile, ".jsonl.gz") {
			stats.NewestLogDate = strings.TrimSuffix(newestFile, ".jsonl.gz")
		}

		if strings.HasSuffix(oldestFile, ".jsonl") {
			stats.OldestLogDate = strings.TrimSuffix(oldestFile, ".jsonl")
		} else if strings.HasSuffix(oldestFile, ".jsonl.gz") {
			stats.OldestLogDate = strings.TrimSuffix(oldestFile, ".jsonl.gz")
		}
	}

	return stats, nil
}

// DailyLogsStats provides statistics about the daily logs system.
type DailyLogsStats struct {
	TotalLogFiles      int    `json:"total_log_files"`
	TotalEntries       int    `json:"total_entries"`
	DiskUsageBytes     int64  `json:"disk_usage_bytes"`
	OldestLogDate      string `json:"oldest_log_date,omitempty"`
	NewestLogDate      string `json:"newest_log_date,omitempty"`
	RetentionDays      int    `json:"retention_days"`
	CompressionEnabled bool   `json:"compression_enabled"`
}
