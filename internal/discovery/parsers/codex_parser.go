// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package parsers

import (
	"regexp"
	"strings"
	"time"

	"github.com/traylinx/switchAILocal/internal/registry"
)

var (
	// Regex to extract ModelPreset blocks
	// We look for id: "..." and show_in_picker: true|false
	codexPresetRegex = regexp.MustCompile(`ModelPreset\s*\{[^}]*id:\s*"([^"]+)"\.to_string\(\)[^}]*show_in_picker:\s*(true|false)[^}]*\}`)

	// Look for id: "..." patterns
	codexIDRegex = regexp.MustCompile(`id:\s*"([^"]+)"\.to_string\(\)`)
)

// CodexParser extracts model definitions from the Codex Rust source file.
type CodexParser struct{}

// NewCodexParser creates a new Codex parser.
func NewCodexParser() *CodexParser {
	return &CodexParser{}
}

// Parse extracts ModelPreset definitions from model_presets.rs.
// It looks for patterns like:
//
//	ModelPreset {
//	    id: "gpt-5.2-codex".to_string(),
//	    ...
//	    show_in_picker: true,
//	}
func (p *CodexParser) Parse(content []byte) ([]*registry.ModelInfo, error) {
	src := string(content)

	matches := codexPresetRegex.FindAllStringSubmatch(src, -1)

	var models []*registry.ModelInfo
	seen := make(map[string]bool)

	for _, match := range matches {
		if len(match) < 3 {
			continue
		}

		id := strings.TrimSpace(match[1])
		showInPicker := match[2] == "true"

		if !showInPicker {
			continue
		}

		if seen[id] {
			continue
		}
		seen[id] = true

		models = append(models, &registry.ModelInfo{
			ID:          id,
			Object:      "model",
			OwnedBy:     "openai",
			Type:        "codex",
			DisplayName: id,
			Created:     time.Now().Unix(),
		})
	}

	// If regex didn't work well, try a simpler approach
	if len(models) == 0 {
		idMatches := codexIDRegex.FindAllStringSubmatch(src, -1)

		for _, match := range idMatches {
			if len(match) < 2 {
				continue
			}
			id := strings.TrimSpace(match[1])
			if seen[id] {
				continue
			}
			seen[id] = true

			models = append(models, &registry.ModelInfo{
				ID:          id,
				Object:      "model",
				OwnedBy:     "openai",
				Type:        "codex",
				DisplayName: id,
				Created:     time.Now().Unix(),
			})
		}
	}

	return models, nil
}
