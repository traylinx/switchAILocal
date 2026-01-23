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
	// Regex to extract ModelConfig entries
	// ModelConfig(name="...", alias="...", ...)
	vibeModelConfigRegex = regexp.MustCompile(`ModelConfig\([^)]*name\s*=\s*["']([^"']+)["']`)

	// Also check for alias fields to provide aliases
	vibeAliasRegex = regexp.MustCompile(`ModelConfig\([^)]*alias\s*=\s*["']([^"']+)["']`)
)

// VibeParser extracts model definitions from the Mistral Vibe Python source.
type VibeParser struct{}

// NewVibeParser creates a new Vibe parser.
func NewVibeParser() *VibeParser {
	return &VibeParser{}
}

// Parse extracts ModelConfig entries from config.py.
// It looks for patterns like:
//
//	DEFAULT_MODELS = [
//	    ModelConfig(name="devstral-2", alias="mistral-vibe-cli-latest"),
//	    ...
//	]
func (p *VibeParser) Parse(content []byte) ([]*registry.ModelInfo, error) {
	src := string(content)

	matches := vibeModelConfigRegex.FindAllStringSubmatch(src, -1)

	seen := make(map[string]bool)
	var models []*registry.ModelInfo

	for _, match := range matches {
		if len(match) < 2 {
			continue
		}

		name := strings.TrimSpace(match[1])
		if seen[name] {
			continue
		}
		seen[name] = true

		models = append(models, &registry.ModelInfo{
			ID:          name,
			Object:      "model",
			OwnedBy:     "mistral",
			Type:        "vibe",
			DisplayName: name,
			Created:     time.Now().Unix(),
		})
	}

	aliasMatches := vibeAliasRegex.FindAllStringSubmatch(src, -1)

	for _, match := range aliasMatches {
		if len(match) < 2 {
			continue
		}

		alias := strings.TrimSpace(match[1])
		if seen[alias] {
			continue
		}
		seen[alias] = true

		models = append(models, &registry.ModelInfo{
			ID:          alias,
			Object:      "model",
			OwnedBy:     "mistral",
			Type:        "vibe",
			DisplayName: alias,
			Created:     time.Now().Unix(),
		})
	}

	return models, nil
}
