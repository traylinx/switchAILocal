// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package registry provides centralized model management for all AI service providers.
// It implements a dynamic model registry with reference counting to track active clients
// and automatically hide models when no clients are available or when quota is exceeded.
package registry

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	misc "github.com/traylinx/switchAILocal/internal/misc"
)

// ModelInfo represents information about an available model
type ModelInfo struct {
	// ID is the unique identifier for the model
	ID string `json:"id"`
	// Object type for the model (typically "model")
	Object string `json:"object"`
	// Created timestamp when the model was created
	Created int64 `json:"created"`
	// OwnedBy indicates the organization that owns the model
	OwnedBy string `json:"owned_by"`
	// Type indicates the model type (e.g., "claude", "gemini", "openai")
	Type string `json:"type"`
	// DisplayName is the human-readable name for the model
	DisplayName string `json:"display_name,omitempty"`
	// Name is used for Gemini-style model names
	Name string `json:"name,omitempty"`
	// Version is the model version
	Version string `json:"version,omitempty"`
	// Description provides detailed information about the model
	Description string `json:"description,omitempty"`
	// InputTokenLimit is the maximum input token limit
	InputTokenLimit int `json:"inputTokenLimit,omitempty"`
	// OutputTokenLimit is the maximum output token limit
	OutputTokenLimit int `json:"outputTokenLimit,omitempty"`
	// SupportedGenerationMethods lists supported generation methods
	SupportedGenerationMethods []string `json:"supportedGenerationMethods,omitempty"`
	// ContextLength is the context window size
	ContextLength int `json:"context_length,omitempty"`
	// MaxCompletionTokens is the maximum completion tokens
	MaxCompletionTokens int `json:"max_completion_tokens,omitempty"`
	// SupportedParameters lists supported parameters
	SupportedParameters []string `json:"supported_parameters,omitempty"`

	// Thinking holds provider-specific reasoning/thinking budget capabilities.
	// This is optional and currently used for Gemini thinking budget normalization.
	Thinking *ThinkingSupport `json:"thinking,omitempty"`
}

// ThinkingSupport describes a model family's supported internal reasoning budget range.
// Values are interpreted in provider-native token units.
type ThinkingSupport struct {
	// Min is the minimum allowed thinking budget (inclusive).
	Min int `json:"min,omitempty"`
	// Max is the maximum allowed thinking budget (inclusive).
	Max int `json:"max,omitempty"`
	// ZeroAllowed indicates whether 0 is a valid value (to disable thinking).
	ZeroAllowed bool `json:"zero_allowed,omitempty"`
	// DynamicAllowed indicates whether -1 is a valid value (dynamic thinking budget).
	DynamicAllowed bool `json:"dynamic_allowed,omitempty"`
	// Levels defines discrete reasoning effort levels (e.g., "low", "medium", "high").
	// When set, the model uses level-based reasoning instead of token budgets.
	Levels []string `json:"levels,omitempty"`
}

// ModelRegistration tracks a model's availability
type ModelRegistration struct {
	// Info contains the model metadata
	Info *ModelInfo
	// Count is the number of active clients that can provide this model
	Count int
	// LastUpdated tracks when this registration was last modified
	LastUpdated time.Time
	// QuotaExceededClients tracks which clients have exceeded quota for this model
	QuotaExceededClients map[string]*time.Time
	// Providers tracks available clients grouped by provider identifier
	Providers map[string]int
	// SuspendedClients tracks temporarily disabled clients keyed by client ID
	SuspendedClients map[string]string
}

// ModelRegistry manages the global registry of available models
type ModelRegistry struct {
	// models maps model ID to registration information
	models map[string]*ModelRegistration
	// clientModels maps client ID to the models it provides
	clientModels map[string][]string
	// clientModelInfos maps client ID to a map of model ID -> ModelInfo
	// This preserves the original model info provided by each client
	clientModelInfos map[string]map[string]*ModelInfo
	// clientProviders maps client ID to its provider identifier
	clientProviders map[string]string
	// mutex ensures thread-safe access to the registry
	mutex *sync.RWMutex
}

// Global model registry instance
var globalRegistry *ModelRegistry
var registryOnce sync.Once

// NewModelRegistry creates a new, empty model registry.
func NewModelRegistry() *ModelRegistry {
	return &ModelRegistry{
		models:           make(map[string]*ModelRegistration),
		clientModels:     make(map[string][]string),
		clientModelInfos: make(map[string]map[string]*ModelInfo),
		clientProviders:  make(map[string]string),
		mutex:            &sync.RWMutex{},
	}
}

// GetGlobalRegistry returns the global model registry instance
func GetGlobalRegistry() *ModelRegistry {
	registryOnce.Do(func() {
		globalRegistry = NewModelRegistry()
	})
	return globalRegistry
}

// RegisterClient registers a client and its supported models
// Parameters:
//   - clientID: Unique identifier for the client
//   - clientProvider: Provider name (e.g., "gemini", "claude", "openai")
//   - models: List of models that this client can provide
func (r *ModelRegistry) RegisterClient(clientID, clientProvider string, models []*ModelInfo) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	provider := strings.ToLower(clientProvider)
	uniqueModelIDs := make([]string, 0, len(models))
	rawModelIDs := make([]string, 0, len(models))
	newModels := make(map[string]*ModelInfo, len(models))
	newCounts := make(map[string]int, len(models))
	for _, model := range models {
		if model == nil || model.ID == "" {
			continue
		}
		rawModelIDs = append(rawModelIDs, model.ID)
		newCounts[model.ID]++
		if _, exists := newModels[model.ID]; exists {
			continue
		}
		newModels[model.ID] = model
		uniqueModelIDs = append(uniqueModelIDs, model.ID)
	}

	if len(uniqueModelIDs) == 0 {
		// No models supplied; unregister existing client state if present.
		r.unregisterClientInternal(clientID)
		delete(r.clientModels, clientID)
		delete(r.clientModelInfos, clientID)
		delete(r.clientProviders, clientID)
		misc.LogCredentialSeparator()
		return
	}

	now := time.Now()

	oldModels, hadExisting := r.clientModels[clientID]
	oldProvider := r.clientProviders[clientID]
	providerChanged := oldProvider != provider
	if !hadExisting {
		// Pure addition path.
		for _, modelID := range rawModelIDs {
			model := newModels[modelID]
			r.addModelRegistration(modelID, provider, model, now)
		}
		r.clientModels[clientID] = append([]string(nil), rawModelIDs...)
		// Store client's own model infos
		clientInfos := make(map[string]*ModelInfo, len(newModels))
		for id, m := range newModels {
			clientInfos[id] = cloneModelInfo(m)
		}
		r.clientModelInfos[clientID] = clientInfos
		if provider != "" {
			r.clientProviders[clientID] = provider
		} else {
			delete(r.clientProviders, clientID)
		}
		log.Debugf("Registered client %s from provider %s with %d models", clientID, clientProvider, len(rawModelIDs))
		misc.LogCredentialSeparator()
		return
	}

	oldCounts := make(map[string]int, len(oldModels))
	for _, id := range oldModels {
		oldCounts[id]++
	}

	added := make([]string, 0)
	for _, id := range uniqueModelIDs {
		if oldCounts[id] == 0 {
			added = append(added, id)
		}
	}

	removed := make([]string, 0)
	for id := range oldCounts {
		if newCounts[id] == 0 {
			removed = append(removed, id)
		}
	}

	// Handle provider change for overlapping models before modifications.
	if providerChanged && oldProvider != "" {
		for id, newCount := range newCounts {
			if newCount == 0 {
				continue
			}
			oldCount := oldCounts[id]
			if oldCount == 0 {
				continue
			}
			toRemove := newCount
			if oldCount < toRemove {
				toRemove = oldCount
			}
			if reg, ok := r.models[id]; ok && reg.Providers != nil {
				if count, okProv := reg.Providers[oldProvider]; okProv {
					if count <= toRemove {
						delete(reg.Providers, oldProvider)
					} else {
						reg.Providers[oldProvider] = count - toRemove
					}
				}
			}
		}
	}

	// Apply removals first to keep counters accurate.
	for _, id := range removed {
		oldCount := oldCounts[id]
		for i := 0; i < oldCount; i++ {
			r.removeModelRegistration(clientID, id, oldProvider, now)
		}
	}

	for id, oldCount := range oldCounts {
		newCount := newCounts[id]
		if newCount == 0 || oldCount <= newCount {
			continue
		}
		overage := oldCount - newCount
		for i := 0; i < overage; i++ {
			r.removeModelRegistration(clientID, id, oldProvider, now)
		}
	}

	// Apply additions.
	for id, newCount := range newCounts {
		oldCount := oldCounts[id]
		if newCount <= oldCount {
			continue
		}
		model := newModels[id]
		diff := newCount - oldCount
		for i := 0; i < diff; i++ {
			r.addModelRegistration(id, provider, model, now)
		}
	}

	// Update metadata for models that remain associated with the client.
	addedSet := make(map[string]struct{}, len(added))
	for _, id := range added {
		addedSet[id] = struct{}{}
	}
	for _, id := range uniqueModelIDs {
		model := newModels[id]
		if reg, ok := r.models[id]; ok {
			reg.Info = cloneModelInfo(model)
			reg.LastUpdated = now
			if reg.QuotaExceededClients != nil {
				delete(reg.QuotaExceededClients, clientID)
			}
			if reg.SuspendedClients != nil {
				delete(reg.SuspendedClients, clientID)
			}
			if providerChanged && provider != "" {
				if _, newlyAdded := addedSet[id]; newlyAdded {
					continue
				}
				overlapCount := newCounts[id]
				if oldCount := oldCounts[id]; oldCount < overlapCount {
					overlapCount = oldCount
				}
				if overlapCount <= 0 {
					continue
				}
				if reg.Providers == nil {
					reg.Providers = make(map[string]int)
				}
				reg.Providers[provider] += overlapCount
			}
		}
	}

	// Update client bookkeeping.
	if len(rawModelIDs) > 0 {
		r.clientModels[clientID] = append([]string(nil), rawModelIDs...)
	}
	// Update client's own model infos
	clientInfos := make(map[string]*ModelInfo, len(newModels))
	for id, m := range newModels {
		clientInfos[id] = cloneModelInfo(m)
	}
	r.clientModelInfos[clientID] = clientInfos
	if provider != "" {
		r.clientProviders[clientID] = provider
	} else {
		delete(r.clientProviders, clientID)
	}

	if len(added) == 0 && len(removed) == 0 && !providerChanged {
		// Only metadata (e.g., display name) changed; skip separator when no log output.
		return
	}

	log.Debugf("Reconciled client %s (provider %s) models: +%d, -%d", clientID, provider, len(added), len(removed))
	misc.LogCredentialSeparator()
}

func (r *ModelRegistry) addModelRegistration(modelID, provider string, model *ModelInfo, now time.Time) {
	if model == nil || modelID == "" {
		return
	}
	if existing, exists := r.models[modelID]; exists {
		existing.Count++
		existing.LastUpdated = now
		existing.Info = cloneModelInfo(model)
		if existing.SuspendedClients == nil {
			existing.SuspendedClients = make(map[string]string)
		}
		if provider != "" {
			if existing.Providers == nil {
				existing.Providers = make(map[string]int)
			}
			existing.Providers[provider]++
		}
		log.Debugf("Incremented count for model %s, now %d clients", modelID, existing.Count)
		return
	}

	registration := &ModelRegistration{
		Info:                 cloneModelInfo(model),
		Count:                1,
		LastUpdated:          now,
		QuotaExceededClients: make(map[string]*time.Time),
		SuspendedClients:     make(map[string]string),
	}
	if provider != "" {
		registration.Providers = map[string]int{provider: 1}
	}
	r.models[modelID] = registration
	log.Debugf("Registered new model %s from provider %s", modelID, provider)
}

func (r *ModelRegistry) removeModelRegistration(clientID, modelID, provider string, now time.Time) {
	registration, exists := r.models[modelID]
	if !exists {
		return
	}
	registration.Count--
	registration.LastUpdated = now
	if registration.QuotaExceededClients != nil {
		delete(registration.QuotaExceededClients, clientID)
	}
	if registration.SuspendedClients != nil {
		delete(registration.SuspendedClients, clientID)
	}
	if registration.Count < 0 {
		registration.Count = 0
	}
	if provider != "" && registration.Providers != nil {
		if count, ok := registration.Providers[provider]; ok {
			if count <= 1 {
				delete(registration.Providers, provider)
			} else {
				registration.Providers[provider] = count - 1
			}
		}
	}
	log.Debugf("Decremented count for model %s, now %d clients", modelID, registration.Count)
	if registration.Count <= 0 {
		delete(r.models, modelID)
		log.Debugf("Removed model %s as no clients remain", modelID)
	}
}

func cloneModelInfo(model *ModelInfo) *ModelInfo {
	if model == nil {
		return nil
	}
	copyModel := *model
	if len(model.SupportedGenerationMethods) > 0 {
		copyModel.SupportedGenerationMethods = append([]string(nil), model.SupportedGenerationMethods...)
	}
	if len(model.SupportedParameters) > 0 {
		copyModel.SupportedParameters = append([]string(nil), model.SupportedParameters...)
	}
	return &copyModel
}

// UnregisterClient removes a client and decrements counts for its models
// Parameters:
//   - clientID: Unique identifier for the client to remove
func (r *ModelRegistry) UnregisterClient(clientID string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.unregisterClientInternal(clientID)
}

// unregisterClientInternal performs the actual client unregistration (internal, no locking)
func (r *ModelRegistry) unregisterClientInternal(clientID string) {
	models, exists := r.clientModels[clientID]
	provider, hasProvider := r.clientProviders[clientID]
	if !exists {
		if hasProvider {
			delete(r.clientProviders, clientID)
		}
		return
	}

	now := time.Now()
	for _, modelID := range models {
		if registration, isExists := r.models[modelID]; isExists {
			registration.Count--
			registration.LastUpdated = now

			// Remove quota tracking for this client
			delete(registration.QuotaExceededClients, clientID)
			if registration.SuspendedClients != nil {
				delete(registration.SuspendedClients, clientID)
			}

			if hasProvider && registration.Providers != nil {
				if count, ok := registration.Providers[provider]; ok {
					if count <= 1 {
						delete(registration.Providers, provider)
					} else {
						registration.Providers[provider] = count - 1
					}
				}
			}

			log.Debugf("Decremented count for model %s, now %d clients", modelID, registration.Count)

			// Remove model if no clients remain
			if registration.Count <= 0 {
				delete(r.models, modelID)
				log.Debugf("Removed model %s as no clients remain", modelID)
			}
		}
	}

	delete(r.clientModels, clientID)
	delete(r.clientModelInfos, clientID)
	if hasProvider {
		delete(r.clientProviders, clientID)
	}
	log.Debugf("Unregistered client %s", clientID)
	// Separator line after completing client unregistration (after the summary line)
	misc.LogCredentialSeparator()
}

// SetModelQuotaExceeded marks a model as quota exceeded for a specific client
// Parameters:
//   - clientID: The client that exceeded quota
//   - modelID: The model that exceeded quota
func (r *ModelRegistry) SetModelQuotaExceeded(clientID, modelID string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if registration, exists := r.models[modelID]; exists {
		now := time.Now()
		registration.QuotaExceededClients[clientID] = &now
		log.Debugf("Marked model %s as quota exceeded for client %s", modelID, clientID)
	}
}

// ClearModelQuotaExceeded removes quota exceeded status for a model and client
// Parameters:
//   - clientID: The client to clear quota status for
//   - modelID: The model to clear quota status for
func (r *ModelRegistry) ClearModelQuotaExceeded(clientID, modelID string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if registration, exists := r.models[modelID]; exists {
		delete(registration.QuotaExceededClients, clientID)
		// log.Debugf("Cleared quota exceeded status for model %s and client %s", modelID, clientID)
	}
}

// SuspendClientModel marks a client's model as temporarily unavailable until explicitly resumed.
// Parameters:
//   - clientID: The client to suspend
//   - modelID: The model affected by the suspension
//   - reason: Optional description for observability
func (r *ModelRegistry) SuspendClientModel(clientID, modelID, reason string) {
	if clientID == "" || modelID == "" {
		return
	}
	r.mutex.Lock()
	defer r.mutex.Unlock()

	registration, exists := r.models[modelID]
	if !exists || registration == nil {
		return
	}
	if registration.SuspendedClients == nil {
		registration.SuspendedClients = make(map[string]string)
	}
	if _, already := registration.SuspendedClients[clientID]; already {
		return
	}
	registration.SuspendedClients[clientID] = reason
	registration.LastUpdated = time.Now()
	if reason != "" {
		log.Debugf("Suspended client %s for model %s: %s", clientID, modelID, reason)
	} else {
		log.Debugf("Suspended client %s for model %s", clientID, modelID)
	}
}

// ResumeClientModel clears a previous suspension so the client counts toward availability again.
// Parameters:
//   - clientID: The client to resume
//   - modelID: The model being resumed
func (r *ModelRegistry) ResumeClientModel(clientID, modelID string) {
	if clientID == "" || modelID == "" {
		return
	}
	r.mutex.Lock()
	defer r.mutex.Unlock()

	registration, exists := r.models[modelID]
	if !exists || registration == nil || registration.SuspendedClients == nil {
		return
	}
	if _, ok := registration.SuspendedClients[clientID]; !ok {
		return
	}
	delete(registration.SuspendedClients, clientID)
	registration.LastUpdated = time.Now()
	log.Debugf("Resumed client %s for model %s", clientID, modelID)
}

// ClientSupportsModel reports whether the client registered support for modelID.
// It checks both the registered model ID (alias) and the DisplayName (upstream model name).
func (r *ModelRegistry) ClientSupportsModel(clientID, modelID string) bool {
	clientID = strings.TrimSpace(clientID)
	modelID = strings.TrimSpace(modelID)
	if clientID == "" || modelID == "" {
		return false
	}

	r.mutex.RLock()
	defer r.mutex.RUnlock()

	models, exists := r.clientModels[clientID]
	if !exists || len(models) == 0 {
		return false
	}

	// First check by model ID (alias) or wildcard
	for _, id := range models {
		trimmedID := strings.TrimSpace(id)
		if trimmedID == "*" || strings.EqualFold(trimmedID, modelID) {
			return true
		}
	}

	// Also check by DisplayName (upstream model name) or wildcard
	clientInfos, hasInfos := r.clientModelInfos[clientID]
	if hasInfos {
		for _, info := range clientInfos {
			if info == nil {
				continue
			}
			displayName := strings.TrimSpace(info.DisplayName)
			if displayName == "*" || strings.EqualFold(displayName, modelID) {
				return true
			}
		}
	}

	return false
}

// GetAvailableModels returns all models that have at least one available client
// Parameters:
//   - handlerType: The handler type to filter models for (e.g., "openai", "claude", "gemini")
//
// Returns:
//   - []map[string]any: List of available models in the requested format
func (r *ModelRegistry) GetAvailableModels(handlerType string) []map[string]any {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return r.getAvailableModelsLocked(handlerType)
}

// getAvailableModelsLocked returns all models that have at least one available client (lock-free)
func (r *ModelRegistry) getAvailableModelsLocked(handlerType string) []map[string]any {
	models := make([]map[string]any, 0)
	quotaExpiredDuration := 5 * time.Minute

	for _, registration := range r.models {
		// Check if model has any non-quota-exceeded clients
		availableClients := registration.Count
		now := time.Now()

		// Count clients that have exceeded quota but haven't recovered yet
		expiredClients := 0
		for _, quotaTime := range registration.QuotaExceededClients {
			if quotaTime != nil && now.Sub(*quotaTime) < quotaExpiredDuration {
				expiredClients++
			}
		}

		cooldownSuspended := 0
		otherSuspended := 0
		if registration.SuspendedClients != nil {
			for _, reason := range registration.SuspendedClients {
				if strings.EqualFold(reason, "quota") {
					cooldownSuspended++
					continue
				}
				otherSuspended++
			}
		}

		effectiveClients := availableClients - expiredClients - otherSuspended
		if effectiveClients < 0 {
			effectiveClients = 0
		}

		// Include models that have available clients, or those solely cooling down.
		if effectiveClients > 0 || (availableClients > 0 && (expiredClients > 0 || cooldownSuspended > 0) && otherSuspended == 0) {
			model := r.convertModelToMap(registration.Info, handlerType)
			if model != nil {
				// Add provider attribution
				if len(registration.Providers) > 0 {
					providers := make([]string, 0, len(registration.Providers))
					for provider, count := range registration.Providers {
						if count > 0 {
							providers = append(providers, provider)
						}
					}
					sort.Strings(providers)
					if len(providers) > 0 {
						model["providers"] = providers
					}
				}
				models = append(models, model)
			}
		}
	}

	return models
}

// GetModelCount returns the number of available clients for a specific model
// Parameters:
//   - modelID: The model ID to check
//
// Returns:
//   - int: Number of available clients for the model
func (r *ModelRegistry) GetModelCount(modelID string) int {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return r.getModelCountLocked(modelID)
}

// getModelCountLocked returns the number of available clients for a specific model (lock-free)
func (r *ModelRegistry) getModelCountLocked(modelID string) int {
	if registration, exists := r.models[modelID]; exists {
		now := time.Now()
		quotaExpiredDuration := 5 * time.Minute

		// Count clients that have exceeded quota but haven't recovered yet
		expiredClients := 0
		for _, quotaTime := range registration.QuotaExceededClients {
			if quotaTime != nil && now.Sub(*quotaTime) < quotaExpiredDuration {
				expiredClients++
			}
		}
		suspendedClients := 0
		if registration.SuspendedClients != nil {
			suspendedClients = len(registration.SuspendedClients)
		}
		result := registration.Count - expiredClients - suspendedClients
		if result < 0 {
			return 0
		}
		return result
	}
	return 0
}

// GetModelProviders returns provider identifiers that currently supply the given model
// Parameters:
//   - modelID: The model ID to check
//
// Returns:
//   - []string: Provider identifiers ordered by availability count (descending)
func (r *ModelRegistry) GetModelProviders(modelID string) []string {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	registration, exists := r.models[modelID]
	if !exists || registration == nil || len(registration.Providers) == 0 {
		return nil
	}

	type providerCount struct {
		name  string
		count int
	}
	providers := make([]providerCount, 0, len(registration.Providers))
	// suspendedByProvider := make(map[string]int)
	// if registration.SuspendedClients != nil {
	// 	for clientID := range registration.SuspendedClients {
	// 		if provider, ok := r.clientProviders[clientID]; ok && provider != "" {
	// 			suspendedByProvider[provider]++
	// 		}
	// 	}
	// }
	for name, count := range registration.Providers {
		if count <= 0 {
			continue
		}
		// adjusted := count - suspendedByProvider[name]
		// if adjusted <= 0 {
		// 	continue
		// }
		// providers = append(providers, providerCount{name: name, count: adjusted})
		providers = append(providers, providerCount{name: name, count: count})
	}
	if len(providers) == 0 {
		return nil
	}

	sort.Slice(providers, func(i, j int) bool {
		if providers[i].count == providers[j].count {
			return providers[i].name < providers[j].name
		}
		return providers[i].count > providers[j].count
	})

	result := make([]string, 0, len(providers))
	for _, item := range providers {
		result = append(result, item.name)
	}
	return result
}

// GetModelInfo returns the registered ModelInfo for the given model ID, if present.
// Returns nil if the model is unknown to the registry.
func (r *ModelRegistry) GetModelInfo(modelID string) *ModelInfo {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	if reg, ok := r.models[modelID]; ok && reg != nil {
		return reg.Info
	}
	return nil
}

// convertModelToMap converts ModelInfo to the appropriate format for different handler types
func (r *ModelRegistry) convertModelToMap(model *ModelInfo, handlerType string) map[string]any {
	if model == nil {
		return nil
	}

	switch handlerType {
	case "openai":
		result := map[string]any{
			"id":       model.ID,
			"object":   "model",
			"owned_by": model.OwnedBy,
		}
		if model.Created > 0 {
			result["created"] = model.Created
		}
		if model.Type != "" {
			result["type"] = model.Type
		}
		if model.DisplayName != "" {
			result["display_name"] = model.DisplayName
		}
		if model.Version != "" {
			result["version"] = model.Version
		}
		if model.Description != "" {
			result["description"] = model.Description
		}
		if model.ContextLength > 0 {
			result["context_length"] = model.ContextLength
		}
		if model.MaxCompletionTokens > 0 {
			result["max_completion_tokens"] = model.MaxCompletionTokens
		}
		if len(model.SupportedParameters) > 0 {
			result["supported_parameters"] = model.SupportedParameters
		}
		return result

	case "claude":
		result := map[string]any{
			"id":       model.ID,
			"object":   "model",
			"owned_by": model.OwnedBy,
		}
		if model.Created > 0 {
			result["created"] = model.Created
		}
		if model.Type != "" {
			result["type"] = model.Type
		}
		if model.DisplayName != "" {
			result["display_name"] = model.DisplayName
		}
		return result

	case "gemini":
		result := map[string]any{}
		if model.Name != "" {
			result["name"] = model.Name
		} else {
			result["name"] = model.ID
		}
		if model.Version != "" {
			result["version"] = model.Version
		}
		if model.DisplayName != "" {
			result["displayName"] = model.DisplayName
		}
		if model.Description != "" {
			result["description"] = model.Description
		}
		if model.InputTokenLimit > 0 {
			result["inputTokenLimit"] = model.InputTokenLimit
		}
		if model.OutputTokenLimit > 0 {
			result["outputTokenLimit"] = model.OutputTokenLimit
		}
		if len(model.SupportedGenerationMethods) > 0 {
			result["supportedGenerationMethods"] = model.SupportedGenerationMethods
		}
		return result

	default:
		// Generic format
		result := map[string]any{
			"id":     model.ID,
			"object": "model",
		}
		if model.OwnedBy != "" {
			result["owned_by"] = model.OwnedBy
		}
		if model.Type != "" {
			result["type"] = model.Type
		}
		if model.Created != 0 {
			result["created"] = model.Created
		}
		return result
	}
}

// CleanupExpiredQuotas removes expired quota tracking entries
func (r *ModelRegistry) CleanupExpiredQuotas() {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	now := time.Now()
	quotaExpiredDuration := 5 * time.Minute

	for modelID, registration := range r.models {
		for clientID, quotaTime := range registration.QuotaExceededClients {
			if quotaTime != nil && now.Sub(*quotaTime) >= quotaExpiredDuration {
				delete(registration.QuotaExceededClients, clientID)
				log.Debugf("Cleaned up expired quota tracking for model %s, client %s", modelID, clientID)
			}
		}
	}
}

// GetFirstAvailableModel returns the first available model for the given handler type.
// It first checks the provided priorityList. If no model from the list is available,
// it prioritizes remaining models by their creation timestamp (newest first).
//
// Parameters:
//   - handlerType: The API handler type (e.g., "openai", "claude", "gemini")
//   - priorityList: Optional list of model IDs to check first
//
// Returns:
//   - string: The model ID of the first available model, or empty string if none available
//   - error: An error if no models are available
func (r *ModelRegistry) GetFirstAvailableModel(handlerType string, priorityList []string) (string, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	// Get all available models for this handler type
	models := r.getAvailableModelsLocked(handlerType)
	if len(models) == 0 {
		return "", fmt.Errorf("no models available for handler type: %s", handlerType)
	}

	// 1. Check priority list first
	for _, priorityID := range priorityList {
		var requiredProvider string
		targetModelID := priorityID

		// Check for provider prefix (e.g. "ollama:gpt-oss:120b-cloud")
		if parts := strings.SplitN(priorityID, ":", 2); len(parts) == 2 {
			requiredProvider = parts[0]
			targetModelID = parts[1]
		}

		for _, model := range models {
			// Check ID match (case-insensitive)
			if id, ok := model["id"].(string); ok && strings.EqualFold(id, targetModelID) {
				// If a specific provider was requested, enforce it
				if requiredProvider != "" {
					if ownedBy, ok := model["owned_by"].(string); !ok || !strings.EqualFold(ownedBy, requiredProvider) {
						continue // Provider mismatch
					}
				}

				if r.getModelCountLocked(id) > 0 {
					return id, nil
				}
			}
		}
	}

	// 2. Sort remaining models by creation timestamp (newest first)
	sort.Slice(models, func(i, j int) bool {
		// Extract created timestamps from map
		createdI, okI := models[i]["created"].(int64)
		createdJ, okJ := models[j]["created"].(int64)
		if !okI || !okJ {
			return false
		}
		return createdI > createdJ
	})

	// Find the first model with available clients
	for _, model := range models {
		if modelID, ok := model["id"].(string); ok {
			if count := r.getModelCountLocked(modelID); count > 0 {
				return modelID, nil
			}
		}
	}

	return "", fmt.Errorf("no available clients for any model in handler type: %s", handlerType)
}

// GetModelsForClient returns the models registered for a specific client.
// Parameters:
//   - clientID: The client identifier (typically auth file name or auth ID)
//
// Returns:
//   - []*ModelInfo: List of models registered for this client, nil if client not found
func (r *ModelRegistry) GetModelsForClient(clientID string) []*ModelInfo {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	modelIDs, exists := r.clientModels[clientID]
	if !exists || len(modelIDs) == 0 {
		return nil
	}

	// Try to use client-specific model infos first
	clientInfos := r.clientModelInfos[clientID]

	seen := make(map[string]struct{})
	result := make([]*ModelInfo, 0, len(modelIDs))
	for _, modelID := range modelIDs {
		if _, dup := seen[modelID]; dup {
			continue
		}
		seen[modelID] = struct{}{}

		// Prefer client's own model info to preserve original type/owned_by
		if clientInfos != nil {
			if info, ok := clientInfos[modelID]; ok && info != nil {
				result = append(result, info)
				continue
			}
		}
		// Fallback to global registry (for backwards compatibility)
		if reg, ok := r.models[modelID]; ok && reg.Info != nil {
			result = append(result, reg.Info)
		}
	}
	return result
}

// ProviderInfo represents information about an AI provider
type ProviderInfo struct {
	// ID is the unique identifier for the provider (e.g., "gemini", "claude", "ollama")
	ID string `json:"id"`
	// Name is the human-readable name
	Name string `json:"name"`
	// Type indicates the provider type ("api" or "cli")
	Type string `json:"type"`
	// Mode indicates the operational mode ("local" or "online")
	Mode string `json:"mode"`
	// Status indicates provider availability ("active", "degraded", "unavailable")
	Status string `json:"status"`
	// ModelCount is the number of models available from this provider
	ModelCount int `json:"model_count"`
	// Models lists the model IDs available from this provider
	Models []string `json:"models,omitempty"`
}

// GetAllProviders returns information about all registered providers
func (r *ModelRegistry) GetAllProviders() []ProviderInfo {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	// Aggregate by provider
	providerModels := make(map[string][]string)

	for modelID, registration := range r.models {
		if registration == nil || registration.Count <= 0 {
			continue
		}
		for provider, count := range registration.Providers {
			if count > 0 {
				providerModels[provider] = append(providerModels[provider], modelID)
			}
		}
	}

	// Build provider info list
	providers := make([]ProviderInfo, 0, len(providerModels))
	for providerID, models := range providerModels {
		// Sort models for consistent output
		sort.Strings(models)

		status := "active"
		if len(models) == 0 {
			status = "unavailable"
		}

		displayName := providerID
		providerType := "api"
		providerMode := "online"

		switch providerID {
		case "gemini":
			displayName = "Google Gemini"
		case "geminicli":
			displayName = "Gemini CLI"
			providerType = "cli"
			providerMode = "local"
		case "vertex":
			displayName = "Google Vertex AI"
		case "aistudio":
			displayName = "Google AI Studio"
		case "claude":
			displayName = "Anthropic Claude"
		case "claudecli":
			displayName = "Claude CLI"
			providerType = "cli"
			providerMode = "local"
		case "codex":
			displayName = "OpenAI Codex"
			providerType = "cli"
			providerMode = "local"
		case "ollama":
			displayName = "Ollama (Local)"
			providerMode = "local"
		case "vibe":
			displayName = "Mistral Vibe"
			providerType = "cli"
			providerMode = "local"
		case "switchai":
			displayName = "SwitchAI"
		case "groq":
			displayName = "Groq"
		case "antigravity":
			displayName = "Antigravity"
			providerType = "cli"
			providerMode = "local"
		case "qwen":
			displayName = "Qwen"
			providerType = "cli"
			providerMode = "local"
		case "iflow":
			displayName = "iFlow"
			providerType = "cli"
			providerMode = "local"
		case "openai":
			displayName = "OpenAI"
		case "openai-compat":
			displayName = "OpenAI Compatible"
		}

		providers = append(providers, ProviderInfo{
			ID:         providerID,
			Name:       displayName,
			Type:       providerType,
			Mode:       providerMode,
			Status:     status,
			ModelCount: len(models),
			Models:     models,
		})
	}

	// Sort by provider ID for consistent output
	sort.Slice(providers, func(i, j int) bool {
		return providers[i].ID < providers[j].ID
	})

	return providers
}

// GetModelsWithMinContext returns all active models that support at least the given context length.
func (mr *ModelRegistry) GetModelsWithMinContext(minContext int) []*ModelInfo {
	mr.mutex.RLock()
	defer mr.mutex.RUnlock()

	var suitable []*ModelInfo
	for _, reg := range mr.models {
		// Only consider models with count > 0 (active)
		// Or maybe we want all registered models?
		// For recommendations, usually we want things the user CAN use.
		// If Count is 0, it means no clients are connected/configured for it.
		// But if it's in the registry, it's "known".
		// Let's stick to active models or those with explicit config.
		// For now, return all registered models as they might represent configured-but-inactive providers.
		// But simpler: just check context length.
		if reg.Info.ContextLength >= minContext {
			suitable = append(suitable, reg.Info)
		}
	}
	// Sort by context length (ascending)
	sort.Slice(suitable, func(i, j int) bool {
		return suitable[i].ContextLength < suitable[j].ContextLength
	})
	return suitable
}
