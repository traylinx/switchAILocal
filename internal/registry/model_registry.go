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
	// Visibility controls normal public catalog exposure. "private" models
	// remain routable internally but are omitted from GetAvailableModels.
	Visibility string `json:"visibility,omitempty"`
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

	// Capabilities describes what the model supports — vision, audio, tool
	// calling, reasoning, etc. Surfaced in /v1/models so OpenAI-compatible
	// clients (Vercel AI SDK / OpenCode / Cursor / Continue.dev) can
	// auto-detect feature support without per-model client-side config. If
	// nil, capabilities are inferred from the model ID at marshal time.
	Capabilities *ModelCapabilities `json:"capabilities,omitempty"`

	// NativeTools declares provider-native tools the upstream model
	// supports without caller-side implementation (e.g. MiniMax M2.7's
	// autonomous `{"type":"web_search"}`). Agent runtimes that discover
	// `/v1/models` can merge these entries into their caller-declared
	// `tools` array so the model picks them on recent-events / browsing
	// queries without relying on server-side autoinject hacks. Empty /
	// absent means the model exposes no provider-native tools.
	NativeTools []NativeTool `json:"native_tools,omitempty"`
}

// NativeTool describes a provider-native tool a model supports out of
// the box. The shape deliberately mirrors the OpenAI tools[] entry a
// caller would declare (`{"type": "...", ...}`) so consumers can splice
// it directly into their own tools array at chat-completion time without
// translation. Params documents the tool's knobs for the operator; they
// are optional hints, not enforced by switchAILocal.
type NativeTool struct {
	// Type is the tool type the upstream model recognises (e.g.
	// "web_search"). Matches the "type" key of an OpenAI tools[] entry.
	Type string `json:"type" yaml:"type"`
	// Description is a short human-readable sentence for the operator /
	// client UI. Not forwarded to the model.
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	// Params documents the per-tool knobs (e.g. force_search,
	// max_keyword). Shape is intentionally loose (map[string]any) since
	// each provider defines its own parameter surface. Callers that want
	// to set a param append it alongside "type" on their tools[] entry.
	Params map[string]any `json:"params,omitempty" yaml:"params,omitempty"`
}

// ModelCapabilities describes the feature surface of a model in a shape
// compatible with what Vercel AI SDK and adjacent ecosystems look for in a
// /v1/models response. Populated from explicit config when present, or
// inferred from the model ID when nil (see InferCapabilities).
//
// The shape mirrors OpenCode's per-model schema so clients that read it
// directly can auto-enable attachment / tool-calling without users having
// to declare it in their local config — which was the bug that motivated
// this field (OpenCode silently strips images when attachment is false).
type ModelCapabilities struct {
	// Attachment reports whether the model accepts file attachments
	// (typically image/pdf passed as content-block image_url with a data:
	// URI). Required for Vercel AI SDK to forward image bytes.
	Attachment bool `json:"attachment"`
	// ToolCall reports whether the model can emit OpenAI-style function
	// calls (tools[] with type=function, returning tool_calls).
	ToolCall bool `json:"tool_call"`
	// Reasoning reports whether the model emits reasoning/thinking chunks
	// distinct from final-answer content (DeepSeek-R1-style or Gemini
	// thinking budget). Clients that render a chain-of-thought UI key off
	// this flag.
	Reasoning bool `json:"reasoning"`
	// Modalities enumerates supported input + output media types. Each
	// entry is one of: text, image, audio, video, pdf. Output is usually
	// just ["text"] but TTS / image-gen / music-gen models output audio
	// or image. A model that omits an entry is assumed not to support it.
	Modalities ModelModalities `json:"modalities"`
}

// ModelModalities lists the media types a model accepts on input and
// produces on output. Same vocabulary the OpenAI Realtime + Anthropic
// models endpoints use (text/image/audio/video/pdf).
type ModelModalities struct {
	Input  []string `json:"input"`
	Output []string `json:"output"`
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
	if model.Capabilities != nil {
		caps := *model.Capabilities
		caps.Modalities.Input = append([]string(nil), model.Capabilities.Modalities.Input...)
		caps.Modalities.Output = append([]string(nil), model.Capabilities.Modalities.Output...)
		copyModel.Capabilities = &caps
	}
	if len(model.NativeTools) > 0 {
		copyModel.NativeTools = cloneNativeTools(model.NativeTools)
	}
	return &copyModel
}

// cloneNativeTools returns a deep copy of the native-tools slice, with
// each NativeTool's Params map copied one level down. Values inside
// Params are retained as-is; operator config values are immutable at
// runtime so aliasing them is safe.
func cloneNativeTools(in []NativeTool) []NativeTool {
	if len(in) == 0 {
		return nil
	}
	out := make([]NativeTool, len(in))
	for i, t := range in {
		out[i] = NativeTool{Type: t.Type, Description: t.Description}
		if len(t.Params) > 0 {
			out[i].Params = make(map[string]any, len(t.Params))
			for k, v := range t.Params {
				out[i].Params[k] = v
			}
		}
	}
	return out
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
		if registration == nil || registration.Info == nil {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(registration.Info.Visibility), "private") {
			continue
		}
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
		// Capability metadata — explicit on the model wins; otherwise infer
		// from the ID. Vercel AI SDK / OpenCode / Cursor / Continue.dev all
		// look for these fields to decide whether to forward image/audio
		// content blocks. Without this, OpenCode silently strips images
		// because it can't tell the model supports vision.
		//
		// Emitted at TOP LEVEL (not nested under "capabilities") because:
		//   1. OpenAI clients look for `vision`, `attachment`, `modalities`
		//      directly on the model object;
		//   2. The legacy `capabilities` field is a string array
		//      ([\"text\",\"image\"]) maintained by sdk/api/handlers/openai/
		//      openai_handlers.go and must not be overwritten.
		if caps := resolveCapabilities(model); caps != nil {
			result["attachment"] = caps.Attachment
			result["tool_call"] = caps.ToolCall
			result["reasoning"] = caps.Reasoning
			result["modalities"] = caps.Modalities
			if containsString(caps.Modalities.Input, "image") {
				result["vision"] = true
			}
			if containsString(caps.Modalities.Input, "audio") {
				result["audio"] = true
			}
		}
		// native_tools: operator-declared provider-native tools the
		// upstream model supports (e.g. MiniMax M2.7's web_search).
		// Emitted only when non-empty so models that don't declare
		// any keep a clean /v1/models shape. Agent runtimes (OpenClaw,
		// Hermes, custom SDKs) discover these via /v1/models at
		// session init and splice them into their caller tools array.
		if len(model.NativeTools) > 0 {
			result["native_tools"] = nativeToolsToMaps(model.NativeTools)
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

// resolveCapabilities returns the model's explicit Capabilities if set, or
// derives a default from the model ID. Always returns a non-nil pointer
// when the model is known; nil when we have no signal at all (the caller
// then omits the field rather than emit an empty stub).
//
// Used by /v1/models so that clients which auto-discover capabilities
// (Vercel AI SDK / OpenCode / Continue.dev) get accurate metadata
// without operators having to declare it per-model in config.
func resolveCapabilities(model *ModelInfo) *ModelCapabilities {
	if model == nil {
		return nil
	}
	if model.Capabilities != nil {
		return model.Capabilities
	}
	return InferCapabilities(model.ID)
}

// InferCapabilities returns a best-effort capability set derived from a
// model identifier. Recognises common families across the providers we
// route to. Returns nil only for IDs we have no signal about — caller
// should treat nil as "unknown, don't surface in /v1/models".
//
// Maintenance: when a new vision/audio model lands, add a case here. The
// alternative — declaring caps in config — works for operators who want
// to override, but the inference path is what makes the gateway "just
// work" for clients on day one.
func InferCapabilities(id string) *ModelCapabilities {
	if id == "" {
		return nil
	}
	lower := strings.ToLower(id)

	// Helpers — vocabularies kept in one place to avoid typos.
	textIn := []string{"text"}
	textOut := []string{"text"}
	visionIn := []string{"text", "image"}
	visionAudioIn := []string{"text", "image", "audio"}
	audioOut := []string{"audio"}
	imageOut := []string{"image"}

	full := func(in, out []string, tools, reason bool) *ModelCapabilities {
		return &ModelCapabilities{
			Attachment: containsString(in, "image") || containsString(in, "pdf") || containsString(in, "audio"),
			ToolCall:   tools,
			Reasoning:  reason,
			Modalities: ModelModalities{Input: in, Output: out},
		}
	}

	// MiniMax family — M2.7 supports vision + audio input, text output.
	// Music + TTS + image gen are separate model IDs.
	if strings.Contains(lower, "minimax-m2") {
		return full(visionAudioIn, textOut, true, false)
	}
	if strings.Contains(lower, ":music-") || strings.Contains(lower, "/music-") || strings.HasSuffix(lower, ":music-2.6") || strings.HasSuffix(lower, ":music-cover") {
		return full(textIn, audioOut, false, false)
	}
	if strings.Contains(lower, ":speech-") || strings.HasSuffix(lower, "speech-02-hd") {
		return full(textIn, audioOut, false, false)
	}
	if strings.Contains(lower, ":image-") || strings.HasSuffix(lower, "image-01") {
		return full(textIn, imageOut, false, false)
	}

	// xiaomi mimo — omni handles vision+audio, others vary.
	if strings.Contains(lower, "mimo-v2-omni") {
		return full(visionAudioIn, textOut, true, false)
	}
	if strings.Contains(lower, "mimo-v2-tts") {
		return full(textIn, audioOut, false, false)
	}
	if strings.Contains(lower, "mimo-v2") { // pro / flash — text + tools
		return full(textIn, textOut, true, false)
	}

	// OpenAI family — modern GPTs are multimodal + tool-calling. The
	// reasoning models (o1/o3) are text-only with reasoning chunks.
	if strings.Contains(lower, "gpt-4o") || strings.Contains(lower, "gpt-5") || strings.Contains(lower, "gpt-4-vision") || strings.Contains(lower, "gpt-4-turbo") {
		return full(visionIn, textOut, true, false)
	}
	if strings.HasPrefix(lower, "o1") || strings.HasPrefix(lower, "o3") || strings.Contains(lower, "deepseek-reason") || strings.Contains(lower, "deepseek-r") {
		return full(textIn, textOut, false, true)
	}
	if strings.Contains(lower, "whisper") {
		// Whisper is audio-in, text-out (transcription).
		return full([]string{"audio"}, textOut, false, false)
	}
	if strings.Contains(lower, "tts") || strings.HasSuffix(lower, "speech") {
		return full(textIn, audioOut, false, false)
	}

	// Claude family — sonnet/opus/haiku from Claude 3.5+ all do vision.
	if strings.Contains(lower, "claude-3.5") || strings.Contains(lower, "claude-3-5") ||
		strings.Contains(lower, "claude-3.7") || strings.Contains(lower, "claude-3-7") ||
		strings.Contains(lower, "claude-4") || strings.Contains(lower, "claude-opus-4") ||
		strings.Contains(lower, "claude-sonnet-4") || strings.Contains(lower, "claude-haiku-4") {
		return full(visionIn, textOut, true, false)
	}
	if strings.Contains(lower, "claude-3-opus") || strings.Contains(lower, "claude-3-sonnet") || strings.Contains(lower, "claude-3-haiku") {
		return full(visionIn, textOut, true, false)
	}

	// Gemini family — 1.5+ are multimodal incl. video; we conservatively
	// claim image+audio (video plumbing isn't widely supported yet).
	if strings.Contains(lower, "gemini-1.5") || strings.Contains(lower, "gemini-2") || strings.Contains(lower, "gemini-pro") {
		return full(visionAudioIn, textOut, true, false)
	}

	// Embedding models — output a vector, not text. Surface that.
	if strings.Contains(lower, "embed") {
		return full(textIn, []string{"embedding"}, false, false)
	}

	// Qwen, llama, kimi, glm — broadly text+tools; vision variants opt-in.
	if strings.Contains(lower, "vl") || strings.Contains(lower, "vision") {
		return full(visionIn, textOut, true, false)
	}
	if strings.Contains(lower, "qwen") || strings.Contains(lower, "llama") || strings.Contains(lower, "kimi") || strings.Contains(lower, "glm") || strings.Contains(lower, "mistral") || strings.Contains(lower, "mercury") || strings.Contains(lower, "gpt-oss") {
		return full(textIn, textOut, true, false)
	}

	// Unknown — return text-only defaults rather than nil. False would
	// cause vision-aware clients to strip; an explicit text-only default
	// is at least honest and makes the response shape predictable.
	return full(textIn, textOut, false, false)
}

// containsString is a tiny helper to keep capability checks readable.
func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// nativeToolsToMaps renders NativeTools as []map[string]any so gin's
// JSON encoder emits the same lower_snake_case keys JSON tags declare
// ("type", "description", "params") regardless of whether the caller
// marshals the map via the explicit handler (convertModelToMap) or via
// the /v1/models filter path (which rebuilds a fresh map per entry).
// Keeps the shape deterministic across the two codepaths.
func nativeToolsToMaps(in []NativeTool) []map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(in))
	for _, t := range in {
		entry := map[string]any{"type": t.Type}
		if t.Description != "" {
			entry["description"] = t.Description
		}
		if len(t.Params) > 0 {
			entry["params"] = t.Params
		}
		out = append(out, entry)
	}
	return out
}
