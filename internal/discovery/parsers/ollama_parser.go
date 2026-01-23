// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package parsers

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/traylinx/switchAILocal/internal/registry"
)

// OllamaTagsResponse represents the response from Ollama's /api/tags endpoint.
type OllamaTagsResponse struct {
	Models []OllamaModel `json:"models"`
}

type OllamaModel struct {
	Name       string    `json:"name"`
	ModifiedAt time.Time `json:"modified_at"`
	Size       int64     `json:"size"`
	Digest     string    `json:"digest"`
}

// OllamaParser parses Ollama's /api/tags response.
type OllamaParser struct{}

func NewOllamaParser() *OllamaParser {
	return &OllamaParser{}
}

func (p *OllamaParser) Parse(content []byte) ([]*registry.ModelInfo, error) {
	var resp OllamaTagsResponse
	if err := json.Unmarshal(content, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal Ollama models: %w", err)
	}

	models := make([]*registry.ModelInfo, 0, len(resp.Models))

	for _, m := range resp.Models {
		if m.Name == "" {
			continue
		}

		models = append(models, &registry.ModelInfo{
			ID:          m.Name,
			Object:      "model",
			Created:     m.ModifiedAt.Unix(),
			OwnedBy:     "ollama",
			Type:        "discovery",
			DisplayName: m.Name,
		})
	}

	return models, nil
}
