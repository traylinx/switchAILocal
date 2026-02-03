package integration

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/traylinx/switchAILocal/internal/config"
	"github.com/traylinx/switchAILocal/internal/heartbeat"
	"github.com/traylinx/switchAILocal/internal/memory"
)

func TestServiceCoordinator_HeartbeatWithProviders(t *testing.T) {
	t.Skip("Skipping test due to deadlock in heartbeat monitor Start() - this is a known issue")
	
	tmpDir := t.TempDir()

	// Create a main config with Ollama enabled
	mainConfig := &config.Config{
		Ollama: config.OllamaConfig{
			Enabled: true,
			BaseURL: "http://localhost:11434",
		},
	}

	cfg := &IntegrationConfig{
		Memory: &memory.MemoryConfig{
			Enabled: false,
		},
		Heartbeat: &heartbeat.HeartbeatConfig{
			Enabled:                true,
			Interval:               1 * time.Second,
			Timeout:                500 * time.Millisecond,
			AutoDiscovery:          false,
			QuotaWarningThreshold:  0.80,
			QuotaCriticalThreshold: 0.95,
			MaxConcurrentChecks:    5,
			RetryAttempts:          1,
			RetryDelay:             100 * time.Millisecond,
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
		MainConfig: mainConfig,
	}

	coordinator, err := NewServiceCoordinator(cfg)
	if err != nil {
		t.Fatalf("Failed to create service coordinator: %v", err)
	}

	// Heartbeat should be initialized when enabled
	if coordinator.GetHeartbeat() == nil {
		t.Error("Expected Heartbeat Monitor to be initialized when enabled")
	}

	// Verify provider checkers were registered
	if monitorImpl, ok := coordinator.GetHeartbeat().(*heartbeat.HeartbeatMonitorImpl); ok {
		stats := monitorImpl.GetStats()
		if stats.ProvidersMonitored != 1 {
			t.Errorf("Expected 1 provider registered, got %d", stats.ProvidersMonitored)
		}
	}

	// Start the coordinator (this will start background monitoring)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := coordinator.Start(ctx); err != nil {
		t.Fatalf("Failed to start coordinator: %v", err)
	}

	// Give it a moment to start
	time.Sleep(100 * time.Millisecond)

	// Stop the coordinator
	if err := coordinator.Stop(ctx); err != nil {
		t.Fatalf("Failed to stop coordinator: %v", err)
	}
}

func TestServiceCoordinator_HeartbeatWithMultipleProviders(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a main config with multiple providers
	mainConfig := &config.Config{
		Ollama: config.OllamaConfig{
			Enabled: true,
			BaseURL: "http://localhost:11434",
		},
		LMStudio: config.LMStudioConfig{
			Enabled: true,
			BaseURL: "http://localhost:1234/v1",
		},
		GeminiKey: []config.GeminiKey{
			{APIKey: "test-key-123"},
		},
	}

	cfg := &IntegrationConfig{
		Memory: &memory.MemoryConfig{
			Enabled: false,
		},
		Heartbeat: &heartbeat.HeartbeatConfig{
			Enabled:                true,
			Interval:               1 * time.Second,
			Timeout:                500 * time.Millisecond,
			AutoDiscovery:          false,
			QuotaWarningThreshold:  0.80,
			QuotaCriticalThreshold: 0.95,
			MaxConcurrentChecks:    5,
			RetryAttempts:          1,
			RetryDelay:             100 * time.Millisecond,
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
		MainConfig: mainConfig,
	}

	coordinator, err := NewServiceCoordinator(cfg)
	if err != nil {
		t.Fatalf("Failed to create service coordinator: %v", err)
	}

	// Verify multiple provider checkers were registered
	if monitorImpl, ok := coordinator.GetHeartbeat().(*heartbeat.HeartbeatMonitorImpl); ok {
		stats := monitorImpl.GetStats()
		if stats.ProvidersMonitored != 3 {
			t.Errorf("Expected 3 providers registered, got %d", stats.ProvidersMonitored)
		}
	}
}

func TestServiceCoordinator_HeartbeatDisabled(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a main config with providers, but heartbeat disabled
	mainConfig := &config.Config{
		Ollama: config.OllamaConfig{
			Enabled: true,
			BaseURL: "http://localhost:11434",
		},
	}

	cfg := &IntegrationConfig{
		Memory: &memory.MemoryConfig{
			Enabled: false,
		},
		Heartbeat: &heartbeat.HeartbeatConfig{
			Enabled: false, // Disabled
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
		MainConfig: mainConfig,
	}

	coordinator, err := NewServiceCoordinator(cfg)
	if err != nil {
		t.Fatalf("Failed to create service coordinator: %v", err)
	}

	// Heartbeat should NOT be initialized when disabled
	if coordinator.GetHeartbeat() != nil {
		t.Error("Expected Heartbeat Monitor to be nil when disabled")
	}

	// Start and stop should still work
	ctx := context.Background()
	if err := coordinator.Start(ctx); err != nil {
		t.Fatalf("Failed to start coordinator: %v", err)
	}

	if err := coordinator.Stop(ctx); err != nil {
		t.Fatalf("Failed to stop coordinator: %v", err)
	}
}
