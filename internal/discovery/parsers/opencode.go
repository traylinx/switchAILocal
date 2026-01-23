// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package parsers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/traylinx/switchAILocal/internal/registry"
)

// OpenCodeParser discovers models from a running OpenCode server.
type OpenCodeParser struct {
	baseURL    string
	httpClient *http.Client
}

// NewOpenCodeParser creates a new OpenCode parser.
// Default URL is http://localhost:4096 if not specified.
func NewOpenCodeParser(baseURL string) *OpenCodeParser {
	if baseURL == "" {
		baseURL = "http://localhost:4096"
	}
	return &OpenCodeParser{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

func (p *OpenCodeParser) Parse(content []byte) ([]*registry.ModelInfo, error) {
	// If content is empty, return static fallback
	if len(content) == 0 {
		return p.StaticModels(), nil
	}

	// Try to parse content as JSON (Standard ModelInfo format)
	var models []*registry.ModelInfo
	if err := json.Unmarshal(content, &models); err == nil && len(models) > 0 && models[0].ID != "" {
		return models, nil
	}

	// Try to parse content as OpenCode Agent format
	var agents []struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description,omitempty"`
	}
	if err := json.Unmarshal(content, &agents); err == nil && len(agents) > 0 {
		now := time.Now().Unix()
		models = make([]*registry.ModelInfo, 0, len(agents))
		for _, agent := range agents {
			id := agent.ID
			if id == "" {
				id = agent.Name
			}
			if id == "" {
				continue
			}

			displayName := agent.Name
			models = append(models, &registry.ModelInfo{
				ID:          id,
				Object:      "model",
				Created:     now,
				OwnedBy:     "opencode",
				Type:        "opencode",
				DisplayName: displayName,
				Description: agent.Description,
			})
		}
		return models, nil
	}

	// Fallback to static if everything else fails
	return p.StaticModels(), nil
}

// StaticModels returns default OpenCode agents.
func (p *OpenCodeParser) StaticModels() []*registry.ModelInfo {
	now := time.Now().Unix()
	return []*registry.ModelInfo{
		{
			ID:          "build",
			Object:      "model",
			Created:     now,
			OwnedBy:     "opencode",
			Type:        "opencode",
			DisplayName: "OpenCode Build Agent",
			Description: "Standard development agent with full tool access",
		},
		{
			ID:          "plan",
			Object:      "model",
			Created:     now,
			OwnedBy:     "opencode",
			Type:        "opencode",
			DisplayName: "OpenCode Plan Agent",
			Description: "Read-only agent for planning and exploration",
		},
		{
			ID:          "explore",
			Object:      "model",
			Created:     now,
			OwnedBy:     "opencode",
			Type:        "opencode",
			DisplayName: "OpenCode Explore Subagent",
			Description: "Specialized subagent for codebase mapping",
		},
	}
}

// DiscoverLive queries the OpenCode server directly for available agents.
func (p *OpenCodeParser) DiscoverLive(ctx context.Context) ([]*registry.ModelInfo, error) {
	// Try to get agents from OpenCode API
	url := fmt.Sprintf("%s/agent", p.baseURL)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("opencode not reachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("opencode returned %d: %s", resp.StatusCode, string(body))
	}

	var agents []struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description,omitempty"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&agents); err != nil {
		return nil, fmt.Errorf("failed to decode agents: %w", err)
	}

	now := time.Now().Unix()
	models := make([]*registry.ModelInfo, 0, len(agents))
	for _, agent := range agents {
		displayName := agent.Name
		if displayName == "" {
			displayName = agent.ID
		}
		models = append(models, &registry.ModelInfo{
			ID:          agent.ID,
			Object:      "model",
			Created:     now,
			OwnedBy:     "opencode",
			Type:        "opencode",
			DisplayName: displayName,
			Description: agent.Description,
		})
	}

	// Always include at least the defaults if API returns nothing
	if len(models) == 0 {
		return p.StaticModels(), nil
	}

	return models, nil
}
