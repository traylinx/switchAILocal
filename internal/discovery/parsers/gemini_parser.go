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
	geminiConstRegex = regexp.MustCompile(`export\s+const\s+(\w+)\s*=\s*['"]([^'"]+)['"]`)
	geminiSetRegex   = regexp.MustCompile(`VALID_GEMINI_MODELS\s*=\s*new\s+Set\(\[\s*([^\]]+)\s*\]\)`)
)

// GeminiParser extracts model definitions from the Gemini CLI TypeScript source.
type GeminiParser struct{}

// NewGeminiParser creates a new Gemini parser.
func NewGeminiParser() *GeminiParser {
	return &GeminiParser{}
}

// Parse extracts model constants from models.ts.
// It does a two-pass parse:
// 1. Extract all constant definitions (export const X = 'value')
// 2. Resolve the VALID_GEMINI_MODELS Set entries
func (p *GeminiParser) Parse(content []byte) ([]*registry.ModelInfo, error) {
	src := string(content)

	// First pass: Extract all constant definitions
	constants := make(map[string]string)
	constMatches := geminiConstRegex.FindAllStringSubmatch(src, -1)

	for _, match := range constMatches {
		if len(match) >= 3 {
			constants[match[1]] = match[2]
		}
	}

	// Second pass: Find VALID_GEMINI_MODELS Set
	setMatch := geminiSetRegex.FindStringSubmatch(src)

	var modelNames []string

	if len(setMatch) >= 2 {
		// Parse the Set contents (comma-separated constant names or string literals)
		entries := strings.Split(setMatch[1], ",")
		for _, entry := range entries {
			entry = strings.TrimSpace(entry)
			// Remove trailing comma if any
			entry = strings.TrimSuffix(entry, ",")
			entry = strings.TrimSpace(entry)

			if entry == "" {
				continue
			}

			// Check if it's a constant reference
			if val, ok := constants[entry]; ok {
				modelNames = append(modelNames, val)
			} else if strings.HasPrefix(entry, "'") || strings.HasPrefix(entry, "\"") {
				// It's a string literal
				val := strings.Trim(entry, "'\"")
				modelNames = append(modelNames, val)
			}
		}
	}

	// If Set parsing failed, fall back to extracting known model constants
	if len(modelNames) == 0 {
		// Known Gemini model constant patterns
		knownPatterns := []string{
			"PREVIEW_GEMINI_MODEL",
			"PREVIEW_GEMINI_FLASH_MODEL",
			"DEFAULT_GEMINI_MODEL",
			"DEFAULT_GEMINI_FLASH_MODEL",
			"DEFAULT_GEMINI_FLASH_LITE_MODEL",
		}
		for _, pattern := range knownPatterns {
			if val, ok := constants[pattern]; ok {
				modelNames = append(modelNames, val)
			}
		}
	}

	// Build model definitions
	seen := make(map[string]bool)
	var models []*registry.ModelInfo

	for _, name := range modelNames {
		if seen[name] {
			continue
		}
		seen[name] = true

		models = append(models, &registry.ModelInfo{
			ID:          name,
			Object:      "model",
			OwnedBy:     "google",
			Type:        "gemini",
			DisplayName: name,
			Created:     time.Now().Unix(),
		})
	}

	return models, nil
}
