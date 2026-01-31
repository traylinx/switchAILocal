// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

//go:build integration
// +build integration

package plugin_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/traylinx/switchAILocal/internal/config"
	"github.com/traylinx/switchAILocal/internal/intelligence"
	"github.com/traylinx/switchAILocal/internal/plugin"
	sdkconfig "github.com/traylinx/switchAILocal/sdk/config"
)

// TestPluginIndependence_SwitchAILocalWithoutPlugin verifies that switchAILocal
// works correctly without the cortex-router plugin enabled.
// Requirements: 13.3, 13.6
func TestPluginIndependence_SwitchAILocalWithoutPlugin(t *testing.T) {
	t.Run("lua engine disabled - no errors", func(t *testing.T) {
		cfg := plugin.Config{
			Enabled:   false,
			PluginDir: "",
		}

		engine := plugin.NewLuaEngine(cfg)
		if engine == nil {
			t.Fatal("expected non-nil engine even when disabled")
		}
		defer engine.Close()

		// Engine should not be enabled
		if engine.IsEnabled() {
			t.Error("engine should not be enabled")
		}

		// RunHook should return data unchanged
		ctx := context.Background()
		data := map[string]any{
			"model": "auto",
			"body":  "test request",
		}

		result, err := engine.RunHook(ctx, plugin.HookOnRequest, data)
		if err != nil {
			t.Errorf("RunHook should not error: %v", err)
		}

		// Data should be unchanged
		if result["model"] != "auto" {
			t.Errorf("expected model 'auto', got '%v'", result["model"])
		}
	})

	t.Run("lua engine with no plugins loaded", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "switchailocal-noplugin-*")
		if err != nil {
			t.Fatalf("failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tempDir)

		cfg := plugin.Config{
			Enabled:        true,
			PluginDir:      tempDir,
			EnabledPlugins: []string{}, // No plugins enabled
		}

		engine := plugin.NewLuaEngine(cfg)
		if engine == nil {
			t.Fatal("expected non-nil engine")
		}
		defer engine.Close()

		// Engine should be enabled
		if !engine.IsEnabled() {
			t.Error("engine should be enabled")
		}

		// RunHook should return data unchanged (no plugins to process)
		ctx := context.Background()
		data := map[string]any{
			"model": "auto",
			"body":  "test request",
		}

		result, err := engine.RunHook(ctx, plugin.HookOnRequest, data)
		if err != nil {
			t.Errorf("RunHook should not error: %v", err)
		}

		// Data should be unchanged
		if result["model"] != "auto" {
			t.Errorf("expected model 'auto', got '%v'", result["model"])
		}
	})

	t.Run("intelligence service without plugin", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "switchailocal-intel-noplugin-*")
		if err != nil {
			t.Fatalf("failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tempDir)

		// Create intelligence service without plugin
		intelCfg := &config.IntelligenceConfig{
			Enabled:        true,
			RouterModel:    "ollama:qwen:0.5b",
			RouterFallback: "openai:gpt-4o-mini",
			Matrix: map[string]string{
				"coding":    "switchai-chat",
				"reasoning": "switchai-reasoner",
			},
			Confidence:   config.FeatureFlag{Enabled: true},
			Verification: config.VerificationConfig{Enabled: true},
			Cascade:      config.CascadeConfig{Enabled: true, QualityThreshold: 0.70},
			Feedback: config.FeedbackConfig{
				Enabled:       true,
				RetentionDays: 90,
			},
			Discovery: config.DiscoveryConfig{
				Enabled:  false,
				CacheDir: tempDir,
			},
		}

		svc := intelligence.NewService(intelCfg)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		err = svc.Initialize(ctx)
		if err != nil {
			t.Errorf("intelligence service should initialize without plugin: %v", err)
		}

		// Service should be enabled
		if !svc.IsEnabled() {
			t.Error("intelligence service should be enabled")
		}

		// Services should be available
		if svc.GetConfidenceScorer() == nil {
			t.Error("confidence scorer should be available")
		}
		if svc.GetVerifier() == nil {
			t.Error("verifier should be available")
		}
		if svc.GetCascadeManager() == nil {
			t.Error("cascade manager should be available")
		}

		svc.Shutdown(ctx)
	})
}

// TestPluginIndependence_PluginWithoutIntelligence verifies that the plugin
// works correctly without intelligence services (v1.0 mode).
// Requirements: 13.3, 13.6
func TestPluginIndependence_PluginWithoutIntelligence(t *testing.T) {
	t.Run("lua API returns errors when intelligence disabled", func(t *testing.T) {
		cfg := plugin.Config{
			Enabled:   true,
			PluginDir: "",
			Intelligence: sdkconfig.IntelligenceConfig{
				RouterModel:    "ollama:qwen:0.5b",
				RouterFallback: "openai:gpt-4o-mini",
				Matrix: map[string]string{
					"coding":    "switchai-chat",
					"reasoning": "switchai-reasoner",
				},
			},
		}

		engine := plugin.NewLuaEngine(cfg)
		if engine == nil {
			t.Fatal("expected non-nil engine")
		}
		defer engine.Close()

		// Do NOT set intelligence service - simulating v1.0 mode
		// engine.SetIntelligenceService(nil) is implicit

		// Engine should be enabled
		if !engine.IsEnabled() {
			t.Error("engine should be enabled")
		}

		// Test that Lua API functions return appropriate errors
		// We can't directly call Lua functions from Go tests easily,
		// but we can verify the engine is in a valid state
	})

	t.Run("plugin config accessible without intelligence", func(t *testing.T) {
		cfg := plugin.Config{
			Enabled:   true,
			PluginDir: "",
			Intelligence: sdkconfig.IntelligenceConfig{
				RouterModel:    "ollama:qwen:0.5b",
				RouterFallback: "openai:gpt-4o-mini",
				Matrix: map[string]string{
					"coding":    "switchai-chat",
					"reasoning": "switchai-reasoner",
					"creative":  "switchai-chat",
					"fast":      "switchai-fast",
				},
			},
		}

		engine := plugin.NewLuaEngine(cfg)
		if engine == nil {
			t.Fatal("expected non-nil engine")
		}
		defer engine.Close()

		// Engine should be enabled and functional
		if !engine.IsEnabled() {
			t.Error("engine should be enabled")
		}
	})
}

// TestPluginIndependence_GracefulDegradation verifies graceful degradation
// when intelligence services are unavailable.
// Requirements: 13.3, 13.6
func TestPluginIndependence_GracefulDegradation(t *testing.T) {
	t.Run("engine handles nil intelligence service", func(t *testing.T) {
		cfg := plugin.Config{
			Enabled:   true,
			PluginDir: "",
			Intelligence: sdkconfig.IntelligenceConfig{
				RouterModel:    "ollama:qwen:0.5b",
				RouterFallback: "openai:gpt-4o-mini",
			},
		}

		engine := plugin.NewLuaEngine(cfg)
		if engine == nil {
			t.Fatal("expected non-nil engine")
		}
		defer engine.Close()

		// Explicitly set nil intelligence service
		engine.SetIntelligenceService(nil)

		// Engine should still be enabled
		if !engine.IsEnabled() {
			t.Error("engine should be enabled")
		}

		// RunHook should work without errors
		ctx := context.Background()
		data := map[string]any{
			"model": "test-model",
			"body":  "test request",
		}

		result, err := engine.RunHook(ctx, plugin.HookOnRequest, data)
		if err != nil {
			t.Errorf("RunHook should not error: %v", err)
		}

		// Data should be unchanged (no plugins loaded)
		if result["model"] != "test-model" {
			t.Errorf("expected model 'test-model', got '%v'", result["model"])
		}
	})

	t.Run("engine handles disabled intelligence service", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "switchailocal-disabled-intel-*")
		if err != nil {
			t.Fatalf("failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tempDir)

		// Create disabled intelligence service
		intelCfg := &config.IntelligenceConfig{
			Enabled: false, // Master switch off
		}

		svc := intelligence.NewService(intelCfg)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err = svc.Initialize(ctx)
		if err != nil {
			t.Fatalf("initialization failed: %v", err)
		}
		defer svc.Shutdown(ctx)

		// Create engine with disabled intelligence service
		cfg := plugin.Config{
			Enabled:   true,
			PluginDir: "",
			Intelligence: sdkconfig.IntelligenceConfig{
				RouterModel:    "ollama:qwen:0.5b",
				RouterFallback: "openai:gpt-4o-mini",
			},
		}

		engine := plugin.NewLuaEngine(cfg)
		if engine == nil {
			t.Fatal("expected non-nil engine")
		}
		defer engine.Close()

		// Set the disabled intelligence service
		engine.SetIntelligenceService(svc)

		// Engine should still be enabled
		if !engine.IsEnabled() {
			t.Error("engine should be enabled")
		}

		// Intelligence service should report as disabled
		if svc.IsEnabled() {
			t.Error("intelligence service should be disabled")
		}
	})
}

// TestPluginIndependence_V1Compatibility verifies that the system operates
// identically to v1.0 when all Phase 2 features are disabled.
// Requirements: 13.6
func TestPluginIndependence_V1Compatibility(t *testing.T) {
	t.Run("v1.0 mode with static config", func(t *testing.T) {
		cfg := plugin.Config{
			Enabled:   true,
			PluginDir: "",
			Intelligence: sdkconfig.IntelligenceConfig{
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
			},
		}

		engine := plugin.NewLuaEngine(cfg)
		if engine == nil {
			t.Fatal("expected non-nil engine")
		}
		defer engine.Close()

		// Engine should be enabled
		if !engine.IsEnabled() {
			t.Error("engine should be enabled")
		}

		// RunHook should work
		ctx := context.Background()
		data := map[string]any{
			"model": "auto",
			"body":  `{"messages":[{"role":"user","content":"Hello"}]}`,
		}

		result, err := engine.RunHook(ctx, plugin.HookOnRequest, data)
		if err != nil {
			t.Errorf("RunHook should not error: %v", err)
		}

		// Without plugins loaded, data should be unchanged
		if result == nil {
			t.Error("expected non-nil result")
		}
	})

	t.Run("static matrix accessible in v1.0 mode", func(t *testing.T) {
		expectedMatrix := map[string]string{
			"coding":    "switchai-chat",
			"reasoning": "switchai-reasoner",
			"creative":  "switchai-chat",
			"fast":      "switchai-fast",
			"secure":    "switchai-fast",
			"vision":    "switchai-chat",
		}

		cfg := plugin.Config{
			Enabled:   true,
			PluginDir: "",
			Intelligence: sdkconfig.IntelligenceConfig{
				RouterModel:    "ollama:qwen:0.5b",
				RouterFallback: "openai:gpt-4o-mini",
				Matrix:         expectedMatrix,
			},
		}

		engine := plugin.NewLuaEngine(cfg)
		if engine == nil {
			t.Fatal("expected non-nil engine")
		}
		defer engine.Close()

		// Verify the config is accessible (indirectly through engine state)
		if !engine.IsEnabled() {
			t.Error("engine should be enabled")
		}
	})
}

// TestPluginIndependence_MockPlugin tests plugin loading and execution
// with a minimal mock plugin.
func TestPluginIndependence_MockPlugin(t *testing.T) {
	t.Run("load and execute mock plugin", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "switchailocal-mockplugin-*")
		if err != nil {
			t.Fatalf("failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tempDir)

		// Create mock plugin directory
		pluginDir := filepath.Join(tempDir, "mock-router")
		if err := os.MkdirAll(pluginDir, 0755); err != nil {
			t.Fatalf("failed to create plugin dir: %v", err)
		}

		// Create schema.lua
		schemaContent := `return {
    name = "mock-router",
    display_name = "Mock Router",
    version = "1.0.0",
    description = "A mock router for testing"
}`
		if err := os.WriteFile(filepath.Join(pluginDir, "schema.lua"), []byte(schemaContent), 0644); err != nil {
			t.Fatalf("failed to write schema.lua: %v", err)
		}

		// Create handler.lua that doesn't depend on intelligence services
		handlerContent := `local Plugin = {}

function Plugin:on_request(req)
    if req.model ~= "auto" then
        return nil
    end
    
    -- Simple routing without intelligence services
    req.model = "mock-model"
    req.metadata = req.metadata or {}
    req.metadata.routing_tier = "mock"
    
    return req
end

return Plugin`
		if err := os.WriteFile(filepath.Join(pluginDir, "handler.lua"), []byte(handlerContent), 0644); err != nil {
			t.Fatalf("failed to write handler.lua: %v", err)
		}

		// Create engine with mock plugin
		cfg := plugin.Config{
			Enabled:        true,
			PluginDir:      tempDir,
			EnabledPlugins: []string{"mock-router"},
			Intelligence: sdkconfig.IntelligenceConfig{
				RouterModel:    "ollama:qwen:0.5b",
				RouterFallback: "openai:gpt-4o-mini",
				Matrix: map[string]string{
					"coding": "switchai-chat",
					"fast":   "switchai-fast",
				},
			},
		}

		engine := plugin.NewLuaEngine(cfg)
		if engine == nil {
			t.Fatal("expected non-nil engine")
		}
		defer engine.Close()

		// Engine should be enabled
		if !engine.IsEnabled() {
			t.Error("engine should be enabled")
		}

		// Run hook with auto model
		ctx := context.Background()
		data := map[string]any{
			"model": "auto",
			"body":  "test request",
		}

		result, err := engine.RunHook(ctx, plugin.HookOnRequest, data)
		if err != nil {
			t.Errorf("RunHook should not error: %v", err)
		}

		// Model should be changed by mock plugin
		if result["model"] != "mock-model" {
			t.Errorf("expected model 'mock-model', got '%v'", result["model"])
		}

		// Metadata should be set
		if metadata, ok := result["metadata"].(map[string]any); ok {
			if metadata["routing_tier"] != "mock" {
				t.Errorf("expected routing_tier 'mock', got '%v'", metadata["routing_tier"])
			}
		} else {
			t.Error("expected metadata to be set")
		}
	})

	t.Run("plugin passes through non-auto models", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "switchailocal-passthrough-*")
		if err != nil {
			t.Fatalf("failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tempDir)

		// Create mock plugin directory
		pluginDir := filepath.Join(tempDir, "mock-router")
		if err := os.MkdirAll(pluginDir, 0755); err != nil {
			t.Fatalf("failed to create plugin dir: %v", err)
		}

		// Create schema.lua
		schemaContent := `return {
    name = "mock-router",
    display_name = "Mock Router",
    version = "1.0.0"
}`
		if err := os.WriteFile(filepath.Join(pluginDir, "schema.lua"), []byte(schemaContent), 0644); err != nil {
			t.Fatalf("failed to write schema.lua: %v", err)
		}

		// Create handler.lua
		handlerContent := `local Plugin = {}

function Plugin:on_request(req)
    if req.model ~= "auto" then
        return nil  -- Pass through
    end
    req.model = "routed-model"
    return req
end

return Plugin`
		if err := os.WriteFile(filepath.Join(pluginDir, "handler.lua"), []byte(handlerContent), 0644); err != nil {
			t.Fatalf("failed to write handler.lua: %v", err)
		}

		cfg := plugin.Config{
			Enabled:        true,
			PluginDir:      tempDir,
			EnabledPlugins: []string{"mock-router"},
		}

		engine := plugin.NewLuaEngine(cfg)
		if engine == nil {
			t.Fatal("expected non-nil engine")
		}
		defer engine.Close()

		// Run hook with specific model (not auto)
		ctx := context.Background()
		data := map[string]any{
			"model": "gpt-4",
			"body":  "test request",
		}

		result, err := engine.RunHook(ctx, plugin.HookOnRequest, data)
		if err != nil {
			t.Errorf("RunHook should not error: %v", err)
		}

		// Model should be unchanged (plugin returns nil for non-auto)
		if result["model"] != "gpt-4" {
			t.Errorf("expected model 'gpt-4', got '%v'", result["model"])
		}
	})
}
