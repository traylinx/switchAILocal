// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package management

import (
	"encoding/json"
	"net/http"

	log "github.com/sirupsen/logrus"
)

// CacheMetricsResponse represents the cache metrics API response.
type CacheMetricsResponse struct {
	Enabled    bool                   `json:"enabled"`
	Metrics    map[string]interface{} `json:"metrics,omitempty"`
	Error      string                 `json:"error,omitempty"`
}

// HandleCacheMetrics returns cache performance metrics.
// GET /v0/management/cache/metrics
func (h *Handler) HandleCacheMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	response := CacheMetricsResponse{
		Enabled: false,
	}

	// Check if intelligence service is available
	if h.intelligenceService == nil || !h.intelligenceService.IsEnabled() {
		response.Error = "intelligence services not enabled"
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
		return
	}

	// Get semantic cache
	cache := h.intelligenceService.GetSemanticCache()
	if cache == nil {
		response.Error = "semantic cache not available"
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
		return
	}

	if !cache.IsEnabled() {
		response.Error = "semantic cache not initialized"
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
		return
	}

	// Get metrics
	response.Enabled = true
	response.Metrics = cache.GetMetricsAsMap()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Errorf("Failed to encode cache metrics response: %v", err)
	}
}

// HandleCacheClear clears all entries from the semantic cache.
// POST /v0/management/cache/clear
func (h *Handler) HandleCacheClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	type ClearResponse struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Error   string `json:"error,omitempty"`
	}

	response := ClearResponse{
		Success: false,
	}

	// Check if intelligence service is available
	if h.intelligenceService == nil || !h.intelligenceService.IsEnabled() {
		response.Error = "intelligence services not enabled"
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	// Get semantic cache
	cache := h.intelligenceService.GetSemanticCache()
	if cache == nil {
		response.Error = "semantic cache not available"
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	if !cache.IsEnabled() {
		response.Error = "semantic cache not initialized"
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	// Clear cache using reflection since we can't import cache package
	// We need to call the Clear() method
	type cacheClearer interface {
		Clear()
	}
	
	if clearer, ok := cache.(cacheClearer); ok {
		clearer.Clear()
		response.Success = true
		response.Message = "Cache cleared successfully"
	} else {
		response.Error = "cache does not support clearing"
	}

	w.Header().Set("Content-Type", "application/json")
	if response.Success {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusInternalServerError)
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Errorf("Failed to encode cache clear response: %v", err)
	}
}
