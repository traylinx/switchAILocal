// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package feedback

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestNewCollector tests collector creation.
func TestNewCollector(t *testing.T) {
	tests := []struct {
		name          string
		dbPath        string
		retentionDays int
		wantErr       bool
	}{
		{
			name:          "valid parameters",
			dbPath:        "/tmp/test.db",
			retentionDays: 90,
			wantErr:       false,
		},
		{
			name:          "empty db path",
			dbPath:        "",
			retentionDays: 90,
			wantErr:       true,
		},
		{
			name:          "zero retention days defaults to 90",
			dbPath:        "/tmp/test.db",
			retentionDays: 0,
			wantErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			collector, err := NewCollector(tt.dbPath, tt.retentionDays)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewCollector() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && collector == nil {
				t.Error("NewCollector() returned nil collector")
			}
		})
	}
}

// TestCollectorInitialize tests collector initialization.
func TestCollectorInitialize(t *testing.T) {
	// Create temporary directory for test database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_feedback.db")

	collector, err := NewCollector(dbPath, 90)
	if err != nil {
		t.Fatalf("NewCollector() failed: %v", err)
	}

	ctx := context.Background()
	if err := collector.Initialize(ctx); err != nil {
		t.Fatalf("Initialize() failed: %v", err)
	}

	// Check that collector is enabled
	if !collector.IsEnabled() {
		t.Error("Collector should be enabled after initialization")
	}

	// Check that database file was created
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("Database file was not created")
	}

	// Cleanup
	collector.Shutdown(ctx)
}

// TestCollectorRecord tests recording feedback.
func TestCollectorRecord(t *testing.T) {
	// Create temporary directory for test database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_feedback.db")

	collector, err := NewCollector(dbPath, 90)
	if err != nil {
		t.Fatalf("NewCollector() failed: %v", err)
	}

	ctx := context.Background()
	if err := collector.Initialize(ctx); err != nil {
		t.Fatalf("Initialize() failed: %v", err)
	}
	defer collector.Shutdown(ctx)

	// Test recording a feedback record
	record := &FeedbackRecord{
		Query:           "What is the capital of France?",
		Intent:          "chat",
		SelectedModel:   "gpt-4",
		RoutingTier:     "semantic",
		Confidence:      0.95,
		MatchedSkill:    "general-knowledge",
		CascadeOccurred: false,
		ResponseQuality: 0.9,
		LatencyMs:       150,
		Success:         true,
		Metadata: map[string]interface{}{
			"test": "value",
		},
	}

	if err := collector.Record(ctx, record); err != nil {
		t.Fatalf("Record() failed: %v", err)
	}

	// Verify record was stored
	records, err := collector.GetRecent(ctx, 10)
	if err != nil {
		t.Fatalf("GetRecent() failed: %v", err)
	}

	if len(records) != 1 {
		t.Fatalf("Expected 1 record, got %d", len(records))
	}

	retrieved := records[0]
	if retrieved.Query != record.Query {
		t.Errorf("Query mismatch: got %s, want %s", retrieved.Query, record.Query)
	}
	if retrieved.Intent != record.Intent {
		t.Errorf("Intent mismatch: got %s, want %s", retrieved.Intent, record.Intent)
	}
	if retrieved.SelectedModel != record.SelectedModel {
		t.Errorf("SelectedModel mismatch: got %s, want %s", retrieved.SelectedModel, record.SelectedModel)
	}
	if retrieved.RoutingTier != record.RoutingTier {
		t.Errorf("RoutingTier mismatch: got %s, want %s", retrieved.RoutingTier, record.RoutingTier)
	}
	if retrieved.Success != record.Success {
		t.Errorf("Success mismatch: got %v, want %v", retrieved.Success, record.Success)
	}
}

// TestCollectorGetStats tests retrieving statistics.
func TestCollectorGetStats(t *testing.T) {
	// Create temporary directory for test database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_feedback.db")

	collector, err := NewCollector(dbPath, 90)
	if err != nil {
		t.Fatalf("NewCollector() failed: %v", err)
	}

	ctx := context.Background()
	if err := collector.Initialize(ctx); err != nil {
		t.Fatalf("Initialize() failed: %v", err)
	}
	defer collector.Shutdown(ctx)

	// Record multiple feedback entries
	records := []*FeedbackRecord{
		{
			Query:           "Query 1",
			Intent:          "chat",
			SelectedModel:   "gpt-4",
			RoutingTier:     "semantic",
			Confidence:      0.95,
			LatencyMs:       100,
			Success:         true,
			CascadeOccurred: false,
		},
		{
			Query:           "Query 2",
			Intent:          "coding",
			SelectedModel:   "gpt-4",
			RoutingTier:     "cognitive",
			Confidence:      0.85,
			LatencyMs:       200,
			Success:         true,
			CascadeOccurred: true,
		},
		{
			Query:           "Query 3",
			Intent:          "reasoning",
			SelectedModel:   "gpt-4",
			RoutingTier:     "reflex",
			Confidence:      0.75,
			LatencyMs:       150,
			Success:         false,
			ErrorMessage:    "Test error",
			CascadeOccurred: false,
		},
	}

	for _, record := range records {
		if err := collector.Record(ctx, record); err != nil {
			t.Fatalf("Record() failed: %v", err)
		}
	}

	// Get statistics
	stats, err := collector.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats() failed: %v", err)
	}

	// Verify total count
	totalRecords, ok := stats["total_records"].(int64)
	if !ok {
		t.Fatal("total_records not found in stats")
	}
	if totalRecords != 3 {
		t.Errorf("Expected 3 total records, got %d", totalRecords)
	}

	// Verify success rate
	successRate, ok := stats["success_rate"].(float64)
	if !ok {
		t.Fatal("success_rate not found in stats")
	}
	expectedSuccessRate := 2.0 / 3.0
	if successRate < expectedSuccessRate-0.01 || successRate > expectedSuccessRate+0.01 {
		t.Errorf("Expected success rate ~%.2f, got %.2f", expectedSuccessRate, successRate)
	}

	// Verify tier distribution
	tierDist, ok := stats["tier_distribution"].(map[string]int64)
	if !ok {
		t.Fatal("tier_distribution not found in stats")
	}
	if tierDist["semantic"] != 1 {
		t.Errorf("Expected 1 semantic tier record, got %d", tierDist["semantic"])
	}
	if tierDist["cognitive"] != 1 {
		t.Errorf("Expected 1 cognitive tier record, got %d", tierDist["cognitive"])
	}
	if tierDist["reflex"] != 1 {
		t.Errorf("Expected 1 reflex tier record, got %d", tierDist["reflex"])
	}

	// Verify cascade rate
	cascadeRate, ok := stats["cascade_rate"].(float64)
	if !ok {
		t.Fatal("cascade_rate not found in stats")
	}
	expectedCascadeRate := 1.0 / 3.0
	if cascadeRate < expectedCascadeRate-0.01 || cascadeRate > expectedCascadeRate+0.01 {
		t.Errorf("Expected cascade rate ~%.2f, got %.2f", expectedCascadeRate, cascadeRate)
	}
}

// TestCollectorRetention tests retention policy enforcement.
func TestCollectorRetention(t *testing.T) {
	// This test would require manipulating timestamps in the database
	// For now, we just verify the cleanup function doesn't error
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_feedback.db")

	collector, err := NewCollector(dbPath, 1) // 1 day retention
	if err != nil {
		t.Fatalf("NewCollector() failed: %v", err)
	}

	ctx := context.Background()
	if err := collector.Initialize(ctx); err != nil {
		t.Fatalf("Initialize() failed: %v", err)
	}
	defer collector.Shutdown(ctx)

	// Record a feedback entry
	record := &FeedbackRecord{
		Query:         "Test query",
		Intent:        "chat",
		SelectedModel: "gpt-4",
		RoutingTier:   "semantic",
		LatencyMs:     100,
		Success:       true,
	}

	if err := collector.Record(ctx, record); err != nil {
		t.Fatalf("Record() failed: %v", err)
	}

	// Manually trigger cleanup (normally happens on shutdown)
	collector.cleanupOldRecords(ctx)

	// Verify no error occurred
	// In a real scenario, we'd need to manipulate created_at timestamps
	// to test actual deletion of old records
}

// TestCollectorNotEnabled tests operations when collector is not enabled.
func TestCollectorNotEnabled(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_feedback.db")

	collector, err := NewCollector(dbPath, 90)
	if err != nil {
		t.Fatalf("NewCollector() failed: %v", err)
	}

	// Don't initialize - collector should not be enabled
	if collector.IsEnabled() {
		t.Error("Collector should not be enabled before initialization")
	}

	ctx := context.Background()

	// Try to record - should fail
	record := &FeedbackRecord{
		Query:         "Test query",
		Intent:        "chat",
		SelectedModel: "gpt-4",
		RoutingTier:   "semantic",
		LatencyMs:     100,
		Success:       true,
	}

	err = collector.Record(ctx, record)
	if err == nil {
		t.Error("Record() should fail when collector is not enabled")
	}

	// Try to get stats - should fail
	_, err = collector.GetStats(ctx)
	if err == nil {
		t.Error("GetStats() should fail when collector is not enabled")
	}

	// Try to get recent - should fail
	_, err = collector.GetRecent(ctx, 10)
	if err == nil {
		t.Error("GetRecent() should fail when collector is not enabled")
	}
}

// TestCollectorShutdown tests graceful shutdown.
func TestCollectorShutdown(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_feedback.db")

	collector, err := NewCollector(dbPath, 90)
	if err != nil {
		t.Fatalf("NewCollector() failed: %v", err)
	}

	ctx := context.Background()
	if err := collector.Initialize(ctx); err != nil {
		t.Fatalf("Initialize() failed: %v", err)
	}

	// Record some data
	record := &FeedbackRecord{
		Query:         "Test query",
		Intent:        "chat",
		SelectedModel: "gpt-4",
		RoutingTier:   "semantic",
		LatencyMs:     100,
		Success:       true,
	}

	if err := collector.Record(ctx, record); err != nil {
		t.Fatalf("Record() failed: %v", err)
	}

	// Shutdown
	if err := collector.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() failed: %v", err)
	}

	// Verify collector is no longer enabled
	if collector.IsEnabled() {
		t.Error("Collector should not be enabled after shutdown")
	}

	// Verify operations fail after shutdown
	err = collector.Record(ctx, record)
	if err == nil {
		t.Error("Record() should fail after shutdown")
	}
}

// TestCollectorTimestamp tests automatic timestamp setting.
func TestCollectorTimestamp(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_feedback.db")

	collector, err := NewCollector(dbPath, 90)
	if err != nil {
		t.Fatalf("NewCollector() failed: %v", err)
	}

	ctx := context.Background()
	if err := collector.Initialize(ctx); err != nil {
		t.Fatalf("Initialize() failed: %v", err)
	}
	defer collector.Shutdown(ctx)

	// Record without timestamp
	record := &FeedbackRecord{
		Query:         "Test query",
		Intent:        "chat",
		SelectedModel: "gpt-4",
		RoutingTier:   "semantic",
		LatencyMs:     100,
		Success:       true,
	}

	before := time.Now()
	if err := collector.Record(ctx, record); err != nil {
		t.Fatalf("Record() failed: %v", err)
	}
	after := time.Now()

	// Retrieve and verify timestamp was set
	records, err := collector.GetRecent(ctx, 1)
	if err != nil {
		t.Fatalf("GetRecent() failed: %v", err)
	}

	if len(records) != 1 {
		t.Fatalf("Expected 1 record, got %d", len(records))
	}

	retrieved := records[0]
	if retrieved.Timestamp.Before(before) || retrieved.Timestamp.After(after) {
		t.Errorf("Timestamp %v not within expected range [%v, %v]", retrieved.Timestamp, before, after)
	}
}
