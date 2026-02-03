package memory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// MemoryManager provides a unified interface for all memory operations.
// It coordinates routing history, provider quirks, and user preferences stores.
type MemoryManager interface {
	// RecordRouting stores a routing decision with its outcome
	RecordRouting(decision *RoutingDecision) error

	// GetUserPreferences retrieves learned preferences for an API key
	GetUserPreferences(apiKeyHash string) (*UserPreferences, error)

	// UpdateUserPreferences updates learned preferences explicitly
	UpdateUserPreferences(prefs *UserPreferences) error

	// DeleteUserPreferences deletes preferences for a user
	DeleteUserPreferences(apiKeyHash string) error

	// AddQuirk records a provider quirk or workaround
	AddQuirk(quirk *Quirk) error

	// GetProviderQuirks retrieves known quirks for a provider
	GetProviderQuirks(provider string) ([]*Quirk, error)

	// GetHistory retrieves routing history for an API key
	GetHistory(apiKeyHash string, limit int) ([]*RoutingDecision, error)

	// GetAllHistory retrieves all routing history
	GetAllHistory(limit int) ([]*RoutingDecision, error)

	// LearnFromOutcome updates preferences based on request outcome
	LearnFromOutcome(decision *RoutingDecision) error

	// GetStats returns aggregated statistics
	GetStats() (*MemoryStats, error)

	// GetAnalytics returns computed analytics summary
	GetAnalytics() (*AnalyticsSummary, error)

	// ComputeAnalytics computes fresh analytics from routing history
	ComputeAnalytics() (*AnalyticsSummary, error)

	// Cleanup performs maintenance tasks (log rotation, old file cleanup)
	Cleanup() error

	// Close gracefully shuts down the memory manager
	Close() error
}

// MemoryManagerImpl implements the MemoryManager interface.
// It coordinates all memory stores and provides unified access.
type MemoryManagerImpl struct {
	config    *MemoryConfig
	structure *DirectoryStructure

	// Individual stores
	routingHistory *RoutingHistoryStore
	quirks         *QuirksStore
	preferences    *PreferencesStore

	// Daily logs and analytics
	dailyLogs *DailyLogsManager
	analytics *AnalyticsEngine

	// Synchronization
	mu     sync.RWMutex
	ctx    context.Context
	cancel context.CancelFunc

	// Cleanup management
	cleanupTicker *time.Ticker
	cleanupDone   chan struct{}
}

// MemoryStats provides statistics about the memory system.
type MemoryStats struct {
	TotalDecisions     int       `json:"total_decisions"`
	TotalUsers         int       `json:"total_users"`
	TotalQuirks        int       `json:"total_quirks"`
	DiskUsageBytes     int64     `json:"disk_usage_bytes"`
	OldestDecision     time.Time `json:"oldest_decision,omitempty"`
	NewestDecision     time.Time `json:"newest_decision,omitempty"`
	LastCleanup        time.Time `json:"last_cleanup,omitempty"`
	RetentionDays      int       `json:"retention_days"`
	CompressionEnabled bool      `json:"compression_enabled"`

	// Daily logs statistics
	DailyLogsStats *DailyLogsStats `json:"daily_logs_stats,omitempty"`

	// Analytics information
	LastAnalyticsUpdate time.Time `json:"last_analytics_update,omitempty"`
}

// NewMemoryManager creates a new memory manager with the given configuration.
// It initializes all stores and starts background maintenance tasks.
func NewMemoryManager(config *MemoryConfig) (MemoryManager, error) {
	if config == nil {
		config = DefaultMemoryConfig()
	}

	// Validate configuration
	if err := validateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	manager := &MemoryManagerImpl{
		config:      config,
		ctx:         ctx,
		cancel:      cancel,
		cleanupDone: make(chan struct{}),
	}

	// Initialize directory structure
	manager.structure = NewDirectoryStructure(config.BaseDir)
	if err := manager.structure.Initialize(); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to initialize directory structure: %w", err)
	}

	// Initialize individual stores
	if err := manager.initializeStores(); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to initialize stores: %w", err)
	}

	// Start background cleanup if enabled
	if config.Enabled {
		manager.startCleanupRoutine()
	}

	return manager, nil
}

// initializeStores creates and initializes all individual memory stores.
func (mm *MemoryManagerImpl) initializeStores() error {
	var err error

	// Initialize routing history store
	routingHistoryPath := mm.structure.GetRoutingHistoryPath()
	mm.routingHistory, err = NewRoutingHistoryStore(routingHistoryPath)
	if err != nil {
		return fmt.Errorf("failed to initialize routing history store: %w", err)
	}

	// Initialize quirks store
	quirksPath := mm.structure.GetProviderQuirksPath()
	mm.quirks, err = NewQuirksStore(quirksPath)
	if err != nil {
		return fmt.Errorf("failed to initialize quirks store: %w", err)
	}

	// Initialize preferences store
	preferencesDir := mm.structure.GetUserPreferencesDir()
	mm.preferences, err = NewPreferencesStore(preferencesDir)
	if err != nil {
		return fmt.Errorf("failed to initialize preferences store: %w", err)
	}

	// Initialize daily logs manager
	dailyLogsDir := mm.structure.GetDailyLogsDir()
	mm.dailyLogs, err = NewDailyLogsManager(dailyLogsDir, mm.config.RetentionDays, mm.config.Compression)
	if err != nil {
		return fmt.Errorf("failed to initialize daily logs manager: %w", err)
	}

	// Initialize analytics engine
	analyticsDir := mm.structure.GetAnalyticsDir()
	mm.analytics = NewAnalyticsEngine(analyticsDir)

	return nil
}

// RecordRouting stores a routing decision with its outcome.
// This is thread-safe and non-blocking.
func (mm *MemoryManagerImpl) RecordRouting(decision *RoutingDecision) error {
	if !mm.config.Enabled {
		return nil // Memory system disabled
	}

	mm.mu.RLock()
	defer mm.mu.RUnlock()

	if mm.routingHistory == nil {
		return fmt.Errorf("routing history store not initialized")
	}

	// Record to main routing history
	if err := mm.routingHistory.RecordRouting(decision); err != nil {
		return fmt.Errorf("failed to record to routing history: %w", err)
	}

	// Record to daily logs
	if mm.dailyLogs != nil {
		if err := mm.dailyLogs.WriteEntry("routing", decision); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write to daily logs: %v\n", err)
		}
	}

	return nil
}

// GetUserPreferences retrieves learned preferences for an API key.
func (mm *MemoryManagerImpl) GetUserPreferences(apiKeyHash string) (*UserPreferences, error) {
	if !mm.config.Enabled {
		// Return default preferences when memory is disabled
		return &UserPreferences{
			APIKeyHash:       apiKeyHash,
			LastUpdated:      time.Now(),
			ModelPreferences: make(map[string]string),
			ProviderBias:     make(map[string]float64),
			CustomRules:      []PreferenceRule{},
		}, nil
	}

	mm.mu.RLock()
	defer mm.mu.RUnlock()

	if mm.preferences == nil {
		return nil, fmt.Errorf("preferences store not initialized")
	}

	return mm.preferences.GetUserPreferences(apiKeyHash)
}

// UpdateUserPreferences updates learned preferences explicitly.
func (mm *MemoryManagerImpl) UpdateUserPreferences(prefs *UserPreferences) error {
	if !mm.config.Enabled {
		return nil
	}

	mm.mu.Lock()
	defer mm.mu.Unlock()

	if mm.preferences == nil {
		return fmt.Errorf("preferences store not initialized")
	}

	return mm.preferences.UpdatePreferences(prefs.APIKeyHash, prefs)
}

// DeleteUserPreferences deletes preferences for a user.
func (mm *MemoryManagerImpl) DeleteUserPreferences(apiKeyHash string) error {
	if !mm.config.Enabled {
		return nil
	}

	mm.mu.Lock()
	defer mm.mu.Unlock()

	if mm.preferences == nil {
		return fmt.Errorf("preferences store not initialized")
	}

	return mm.preferences.DeleteUserPreferences(apiKeyHash)
}

// AddQuirk records a provider quirk or workaround.
func (mm *MemoryManagerImpl) AddQuirk(quirk *Quirk) error {
	if !mm.config.Enabled {
		return nil // Memory system disabled
	}

	mm.mu.RLock()
	defer mm.mu.RUnlock()

	if mm.quirks == nil {
		return fmt.Errorf("quirks store not initialized")
	}

	// Record to quirks store
	if err := mm.quirks.AddQuirk(quirk); err != nil {
		return fmt.Errorf("failed to add quirk: %w", err)
	}

	// Record to daily logs
	if mm.dailyLogs != nil {
		if err := mm.dailyLogs.WriteEntry("quirk", quirk); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write to daily logs: %v\n", err)
		}
	}

	return nil
}

// GetProviderQuirks retrieves known quirks for a provider.
func (mm *MemoryManagerImpl) GetProviderQuirks(provider string) ([]*Quirk, error) {
	if !mm.config.Enabled {
		return []*Quirk{}, nil // Return empty slice when disabled
	}

	mm.mu.RLock()
	defer mm.mu.RUnlock()

	if mm.quirks == nil {
		return nil, fmt.Errorf("quirks store not initialized")
	}

	return mm.quirks.GetProviderQuirks(provider)
}

// GetHistory retrieves routing history for an API key.
func (mm *MemoryManagerImpl) GetHistory(apiKeyHash string, limit int) ([]*RoutingDecision, error) {
	if !mm.config.Enabled {
		return []*RoutingDecision{}, nil // Return empty slice when disabled
	}

	mm.mu.RLock()
	defer mm.mu.RUnlock()

	if mm.routingHistory == nil {
		return nil, fmt.Errorf("routing history store not initialized")
	}

	return mm.routingHistory.GetHistory(apiKeyHash, limit)
}

// GetAllHistory retrieves all routing history.
func (mm *MemoryManagerImpl) GetAllHistory(limit int) ([]*RoutingDecision, error) {
	if !mm.config.Enabled {
		return []*RoutingDecision{}, nil // Return empty slice when disabled
	}

	mm.mu.RLock()
	defer mm.mu.RUnlock()

	if mm.routingHistory == nil {
		return nil, fmt.Errorf("routing history store not initialized")
	}

	return mm.routingHistory.GetAllHistory(limit)
}

// LearnFromOutcome updates preferences based on request outcome.
func (mm *MemoryManagerImpl) LearnFromOutcome(decision *RoutingDecision) error {
	if !mm.config.Enabled {
		return nil // Memory system disabled
	}

	mm.mu.RLock()
	defer mm.mu.RUnlock()

	if mm.preferences == nil {
		return fmt.Errorf("preferences store not initialized")
	}

	// Update preferences
	if err := mm.preferences.LearnFromOutcome(decision); err != nil {
		return fmt.Errorf("failed to learn from outcome: %w", err)
	}

	// Record to daily logs
	if mm.dailyLogs != nil {
		if err := mm.dailyLogs.WriteEntry("preference_update", decision); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write to daily logs: %v\n", err)
		}
	}

	return nil
}

// GetStats returns aggregated statistics about the memory system.
func (mm *MemoryManagerImpl) GetStats() (*MemoryStats, error) {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	stats := &MemoryStats{
		RetentionDays:      mm.config.RetentionDays,
		CompressionEnabled: mm.config.Compression,
	}

	if !mm.config.Enabled {
		return stats, nil
	}

	// Get routing history stats
	if mm.routingHistory != nil {
		if count, err := mm.routingHistory.Count(); err == nil {
			stats.TotalDecisions = count
		}

		// Get oldest and newest decisions
		if history, err := mm.routingHistory.GetAllHistory(1); err == nil && len(history) > 0 {
			stats.NewestDecision = history[0].Timestamp
		}

		// For oldest, we'd need to read from the end of the file
		// For now, we'll skip this to avoid performance impact
	}

	// Get preferences stats
	if mm.preferences != nil {
		if count, err := mm.preferences.Count(); err == nil {
			stats.TotalUsers = count
		}
	}

	// Get quirks stats
	if mm.quirks != nil {
		stats.TotalQuirks = mm.quirks.Count()
	}

	// Get daily logs stats
	if mm.dailyLogs != nil {
		if dailyStats, err := mm.dailyLogs.GetStats(); err == nil {
			stats.DailyLogsStats = dailyStats
		}
	}

	// Get analytics update time
	if mm.analytics != nil {
		stats.LastAnalyticsUpdate = mm.analytics.GetLastUpdate()
	}

	// Calculate disk usage
	if diskUsage, err := mm.calculateDiskUsage(); err == nil {
		stats.DiskUsageBytes = diskUsage
	}

	return stats, nil
}

// GetAnalytics returns the most recent analytics summary.
func (mm *MemoryManagerImpl) GetAnalytics() (*AnalyticsSummary, error) {
	if !mm.config.Enabled {
		return &AnalyticsSummary{
			GeneratedAt:       time.Now(),
			ProviderStats:     make(map[string]*ProviderStats),
			ModelPerformance:  make(map[string]*ModelPerformance),
			TierEffectiveness: &TierEffectiveness{},
			CostAnalysis:      &CostAnalysis{},
			TrendAnalysis:     &TrendAnalysis{},
		}, nil
	}

	mm.mu.RLock()
	defer mm.mu.RUnlock()

	if mm.analytics == nil {
		return nil, fmt.Errorf("analytics engine not initialized")
	}

	return mm.analytics.LoadAnalytics()
}

// ComputeAnalytics computes fresh analytics from all routing history.
func (mm *MemoryManagerImpl) ComputeAnalytics() (*AnalyticsSummary, error) {
	if !mm.config.Enabled {
		return mm.GetAnalytics() // Return empty analytics when disabled
	}

	mm.mu.RLock()
	defer mm.mu.RUnlock()

	if mm.analytics == nil {
		return nil, fmt.Errorf("analytics engine not initialized")
	}

	if mm.routingHistory == nil {
		return nil, fmt.Errorf("routing history store not initialized")
	}

	// Get all routing history for analytics
	decisions, err := mm.routingHistory.GetAllHistory(-1) // Get all decisions
	if err != nil {
		return nil, fmt.Errorf("failed to get routing history: %w", err)
	}

	// Compute analytics
	return mm.analytics.ComputeAnalytics(decisions)
}

// calculateDiskUsage calculates the total disk usage of the memory system.
func (mm *MemoryManagerImpl) calculateDiskUsage() (int64, error) {
	var totalSize int64

	err := filepath.Walk(mm.config.BaseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files we can't access
		}
		if !info.IsDir() {
			totalSize += info.Size()
		}
		return nil
	})

	return totalSize, err
}

// Cleanup performs maintenance tasks like log rotation and old file cleanup.
func (mm *MemoryManagerImpl) Cleanup() error {
	if !mm.config.Enabled {
		return nil
	}

	mm.mu.Lock()
	defer mm.mu.Unlock()

	// Clean up old daily logs
	if mm.dailyLogs != nil {
		if err := mm.dailyLogs.CleanupOldLogs(); err != nil {
			return fmt.Errorf("failed to cleanup daily logs: %w", err)
		}
	}

	// Clean up old routing history logs (existing functionality)
	if err := mm.cleanupOldLogs(); err != nil {
		return fmt.Errorf("failed to cleanup old logs: %w", err)
	}

	// Compress old files if enabled
	if mm.config.Compression {
		if err := mm.compressOldFiles(); err != nil {
			return fmt.Errorf("failed to compress old files: %w", err)
		}
	}

	// Recompute analytics after cleanup
	if mm.analytics != nil && mm.routingHistory != nil {
		if decisions, err := mm.routingHistory.GetAllHistory(-1); err == nil {
			// Compute analytics in background (don't fail cleanup on analytics error)
			go func() {
				if _, err := mm.analytics.ComputeAnalytics(decisions); err != nil {
					fmt.Fprintf(os.Stderr, "failed to compute analytics in background: %v\n", err)
				}
			}()
		}
	}

	return nil
}

// cleanupOldLogs removes log files older than the retention period.
func (mm *MemoryManagerImpl) cleanupOldLogs() error {
	dailyLogsDir := mm.structure.GetDailyLogsDir()

	cutoffTime := time.Now().AddDate(0, 0, -mm.config.RetentionDays)

	entries, err := os.ReadDir(dailyLogsDir)
	if err != nil {
		return fmt.Errorf("failed to read daily logs directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoffTime) {
			filePath := filepath.Join(dailyLogsDir, entry.Name())
			if err := os.Remove(filePath); err != nil {
				// Log error but continue with other files
				continue
			}
		}
	}

	return nil
}

// compressOldFiles compresses files older than 7 days.
func (mm *MemoryManagerImpl) compressOldFiles() error {
	// For now, we'll implement a simple placeholder
	// In a full implementation, this would use gzip compression
	// on files older than 7 days
	return nil
}

// startCleanupRoutine starts the background cleanup routine.
func (mm *MemoryManagerImpl) startCleanupRoutine() {
	// Run cleanup every 24 hours
	mm.cleanupTicker = time.NewTicker(24 * time.Hour)

	go func() {
		// Ensure cleanupDone is always closed, even if panic occurs
		defer func() {
			if r := recover(); r != nil {
				fmt.Fprintf(os.Stderr, "recovered from panic in cleanup routine: %v\n", r)
			}
			close(mm.cleanupDone)
		}()

		for {
			select {
			case <-mm.ctx.Done():
				return
			case <-mm.cleanupTicker.C:
				if err := mm.Cleanup(); err != nil {
					fmt.Fprintf(os.Stderr, "background cleanup failed: %v\n", err)
				}
			}
		}
	}()
}

// Close gracefully shuts down the memory manager.
func (mm *MemoryManagerImpl) Close() error {
	// Cancel context BEFORE acquiring lock to avoid deadlock
	// The cleanup routine needs to acquire the lock to finish
	if mm.cancel != nil {
		mm.cancel()
	}

	// Stop cleanup routine and wait for it to finish (without holding lock)
	if mm.cleanupTicker != nil {
		mm.cleanupTicker.Stop()
		<-mm.cleanupDone
	}

	// Now safe to acquire lock for cleanup
	mm.mu.Lock()
	defer mm.mu.Unlock()

	var errors []error

	// Close individual stores
	if mm.routingHistory != nil {
		if err := mm.routingHistory.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close routing history store: %w", err))
		}
	}

	if mm.preferences != nil {
		if err := mm.preferences.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close preferences store: %w", err))
		}
	}

	if mm.quirks != nil {
		if err := mm.quirks.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close quirks store: %w", err))
		}
	}

	// Close daily logs manager
	if mm.dailyLogs != nil {
		if err := mm.dailyLogs.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close daily logs manager: %w", err))
		}
	}

	// Return combined errors if any
	if len(errors) > 0 {
		return fmt.Errorf("errors during shutdown: %v", errors)
	}

	return nil
}

// validateConfig validates the memory configuration.
func validateConfig(config *MemoryConfig) error {
	if config.BaseDir == "" {
		return fmt.Errorf("base directory cannot be empty")
	}

	if config.RetentionDays < 1 {
		return fmt.Errorf("retention days must be at least 1")
	}

	if config.MaxLogSizeMB < 1 {
		return fmt.Errorf("max log size must be at least 1 MB")
	}

	return nil
}
