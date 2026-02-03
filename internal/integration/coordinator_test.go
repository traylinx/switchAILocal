package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/traylinx/switchAILocal/internal/config"
	"github.com/traylinx/switchAILocal/internal/heartbeat"
	"github.com/traylinx/switchAILocal/internal/memory"
)

func TestNewServiceCoordinator(t *testing.T) {
	// Create temporary directory for test
	tmpDir := t.TempDir()

	cfg := &IntegrationConfig{
		Memory: &memory.MemoryConfig{
			Enabled:       false, // Disabled for basic test
			BaseDir:       filepath.Join(tmpDir, "memory"),
			RetentionDays: 90,
			MaxLogSizeMB:  100,
			Compression:   true,
		},
		Heartbeat: &heartbeat.HeartbeatConfig{
			Enabled:                false, // Disabled for basic test
			Interval:               5 * time.Minute,
			Timeout:                5 * time.Second,
			AutoDiscovery:          true,
			QuotaWarningThreshold:  0.80,
			QuotaCriticalThreshold: 0.95,
			MaxConcurrentChecks:    10,
			RetryAttempts:          2,
			RetryDelay:             time.Second,
		},
		Steering: &config.SteeringConfig{
			Enabled:   false,
			RulesDir:  filepath.Join(tmpDir, "steering"),
			HotReload: true,
		},
		Hooks: &config.HooksConfig{
			Enabled:   false,
			HooksDir:  filepath.Join(tmpDir, "hooks"),
			HotReload: true,
		},
	}

	coordinator, err := NewServiceCoordinator(cfg)
	if err != nil {
		t.Fatalf("Failed to create service coordinator: %v", err)
	}

	if coordinator == nil {
		t.Fatal("Expected non-nil coordinator")
	}

	// Verify systems are initialized
	if coordinator.GetEventBus() == nil {
		t.Error("Expected EventBus to be initialized")
	}

	if coordinator.GetSteering() == nil {
		t.Error("Expected Steering Engine to be initialized")
	}

	if coordinator.GetHooks() == nil {
		t.Error("Expected Hooks Manager to be initialized")
	}

	// Memory and Heartbeat should be nil when disabled
	if coordinator.GetMemory() != nil {
		t.Error("Expected Memory Manager to be nil when disabled")
	}

	if coordinator.GetHeartbeat() != nil {
		t.Error("Expected Heartbeat Monitor to be nil when disabled")
	}
}

func TestNewServiceCoordinator_WithNilConfig(t *testing.T) {
	_, err := NewServiceCoordinator(nil)
	if err == nil {
		t.Error("Expected error when creating coordinator with nil config")
	}
}

func TestServiceCoordinator_StartStop(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &IntegrationConfig{
		Memory: &memory.MemoryConfig{
			Enabled:       false,
			BaseDir:       filepath.Join(tmpDir, "memory"),
			RetentionDays: 90,
			MaxLogSizeMB:  100,
			Compression:   true,
		},
		Heartbeat: &heartbeat.HeartbeatConfig{
			Enabled:                false,
			Interval:               5 * time.Minute,
			Timeout:                5 * time.Second,
			AutoDiscovery:          true,
			QuotaWarningThreshold:  0.80,
			QuotaCriticalThreshold: 0.95,
			MaxConcurrentChecks:    10,
			RetryAttempts:          2,
			RetryDelay:             time.Second,
		},
		Steering: &config.SteeringConfig{
			Enabled:   false,
			RulesDir:  filepath.Join(tmpDir, "steering"),
			HotReload: false, // Disable hot-reload for test
		},
		Hooks: &config.HooksConfig{
			Enabled:   false,
			HooksDir:  filepath.Join(tmpDir, "hooks"),
			HotReload: false, // Disable hot-reload for test
		},
	}

	coordinator, err := NewServiceCoordinator(cfg)
	if err != nil {
		t.Fatalf("Failed to create service coordinator: %v", err)
	}

	// Test Start
	ctx := context.Background()
	if err := coordinator.Start(ctx); err != nil {
		t.Fatalf("Failed to start coordinator: %v", err)
	}

	if !coordinator.IsStarted() {
		t.Error("Expected coordinator to be started")
	}

	// Test double start (should return error)
	if err := coordinator.Start(ctx); err == nil {
		t.Error("Expected error when starting already started coordinator")
	}

	// Test Stop
	if err := coordinator.Stop(ctx); err != nil {
		t.Fatalf("Failed to stop coordinator: %v", err)
	}

	if coordinator.IsStarted() {
		t.Error("Expected coordinator to be stopped")
	}

	// Test double stop (should be idempotent)
	if err := coordinator.Stop(ctx); err != nil {
		t.Error("Expected no error when stopping already stopped coordinator")
	}
}

func TestServiceCoordinator_WithMemoryEnabled(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &IntegrationConfig{
		Memory: &memory.MemoryConfig{
			Enabled:       true, // Enable memory system
			BaseDir:       filepath.Join(tmpDir, "memory"),
			RetentionDays: 90,
			MaxLogSizeMB:  100,
			Compression:   true,
		},
		Heartbeat: &heartbeat.HeartbeatConfig{
			Enabled: false,
		},
		Steering: &config.SteeringConfig{
			Enabled:   false,
			RulesDir:  filepath.Join(tmpDir, "steering"),
			HotReload: false,
		},
		Hooks: &config.HooksConfig{
			Enabled:   false,
			HooksDir:  filepath.Join(tmpDir, "hooks"),
			HotReload: false,
		},
	}

	coordinator, err := NewServiceCoordinator(cfg)
	if err != nil {
		t.Fatalf("Failed to create service coordinator: %v", err)
	}

	// Memory should be initialized when enabled
	if coordinator.GetMemory() == nil {
		t.Error("Expected Memory Manager to be initialized when enabled")
	}

	// Start and stop
	ctx := context.Background()
	if err := coordinator.Start(ctx); err != nil {
		t.Fatalf("Failed to start coordinator: %v", err)
	}

	if err := coordinator.Stop(ctx); err != nil {
		t.Fatalf("Failed to stop coordinator: %v", err)
	}
}

func TestServiceCoordinator_WithHeartbeatEnabled(t *testing.T) {
	t.Skip("Skipping heartbeat test - requires provider checkers to be registered")
	
	tmpDir := t.TempDir()

	cfg := &IntegrationConfig{
		Memory: &memory.MemoryConfig{
			Enabled: false,
		},
		Heartbeat: &heartbeat.HeartbeatConfig{
			Enabled:                true, // Enable heartbeat
			Interval:               100 * time.Millisecond,
			Timeout:                50 * time.Millisecond,
			AutoDiscovery:          false,
			QuotaWarningThreshold:  0.80,
			QuotaCriticalThreshold: 0.95,
			MaxConcurrentChecks:    5,
			RetryAttempts:          1,
			RetryDelay:             10 * time.Millisecond,
		},
		Steering: &config.SteeringConfig{
			Enabled:   false,
			RulesDir:  filepath.Join(tmpDir, "steering"),
			HotReload: false,
		},
		Hooks: &config.HooksConfig{
			Enabled:   false,
			HooksDir:  filepath.Join(tmpDir, "hooks"),
			HotReload: false,
		},
	}

	coordinator, err := NewServiceCoordinator(cfg)
	if err != nil {
		t.Fatalf("Failed to create service coordinator: %v", err)
	}

	// Heartbeat should be initialized when enabled
	if coordinator.GetHeartbeat() == nil {
		t.Error("Expected Heartbeat Monitor to be initialized when enabled")
	}

	// Start and stop
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	if err := coordinator.Start(ctx); err != nil {
		t.Fatalf("Failed to start coordinator: %v", err)
	}

	// Give heartbeat a moment to start
	time.Sleep(200 * time.Millisecond)

	if err := coordinator.Stop(ctx); err != nil {
		t.Fatalf("Failed to stop coordinator: %v", err)
	}
}

func TestServiceCoordinator_DefaultConfiguration(t *testing.T) {
	// Test with empty config - should apply defaults
	cfg := &IntegrationConfig{}

	coordinator, err := NewServiceCoordinator(cfg)
	if err != nil {
		t.Fatalf("Failed to create service coordinator with empty config: %v", err)
	}

	if coordinator == nil {
		t.Fatal("Expected non-nil coordinator")
	}

	// Verify defaults were applied
	if coordinator.config.Memory == nil {
		t.Error("Expected Memory config to have defaults applied")
	}

	if coordinator.config.Heartbeat == nil {
		t.Error("Expected Heartbeat config to have defaults applied")
	}

	if coordinator.config.Steering == nil {
		t.Error("Expected Steering config to have defaults applied")
	}

	if coordinator.config.Hooks == nil {
		t.Error("Expected Hooks config to have defaults applied")
	}
}

func TestServiceCoordinator_SteeringRulesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	steeringDir := filepath.Join(tmpDir, "custom-steering")

	// Create the steering directory
	if err := os.MkdirAll(steeringDir, 0755); err != nil {
		t.Fatalf("Failed to create steering directory: %v", err)
	}

	cfg := &IntegrationConfig{
		Memory: &memory.MemoryConfig{
			Enabled: false,
		},
		Heartbeat: &heartbeat.HeartbeatConfig{
			Enabled: false,
		},
		Steering: &config.SteeringConfig{
			Enabled:   true,
			RulesDir:  steeringDir,
			HotReload: false,
		},
		Hooks: &config.HooksConfig{
			Enabled:   false,
			HooksDir:  filepath.Join(tmpDir, "hooks"),
			HotReload: false,
		},
	}

	coordinator, err := NewServiceCoordinator(cfg)
	if err != nil {
		t.Fatalf("Failed to create service coordinator: %v", err)
	}

	// Verify steering engine is initialized
	if coordinator.GetSteering() == nil {
		t.Error("Expected Steering Engine to be initialized")
	}

	// Start and stop
	ctx := context.Background()
	if err := coordinator.Start(ctx); err != nil {
		t.Fatalf("Failed to start coordinator: %v", err)
	}

	if err := coordinator.Stop(ctx); err != nil {
		t.Fatalf("Failed to stop coordinator: %v", err)
	}
}

func TestServiceCoordinator_HooksDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	hooksDir := filepath.Join(tmpDir, "custom-hooks")

	// Create the hooks directory
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		t.Fatalf("Failed to create hooks directory: %v", err)
	}

	cfg := &IntegrationConfig{
		Memory: &memory.MemoryConfig{
			Enabled: false,
		},
		Heartbeat: &heartbeat.HeartbeatConfig{
			Enabled: false,
		},
		Steering: &config.SteeringConfig{
			Enabled:   false,
			RulesDir:  filepath.Join(tmpDir, "steering"),
			HotReload: false,
		},
		Hooks: &config.HooksConfig{
			Enabled:   true,
			HooksDir:  hooksDir,
			HotReload: false,
		},
	}

	coordinator, err := NewServiceCoordinator(cfg)
	if err != nil {
		t.Fatalf("Failed to create service coordinator: %v", err)
	}

	// Verify hooks manager is initialized
	if coordinator.GetHooks() == nil {
		t.Error("Expected Hooks Manager to be initialized")
	}

	// Start and stop
	ctx := context.Background()
	if err := coordinator.Start(ctx); err != nil {
		t.Fatalf("Failed to start coordinator: %v", err)
	}

	if err := coordinator.Stop(ctx); err != nil {
		t.Fatalf("Failed to stop coordinator: %v", err)
	}
}
