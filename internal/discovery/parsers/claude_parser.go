// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package parsers

import (
	"time"

	"github.com/traylinx/switchAILocal/internal/registry"
)

// ClaudeParser provides Claude model discovery.
// Since Claude Code's source is not public, this uses a static fallback list.
type ClaudeParser struct{}

// NewClaudeParser creates a new Claude parser.
func NewClaudeParser() *ClaudeParser {
	return &ClaudeParser{}
}

// StaticModels returns the fallback Claude model list.
// These are the known Claude models supported by Claude Code CLI.
func (p *ClaudeParser) StaticModels() []*registry.ModelInfo {
	now := time.Now().Unix()
	return []*registry.ModelInfo{
		{
			ID:          "claude-sonnet-4-5-20250929",
			Object:      "model",
			OwnedBy:     "anthropic",
			Type:        "claude",
			DisplayName: "Claude 4.5 Sonnet",
			Created:     now,
		},
		{
			ID:          "claude-opus-4-5-20251101",
			Object:      "model",
			OwnedBy:     "anthropic",
			Type:        "claude",
			DisplayName: "Claude 4.5 Opus",
			Created:     now,
		},
		{
			ID:          "claude-haiku-4-5-20251001",
			Object:      "model",
			OwnedBy:     "anthropic",
			Type:        "claude",
			DisplayName: "Claude 4.5 Haiku",
			Created:     now,
		},
		{
			ID:          "claude-sonnet-4-20250514",
			Object:      "model",
			OwnedBy:     "anthropic",
			Type:        "claude",
			DisplayName: "Claude 4 Sonnet",
			Created:     now,
		},
		{
			ID:          "claude-opus-4-20250514",
			Object:      "model",
			OwnedBy:     "anthropic",
			Type:        "claude",
			DisplayName: "Claude 4 Opus",
			Created:     now,
		},
		{
			ID:          "claude-3-5-haiku-20241022",
			Object:      "model",
			OwnedBy:     "anthropic",
			Type:        "claude",
			DisplayName: "Claude 3.5 Haiku",
			Created:     now,
		},
	}
}

// Parse is not used for Claude since we use the API or static fallback.
// This implementation returns the static models.
func (p *ClaudeParser) Parse(content []byte) ([]*registry.ModelInfo, error) {
	return p.StaticModels(), nil
}
