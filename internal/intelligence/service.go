// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package intelligence provides the main orchestrator for Phase 2 intelligent routing features.
// It manages the lifecycle of all intelligence services including discovery, capability analysis,
// dynamic matrix building, skill registry, embedding engine, semantic tier, and more.
package intelligence

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	log "github.com/sirupsen/logrus"
	"github.com/traylinx/switchAILocal/internal/config"
	"github.com/traylinx/switchAILocal/internal/intelligence/cache"
	"github.com/traylinx/switchAILocal/internal/intelligence/cascade"
	"github.com/traylinx/switchAILocal/internal/intelligence/confidence"
	"github.com/traylinx/switchAILocal/internal/intelligence/discovery"
	"github.com/traylinx/switchAILocal/internal/intelligence/embedding"
	"github.com/traylinx/switchAILocal/internal/intelligence/feedback"
	"github.com/traylinx/switchAILocal/internal/intelligence/matrix"
	"github.com/traylinx/switchAILocal/internal/intelligence/semantic"
	"github.com/traylinx/switchAILocal/internal/intelligence/skills"
	"github.com/traylinx/switchAILocal/internal/intelligence/verification"
	"github.com/traylinx/switchAILocal/internal/util"
)

// Service is the main orchestrator for all intelligence services.
// It manages the lifecycle of Phase 2 features and provides graceful degradation
// when services are disabled or unavailable.
type Service struct {
	// config holds the intelligence configuration
	config *config.IntelligenceConfig

	// enabled indicates whether intelligence services are active
	enabled bool

	// mu protects concurrent access to service state
	mu sync.RWMutex

	// Services
	discovery *discovery.Service
	matrix    *matrix.Builder
	skills    *skills.Registry
	embedding *embedding.Engine
	semantic  *semantic.Tier
	cache     *cache.SemanticCache

	// Services (to be added in future phases)
	// capability *capability.CapabilityAnalyzer
	confidence *confidence.Scorer
	verifier   *verification.Verifier
	cascade    *cascade.Manager
	feedback   *feedback.Collector
}

// NewService creates a new IntelligenceService instance.
// The service is not initialized until Initialize() is called.
//
// Parameters:
//   - cfg: The intelligence configuration
//
// Returns:
//   - *Service: A new service instance
func NewService(cfg *config.IntelligenceConfig) *Service {
	if cfg == nil {
		cfg = &config.IntelligenceConfig{}
	}

	return &Service{
		config:  cfg,
		enabled: cfg.Enabled,
	}
}

// Initialize starts all enabled intelligence services.
// Services are started based on their individual feature flags.
// If the master switch (intelligence.enabled) is false, this is a no-op.
//
// Parameters:
//   - ctx: Context for initialization operations
//
// Returns:
//   - error: Any error encountered during initialization
func (s *Service) Initialize(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// If intelligence is disabled, nothing to initialize
	if !s.enabled {
		log.Info("Intelligence services disabled (intelligence.enabled: false)")
		return nil
	}

	log.Info("Initializing intelligence services...")

	// Phase 2 (µP2): Discovery Service
	if s.config.Discovery.Enabled {
		log.Info("Initializing discovery service...")
		expandedCacheDir, err := util.ExpandPath(s.config.Discovery.CacheDir)
		if err != nil {
			log.Warnf("Failed to expand discovery cache dir: %v", err)
			expandedCacheDir = s.config.Discovery.CacheDir
		}
		discoverySvc, err := discovery.NewService(expandedCacheDir)
		if err != nil {
			log.Warnf("Failed to create discovery service: %v", err)
		} else {
			s.discovery = discoverySvc
			// Run initial discovery
			if err := s.discovery.DiscoverAll(ctx); err != nil {
				log.Warnf("Initial discovery failed: %v", err)
			} else {
				// Write registry to disk
				if err := s.discovery.WriteRegistry(""); err != nil {
					log.Warnf("Failed to write discovery registry: %v", err)
				}
			}
		}
	}

	// Phase 4 (µP4): Dynamic Matrix Builder
	if s.config.AutoAssign.Enabled {
		log.Info("Initializing dynamic matrix builder...")
		s.matrix = matrix.NewBuilder(
			s.config.AutoAssign.PreferLocal,
			s.config.AutoAssign.CostOptimization,
			s.config.AutoAssign.Overrides,
		)

		// Build initial matrix if discovery is available
		if s.discovery != nil {
			models := s.discovery.GetAvailableModels()
			if len(models) > 0 {
				// Convert discovery models to matrix models
				matrixModels := make([]*matrix.ModelWithCapability, len(models))
				for i, m := range models {
					matrixModels[i] = &matrix.ModelWithCapability{
						ID:           m.ID,
						Provider:     m.Provider,
						DisplayName:  m.DisplayName,
						Capabilities: m.Capabilities,
					}
				}
				s.matrix.Build(matrixModels)
				log.Infof("Built initial dynamic matrix with %d models", len(models))
			} else {
				log.Warn("No models discovered, dynamic matrix will be empty")
			}
		} else {
			log.Warn("Discovery service not available, dynamic matrix will be empty")
		}
	}

	// Phase 5 (µP5): Skill Registry
	if s.config.Skills.Enabled {
		log.Info("Initializing skill registry...")
		s.skills = skills.NewRegistry(s.config.SkillMatching.ConfidenceThreshold)

		// Embedding engine will be set after initialization (Phase 6)

		// Load all skills from directory
		if s.config.Skills.Directory != "" {
			if err := s.skills.LoadAll(s.config.Skills.Directory); err != nil {
				log.Warnf("Failed to load skills: %v", err)
			} else {
				log.Infof("Loaded %d skills", s.skills.GetSkillCount())
			}
		} else {
			log.Warn("Skills directory not configured, skill registry will be empty")
		}
	}

	// Phase 6 (µP6): Embedding Engine
	if s.config.Embedding.Enabled {
		log.Info("Initializing embedding engine...")
		locator := embedding.NewModelLocator()
		modelName := s.config.Embedding.Model
		if modelName == "" {
			modelName = embedding.DefaultModelName
		}

		// Check if model exists
		if !locator.ModelExists(modelName) {
			log.Warnf("Embedding model not found: %s. Run 'scripts/download-embedding-model.sh' to download.", modelName)
		} else {
			cfg := embedding.Config{
				ModelPath:         locator.GetModelPath(modelName),
				VocabPath:         locator.GetVocabPath(modelName),
				SharedLibraryPath: locator.GetSharedLibraryPath(),
			}

			engine, err := embedding.NewEngine(cfg)
			if err != nil {
				log.Warnf("Failed to create embedding engine: %v", err)
			} else {
				if err := engine.Initialize(cfg.SharedLibraryPath); err != nil {
					log.Warnf("Failed to initialize embedding engine: %v", err)
				} else {
					s.embedding = engine
					log.Infof("Embedding engine initialized with model: %s", modelName)

					// Update skill registry with embedding engine
					if s.skills != nil {
						s.skills.SetEmbeddingEngine(engine)
						// Reload skills to compute embeddings
						if s.config.Skills.Directory != "" {
							if err := s.skills.LoadAll(s.config.Skills.Directory); err != nil {
								log.Warnf("Failed to reload skills with embeddings: %v", err)
							}
						}
					}
				}
			}
		}
	}

	// Phase 7 (µP7): Semantic Tier
	if s.config.SemanticTier.Enabled {
		log.Info("Initializing semantic tier...")
		if s.embedding != nil && s.embedding.IsEnabled() {
			s.semantic = semantic.NewTier(s.embedding, s.config.SemanticTier.ConfidenceThreshold)

			// Find intents.yaml path (relative to skills directory or use default)
			intentsPath := filepath.Join(filepath.Dir(s.config.Skills.Directory), "internal/intelligence/semantic/intents.yaml")
			// Try alternative paths
			if _, err := filepath.Abs(intentsPath); err != nil {
				intentsPath = "internal/intelligence/semantic/intents.yaml"
			}

			if err := s.semantic.Initialize(intentsPath); err != nil {
				log.Warnf("Failed to initialize semantic tier: %v", err)
				s.semantic = nil
			} else {
				log.Infof("Semantic tier initialized with %d intents", s.semantic.GetIntentCount())
			}
		} else {
			log.Warn("Embedding engine not available, semantic tier will be disabled")
		}
	}

	// Phase 9 (µP9): Semantic Cache
	if s.config.SemanticCache.Enabled {
		log.Info("Initializing semantic cache...")
		if s.embedding != nil && s.embedding.IsEnabled() {
			s.cache = cache.NewSemanticCache(
				s.embedding,
				s.config.SemanticCache.SimilarityThreshold,
				s.config.SemanticCache.MaxSize,
			)
			log.Infof("Semantic cache initialized (threshold: %.2f, max size: %d)",
				s.config.SemanticCache.SimilarityThreshold,
				s.config.SemanticCache.MaxSize)
		} else {
			log.Warn("Embedding engine not available, semantic cache will be disabled")
		}
	}

	// Phase 10 (µP10): Confidence Scorer
	if s.config.Confidence.Enabled {
		log.Info("Initializing confidence scorer...")
		s.confidence = confidence.NewScorer()
	}

	// Phase 11 (µP11): Verifier
	if s.config.Verification.Enabled {
		log.Info("Initializing verifier...")
		s.verifier = verification.NewVerifier()
	}

	// Phase 12 (µP12): Cascade Manager
	if s.config.Cascade.Enabled {
		log.Info("Initializing cascade manager...")
		s.cascade = cascade.NewManager(cascade.Config{
			Enabled:          true,
			QualityThreshold: s.config.Cascade.QualityThreshold,
			MaxCascades:      2, // fast -> standard -> reasoning
		})
		log.Infof("Cascade manager initialized (threshold: %.2f)", s.config.Cascade.QualityThreshold)
	}

	// Phase 13 (µP13): Feedback Collector
	if s.config.Feedback.Enabled {
		log.Info("Initializing feedback collector...")
		expandedCacheDir, err := util.ExpandPath(s.config.Discovery.CacheDir)
		if err != nil {
			log.Warnf("Failed to expand discovery cache dir for feedback: %v", err)
			expandedCacheDir = s.config.Discovery.CacheDir
		}
		dbPath := filepath.Join(expandedCacheDir, "feedback.db")
		collector, err := feedback.NewCollector(dbPath, s.config.Feedback.RetentionDays)
		if err != nil {
			log.Warnf("Failed to create feedback collector: %v", err)
		} else {
			if err := collector.Initialize(ctx); err != nil {
				log.Warnf("Failed to initialize feedback collector: %v", err)
			} else {
				s.feedback = collector
				log.Infof("Feedback collector initialized (retention: %d days)", s.config.Feedback.RetentionDays)
			}
		}
	}

	log.Info("Intelligence services initialized successfully")
	return nil
}

// IsEnabled returns whether intelligence services are active.
// This is used by Lua API functions to determine if they should
// return errors or attempt to use intelligence features.
//
// Returns:
//   - bool: true if intelligence services are enabled and initialized
func (s *Service) IsEnabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.enabled
}

// Shutdown gracefully stops all running intelligence services.
// It ensures all resources are properly cleaned up and any pending
// operations are completed or cancelled.
//
// Parameters:
//   - ctx: Context for shutdown operations (with timeout)
//
// Returns:
//   - error: Any error encountered during shutdown
func (s *Service) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.enabled {
		return nil
	}

	log.Info("Shutting down intelligence services...")

	var shutdownErrors []error

	// Shutdown services in reverse order of initialization

	// Phase 9 (µP9): Semantic Cache
	if s.cache != nil {
		// Cache doesn't need explicit shutdown, just clear it
		s.cache.Clear()
	}

	// Phase 7 (µP7): Semantic Tier
	if s.semantic != nil {
		if err := s.semantic.Shutdown(); err != nil {
			shutdownErrors = append(shutdownErrors, fmt.Errorf("semantic tier shutdown: %w", err))
		}
	}

	// Phase 6 (µP6): Embedding Engine
	if s.embedding != nil {
		if err := s.embedding.Shutdown(); err != nil {
			shutdownErrors = append(shutdownErrors, fmt.Errorf("embedding shutdown: %w", err))
		}
	}

	// TODO: Shutdown services in reverse order of initialization
	// Phase 13 (µP13): Feedback Collector
	if s.feedback != nil {
		if err := s.feedback.Shutdown(ctx); err != nil {
			shutdownErrors = append(shutdownErrors, fmt.Errorf("feedback shutdown: %w", err))
		}
	}

	// ... (other services)

	// Phase 2 (µP2): Discovery Service
	if s.discovery != nil {
		if err := s.discovery.Shutdown(ctx); err != nil {
			shutdownErrors = append(shutdownErrors, fmt.Errorf("discovery shutdown: %w", err))
		}
	}

	if len(shutdownErrors) > 0 {
		log.Warnf("Intelligence services shutdown completed with %d errors", len(shutdownErrors))
		return fmt.Errorf("shutdown errors: %v", shutdownErrors)
	}

	log.Info("Intelligence services shut down successfully")
	return nil
}

// GetConfig returns the current intelligence configuration.
// This is useful for services that need to access configuration values.
//
// Returns:
//   - *config.IntelligenceConfig: The intelligence configuration
func (s *Service) GetConfig() *config.IntelligenceConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config
}

// GetDiscoveryService returns the discovery service instance.
// Returns nil if discovery is not enabled or not initialized.
//
// Returns:
//   - DiscoveryServiceInterface: The discovery service instance, or nil
func (s *Service) GetDiscoveryService() DiscoveryServiceInterface {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.discovery == nil {
		return nil
	}
	return s.discovery
}

// GetMatrixBuilder returns the dynamic matrix builder instance.
// Returns nil if auto-assign is not enabled or not initialized.
//
// Returns:
//   - MatrixBuilderInterface: The matrix builder instance, or nil
func (s *Service) GetMatrixBuilder() MatrixBuilderInterface {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.matrix == nil {
		return nil
	}
	return s.matrix
}

// IsModelAvailable checks if a model ID is in the discovered models.
// Returns false if discovery is not enabled or the model is not found.
//
// Parameters:
//   - modelID: The model ID to check
//
// Returns:
//   - bool: true if the model is available
func (s *Service) IsModelAvailable(modelID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.discovery == nil {
		return false
	}

	models := s.discovery.GetAvailableModels()
	for _, m := range models {
		if m.ID == modelID {
			return m.IsAvailable
		}
	}
	return false
}

// GetSkillRegistry returns the skill registry instance as an interface.
// Returns nil if skills are not enabled or not initialized.
//
// Returns:
//   - *skills.Registry: The skill registry instance, or nil
func (s *Service) GetSkillRegistry() *skills.Registry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.skills
}

// GetEmbeddingEngine returns the embedding engine instance.
// Returns nil if embedding is not enabled or not initialized.
//
// Returns:
//   - *embedding.Engine: The embedding engine instance, or nil
func (s *Service) GetEmbeddingEngine() *embedding.Engine {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.embedding
}

// GetSemanticTier returns the semantic tier instance.
// Returns nil if semantic tier is not enabled or not initialized.
//
// Returns:
//   - SemanticTierInterface: The semantic tier instance, or nil
func (s *Service) GetSemanticTier() SemanticTierInterface {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.semantic == nil {
		return nil
	}
	return &semanticTierWrapper{tier: s.semantic}
}

// GetSemanticCache returns the semantic cache instance.
// Returns nil if semantic cache is not enabled or not initialized.
//
// Returns:
//   - SemanticCacheInterface: The semantic cache instance, or nil
func (s *Service) GetSemanticCache() SemanticCacheInterface {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.cache == nil {
		return nil
	}
	return s.cache
}

// GetConfidenceScorer returns the confidence scorer instance.
func (s *Service) GetConfidenceScorer() *confidence.Scorer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.confidence
}

// GetVerifier returns the verifier instance.
func (s *Service) GetVerifier() *verification.Verifier {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.verifier
}

// GetCascadeManager returns the cascade manager instance.
// Returns nil if cascade is not enabled or not initialized.
//
// Returns:
//   - CascadeManagerInterface: The cascade manager instance, or nil
func (s *Service) GetCascadeManager() CascadeManagerInterface {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.cascade == nil {
		return nil
	}
	return &cascadeManagerWrapper{manager: s.cascade}
}

// GetFeedbackCollector returns the feedback collector instance.
// Returns nil if feedback is not enabled or not initialized.
//
// Returns:
//   - FeedbackCollectorInterface: The feedback collector instance, or nil
func (s *Service) GetFeedbackCollector() FeedbackCollectorInterface {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.feedback == nil {
		return nil
	}
	return &feedbackCollectorWrapper{collector: s.feedback}
}

// feedbackCollectorWrapper wraps the feedback.Collector to implement FeedbackCollectorInterface.
type feedbackCollectorWrapper struct {
	collector *feedback.Collector
}

func (w *feedbackCollectorWrapper) IsEnabled() bool {
	return w.collector.IsEnabled()
}

func (w *feedbackCollectorWrapper) Record(ctx interface{}, record *FeedbackRecord) error {
	// Convert interface record to feedback.FeedbackRecord
	fbRecord := &feedback.FeedbackRecord{
		Query:           record.Query,
		Intent:          record.Intent,
		SelectedModel:   record.SelectedModel,
		RoutingTier:     record.RoutingTier,
		Confidence:      record.Confidence,
		MatchedSkill:    record.MatchedSkill,
		CascadeOccurred: record.CascadeOccurred,
		ResponseQuality: record.ResponseQuality,
		LatencyMs:       record.LatencyMs,
		Success:         record.Success,
		ErrorMessage:    record.ErrorMessage,
		Metadata:        record.Metadata,
	}

	// Convert context
	var ctxVal context.Context
	if c, ok := ctx.(context.Context); ok {
		ctxVal = c
	} else {
		ctxVal = context.Background()
	}

	return w.collector.Record(ctxVal, fbRecord)
}

func (w *feedbackCollectorWrapper) GetStats(ctx interface{}) (map[string]interface{}, error) {
	// Convert context
	var ctxVal context.Context
	if c, ok := ctx.(context.Context); ok {
		ctxVal = c
	} else {
		ctxVal = context.Background()
	}

	return w.collector.GetStats(ctxVal)
}

func (w *feedbackCollectorWrapper) GetRecent(ctx interface{}, limit int) (interface{}, error) {
	// Convert context
	var ctxVal context.Context
	if c, ok := ctx.(context.Context); ok {
		ctxVal = c
	} else {
		ctxVal = context.Background()
	}

	return w.collector.GetRecent(ctxVal, limit)
}

// cascadeManagerWrapper wraps the cascade.Manager to implement CascadeManagerInterface.
type cascadeManagerWrapper struct {
	manager *cascade.Manager
}

func (w *cascadeManagerWrapper) IsEnabled() bool {
	return w.manager.IsEnabled()
}

func (w *cascadeManagerWrapper) EvaluateResponse(response string, currentTier string) *CascadeDecision {
	tier := cascade.Tier(currentTier)
	result := w.manager.EvaluateResponse(response, tier)
	if result == nil {
		return nil
	}

	// Convert signals
	signals := make([]CascadeQualitySignal, len(result.Signals))
	for i, s := range result.Signals {
		signals[i] = CascadeQualitySignal{
			Type:        string(s.Type),
			Severity:    s.Severity,
			Description: s.Description,
		}
	}

	return &CascadeDecision{
		ShouldCascade: result.ShouldCascade,
		CurrentTier:   string(result.CurrentTier),
		NextTier:      string(result.NextTier),
		Signals:       signals,
		QualityScore:  result.QualityScore,
		Reason:        result.Reason,
	}
}

func (w *cascadeManagerWrapper) GetMetricsAsMap() map[string]interface{} {
	return w.manager.GetMetricsAsMap()
}

// semanticTierWrapper wraps the semantic.Tier to implement SemanticTierInterface.
// This avoids exposing the concrete type and handles the MatchResult conversion.
type semanticTierWrapper struct {
	tier *semantic.Tier
}

func (w *semanticTierWrapper) MatchIntent(query string) (*SemanticMatchResult, error) {
	result, err := w.tier.MatchIntent(query)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	return &SemanticMatchResult{
		Intent:     result.Intent,
		Confidence: result.Confidence,
		LatencyMs:  result.LatencyMs,
	}, nil
}

func (w *semanticTierWrapper) IsEnabled() bool {
	return w.tier.IsEnabled()
}

func (w *semanticTierWrapper) GetMetrics() map[string]interface{} {
	return w.tier.GetMetrics()
}
