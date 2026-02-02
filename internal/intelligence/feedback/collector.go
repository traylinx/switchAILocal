// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package feedback provides feedback collection and storage for routing decisions.
// It records routing outcomes to enable future learning and system improvement.
package feedback

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3" // SQLite driver
	log "github.com/sirupsen/logrus"

	"github.com/traylinx/switchAILocal/internal/util"
)

// FeedbackRecord represents a single routing feedback entry.
type FeedbackRecord struct {
	ID              int64                  `json:"id"`
	Timestamp       time.Time              `json:"timestamp"`
	Query           string                 `json:"query"`
	Intent          string                 `json:"intent"`
	SelectedModel   string                 `json:"selected_model"`
	RoutingTier     string                 `json:"routing_tier"` // reflex, semantic, cognitive
	Confidence      float64                `json:"confidence"`
	MatchedSkill    string                 `json:"matched_skill,omitempty"`
	CascadeOccurred bool                   `json:"cascade_occurred"`
	ResponseQuality float64                `json:"response_quality,omitempty"`
	LatencyMs       int64                  `json:"latency_ms"`
	Success         bool                   `json:"success"`
	ErrorMessage    string                 `json:"error_message,omitempty"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

// Collector manages feedback collection and storage.
type Collector struct {
	db            *sql.DB
	dbPath        string
	retentionDays int
	enabled       bool
	stateBox      *util.StateBox
	mu            sync.RWMutex
}

// NewCollector creates a new feedback collector instance.
//
// Parameters:
//   - dbPath: Path to the SQLite database file (can be relative or absolute)
//   - retentionDays: Number of days to retain feedback records
//
// Returns:
//   - *Collector: A new collector instance
//   - error: Any error encountered during creation
//
// Note: If a StateBox is set via SetStateBox(), the dbPath will be resolved
// relative to the StateBox intelligence directory.
func NewCollector(dbPath string, retentionDays int) (*Collector, error) {
	if dbPath == "" {
		return nil, fmt.Errorf("database path cannot be empty")
	}

	if retentionDays <= 0 {
		retentionDays = 90 // Default to 90 days
	}

	return &Collector{
		dbPath:        dbPath,
		retentionDays: retentionDays,
		enabled:       false,
	}, nil
}

// SetStateBox configures the State Box for the feedback collector.
// This should be called before Initialize() to ensure the database path
// is resolved correctly within the State Box directory structure.
//
// Parameters:
//   - sb: The StateBox instance to use for path resolution
func (c *Collector) SetStateBox(sb *util.StateBox) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stateBox = sb
}

// Initialize sets up the database and creates necessary tables.
// If a StateBox is configured, the database path is resolved relative to
// the StateBox intelligence directory. In read-only mode, the database
// is opened with SQLITE_OPEN_READONLY flag.
//
// Parameters:
//   - ctx: Context for initialization operations
//
// Returns:
//   - error: Any error encountered during initialization
func (c *Collector) Initialize(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Resolve database path using StateBox if available
	resolvedPath := c.dbPath
	if c.stateBox != nil {
		// If dbPath is just a filename, place it in the intelligence directory
		if filepath.Base(c.dbPath) == c.dbPath {
			resolvedPath = filepath.Join(c.stateBox.IntelligenceDir(), c.dbPath)
		} else {
			// Otherwise, resolve relative to StateBox root
			resolvedPath = c.stateBox.ResolvePath(c.dbPath)
		}
	}

	// Update the dbPath to the resolved path
	c.dbPath = resolvedPath

	// Ensure directory exists (skip in read-only mode)
	dir := filepath.Dir(c.dbPath)
	if c.stateBox != nil && c.stateBox.IsReadOnly() {
		// In read-only mode, verify directory exists but don't create it
		if _, err := os.Stat(dir); err != nil {
			return fmt.Errorf("database directory does not exist in read-only mode: %w", err)
		}
	} else {
		// Create directory with secure permissions (0700)
		if c.stateBox != nil {
			if err := c.stateBox.EnsureDir(dir); err != nil {
				return fmt.Errorf("failed to create database directory: %w", err)
			}
		} else {
			// Fallback to standard mkdir if no StateBox
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("failed to create database directory: %w", err)
			}
		}
	}

	// Open database with appropriate mode
	var db *sql.DB
	var err error

	if c.stateBox != nil && c.stateBox.IsReadOnly() {
		// Open in read-only mode
		dsn := fmt.Sprintf("file:%s?mode=ro", c.dbPath)
		db, err = sql.Open("sqlite3", dsn)
		if err != nil {
			return fmt.Errorf("failed to open database in read-only mode: %w", err)
		}
		log.Infof("Feedback collector initialized in read-only mode (db: %s)", c.dbPath)
	} else {
		// Open in read-write mode
		db, err = sql.Open("sqlite3", c.dbPath)
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}

		// Set connection pool settings
		db.SetMaxOpenConns(1) // SQLite works best with single connection
		db.SetMaxIdleConns(1)

		// Create tables
		schema := `
		CREATE TABLE IF NOT EXISTS feedback (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp DATETIME NOT NULL,
			query TEXT NOT NULL,
			intent TEXT NOT NULL,
			selected_model TEXT NOT NULL,
			routing_tier TEXT NOT NULL,
			confidence REAL,
			matched_skill TEXT,
			cascade_occurred INTEGER NOT NULL DEFAULT 0,
			response_quality REAL,
			latency_ms INTEGER NOT NULL,
			success INTEGER NOT NULL,
			error_message TEXT,
			metadata TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE INDEX IF NOT EXISTS idx_feedback_timestamp ON feedback(timestamp);
		CREATE INDEX IF NOT EXISTS idx_feedback_intent ON feedback(intent);
		CREATE INDEX IF NOT EXISTS idx_feedback_model ON feedback(selected_model);
		CREATE INDEX IF NOT EXISTS idx_feedback_tier ON feedback(routing_tier);
		CREATE INDEX IF NOT EXISTS idx_feedback_created_at ON feedback(created_at);
		`

		if _, err := db.ExecContext(ctx, schema); err != nil {
			db.Close()
			return fmt.Errorf("failed to create schema: %w", err)
		}

		log.Infof("Feedback collector initialized (db: %s, retention: %d days)", c.dbPath, c.retentionDays)

		// Run initial cleanup (only in read-write mode)
		go c.cleanupOldRecords(context.Background())
	}

	c.db = db
	c.enabled = true

	return nil
}

// IsEnabled returns whether the collector is active.
//
// Returns:
//   - bool: true if the collector is enabled
func (c *Collector) IsEnabled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.enabled
}

// Record stores a feedback record in the database.
// In read-only mode, this method returns an error without attempting to insert.
//
// Parameters:
//   - ctx: Context for the operation
//   - record: The feedback record to store
//
// Returns:
//   - error: Any error encountered during storage, including ErrReadOnlyMode
func (c *Collector) Record(ctx context.Context, record *FeedbackRecord) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.enabled {
		return fmt.Errorf("feedback collector not enabled")
	}

	// Check read-only mode
	if c.stateBox != nil && c.stateBox.IsReadOnly() {
		return util.ErrReadOnlyMode
	}

	if record == nil {
		return fmt.Errorf("record cannot be nil")
	}

	// Set timestamp if not provided
	if record.Timestamp.IsZero() {
		record.Timestamp = time.Now()
	}

	// Serialize metadata
	var metadataJSON []byte
	var err error
	if record.Metadata != nil {
		metadataJSON, err = json.Marshal(record.Metadata)
		if err != nil {
			log.Warnf("Failed to marshal metadata: %v", err)
			metadataJSON = []byte("{}")
		}
	}

	// Insert record
	query := `
	INSERT INTO feedback (
		timestamp, query, intent, selected_model, routing_tier,
		confidence, matched_skill, cascade_occurred, response_quality,
		latency_ms, success, error_message, metadata
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	result, err := c.db.ExecContext(ctx, query,
		record.Timestamp,
		record.Query,
		record.Intent,
		record.SelectedModel,
		record.RoutingTier,
		record.Confidence,
		record.MatchedSkill,
		boolToInt(record.CascadeOccurred),
		record.ResponseQuality,
		record.LatencyMs,
		boolToInt(record.Success),
		record.ErrorMessage,
		string(metadataJSON),
	)

	if err != nil {
		return fmt.Errorf("failed to insert feedback: %w", err)
	}

	id, err := result.LastInsertId()
	if err == nil {
		record.ID = id
	}

	return nil
}

// GetRecent retrieves the most recent feedback records.
//
// Parameters:
//   - ctx: Context for the operation
//   - limit: Maximum number of records to retrieve
//
// Returns:
//   - []*FeedbackRecord: The retrieved records
//   - error: Any error encountered during retrieval
func (c *Collector) GetRecent(ctx context.Context, limit int) ([]*FeedbackRecord, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.enabled {
		return nil, fmt.Errorf("feedback collector not enabled")
	}

	if limit <= 0 {
		limit = 100
	}

	query := `
	SELECT id, timestamp, query, intent, selected_model, routing_tier,
	       confidence, matched_skill, cascade_occurred, response_quality,
	       latency_ms, success, error_message, metadata
	FROM feedback
	ORDER BY timestamp DESC
	LIMIT ?
	`

	rows, err := c.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query feedback: %w", err)
	}
	defer rows.Close()

	var records []*FeedbackRecord
	for rows.Next() {
		record, err := scanFeedbackRecord(rows)
		if err != nil {
			log.Warnf("Failed to scan feedback record: %v", err)
			continue
		}
		records = append(records, record)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating feedback records: %w", err)
	}

	return records, nil
}

// GetStats returns aggregated statistics about feedback records.
//
// Parameters:
//   - ctx: Context for the operation
//
// Returns:
//   - map[string]interface{}: Statistics including counts, success rates, etc.
//   - error: Any error encountered during retrieval
func (c *Collector) GetStats(ctx context.Context) (map[string]interface{}, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.enabled {
		return nil, fmt.Errorf("feedback collector not enabled")
	}

	stats := make(map[string]interface{})

	// Total count
	var totalCount int64
	err := c.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM feedback").Scan(&totalCount)
	if err != nil {
		return nil, fmt.Errorf("failed to get total count: %w", err)
	}
	stats["total_records"] = totalCount

	// Success rate
	var successCount int64
	err = c.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM feedback WHERE success = 1").Scan(&successCount)
	if err != nil {
		return nil, fmt.Errorf("failed to get success count: %w", err)
	}
	if totalCount > 0 {
		stats["success_rate"] = float64(successCount) / float64(totalCount)
	} else {
		stats["success_rate"] = 0.0
	}

	// Tier distribution
	tierQuery := `
	SELECT routing_tier, COUNT(*) as count
	FROM feedback
	GROUP BY routing_tier
	`
	rows, err := c.db.QueryContext(ctx, tierQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to get tier distribution: %w", err)
	}
	defer rows.Close()

	tierDist := make(map[string]int64)
	for rows.Next() {
		var tier string
		var count int64
		if err := rows.Scan(&tier, &count); err != nil {
			continue
		}
		tierDist[tier] = count
	}
	stats["tier_distribution"] = tierDist

	// Average latency
	var avgLatency float64
	err = c.db.QueryRowContext(ctx, "SELECT AVG(latency_ms) FROM feedback").Scan(&avgLatency)
	if err != nil {
		return nil, fmt.Errorf("failed to get average latency: %w", err)
	}
	stats["avg_latency_ms"] = avgLatency

	// Cascade rate
	var cascadeCount int64
	err = c.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM feedback WHERE cascade_occurred = 1").Scan(&cascadeCount)
	if err != nil {
		return nil, fmt.Errorf("failed to get cascade count: %w", err)
	}
	if totalCount > 0 {
		stats["cascade_rate"] = float64(cascadeCount) / float64(totalCount)
	} else {
		stats["cascade_rate"] = 0.0
	}

	return stats, nil
}

// cleanupOldRecords removes records older than the retention period.
// NOTE: This function should be called without holding any locks.
func (c *Collector) cleanupOldRecords(ctx context.Context) {
	// Check if enabled without lock
	if !c.IsEnabled() {
		return
	}

	cutoffDate := time.Now().AddDate(0, 0, -c.retentionDays)

	query := "DELETE FROM feedback WHERE created_at < ?"
	result, err := c.db.ExecContext(ctx, query, cutoffDate)
	if err != nil {
		log.Warnf("Failed to cleanup old feedback records: %v", err)
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err == nil && rowsAffected > 0 {
		log.Infof("Cleaned up %d old feedback records (older than %d days)", rowsAffected, c.retentionDays)
	}
}

// Shutdown closes the database connection.
//
// Parameters:
//   - ctx: Context for shutdown operations
//
// Returns:
//   - error: Any error encountered during shutdown
func (c *Collector) Shutdown(ctx context.Context) error {
	// Run final cleanup before acquiring lock
	if c.IsEnabled() {
		c.cleanupOldRecords(ctx)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.enabled {
		return nil
	}

	if c.db != nil {
		if err := c.db.Close(); err != nil {
			return fmt.Errorf("failed to close database: %w", err)
		}
	}

	c.enabled = false
	log.Info("Feedback collector shut down")
	return nil
}

// scanFeedbackRecord scans a database row into a FeedbackRecord.
func scanFeedbackRecord(rows *sql.Rows) (*FeedbackRecord, error) {
	var record FeedbackRecord
	var cascadeInt, successInt int
	var metadataJSON sql.NullString

	err := rows.Scan(
		&record.ID,
		&record.Timestamp,
		&record.Query,
		&record.Intent,
		&record.SelectedModel,
		&record.RoutingTier,
		&record.Confidence,
		&record.MatchedSkill,
		&cascadeInt,
		&record.ResponseQuality,
		&record.LatencyMs,
		&successInt,
		&record.ErrorMessage,
		&metadataJSON,
	)
	if err != nil {
		return nil, err
	}

	record.CascadeOccurred = cascadeInt == 1
	record.Success = successInt == 1

	// Deserialize metadata
	if metadataJSON.Valid && metadataJSON.String != "" {
		if err := json.Unmarshal([]byte(metadataJSON.String), &record.Metadata); err != nil {
			log.Warnf("Failed to unmarshal metadata: %v", err)
		}
	}

	return &record, nil
}

// boolToInt converts a boolean to an integer (0 or 1).
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
