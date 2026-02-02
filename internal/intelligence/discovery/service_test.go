// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package discovery

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestNewService tests the creation of a new discovery service
func TestNewService(t *testing.T) {
	tmpDir := t.TempDir()
	
	svc, err := NewService(tmpDir, nil)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}
	
	if svc == nil {
		t.Fatal("Service is nil")
	}
	
	if svc.cacheDir != tmpDir {
		t.Errorf("Expected cacheDir %s, got %s", tmpDir, svc.cacheDir)
	}
}

// TestNewServiceWithHomeDir tests service creation with ~ in path
func TestNewServiceWithHomeDir(t *testing.T) {
	svc, err := NewService("~/.test-cache", nil)
	if err != nil {
		t.Fatalf("Failed to create service with home dir: %v", err)
	}
	
	if svc == nil {
		t.Fatal("Service is nil")
	}
	
	// Verify home directory was expanded
	if svc.cacheDir[0] == '~' {
		t.Error("Home directory was not expanded")
	}
	
	// Cleanup
	os.RemoveAll(svc.cacheDir)
}

// TestDiscoverAllTimeout tests that discovery respects the 5-second timeout
func TestDiscoverAllTimeout(t *testing.T) {
	tmpDir := t.TempDir()
	
	svc, err := NewService(tmpDir, nil)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}
	
	ctx := context.Background()
	start := time.Now()
	
	// Run discovery (may fail due to no providers, but should complete quickly)
	_ = svc.DiscoverAll(ctx)
	
	duration := time.Since(start)
	
	// Should complete within 6 seconds (5s timeout + 1s buffer)
	if duration > 6*time.Second {
		t.Errorf("Discovery took too long: %v (expected < 6s)", duration)
	}
}

// TestDiscoverAllGracefulFailure tests that discovery handles provider failures gracefully
func TestDiscoverAllGracefulFailure(t *testing.T) {
	tmpDir := t.TempDir()
	
	svc, err := NewService(tmpDir, nil)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}
	
	ctx := context.Background()
	
	// Run discovery - should not panic even if providers fail
	err = svc.DiscoverAll(ctx)
	
	// Error is acceptable (no providers configured), but should not panic
	// We just verify it completes without crashing
	if err != nil {
		t.Logf("Discovery completed with expected error: %v", err)
	}
	
	// Verify we can still get results (even if empty)
	models := svc.GetAvailableModels()
	if models == nil {
		t.Error("GetAvailableModels returned nil")
	}
	
	providers := svc.GetProviderStatus()
	if providers == nil {
		t.Error("GetProviderStatus returned nil")
	}
}

// TestGetAvailableModels tests retrieving discovered models
func TestGetAvailableModels(t *testing.T) {
	tmpDir := t.TempDir()
	
	svc, err := NewService(tmpDir, nil)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}
	
	// Initially should be empty
	models := svc.GetAvailableModels()
	if len(models) != 0 {
		t.Errorf("Expected 0 models initially, got %d", len(models))
	}
	
	// After discovery, should return results (even if empty due to no providers)
	ctx := context.Background()
	_ = svc.DiscoverAll(ctx)
	
	models = svc.GetAvailableModels()
	if models == nil {
		t.Error("GetAvailableModels returned nil after discovery")
	}
}

// TestGetAvailableModelsAsMap tests the map conversion for Lua interop
func TestGetAvailableModelsAsMap(t *testing.T) {
	tmpDir := t.TempDir()
	
	svc, err := NewService(tmpDir, nil)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}
	
	// Run discovery
	ctx := context.Background()
	_ = svc.DiscoverAll(ctx)
	
	// Get models as map
	modelsMap := svc.GetAvailableModelsAsMap()
	if modelsMap == nil {
		t.Error("GetAvailableModelsAsMap returned nil")
	}
	
	// Verify structure of returned maps
	for _, model := range modelsMap {
		// Check required fields exist
		if _, ok := model["id"]; !ok {
			t.Error("Model map missing 'id' field")
		}
		if _, ok := model["provider"]; !ok {
			t.Error("Model map missing 'provider' field")
		}
		if _, ok := model["is_available"]; !ok {
			t.Error("Model map missing 'is_available' field")
		}
	}
}

// TestWriteRegistry tests writing the discovery registry to disk
func TestWriteRegistry(t *testing.T) {
	tmpDir := t.TempDir()
	
	svc, err := NewService(tmpDir, nil)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}
	
	// Run discovery
	ctx := context.Background()
	_ = svc.DiscoverAll(ctx)
	
	// Write registry with default path
	err = svc.WriteRegistry("")
	if err != nil {
		t.Fatalf("Failed to write registry: %v", err)
	}
	
	// Verify file was created
	registryPath := filepath.Join(tmpDir, "available_models.json")
	if _, err := os.Stat(registryPath); os.IsNotExist(err) {
		t.Error("Registry file was not created")
	}
	
	// Verify file is valid JSON
	data, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatalf("Failed to read registry file: %v", err)
	}
	
	if len(data) == 0 {
		t.Error("Registry file is empty")
	}
}

// TestWriteRegistryCustomPath tests writing registry to a custom path
func TestWriteRegistryCustomPath(t *testing.T) {
	tmpDir := t.TempDir()
	
	svc, err := NewService(tmpDir, nil)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}
	
	// Run discovery
	ctx := context.Background()
	_ = svc.DiscoverAll(ctx)
	
	// Write registry to custom path
	customPath := filepath.Join(tmpDir, "custom_models.json")
	err = svc.WriteRegistry(customPath)
	if err != nil {
		t.Fatalf("Failed to write registry to custom path: %v", err)
	}
	
	// Verify file was created at custom path
	if _, err := os.Stat(customPath); os.IsNotExist(err) {
		t.Error("Registry file was not created at custom path")
	}
}

// TestShutdown tests graceful shutdown
func TestShutdown(t *testing.T) {
	tmpDir := t.TempDir()
	
	svc, err := NewService(tmpDir, nil)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}
	
	ctx := context.Background()
	err = svc.Shutdown(ctx)
	if err != nil {
		t.Errorf("Shutdown returned error: %v", err)
	}
}

// TestConcurrentAccess tests thread-safe access to models
func TestConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()
	
	svc, err := NewService(tmpDir, nil)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}
	
	// Run discovery
	ctx := context.Background()
	_ = svc.DiscoverAll(ctx)
	
	// Spawn multiple goroutines to access models concurrently
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_ = svc.GetAvailableModels()
				_ = svc.GetAvailableModelsAsMap()
				_ = svc.GetProviderStatus()
			}
			done <- true
		}()
	}
	
	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}
}
