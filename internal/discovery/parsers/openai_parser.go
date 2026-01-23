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

// OpenAIModelsResponse represents the standard OpenAI /v1/models response format.
type OpenAIModelsResponse struct {
	Object string        `json:"object"`
	Data   []OpenAIModel `json:"data"`
}

type OpenAIModel struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// OpenAIParser parses standard OpenAI-compatible model lists.
type OpenAIParser struct {
	ProviderID string
}

func NewOpenAIParser(providerID string) *OpenAIParser {
	return &OpenAIParser{ProviderID: providerID}
}

func (p *OpenAIParser) Parse(content []byte) ([]*registry.ModelInfo, error) {
	var resp OpenAIModelsResponse
	if err := json.Unmarshal(content, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal OpenAI models: %w", err)
	}

	models := make([]*registry.ModelInfo, 0, len(resp.Data))
	now := time.Now().Unix()

	for _, m := range resp.Data {
		if m.ID == "" {
			continue
		}

		created := m.Created
		if created == 0 {
			created = now
		}

		ownedBy := m.OwnedBy
		if ownedBy == "" {
			ownedBy = p.ProviderID
		}

		models = append(models, &registry.ModelInfo{
			ID:          m.ID,
			Object:      "model",
			Created:     created,
			OwnedBy:     ownedBy,
			Type:        "discovery",
			DisplayName: m.ID,
		})
	}

	return models, nil
}
