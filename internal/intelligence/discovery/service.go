// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package discovery provides model discovery services for the intelligence system.
// It wraps the existing discovery.Discoverer to provide parallel provider queries
// with timeout enforcement and registry writing capabilities.
package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/traylinx/switchAILocal/internal/discovery"
	"github.com/traylinx/switchAILocal/internal/intelligence/capability"
	"github.com/traylinx/switchAILocal/internal/util"
)

// DiscoveredModel represents a model discovered from a provider.
type DiscoveredModel struct {
	ID           string                      `json:"id"`
	Provider     string                      `json:"provider"`
	DisplayName  string                      `json:"display_name"`
	Capabilities *capability.ModelCapability `json:"capabilities,omitempty"`
	IsAvailable  bool                        `json:"is_available"`
	DiscoveredAt time.Time                   `json:"discovered_at"`
}

// ModelCapability is re-exported from the capability package for convenience.
type ModelCapability = capability.ModelCapability

// ProviderStatus represents the status of a provider during discovery.
type ProviderStatus struct {
	Provider     string    `json:"provider"`
	IsAvailable  bool      `json:"is_available"`
	ModelCount   int       `json:"model_count"`
	Error        string    `json:"error,omitempty"`
	DiscoveredAt time.Time `json:"discovered_at"`
}

// DiscoveryRegistry represents the complete discovery output.
type DiscoveryRegistry struct {
	Providers    []*ProviderStatus   `json:"providers"`
	Models       []*DiscoveredModel  `json:"models"`
	GeneratedAt  time.Time           `json:"generated_at"`
	TotalModels  int                 `json:"total_models"`
}

// Service wraps the existing discovery.Discoverer and provides
// intelligence-specific functionality like parallel queries and registry writing.
type Service struct {
	discoverer *discovery.Discoverer
	analyzer   *capability.Analyzer
	models     []*DiscoveredModel
	providers  []*ProviderStatus
	mu         sync.RWMutex
	cacheDir   string
	stateBox   *util.StateBox
}

// NewService creates a new DiscoveryService instance.
//
// Parameters:
//   - cacheDir: Directory where discovery cache and registry will be stored (deprecated, use SetStateBox)
//   - stateBox: StateBox instance for path resolution and read-only mode enforcement
//
// Returns:
//   - *Service: A new discovery service instance
//   - error: Any error encountered during initialization
func NewService(cacheDir string, stateBox *util.StateBox) (*Service, error) {
	// If StateBox is provided, use it for path resolution
	if stateBox != nil {
		cacheDir = stateBox.DiscoveryDir()
		
		// Ensure the discovery directory exists
		if err := stateBox.EnsureDir(cacheDir); err != nil {
			return nil, fmt.Errorf("failed to create discovery directory: %w", err)
		}
	} else {
		// Fallback to legacy behavior if StateBox is not provided
		// Expand home directory if needed
		if len(cacheDir) > 0 && cacheDir[0] == '~' {
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, fmt.Errorf("failed to get home directory: %w", err)
			}
			cacheDir = filepath.Join(home, cacheDir[1:])
		}

		// Create cache directory if it doesn't exist
		if err := os.MkdirAll(cacheDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create cache directory: %w", err)
		}
	}

	// Create the underlying discoverer
	discoverer, err := discovery.NewDiscoverer(cacheDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create discoverer: %w", err)
	}

	return &Service{
		discoverer: discoverer,
		analyzer:   capability.NewAnalyzer(),
		models:     make([]*DiscoveredModel, 0),
		providers:  make([]*ProviderStatus, 0),
		cacheDir:   cacheDir,
		stateBox:   stateBox,
	}, nil
}

// DiscoverAll runs discovery for all configured providers with a 5-second timeout.
// It queries providers in parallel and gracefully handles failures.
//
// Parameters:
//   - ctx: Context for the discovery operation
//
// Returns:
//   - error: Any error encountered during discovery (non-fatal provider errors are logged)
func (s *Service) DiscoverAll(ctx context.Context) error {
	// Create a context with 5-second timeout
	discoveryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	log.Info("Starting model discovery (5s timeout)...")
	startTime := time.Now()

	// Run discovery using the existing discoverer
	providerModels, err := s.discoverer.DiscoverAll(discoveryCtx)
	
	// Even if there's an error, we may have partial results
	s.mu.Lock()
	defer s.mu.Unlock()

	// Reset state
	s.models = make([]*DiscoveredModel, 0)
	s.providers = make([]*ProviderStatus, 0)

	now := time.Now()
	totalModels := 0

	// Process results from each provider
	for providerID, models := range providerModels {
		status := &ProviderStatus{
			Provider:     providerID,
			IsAvailable:  len(models) > 0,
			ModelCount:   len(models),
			DiscoveredAt: now,
		}

		if len(models) == 0 {
			status.Error = "no models discovered"
		}

		s.providers = append(s.providers, status)

		// Convert registry.ModelInfo to DiscoveredModel
		for _, model := range models {
			discovered := &DiscoveredModel{
				ID:           model.ID,
				Provider:     providerID,
				DisplayName:  model.DisplayName,
				IsAvailable:  true,
				DiscoveredAt: now,
			}
			
			// Analyze capabilities using the capability analyzer
			// Convert to capability.DiscoveredModel for analysis
			capModel := &capability.DiscoveredModel{
				ID:          discovered.ID,
				Provider:    discovered.Provider,
				DisplayName: discovered.DisplayName,
			}
			discovered.Capabilities = s.analyzer.Analyze(capModel)
			
			s.models = append(s.models, discovered)
			totalModels++
		}
	}

	duration := time.Since(startTime)
	log.Infof("Discovery completed in %v: %d models from %d providers", 
		duration, totalModels, len(s.providers))

	if err != nil {
		log.Warnf("Discovery completed with errors: %v", err)
	}

	return nil
}

// GetAvailableModels returns the list of discovered models.
//
// Returns:
//   - []*DiscoveredModel: List of discovered models
func (s *Service) GetAvailableModels() []*DiscoveredModel {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Return a copy to prevent external modification
	result := make([]*DiscoveredModel, len(s.models))
	copy(result, s.models)
	return result
}

// GetAvailableModelsAsMap returns discovered models as a slice of maps for Lua interop.
//
// Returns:
//   - []map[string]interface{}: List of models as maps
func (s *Service) GetAvailableModelsAsMap() []map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]map[string]interface{}, 0, len(s.models))
	for _, model := range s.models {
		m := map[string]interface{}{
			"id":            model.ID,
			"provider":      model.Provider,
			"display_name":  model.DisplayName,
			"is_available":  model.IsAvailable,
			"discovered_at": model.DiscoveredAt.Format(time.RFC3339),
		}
		if model.Capabilities != nil {
			m["capabilities"] = map[string]interface{}{
				"supports_coding":    model.Capabilities.SupportsCoding,
				"supports_reasoning": model.Capabilities.SupportsReasoning,
				"supports_vision":    model.Capabilities.SupportsVision,
				"context_window":     model.Capabilities.ContextWindow,
				"estimated_latency":  model.Capabilities.EstimatedLatency,
				"cost_tier":          model.Capabilities.CostTier,
				"is_local":           model.Capabilities.IsLocal,
			}
		}
		result = append(result, m)
	}
	return result
}

// GetProviderStatus returns the status of all providers.
//
// Returns:
//   - []*ProviderStatus: List of provider statuses
func (s *Service) GetProviderStatus() []*ProviderStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Return a copy to prevent external modification
	result := make([]*ProviderStatus, len(s.providers))
	copy(result, s.providers)
	return result
}

// WriteRegistry writes the discovery results to available_models.json.
//
// Parameters:
//   - path: Path to write the registry file (if empty, uses cacheDir/available_models.json)
//
// Returns:
//   - error: Any error encountered during writing
func (s *Service) WriteRegistry(path string) error {
	// Check read-only mode first
	if s.stateBox != nil && s.stateBox.IsReadOnly() {
		log.Warn("Skipping registry write: read-only mode enabled")
		return util.ErrReadOnlyMode
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	// Use default path if not specified
	if path == "" {
		path = filepath.Join(s.cacheDir, "available_models.json")
	}

	// Create the registry structure
	registry := &DiscoveryRegistry{
		Providers:   s.providers,
		Models:      s.models,
		GeneratedAt: time.Now(),
		TotalModels: len(s.models),
	}

	// Use SecureWriteJSON if StateBox is available, otherwise fall back to legacy write
	if s.stateBox != nil {
		opts := &util.SecureWriteOptions{
			CreateBackup: true,
			Permissions:  0600,
		}
		if err := util.SecureWriteJSON(s.stateBox, path, registry, opts); err != nil {
			return fmt.Errorf("failed to write registry file: %w", err)
		}
	} else {
		// Legacy write for backward compatibility
		data, err := json.MarshalIndent(registry, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal registry: %w", err)
		}

		if err := os.WriteFile(path, data, 0644); err != nil {
			return fmt.Errorf("failed to write registry file: %w", err)
		}
	}

	log.Infof("Wrote discovery registry to %s (%d models)", path, len(s.models))
	return nil
}

// SetStateBox configures the StateBox for the discovery service.
// This allows the service to use StateBox for path resolution and read-only mode enforcement.
//
// Parameters:
//   - sb: StateBox instance to use
func (s *Service) SetStateBox(sb *util.StateBox) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.stateBox = sb
	if sb != nil {
		// Update cacheDir to use StateBox discovery directory
		s.cacheDir = sb.DiscoveryDir()
		
		// Ensure the discovery directory exists
		if err := sb.EnsureDir(s.cacheDir); err != nil {
			log.Warnf("Failed to create discovery directory: %v", err)
		}
	}
}

// Shutdown gracefully stops the discovery service.
//
// Parameters:
//   - ctx: Context for shutdown operations
//
// Returns:
//   - error: Any error encountered during shutdown
func (s *Service) Shutdown(ctx context.Context) error {
	// Discovery service doesn't have any background processes to stop
	log.Debug("Discovery service shutdown complete")
	return nil
}
