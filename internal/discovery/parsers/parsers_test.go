// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package parsers

import (
	"testing"
)

func TestCodexParser_Parse(t *testing.T) {
	parser := NewCodexParser()

	// Sample content from model_presets.rs
	content := []byte(`
        ModelPreset {
            id: "gpt-5.2-codex".to_string(),
            name: "GPT 5.2 Codex".to_string(),
            show_in_picker: true,
        },
        ModelPreset {
            id: "gpt-5.1-codex".to_string(),
            name: "GPT 5.1 Codex".to_string(),
            show_in_picker: true,
        },
        ModelPreset {
            id: "internal-test".to_string(),
            name: "Test Model".to_string(),
            show_in_picker: false,
        },
    `)

	models, err := parser.Parse(content)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Should have 2 models (excludes show_in_picker: false)
	if len(models) != 2 {
		t.Errorf("Expected 2 models, got %d", len(models))
	}

	// Verify first model
	if len(models) > 0 && models[0].ID != "gpt-5.2-codex" {
		t.Errorf("Expected first model ID 'gpt-5.2-codex', got '%s'", models[0].ID)
	}
}

func TestGeminiParser_Parse(t *testing.T) {
	parser := NewGeminiParser()

	// Sample content from models.ts
	content := []byte(`
export const DEFAULT_GEMINI_MODEL = 'gemini-2.5-flash';
export const DEFAULT_GEMINI_PRO_MODEL = 'gemini-2.5-pro';
export const PREVIEW_GEMINI_MODEL = 'gemini-3-pro-preview';

export const VALID_GEMINI_MODELS = new Set([
    DEFAULT_GEMINI_MODEL,
    DEFAULT_GEMINI_PRO_MODEL,
    PREVIEW_GEMINI_MODEL,
]);
    `)

	models, err := parser.Parse(content)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(models) < 3 {
		t.Errorf("Expected at least 3 models, got %d", len(models))
	}

	// Check that resolved values are present
	foundFlash := false
	for _, m := range models {
		if m.ID == "gemini-2.5-flash" {
			foundFlash = true
			break
		}
	}
	if !foundFlash {
		t.Error("Expected to find 'gemini-2.5-flash' model")
	}
}

func TestVibeParser_Parse(t *testing.T) {
	parser := NewVibeParser()

	// Sample content from config.py
	content := []byte(`
DEFAULT_MODELS = [
    ModelConfig(name="devstral-2", alias="mistral-vibe-cli-latest"),
    ModelConfig(name="codestral-2501"),
    ModelConfig(name="ministral-8b", alias="ministral"),
]
    `)

	models, err := parser.Parse(content)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(models) < 3 {
		t.Errorf("Expected at least 3 models (names), got %d", len(models))
	}

	// Check that model names are extracted
	foundDevstral := false
	for _, m := range models {
		if m.ID == "devstral-2" {
			foundDevstral = true
			break
		}
	}
	if !foundDevstral {
		t.Error("Expected to find 'devstral-2' model")
	}
}

func TestClaudeParser_StaticModels(t *testing.T) {
	parser := NewClaudeParser()

	models := parser.StaticModels()

	if len(models) < 5 {
		t.Errorf("Expected at least 5 static Claude models, got %d", len(models))
	}

	// Check that well-known models are present
	foundSonnet := false
	for _, m := range models {
		if m.ID == "claude-sonnet-4-5-20250929" {
			foundSonnet = true
			break
		}
	}
	if !foundSonnet {
		t.Error("Expected to find 'claude-sonnet-4-5-20250929' in static models")
	}
}
