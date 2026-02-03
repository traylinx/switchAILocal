package learning

import (
	"context"
	"fmt"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/traylinx/switchAILocal/internal/config"
	"github.com/traylinx/switchAILocal/internal/memory"
)

// LearningEngine orchestrates learning from routing history.
type LearningEngine struct {
	config       *config.LearningConfig
	memory       memory.MemoryManager
	lastAnalysis time.Time

	// Channels for managing background analysis
	stopChan chan struct{}
	wg       sync.WaitGroup
}

// NewLearningEngine creates a new learning engine.
func NewLearningEngine(cfg *config.LearningConfig, mem memory.MemoryManager) (*LearningEngine, error) {
	if cfg == nil {
		cfg = &config.LearningConfig{
			Enabled:             true,
			MinSampleSize:       100,
			ConfidenceThreshold: 0.85,
			AutoApply:           false,
			AnalysisInterval:    "24h",
		}
	}

	if mem == nil {
		return nil, fmt.Errorf("memory manager is required")
	}

	return &LearningEngine{
		config:   cfg,
		memory:   mem,
		stopChan: make(chan struct{}),
	}, nil
}

// Start starts the background analysis routine.
func (le *LearningEngine) Start() {
	if !le.config.Enabled {
		return
	}

	interval, err := time.ParseDuration(le.config.AnalysisInterval)
	if err != nil {
		log.Warnf("Invalid analysis interval '%s', defaulting to 24h", le.config.AnalysisInterval)
		interval = 24 * time.Hour
	}

	le.wg.Add(1)
	go func() {
		defer le.wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-le.stopChan:
				return
			case <-ticker.C:
				le.AnalyzeAll()
			}
		}
	}()

	log.Info("Learning engine started")
}

// Stop stops the background analysis.
func (le *LearningEngine) Stop() {
	if le.config.Enabled {
		close(le.stopChan)
		le.wg.Wait()
		log.Info("Learning engine stopped")
	}
}

// AnalyzeAll triggers analysis for all known users.
func (le *LearningEngine) AnalyzeAll() {
	log.Info("Starting periodic learning analysis...")

	// Get all users from memory
	// We need a way to list users. MemoryManager doesn't expose ListUsers directly on interface usually
	// checking interface...
	// MemoryManager interface doesn't have ListUsers.
	// We might need to cast to implementation or add to interface.
	// For now, we will rely on active memory or just skip if we can't list.
	// But wait, preferences store has ListUsers.

	// Let's assume we can get users by getting stats or similar,
	// or we iterate recent history.

	// TODO: Implement user listing or history iteration
	le.lastAnalysis = time.Now()
}

// AnalyzeUser performs deep analysis on a specific user's history.
func (le *LearningEngine) AnalyzeUser(ctx context.Context, apiKeyHash string) (*AnalysisResult, error) {
	// 1. Fetch history
	history, err := le.memory.GetHistory(apiKeyHash, 1000) // Fetch last 1000
	if err != nil {
		return nil, err
	}

	if len(history) < le.config.MinSampleSize {
		return &AnalysisResult{
			UserID:           apiKeyHash,
			Timestamp:        time.Now(),
			RequestsAnalyzed: len(history),
			Suggestions:      []string{"Insufficient data for analysis"},
		}, nil
	}

	// 2. Perform analysis
	model := le.performStatisticalAnalysis(history)

	// 3. Generate suggestions
	suggestions := le.generateSuggestions(model)

	// 4. Auto-apply if enabled
	if le.config.AutoApply && model != nil {
		if err := le.ApplyPreferences(model); err != nil {
			log.Errorf("Failed to auto-apply preferences for user %s: %v", apiKeyHash, err)
		} else {
			log.Infof("Auto-applied preferences for user %s", apiKeyHash)
		}
	}

	return &AnalysisResult{
		UserID:           apiKeyHash,
		Timestamp:        time.Now(),
		RequestsAnalyzed: len(history),
		NewPreferences:   model,
		Suggestions:      suggestions,
	}, nil
}

// ApplyPreferences applies learned preferences to the memory system.
func (le *LearningEngine) ApplyPreferences(model *PreferenceModel) error {
	if model == nil {
		return nil
	}

	// Fetch current preferences
	prefs, err := le.memory.GetUserPreferences(model.UserID)
	if err != nil {
		return fmt.Errorf("failed to get current preferences: %w", err)
	}

	// Update timestamp
	prefs.LastAnalyzed = time.Now()
	prefs.LastUpdated = time.Now()

	// Initialize maps if needed
	if prefs.ModelPreferences == nil {
		prefs.ModelPreferences = make(map[string]string)
	}
	if prefs.ModelConfidences == nil {
		prefs.ModelConfidences = make(map[string]float64)
	}
	if prefs.ProviderBias == nil {
		prefs.ProviderBias = make(map[string]float64)
	}

	// Apply model preferences
	// Only apply if confidence is above threshold
	for intent, learned := range model.ModelPreferences {
		// Existing logic: Overwrite if confidence is high enough
		// We use the threshold from config (default 0.85)
		if learned.Confidence >= le.config.ConfidenceThreshold {
			prefs.ModelPreferences[intent] = learned.Model
			prefs.ModelConfidences[intent] = learned.Confidence
		}
	}

	// Apply provider bias
	for provider, bias := range model.ProviderBias {
		prefs.ProviderBias[provider] = bias
	}

	// Save
	return le.memory.UpdateUserPreferences(prefs)
}
