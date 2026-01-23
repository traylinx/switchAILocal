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
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	"github.com/traylinx/switchAILocal/internal/config"
	switchailocalauth "github.com/traylinx/switchAILocal/sdk/switchailocal/auth"
	switchailocalexecutor "github.com/traylinx/switchAILocal/sdk/switchailocal/executor"
	"github.com/traylinx/switchAILocal/internal/registry"
)

// OpenCode default configuration
const (
	DefaultOpenCodeURL = "http://localhost:4096"
	openCodeMaxRetries = 3
	openCodeRetryDelay = 100 * time.Millisecond
)

// Global session store for OpenCode (singleton pattern)
var (
	openCodeSessionStore     *OpenCodeSessionStore
	openCodeSessionStoreOnce sync.Once
)

// getOpenCodeSessionStore returns the singleton session store.
func getOpenCodeSessionStore() *OpenCodeSessionStore {
	openCodeSessionStoreOnce.Do(func() {
		// Default TTL of 1 hour for sessions
		openCodeSessionStore = NewOpenCodeSessionStore(time.Hour)
	})
	return openCodeSessionStore
}

// OpenCodeExecutor handles requests to the OpenCode API.
type OpenCodeExecutor struct {
	config *config.Config
}

// NewOpenCodeExecutor creates a new OpenCodeExecutor instance.
func NewOpenCodeExecutor(cfg *config.Config) *OpenCodeExecutor {
	return &OpenCodeExecutor{
		config: cfg,
	}
}

// Identifier returns the executor identifier.
func (e *OpenCodeExecutor) Identifier() string { return "opencode" }

// FetchOpenCodeModels retrieves available models from the local OpenCode server.
func FetchOpenCodeModels(ctx context.Context, cfg *config.Config) []*registry.ModelInfo {
	baseURL := DefaultOpenCodeURL
	if cfg != nil && cfg.OpenCode.BaseURL != "" {
		baseURL = strings.TrimSuffix(cfg.OpenCode.BaseURL, "/")
	}

	client := &http.Client{Timeout: 5 * time.Second}
	url := fmt.Sprintf("%s/agent", baseURL)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Debugf("opencode executor: failed to fetch models: %v", err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil
	}

	var agents []struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description,omitempty"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&agents); err != nil {
		return nil
	}

	now := time.Now().Unix()
	models := make([]*registry.ModelInfo, 0, len(agents))
	for _, agent := range agents {
		id := agent.ID
		if id == "" {
			id = agent.Name
		}
		if id == "" {
			continue
		}
		displayName := agent.Name
		if displayName == "" {
			displayName = id
		}
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

	return models
}

// PrepareRequest prepares the HTTP request (no-op).
func (e *OpenCodeExecutor) PrepareRequest(_ *http.Request, _ *switchailocalauth.Auth) error {
	return nil
}

// Refresh refreshes the authentication credentials (no-op).
func (e *OpenCodeExecutor) Refresh(_ context.Context, auth *switchailocalauth.Auth) (*switchailocalauth.Auth, error) {
	return auth, nil
}

// CountTokens provides a token count estimation.
// Note: This is a rough heuristic (~4 bytes per token). Actual token counts
// vary by model and tokenizer. For accurate counts, use a dedicated tokenizer.
func (e *OpenCodeExecutor) CountTokens(ctx context.Context, auth *switchailocalauth.Auth, req switchailocalexecutor.Request, opts switchailocalexecutor.Options) (switchailocalexecutor.Response, error) {
	// Extract actual message content for better estimation
	messages := gjson.GetBytes(req.Payload, "messages")
	totalChars := 0
	if messages.IsArray() {
		for _, msg := range messages.Array() {
			content := msg.Get("content").String()
			totalChars += len(content)
		}
	}
	if totalChars == 0 {
		totalChars = len(req.Payload)
	}
	// Rough estimate: ~4 characters per token
	count := totalChars / 4
	if count == 0 {
		count = 1
	}
	return switchailocalexecutor.Response{Payload: []byte(fmt.Sprintf(`{"totalTokens": %d}`, count))}, nil
}

// extractClientSessionID extracts the session_id from the request payload.
func extractClientSessionID(payload []byte) string {
	// Try extra_body.session_id first (common pattern)
	if sid := gjson.GetBytes(payload, "extra_body.session_id").String(); sid != "" {
		return sid
	}
	// Fallback to top-level session_id
	if sid := gjson.GetBytes(payload, "session_id").String(); sid != "" {
		return sid
	}
	return ""
}

// extractLastMessage extracts the last user message content from the OpenAI-format payload.
func extractLastMessage(payload []byte) string {
	messages := gjson.GetBytes(payload, "messages")
	if !messages.IsArray() {
		return ""
	}

	arr := messages.Array()
	if len(arr) == 0 {
		return ""
	}

	// Get last message
	lastMsg := arr[len(arr)-1]

	// Handle content as string or array of content parts
	content := lastMsg.Get("content")
	if content.Type == gjson.String {
		return content.String()
	}

	// Handle content as array (multimodal)
	if content.IsArray() {
		for _, part := range content.Array() {
			if part.Get("type").String() == "text" {
				return part.Get("text").String()
			}
		}
	}

	return ""
}

// getOrCreateSession resolves the OpenCode session ID for a request.
func (e *OpenCodeExecutor) getOrCreateSession(ctx context.Context, payload []byte) (string, bool, error) {
	clientSessionID := extractClientSessionID(payload)
	store := getOpenCodeSessionStore()

	if clientSessionID != "" {
		if openCodeSessionID, exists := store.GetOrCreate(clientSessionID); !exists {
			newSessionID, err := e.createSessionWithRetry(ctx)
			if err != nil {
				return "", false, fmt.Errorf("failed to create session: %w", err)
			}
			store.Set(clientSessionID, newSessionID)
			return newSessionID, true, nil
		} else {
			store.Touch(clientSessionID)
			return openCodeSessionID, false, nil
		}
	}

	sessionID, err := e.createSessionWithRetry(ctx)
	if err != nil {
		return "", true, fmt.Errorf("failed to create session: %w", err)
	}
	return sessionID, true, nil
}

// Execute performs a non-streaming request to the OpenCode API.
func (e *OpenCodeExecutor) Execute(ctx context.Context, auth *switchailocalauth.Auth, req switchailocalexecutor.Request, opts switchailocalexecutor.Options) (switchailocalexecutor.Response, error) {
	sessionID, _, err := e.getOrCreateSession(ctx, req.Payload)
	if err != nil {
		return switchailocalexecutor.Response{}, fmt.Errorf("failed to get/create opencode session: %w", err)
	}

	lastMsg := extractLastMessage(req.Payload)
	if lastMsg == "" {
		return switchailocalexecutor.Response{}, fmt.Errorf("no user message found in request")
	}

	// For non-streaming, we must connect to SSE BEFORE sending the message
	// to ensure we capture the full response including the completion event.
	contentChan := make(chan string, 1)
	errChan := make(chan error, 1)

	// Start SSE listener
	go func() {
		content, err := e.waitForResponse(ctx, sessionID)
		if err != nil {
			errChan <- err
			return
		}
		contentChan <- content
	}()

	// Small delay to ensure SSE is established (though waitForResponse handles connection)
	time.Sleep(100 * time.Millisecond)

	// Send the message
	if _, err := e.sendMessage(ctx, sessionID, lastMsg); err != nil {
		return switchailocalexecutor.Response{}, fmt.Errorf("failed to send message: %w", err)
	}

	// Wait for response
	select {
	case <-ctx.Done():
		return switchailocalexecutor.Response{}, ctx.Err()
	case err := <-errChan:
		return switchailocalexecutor.Response{}, err
	case content := <-contentChan:
		response := map[string]interface{}{
			"id":      "chatcmpl-" + sessionID,
			"object":  "chat.completion",
			"created": time.Now().Unix(),
			"model":   req.Model,
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": content,
					},
					"finish_reason": "stop",
				},
			},
		}
		respBytes, _ := json.Marshal(response)
		return switchailocalexecutor.Response{Payload: respBytes}, nil
	}
}

// waitForResponse collects the complete response from OpenCode SSE stream.
func (e *OpenCodeExecutor) waitForResponse(ctx context.Context, sessionID string) (string, error) {
	url := fmt.Sprintf("%s/event", e.baseURL())
	reqSSE, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create SSE request: %w", err)
	}
	reqSSE.Header.Set("Accept", "text/event-stream")

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(reqSSE)
	if err != nil {
		return "", fmt.Errorf("failed to connect to SSE: %w", err)
	}
	defer resp.Body.Close()

	var content strings.Builder
	reader := bufio.NewReader(resp.Body)

	// Use goroutine with channels for proper context cancellation
	lines := make(chan string)
	errs := make(chan error, 1)

	go func() {
		defer close(lines)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if err != io.EOF {
					errs <- err
				}
				return
			}
			lines <- line
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return content.String(), ctx.Err()
		case err := <-errs:
			return content.String(), fmt.Errorf("SSE read error: %w", err)
		case line, ok := <-lines:
			if !ok {
				return content.String(), nil
			}

			if strings.HasPrefix(line, "data: ") {
				dataStr := strings.TrimPrefix(strings.TrimSpace(line), "data: ")
				if dataStr == "" {
					continue
				}

				log.Debugf("[OpenCode SSE] Raw event: %s", dataStr)

				// OpenCode events have fields nested inside "properties"
				props := gjson.Get(dataStr, "properties")
				eventType := gjson.Get(dataStr, "type").String()

				// Ignore heartbeats
				if eventType == "server.heartbeat" {
					continue
				}

				eventSessionID := props.Get("sessionID").String()

				if eventSessionID != sessionID && eventSessionID != "" {
					continue
				}

				// Delta is usually in properties.delta or properties.status.delta etc.
				// Based on observerved events, we should look for delta or content
				delta := props.Get("delta").String()
				if delta == "" {
					delta = props.Get("content").String()
				}

				if delta != "" {
					content.WriteString(delta)
				}

				// Handle completion events
				if eventType == "session.idle" || eventType == "message.completed" || eventType == "assistant.complete" || eventType == "complete" {
					log.Debugf("[OpenCode SSE] Completion event received: %s", eventType)
					return content.String(), nil
				}
			}
		}
	}
}

// ExecuteStream performs a streaming request to the OpenCode API.
func (e *OpenCodeExecutor) ExecuteStream(ctx context.Context, auth *switchailocalauth.Auth, req switchailocalexecutor.Request, opts switchailocalexecutor.Options) (<-chan switchailocalexecutor.StreamChunk, error) {
	sessionID, _, err := e.getOrCreateSession(ctx, req.Payload)
	if err != nil {
		return nil, fmt.Errorf("failed to get/create session: %w", err)
	}

	lastMsg := extractLastMessage(req.Payload)
	if lastMsg == "" {
		return nil, fmt.Errorf("no user message found in request")
	}

	out := make(chan switchailocalexecutor.StreamChunk)
	readyChan := make(chan struct{})

	go func() {
		defer close(out)

		url := fmt.Sprintf("%s/event", e.baseURL())
		reqSSE, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			out <- switchailocalexecutor.StreamChunk{Err: fmt.Errorf("failed to create SSE request: %w", err)}
			close(readyChan)
			return
		}
		reqSSE.Header.Set("Accept", "text/event-stream")

		client := &http.Client{Timeout: 0}
		resp, err := client.Do(reqSSE)
		if err != nil {
			out <- switchailocalexecutor.StreamChunk{Err: fmt.Errorf("failed to connect to SSE: %w", err)}
			close(readyChan)
			return
		}
		defer resp.Body.Close()

		close(readyChan) // Signal that SSE is connected

		reader := bufio.NewReader(resp.Body)

		// Use goroutine with channels for proper context cancellation
		lines := make(chan string)
		errs := make(chan error, 1)

		go func() {
			defer close(lines)
			for {
				line, err := reader.ReadString('\n')
				if err != nil {
					if err != io.EOF {
						errs <- err
					}
					return
				}
				lines <- line
			}
		}()

		for {
			select {
			case <-ctx.Done():
				out <- switchailocalexecutor.StreamChunk{Payload: []byte("data: [DONE]\n\n")}
				return
			case err := <-errs:
				out <- switchailocalexecutor.StreamChunk{Err: fmt.Errorf("SSE read error: %w", err)}
				return
			case line, ok := <-lines:
				if !ok {
					out <- switchailocalexecutor.StreamChunk{Payload: []byte("data: [DONE]\n\n")}
					return
				}

				if strings.HasPrefix(line, "data: ") {
					dataStr := strings.TrimPrefix(strings.TrimSpace(line), "data: ")
					if dataStr == "" {
						continue
					}

					eventType := gjson.Get(dataStr, "type").String()
					eventSessionID := gjson.Get(dataStr, "sessionID").String()

					if eventSessionID != sessionID && eventSessionID != "" {
						continue
					}

					delta := gjson.Get(dataStr, "delta").String()
					if delta != "" {
						chunk := map[string]interface{}{
							"id":      "chatcmpl-" + sessionID,
							"object":  "chat.completion.chunk",
							"created": time.Now().Unix(),
							"model":   req.Model,
							"choices": []map[string]interface{}{
								{
									"index": 0,
									"delta": map[string]interface{}{
										"content": delta,
									},
								},
							},
						}
						b, _ := json.Marshal(chunk)
						out <- switchailocalexecutor.StreamChunk{Payload: append([]byte("data: "), append(b, []byte("\n\n")...)...)}
					}

					if eventType == "message.completed" || eventType == "assistant.complete" {
						out <- switchailocalexecutor.StreamChunk{Payload: []byte("data: [DONE]\n\n")}
						return
					}
				}
			}
		}
	}()

	return out, nil
}

func (e *OpenCodeExecutor) baseURL() string {
	if e.config == nil {
		return DefaultOpenCodeURL
	}
	url := e.config.OpenCode.BaseURL
	if url == "" {
		return DefaultOpenCodeURL
	}
	return strings.TrimRight(url, "/")
}

// createSessionWithRetry creates a session with exponential backoff retry.
func (e *OpenCodeExecutor) createSessionWithRetry(ctx context.Context) (string, error) {
	var lastErr error
	for attempt := 0; attempt < openCodeMaxRetries; attempt++ {
		if attempt > 0 {
			delay := openCodeRetryDelay * time.Duration(1<<(attempt-1)) // Exponential backoff
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(delay):
			}
		}
		sessionID, err := e.createSession(ctx)
		if err == nil {
			return sessionID, nil
		}
		lastErr = err
	}
	return "", fmt.Errorf("failed after %d attempts: %w", openCodeMaxRetries, lastErr)
}

func (e *OpenCodeExecutor) createSession(ctx context.Context) (string, error) {
	url := fmt.Sprintf("%s/session", e.baseURL())
	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("opencode returned status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}
	return result.ID, nil
}

func (e *OpenCodeExecutor) sendMessage(ctx context.Context, sessionID, content string) (string, error) {
	url := fmt.Sprintf("%s/session/%s/message", e.baseURL(), sessionID)
	payload := map[string]interface{}{
		"role": "user",
		"parts": []map[string]interface{}{
			{"type": "text", "text": content},
		},
	}

	jsonData, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to send message, status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&result)
	return result.ID, nil
}
