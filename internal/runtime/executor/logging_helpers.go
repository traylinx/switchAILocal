// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package executor

import (
	"bytes"
	"context"
	"fmt"
	"html"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/traylinx/switchAILocal/internal/config"
	"github.com/traylinx/switchAILocal/internal/util"
)

const (
	apiAttemptsKey = "API_UPSTREAM_ATTEMPTS"
	apiRequestKey  = "API_REQUEST"
	apiResponseKey = "API_RESPONSE"
)

type loggingContextKey string

const (
	// LoggingStoreKey is the context key for the thread-safe logging store.
	LoggingStoreKey loggingContextKey = "SWITCHAI_LOGGING_STORE"
)

// LoggingStore handles thread-safe accumulation of upstream API attempts.
type LoggingStore struct {
	mu       sync.RWMutex
	attempts []*upstreamAttempt
	cfg      *config.Config
}

// NewLoggingStore creates a new thread-safe logging store.
func NewLoggingStore(cfg *config.Config) *LoggingStore {
	return &LoggingStore{
		cfg: cfg,
	}
}

// upstreamRequestLog captures the outbound upstream request details for logging.
type upstreamRequestLog struct {
	URL       string
	Method    string
	Headers   http.Header
	Body      []byte
	Provider  string
	AuthID    string
	AuthLabel string
	AuthType  string
	AuthValue string
}

type upstreamAttempt struct {
	index                int
	request              string
	response             *strings.Builder
	responseIntroWritten bool
	statusWritten        bool
	headersWritten       bool
	bodyStarted          bool
	bodyHasContent       bool
	errorWritten         bool
}

// recordAPIRequest stores the upstream request metadata in LoggingStore if available,
// falling back to legacy Gin context logic for backward compatibility.
func recordAPIRequest(ctx context.Context, cfg *config.Config, info upstreamRequestLog) {
	if cfg == nil || !cfg.RequestLog {
		return
	}

	// Try to use thread-safe LoggingStore first
	if store, ok := loggingStoreFrom(ctx); ok {
		store.RecordRequest(info)
		return
	}

	// Fallback to legacy Gin context logic (vulnerable to race conditions)
	ginCtx := ginContextFrom(ctx)
	if ginCtx == nil {
		return
	}

	attempts := getAttempts(ginCtx)
	index := len(attempts) + 1

	builder := &strings.Builder{}
	builder.WriteString(fmt.Sprintf("=== API REQUEST %d ===\n", index))
	builder.WriteString(fmt.Sprintf("Timestamp: %s\n", time.Now().Format(time.RFC3339Nano)))
	if info.URL != "" {
		builder.WriteString(fmt.Sprintf("Upstream URL: %s\n", info.URL))
	} else {
		builder.WriteString("Upstream URL: <unknown>\n")
	}
	if info.Method != "" {
		builder.WriteString(fmt.Sprintf("HTTP Method: %s\n", info.Method))
	}
	if auth := formatAuthInfo(info); auth != "" {
		builder.WriteString(fmt.Sprintf("Auth: %s\n", auth))
	}
	builder.WriteString("\nHeaders:\n")
	writeHeaders(builder, info.Headers)
	builder.WriteString("\nBody:\n")
	if len(info.Body) > 0 {
		builder.WriteString(string(bytes.Clone(info.Body)))
	} else {
		builder.WriteString("<empty>")
	}
	builder.WriteString("\n\n")

	attempt := &upstreamAttempt{
		index:    index,
		request:  builder.String(),
		response: &strings.Builder{},
	}
	attempts = append(attempts, attempt)
	ginCtx.Set(apiAttemptsKey, attempts)
	updateAggregatedRequest(ginCtx, attempts)
}

// loggingStoreFrom retrieves the LoggingStore from the context if it exists.
func loggingStoreFrom(ctx context.Context) (*LoggingStore, bool) {
	if ctx == nil {
		return nil, false
	}
	if store, ok := ctx.Value(LoggingStoreKey).(*LoggingStore); ok && store != nil {
		return store, true
	}
	return nil, false
}

// RecordRequest records an upstream request.
func (s *LoggingStore) RecordRequest(info upstreamRequestLog) {
	s.mu.Lock()
	defer s.mu.Unlock()

	index := len(s.attempts) + 1
	builder := &strings.Builder{}
	builder.WriteString(fmt.Sprintf("=== API REQUEST %d ===\n", index))
	builder.WriteString(fmt.Sprintf("Timestamp: %s\n", time.Now().Format(time.RFC3339Nano)))
	if info.URL != "" {
		builder.WriteString(fmt.Sprintf("Upstream URL: %s\n", info.URL))
	}
	if info.Method != "" {
		builder.WriteString(fmt.Sprintf("HTTP Method: %s\n", info.Method))
	}
	if auth := formatAuthInfo(info); auth != "" {
		builder.WriteString(fmt.Sprintf("Auth: %s\n", auth))
	}
	builder.WriteString("\nHeaders:\n")
	writeHeaders(builder, info.Headers)
	builder.WriteString("\nBody:\n")
	if len(info.Body) > 0 {
		builder.WriteString(string(bytes.Clone(info.Body)))
	} else {
		builder.WriteString("<empty>")
	}
	builder.WriteString("\n\n")

	attempt := &upstreamAttempt{
		index:    index,
		request:  builder.String(),
		response: &strings.Builder{},
	}
	s.attempts = append(s.attempts, attempt)
}

// RecordResponseMetadata records status code and headers for the latest attempt.
func (s *LoggingStore) RecordResponseMetadata(status int, headers http.Header) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.attempts) == 0 {
		return
	}
	attempt := s.attempts[len(s.attempts)-1]

	ensureResponseIntro(attempt)

	if !attempt.statusWritten {
		attempt.response.WriteString(fmt.Sprintf("Status: %d %s\n", status, http.StatusText(status)))
		attempt.statusWritten = true
	}
	if !attempt.headersWritten {
		attempt.response.WriteString("Headers:\n")
		writeHeaders(attempt.response, headers)
		attempt.headersWritten = true
		attempt.response.WriteString("\n")
	}
}

// AppendResponseChunk appends a chunk to the latest attempt's response.
func (s *LoggingStore) AppendResponseChunk(chunk []byte) {
	if len(chunk) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.attempts) == 0 {
		// Create a dummy attempt if none exists (should not happen in normal flow)
		s.attempts = append(s.attempts, &upstreamAttempt{
			index:    1,
			request:  "=== API REQUEST 1 ===\n<missing>\n\n",
			response: &strings.Builder{},
		})
	}
	attempt := s.attempts[len(s.attempts)-1]

	ensureResponseIntro(attempt)

	if !attempt.headersWritten {
		attempt.response.WriteString("Headers:\n")
		writeHeaders(attempt.response, nil)
		attempt.headersWritten = true
		attempt.response.WriteString("\n")
	}
	if !attempt.bodyStarted {
		attempt.response.WriteString("Body:\n")
		attempt.bodyStarted = true
	}
	if attempt.bodyHasContent {
		attempt.response.WriteString("\n\n")
	}
	attempt.response.WriteString(string(bytes.TrimSpace(bytes.Clone(chunk))))
	attempt.bodyHasContent = true
}

// Finalize finalizes the logs and writes them to the Gin context.
func (s *LoggingStore) Finalize(ctx context.Context) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ginCtx := ginContextFrom(ctx)
	if ginCtx == nil {
		return
	}

	// Update Gin context for legacy compatibility (final aggregation only)
	ginCtx.Set(apiAttemptsKey, s.attempts)
	updateAggregatedRequest(ginCtx, s.attempts)
	updateAggregatedResponse(ginCtx, s.attempts)
}

// recordAPIResponseMetadata captures upstream response status/header information for the latest attempt.
func recordAPIResponseMetadata(ctx context.Context, cfg *config.Config, status int, headers http.Header) {
	if cfg == nil || !cfg.RequestLog {
		return
	}

	// Try to use thread-safe LoggingStore first
	if store, ok := loggingStoreFrom(ctx); ok {
		store.RecordResponseMetadata(status, headers)
		return
	}

	// Fallback to legacy Gin context logic
	ginCtx := ginContextFrom(ctx)
	if ginCtx == nil {
		return
	}
	attempts, attempt := ensureAttempt(ginCtx)
	ensureResponseIntro(attempt)

	if status > 0 && !attempt.statusWritten {
		attempt.response.WriteString(fmt.Sprintf("Status: %d\n", status))
		attempt.statusWritten = true
	}
	if !attempt.headersWritten {
		attempt.response.WriteString("Headers:\n")
		writeHeaders(attempt.response, headers)
		attempt.headersWritten = true
		attempt.response.WriteString("\n")
	}

	updateAggregatedResponse(ginCtx, attempts)
}

// recordAPIResponseError captures an upstream response error for the latest attempt.
func recordAPIResponseError(ctx context.Context, cfg *config.Config, err error) {
	if cfg == nil || !cfg.RequestLog || err == nil {
		return
	}

	// Try to use thread-safe LoggingStore first
	if store, ok := loggingStoreFrom(ctx); ok {
		store.mu.Lock()
		defer store.mu.Unlock()
		if len(store.attempts) > 0 {
			attempt := store.attempts[len(store.attempts)-1]
			if !attempt.errorWritten {
				attempt.response.WriteString(fmt.Sprintf("\nError: %v\n", err))
				attempt.errorWritten = true
			}
		}
		return
	}

	// Fallback to legacy Gin context logic
	ginCtx := ginContextFrom(ctx)
	if ginCtx == nil {
		return
	}
	attempts, attempt := ensureAttempt(ginCtx)
	ensureResponseIntro(attempt) // Ensure intro is written for legacy path too

	if attempt.bodyStarted && !attempt.bodyHasContent {
		// Ensure body does not stay empty marker if error arrives first.
		attempt.bodyStarted = false
	}
	if !attempt.errorWritten {
		attempt.response.WriteString(fmt.Sprintf("\nError: %v\n", err))
		attempt.errorWritten = true
		updateAggregatedResponse(ginCtx, attempts)
	}
}

// appendAPIResponseChunk adds a chunk of the upstream response body to the latest attempt.
func appendAPIResponseChunk(ctx context.Context, cfg *config.Config, chunk []byte) {
	if cfg == nil || !cfg.RequestLog || len(chunk) == 0 {
		return
	}

	// Try to use thread-safe LoggingStore first
	if store, ok := loggingStoreFrom(ctx); ok {
		store.AppendResponseChunk(chunk)
		return
	}

	// Fallback to legacy Gin context logic
	ginCtx := ginContextFrom(ctx)
	if ginCtx == nil {
		return
	}
	data := bytes.TrimSpace(bytes.Clone(chunk))
	if len(data) == 0 { // Re-check after trimming
		return
	}
	_, attempt := ensureAttempt(ginCtx)
	ensureResponseIntro(attempt)

	if !attempt.headersWritten {
		attempt.response.WriteString("Headers:\n")
		writeHeaders(attempt.response, nil)
		attempt.headersWritten = true
		attempt.response.WriteString("\n")
	}
	if !attempt.bodyStarted {
		attempt.response.WriteString("Body:\n")
		attempt.bodyStarted = true
	}
	if attempt.bodyHasContent {
		attempt.response.WriteString("\n\n")
	}
	attempt.response.WriteString(string(data))
	attempt.bodyHasContent = true
}

// FinalizeAPIResponse aggregates all upstream attempts into a single request/response log.
func FinalizeAPIResponse(ctx context.Context, cfg *config.Config) {
	if cfg == nil || !cfg.RequestLog {
		return
	}

	// Try to use thread-safe LoggingStore first
	if store, ok := loggingStoreFrom(ctx); ok {
		store.Finalize(ctx)
		return
	}

	// Fallback to legacy Gin context logic
	ginCtx := ginContextFrom(ctx)
	if ginCtx == nil {
		return
	}
	attempts := getAttempts(ginCtx)
	updateAggregatedRequest(ginCtx, attempts)
	updateAggregatedResponse(ginCtx, attempts)
}

func ginContextFrom(ctx context.Context) *gin.Context {
	ginCtx, _ := ctx.Value("gin").(*gin.Context)
	return ginCtx
}

func getAttempts(ginCtx *gin.Context) []*upstreamAttempt {
	if ginCtx == nil {
		return nil
	}
	if value, exists := ginCtx.Get(apiAttemptsKey); exists {
		if attempts, ok := value.([]*upstreamAttempt); ok {
			return attempts
		}
	}
	return nil
}

func ensureAttempt(ginCtx *gin.Context) ([]*upstreamAttempt, *upstreamAttempt) {
	attempts := getAttempts(ginCtx)
	if len(attempts) == 0 {
		attempt := &upstreamAttempt{
			index:    1,
			request:  "=== API REQUEST 1 ===\n<missing>\n\n",
			response: &strings.Builder{},
		}
		attempts = []*upstreamAttempt{attempt}
		ginCtx.Set(apiAttemptsKey, attempts)
		updateAggregatedRequest(ginCtx, attempts)
	}
	return attempts, attempts[len(attempts)-1]
}

func ensureResponseIntro(attempt *upstreamAttempt) {
	if attempt == nil || attempt.response == nil || attempt.responseIntroWritten {
		return
	}
	attempt.response.WriteString(fmt.Sprintf("=== API RESPONSE %d ===\n", attempt.index))
	attempt.response.WriteString(fmt.Sprintf("Timestamp: %s\n", time.Now().Format(time.RFC3339Nano)))
	attempt.response.WriteString("\n")
	attempt.responseIntroWritten = true
}

func updateAggregatedRequest(ginCtx *gin.Context, attempts []*upstreamAttempt) {
	if ginCtx == nil {
		return
	}
	var builder strings.Builder
	for _, attempt := range attempts {
		builder.WriteString(attempt.request)
	}
	ginCtx.Set(apiRequestKey, []byte(builder.String()))
}

func updateAggregatedResponse(ginCtx *gin.Context, attempts []*upstreamAttempt) {
	if ginCtx == nil {
		return
	}
	var builder strings.Builder
	for idx, attempt := range attempts {
		if attempt == nil || attempt.response == nil {
			continue
		}
		responseText := attempt.response.String()
		if responseText == "" {
			continue
		}
		builder.WriteString(responseText)
		if !strings.HasSuffix(responseText, "\n") {
			builder.WriteString("\n")
		}
		if idx < len(attempts)-1 {
			builder.WriteString("\n")
		}
	}
	ginCtx.Set(apiResponseKey, []byte(builder.String()))
}

func writeHeaders(builder *strings.Builder, headers http.Header) {
	if builder == nil {
		return
	}
	if len(headers) == 0 {
		builder.WriteString("<none>\n")
		return
	}
	keys := make([]string, 0, len(headers))
	for key := range headers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		values := headers[key]
		if len(values) == 0 {
			builder.WriteString(fmt.Sprintf("%s:\n", key))
			continue
		}
		for _, value := range values {
			masked := util.MaskSensitiveHeaderValue(key, value)
			builder.WriteString(fmt.Sprintf("%s: %s\n", key, masked))
		}
	}
}

func formatAuthInfo(info upstreamRequestLog) string {
	var parts []string
	if trimmed := strings.TrimSpace(info.Provider); trimmed != "" {
		parts = append(parts, fmt.Sprintf("provider=%s", trimmed))
	}
	if trimmed := strings.TrimSpace(info.AuthID); trimmed != "" {
		parts = append(parts, fmt.Sprintf("auth_id=%s", trimmed))
	}
	if trimmed := strings.TrimSpace(info.AuthLabel); trimmed != "" {
		parts = append(parts, fmt.Sprintf("label=%s", trimmed))
	}

	authType := strings.ToLower(strings.TrimSpace(info.AuthType))
	authValue := strings.TrimSpace(info.AuthValue)
	switch authType {
	case "api_key":
		if authValue != "" {
			parts = append(parts, fmt.Sprintf("type=api_key value=%s", util.HideAPIKey(authValue)))
		} else {
			parts = append(parts, "type=api_key")
		}
	case "oauth":
		if authValue != "" {
			parts = append(parts, fmt.Sprintf("type=oauth account=%s", authValue))
		} else {
			parts = append(parts, "type=oauth")
		}
	default:
		if authType != "" {
			if authValue != "" {
				parts = append(parts, fmt.Sprintf("type=%s value=%s", authType, authValue))
			} else {
				parts = append(parts, fmt.Sprintf("type=%s", authType))
			}
		}
	}

	return strings.Join(parts, ", ")
}

func summarizeErrorBody(contentType string, body []byte) string {
	isHTML := strings.Contains(strings.ToLower(contentType), "text/html")
	if !isHTML {
		trimmed := bytes.TrimSpace(bytes.ToLower(body))
		if bytes.HasPrefix(trimmed, []byte("<!doctype html")) || bytes.HasPrefix(trimmed, []byte("<html")) {
			isHTML = true
		}
	}
	if isHTML {
		if title := extractHTMLTitle(body); title != "" {
			return title
		}
		return "[html body omitted]"
	}
	return string(body)
}

func extractHTMLTitle(body []byte) string {
	lower := bytes.ToLower(body)
	start := bytes.Index(lower, []byte("<title"))
	if start == -1 {
		return ""
	}
	gt := bytes.IndexByte(lower[start:], '>')
	if gt == -1 {
		return ""
	}
	start += gt + 1
	end := bytes.Index(lower[start:], []byte("</title>"))
	if end == -1 {
		return ""
	}
	title := string(body[start : start+end])
	title = html.UnescapeString(title)
	title = strings.TrimSpace(title)
	if title == "" {
		return ""
	}
	return strings.Join(strings.Fields(title), " ")
}
