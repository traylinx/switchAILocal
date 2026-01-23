// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package opencode provides a client for interacting with the OpenCode server API.
package opencode

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client provides methods to interact with the OpenCode server.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new OpenCode client.
func NewClient(baseURL string) *Client {
	if baseURL == "" {
		baseURL = "http://localhost:4096"
	}
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// Provider represents an AI provider available in OpenCode.
type Provider struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// Agent represents an agent available in OpenCode.
type Agent struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	SystemRole  string `json:"systemRole,omitempty"`
}

// HealthStatus represents the health check response.
type HealthStatus struct {
	Status  string `json:"status"`
	Version string `json:"version,omitempty"`
}

// IsHealthy checks if the OpenCode server is running and healthy.
func (c *Client) IsHealthy(ctx context.Context) (bool, error) {
	url := fmt.Sprintf("%s/health", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK, nil
}

// ListProviders fetches all available providers from OpenCode.
func (c *Client) ListProviders(ctx context.Context) ([]Provider, error) {
	url := fmt.Sprintf("%s/provider", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to list providers: status %d, body: %s", resp.StatusCode, string(body))
	}

	var providers []Provider
	if err := json.NewDecoder(resp.Body).Decode(&providers); err != nil {
		return nil, fmt.Errorf("failed to decode providers: %w", err)
	}

	return providers, nil
}

// ListAgents fetches all available agents from OpenCode.
func (c *Client) ListAgents(ctx context.Context) ([]Agent, error) {
	url := fmt.Sprintf("%s/agent", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to list agents: status %d, body: %s", resp.StatusCode, string(body))
	}

	var agents []Agent
	if err := json.NewDecoder(resp.Body).Decode(&agents); err != nil {
		return nil, fmt.Errorf("failed to decode agents: %w", err)
	}

	return agents, nil
}

// GetModels returns a list of model IDs formatted for the /v1/models endpoint.
// It combines agents and providers into opencode:* model identifiers.
func (c *Client) GetModels(ctx context.Context) ([]string, error) {
	var models []string

	// Fetch agents first (primary models)
	agents, err := c.ListAgents(ctx)
	if err == nil {
		for _, agent := range agents {
			models = append(models, fmt.Sprintf("opencode:%s", agent.ID))
		}
	}

	// Optionally add providers as model options
	providers, err := c.ListProviders(ctx)
	if err == nil {
		for _, provider := range providers {
			models = append(models, fmt.Sprintf("opencode:%s", provider.ID))
		}
	}

	// Always include default models if list is empty
	if len(models) == 0 {
		models = []string{
			"opencode:build",
			"opencode:plan",
			"opencode:explore",
		}
	}

	return models, nil
}
