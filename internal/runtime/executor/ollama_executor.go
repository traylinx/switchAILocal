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

// OllamaExecutor provides integration with locally running Ollama instances.
// It communicates via HTTP to the Ollama API (default: http://localhost:11434).
type OllamaExecutor struct {
	cfg     *config.Config
	baseURL string
	client  *http.Client
}

// NewOllamaExecutor creates a new executor for Ollama.
func NewOllamaExecutor(cfg *config.Config) *OllamaExecutor {
	baseURL := "http://localhost:11434"
	if cfg != nil && cfg.Ollama.BaseURL != "" {
		baseURL = strings.TrimSuffix(cfg.Ollama.BaseURL, "/")
	}
	return &OllamaExecutor{
		cfg:     cfg,
		baseURL: baseURL,
		client: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}
}

func (e *OllamaExecutor) Identifier() string { return "ollama" }

func (e *OllamaExecutor) PrepareRequest(_ *http.Request, _ *auth.Auth) error { return nil }

func (e *OllamaExecutor) Execute(ctx context.Context, _ *auth.Auth, req executor.Request, opts executor.Options) (executor.Response, error) {
	// Parse the incoming OpenAI-format payload
	var openAIReq struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
		Stream      bool    `json:"stream"`
		Temperature float64 `json:"temperature,omitempty"`
		MaxTokens   int     `json:"max_tokens,omitempty"`
	}
	if err := json.Unmarshal(req.Payload, &openAIReq); err != nil {
		return executor.Response{}, fmt.Errorf("failed to parse OpenAI request: %w", err)
	}

	// Use req.Model which has the normalized model name (provider prefix stripped)
	// This ensures we send the correct model name to Ollama
	modelName := req.Model
	if modelName == "" {
		modelName = openAIReq.Model
	}

	// Convert to Ollama format
	ollamaReq := map[string]interface{}{
		"model":    modelName,
		"messages": openAIReq.Messages,
		"stream":   false,
	}
	if openAIReq.Temperature > 0 {
		ollamaReq["options"] = map[string]interface{}{
			"temperature": openAIReq.Temperature,
		}
	}

	reqBody, err := json.Marshal(ollamaReq)
	if err != nil {
		return executor.Response{}, fmt.Errorf("failed to marshal Ollama request: %w", err)
	}

	log.Debugf("Ollama request: %s", string(reqBody))

	// Make HTTP request to Ollama
	httpReq, err := http.NewRequestWithContext(ctx, "POST", e.baseURL+"/api/chat", bytes.NewReader(reqBody))
	if err != nil {
		return executor.Response{}, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(httpReq)
	if err != nil {
		return executor.Response{}, fmt.Errorf("Ollama request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return executor.Response{}, fmt.Errorf("Ollama returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse Ollama response
	var ollamaResp struct {
		Model     string `json:"model"`
		CreatedAt string `json:"created_at"`
		Message   struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		Done            bool  `json:"done"`
		TotalDuration   int64 `json:"total_duration"`
		PromptEvalCount int   `json:"prompt_eval_count"`
		EvalCount       int   `json:"eval_count"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return executor.Response{}, fmt.Errorf("failed to parse Ollama response: %w", err)
	}

	log.Debugf("Ollama response: model=%s, content_len=%d", ollamaResp.Model, len(ollamaResp.Message.Content))

	// Convert to OpenAI format
	openAIResp := map[string]interface{}{
		"id":      "chatcmpl-ollama-" + uuid.New().String(),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   ollamaResp.Model,
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]string{
					"role":    ollamaResp.Message.Role,
					"content": ollamaResp.Message.Content,
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]int{
			"prompt_tokens":     ollamaResp.PromptEvalCount,
			"completion_tokens": ollamaResp.EvalCount,
			"total_tokens":      ollamaResp.PromptEvalCount + ollamaResp.EvalCount,
		},
	}

	finalBytes, _ := json.Marshal(openAIResp)
	return executor.Response{Payload: finalBytes}, nil
}

func (e *OllamaExecutor) ExecuteStream(ctx context.Context, _ *auth.Auth, req executor.Request, opts executor.Options) (<-chan executor.StreamChunk, error) {
	// Parse the incoming OpenAI-format payload
	var openAIReq struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
		Temperature float64 `json:"temperature,omitempty"`
	}
	if err := json.Unmarshal(req.Payload, &openAIReq); err != nil {
		return nil, fmt.Errorf("failed to parse OpenAI request: %w", err)
	}

	// Use req.Model which has the normalized model name (provider prefix stripped)
	// This ensures we send the correct model name to Ollama
	modelName := req.Model
	if modelName == "" {
		modelName = openAIReq.Model
	}

	// Convert to Ollama format with streaming enabled
	ollamaReq := map[string]interface{}{
		"model":    modelName,
		"messages": openAIReq.Messages,
		"stream":   true,
	}
	if openAIReq.Temperature > 0 {
		ollamaReq["options"] = map[string]interface{}{
			"temperature": openAIReq.Temperature,
		}
	}

	reqBody, err := json.Marshal(ollamaReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Ollama request: %w", err)
	}

	// Make HTTP request to Ollama
	httpReq, err := http.NewRequestWithContext(ctx, "POST", e.baseURL+"/api/chat", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("Ollama request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("Ollama returned status %d: %s", resp.StatusCode, string(body))
	}

	chunkID := "chatcmpl-ollama-" + uuid.New().String()
	ch := make(chan executor.StreamChunk, 64)

	go func() {
		defer close(ch)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}

			var ollamaChunk struct {
				Model   string `json:"model"`
				Message struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				} `json:"message"`
				Done bool `json:"done"`
			}

			if err := json.Unmarshal(line, &ollamaChunk); err != nil {
				log.Warnf("Failed to parse Ollama stream chunk: %v", err)
				continue
			}

			// Convert to OpenAI SSE format
			openAIChunk := map[string]interface{}{
				"id":      chunkID,
				"object":  "chat.completion.chunk",
				"created": time.Now().Unix(),
				"model":   ollamaChunk.Model,
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"delta": map[string]string{
							"content": ollamaChunk.Message.Content,
						},
						"finish_reason": nil,
					},
				},
			}

			if ollamaChunk.Done {
				openAIChunk["choices"].([]map[string]interface{})[0]["finish_reason"] = "stop"
			}

			chunkBytes, _ := json.Marshal(openAIChunk)
			// Return raw JSON bytes without "data: " prefix or newlines
			// The upstream handler will wrap this in "data: %s\n\n"

			select {
			case ch <- executor.StreamChunk{Payload: chunkBytes}:
			case <-ctx.Done():
				return
			}

			if ollamaChunk.Done {
				return
			}
		}
	}()

	return ch, nil
}

func (e *OllamaExecutor) CountTokens(ctx context.Context, _ *auth.Auth, req executor.Request, opts executor.Options) (executor.Response, error) {
	return executor.Response{Payload: []byte(`{"totalTokens": 0}`)}, nil
}

func (e *OllamaExecutor) Refresh(_ context.Context, a *auth.Auth) (*auth.Auth, error) {
	return a, nil
}
