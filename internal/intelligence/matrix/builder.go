// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package matrix provides dynamic capability matrix building for the intelligence system.
// It auto-assigns optimal models to capability slots based on discovered model capabilities.
package matrix

import (
	"sort"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/traylinx/switchAILocal/internal/intelligence/capability"
)

// CapabilitySlot represents a capability slot in the routing matrix.
type CapabilitySlot string

const (
	SlotCoding    CapabilitySlot = "coding"
	SlotReasoning CapabilitySlot = "reasoning"
	SlotCreative  CapabilitySlot = "creative"
	SlotFast      CapabilitySlot = "fast"
	SlotSecure    CapabilitySlot = "secure"
	SlotVision    CapabilitySlot = "vision"
)

// AllSlots returns all capability slots.
func AllSlots() []CapabilitySlot {
	return []CapabilitySlot{
		SlotCoding,
		SlotReasoning,
		SlotCreative,
		SlotFast,
		SlotSecure,
		SlotVision,
	}
}

// MatrixAssignment represents a model assignment for a capability slot.
type MatrixAssignment struct {
	Primary   string   `json:"primary"`
	Fallbacks []string `json:"fallbacks"`
	Score     float64  `json:"score"`
	Reason    string   `json:"reason"`
}

// DynamicMatrix represents the complete capability-to-model mapping.
type DynamicMatrix struct {
	Assignments map[CapabilitySlot]*MatrixAssignment `json:"assignments"`
	GeneratedAt time.Time                            `json:"generated_at"`
}

// ModelWithCapability combines a model ID with its analyzed capabilities.
type ModelWithCapability struct {
	ID           string
	Provider     string
	DisplayName  string
	Capabilities *capability.ModelCapability
}

// Builder builds dynamic capability matrices from discovered models.
type Builder struct {
	preferLocal      bool
	costOptimization bool
	overrides        map[string]string
	currentMatrix    *DynamicMatrix
	mu               sync.RWMutex
}

// NewBuilder creates a new DynamicMatrixBuilder instance.
//
// Parameters:
//   - preferLocal: Whether to prefer local models for the secure slot
//   - costOptimization: Whether to optimize for cost
//   - overrides: Manual overrides for specific slots
//
// Returns:
//   - *Builder: A new matrix builder instance
func NewBuilder(preferLocal, costOptimization bool, overrides map[string]string) *Builder {
	if overrides == nil {
		overrides = make(map[string]string)
	}
	return &Builder{
		preferLocal:      preferLocal,
		costOptimization: costOptimization,
		overrides:        overrides,
	}
}


// Build generates a dynamic matrix from the provided models with capabilities.
// It scores and ranks models for each capability slot and assigns primary models
// with fallback chains.
//
// Parameters:
//   - models: List of models with their analyzed capabilities
//
// Returns:
//   - *DynamicMatrix: The generated capability matrix
func (b *Builder) Build(models []*ModelWithCapability) *DynamicMatrix {
	b.mu.Lock()
	defer b.mu.Unlock()

	matrix := &DynamicMatrix{
		Assignments: make(map[CapabilitySlot]*MatrixAssignment),
		GeneratedAt: time.Now(),
	}

	// Build assignments for each slot
	for _, slot := range AllSlots() {
		assignment := b.buildSlotAssignment(slot, models)
		matrix.Assignments[slot] = assignment
	}

	// Apply manual overrides
	b.applyOverrides(matrix)

	b.currentMatrix = matrix
	log.Infof("Built dynamic matrix with %d slots", len(matrix.Assignments))

	return matrix
}

// buildSlotAssignment builds the assignment for a single capability slot.
func (b *Builder) buildSlotAssignment(slot CapabilitySlot, models []*ModelWithCapability) *MatrixAssignment {
	// Score all models for this slot
	type scoredModel struct {
		model *ModelWithCapability
		score float64
	}

	var scored []scoredModel
	for _, model := range models {
		if model.Capabilities == nil {
			continue
		}
		score := b.scoreModelForSlot(slot, model)
		if score > 0 {
			scored = append(scored, scoredModel{model: model, score: score})
		}
	}

	// Sort by score descending
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	// Build assignment
	assignment := &MatrixAssignment{
		Fallbacks: make([]string, 0),
	}

	if len(scored) == 0 {
		assignment.Reason = "no suitable models found"
		return assignment
	}

	// Primary is the highest scored model
	assignment.Primary = scored[0].model.ID
	assignment.Score = scored[0].score
	assignment.Reason = b.getAssignmentReason(slot, scored[0].model)

	// Fallbacks are the next best models (up to 3)
	maxFallbacks := 3
	for i := 1; i < len(scored) && i <= maxFallbacks; i++ {
		assignment.Fallbacks = append(assignment.Fallbacks, scored[i].model.ID)
	}

	log.Debugf("Slot %s: primary=%s (score=%.2f), fallbacks=%v",
		slot, assignment.Primary, assignment.Score, assignment.Fallbacks)

	return assignment
}

// scoreModelForSlot calculates a score for a model's suitability for a slot.
// Higher scores indicate better suitability.
func (b *Builder) scoreModelForSlot(slot CapabilitySlot, model *ModelWithCapability) float64 {
	cap := model.Capabilities
	if cap == nil {
		return 0
	}

	var score float64

	switch slot {
	case SlotCoding:
		score = b.scoreCoding(cap)
	case SlotReasoning:
		score = b.scoreReasoning(cap)
	case SlotCreative:
		score = b.scoreCreative(cap)
	case SlotFast:
		score = b.scoreFast(cap)
	case SlotSecure:
		score = b.scoreSecure(cap)
	case SlotVision:
		score = b.scoreVision(cap)
	default:
		score = 0.5 // Default score for unknown slots
	}

	// Apply cost optimization modifier
	if b.costOptimization {
		score = b.applyCostModifier(score, cap)
	}

	return score
}

// scoreCoding scores a model for the coding slot.
// Prioritizes: coding capability, context window, latency
func (b *Builder) scoreCoding(cap *capability.ModelCapability) float64 {
	var score float64

	// Primary: coding capability (50% weight)
	if cap.SupportsCoding {
		score += 0.5
	}

	// Secondary: context window (30% weight)
	// Larger context is better for coding
	contextScore := float64(cap.ContextWindow) / 200000.0 // Normalize to 200k max
	if contextScore > 1.0 {
		contextScore = 1.0
	}
	score += contextScore * 0.3

	// Tertiary: latency (20% weight)
	// Standard latency is acceptable for coding
	switch cap.EstimatedLatency {
	case "fast":
		score += 0.15
	case "standard":
		score += 0.2
	case "slow":
		score += 0.1
	}

	return score
}

// scoreReasoning scores a model for the reasoning slot.
// Prioritizes: reasoning capability, accuracy over speed
func (b *Builder) scoreReasoning(cap *capability.ModelCapability) float64 {
	var score float64

	// Primary: reasoning capability (60% weight)
	if cap.SupportsReasoning {
		score += 0.6
	}

	// Secondary: context window (25% weight)
	contextScore := float64(cap.ContextWindow) / 200000.0
	if contextScore > 1.0 {
		contextScore = 1.0
	}
	score += contextScore * 0.25

	// Tertiary: we prefer slower models for reasoning (they're usually more capable)
	switch cap.EstimatedLatency {
	case "slow":
		score += 0.15
	case "standard":
		score += 0.1
	case "fast":
		score += 0.05
	}

	return score
}

// scoreCreative scores a model for the creative slot.
// Prioritizes: general capability, context window
func (b *Builder) scoreCreative(cap *capability.ModelCapability) float64 {
	var score float64

	// Creative tasks benefit from larger context and general capability
	// No specific capability flag, so we use context and cost tier as proxies

	// Context window (40% weight)
	contextScore := float64(cap.ContextWindow) / 200000.0
	if contextScore > 1.0 {
		contextScore = 1.0
	}
	score += contextScore * 0.4

	// Cost tier as proxy for capability (40% weight)
	switch cap.CostTier {
	case "high":
		score += 0.4
	case "medium":
		score += 0.3
	case "low":
		score += 0.2
	case "free":
		score += 0.15
	}

	// Standard latency is fine (20% weight)
	switch cap.EstimatedLatency {
	case "standard":
		score += 0.2
	case "fast":
		score += 0.15
	case "slow":
		score += 0.1
	}

	return score
}


// scoreFast scores a model for the fast slot.
// Prioritizes: low latency, low cost
func (b *Builder) scoreFast(cap *capability.ModelCapability) float64 {
	var score float64

	// Primary: latency (60% weight)
	switch cap.EstimatedLatency {
	case "fast":
		score += 0.6
	case "standard":
		score += 0.3
	case "slow":
		score += 0.1
	}

	// Secondary: cost (30% weight)
	switch cap.CostTier {
	case "free":
		score += 0.3
	case "low":
		score += 0.25
	case "medium":
		score += 0.15
	case "high":
		score += 0.05
	}

	// Tertiary: local models are often faster (10% weight)
	if cap.IsLocal {
		score += 0.1
	}

	return score
}

// scoreSecure scores a model for the secure slot.
// Prioritizes: local models (data stays on-premise)
func (b *Builder) scoreSecure(cap *capability.ModelCapability) float64 {
	var score float64

	// Primary: local models strongly preferred (70% weight)
	if cap.IsLocal {
		score += 0.7
	}

	// If preferLocal is enabled, boost local models even more
	if b.preferLocal && cap.IsLocal {
		score += 0.2
	}

	// Secondary: general capability (remaining weight)
	// Context window as proxy
	contextScore := float64(cap.ContextWindow) / 200000.0
	if contextScore > 1.0 {
		contextScore = 1.0
	}
	
	if cap.IsLocal {
		score += contextScore * 0.1
	} else {
		// Non-local models get a much lower base score
		score += contextScore * 0.3
	}

	return score
}

// scoreVision scores a model for the vision slot.
// Prioritizes: vision capability
func (b *Builder) scoreVision(cap *capability.ModelCapability) float64 {
	var score float64

	// Primary: vision capability (70% weight)
	if cap.SupportsVision {
		score += 0.7
	} else {
		// No vision support = not suitable
		return 0
	}

	// Secondary: context window (20% weight)
	contextScore := float64(cap.ContextWindow) / 200000.0
	if contextScore > 1.0 {
		contextScore = 1.0
	}
	score += contextScore * 0.2

	// Tertiary: latency (10% weight)
	switch cap.EstimatedLatency {
	case "fast":
		score += 0.1
	case "standard":
		score += 0.08
	case "slow":
		score += 0.05
	}

	return score
}

// applyCostModifier adjusts the score based on cost optimization settings.
func (b *Builder) applyCostModifier(score float64, cap *capability.ModelCapability) float64 {
	// Apply a multiplier based on cost tier
	switch cap.CostTier {
	case "free":
		return score * 1.2 // 20% boost for free models
	case "low":
		return score * 1.1 // 10% boost for low cost
	case "medium":
		return score // No change
	case "high":
		return score * 0.9 // 10% penalty for high cost
	}
	return score
}

// getAssignmentReason generates a human-readable reason for the assignment.
func (b *Builder) getAssignmentReason(slot CapabilitySlot, model *ModelWithCapability) string {
	cap := model.Capabilities
	if cap == nil {
		return "default assignment"
	}

	switch slot {
	case SlotCoding:
		if cap.SupportsCoding {
			return "coding-optimized model"
		}
		return "general model with good context"
	case SlotReasoning:
		if cap.SupportsReasoning {
			return "reasoning-optimized model"
		}
		return "high-capability model"
	case SlotCreative:
		return "general-purpose model"
	case SlotFast:
		if cap.EstimatedLatency == "fast" {
			return "low-latency model"
		}
		return "efficient model"
	case SlotSecure:
		if cap.IsLocal {
			return "local model (data stays on-premise)"
		}
		return "fallback to cloud model"
	case SlotVision:
		if cap.SupportsVision {
			return "vision-capable model"
		}
		return "no vision support"
	}
	return "auto-assigned"
}

// applyOverrides applies manual overrides from configuration.
func (b *Builder) applyOverrides(matrix *DynamicMatrix) {
	for slotStr, modelID := range b.overrides {
		slot := CapabilitySlot(slotStr)
		if assignment, exists := matrix.Assignments[slot]; exists {
			log.Infof("Applying override for slot %s: %s -> %s", slot, assignment.Primary, modelID)
			// Move current primary to fallbacks if it's different
			if assignment.Primary != "" && assignment.Primary != modelID {
				assignment.Fallbacks = append([]string{assignment.Primary}, assignment.Fallbacks...)
			}
			assignment.Primary = modelID
			assignment.Reason = "manual override"
		} else {
			// Create new assignment for override
			matrix.Assignments[slot] = &MatrixAssignment{
				Primary:   modelID,
				Fallbacks: []string{},
				Score:     1.0,
				Reason:    "manual override",
			}
		}
	}
}

// GetCurrentMatrix returns the most recently built matrix.
// Returns nil if no matrix has been built yet.
//
// Returns:
//   - *DynamicMatrix: The current matrix, or nil
func (b *Builder) GetCurrentMatrix() *DynamicMatrix {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.currentMatrix
}

// GetCurrentMatrixAsMap returns the current matrix as a map for Lua interop.
//
// Returns:
//   - map[string]interface{}: The matrix as a map, or nil
func (b *Builder) GetCurrentMatrixAsMap() map[string]interface{} {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.currentMatrix == nil {
		return nil
	}

	result := make(map[string]interface{})
	for slot, assignment := range b.currentMatrix.Assignments {
		result[string(slot)] = map[string]interface{}{
			"primary":   assignment.Primary,
			"fallbacks": assignment.Fallbacks,
			"score":     assignment.Score,
			"reason":    assignment.Reason,
		}
	}
	return result
}

// SetOverrides updates the manual overrides.
//
// Parameters:
//   - overrides: New override mappings
func (b *Builder) SetOverrides(overrides map[string]string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if overrides == nil {
		overrides = make(map[string]string)
	}
	b.overrides = overrides
}

// SetPreferLocal updates the prefer local setting.
//
// Parameters:
//   - preferLocal: Whether to prefer local models
func (b *Builder) SetPreferLocal(preferLocal bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.preferLocal = preferLocal
}

// SetCostOptimization updates the cost optimization setting.
//
// Parameters:
//   - costOptimization: Whether to optimize for cost
func (b *Builder) SetCostOptimization(costOptimization bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.costOptimization = costOptimization
}
