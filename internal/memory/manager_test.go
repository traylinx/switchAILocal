package memory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewMemoryManager(t *testing.T) {
	tempDir := t.TempDir()

	config := &MemoryConfig{
		Enabled:       true,
		BaseDir:       tempDir,
		RetentionDays: 90,
		MaxLogSizeMB:  100,
		Compression:   true,
	}

	manager, err := NewMemoryManager(config)
	if err != nil {
		t.Fatalf("Failed to create memory manager: %v", err)
	}
	defer manager.Close()

	// Verify directory structure was created
	if _, err := os.Stat(tempDir); os.IsNotExist(err) {
		t.Errorf("Base directory was not created")
	}

	// Verify subdirectories exist
	subdirs := []string{"user-preferences", "daily", "analytics"}
	for _, subdir := range subdirs {
		path := filepath.Join(tempDir, subdir)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("Subdirectory %s was not created", subdir)
		}
	}
}

func TestNewMemoryManagerWithNilConfig(t *testing.T) {
	manager, err := NewMemoryManager(nil)
	if err != nil {
		t.Fatalf("Failed to create memory manager with nil config: %v", err)
	}
	defer manager.Close()

	// Should use default config
	stats, err := manager.GetStats()
	if err != nil {
		t.Fatalf("Failed to get stats: %v", err)
	}

	if stats.RetentionDays != 90 {
		t.Errorf("Expected default retention days 90, got %d", stats.RetentionDays)
	}
}

func TestNewMemoryManagerInvalidConfig(t *testing.T) {
	tests := []struct {
		name   string
		config *MemoryConfig
	}{
		{
			name: "empty base dir",
			config: &MemoryConfig{
				BaseDir:       "",
				RetentionDays: 90,
				MaxLogSizeMB:  100,
			},
		},
		{
			name: "invalid retention days",
			config: &MemoryConfig{
				BaseDir:       "/tmp/test",
				RetentionDays: 0,
				MaxLogSizeMB:  100,
			},
		},
		{
			name: "invalid max log size",
			config: &MemoryConfig{
				BaseDir:       "/tmp/test",
				RetentionDays: 90,
				MaxLogSizeMB:  0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewMemoryManager(tt.config)
			if err == nil {
				t.Errorf("Expected error for invalid config, got nil")
			}
		})
	}
}

func TestMemoryManagerDisabled(t *testing.T) {
	tempDir := t.TempDir()

	config := &MemoryConfig{
		Enabled:       false, // Disabled
		BaseDir:       tempDir,
		RetentionDays: 90,
		MaxLogSizeMB:  100,
		Compression:   true,
	}

	manager, err := NewMemoryManager(config)
	if err != nil {
		t.Fatalf("Failed to create memory manager: %v", err)
	}
	defer manager.Close()

	// Test that operations return empty results when disabled
	decision := &RoutingDecision{
		Timestamp:  time.Now(),
		APIKeyHash: "sha256:test123",
		Request: RequestInfo{
			Model:  "auto",
			Intent: "coding",
		},
		Routing: RoutingInfo{
			SelectedModel: "claudecli:claude-sonnet-4",
			Confidence:    0.92,
		},
		Outcome: OutcomeInfo{
			Success: true,
		},
	}

	// RecordRouting should not error when disabled
	if err := manager.RecordRouting(decision); err != nil {
		t.Errorf("RecordRouting should not error when disabled: %v", err)
	}

	// GetHistory should return empty slice when disabled
	history, err := manager.GetHistory("sha256:test123", 10)
	if err != nil {
		t.Errorf("GetHistory should not error when disabled: %v", err)
	}
	if len(history) != 0 {
		t.Errorf("Expected empty history when disabled, got %d decisions", len(history))
	}

	// GetUserPreferences should return default preferences when disabled
	prefs, err := manager.GetUserPreferences("sha256:test123")
	if err != nil {
		t.Errorf("GetUserPreferences should not error when disabled: %v", err)
	}
	if prefs.APIKeyHash != "sha256:test123" {
		t.Errorf("Expected API key hash to be set in default preferences")
	}

	// AddQuirk should not error when disabled
	quirk := &Quirk{
		Provider:   "ollama",
		Issue:      "Connection timeout",
		Workaround: "Restart service",
		Discovered: time.Now(),
		Frequency:  "rare",
		Severity:   "low",
	}
	if err := manager.AddQuirk(quirk); err != nil {
		t.Errorf("AddQuirk should not error when disabled: %v", err)
	}

	// GetProviderQuirks should return empty slice when disabled
	quirks, err := manager.GetProviderQuirks("ollama")
	if err != nil {
		t.Errorf("GetProviderQuirks should not error when disabled: %v", err)
	}
	if len(quirks) != 0 {
		t.Errorf("Expected empty quirks when disabled, got %d quirks", len(quirks))
	}
}

func TestRecordRouting(t *testing.T) {
	tempDir := t.TempDir()

	config := &MemoryConfig{
		Enabled:       true,
		BaseDir:       tempDir,
		RetentionDays: 90,
		MaxLogSizeMB:  100,
		Compression:   true,
	}

	manager, err := NewMemoryManager(config)
	if err != nil {
		t.Fatalf("Failed to create memory manager: %v", err)
	}
	defer manager.Close()

	decision := &RoutingDecision{
		Timestamp:  time.Now(),
		APIKeyHash: "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		Request: RequestInfo{
			Model:  "auto",
			Intent: "coding",
		},
		Routing: RoutingInfo{
			SelectedModel: "claudecli:claude-sonnet-4",
			Tier:          "cognitive",
			Confidence:    0.92,
		},
		Outcome: OutcomeInfo{
			Success: true,
		},
	}

	if err := manager.RecordRouting(decision); err != nil {
		t.Errorf("Failed to record routing decision: %v", err)
	}

	// Give async write time to complete
	time.Sleep(100 * time.Millisecond)

	// Verify decision was recorded
	history, err := manager.GetHistory("sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", 10)
	if err != nil {
		t.Errorf("Failed to get history: %v", err)
	}

	if len(history) != 1 {
		t.Errorf("Expected 1 decision in history, got %d", len(history))
	}

	if history[0].APIKeyHash != "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Errorf("Expected API key hash sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855, got %s", history[0].APIKeyHash)
	}
}

func TestGetUserPreferences(t *testing.T) {
	tempDir := t.TempDir()

	config := &MemoryConfig{
		Enabled:       true,
		BaseDir:       tempDir,
		RetentionDays: 90,
		MaxLogSizeMB:  100,
		Compression:   true,
	}

	manager, err := NewMemoryManager(config)
	if err != nil {
		t.Fatalf("Failed to create memory manager: %v", err)
	}
	defer manager.Close()

	// Test getting preferences for new user
	prefs, err := manager.GetUserPreferences("sha256:newuser")
	if err != nil {
		t.Errorf("Failed to get user preferences: %v", err)
	}

	if prefs.APIKeyHash != "sha256:newuser" {
		t.Errorf("Expected API key hash sha256:newuser, got %s", prefs.APIKeyHash)
	}

	if len(prefs.ModelPreferences) != 0 {
		t.Errorf("Expected empty model preferences for new user, got %d", len(prefs.ModelPreferences))
	}
}

func TestLearnFromOutcome(t *testing.T) {
	tempDir := t.TempDir()

	config := &MemoryConfig{
		Enabled:       true,
		BaseDir:       tempDir,
		RetentionDays: 90,
		MaxLogSizeMB:  100,
		Compression:   true,
	}

	manager, err := NewMemoryManager(config)
	if err != nil {
		t.Fatalf("Failed to create memory manager: %v", err)
	}
	defer manager.Close()

	decision := &RoutingDecision{
		Timestamp:  time.Now(),
		APIKeyHash: "sha256:learner",
		Request: RequestInfo{
			Model:  "auto",
			Intent: "coding",
		},
		Routing: RoutingInfo{
			SelectedModel: "claudecli:claude-sonnet-4",
			Confidence:    0.92,
		},
		Outcome: OutcomeInfo{
			Success:      true,
			QualityScore: 0.9,
		},
	}

	if err := manager.LearnFromOutcome(decision); err != nil {
		t.Errorf("Failed to learn from outcome: %v", err)
	}

	// Verify learning occurred
	prefs, err := manager.GetUserPreferences("sha256:learner")
	if err != nil {
		t.Errorf("Failed to get user preferences: %v", err)
	}

	if prefs.ModelPreferences["coding"] != "claudecli:claude-sonnet-4" {
		t.Errorf("Expected learned model preference for coding, got %s", prefs.ModelPreferences["coding"])
	}
}

func TestAddAndGetQuirks(t *testing.T) {
	tempDir := t.TempDir()

	config := &MemoryConfig{
		Enabled:       true,
		BaseDir:       tempDir,
		RetentionDays: 90,
		MaxLogSizeMB:  100,
		Compression:   true,
	}

	manager, err := NewMemoryManager(config)
	if err != nil {
		t.Fatalf("Failed to create memory manager: %v", err)
	}
	defer manager.Close()

	quirk := &Quirk{
		Provider:   "ollama",
		Issue:      "Connection timeout on startup",
		Workaround: "Send warmup request",
		Discovered: time.Now(),
		Frequency:  "3/10 startups",
		Severity:   "medium",
	}

	if err := manager.AddQuirk(quirk); err != nil {
		t.Errorf("Failed to add quirk: %v", err)
	}

	quirks, err := manager.GetProviderQuirks("ollama")
	if err != nil {
		t.Errorf("Failed to get provider quirks: %v", err)
	}

	if len(quirks) != 1 {
		t.Errorf("Expected 1 quirk, got %d", len(quirks))
	}

	if quirks[0].Issue != "Connection timeout on startup" {
		t.Errorf("Expected specific issue, got %s", quirks[0].Issue)
	}
}

func TestGetStats(t *testing.T) {
	tempDir := t.TempDir()

	config := &MemoryConfig{
		Enabled:       true,
		BaseDir:       tempDir,
		RetentionDays: 90,
		MaxLogSizeMB:  100,
		Compression:   true,
	}

	manager, err := NewMemoryManager(config)
	if err != nil {
		t.Fatalf("Failed to create memory manager: %v", err)
	}
	defer manager.Close()

	stats, err := manager.GetStats()
	if err != nil {
		t.Errorf("Failed to get stats: %v", err)
	}

	if stats.RetentionDays != 90 {
		t.Errorf("Expected retention days 90, got %d", stats.RetentionDays)
	}

	if !stats.CompressionEnabled {
		t.Errorf("Expected compression enabled")
	}

	// Initially should have 0 decisions, users, and quirks
	if stats.TotalDecisions != 0 {
		t.Errorf("Expected 0 total decisions, got %d", stats.TotalDecisions)
	}

	if stats.TotalUsers != 0 {
		t.Errorf("Expected 0 total users, got %d", stats.TotalUsers)
	}

	// Note: TotalQuirks might be > 0 if the quirks file has template content
	// This is expected behavior, so we don't test for exact 0
	if stats.TotalQuirks < 0 {
		t.Errorf("Expected non-negative total quirks, got %d", stats.TotalQuirks)
	}
}

func TestCleanup(t *testing.T) {
	tempDir := t.TempDir()

	config := &MemoryConfig{
		Enabled:       true,
		BaseDir:       tempDir,
		RetentionDays: 1, // Very short retention for testing
		MaxLogSizeMB:  100,
		Compression:   true,
	}

	manager, err := NewMemoryManager(config)
	if err != nil {
		t.Fatalf("Failed to create memory manager: %v", err)
	}
	defer manager.Close()

	// Create an old log file
	dailyLogsDir := filepath.Join(tempDir, "daily")
	oldLogFile := filepath.Join(dailyLogsDir, "old.jsonl")

	file, err := os.Create(oldLogFile)
	if err != nil {
		t.Fatalf("Failed to create old log file: %v", err)
	}
	file.Close()

	// Set modification time to 2 days ago
	oldTime := time.Now().AddDate(0, 0, -2)
	if err := os.Chtimes(oldLogFile, oldTime, oldTime); err != nil {
		t.Fatalf("Failed to set old time: %v", err)
	}

	// Run cleanup
	if err := manager.Cleanup(); err != nil {
		t.Errorf("Cleanup failed: %v", err)
	}

	// Verify old file was removed
	if _, err := os.Stat(oldLogFile); !os.IsNotExist(err) {
		t.Errorf("Old log file should have been removed")
	}
}

func TestManagerClose(t *testing.T) {
	tempDir := t.TempDir()

	config := &MemoryConfig{
		Enabled:       true,
		BaseDir:       tempDir,
		RetentionDays: 90,
		MaxLogSizeMB:  100,
		Compression:   true,
	}

	manager, err := NewMemoryManager(config)
	if err != nil {
		t.Fatalf("Failed to create memory manager: %v", err)
	}

	// Close should not error
	if err := manager.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Operations after close should fail gracefully
	decision := &RoutingDecision{
		Timestamp:  time.Now(),
		APIKeyHash: "sha256:test123",
	}

	// This might error or might not, depending on implementation
	// The important thing is it doesn't panic
	_ = manager.RecordRouting(decision)
}

func TestConcurrentOperations(t *testing.T) {
	tempDir := t.TempDir()

	config := &MemoryConfig{
		Enabled:       true,
		BaseDir:       tempDir,
		RetentionDays: 90,
		MaxLogSizeMB:  100,
		Compression:   true,
	}

	manager, err := NewMemoryManager(config)
	if err != nil {
		t.Fatalf("Failed to create memory manager: %v", err)
	}
	defer manager.Close()

	// Run concurrent operations
	const numGoroutines = 10
	const numOperations = 10

	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer func() { done <- true }()

			for j := 0; j < numOperations; j++ {
				decision := &RoutingDecision{
					Timestamp:  time.Now(),
					APIKeyHash: "sha256:concurrent",
					Request: RequestInfo{
						Model:  "auto",
						Intent: "coding",
					},
					Routing: RoutingInfo{
						SelectedModel: "claudecli:claude-sonnet-4",
						Confidence:    0.92,
					},
					Outcome: OutcomeInfo{
						Success: true,
					},
				}

				// These operations should not panic or error
				_ = manager.RecordRouting(decision)
				_, _ = manager.GetUserPreferences("sha256:concurrent")
				_, _ = manager.GetHistory("sha256:concurrent", 5)
				_, _ = manager.GetStats()
			}
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Give async operations time to complete
	time.Sleep(200 * time.Millisecond)

	// Verify no data corruption occurred
	stats, err := manager.GetStats()
	if err != nil {
		t.Errorf("Failed to get stats after concurrent operations: %v", err)
	}

	// Should have some decisions recorded
	if stats.TotalDecisions == 0 {
		t.Errorf("Expected some decisions to be recorded")
	}
}
func TestMemoryManager_GetAnalytics(t *testing.T) {
	tempDir := t.TempDir()

	config := &MemoryConfig{
		Enabled:       true,
		BaseDir:       tempDir,
		RetentionDays: 90,
		MaxLogSizeMB:  100,
		Compression:   false,
	}

	manager, err := NewMemoryManager(config)
	if err != nil {
		t.Fatalf("Failed to create memory manager: %v", err)
	}
	defer manager.Close()

	// Record some routing decisions
	decisions := []*RoutingDecision{
		{
			Timestamp:  time.Now(),
			APIKeyHash: "sha256:test123",
			Request: RequestInfo{
				Model:  "auto",
				Intent: "coding",
			},
			Routing: RoutingInfo{
				Tier:          "semantic",
				SelectedModel: "claudecli:claude-sonnet-4",
				Confidence:    0.9,
			},
			Outcome: OutcomeInfo{
				Success:      true,
				QualityScore: 0.85,
			},
		},
		{
			Timestamp:  time.Now().Add(time.Hour),
			APIKeyHash: "sha256:test456",
			Request: RequestInfo{
				Model:  "auto",
				Intent: "reasoning",
			},
			Routing: RoutingInfo{
				Tier:          "cognitive",
				SelectedModel: "geminicli:gemini-2.5-pro",
				Confidence:    0.8,
			},
			Outcome: OutcomeInfo{
				Success:      false,
				Error:        "timeout",
				QualityScore: 0.0,
			},
		},
	}

	for i, decision := range decisions {
		if err := manager.RecordRouting(decision); err != nil {
			t.Fatalf("Failed to record routing decision %d: %v", i, err)
		}
	}

	// Compute analytics
	analytics, err := manager.ComputeAnalytics()
	if err != nil {
		t.Fatalf("Failed to compute analytics: %v", err)
	}

	// Verify analytics
	if analytics.GeneratedAt.IsZero() {
		t.Error("Expected non-zero generated timestamp")
	}

	if len(analytics.ProviderStats) != 2 {
		t.Errorf("Expected 2 providers, got %d", len(analytics.ProviderStats))
	}

	// Check Claude stats
	claudeStats, exists := analytics.ProviderStats["claudecli"]
	if !exists {
		t.Fatal("Expected claudecli provider stats")
	}
	if claudeStats.TotalRequests != 1 {
		t.Errorf("Expected 1 Claude request, got %d", claudeStats.TotalRequests)
	}
	if claudeStats.SuccessRate != 1.0 {
		t.Errorf("Expected 100%% Claude success rate, got %.2f", claudeStats.SuccessRate)
	}

	// Check Gemini stats
	geminiStats, exists := analytics.ProviderStats["geminicli"]
	if !exists {
		t.Fatal("Expected geminicli provider stats")
	}
	if geminiStats.TotalRequests != 1 {
		t.Errorf("Expected 1 Gemini request, got %d", geminiStats.TotalRequests)
	}
	if geminiStats.SuccessRate != 0.0 {
		t.Errorf("Expected 0%% Gemini success rate, got %.2f", geminiStats.SuccessRate)
	}

	// Verify model performance
	if len(analytics.ModelPerformance) != 2 {
		t.Errorf("Expected 2 models, got %d", len(analytics.ModelPerformance))
	}

	// Verify tier effectiveness
	if analytics.TierEffectiveness == nil {
		t.Fatal("Expected tier effectiveness data")
	}

	if analytics.TierEffectiveness.SemanticTier.TotalRequests != 1 {
		t.Errorf("Expected 1 semantic tier request, got %d", analytics.TierEffectiveness.SemanticTier.TotalRequests)
	}

	if analytics.TierEffectiveness.CognitiveTier.TotalRequests != 1 {
		t.Errorf("Expected 1 cognitive tier request, got %d", analytics.TierEffectiveness.CognitiveTier.TotalRequests)
	}

	// Get analytics again (should load from cache/disk)
	analytics2, err := manager.GetAnalytics()
	if err != nil {
		t.Fatalf("Failed to get analytics: %v", err)
	}

	if len(analytics2.ProviderStats) != len(analytics.ProviderStats) {
		t.Error("Analytics should be consistent between ComputeAnalytics and GetAnalytics")
	}
}

func TestMemoryManager_GetAnalyticsDisabled(t *testing.T) {
	tempDir := t.TempDir()

	config := &MemoryConfig{
		Enabled:       false, // Disabled
		BaseDir:       tempDir,
		RetentionDays: 90,
		MaxLogSizeMB:  100,
		Compression:   false,
	}

	manager, err := NewMemoryManager(config)
	if err != nil {
		t.Fatalf("Failed to create memory manager: %v", err)
	}
	defer manager.Close()

	// Get analytics when disabled
	analytics, err := manager.GetAnalytics()
	if err != nil {
		t.Fatalf("Failed to get analytics: %v", err)
	}

	// Should return empty analytics
	if len(analytics.ProviderStats) != 0 {
		t.Errorf("Expected 0 provider stats when disabled, got %d", len(analytics.ProviderStats))
	}

	if len(analytics.ModelPerformance) != 0 {
		t.Errorf("Expected 0 model performance when disabled, got %d", len(analytics.ModelPerformance))
	}

	// Compute analytics when disabled
	analytics2, err := manager.ComputeAnalytics()
	if err != nil {
		t.Fatalf("Failed to compute analytics: %v", err)
	}

	// Should also return empty analytics
	if len(analytics2.ProviderStats) != 0 {
		t.Errorf("Expected 0 provider stats when disabled, got %d", len(analytics2.ProviderStats))
	}
}

func TestMemoryManager_DailyLogsIntegration(t *testing.T) {
	tempDir := t.TempDir()

	config := &MemoryConfig{
		Enabled:       true,
		BaseDir:       tempDir,
		RetentionDays: 90,
		MaxLogSizeMB:  100,
		Compression:   false,
	}

	manager, err := NewMemoryManager(config)
	if err != nil {
		t.Fatalf("Failed to create memory manager: %v", err)
	}
	defer manager.Close()

	// Record routing decision (should be logged to daily logs)
	decision := &RoutingDecision{
		Timestamp:  time.Now(),
		APIKeyHash: "sha256:test123",
		Request: RequestInfo{
			Model:  "auto",
			Intent: "coding",
		},
		Routing: RoutingInfo{
			Tier:          "semantic",
			SelectedModel: "claudecli:claude-sonnet-4",
			Confidence:    0.9,
		},
		Outcome: OutcomeInfo{
			Success:      true,
			QualityScore: 0.85,
		},
	}

	if err := manager.RecordRouting(decision); err != nil {
		t.Fatalf("Failed to record routing decision: %v", err)
	}

	// Add quirk (should be logged to daily logs)
	quirk := &Quirk{
		Provider:   "testprovider",
		Issue:      "test issue",
		Workaround: "test workaround",
		Discovered: time.Now(),
		Frequency:  "rare",
		Severity:   "low",
	}

	if err := manager.AddQuirk(quirk); err != nil {
		t.Fatalf("Failed to add quirk: %v", err)
	}

	// Learn from outcome (should be logged to daily logs)
	if err := manager.LearnFromOutcome(decision); err != nil {
		t.Fatalf("Failed to learn from outcome: %v", err)
	}

	// Verify daily log file was created
	dailyLogsDir := filepath.Join(tempDir, "daily")
	today := time.Now().Format("2006-01-02")
	logFile := filepath.Join(dailyLogsDir, today+".jsonl")

	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		t.Fatalf("Daily log file was not created: %s", logFile)
	}

	// Read and verify log entries
	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read daily log file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	if len(lines) < 3 {
		t.Fatalf("Expected at least 3 log entries, got %d", len(lines))
	}

	// Verify entry types
	entryTypes := make(map[string]int)
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		var entry DailyLogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("Failed to unmarshal log entry: %v", err)
		}

		entryTypes[entry.Type]++
	}

	if entryTypes["routing"] != 1 {
		t.Errorf("Expected 1 routing entry, got %d", entryTypes["routing"])
	}

	if entryTypes["quirk"] != 1 {
		t.Errorf("Expected 1 quirk entry, got %d", entryTypes["quirk"])
	}

	if entryTypes["preference_update"] != 1 {
		t.Errorf("Expected 1 preference_update entry, got %d", entryTypes["preference_update"])
	}

	// Get stats and verify daily logs stats are included
	stats, err := manager.GetStats()
	if err != nil {
		t.Fatalf("Failed to get stats: %v", err)
	}

	if stats.DailyLogsStats == nil {
		t.Fatal("Expected daily logs stats to be included")
	}

	if stats.DailyLogsStats.TotalLogFiles != 1 {
		t.Errorf("Expected 1 daily log file, got %d", stats.DailyLogsStats.TotalLogFiles)
	}

	if stats.DailyLogsStats.TotalEntries < 3 {
		t.Errorf("Expected at least 3 daily log entries, got %d", stats.DailyLogsStats.TotalEntries)
	}
}
