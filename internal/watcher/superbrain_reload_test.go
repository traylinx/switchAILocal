// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package watcher

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/traylinx/switchAILocal/internal/config"
)

// TestSuperbrainConfigHotReload tests that Superbrain config changes trigger the reload callback.
func TestSuperbrainConfigHotReload(t *testing.T) {
	// Create temporary directory for test
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	authDir := filepath.Join(tmpDir, "auth")

	// Create auth directory
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		t.Fatalf("failed to create auth dir: %v", err)
	}

	// Write initial config with Superbrain disabled
	initialConfig := `
port: 8080
auth-dir: ` + authDir + `
superbrain:
  enabled: false
  mode: disabled
  overwatch:
    silence_threshold_ms: 30000
    log_buffer_size: 50
    heartbeat_interval_ms: 1000
    max_restart_attempts: 2
  doctor:
    model: gemini-flash
    timeout_ms: 5000
  stdin_injection:
    mode: disabled
  context_sculptor:
    enabled: false
  fallback:
    enabled: false
  consensus:
    enabled: false
  security:
    audit_log_enabled: false
`
	if err := os.WriteFile(configPath, []byte(initialConfig), 0o644); err != nil {
		t.Fatalf("failed to write initial config: %v", err)
	}

	// Track config reloads
	var configReloads int32
	var superbrainReloads int32
	var lastSuperbrainConfig atomic.Value

	// Create watcher with callbacks
	w, err := NewWatcher(configPath, authDir, func(cfg *config.Config) {
		atomic.AddInt32(&configReloads, 1)
	})
	if err != nil {
		t.Fatalf("failed to create watcher: %v", err)
	}
	defer func() {
		if err := w.Stop(); err != nil {
			t.Logf("warning: failed to stop watcher: %v", err)
		}
	}()

	// Set Superbrain reload callback
	w.SetSuperbrainReloadCallback(func(cfg *config.SuperbrainConfig) {
		atomic.AddInt32(&superbrainReloads, 1)
		lastSuperbrainConfig.Store(cfg)
	})

	// Load initial config
	initialCfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load initial config: %v", err)
	}
	w.SetConfig(initialCfg)

	// Start watcher
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := w.Start(ctx); err != nil {
		t.Fatalf("failed to start watcher: %v", err)
	}

	// Wait for watcher to be ready
	time.Sleep(200 * time.Millisecond)

	// Update config to enable Superbrain
	updatedConfig := `
port: 8080
auth-dir: ` + authDir + `
superbrain:
  enabled: true
  mode: observe
  overwatch:
    silence_threshold_ms: 20000
    log_buffer_size: 100
    heartbeat_interval_ms: 500
    max_restart_attempts: 3
  doctor:
    model: gemini-pro
    timeout_ms: 10000
  stdin_injection:
    mode: conservative
  context_sculptor:
    enabled: true
  fallback:
    enabled: true
  consensus:
    enabled: false
  security:
    audit_log_enabled: true
`
	if err := os.WriteFile(configPath, []byte(updatedConfig), 0o644); err != nil {
		t.Fatalf("failed to write updated config: %v", err)
	}

	// Wait for config reload to be detected and processed
	time.Sleep(500 * time.Millisecond)

	// Verify config reload was triggered
	if atomic.LoadInt32(&configReloads) == 0 {
		t.Error("expected config reload callback to be called")
	}

	// Verify Superbrain reload was triggered
	if atomic.LoadInt32(&superbrainReloads) == 0 {
		t.Error("expected Superbrain reload callback to be called")
	}

	// Verify the new Superbrain config was passed to the callback
	newCfg := lastSuperbrainConfig.Load()
	if newCfg == nil {
		t.Fatal("expected Superbrain config to be stored")
	}

	superbrainCfg, ok := newCfg.(*config.SuperbrainConfig)
	if !ok {
		t.Fatal("expected config to be *config.SuperbrainConfig")
	}

	// Verify config values
	if !superbrainCfg.Enabled {
		t.Error("expected Superbrain to be enabled")
	}
	if superbrainCfg.Mode != "observe" {
		t.Errorf("expected mode to be 'observe', got %s", superbrainCfg.Mode)
	}
	if superbrainCfg.Overwatch.SilenceThresholdMs != 20000 {
		t.Errorf("expected silence threshold to be 20000, got %d", superbrainCfg.Overwatch.SilenceThresholdMs)
	}
	if superbrainCfg.Overwatch.LogBufferSize != 100 {
		t.Errorf("expected log buffer size to be 100, got %d", superbrainCfg.Overwatch.LogBufferSize)
	}
	if superbrainCfg.Doctor.Model != "gemini-pro" {
		t.Errorf("expected doctor model to be 'gemini-pro', got %s", superbrainCfg.Doctor.Model)
	}
	if superbrainCfg.StdinInjection.Mode != "conservative" {
		t.Errorf("expected stdin injection mode to be 'conservative', got %s", superbrainCfg.StdinInjection.Mode)
	}
	if !superbrainCfg.ContextSculptor.Enabled {
		t.Error("expected context sculptor to be enabled")
	}
	if !superbrainCfg.Fallback.Enabled {
		t.Error("expected fallback to be enabled")
	}
	if !superbrainCfg.Security.AuditLogEnabled {
		t.Error("expected audit log to be enabled")
	}
}

// TestSuperbrainModeTransition tests mode transitions without restart.
func TestSuperbrainModeTransition(t *testing.T) {
	// Create temporary directory for test
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	authDir := filepath.Join(tmpDir, "auth")

	// Create auth directory
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		t.Fatalf("failed to create auth dir: %v", err)
	}

	// Test mode transitions: disabled -> observe -> diagnose -> conservative -> autopilot
	modes := []string{"disabled", "observe", "diagnose", "conservative", "autopilot"}

	// Write initial config with first mode
	initialMode := modes[0]
	initialConfig := `
port: 8080
auth-dir: ` + authDir + `
superbrain:
  enabled: ` + (map[bool]string{true: "true", false: "false"}[initialMode != "disabled"]) + `
  mode: ` + initialMode + `
  overwatch:
    silence_threshold_ms: 30000
    log_buffer_size: 50
    heartbeat_interval_ms: 1000
    max_restart_attempts: 2
  doctor:
    model: gemini-flash
    timeout_ms: 5000
  stdin_injection:
    mode: disabled
  context_sculptor:
    enabled: false
  fallback:
    enabled: false
  consensus:
    enabled: false
  security:
    audit_log_enabled: false
`
	if err := os.WriteFile(configPath, []byte(initialConfig), 0o644); err != nil {
		t.Fatalf("failed to write initial config: %v", err)
	}

	// Track reloads
	var superbrainReloads int32
	var lastMode atomic.Value

	// Create watcher
	w, err := NewWatcher(configPath, authDir, func(cfg *config.Config) {})
	if err != nil {
		t.Fatalf("failed to create watcher: %v", err)
	}
	defer func() {
		if err := w.Stop(); err != nil {
			t.Logf("warning: failed to stop watcher: %v", err)
		}
	}()

	// Set Superbrain reload callback
	w.SetSuperbrainReloadCallback(func(cfg *config.SuperbrainConfig) {
		atomic.AddInt32(&superbrainReloads, 1)
		lastMode.Store(cfg.Mode)
	})

	// Load initial config
	initialCfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load initial config: %v", err)
	}
	w.SetConfig(initialCfg)

	// Start watcher
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := w.Start(ctx); err != nil {
		t.Fatalf("failed to start watcher: %v", err)
	}

	// Wait for watcher to be ready
	time.Sleep(200 * time.Millisecond)

	// Test each mode transition
	for i := 1; i < len(modes); i++ {
		previousMode := modes[i-1]
		nextMode := modes[i]

		t.Run("transition_"+previousMode+"_to_"+nextMode, func(t *testing.T) {
			// Reset counter for this transition
			atomic.StoreInt32(&superbrainReloads, 0)

			// Update to next mode
			nextConfig := `
port: 8080
auth-dir: ` + authDir + `
superbrain:
  enabled: ` + (map[bool]string{true: "true", false: "false"}[nextMode != "disabled"]) + `
  mode: ` + nextMode + `
  overwatch:
    silence_threshold_ms: 30000
    log_buffer_size: 50
    heartbeat_interval_ms: 1000
    max_restart_attempts: 2
  doctor:
    model: gemini-flash
    timeout_ms: 5000
  stdin_injection:
    mode: disabled
  context_sculptor:
    enabled: false
  fallback:
    enabled: false
  consensus:
    enabled: false
  security:
    audit_log_enabled: false
`
			if err := os.WriteFile(configPath, []byte(nextConfig), 0o644); err != nil {
				t.Fatalf("failed to write updated config: %v", err)
			}

			// Wait for reload
			time.Sleep(500 * time.Millisecond)

			// Verify reload was triggered
			if atomic.LoadInt32(&superbrainReloads) == 0 {
				t.Errorf("expected Superbrain reload callback to be called for transition %s -> %s", previousMode, nextMode)
			}

			// Verify mode was updated
			newMode := lastMode.Load()
			if newMode == nil {
				t.Fatal("expected mode to be stored")
			}

			if newMode.(string) != nextMode {
				t.Errorf("expected mode to be %s, got %s", nextMode, newMode.(string))
			}
		})
	}
}

// TestSuperbrainConfigNoChangeNoCallback tests that callback is not called when Superbrain config doesn't change.
func TestSuperbrainConfigNoChangeNoCallback(t *testing.T) {
	// Create temporary directory for test
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	authDir := filepath.Join(tmpDir, "auth")

	// Create auth directory
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		t.Fatalf("failed to create auth dir: %v", err)
	}

	// Write initial config
	initialConfig := `
port: 8080
auth-dir: ` + authDir + `
superbrain:
  enabled: true
  mode: observe
  overwatch:
    silence_threshold_ms: 30000
    log_buffer_size: 50
    heartbeat_interval_ms: 1000
    max_restart_attempts: 2
  doctor:
    model: gemini-flash
    timeout_ms: 5000
  stdin_injection:
    mode: disabled
  context_sculptor:
    enabled: false
  fallback:
    enabled: false
  consensus:
    enabled: false
  security:
    audit_log_enabled: false
`
	if err := os.WriteFile(configPath, []byte(initialConfig), 0o644); err != nil {
		t.Fatalf("failed to write initial config: %v", err)
	}

	// Track reloads
	var configReloads int32
	var superbrainReloads int32

	// Create watcher
	w, err := NewWatcher(configPath, authDir, func(cfg *config.Config) {
		atomic.AddInt32(&configReloads, 1)
	})
	if err != nil {
		t.Fatalf("failed to create watcher: %v", err)
	}
	defer func() {
		if err := w.Stop(); err != nil {
			t.Logf("warning: failed to stop watcher: %v", err)
		}
	}()

	// Set Superbrain reload callback
	w.SetSuperbrainReloadCallback(func(cfg *config.SuperbrainConfig) {
		atomic.AddInt32(&superbrainReloads, 1)
	})

	// Load initial config
	initialCfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load initial config: %v", err)
	}
	w.SetConfig(initialCfg)

	// Start watcher
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := w.Start(ctx); err != nil {
		t.Fatalf("failed to start watcher: %v", err)
	}

	// Wait for watcher to be ready
	time.Sleep(200 * time.Millisecond)

	// Update config but keep Superbrain config the same (only change port)
	updatedConfig := `
port: 9090
auth-dir: ` + authDir + `
superbrain:
  enabled: true
  mode: observe
  overwatch:
    silence_threshold_ms: 30000
    log_buffer_size: 50
    heartbeat_interval_ms: 1000
    max_restart_attempts: 2
  doctor:
    model: gemini-flash
    timeout_ms: 5000
  stdin_injection:
    mode: disabled
  context_sculptor:
    enabled: false
  fallback:
    enabled: false
  consensus:
    enabled: false
  security:
    audit_log_enabled: false
`
	if err := os.WriteFile(configPath, []byte(updatedConfig), 0o644); err != nil {
		t.Fatalf("failed to write updated config: %v", err)
	}

	// Wait for config reload
	time.Sleep(500 * time.Millisecond)

	// Verify config reload was triggered
	if atomic.LoadInt32(&configReloads) == 0 {
		t.Error("expected config reload callback to be called")
	}

	// Verify Superbrain reload was NOT triggered (config didn't change)
	if atomic.LoadInt32(&superbrainReloads) != 0 {
		t.Error("expected Superbrain reload callback NOT to be called when config unchanged")
	}
}
