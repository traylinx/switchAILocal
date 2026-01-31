// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package management

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/traylinx/switchAILocal/internal/intelligence"
)

// SkillsResponse represents the response for the skills endpoint.
type SkillsResponse struct {
	// Count is the total number of loaded skills
	Count int `json:"count"`

	// Skills contains metadata for all loaded skills
	Skills []SkillMetadata `json:"skills"`

	// EmbeddingsAvailable indicates whether embeddings have been computed
	EmbeddingsAvailable bool `json:"embeddings_available"`

	// UsageStats contains usage statistics for each skill
	UsageStats map[string]int64 `json:"usage_stats,omitempty"`
}

// SkillMetadata represents metadata for a single skill.
type SkillMetadata struct {
	// ID is the unique identifier for the skill
	ID string `json:"id"`

	// Name is the human-readable name
	Name string `json:"name"`

	// Description explains what the skill does
	Description string `json:"description"`

	// RequiredCapability specifies which capability slot this skill needs
	RequiredCapability string `json:"required_capability"`

	// HasEmbedding indicates whether an embedding has been computed for this skill
	HasEmbedding bool `json:"has_embedding"`
}

// SkillsHandler handles the /v0/management/skills endpoint.
// It requires an intelligence service to be provided.
type SkillsHandler struct {
	intelligenceService *intelligence.Service
}

// NewSkillsHandler creates a new skills handler.
//
// Parameters:
//   - intelligenceService: The intelligence service instance
//
// Returns:
//   - *SkillsHandler: A new handler instance
func NewSkillsHandler(intelligenceService *intelligence.Service) *SkillsHandler {
	return &SkillsHandler{
		intelligenceService: intelligenceService,
	}
}

// GetSkills handles GET /v0/management/skills
// Returns skill count, metadata, and usage statistics.
//
// Response:
//   - 200: Success with skill metadata
//   - 503: Intelligence services not enabled
func (h *SkillsHandler) GetSkills(c *gin.Context) {
	// Check if intelligence service is enabled
	if h.intelligenceService == nil || !h.intelligenceService.IsEnabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "intelligence services not enabled",
		})
		return
	}

	// Get skill registry
	registry := h.intelligenceService.GetSkillRegistry()
	if registry == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "skill registry not available",
		})
		return
	}

	// Get all skills
	allSkills := registry.GetAllSkills()

	// Build metadata
	skillMetadata := make([]SkillMetadata, len(allSkills))
	for i, skill := range allSkills {
		skillMetadata[i] = SkillMetadata{
			ID:                 skill.GetID(),
			Name:               skill.GetName(),
			Description:        skill.GetDescription(),
			RequiredCapability: skill.GetRequiredCapability(),
			HasEmbedding:       skill.GetEmbeddingLength() > 0,
		}
	}

	// Build response
	response := SkillsResponse{
		Count:               registry.GetSkillCount(),
		Skills:              skillMetadata,
		EmbeddingsAvailable: registry.HasEmbeddings(),
		UsageStats:          registry.GetUsageStats(),
	}

	c.JSON(http.StatusOK, response)
}
