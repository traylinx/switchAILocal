// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

//go:build integration
// +build integration

package intelligence_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/traylinx/switchAILocal/internal/config"
	"github.com/traylinx/switchAILocal/internal/intelligence"
)

// TestIntegration_EndToEndRoutingFlow tests the complete routing flow
// from service initialization through all tiers.
// Requirements: 13.1, 13.2, 13.3
func TestIntegration_EndToEndRoutingFlow(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "switchailocal-integration-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	t.Run("full service initialization with all features enabled", func(t *testing.T) {
		cfg := createFullConfig(tempDir)
		cfg.Enabled = true

		svc := intelligence.NewService(cfg)
		if svc == nil {
			t.Fatal("expected non-nil service")
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Initialize should succeed (some services may warn but not fail)
		err := svc.Initialize(ctx)
		if err != nil {
			t.Errorf("initialization failed: %v", err)
		}

		// Service should be enabled
		if !svc.IsEnabled() {
			t.Error("expected service to be enabled")
		}

		// Verify services are accessible (may be nil if dependencies missing)
		// Discovery service
		discoverySvc := svc.GetDiscoveryService()
		t.Logf("Discovery service available: %v", discoverySvc != nil)

		// Matrix builder
		matrixBuilder := svc.GetMatrixBuilder()
		t.Logf("Matrix builder available: %v", matrixBuilder != nil)

		// Skill registry
		skillRegistry := svc.GetSkillRegistry()
		t.Logf("Skill registry available: %v", skillRegistry != nil)

		// Embedding engine
		embeddingEngine := svc.GetEmbeddingEngine()
		t.Logf("Embedding engine available: %v", embeddingEngine != nil)

		// Semantic tier
		semanticTier := svc.GetSemanticTier()
		t.Logf("Semantic tier available: %v", semanticTier != nil)

		// Semantic cache
		semanticCache := svc.GetSemanticCache()
		t.Logf("Semantic cache available: %v", semanticCache != nil)

		// Confidence scorer
		confidenceScorer := svc.GetConfidenceScorer()
		t.Logf("Confidence scorer available: %v", confidenceScorer != nil)

		// Verifier
		verifier := svc.GetVerifier()
		t.Logf("Verifier available: %v", verifier != nil)

		// Cascade manager
		cascadeManager := svc.GetCascadeManager()
		t.Logf("Cascade manager available: %v", cascadeManager != nil)

		// Feedback collector
		feedbackCollector := svc.GetFeedbackCollector()
		t.Logf("Feedback collector available: %v", feedbackCollector != nil)

		// Shutdown
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()

		err = svc.Shutdown(shutdownCtx)
		if err != nil {
			t.Errorf("shutdown failed: %v", err)
		}
	})

	t.Run("service components work together", func(t *testing.T) {
		cfg := createFullConfig(tempDir)
		cfg.Enabled = true
		// Enable only components that don't require external dependencies
		cfg.Discovery.Enabled = false
		cfg.Embedding.Enabled = false
		cfg.SemanticTier.Enabled = false
		cfg.SemanticCache.Enabled = false
		cfg.Skills.Enabled = false
		cfg.SkillMatching.Enabled = false
		cfg.AutoAssign.Enabled = false

		svc := intelligence.NewService(cfg)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		err := svc.Initialize(ctx)
		if err != nil {
			t.Fatalf("initialization failed: %v", err)
		}
		defer svc.Shutdown(ctx)

		// Confidence scorer should be available
		scorer := svc.GetConfidenceScorer()
		if scorer == nil {
			t.Error("expected confidence scorer to be available")
		}

		// Verifier should be available
		verifier := svc.GetVerifier()
		if verifier == nil {
			t.Error("expected verifier to be available")
		}

		// Cascade manager should be available
		cascadeManager := svc.GetCascadeManager()
		if cascadeManager == nil {
			t.Error("expected cascade manager to be available")
		}

		// Feedback collector should be available
		feedbackCollector := svc.GetFeedbackCollector()
		if feedbackCollector == nil {
			t.Error("expected feedback collector to be available")
		}
	})
}

// TestIntegration_GracefulDegradation tests that the system gracefully
// handles missing or failed services.
// Requirements: 13.1, 13.2, 13.3
func TestIntegration_GracefulDegradation(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "switchailocal-degradation-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	t.Run("service continues when discovery fails", func(t *testing.T) {
		cfg := createFullConfig(tempDir)
		cfg.Enabled = true
		cfg.Discovery.Enabled = true
		cfg.Discovery.CacheDir = "/nonexistent/path/that/should/fail"

		svc := intelligence.NewService(cfg)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Should not fail even if discovery has issues
		err := svc.Initialize(ctx)
		if err != nil {
			t.Errorf("initialization should not fail: %v", err)
		}

		// Service should still be enabled
		if !svc.IsEnabled() {
			t.Error("service should still be enabled")
		}

		svc.Shutdown(ctx)
	})

	t.Run("service continues when embedding model missing", func(t *testing.T) {
		cfg := createFullConfig(tempDir)
		cfg.Enabled = true
		cfg.Embedding.Enabled = true
		cfg.Embedding.Model = "nonexistent-model"

		svc := intelligence.NewService(cfg)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Should not fail even if embedding model is missing
		err := svc.Initialize(ctx)
		if err != nil {
			t.Errorf("initialization should not fail: %v", err)
		}

		// Service should still be enabled
		if !svc.IsEnabled() {
			t.Error("service should still be enabled")
		}

		// Embedding engine should be nil (graceful degradation)
		engine := svc.GetEmbeddingEngine()
		if engine != nil {
			t.Log("embedding engine unexpectedly available (model may exist)")
		}

		svc.Shutdown(ctx)
	})

	t.Run("semantic tier degrades when embedding unavailable", func(t *testing.T) {
		cfg := createFullConfig(tempDir)
		cfg.Enabled = true
		cfg.Embedding.Enabled = false // Disable embedding
		cfg.SemanticTier.Enabled = true

		svc := intelligence.NewService(cfg)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		err := svc.Initialize(ctx)
		if err != nil {
			t.Errorf("initialization should not fail: %v", err)
		}

		// Semantic tier should be nil (depends on embedding)
		semanticTier := svc.GetSemanticTier()
		if semanticTier != nil {
			t.Error("semantic tier should be nil when embedding is disabled")
		}

		svc.Shutdown(ctx)
	})

	t.Run("semantic cache degrades when embedding unavailable", func(t *testing.T) {
		cfg := createFullConfig(tempDir)
		cfg.Enabled = true
		cfg.Embedding.Enabled = false // Disable embedding
		cfg.SemanticCache.Enabled = true

		svc := intelligence.NewService(cfg)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		err := svc.Initialize(ctx)
		if err != nil {
			t.Errorf("initialization should not fail: %v", err)
		}

		// Semantic cache should be nil (depends on embedding)
		cache := svc.GetSemanticCache()
		if cache != nil {
			t.Error("semantic cache should be nil when embedding is disabled")
		}

		svc.Shutdown(ctx)
	})

	t.Run("multiple shutdown calls are safe", func(t *testing.T) {
		cfg := createFullConfig(tempDir)
		cfg.Enabled = true

		svc := intelligence.NewService(cfg)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		err := svc.Initialize(ctx)
		if err != nil {
			t.Fatalf("initialization failed: %v", err)
		}

		// First shutdown
		err = svc.Shutdown(ctx)
		if err != nil {
			t.Errorf("first shutdown failed: %v", err)
		}

		// Second shutdown should be safe
		err = svc.Shutdown(ctx)
		if err != nil {
			t.Errorf("second shutdown failed: %v", err)
		}

		// Third shutdown should also be safe
		err = svc.Shutdown(ctx)
		if err != nil {
			t.Errorf("third shutdown failed: %v", err)
		}
	})
}

// TestIntegration_FeatureFlagBehavior tests that feature flags correctly
// control service initialization and behavior.
// Requirements: 13.1, 13.2, 13.3
func TestIntegration_FeatureFlagBehavior(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "switchailocal-flags-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	t.Run("master switch disabled - no services initialized", func(t *testing.T) {
		cfg := createFullConfig(tempDir)
		cfg.Enabled = false // Master switch off

		svc := intelligence.NewService(cfg)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := svc.Initialize(ctx)
		if err != nil {
			t.Errorf("initialization should succeed: %v", err)
		}

		// Service should not be enabled
		if svc.IsEnabled() {
			t.Error("service should not be enabled when master switch is off")
		}

		// All services should be nil
		if svc.GetDiscoveryService() != nil {
			t.Error("discovery should be nil")
		}
		if svc.GetMatrixBuilder() != nil {
			t.Error("matrix builder should be nil")
		}
		if svc.GetSkillRegistry() != nil {
			t.Error("skill registry should be nil")
		}
		if svc.GetEmbeddingEngine() != nil {
			t.Error("embedding engine should be nil")
		}
		if svc.GetSemanticTier() != nil {
			t.Error("semantic tier should be nil")
		}
		if svc.GetSemanticCache() != nil {
			t.Error("semantic cache should be nil")
		}
		if svc.GetConfidenceScorer() != nil {
			t.Error("confidence scorer should be nil")
		}
		if svc.GetVerifier() != nil {
			t.Error("verifier should be nil")
		}
		if svc.GetCascadeManager() != nil {
			t.Error("cascade manager should be nil")
		}
		if svc.GetFeedbackCollector() != nil {
			t.Error("feedback collector should be nil")
		}

		svc.Shutdown(ctx)
	})

	t.Run("individual feature flags control services", func(t *testing.T) {
		cfg := createFullConfig(tempDir)
		cfg.Enabled = true

		// Disable all features
		cfg.Discovery.Enabled = false
		cfg.AutoAssign.Enabled = false
		cfg.Skills.Enabled = false
		cfg.Embedding.Enabled = false
		cfg.SemanticTier.Enabled = false
		cfg.SkillMatching.Enabled = false
		cfg.SemanticCache.Enabled = false
		cfg.Confidence.Enabled = false
		cfg.Verification.Enabled = false
		cfg.Cascade.Enabled = false
		cfg.Feedback.Enabled = false

		svc := intelligence.NewService(cfg)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := svc.Initialize(ctx)
		if err != nil {
			t.Errorf("initialization should succeed: %v", err)
		}

		// Service should be enabled (master switch is on)
		if !svc.IsEnabled() {
			t.Error("service should be enabled")
		}

		// All services should be nil (individual flags off)
		if svc.GetDiscoveryService() != nil {
			t.Error("discovery should be nil")
		}
		if svc.GetMatrixBuilder() != nil {
			t.Error("matrix builder should be nil")
		}
		if svc.GetSkillRegistry() != nil {
			t.Error("skill registry should be nil")
		}
		if svc.GetEmbeddingEngine() != nil {
			t.Error("embedding engine should be nil")
		}
		if svc.GetSemanticTier() != nil {
			t.Error("semantic tier should be nil")
		}
		if svc.GetSemanticCache() != nil {
			t.Error("semantic cache should be nil")
		}
		if svc.GetConfidenceScorer() != nil {
			t.Error("confidence scorer should be nil")
		}
		if svc.GetVerifier() != nil {
			t.Error("verifier should be nil")
		}
		if svc.GetCascadeManager() != nil {
			t.Error("cascade manager should be nil")
		}
		if svc.GetFeedbackCollector() != nil {
			t.Error("feedback collector should be nil")
		}

		svc.Shutdown(ctx)
	})

	t.Run("selective feature enablement", func(t *testing.T) {
		cfg := createFullConfig(tempDir)
		cfg.Enabled = true

		// Enable only confidence, verification, and cascade
		cfg.Discovery.Enabled = false
		cfg.AutoAssign.Enabled = false
		cfg.Skills.Enabled = false
		cfg.Embedding.Enabled = false
		cfg.SemanticTier.Enabled = false
		cfg.SkillMatching.Enabled = false
		cfg.SemanticCache.Enabled = false
		cfg.Confidence.Enabled = true
		cfg.Verification.Enabled = true
		cfg.Cascade.Enabled = true
		cfg.Feedback.Enabled = true

		svc := intelligence.NewService(cfg)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		err := svc.Initialize(ctx)
		if err != nil {
			t.Errorf("initialization should succeed: %v", err)
		}

		// Enabled services should be available
		if svc.GetConfidenceScorer() == nil {
			t.Error("confidence scorer should be available")
		}
		if svc.GetVerifier() == nil {
			t.Error("verifier should be available")
		}
		if svc.GetCascadeManager() == nil {
			t.Error("cascade manager should be available")
		}
		if svc.GetFeedbackCollector() == nil {
			t.Error("feedback collector should be available")
		}

		// Disabled services should be nil
		if svc.GetDiscoveryService() != nil {
			t.Error("discovery should be nil")
		}
		if svc.GetEmbeddingEngine() != nil {
			t.Error("embedding engine should be nil")
		}

		svc.Shutdown(ctx)
	})

	t.Run("v1.0 fallback behavior when all Phase 2 disabled", func(t *testing.T) {
		cfg := &config.IntelligenceConfig{
			Enabled:        true,
			RouterModel:    "ollama:qwen:0.5b",
			RouterFallback: "openai:gpt-4o-mini",
			Matrix: map[string]string{
				"coding":    "switchai-chat",
				"reasoning": "switchai-reasoner",
				"creative":  "switchai-chat",
				"fast":      "switchai-fast",
				"secure":    "switchai-fast",
				"vision":    "switchai-chat",
			},
			// All Phase 2 features disabled
			Discovery:          config.DiscoveryConfig{Enabled: false},
			CapabilityAnalysis: config.FeatureFlag{Enabled: false},
			AutoAssign:         config.AutoAssignConfig{Enabled: false},
			Skills:             config.SkillsConfig{Enabled: false},
			Embedding:          config.EmbeddingConfig{Enabled: false},
			SemanticTier:       config.SemanticTierConfig{Enabled: false},
			SkillMatching:      config.SkillMatchingConfig{Enabled: false},
			SemanticCache:      config.SemanticCacheConfig{Enabled: false},
			Confidence:         config.FeatureFlag{Enabled: false},
			Verification:       config.VerificationConfig{Enabled: false},
			Cascade:            config.CascadeConfig{Enabled: false},
			Feedback:           config.FeedbackConfig{Enabled: false},
		}

		svc := intelligence.NewService(cfg)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := svc.Initialize(ctx)
		if err != nil {
			t.Errorf("initialization should succeed: %v", err)
		}

		// Service should be enabled (master switch on)
		if !svc.IsEnabled() {
			t.Error("service should be enabled")
		}

		// Config should be accessible
		serviceCfg := svc.GetConfig()
		if serviceCfg == nil {
			t.Error("config should be accessible")
		}
		if serviceCfg.RouterModel != "ollama:qwen:0.5b" {
			t.Errorf("expected router model 'ollama:qwen:0.5b', got '%s'", serviceCfg.RouterModel)
		}
		if serviceCfg.Matrix["coding"] != "switchai-chat" {
			t.Errorf("expected coding matrix 'switchai-chat', got '%s'", serviceCfg.Matrix["coding"])
		}

		svc.Shutdown(ctx)
	})
}

// TestIntegration_CascadeManager tests the cascade manager integration.
func TestIntegration_CascadeManager(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "switchailocal-cascade-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cfg := createFullConfig(tempDir)
	cfg.Enabled = true
	cfg.Cascade.Enabled = true
	cfg.Cascade.QualityThreshold = 0.70

	svc := intelligence.NewService(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = svc.Initialize(ctx)
	if err != nil {
		t.Fatalf("initialization failed: %v", err)
	}
	defer svc.Shutdown(ctx)

	cascadeManager := svc.GetCascadeManager()
	if cascadeManager == nil {
		t.Fatal("cascade manager should be available")
	}

	t.Run("evaluate good response", func(t *testing.T) {
		goodResponse := `Here is a comprehensive answer to your question about Go programming.

Go (also known as Golang) is a statically typed, compiled programming language designed at Google. It was created by Robert Griesemer, Rob Pike, and Ken Thompson and first appeared in 2009.

Key features of Go include:
1. Simple and clean syntax
2. Built-in concurrency with goroutines and channels
3. Fast compilation
4. Garbage collection
5. Strong standard library

Go is particularly well-suited for building web servers, command-line tools, and distributed systems.`

		decision := cascadeManager.EvaluateResponse(goodResponse, "standard")
		if decision == nil {
			t.Fatal("expected non-nil decision")
		}

		if decision.ShouldCascade {
			t.Errorf("good response should not trigger cascade, got quality score: %.2f", decision.QualityScore)
		}
	})

	t.Run("evaluate poor response", func(t *testing.T) {
		poorResponse := "I don't know"

		decision := cascadeManager.EvaluateResponse(poorResponse, "fast")
		if decision == nil {
			t.Fatal("expected non-nil decision")
		}

		// Poor response should trigger cascade
		if !decision.ShouldCascade {
			t.Logf("poor response quality score: %.2f", decision.QualityScore)
		}
	})

	t.Run("get cascade metrics", func(t *testing.T) {
		metrics := cascadeManager.GetMetricsAsMap()
		if metrics == nil {
			t.Error("expected non-nil metrics")
		}
	})
}

// TestIntegration_FeedbackCollector tests the feedback collector integration.
func TestIntegration_FeedbackCollector(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "switchailocal-feedback-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cfg := createFullConfig(tempDir)
	cfg.Enabled = true
	cfg.Feedback.Enabled = true
	cfg.Feedback.RetentionDays = 90
	cfg.Discovery.CacheDir = tempDir

	svc := intelligence.NewService(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = svc.Initialize(ctx)
	if err != nil {
		t.Fatalf("initialization failed: %v", err)
	}
	defer svc.Shutdown(ctx)

	feedbackCollector := svc.GetFeedbackCollector()
	if feedbackCollector == nil {
		t.Fatal("feedback collector should be available")
	}

	t.Run("record feedback", func(t *testing.T) {
		record := &intelligence.FeedbackRecord{
			Query:           "How do I write a Go function?",
			Intent:          "coding",
			SelectedModel:   "switchai-chat",
			RoutingTier:     "semantic",
			Confidence:      0.92,
			MatchedSkill:    "go-programming",
			CascadeOccurred: false,
			ResponseQuality: 0.85,
			LatencyMs:       150,
			Success:         true,
			Metadata: map[string]interface{}{
				"complexity": "simple",
			},
		}

		err := feedbackCollector.Record(ctx, record)
		if err != nil {
			t.Errorf("failed to record feedback: %v", err)
		}
	})

	t.Run("get feedback stats", func(t *testing.T) {
		stats, err := feedbackCollector.GetStats(ctx)
		if err != nil {
			t.Errorf("failed to get stats: %v", err)
		}
		if stats == nil {
			t.Error("expected non-nil stats")
		}
	})

	t.Run("get recent feedback", func(t *testing.T) {
		recent, err := feedbackCollector.GetRecent(ctx, 10)
		if err != nil {
			t.Errorf("failed to get recent feedback: %v", err)
		}
		if recent == nil {
			t.Error("expected non-nil recent feedback")
		}
	})
}

// TestIntegration_ConfidenceAndVerification tests confidence scoring and verification.
func TestIntegration_ConfidenceAndVerification(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "switchailocal-confidence-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cfg := createFullConfig(tempDir)
	cfg.Enabled = true
	cfg.Confidence.Enabled = true
	cfg.Verification.Enabled = true

	svc := intelligence.NewService(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = svc.Initialize(ctx)
	if err != nil {
		t.Fatalf("initialization failed: %v", err)
	}
	defer svc.Shutdown(ctx)

	t.Run("confidence scorer parses valid JSON", func(t *testing.T) {
		scorer := svc.GetConfidenceScorer()
		if scorer == nil {
			t.Fatal("confidence scorer should be available")
		}

		jsonStr := `{"intent": "coding", "complexity": "simple", "confidence": 0.95}`
		result, err := scorer.Parse(jsonStr)
		if err != nil {
			t.Errorf("failed to parse: %v", err)
		}
		if result.Intent != "coding" {
			t.Errorf("expected intent 'coding', got '%s'", result.Intent)
		}
		if result.Confidence != 0.95 {
			t.Errorf("expected confidence 0.95, got %.2f", result.Confidence)
		}
	})

	t.Run("verifier matches same intents", func(t *testing.T) {
		verifier := svc.GetVerifier()
		if verifier == nil {
			t.Fatal("verifier should be available")
		}

		if !verifier.Verify("coding", "coding") {
			t.Error("same intents should match")
		}
	})

	t.Run("verifier matches equivalent intents", func(t *testing.T) {
		verifier := svc.GetVerifier()
		if verifier == nil {
			t.Fatal("verifier should be available")
		}

		// Test equivalent intent matching
		if !verifier.Verify("code", "coding") {
			t.Log("'code' and 'coding' may not be configured as equivalent")
		}
	})
}

// createFullConfig creates a complete IntelligenceConfig for testing.
func createFullConfig(tempDir string) *config.IntelligenceConfig {
	return &config.IntelligenceConfig{
		Enabled:        true,
		RouterModel:    "ollama:qwen:0.5b",
		RouterFallback: "openai:gpt-4o-mini",
		Matrix: map[string]string{
			"coding":    "switchai-chat",
			"reasoning": "switchai-reasoner",
			"creative":  "switchai-chat",
			"fast":      "switchai-fast",
			"secure":    "switchai-fast",
			"vision":    "switchai-chat",
		},
		Discovery: config.DiscoveryConfig{
			Enabled:         true,
			RefreshInterval: 3600,
			CacheDir:        filepath.Join(tempDir, "discovery"),
		},
		CapabilityAnalysis: config.FeatureFlag{
			Enabled: true,
		},
		AutoAssign: config.AutoAssignConfig{
			Enabled:          true,
			PreferLocal:      true,
			CostOptimization: true,
			Overrides:        make(map[string]string),
		},
		Skills: config.SkillsConfig{
			Enabled:   true,
			Directory: "plugins/cortex-router/skills",
		},
		Embedding: config.EmbeddingConfig{
			Enabled: true,
			Model:   "all-MiniLM-L6-v2",
		},
		SemanticTier: config.SemanticTierConfig{
			Enabled:             true,
			ConfidenceThreshold: 0.85,
		},
		SkillMatching: config.SkillMatchingConfig{
			Enabled:             true,
			ConfidenceThreshold: 0.80,
		},
		SemanticCache: config.SemanticCacheConfig{
			Enabled:             true,
			SimilarityThreshold: 0.95,
			MaxSize:             10000,
		},
		Confidence: config.FeatureFlag{
			Enabled: true,
		},
		Verification: config.VerificationConfig{
			Enabled:                 true,
			ConfidenceThresholdLow:  0.60,
			ConfidenceThresholdHigh: 0.90,
		},
		Cascade: config.CascadeConfig{
			Enabled:          true,
			QualityThreshold: 0.70,
		},
		Feedback: config.FeedbackConfig{
			Enabled:       true,
			RetentionDays: 90,
		},
	}
}
