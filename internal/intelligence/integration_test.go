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

// TestCheckpoint_VerifyFoundation tests the checkpoint requirements for µPhase 1.
// This test verifies:
// 1. Server starts with intelligence.enabled: false (no errors)
// 2. Server starts with intelligence.enabled: true (service initializes)
// 3. Service can be shut down gracefully
func TestCheckpoint_VerifyFoundation(t *testing.T) {
	// Create a temporary auth directory for testing
	tempDir, err := os.MkdirTemp("", "switchailocal-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	t.Run("intelligence disabled - no errors", func(t *testing.T) {
		cfg := &config.IntelligenceConfig{
			Enabled:        false,
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
		}

		svc := intelligence.NewService(cfg)
		if svc == nil {
			t.Fatal("expected non-nil service")
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Initialize should succeed without errors
		err := svc.Initialize(ctx)
		if err != nil {
			t.Errorf("initialization failed: %v", err)
		}

		// Service should not be enabled
		if svc.IsEnabled() {
			t.Error("expected service to be disabled")
		}

		// Shutdown should succeed
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()

		err = svc.Shutdown(shutdownCtx)
		if err != nil {
			t.Errorf("shutdown failed: %v", err)
		}
	})

	t.Run("intelligence enabled - service initializes", func(t *testing.T) {
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
			Discovery: config.DiscoveryConfig{
				Enabled:         false, // Disabled for now (µP2)
				RefreshInterval: 3600,
				CacheDir:        filepath.Join(tempDir, "discovery"),
			},
			CapabilityAnalysis: config.FeatureFlag{
				Enabled: false, // Disabled for now (µP3)
			},
			AutoAssign: config.AutoAssignConfig{
				Enabled:          false, // Disabled for now (µP4)
				PreferLocal:      true,
				CostOptimization: true,
				Overrides:        make(map[string]string),
			},
			Skills: config.SkillsConfig{
				Enabled:   false, // Disabled for now (µP5)
				Directory: "plugins/cortex-router/skills",
			},
			Embedding: config.EmbeddingConfig{
				Enabled: false, // Disabled for now (µP6)
				Model:   "all-MiniLM-L6-v2",
			},
			SemanticTier: config.SemanticTierConfig{
				Enabled:             false, // Disabled for now (µP7)
				ConfidenceThreshold: 0.85,
			},
			SkillMatching: config.SkillMatchingConfig{
				Enabled:             false, // Disabled for now (µP8)
				ConfidenceThreshold: 0.80,
			},
			SemanticCache: config.SemanticCacheConfig{
				Enabled:             false, // Disabled for now (µP9)
				SimilarityThreshold: 0.95,
				MaxSize:             10000,
			},
			Confidence: config.FeatureFlag{
				Enabled: false, // Disabled for now (µP10)
			},
			Verification: config.VerificationConfig{
				Enabled:                 false, // Disabled for now (µP11)
				ConfidenceThresholdLow:  0.60,
				ConfidenceThresholdHigh: 0.90,
			},
			Cascade: config.CascadeConfig{
				Enabled:          false, // Disabled for now (µP12)
				QualityThreshold: 0.70,
			},
			Feedback: config.FeedbackConfig{
				Enabled:       false, // Disabled for now (µP13)
				RetentionDays: 90,
			},
		}

		svc := intelligence.NewService(cfg)
		if svc == nil {
			t.Fatal("expected non-nil service")
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Initialize should succeed
		err := svc.Initialize(ctx)
		if err != nil {
			t.Errorf("initialization failed: %v", err)
		}

		// Service should be enabled
		if !svc.IsEnabled() {
			t.Error("expected service to be enabled")
		}

		// Shutdown should succeed
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()

		err = svc.Shutdown(shutdownCtx)
		if err != nil {
			t.Errorf("shutdown failed: %v", err)
		}
	})

	t.Run("graceful shutdown", func(t *testing.T) {
		cfg := &config.IntelligenceConfig{
			Enabled:        true,
			RouterModel:    "ollama:qwen:0.5b",
			RouterFallback: "openai:gpt-4o-mini",
		}

		svc := intelligence.NewService(cfg)

		// Initialize
		initCtx, initCancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := svc.Initialize(initCtx)
		initCancel()
		if err != nil {
			t.Fatalf("initialization failed: %v", err)
		}

		// Shutdown with short timeout
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer shutdownCancel()

		start := time.Now()
		err = svc.Shutdown(shutdownCtx)
		duration := time.Since(start)

		if err != nil {
			t.Errorf("shutdown failed: %v", err)
		}

		// Shutdown should complete quickly (< 1 second for now since no services are running)
		if duration > 1*time.Second {
			t.Errorf("shutdown took too long: %v", duration)
		}

		// Multiple shutdowns should be safe
		err = svc.Shutdown(shutdownCtx)
		if err != nil {
			t.Errorf("second shutdown failed: %v", err)
		}
	})
}
