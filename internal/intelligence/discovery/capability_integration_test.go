// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package discovery

import (
	"context"
	"encoding/json"
	"os"
	"testing"
)

// TestCapabilityIntegration tests that capabilities are correctly
// integrated into the discovery service and written to JSON.
func TestCapabilityIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Create discovery service
	svc, err := NewService(tmpDir, nil)
	if err != nil {
		t.Fatalf("Failed to create discovery service: %v", err)
	}
	
	// Run discovery
	ctx := context.Background()
	err = svc.DiscoverAll(ctx)
	if err != nil {
		t.Logf("Discovery completed with error (expected if no providers): %v", err)
	}
	
	// Get models
	models := svc.GetAvailableModels()
	
	// Verify capabilities are populated
	for _, model := range models {
		if model.Capabilities == nil {
			t.Errorf("Model %s has nil capabilities", model.ID)
			continue
		}
		
		// Verify capability fields exist (values depend on model name)
		t.Logf("Model %s capabilities: coding=%v, reasoning=%v, vision=%v, local=%v",
			model.ID,
			model.Capabilities.SupportsCoding,
			model.Capabilities.SupportsReasoning,
			model.Capabilities.SupportsVision,
			model.Capabilities.IsLocal)
	}
	
	// Write registry
	registryPath := tmpDir + "/test_registry.json"
	err = svc.WriteRegistry(registryPath)
	if err != nil {
		t.Fatalf("Failed to write registry: %v", err)
	}
	
	// Read and verify JSON structure
	data, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatalf("Failed to read registry file: %v", err)
	}
	
	var registry struct {
		Models []struct {
			ID           string `json:"id"`
			Capabilities *struct {
				SupportsCoding    bool   `json:"supports_coding"`
				SupportsReasoning bool   `json:"supports_reasoning"`
				SupportsVision    bool   `json:"supports_vision"`
				ContextWindow     int    `json:"context_window"`
				EstimatedLatency  string `json:"estimated_latency"`
				CostTier          string `json:"cost_tier"`
				IsLocal           bool   `json:"is_local"`
			} `json:"capabilities"`
		} `json:"models"`
	}
	
	err = json.Unmarshal(data, &registry)
	if err != nil {
		t.Fatalf("Failed to unmarshal registry JSON: %v", err)
	}
	
	// Verify capabilities are in JSON
	for _, model := range registry.Models {
		if model.Capabilities == nil {
			t.Errorf("Model %s has nil capabilities in JSON", model.ID)
			continue
		}
		
		// Verify all capability fields are present
		if model.Capabilities.ContextWindow == 0 {
			t.Errorf("Model %s has zero context window", model.ID)
		}
		if model.Capabilities.EstimatedLatency == "" {
			t.Errorf("Model %s has empty estimated latency", model.ID)
		}
		if model.Capabilities.CostTier == "" {
			t.Errorf("Model %s has empty cost tier", model.ID)
		}
		
		t.Logf("✓ Model %s has complete capability data in JSON", model.ID)
	}
	
	t.Logf("✓ Verified %d models have capability data in JSON", len(registry.Models))
}
