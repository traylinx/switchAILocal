// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package discovery

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestCheckpoint_DiscoveryRunsOnStartup verifies that discovery runs when enabled
func TestCheckpoint_DiscoveryRunsOnStartup(t *testing.T) {
	tmpDir := t.TempDir()
	
	svc, err := NewService(tmpDir, nil)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}
	
	ctx := context.Background()
	
	// Run discovery (simulating startup)
	err = svc.DiscoverAll(ctx)
	if err != nil {
		t.Logf("Discovery completed with error (acceptable): %v", err)
	}
	
	// Verify models were discovered
	models := svc.GetAvailableModels()
	if models == nil {
		t.Fatal("GetAvailableModels returned nil")
	}
	
	t.Logf("Discovery completed: %d models discovered", len(models))
}

// TestCheckpoint_JSONFileCreated verifies that available_models.json is created
func TestCheckpoint_JSONFileCreated(t *testing.T) {
	tmpDir := t.TempDir()
	
	svc, err := NewService(tmpDir, nil)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}
	
	ctx := context.Background()
	
	// Run discovery
	err = svc.DiscoverAll(ctx)
	if err != nil {
		t.Logf("Discovery completed with error (acceptable): %v", err)
	}
	
	// Write registry
	err = svc.WriteRegistry("")
	if err != nil {
		t.Fatalf("Failed to write registry: %v", err)
	}
	
	// Verify JSON file exists
	registryPath := filepath.Join(tmpDir, "available_models.json")
	if _, err := os.Stat(registryPath); os.IsNotExist(err) {
		t.Fatal("available_models.json was not created")
	}
	
	// Verify JSON is valid
	data, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatalf("Failed to read registry file: %v", err)
	}
	
	var registry DiscoveryRegistry
	if err := json.Unmarshal(data, &registry); err != nil {
		t.Fatalf("Registry file contains invalid JSON: %v", err)
	}
	
	// Verify structure
	if registry.Providers == nil {
		t.Error("Registry missing providers field")
	}
	if registry.Models == nil {
		t.Error("Registry missing models field")
	}
	if registry.GeneratedAt.IsZero() {
		t.Error("Registry missing generated_at timestamp")
	}
	
	t.Logf("Registry file created successfully with %d models from %d providers", 
		registry.TotalModels, len(registry.Providers))
}

// TestCheckpoint_LuaAPIReturnsModelsOrError verifies Lua API behavior
func TestCheckpoint_LuaAPIReturnsModelsOrError(t *testing.T) {
	tmpDir := t.TempDir()
	
	svc, err := NewService(tmpDir, nil)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}
	
	ctx := context.Background()
	
	// Run discovery
	_ = svc.DiscoverAll(ctx)
	
	// Test GetAvailableModelsAsMap (used by Lua API)
	modelsMap := svc.GetAvailableModelsAsMap()
	if modelsMap == nil {
		t.Fatal("GetAvailableModelsAsMap returned nil")
	}
	
	// Verify each model has required fields for Lua
	for i, model := range modelsMap {
		if _, ok := model["id"]; !ok {
			t.Errorf("Model %d missing 'id' field", i)
		}
		if _, ok := model["provider"]; !ok {
			t.Errorf("Model %d missing 'provider' field", i)
		}
		if _, ok := model["is_available"]; !ok {
			t.Errorf("Model %d missing 'is_available' field", i)
		}
		if _, ok := model["discovered_at"]; !ok {
			t.Errorf("Model %d missing 'discovered_at' field", i)
		}
	}
	
	t.Logf("Lua API compatibility verified: %d models available", len(modelsMap))
}
