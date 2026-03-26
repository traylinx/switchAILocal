// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package observability provides structured event logging, metrics, and tracing
// for integrating switchAILocal with external monitoring systems.
package observability

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

// RequestEvent is a structured per-request telemetry record emitted as NDJSON.
// It captures the full lifecycle of a proxied API request, including routing
// decisions, provider selection, latency, token counts, and quality scores.
type RequestEvent struct {
	// Timestamp of the event in ISO 8601 format.
	Timestamp string `json:"timestamp"`

	// RequestID is the unique identifier for this request.
	RequestID string `json:"request_id"`

	// Model is the model originally requested by the client.
	RequestedModel string `json:"requested_model"`

	// SelectedModel is the model actually used after auto-routing.
	SelectedModel string `json:"selected_model,omitempty"`

	// Provider is the provider that served the request.
	Provider string `json:"provider"`

	// Intent is the classified intent (e.g., "coding", "reasoning", "fast").
	Intent string `json:"intent,omitempty"`

	// Complexity is the estimated query complexity (0.0 to 1.0).
	Complexity float64 `json:"complexity,omitempty"`

	// LatencyMs is the total request duration in milliseconds.
	LatencyMs int64 `json:"latency_ms"`

	// HTTPStatus is the upstream HTTP response status code.
	HTTPStatus int `json:"http_status"`

	// Success indicates whether the request completed successfully.
	Success bool `json:"success"`

	// InputTokens is the number of tokens in the request.
	InputTokens int `json:"input_tokens,omitempty"`

	// OutputTokens is the number of tokens in the response.
	OutputTokens int `json:"output_tokens,omitempty"`

	// RQS is the Routing Quality Score for this request (0.0 to 1.0).
	RQS float64 `json:"rqs,omitempty"`

	// AutoRouted indicates whether auto-routing was used.
	AutoRouted bool `json:"auto_routed"`

	// FallbacksAttempted is the number of fallback providers tried before success.
	FallbacksAttempted int `json:"fallbacks_attempted,omitempty"`

	// Streaming indicates whether this was a streaming response.
	Streaming bool `json:"streaming,omitempty"`

	// Error contains the error message if the request failed.
	Error string `json:"error,omitempty"`
}

// EventEmitter writes structured RequestEvent records to a configured output.
type EventEmitter struct {
	mu      sync.Mutex
	writer  io.Writer
	closer  io.Closer
	enabled bool
}

// NewEventEmitter creates a new emitter based on the provided configuration.
// output: "stdout" or "file"
// filePath: path to the NDJSON file (only used when output is "file")
func NewEventEmitter(enabled bool, output, filePath string) (*EventEmitter, error) {
	if !enabled {
		return &EventEmitter{enabled: false}, nil
	}

	e := &EventEmitter{enabled: true}

	switch strings.ToLower(strings.TrimSpace(output)) {
	case "file":
		dir := filepath.Dir(filePath)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("observability: failed to create event log directory %s: %w", dir, err)
		}
		f, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return nil, fmt.Errorf("observability: failed to open event log file %s: %w", filePath, err)
		}
		e.writer = f
		e.closer = f
	default:
		// "stdout" or anything else defaults to stdout
		e.writer = os.Stdout
	}

	return e, nil
}

// Emit writes a single RequestEvent as a JSON line to the configured output.
func (e *EventEmitter) Emit(event *RequestEvent) {
	if !e.enabled || e.writer == nil {
		return
	}

	if event.Timestamp == "" {
		event.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}

	data, err := json.Marshal(event)
	if err != nil {
		log.WithError(err).Warn("observability: failed to marshal request event")
		return
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	if _, err := e.writer.Write(append(data, '\n')); err != nil {
		log.WithError(err).Warn("observability: failed to write request event")
	}
}

// Close shuts down the emitter and flushes any buffered output.
func (e *EventEmitter) Close() error {
	if e.closer != nil {
		return e.closer.Close()
	}
	return nil
}

// IsEnabled returns whether the emitter is active.
func (e *EventEmitter) IsEnabled() bool {
	return e.enabled
}

// GinMiddleware creates a Gin middleware that emits a RequestEvent after every request completes.
func GinMiddleware(emitter *EventEmitter) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		if !emitter.IsEnabled() {
			return
		}

		// Skip management and non-AI routes
		path := c.Request.URL.Path
		if !strings.HasPrefix(path, "/v1/") && !strings.HasPrefix(path, "/api/provider/") {
			return
		}

		latencyMs := time.Since(start).Milliseconds()
		status := c.Writer.Status()

		// Read tracking headers set by handlers
		reqID := c.GetString("request_id")
		model := c.GetString("requested_model")
		if model == "" && c.Request.URL != nil {
			// fallback to a generic model indicator if not set explicitly
			model = "unknown"
		}

		selectedModel := c.Writer.Header().Get("X-Selected-Model")
		provider := c.Writer.Header().Get("X-Provider")
		if provider == "" {
			provider = "unknown"
		}
		
		autoRouted := c.Writer.Header().Get("X-Auto-Routed") == "true"
		
		var rqs float64
		if rqsStr := c.Writer.Header().Get("X-RQS"); rqsStr != "" {
			if parsed, err := strconv.ParseFloat(rqsStr, 64); err == nil {
				rqs = parsed
			}
		}

		var fallbacks int
		if fallbacksStr := c.Writer.Header().Get("X-Fallbacks"); fallbacksStr != "" {
			if parsed, err := strconv.Atoi(fallbacksStr); err == nil {
				fallbacks = parsed
			}
		}

		var inTokens, outTokens int
		if usageStr := c.Writer.Header().Get("X-Usage-Input"); usageStr != "" {
			inTokens, _ = strconv.Atoi(usageStr)
		}
		if usageStr := c.Writer.Header().Get("X-Usage-Output"); usageStr != "" {
			outTokens, _ = strconv.Atoi(usageStr)
		}

		success := status >= 200 && status < 300
		errMsg := c.Errors.ByType(gin.ErrorTypePrivate).String()

		event := &RequestEvent{
			RequestID:          reqID,
			RequestedModel:     model,
			SelectedModel:      selectedModel,
			Provider:           provider,
			LatencyMs:          latencyMs,
			HTTPStatus:         status,
			Success:            success,
			InputTokens:        inTokens,
			OutputTokens:       outTokens,
			RQS:                rqs,
			AutoRouted:         autoRouted,
			FallbacksAttempted: fallbacks,
			Error:              errMsg,
		}

		emitter.Emit(event)
	}
}
