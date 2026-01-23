// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package executor

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"github.com/traylinx/switchAILocal/internal/config"
	"github.com/traylinx/switchAILocal/sdk/switchailocal/auth"
	"github.com/traylinx/switchAILocal/sdk/switchailocal/executor"
)

// LMStudioExecutor provides integration with locally running LM Studio instances.
// LM Studio is OpenAI-compatible, so this executor forwards requests directly
// without translation (unlike Ollama which needs API format conversion).
type LMStudioExecutor struct {
	cfg     *config.Config
	baseURL string
	client  *http.Client
}

// NewLMStudioExecutor creates a new executor for LM Studio.
func NewLMStudioExecutor(cfg *config.Config) *LMStudioExecutor {
	baseURL := "http://localhost:1234/v1"
	if cfg != nil && cfg.LMStudio.BaseURL != "" {
		baseURL = strings.TrimSuffix(cfg.LMStudio.BaseURL, "/")
	}
	return &LMStudioExecutor{
		cfg:     cfg,
		baseURL: baseURL,
		client: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}
}

func (e *LMStudioExecutor) Identifier() string { return "lmstudio" }

func (e *LMStudioExecutor) PrepareRequest(_ *http.Request, _ *auth.Auth) error { return nil }

// Execute forwards OpenAI-format requests to LM Studio server.
func (e *LMStudioExecutor) Execute(ctx context.Context, _ *auth.Auth, req executor.Request, opts executor.Options) (executor.Response, error) {
	url := e.baseURL + "/chat/completions"

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(req.Payload))
	if err != nil {
		return executor.Response{}, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	log.Debugf("LM Studio request to: %s", url)

	resp, err := e.client.Do(httpReq)
	if err != nil {
		return executor.Response{}, fmt.Errorf("LM Studio request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return executor.Response{}, fmt.Errorf("LM Studio returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return executor.Response{}, fmt.Errorf("failed to read LM Studio response: %w", err)
	}

	log.Debugf("LM Studio response: %d bytes", len(body))
	return executor.Response{Payload: body}, nil
}

// ExecuteStream forwards streaming requests to LM Studio and converts to SSE.
func (e *LMStudioExecutor) ExecuteStream(ctx context.Context, _ *auth.Auth, req executor.Request, opts executor.Options) (<-chan executor.StreamChunk, error) {
	// Ensure stream=true in the payload
	var payload map[string]interface{}
	if err := json.Unmarshal(req.Payload, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse request: %w", err)
	}
	payload["stream"] = true
	modifiedPayload, _ := json.Marshal(payload)

	url := e.baseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(modifiedPayload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	log.Debugf("LM Studio streaming request to: %s", url)

	resp, err := e.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("LM Studio request failed: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("LM Studio returned status %d: %s", resp.StatusCode, string(body))
	}

	ch := make(chan executor.StreamChunk, 64)

	go func() {
		defer close(ch)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(nil, 10_485_760) // 10MB buffer

		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			default:
			}

			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}

			// LM Studio returns SSE format: "data: {...}"
			// Pass through as-is since it's already OpenAI-compatible
			select {
			case ch <- executor.StreamChunk{Payload: append(line, '\n', '\n')}:
			case <-ctx.Done():
				return
			}

			// Check for [DONE] signal
			if bytes.Contains(line, []byte("[DONE]")) {
				return
			}
		}

		if err := scanner.Err(); err != nil {
			log.Errorf("LM Studio stream scan error: %v", err)
		}
	}()

	return ch, nil
}

func (e *LMStudioExecutor) CountTokens(ctx context.Context, _ *auth.Auth, req executor.Request, opts executor.Options) (executor.Response, error) {
	return executor.Response{Payload: []byte(`{"total_tokens": 0}`)}, nil
}

func (e *LMStudioExecutor) Refresh(_ context.Context, a *auth.Auth) (*auth.Auth, error) {
	return a, nil
}

// DiscoverModels queries LM Studio for available models.
func (e *LMStudioExecutor) DiscoverModels(ctx context.Context) ([]string, error) {
	url := e.baseURL + "/models"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create models request: %w", err)
	}

	resp, err := e.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("LM Studio models request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("LM Studio models returned status %d: %s", resp.StatusCode, string(body))
	}

	var modelsResp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&modelsResp); err != nil {
		return nil, fmt.Errorf("failed to parse models response: %w", err)
	}

	models := make([]string, 0, len(modelsResp.Data))
	for _, m := range modelsResp.Data {
		models = append(models, m.ID)
	}

	log.Debugf("LM Studio discovered %d models: %v", len(models), models)
	return models, nil
}

// Ensure uuid is used (for consistency with other executors)
var _ = uuid.New
