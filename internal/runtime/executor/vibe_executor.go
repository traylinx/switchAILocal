// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"github.com/traylinx/switchAILocal/internal/config"
	"github.com/traylinx/switchAILocal/sdk/switchailocal/auth"
	"github.com/traylinx/switchAILocal/sdk/switchailocal/executor"
)

// VibeExecutor wraps the local 'vibe' CLI tool.
type VibeExecutor struct {
	cfg *config.Config
}

func NewVibeExecutor(cfg *config.Config) *VibeExecutor {
	return &VibeExecutor{cfg: cfg}
}

func (e *VibeExecutor) Identifier() string { return "vibe" }

func (e *VibeExecutor) PrepareRequest(_ *http.Request, _ *auth.Auth) error { return nil }

func (e *VibeExecutor) Execute(ctx context.Context, auth *auth.Auth, req executor.Request, opts executor.Options) (executor.Response, error) {
	// 1. Extract the actual prompt/messages from the payload
	var payloadMap map[string]interface{}
	if err := json.Unmarshal(req.Payload, &payloadMap); err != nil {
		return executor.Response{}, fmt.Errorf("failed to parse payload: %w", err)
	}

	prompt := ""
	// Try OpenAI format "messages"
	if msgs, ok := payloadMap["messages"].([]interface{}); ok {
		for _, m := range msgs {
			if msgMap, ok := m.(map[string]interface{}); ok {
				if content, ok := msgMap["content"].(string); ok {
					prompt += content + "\n"
				}
			}
		}
	} else if p, ok := payloadMap["prompt"].(string); ok {
		// Try OpenAI "prompt" (legacy)
		prompt = p
	} else {
		// Fallback: treat the whole payload as string if it looks like one, or fail
		prompt = string(req.Payload)
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		prompt = "Hello"
	}

	// 2. Construct the vibe command
	// vibe -p "prompt" --output json
	// We use the full path explicit or just vibe
	cmdName := "vibe"
	args := []string{"-p", prompt, "--output", "json"}

	log.Debugf("Executing vibe command: %s %v", cmdName, args)

	cmd := exec.CommandContext(ctx, cmdName, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// 3. Run
	err := cmd.Run()
	if err != nil {
		log.Errorf("vibe command failed: %v, stderr: %s", err, stderr.String())
		return executor.Response{}, fmt.Errorf("vibe execution failed: %w", err)
	}

	// 4. Parse Output
	outputBytes := stdout.Bytes()
	log.Debugf("vibe output: %s", string(outputBytes))

	// Attempt to construct an OpenAI-compatible response wrapper manually
	responseContent := string(outputBytes)

	// Basic JSON parsing to see if we can extract "clean" content
	var vibeResp map[string]interface{}
	if err := json.Unmarshal(outputBytes, &vibeResp); err == nil {
		// Check common keys if vibe output is structured
		if val, ok := vibeResp["content"].(string); ok {
			responseContent = val
		} else if val, ok := vibeResp["response"].(string); ok {
			responseContent = val
		} else if val, ok := vibeResp["text"].(string); ok {
			responseContent = val
		}
	}

	// Construct fake OpenAI response
	openAIResp := map[string]interface{}{
		"id":      "chatcmpl-vibe-" + uuid.New().String(),
		"object":  "chat.completion",
		"created": 1234567890,
		"model":   req.Model,
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]string{
					"role":    "assistant",
					"content": responseContent,
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]int{
			"prompt_tokens":     len(prompt),
			"completion_tokens": len(responseContent),
			"total_tokens":      len(prompt) + len(responseContent),
		},
	}

	finalBytes, _ := json.Marshal(openAIResp)
	return executor.Response{Payload: finalBytes}, nil
}

func (e *VibeExecutor) ExecuteStream(ctx context.Context, auth *auth.Auth, req executor.Request, opts executor.Options) (<-chan executor.StreamChunk, error) {
	return nil, fmt.Errorf("streaming not supported for vibe")
}

func (e *VibeExecutor) CountTokens(ctx context.Context, auth *auth.Auth, req executor.Request, opts executor.Options) (executor.Response, error) {
	return executor.Response{Payload: []byte(`{"totalTokens": 0}`)}, nil
}

func (e *VibeExecutor) Refresh(_ context.Context, auth *auth.Auth) (*auth.Auth, error) {
	return auth, nil
}
