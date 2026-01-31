// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package intelligence

import (
	"context"
	"testing"
	"time"

	"github.com/traylinx/switchAILocal/internal/config"
)

// TestNewService verifies that NewService creates a service instance correctly.
func TestNewService(t *testing.T) {
	t.Run("creates service with nil config", func(t *testing.T) {
		svc := NewService(nil)
		if svc == nil {
			t.Fatal("expected non-nil service")
		}
		if svc.config == nil {
			t.Fatal("expected non-nil config")
		}
		if svc.enabled {
			t.Error("expected service to be disabled with nil config")
		}
	})

	t.Run("creates service with disabled config", func(t *testing.T) {
		cfg := &config.IntelligenceConfig{
			Enabled: false,
		}
		svc := NewService(cfg)
		if svc == nil {
			t.Fatal("expected non-nil service")
		}
		if svc.enabled {
			t.Error("expected service to be disabled")
		}
	})

	t.Run("creates service with enabled config", func(t *testing.T) {
		cfg := &config.IntelligenceConfig{
			Enabled: true,
		}
		svc := NewService(cfg)
		if svc == nil {
			t.Fatal("expected non-nil service")
		}
		if !svc.enabled {
			t.Error("expected service to be enabled")
		}
	})
}

// TestServiceInitialize verifies that Initialize works correctly.
func TestServiceInitialize(t *testing.T) {
	t.Run("initialize with disabled service", func(t *testing.T) {
		cfg := &config.IntelligenceConfig{
			Enabled: false,
		}
		svc := NewService(cfg)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := svc.Initialize(ctx)
		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
	})

	t.Run("initialize with enabled service", func(t *testing.T) {
		cfg := &config.IntelligenceConfig{
			Enabled: true,
		}
		svc := NewService(cfg)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := svc.Initialize(ctx)
		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
	})

	t.Run("initialize with cancelled context", func(t *testing.T) {
		cfg := &config.IntelligenceConfig{
			Enabled: true,
		}
		svc := NewService(cfg)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		err := svc.Initialize(ctx)
		// Should not error even with cancelled context since no services are initialized yet
		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
	})
}

// TestServiceIsEnabled verifies that IsEnabled returns correct status.
func TestServiceIsEnabled(t *testing.T) {
	t.Run("returns false when disabled", func(t *testing.T) {
		cfg := &config.IntelligenceConfig{
			Enabled: false,
		}
		svc := NewService(cfg)

		if svc.IsEnabled() {
			t.Error("expected IsEnabled to return false")
		}
	})

	t.Run("returns true when enabled", func(t *testing.T) {
		cfg := &config.IntelligenceConfig{
			Enabled: true,
		}
		svc := NewService(cfg)

		if !svc.IsEnabled() {
			t.Error("expected IsEnabled to return true")
		}
	})

	t.Run("returns correct status after initialization", func(t *testing.T) {
		cfg := &config.IntelligenceConfig{
			Enabled: true,
		}
		svc := NewService(cfg)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := svc.Initialize(ctx)
		if err != nil {
			t.Fatalf("initialization failed: %v", err)
		}

		if !svc.IsEnabled() {
			t.Error("expected IsEnabled to return true after initialization")
		}
	})
}

// TestServiceShutdown verifies that Shutdown works correctly.
func TestServiceShutdown(t *testing.T) {
	t.Run("shutdown with disabled service", func(t *testing.T) {
		cfg := &config.IntelligenceConfig{
			Enabled: false,
		}
		svc := NewService(cfg)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := svc.Shutdown(ctx)
		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
	})

	t.Run("shutdown with enabled service", func(t *testing.T) {
		cfg := &config.IntelligenceConfig{
			Enabled: true,
		}
		svc := NewService(cfg)

		// Initialize first
		initCtx, initCancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := svc.Initialize(initCtx)
		initCancel()
		if err != nil {
			t.Fatalf("initialization failed: %v", err)
		}

		// Now shutdown
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()

		err = svc.Shutdown(shutdownCtx)
		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
	})

	t.Run("shutdown without initialization", func(t *testing.T) {
		cfg := &config.IntelligenceConfig{
			Enabled: true,
		}
		svc := NewService(cfg)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Shutdown without Initialize should not error
		err := svc.Shutdown(ctx)
		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
	})

	t.Run("shutdown with cancelled context", func(t *testing.T) {
		cfg := &config.IntelligenceConfig{
			Enabled: true,
		}
		svc := NewService(cfg)

		// Initialize first
		initCtx, initCancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := svc.Initialize(initCtx)
		initCancel()
		if err != nil {
			t.Fatalf("initialization failed: %v", err)
		}

		// Shutdown with cancelled context
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		err = svc.Shutdown(ctx)
		// Should not error even with cancelled context since no services are running yet
		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
	})
}

// TestServiceGetConfig verifies that GetConfig returns the correct configuration.
func TestServiceGetConfig(t *testing.T) {
	t.Run("returns config", func(t *testing.T) {
		cfg := &config.IntelligenceConfig{
			Enabled:     true,
			RouterModel: "test-model",
		}
		svc := NewService(cfg)

		returnedCfg := svc.GetConfig()
		if returnedCfg == nil {
			t.Fatal("expected non-nil config")
		}
		if returnedCfg.RouterModel != "test-model" {
			t.Errorf("expected RouterModel to be 'test-model', got: %s", returnedCfg.RouterModel)
		}
	})
}

// TestServiceConcurrency verifies that the service is safe for concurrent access.
func TestServiceConcurrency(t *testing.T) {
	cfg := &config.IntelligenceConfig{
		Enabled: true,
	}
	svc := NewService(cfg)

	// Initialize the service
	initCtx, initCancel := context.WithTimeout(context.Background(), 5*time.Second)
	err := svc.Initialize(initCtx)
	initCancel()
	if err != nil {
		t.Fatalf("initialization failed: %v", err)
	}

	// Run concurrent operations
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- true }()
			// Test concurrent reads
			_ = svc.IsEnabled()
			_ = svc.GetConfig()
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	err = svc.Shutdown(shutdownCtx)
	if err != nil {
		t.Errorf("shutdown failed: %v", err)
	}
}
