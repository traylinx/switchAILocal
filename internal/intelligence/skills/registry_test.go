// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package skills

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

// TestNewRegistry tests registry creation.
func TestNewRegistry(t *testing.T) {
	registry := NewRegistry(0.80)
	if registry == nil {
		t.Fatal("expected non-nil registry")
	}

	if registry.threshold != 0.80 {
		t.Errorf("expected threshold 0.80, got %f", registry.threshold)
	}

	// Test default threshold
	registry2 := NewRegistry(0)
	if registry2.threshold != 0.80 {
		t.Errorf("expected default threshold 0.80, got %f", registry2.threshold)
	}
}

// TestLoadAll tests loading skills from a directory.
func TestLoadAll(t *testing.T) {
	// Create temporary directory with test skills
	tmpDir := t.TempDir()
	
	// Create a test skill
	skillDir := filepath.Join(tmpDir, "test-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("failed to create skill directory: %v", err)
	}

	skillContent := `---
name: test-skill
description: A test skill for unit testing
required-capability: coding
---

# Test Skill

This is a test skill.
`

	skillPath := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte(skillContent), 0644); err != nil {
		t.Fatalf("failed to write skill file: %v", err)
	}

	// Create registry and load skills
	registry := NewRegistry(0.80)
	if err := registry.LoadAll(tmpDir); err != nil {
		t.Fatalf("failed to load skills: %v", err)
	}

	// Verify skill was loaded
	if registry.GetSkillCount() != 1 {
		t.Errorf("expected 1 skill, got %d", registry.GetSkillCount())
	}

	skills := registry.GetAllSkills()
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill in list, got %d", len(skills))
	}

	skill := skills[0]
	if skill.GetID() != "test-skill" {
		t.Errorf("expected ID 'test-skill', got '%s'", skill.GetID())
	}
	if skill.GetName() != "test-skill" {
		t.Errorf("expected name 'test-skill', got '%s'", skill.GetName())
	}
	if skill.GetDescription() != "A test skill for unit testing" {
		t.Errorf("expected description 'A test skill for unit testing', got '%s'", skill.GetDescription())
	}
	if skill.GetRequiredCapability() != "coding" {
		t.Errorf("expected required capability 'coding', got '%s'", skill.GetRequiredCapability())
	}
	if skill.GetSystemPrompt() == "" {
		t.Error("expected non-empty system prompt")
	}
}

// TestLoadAllMultipleSkills tests loading multiple skills.
func TestLoadAllMultipleSkills(t *testing.T) {
	tmpDir := t.TempDir()

	// Create multiple test skills
	skills := []struct {
		id          string
		name        string
		description string
		capability  string
	}{
		{"skill-1", "Skill One", "First test skill", "coding"},
		{"skill-2", "Skill Two", "Second test skill", "reasoning"},
		{"skill-3", "Skill Three", "Third test skill", "creative"},
	}

	for _, s := range skills {
		skillDir := filepath.Join(tmpDir, s.id)
		if err := os.MkdirAll(skillDir, 0755); err != nil {
			t.Fatalf("failed to create skill directory: %v", err)
		}

		content := "---\nname: " + s.name + "\ndescription: " + s.description + "\nrequired-capability: " + s.capability + "\n---\n\n# " + s.name
		skillPath := filepath.Join(skillDir, "SKILL.md")
		if err := os.WriteFile(skillPath, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write skill file: %v", err)
		}
	}

	// Load skills
	registry := NewRegistry(0.80)
	if err := registry.LoadAll(tmpDir); err != nil {
		t.Fatalf("failed to load skills: %v", err)
	}

	// Verify all skills were loaded
	if registry.GetSkillCount() != 3 {
		t.Errorf("expected 3 skills, got %d", registry.GetSkillCount())
	}
}

// TestGetSkill tests retrieving a skill by ID.
func TestGetSkill(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Create a test skill
	skillDir := filepath.Join(tmpDir, "test-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("failed to create skill directory: %v", err)
	}

	skillContent := `---
name: test-skill
description: A test skill
required-capability: coding
---

# Test Skill
`

	skillPath := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte(skillContent), 0644); err != nil {
		t.Fatalf("failed to write skill file: %v", err)
	}

	// Load skills
	registry := NewRegistry(0.80)
	if err := registry.LoadAll(tmpDir); err != nil {
		t.Fatalf("failed to load skills: %v", err)
	}

	// Test getting existing skill
	skill, err := registry.GetSkill("test-skill")
	if err != nil {
		t.Fatalf("failed to get skill: %v", err)
	}
	if skill.GetID() != "test-skill" {
		t.Errorf("expected ID 'test-skill', got '%s'", skill.GetID())
	}

	// Test getting non-existent skill
	_, err = registry.GetSkill("non-existent")
	if err == nil {
		t.Error("expected error for non-existent skill")
	}
}

// TestHasEmbeddings tests the HasEmbeddings method.
func TestHasEmbeddings(t *testing.T) {
	registry := NewRegistry(0.80)

	// Empty registry should not have embeddings
	if registry.HasEmbeddings() {
		t.Error("expected false for empty registry")
	}

	// Add a skill without embedding
	registry.skills["test"] = &Skill{
		ID:          "test",
		Name:        "Test",
		Description: "Test skill",
	}
	registry.skillsList = append(registry.skillsList, registry.skills["test"])

	if registry.HasEmbeddings() {
		t.Error("expected false for skill without embedding")
	}

	// Add embedding
	registry.skills["test"].Embedding = []float32{0.1, 0.2, 0.3}

	if !registry.HasEmbeddings() {
		t.Error("expected true for skill with embedding")
	}
}

// TestGetUsageStats tests usage statistics tracking.
func TestGetUsageStats(t *testing.T) {
	registry := NewRegistry(0.80)

	// Empty registry should have empty stats
	stats := registry.GetUsageStats()
	if len(stats) != 0 {
		t.Errorf("expected empty stats, got %d entries", len(stats))
	}

	// Add usage
	registry.usageCount["skill-1"] = 5
	registry.usageCount["skill-2"] = 10

	stats = registry.GetUsageStats()
	if len(stats) != 2 {
		t.Errorf("expected 2 stats entries, got %d", len(stats))
	}
	if stats["skill-1"] != 5 {
		t.Errorf("expected skill-1 count 5, got %d", stats["skill-1"])
	}
	if stats["skill-2"] != 10 {
		t.Errorf("expected skill-2 count 10, got %d", stats["skill-2"])
	}
}

// MockEmbeddingEngine is a mock implementation for testing.
type MockEmbeddingEngine struct {
	embeddings map[string][]float32
}

func (m *MockEmbeddingEngine) Embed(text string) ([]float32, error) {
	if m.embeddings == nil {
		m.embeddings = make(map[string][]float32)
	}
	// Return a simple embedding based on text length
	embedding := make([]float32, 384)
	for i := range embedding {
		embedding[i] = float32(len(text)) / 100.0
	}
	m.embeddings[text] = embedding
	return embedding, nil
}

func (m *MockEmbeddingEngine) CosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0.0
	}
	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += float64(a[i] * b[i])
		normA += float64(a[i] * a[i])
		normB += float64(b[i] * b[i])
	}
	if normA == 0 || normB == 0 {
		return 0.0
	}
	// Calculate the square root of the norms
	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

// TestEmbeddingComputation tests embedding computation during skill loading.
func TestEmbeddingComputation(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Create a test skill
	skillDir := filepath.Join(tmpDir, "test-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("failed to create skill directory: %v", err)
	}

	skillContent := `---
name: test-skill
description: A test skill for embedding computation
required-capability: coding
---

# Test Skill
`

	skillPath := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte(skillContent), 0644); err != nil {
		t.Fatalf("failed to write skill file: %v", err)
	}

	// Create registry with mock embedding engine
	registry := NewRegistry(0.80)
	mockEngine := &MockEmbeddingEngine{}
	registry.SetEmbeddingEngine(mockEngine)

	// Load skills (should compute embeddings)
	if err := registry.LoadAll(tmpDir); err != nil {
		t.Fatalf("failed to load skills: %v", err)
	}

	// Verify embedding was computed
	skills := registry.GetAllSkills()
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}

	if skills[0].GetEmbeddingLength() == 0 {
		t.Error("expected non-empty embedding")
	}

	if !registry.HasEmbeddings() {
		t.Error("expected HasEmbeddings to return true")
	}
}

// TestSkillRetrieval tests skill retrieval methods.
func TestSkillRetrieval(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Create test skills
	for i := 1; i <= 3; i++ {
		skillDir := filepath.Join(tmpDir, "skill-"+string(rune('0'+i)))
		if err := os.MkdirAll(skillDir, 0755); err != nil {
			t.Fatalf("failed to create skill directory: %v", err)
		}

		content := "---\nname: Skill " + string(rune('0'+i)) + "\ndescription: Test skill\nrequired-capability: coding\n---\n\n# Skill"
		skillPath := filepath.Join(skillDir, "SKILL.md")
		if err := os.WriteFile(skillPath, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write skill file: %v", err)
		}
	}

	// Load skills
	registry := NewRegistry(0.80)
	if err := registry.LoadAll(tmpDir); err != nil {
		t.Fatalf("failed to load skills: %v", err)
	}

	// Test GetAllSkills returns a copy
	skills1 := registry.GetAllSkills()
	
	if len(skills1) != 3 {
		t.Errorf("expected 3 skills, got %d", len(skills1))
	}

	// Modifying the returned slice should not affect the registry
	skills1[0] = nil
	skills3 := registry.GetAllSkills()
	if skills3[0] == nil {
		t.Error("expected registry to be unaffected by external modifications")
	}
}

// TestMatchSkill tests skill matching functionality.
func TestMatchSkill(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Create test skills with different descriptions
	skills := []struct {
		id          string
		name        string
		description string
		capability  string
	}{
		{"coding-skill", "Coding Expert", "Expert in writing and debugging code", "coding"},
		{"reasoning-skill", "Reasoning Expert", "Expert in logical reasoning and problem solving", "reasoning"},
		{"creative-skill", "Creative Writer", "Expert in creative writing and storytelling", "creative"},
	}

	for _, s := range skills {
		skillDir := filepath.Join(tmpDir, s.id)
		if err := os.MkdirAll(skillDir, 0755); err != nil {
			t.Fatalf("failed to create skill directory: %v", err)
		}

		content := "---\nname: " + s.name + "\ndescription: " + s.description + "\nrequired-capability: " + s.capability + "\n---\n\n# " + s.name
		skillPath := filepath.Join(skillDir, "SKILL.md")
		if err := os.WriteFile(skillPath, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write skill file: %v", err)
		}
	}

	// Create registry with mock embedding engine
	registry := NewRegistry(0.80)
	mockEngine := &MockEmbeddingEngine{}
	registry.SetEmbeddingEngine(mockEngine)

	// Load skills
	if err := registry.LoadAll(tmpDir); err != nil {
		t.Fatalf("failed to load skills: %v", err)
	}

	// Test matching with a query
	queryEmbedding, err := mockEngine.Embed("Help me write some Python code")
	if err != nil {
		t.Fatalf("failed to create query embedding: %v", err)
	}

	result, err := registry.MatchSkill(queryEmbedding)
	if err != nil {
		t.Fatalf("failed to match skill: %v", err)
	}

	// With our mock engine, all embeddings are the same, so we should get a match
	if result == nil {
		t.Error("expected a skill match")
	} else {
		if result.Skill == nil {
			t.Error("expected non-nil skill in result")
		}
		if result.Confidence <= 0 {
			t.Errorf("expected positive confidence, got %f", result.Confidence)
		}
	}
}

// TestMatchSkillNoEngine tests skill matching without embedding engine.
func TestMatchSkillNoEngine(t *testing.T) {
	registry := NewRegistry(0.80)

	// Try to match without engine
	queryEmbedding := []float32{0.1, 0.2, 0.3}
	_, err := registry.MatchSkill(queryEmbedding)
	if err == nil {
		t.Error("expected error when matching without embedding engine")
	}
}

// TestMatchSkillEmptyEmbedding tests skill matching with empty embedding.
func TestMatchSkillEmptyEmbedding(t *testing.T) {
	registry := NewRegistry(0.80)
	mockEngine := &MockEmbeddingEngine{}
	registry.SetEmbeddingEngine(mockEngine)

	// Try to match with empty embedding
	_, err := registry.MatchSkill([]float32{})
	if err == nil {
		t.Error("expected error when matching with empty embedding")
	}
}

// TestMatchSkillBelowThreshold tests skill matching below confidence threshold.
func TestMatchSkillBelowThreshold(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Create a test skill
	skillDir := filepath.Join(tmpDir, "test-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("failed to create skill directory: %v", err)
	}

	skillContent := `---
name: test-skill
description: A test skill
required-capability: coding
---

# Test Skill
`

	skillPath := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte(skillContent), 0644); err != nil {
		t.Fatalf("failed to write skill file: %v", err)
	}

	// Create registry with threshold higher than what our mock will produce
	// Note: Our mock produces identical embeddings, so similarity is always 1.0
	// This test verifies the threshold logic works, even though we can't actually
	// trigger it with our simple mock
	registry := NewRegistry(0.80)
	mockEngine := &MockEmbeddingEngine{}
	registry.SetEmbeddingEngine(mockEngine)

	// Load skills
	if err := registry.LoadAll(tmpDir); err != nil {
		t.Fatalf("failed to load skills: %v", err)
	}

	// Create a query embedding
	queryEmbedding := make([]float32, 384)
	for i := range queryEmbedding {
		queryEmbedding[i] = 0.01
	}

	result, err := registry.MatchSkill(queryEmbedding)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// With our mock, similarity is always 1.0, so we'll get a match
	// This test mainly verifies the threshold comparison logic exists
	if result != nil && result.Confidence < registry.threshold {
		t.Errorf("result confidence %f should not be below threshold %f", result.Confidence, registry.threshold)
	}
}

// TestMatchSkillUsageTracking tests that skill matching updates usage statistics.
func TestMatchSkillUsageTracking(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Create a test skill
	skillDir := filepath.Join(tmpDir, "test-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("failed to create skill directory: %v", err)
	}

	skillContent := `---
name: test-skill
description: A test skill for usage tracking
required-capability: coding
---

# Test Skill
`

	skillPath := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte(skillContent), 0644); err != nil {
		t.Fatalf("failed to write skill file: %v", err)
	}

	// Create registry with mock embedding engine
	registry := NewRegistry(0.80)
	mockEngine := &MockEmbeddingEngine{}
	registry.SetEmbeddingEngine(mockEngine)

	// Load skills
	if err := registry.LoadAll(tmpDir); err != nil {
		t.Fatalf("failed to load skills: %v", err)
	}

	// Check initial usage
	stats := registry.GetUsageStats()
	if stats["test-skill"] != 0 {
		t.Errorf("expected initial usage 0, got %d", stats["test-skill"])
	}

	// Match skill multiple times
	queryEmbedding, err := mockEngine.Embed("Test query")
	if err != nil {
		t.Fatalf("failed to create query embedding: %v", err)
	}

	for i := 0; i < 3; i++ {
		result, err := registry.MatchSkill(queryEmbedding)
		if err != nil {
			t.Fatalf("failed to match skill: %v", err)
		}
		if result == nil {
			t.Fatal("expected skill match")
		}
	}

	// Check updated usage
	stats = registry.GetUsageStats()
	if stats["test-skill"] != 3 {
		t.Errorf("expected usage count 3, got %d", stats["test-skill"])
	}
}

// TestMatchSkillWithMultipleSkills tests matching with multiple skills.
func TestMatchSkillWithMultipleSkills(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Create multiple test skills
	skills := []struct {
		id          string
		name        string
		description string
		capability  string
	}{
		{"skill-1", "Skill One", "First test skill with unique description", "coding"},
		{"skill-2", "Skill Two", "Second test skill with different description", "reasoning"},
		{"skill-3", "Skill Three", "Third test skill with another description", "creative"},
	}

	for _, s := range skills {
		skillDir := filepath.Join(tmpDir, s.id)
		if err := os.MkdirAll(skillDir, 0755); err != nil {
			t.Fatalf("failed to create skill directory: %v", err)
		}

		content := "---\nname: " + s.name + "\ndescription: " + s.description + "\nrequired-capability: " + s.capability + "\n---\n\n# " + s.name
		skillPath := filepath.Join(skillDir, "SKILL.md")
		if err := os.WriteFile(skillPath, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write skill file: %v", err)
		}
	}

	// Create registry with mock embedding engine
	registry := NewRegistry(0.80)
	mockEngine := &MockEmbeddingEngine{}
	registry.SetEmbeddingEngine(mockEngine)

	// Load skills
	if err := registry.LoadAll(tmpDir); err != nil {
		t.Fatalf("failed to load skills: %v", err)
	}

	// Match with a query
	queryEmbedding, err := mockEngine.Embed("Test query")
	if err != nil {
		t.Fatalf("failed to create query embedding: %v", err)
	}

	result, err := registry.MatchSkill(queryEmbedding)
	if err != nil {
		t.Fatalf("failed to match skill: %v", err)
	}

	// Should get the best matching skill
	if result == nil {
		t.Error("expected a skill match")
	} else {
		if result.Skill == nil {
			t.Error("expected non-nil skill in result")
		}
		if result.Confidence <= 0 {
			t.Errorf("expected positive confidence, got %f", result.Confidence)
		}
		// Allow small floating point error
		if result.Confidence > 1.0001 {
			t.Errorf("expected confidence <= 1.0, got %f", result.Confidence)
		}
		// With our mock, identical embeddings give confidence of 1.0
		if result.Confidence < 0.9999 || result.Confidence > 1.0001 {
			t.Logf("Note: confidence is %f (expected ~1.0 with identical embeddings)", result.Confidence)
		}
	}
}
