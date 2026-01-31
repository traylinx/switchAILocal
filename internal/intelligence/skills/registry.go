// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package skills provides an enhanced skill registry for Phase 2 intelligent routing.
// It loads skills from SKILL.md files and supports embedding-based semantic matching.
package skills

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/goccy/go-yaml"
	log "github.com/sirupsen/logrus"
)

// Skill represents a domain-specific expertise definition with system prompts
// and model requirements. Skills are loaded from SKILL.md files.
type Skill struct {
	// ID is the unique identifier for the skill (derived from directory name)
	ID string `json:"id" yaml:"-"`

	// Name is the human-readable name of the skill
	Name string `json:"name" yaml:"name"`

	// Description explains what the skill does
	Description string `json:"description" yaml:"description"`

	// RequiredCapability specifies which capability slot this skill needs
	RequiredCapability string `json:"required_capability" yaml:"required-capability"`

	// SystemPrompt is the full content of the SKILL.md file
	SystemPrompt string `json:"system_prompt" yaml:"-"`

	// Embedding is the pre-computed embedding vector for semantic matching
	// This is populated when the embedding engine is available
	Embedding []float32 `json:"-" yaml:"-"`
}

// GetID returns the skill ID.
func (s *Skill) GetID() string {
	return s.ID
}

// GetName returns the skill name.
func (s *Skill) GetName() string {
	return s.Name
}

// GetDescription returns the skill description.
func (s *Skill) GetDescription() string {
	return s.Description
}

// GetRequiredCapability returns the required capability.
func (s *Skill) GetRequiredCapability() string {
	return s.RequiredCapability
}

// GetSystemPrompt returns the system prompt.
func (s *Skill) GetSystemPrompt() string {
	return s.SystemPrompt
}

// GetEmbeddingLength returns the length of the embedding vector.
func (s *Skill) GetEmbeddingLength() int {
	return len(s.Embedding)
}

// SkillMatchResult represents the result of matching a query to a skill.
type SkillMatchResult struct {
	// Skill is the matched skill
	Skill *Skill `json:"skill"`

	// Confidence is the similarity score (0.0-1.0)
	Confidence float64 `json:"confidence"`
}

// SkillInfo is an interface for accessing skill information.
// This allows skills to be used through interfaces in other packages.
type SkillInfo interface {
	GetID() string
	GetName() string
	GetDescription() string
	GetRequiredCapability() string
	GetSystemPrompt() string
	GetEmbeddingLength() int
}

// EmbeddingEngine defines the interface for computing embeddings.
// This allows the skill registry to work with or without the embedding engine.
type EmbeddingEngine interface {
	// Embed computes the embedding vector for a text
	Embed(text string) ([]float32, error)

	// CosineSimilarity computes the cosine similarity between two vectors
	CosineSimilarity(a, b []float32) float64
}

// Registry manages the collection of skills and provides semantic matching.
// It reuses the existing SKILL.md parsing logic from lua_engine.go.
type Registry struct {
	// skills holds all loaded skills indexed by ID
	skills map[string]*Skill

	// skillsList holds all skills in a slice for iteration
	skillsList []*Skill

	// engine is the embedding engine for semantic matching (optional)
	engine EmbeddingEngine

	// threshold is the minimum confidence score for skill matching
	threshold float64

	// mu protects concurrent access to the registry
	mu sync.RWMutex

	// usageCount tracks how many times each skill has been matched
	usageCount map[string]int64
}

// NewRegistry creates a new skill registry.
//
// Parameters:
//   - threshold: Minimum confidence score for skill matching (default: 0.80)
//
// Returns:
//   - *Registry: A new registry instance
func NewRegistry(threshold float64) *Registry {
	if threshold <= 0 {
		threshold = 0.80 // Default threshold
	}

	return &Registry{
		skills:     make(map[string]*Skill),
		skillsList: make([]*Skill, 0),
		threshold:  threshold,
		usageCount: make(map[string]int64),
	}
}

// SetEmbeddingEngine sets the embedding engine for semantic matching.
// This should be called before LoadAll() to enable embedding computation.
//
// Parameters:
//   - engine: The embedding engine instance
func (r *Registry) SetEmbeddingEngine(engine EmbeddingEngine) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.engine = engine
}

// LoadAll loads all skills from the specified directory.
// It walks the directory tree and parses all SKILL.md files.
// This reuses the parsing logic from lua_engine.go.
//
// Parameters:
//   - skillsDir: Path to the skills directory
//
// Returns:
//   - error: Any error encountered during loading
func (r *Registry) LoadAll(skillsDir string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if skillsDir == "" {
		return fmt.Errorf("skills directory not specified")
	}

	// Check if directory exists
	if _, err := os.Stat(skillsDir); os.IsNotExist(err) {
		return fmt.Errorf("skills directory does not exist: %s", skillsDir)
	}

	log.Infof("Loading skills from %s...", skillsDir)

	var loadedSkills []*Skill
	count := 0

	// Walk the directory tree looking for SKILL.md files
	err := filepath.WalkDir(skillsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip if not a SKILL.md file
		if d.IsDir() || !strings.EqualFold(d.Name(), "SKILL.md") {
			return nil
		}

		// Read the file content
		content, err := os.ReadFile(path)
		if err != nil {
			log.Warnf("Failed to read SKILL.md at %s: %v", path, err)
			return nil // Continue with other files
		}

		// Parse the YAML frontmatter
		// Format: ---\nYAML\n---\nContent
		parts := strings.SplitN(string(content), "---", 3)
		if len(parts) < 3 {
			log.Warnf("Invalid SKILL.md format at %s (missing frontmatter)", path)
			return nil
		}

		// Parse YAML frontmatter
		var skill Skill
		if err := yaml.Unmarshal([]byte(parts[1]), &skill); err != nil {
			log.Warnf("Failed to parse SKILL.md frontmatter at %s: %v", path, err)
			return nil
		}

		// Set ID from directory name if not specified
		if skill.Name == "" {
			skill.Name = filepath.Base(filepath.Dir(path))
		}
		skill.ID = filepath.Base(filepath.Dir(path))

		// Store the full content as system prompt
		skill.SystemPrompt = string(content)

		loadedSkills = append(loadedSkills, &skill)
		count++

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to walk skills directory: %w", err)
	}

	// Store loaded skills
	r.skills = make(map[string]*Skill)
	r.skillsList = loadedSkills
	for _, skill := range loadedSkills {
		r.skills[skill.ID] = skill
	}

	log.Infof("Loaded %d skills", count)

	// Compute embeddings if engine is available
	if r.engine != nil {
		log.Info("Computing embeddings for skills...")
		if err := r.computeEmbeddings(); err != nil {
			log.Warnf("Failed to compute embeddings: %v", err)
		} else {
			log.Infof("Computed embeddings for %d skills", count)
		}
	}

	return nil
}

// computeEmbeddings pre-computes embeddings for all skill descriptions.
// This is called internally by LoadAll() when an embedding engine is available.
// Must be called with lock held.
func (r *Registry) computeEmbeddings() error {
	if r.engine == nil {
		return fmt.Errorf("embedding engine not available")
	}

	for _, skill := range r.skillsList {
		// Use description for embedding (more concise than full content)
		embedding, err := r.engine.Embed(skill.Description)
		if err != nil {
			log.Warnf("Failed to compute embedding for skill %s: %v", skill.ID, err)
			continue
		}
		skill.Embedding = embedding
	}

	return nil
}

// GetSkill retrieves a skill by its ID.
//
// Parameters:
//   - id: The skill ID
//
// Returns:
//   - *Skill: The skill, or nil if not found
//   - error: Any error encountered
func (r *Registry) GetSkill(id string) (*Skill, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	skill, ok := r.skills[id]
	if !ok {
		return nil, fmt.Errorf("skill not found: %s", id)
	}

	return skill, nil
}

// GetAllSkills returns all loaded skills as an interface slice.
//
// Returns:
//   - []SkillInfo: A slice of all skills as interfaces
func (r *Registry) GetAllSkills() []SkillInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Return skills as interface slice
	result := make([]SkillInfo, len(r.skillsList))
	for i, skill := range r.skillsList {
		result[i] = skill
	}
	return result
}

// GetAllSkillsAsPointers returns all loaded skills as pointers.
// This is used internally when concrete types are needed.
//
// Returns:
//   - []*Skill: A slice of all skills
func (r *Registry) GetAllSkillsAsPointers() []*Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Return a copy to prevent external modification
	result := make([]*Skill, len(r.skillsList))
	copy(result, r.skillsList)
	return result
}

// MatchSkill finds the best matching skill for a query embedding.
// Returns nil if no skill matches above the confidence threshold.
//
// Parameters:
//   - queryEmbedding: The embedding vector of the query
//
// Returns:
//   - *SkillMatchResult: The match result, or nil if no match
//   - error: Any error encountered
func (r *Registry) MatchSkill(queryEmbedding []float32) (*SkillMatchResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.engine == nil {
		return nil, fmt.Errorf("embedding engine not available")
	}

	if len(queryEmbedding) == 0 {
		return nil, fmt.Errorf("query embedding is empty")
	}

	var bestSkill *Skill
	var bestScore float64

	// Find the skill with highest similarity
	for _, skill := range r.skillsList {
		if len(skill.Embedding) == 0 {
			continue // Skip skills without embeddings
		}

		similarity := r.engine.CosineSimilarity(queryEmbedding, skill.Embedding)
		if similarity > bestScore {
			bestScore = similarity
			bestSkill = skill
		}
	}

	// Check if best match exceeds threshold
	if bestSkill == nil || bestScore < r.threshold {
		return nil, nil // No match above threshold
	}

	// Track usage
	r.mu.RUnlock()
	r.mu.Lock()
	r.usageCount[bestSkill.ID]++
	r.mu.Unlock()
	r.mu.RLock()

	return &SkillMatchResult{
		Skill:      bestSkill,
		Confidence: bestScore,
	}, nil
}

// GetSkillCount returns the total number of loaded skills.
//
// Returns:
//   - int: The number of skills
func (r *Registry) GetSkillCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.skillsList)
}

// GetUsageStats returns usage statistics for all skills.
//
// Returns:
//   - map[string]int64: Map of skill ID to usage count
func (r *Registry) GetUsageStats() map[string]int64 {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Return a copy to prevent external modification
	result := make(map[string]int64)
	for k, v := range r.usageCount {
		result[k] = v
	}
	return result
}

// HasEmbeddings returns whether embeddings have been computed for skills.
//
// Returns:
//   - bool: true if embeddings are available
func (r *Registry) HasEmbeddings() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.skillsList) == 0 {
		return false
	}

	// Check if at least one skill has an embedding
	for _, skill := range r.skillsList {
		if len(skill.Embedding) > 0 {
			return true
		}
	}

	return false
}
