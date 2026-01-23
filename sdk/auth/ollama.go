// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/traylinx/switchAILocal/internal/config"
	coreauth "github.com/traylinx/switchAILocal/sdk/switchailocal/auth"
)

// OllamaAuthenticator handles the "login" for Ollama which checks if the local instance is running.
type OllamaAuthenticator struct{}

func NewOllamaAuthenticator() *OllamaAuthenticator {
	return &OllamaAuthenticator{}
}

func (a *OllamaAuthenticator) Provider() string {
	return "ollama"
}

// RefreshLead returns nil as there's no token expiration to manage
func (a *OllamaAuthenticator) RefreshLead() *time.Duration {
	return nil
}

// OllamaTokenStorage implements baseauth.TokenStorage
type OllamaTokenStorage struct {
	BaseURL     string   `json:"base_url"`
	Models      []string `json:"models,omitempty"`
	ConnectedAt string   `json:"connected_at"`
}

func (s *OllamaTokenStorage) SaveTokenToFile(path string) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// Login checks if Ollama is running and retrieves available models
func (a *OllamaAuthenticator) Login(ctx context.Context, cfg *config.Config, opts *LoginOptions) (*coreauth.Auth, error) {
	baseURL := "http://localhost:11434"
	if cfg != nil && cfg.Ollama.BaseURL != "" {
		baseURL = cfg.Ollama.BaseURL
	}

	fmt.Printf("Checking Ollama at %s...\n", baseURL)

	// Check if Ollama is running by hitting the tags endpoint
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(baseURL + "/api/tags")
	if err != nil {
		return nil, fmt.Errorf("Ollama is not running at %s: %w\nPlease start Ollama with: ollama serve", baseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Ollama returned status %d - please ensure Ollama is running", resp.StatusCode)
	}

	// Parse available models
	var tagsResp struct {
		Models []struct {
			Name       string `json:"name"`
			ModifiedAt string `json:"modified_at"`
			Size       int64  `json:"size"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tagsResp); err != nil {
		return nil, fmt.Errorf("failed to parse Ollama models: %w", err)
	}

	modelNames := make([]string, 0, len(tagsResp.Models))
	for _, m := range tagsResp.Models {
		modelNames = append(modelNames, m.Name)
	}

	if len(modelNames) == 0 {
		fmt.Println("⚠️  No models found. Pull a model with: ollama pull llama3.2")
	} else {
		fmt.Printf("✅ Ollama connected! Found %d model(s):\n", len(modelNames))
		for _, name := range modelNames {
			fmt.Printf("   - %s\n", name)
		}
	}

	fileName := "ollama-local.json"

	metadata := map[string]any{
		"type":       "local-server",
		"base_url":   baseURL,
		"models":     modelNames,
		"created_at": time.Now().UTC(),
	}

	storage := &OllamaTokenStorage{
		BaseURL:     baseURL,
		Models:      modelNames,
		ConnectedAt: time.Now().UTC().Format(time.RFC3339),
	}

	return &coreauth.Auth{
		ID:       fileName,
		Provider: a.Provider(),
		FileName: fileName,
		Storage:  storage,
		Metadata: metadata,
	}, nil
}
