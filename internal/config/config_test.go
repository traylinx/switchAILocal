// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package config

import (
	"os"
	"testing"
)

func TestLoadConfig_SecureDefaults(t *testing.T) {
	// Create a temporary empty config file
	f, err := os.CreateTemp("", "config_test_*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(f.Name())
	f.Close()

	// Load the config (should apply defaults)
	cfg, err := LoadConfig(f.Name())
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify Secure Defaults
	if !cfg.WebsocketAuth {
		t.Error("Security Regression: WebsocketAuth should be true by default")
	}

	if cfg.Host != "" {
		t.Errorf("Host should be empty by default (bind all), got: %s", cfg.Host)
	}
}

func TestLoadConfig_ExplicitDisable(t *testing.T) {
	// Create a config file that explicitly disables auth
	content := []byte("ws-auth: false")
	f, err := os.CreateTemp("", "config_insecure_*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(content); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	f.Close()

	cfg, err := LoadConfig(f.Name())
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if cfg.WebsocketAuth {
		t.Error("Config Loader failed to respect explicit disable of WebsocketAuth")
	}
}

func TestLoadConfig_SuperbrainDefaults(t *testing.T) {
	// Create a temporary empty config file
	f, err := os.CreateTemp("", "config_superbrain_test_*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(f.Name())
	f.Close()

	// Load the config (should apply Superbrain defaults)
	cfg, err := LoadConfig(f.Name())
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify Superbrain defaults
	if cfg.Superbrain.Enabled {
		t.Error("Superbrain should be disabled by default")
	}

	if cfg.Superbrain.Mode != "disabled" {
		t.Errorf("Superbrain mode should be 'disabled' by default, got: %s", cfg.Superbrain.Mode)
	}

	// Verify Overwatch defaults
	if cfg.Superbrain.Overwatch.SilenceThresholdMs != 30000 {
		t.Errorf("Overwatch silence threshold should be 30000ms, got: %d", cfg.Superbrain.Overwatch.SilenceThresholdMs)
	}

	if cfg.Superbrain.Overwatch.LogBufferSize != 50 {
		t.Errorf("Overwatch log buffer size should be 50, got: %d", cfg.Superbrain.Overwatch.LogBufferSize)
	}

	if cfg.Superbrain.Overwatch.HeartbeatIntervalMs != 1000 {
		t.Errorf("Overwatch heartbeat interval should be 1000ms, got: %d", cfg.Superbrain.Overwatch.HeartbeatIntervalMs)
	}

	if cfg.Superbrain.Overwatch.MaxRestartAttempts != 2 {
		t.Errorf("Overwatch max restart attempts should be 2, got: %d", cfg.Superbrain.Overwatch.MaxRestartAttempts)
	}

	// Verify Doctor defaults
	if cfg.Superbrain.Doctor.Model != "gemini-flash" {
		t.Errorf("Doctor model should be 'gemini-flash', got: %s", cfg.Superbrain.Doctor.Model)
	}

	if cfg.Superbrain.Doctor.TimeoutMs != 5000 {
		t.Errorf("Doctor timeout should be 5000ms, got: %d", cfg.Superbrain.Doctor.TimeoutMs)
	}

	// Verify StdinInjection defaults
	if cfg.Superbrain.StdinInjection.Mode != "conservative" {
		t.Errorf("StdinInjection mode should be 'conservative', got: %s", cfg.Superbrain.StdinInjection.Mode)
	}

	// Verify ContextSculptor defaults
	if !cfg.Superbrain.ContextSculptor.Enabled {
		t.Error("ContextSculptor should be enabled by default")
	}

	if cfg.Superbrain.ContextSculptor.TokenEstimator != "tiktoken" {
		t.Errorf("ContextSculptor token estimator should be 'tiktoken', got: %s", cfg.Superbrain.ContextSculptor.TokenEstimator)
	}

	expectedPriorityFiles := []string{"README.md", "main.go", "index.ts", "package.json"}
	if len(cfg.Superbrain.ContextSculptor.PriorityFiles) != len(expectedPriorityFiles) {
		t.Errorf("ContextSculptor should have %d priority files, got: %d", len(expectedPriorityFiles), len(cfg.Superbrain.ContextSculptor.PriorityFiles))
	}

	// Verify Fallback defaults
	if !cfg.Superbrain.Fallback.Enabled {
		t.Error("Fallback should be enabled by default")
	}

	if cfg.Superbrain.Fallback.MinSuccessRate != 0.5 {
		t.Errorf("Fallback min success rate should be 0.5, got: %f", cfg.Superbrain.Fallback.MinSuccessRate)
	}

	expectedProviders := []string{"geminicli", "gemini", "ollama"}
	if len(cfg.Superbrain.Fallback.Providers) != len(expectedProviders) {
		t.Errorf("Fallback should have %d providers, got: %d", len(expectedProviders), len(cfg.Superbrain.Fallback.Providers))
	}

	// Verify Consensus defaults
	if cfg.Superbrain.Consensus.Enabled {
		t.Error("Consensus should be disabled by default")
	}

	if cfg.Superbrain.Consensus.VerificationModel != "gemini-flash" {
		t.Errorf("Consensus verification model should be 'gemini-flash', got: %s", cfg.Superbrain.Consensus.VerificationModel)
	}

	// Verify Security defaults
	if !cfg.Superbrain.Security.AuditLogEnabled {
		t.Error("Security audit log should be enabled by default")
	}

	if cfg.Superbrain.Security.AuditLogPath != "./logs/superbrain_audit.log" {
		t.Errorf("Security audit log path should be './logs/superbrain_audit.log', got: %s", cfg.Superbrain.Security.AuditLogPath)
	}
}

func TestLoadConfig_SuperbrainCustomValues(t *testing.T) {
	// Create a config file with custom Superbrain settings
	content := []byte(`
superbrain:
  enabled: true
  mode: autopilot
  overwatch:
    silence_threshold_ms: 60000
    log_buffer_size: 100
    heartbeat_interval_ms: 2000
    max_restart_attempts: 3
  doctor:
    model: claude-haiku
    timeout_ms: 10000
  stdin_injection:
    mode: autopilot
    forbidden_patterns:
      - "delete"
      - "rm -rf"
  context_sculptor:
    enabled: false
    token_estimator: simple
    priority_files:
      - "custom.md"
  fallback:
    enabled: false
    providers:
      - "claude"
      - "openai"
    min_success_rate: 0.8
  consensus:
    enabled: true
    verification_model: claude-sonnet
    trigger_patterns:
      - "incomplete"
  security:
    audit_log_enabled: false
    audit_log_path: "/var/log/superbrain.log"
    forbidden_operations:
      - "sudo"
`)

	f, err := os.CreateTemp("", "config_superbrain_custom_*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(content); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	f.Close()

	cfg, err := LoadConfig(f.Name())
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify custom values are loaded
	if !cfg.Superbrain.Enabled {
		t.Error("Superbrain should be enabled")
	}

	if cfg.Superbrain.Mode != "autopilot" {
		t.Errorf("Superbrain mode should be 'autopilot', got: %s", cfg.Superbrain.Mode)
	}

	if cfg.Superbrain.Overwatch.SilenceThresholdMs != 60000 {
		t.Errorf("Overwatch silence threshold should be 60000ms, got: %d", cfg.Superbrain.Overwatch.SilenceThresholdMs)
	}

	if cfg.Superbrain.Doctor.Model != "claude-haiku" {
		t.Errorf("Doctor model should be 'claude-haiku', got: %s", cfg.Superbrain.Doctor.Model)
	}

	if cfg.Superbrain.StdinInjection.Mode != "autopilot" {
		t.Errorf("StdinInjection mode should be 'autopilot', got: %s", cfg.Superbrain.StdinInjection.Mode)
	}

	if len(cfg.Superbrain.StdinInjection.ForbiddenPatterns) != 2 {
		t.Errorf("Should have 2 forbidden patterns, got: %d", len(cfg.Superbrain.StdinInjection.ForbiddenPatterns))
	}

	if cfg.Superbrain.ContextSculptor.Enabled {
		t.Error("ContextSculptor should be disabled")
	}

	if cfg.Superbrain.ContextSculptor.TokenEstimator != "simple" {
		t.Errorf("ContextSculptor token estimator should be 'simple', got: %s", cfg.Superbrain.ContextSculptor.TokenEstimator)
	}

	if cfg.Superbrain.Fallback.Enabled {
		t.Error("Fallback should be disabled")
	}

	if cfg.Superbrain.Fallback.MinSuccessRate != 0.8 {
		t.Errorf("Fallback min success rate should be 0.8, got: %f", cfg.Superbrain.Fallback.MinSuccessRate)
	}

	if cfg.Superbrain.Consensus.Enabled != true {
		t.Error("Consensus should be enabled")
	}

	if cfg.Superbrain.Security.AuditLogEnabled {
		t.Error("Security audit log should be disabled")
	}
}

func TestSanitizeSuperbrain_InvalidMode(t *testing.T) {
	// Create a config with invalid mode
	content := []byte(`
superbrain:
  enabled: true
  mode: invalid-mode
`)

	f, err := os.CreateTemp("", "config_superbrain_invalid_*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(content); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	f.Close()

	cfg, err := LoadConfig(f.Name())
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Invalid mode should be sanitized to "disabled"
	if cfg.Superbrain.Mode != "disabled" {
		t.Errorf("Invalid mode should be sanitized to 'disabled', got: %s", cfg.Superbrain.Mode)
	}

	// Enabled should be set to false when mode is disabled
	if cfg.Superbrain.Enabled {
		t.Error("Enabled should be false when mode is 'disabled'")
	}
}

func TestSanitizeSuperbrain_BoundaryValues(t *testing.T) {
	// Create a config with boundary-violating values
	content := []byte(`
superbrain:
  enabled: true
  mode: autopilot
  overwatch:
    silence_threshold_ms: 500
    log_buffer_size: 5
    heartbeat_interval_ms: 50
    max_restart_attempts: 10
  doctor:
    timeout_ms: 500
  fallback:
    min_success_rate: 1.5
`)

	f, err := os.CreateTemp("", "config_superbrain_boundary_*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(content); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	f.Close()

	cfg, err := LoadConfig(f.Name())
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify boundary constraints are enforced
	if cfg.Superbrain.Overwatch.SilenceThresholdMs < 1000 {
		t.Errorf("Silence threshold should be at least 1000ms, got: %d", cfg.Superbrain.Overwatch.SilenceThresholdMs)
	}

	if cfg.Superbrain.Overwatch.LogBufferSize < 10 {
		t.Errorf("Log buffer size should be at least 10, got: %d", cfg.Superbrain.Overwatch.LogBufferSize)
	}

	if cfg.Superbrain.Overwatch.HeartbeatIntervalMs < 100 {
		t.Errorf("Heartbeat interval should be at least 100ms, got: %d", cfg.Superbrain.Overwatch.HeartbeatIntervalMs)
	}

	if cfg.Superbrain.Overwatch.MaxRestartAttempts > 5 {
		t.Errorf("Max restart attempts should be at most 5, got: %d", cfg.Superbrain.Overwatch.MaxRestartAttempts)
	}

	if cfg.Superbrain.Doctor.TimeoutMs < 1000 {
		t.Errorf("Doctor timeout should be at least 1000ms, got: %d", cfg.Superbrain.Doctor.TimeoutMs)
	}

	if cfg.Superbrain.Fallback.MinSuccessRate > 1.0 {
		t.Errorf("Min success rate should be at most 1.0, got: %f", cfg.Superbrain.Fallback.MinSuccessRate)
	}
}
