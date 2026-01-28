// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package executor provides runtime execution capabilities for various AI service providers.
// This file implements the Vertex AI Gemini executor that talks to Google Vertex AI
// endpoints using service account credentials or API keys.
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

	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	vertexauth "github.com/traylinx/switchAILocal/internal/auth/vertex"
	"github.com/traylinx/switchAILocal/internal/config"
	"github.com/traylinx/switchAILocal/internal/constant"
	"github.com/traylinx/switchAILocal/internal/util"
	switchailocalauth "github.com/traylinx/switchAILocal/sdk/switchailocal/auth"
	switchailocalexecutor "github.com/traylinx/switchAILocal/sdk/switchailocal/executor"
	sdktranslator "github.com/traylinx/switchAILocal/sdk/translator"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const (
	// vertexAPIVersion aligns with current public Vertex Generative AI API.
	vertexAPIVersion = "v1"
)

// GeminiVertexExecutor sends requests to Vertex AI Gemini endpoints using service account credentials.
type GeminiVertexExecutor struct {
	cfg *config.Config
}

// NewGeminiVertexExecutor creates a new Vertex AI Gemini executor instance.
//
// Parameters:
//   - cfg: The application configuration
//
// Returns:
//   - *GeminiVertexExecutor: A new Vertex AI Gemini executor instance
func NewGeminiVertexExecutor(cfg *config.Config) *GeminiVertexExecutor {
	return &GeminiVertexExecutor{cfg: cfg}
}

// Identifier returns the executor identifier.
func (e *GeminiVertexExecutor) Identifier() string { return "vertex" }

// PrepareRequest prepares the HTTP request for execution (no-op for Vertex).
func (e *GeminiVertexExecutor) PrepareRequest(_ *http.Request, _ *switchailocalauth.Auth) error {
	return nil
}

// Execute performs a non-streaming request to the Vertex AI API.
func (e *GeminiVertexExecutor) Execute(ctx context.Context, auth *switchailocalauth.Auth, req switchailocalexecutor.Request, opts switchailocalexecutor.Options) (resp switchailocalexecutor.Response, err error) {
	// Try API key authentication first
	apiKey, baseURL := vertexAPICreds(auth)

	// If no API key found, fall back to service account authentication
	if apiKey == "" {
		projectID, location, saJSON, errCreds := vertexCreds(auth)
		if errCreds != nil {
			return resp, errCreds
		}
		return e.executeWithServiceAccount(ctx, auth, req, opts, projectID, location, saJSON)
	}

	// Use API key authentication
	return e.executeWithAPIKey(ctx, auth, req, opts, apiKey, baseURL)
}

// ExecuteStream performs a streaming request to the Vertex AI API.
func (e *GeminiVertexExecutor) ExecuteStream(ctx context.Context, auth *switchailocalauth.Auth, req switchailocalexecutor.Request, opts switchailocalexecutor.Options) (stream <-chan switchailocalexecutor.StreamChunk, err error) {
	// Try API key authentication first
	apiKey, baseURL := vertexAPICreds(auth)

	// If no API key found, fall back to service account authentication
	if apiKey == "" {
		projectID, location, saJSON, errCreds := vertexCreds(auth)
		if errCreds != nil {
			return nil, errCreds
		}
		return e.executeStreamWithServiceAccount(ctx, auth, req, opts, projectID, location, saJSON)
	}

	// Use API key authentication
	return e.executeStreamWithAPIKey(ctx, auth, req, opts, apiKey, baseURL)
}

// CountTokens counts tokens for the given request using the Vertex AI API.
func (e *GeminiVertexExecutor) CountTokens(ctx context.Context, auth *switchailocalauth.Auth, req switchailocalexecutor.Request, opts switchailocalexecutor.Options) (switchailocalexecutor.Response, error) {
	// Try API key authentication first
	apiKey, baseURL := vertexAPICreds(auth)

	// If no API key found, fall back to service account authentication
	if apiKey == "" {
		projectID, location, saJSON, errCreds := vertexCreds(auth)
		if errCreds != nil {
			return switchailocalexecutor.Response{}, errCreds
		}
		return e.countTokensWithServiceAccount(ctx, auth, req, opts, projectID, location, saJSON)
	}

	// Use API key authentication
	return e.countTokensWithAPIKey(ctx, auth, req, opts, apiKey, baseURL)
}

// Refresh refreshes the authentication credentials (no-op for Vertex).
func (e *GeminiVertexExecutor) Refresh(_ context.Context, auth *switchailocalauth.Auth) (*switchailocalauth.Auth, error) {
	return auth, nil
}

// executeWithServiceAccount handles authentication using service account credentials.
// This method contains the original service account authentication logic.
func (e *GeminiVertexExecutor) executeWithServiceAccount(ctx context.Context, auth *switchailocalauth.Auth, req switchailocalexecutor.Request, opts switchailocalexecutor.Options, projectID, location string, saJSON []byte) (resp switchailocalexecutor.Response, err error) {
	reporter := newUsageReporter(ctx, e.Identifier(), req.Model, auth)
	defer reporter.trackFailure(ctx, &err)

	upstreamModel := util.ResolveOriginalModel(req.Model, req.Metadata)

	from := opts.SourceFormat
	to := sdktranslator.FromString("gemini")
	body := sdktranslator.TranslateRequest(from, to, req.Model, bytes.Clone(req.Payload), false)
	if budgetOverride, includeOverride, ok := util.ResolveThinkingConfigFromMetadata(req.Model, req.Metadata); ok && util.ModelSupportsThinking(req.Model) {
		if budgetOverride != nil {
			norm := util.NormalizeThinkingBudget(req.Model, *budgetOverride)
			budgetOverride = &norm
		}
		body = util.ApplyGeminiThinkingConfig(body, budgetOverride, includeOverride)
	}
	body = util.ApplyDefaultThinkingIfNeeded(req.Model, body)
	body = util.NormalizeGeminiThinkingBudget(req.Model, body)
	body = util.StripThinkingConfigIfUnsupported(req.Model, body)
	body = fixGeminiImageAspectRatio(req.Model, body)
	body = applyPayloadConfig(e.cfg, req.Model, body)
	body, _ = sjson.SetBytes(body, "model", upstreamModel)

	action := "generateContent"
	if req.Metadata != nil {
		if a, _ := req.Metadata["action"].(string); a == "countTokens" {
			action = "countTokens"
		}
	}
	baseURL := vertexBaseURL(location)
	url := fmt.Sprintf("%s/%s/projects/%s/locations/%s/publishers/google/models/%s:%s", baseURL, vertexAPIVersion, projectID, location, upstreamModel, action)
	if opts.Alt != "" && action != "countTokens" {
		url = url + fmt.Sprintf("?$alt=%s", opts.Alt)
	}
	body, _ = sjson.DeleteBytes(body, "session_id")

	httpReq, errNewReq := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if errNewReq != nil {
		return resp, errNewReq
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if token, errTok := vertexAccessToken(ctx, e.cfg, auth, saJSON); errTok == nil && token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+token)
	} else if errTok != nil {
		log.Errorf("vertex executor: access token error: %v", errTok)
		return resp, statusErr{code: 500, msg: "internal server error"}
	}
	applyGeminiHeaders(httpReq, auth)

	var authID, authLabel, authType, authValue string
	if auth != nil {
		authID = auth.ID
		authLabel = auth.Label
		authType, authValue = auth.AccountInfo()
	}
	recordAPIRequest(ctx, e.cfg, upstreamRequestLog{
		URL:       url,
		Method:    http.MethodPost,
		Headers:   httpReq.Header.Clone(),
		Body:      body,
		Provider:  e.Identifier(),
		AuthID:    authID,
		AuthLabel: authLabel,
		AuthType:  authType,
		AuthValue: authValue,
	})

	httpClient := newProxyAwareHTTPClient(ctx, e.cfg, auth, 0)
	httpResp, errDo := httpClient.Do(httpReq)
	if errDo != nil {
		recordAPIResponseError(ctx, e.cfg, errDo)
		return resp, errDo
	}
	defer func() {
		if errClose := httpResp.Body.Close(); errClose != nil {
			log.Errorf("vertex executor: close response body error: %v", errClose)
		}
	}()
	recordAPIResponseMetadata(ctx, e.cfg, httpResp.StatusCode, httpResp.Header.Clone())
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		b, _ := io.ReadAll(httpResp.Body)
		appendAPIResponseChunk(ctx, e.cfg, b)
		log.Debugf("request error, error status: %d, error body: %s", httpResp.StatusCode, summarizeErrorBody(httpResp.Header.Get("Content-Type"), b))
		err = statusErr{code: httpResp.StatusCode, msg: string(b)}
		return resp, err
	}
	data, errRead := io.ReadAll(httpResp.Body)
	if errRead != nil {
		recordAPIResponseError(ctx, e.cfg, errRead)
		return resp, errRead
	}
	appendAPIResponseChunk(ctx, e.cfg, data)
	reporter.publish(ctx, parseGeminiUsage(data))
	var param any
	out := sdktranslator.TranslateNonStream(ctx, to, from, req.Model, bytes.Clone(opts.OriginalRequest), body, data, &param)
	resp = switchailocalexecutor.Response{Payload: []byte(out)}
	return resp, nil
}

// executeWithAPIKey handles authentication using API key credentials.
func (e *GeminiVertexExecutor) executeWithAPIKey(ctx context.Context, auth *switchailocalauth.Auth, req switchailocalexecutor.Request, opts switchailocalexecutor.Options, apiKey, baseURL string) (resp switchailocalexecutor.Response, err error) {
	reporter := newUsageReporter(ctx, e.Identifier(), req.Model, auth)
	defer reporter.trackFailure(ctx, &err)

	upstreamModel := util.ResolveOriginalModel(req.Model, req.Metadata)

	from := opts.SourceFormat
	to := sdktranslator.FromString("gemini")
	body := sdktranslator.TranslateRequest(from, to, req.Model, bytes.Clone(req.Payload), false)
	if budgetOverride, includeOverride, ok := util.ResolveThinkingConfigFromMetadata(req.Model, req.Metadata); ok && util.ModelSupportsThinking(req.Model) {
		if budgetOverride != nil {
			norm := util.NormalizeThinkingBudget(req.Model, *budgetOverride)
			budgetOverride = &norm
		}
		body = util.ApplyGeminiThinkingConfig(body, budgetOverride, includeOverride)
	}
	body = util.ApplyDefaultThinkingIfNeeded(req.Model, body)
	body = util.NormalizeGeminiThinkingBudget(req.Model, body)
	body = util.StripThinkingConfigIfUnsupported(req.Model, body)
	body = fixGeminiImageAspectRatio(req.Model, body)
	body = applyPayloadConfig(e.cfg, req.Model, body)
	body, _ = sjson.SetBytes(body, "model", upstreamModel)

	action := "generateContent"
	if req.Metadata != nil {
		if a, _ := req.Metadata["action"].(string); a == "countTokens" {
			action = "countTokens"
		}
	}

	// For API key auth, use simpler URL format without project/location
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com"
	}
	url := fmt.Sprintf("%s/%s/publishers/google/models/%s:%s", baseURL, vertexAPIVersion, upstreamModel, action)
	if opts.Alt != "" && action != "countTokens" {
		url = url + fmt.Sprintf("?$alt=%s", opts.Alt)
	}
	body, _ = sjson.DeleteBytes(body, "session_id")

	httpReq, errNewReq := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if errNewReq != nil {
		return resp, errNewReq
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		httpReq.Header.Set("x-goog-api-key", apiKey)
	}
	applyGeminiHeaders(httpReq, auth)

	var authID, authLabel, authType, authValue string
	if auth != nil {
		authID = auth.ID
		authLabel = auth.Label
		authType, authValue = auth.AccountInfo()
	}
	recordAPIRequest(ctx, e.cfg, upstreamRequestLog{
		URL:       url,
		Method:    http.MethodPost,
		Headers:   httpReq.Header.Clone(),
		Body:      body,
		Provider:  e.Identifier(),
		AuthID:    authID,
		AuthLabel: authLabel,
		AuthType:  authType,
		AuthValue: authValue,
	})

	httpClient := newProxyAwareHTTPClient(ctx, e.cfg, auth, 0)
	httpResp, errDo := httpClient.Do(httpReq)
	if errDo != nil {
		recordAPIResponseError(ctx, e.cfg, errDo)
		return resp, errDo
	}
	defer func() {
		if errClose := httpResp.Body.Close(); errClose != nil {
			log.Errorf("vertex executor: close response body error: %v", errClose)
		}
	}()
	recordAPIResponseMetadata(ctx, e.cfg, httpResp.StatusCode, httpResp.Header.Clone())
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		b, _ := io.ReadAll(httpResp.Body)
		appendAPIResponseChunk(ctx, e.cfg, b)
		log.Debugf("request error, error status: %d, error body: %s", httpResp.StatusCode, summarizeErrorBody(httpResp.Header.Get("Content-Type"), b))
		err = statusErr{code: httpResp.StatusCode, msg: string(b)}
		return resp, err
	}
	data, errRead := io.ReadAll(httpResp.Body)
	if errRead != nil {
		recordAPIResponseError(ctx, e.cfg, errRead)
		return resp, errRead
	}
	appendAPIResponseChunk(ctx, e.cfg, data)
	reporter.publish(ctx, parseGeminiUsage(data))
	var param any
	out := sdktranslator.TranslateNonStream(ctx, to, from, req.Model, bytes.Clone(opts.OriginalRequest), body, data, &param)
	resp = switchailocalexecutor.Response{Payload: []byte(out)}
	return resp, nil
}

// executeStreamWithServiceAccount handles streaming authentication using service account credentials.
func (e *GeminiVertexExecutor) executeStreamWithServiceAccount(ctx context.Context, auth *switchailocalauth.Auth, req switchailocalexecutor.Request, opts switchailocalexecutor.Options, projectID, location string, saJSON []byte) (stream <-chan switchailocalexecutor.StreamChunk, err error) {
	reporter := newUsageReporter(ctx, e.Identifier(), req.Model, auth)
	defer reporter.trackFailure(ctx, &err)

	upstreamModel := util.ResolveOriginalModel(req.Model, req.Metadata)

	from := opts.SourceFormat
	to := sdktranslator.FromString("gemini")
	body := sdktranslator.TranslateRequest(from, to, req.Model, bytes.Clone(req.Payload), true)
	if budgetOverride, includeOverride, ok := util.ResolveThinkingConfigFromMetadata(req.Model, req.Metadata); ok && util.ModelSupportsThinking(req.Model) {
		if budgetOverride != nil {
			norm := util.NormalizeThinkingBudget(req.Model, *budgetOverride)
			budgetOverride = &norm
		}
		body = util.ApplyGeminiThinkingConfig(body, budgetOverride, includeOverride)
	}
	body = util.ApplyDefaultThinkingIfNeeded(req.Model, body)
	body = util.NormalizeGeminiThinkingBudget(req.Model, body)
	body = util.StripThinkingConfigIfUnsupported(req.Model, body)
	body = fixGeminiImageAspectRatio(req.Model, body)
	body = applyPayloadConfig(e.cfg, req.Model, body)
	body, _ = sjson.SetBytes(body, "model", upstreamModel)

	baseURL := vertexBaseURL(location)
	url := fmt.Sprintf("%s/%s/projects/%s/locations/%s/publishers/google/models/%s:%s", baseURL, vertexAPIVersion, projectID, location, upstreamModel, "streamGenerateContent")
	if opts.Alt == "" {
		url = url + "?alt=sse"
	} else {
		url = url + fmt.Sprintf("?$alt=%s", opts.Alt)
	}
	body, _ = sjson.DeleteBytes(body, "session_id")

	httpReq, errNewReq := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if errNewReq != nil {
		return nil, errNewReq
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if token, errTok := vertexAccessToken(ctx, e.cfg, auth, saJSON); errTok == nil && token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+token)
	} else if errTok != nil {
		log.Errorf("vertex executor: access token error: %v", errTok)
		return nil, statusErr{code: 500, msg: "internal server error"}
	}
	applyGeminiHeaders(httpReq, auth)

	var authID, authLabel, authType, authValue string
	if auth != nil {
		authID = auth.ID
		authLabel = auth.Label
		authType, authValue = auth.AccountInfo()
	}
	recordAPIRequest(ctx, e.cfg, upstreamRequestLog{
		URL:       url,
		Method:    http.MethodPost,
		Headers:   httpReq.Header.Clone(),
		Body:      body,
		Provider:  e.Identifier(),
		AuthID:    authID,
		AuthLabel: authLabel,
		AuthType:  authType,
		AuthValue: authValue,
	})

	httpClient := newProxyAwareHTTPClient(ctx, e.cfg, auth, 0)
	httpResp, errDo := httpClient.Do(httpReq)
	if errDo != nil {
		recordAPIResponseError(ctx, e.cfg, errDo)
		return nil, errDo
	}
	recordAPIResponseMetadata(ctx, e.cfg, httpResp.StatusCode, httpResp.Header.Clone())
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		b, _ := io.ReadAll(httpResp.Body)
		appendAPIResponseChunk(ctx, e.cfg, b)
		log.Debugf("request error, error status: %d, error body: %s", httpResp.StatusCode, summarizeErrorBody(httpResp.Header.Get("Content-Type"), b))
		if errClose := httpResp.Body.Close(); errClose != nil {
			log.Errorf("vertex executor: close response body error: %v", errClose)
		}
		return nil, statusErr{code: httpResp.StatusCode, msg: string(b)}
	}

	out := make(chan switchailocalexecutor.StreamChunk)
	stream = out
	go func() {
		defer close(out)
		defer func() {
			FinalizeAPIResponse(ctx, e.cfg)
			if errClose := httpResp.Body.Close(); errClose != nil {
				log.Errorf("vertex executor: close response body error: %v", errClose)
			}
		}()
		scanner := bufio.NewScanner(httpResp.Body)
		scanner.Buffer(nil, constant.MaxStreamingScannerBuffer)
		var param any
		for scanner.Scan() {
			line := scanner.Bytes()
			appendAPIResponseChunk(ctx, e.cfg, line)
			if detail, ok := parseGeminiStreamUsage(line); ok {
				reporter.publish(ctx, detail)
			}
			lines := sdktranslator.TranslateStream(ctx, to, from, req.Model, bytes.Clone(opts.OriginalRequest), body, bytes.Clone(line), &param)
			for i := range lines {
				out <- switchailocalexecutor.StreamChunk{Payload: []byte(lines[i])}
			}
		}
		lines := sdktranslator.TranslateStream(ctx, to, from, req.Model, bytes.Clone(opts.OriginalRequest), body, []byte("[DONE]"), &param)
		for i := range lines {
			out <- switchailocalexecutor.StreamChunk{Payload: []byte(lines[i])}
		}
		if errScan := scanner.Err(); errScan != nil {
			recordAPIResponseError(ctx, e.cfg, errScan)
			reporter.publishFailure(ctx)
			out <- switchailocalexecutor.StreamChunk{Err: errScan}
		}
	}()
	return stream, nil
}

// executeStreamWithAPIKey handles streaming authentication using API key credentials.
func (e *GeminiVertexExecutor) executeStreamWithAPIKey(ctx context.Context, auth *switchailocalauth.Auth, req switchailocalexecutor.Request, opts switchailocalexecutor.Options, apiKey, baseURL string) (stream <-chan switchailocalexecutor.StreamChunk, err error) {
	reporter := newUsageReporter(ctx, e.Identifier(), req.Model, auth)
	defer reporter.trackFailure(ctx, &err)

	upstreamModel := util.ResolveOriginalModel(req.Model, req.Metadata)

	from := opts.SourceFormat
	to := sdktranslator.FromString("gemini")
	body := sdktranslator.TranslateRequest(from, to, req.Model, bytes.Clone(req.Payload), true)
	if budgetOverride, includeOverride, ok := util.ResolveThinkingConfigFromMetadata(req.Model, req.Metadata); ok && util.ModelSupportsThinking(req.Model) {
		if budgetOverride != nil {
			norm := util.NormalizeThinkingBudget(req.Model, *budgetOverride)
			budgetOverride = &norm
		}
		body = util.ApplyGeminiThinkingConfig(body, budgetOverride, includeOverride)
	}
	body = util.ApplyDefaultThinkingIfNeeded(req.Model, body)
	body = util.NormalizeGeminiThinkingBudget(req.Model, body)
	body = util.StripThinkingConfigIfUnsupported(req.Model, body)
	body = fixGeminiImageAspectRatio(req.Model, body)
	body = applyPayloadConfig(e.cfg, req.Model, body)
	body, _ = sjson.SetBytes(body, "model", upstreamModel)

	// For API key auth, use simpler URL format without project/location
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com"
	}
	url := fmt.Sprintf("%s/%s/publishers/google/models/%s:%s", baseURL, vertexAPIVersion, upstreamModel, "streamGenerateContent")
	if opts.Alt == "" {
		url = url + "?alt=sse"
	} else {
		url = url + fmt.Sprintf("?$alt=%s", opts.Alt)
	}
	body, _ = sjson.DeleteBytes(body, "session_id")

	httpReq, errNewReq := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if errNewReq != nil {
		return nil, errNewReq
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		httpReq.Header.Set("x-goog-api-key", apiKey)
	}
	applyGeminiHeaders(httpReq, auth)

	var authID, authLabel, authType, authValue string
	if auth != nil {
		authID = auth.ID
		authLabel = auth.Label
		authType, authValue = auth.AccountInfo()
	}
	recordAPIRequest(ctx, e.cfg, upstreamRequestLog{
		URL:       url,
		Method:    http.MethodPost,
		Headers:   httpReq.Header.Clone(),
		Body:      body,
		Provider:  e.Identifier(),
		AuthID:    authID,
		AuthLabel: authLabel,
		AuthType:  authType,
		AuthValue: authValue,
	})

	httpClient := newProxyAwareHTTPClient(ctx, e.cfg, auth, 0)
	httpResp, errDo := httpClient.Do(httpReq)
	if errDo != nil {
		recordAPIResponseError(ctx, e.cfg, errDo)
		return nil, errDo
	}
	recordAPIResponseMetadata(ctx, e.cfg, httpResp.StatusCode, httpResp.Header.Clone())
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		b, _ := io.ReadAll(httpResp.Body)
		appendAPIResponseChunk(ctx, e.cfg, b)
		log.Debugf("request error, error status: %d, error body: %s", httpResp.StatusCode, summarizeErrorBody(httpResp.Header.Get("Content-Type"), b))
		if errClose := httpResp.Body.Close(); errClose != nil {
			log.Errorf("vertex executor: close response body error: %v", errClose)
		}
		return nil, statusErr{code: httpResp.StatusCode, msg: string(b)}
	}

	out := make(chan switchailocalexecutor.StreamChunk)
	stream = out
	go func() {
		defer close(out)
		defer func() {
			FinalizeAPIResponse(ctx, e.cfg)
			if errClose := httpResp.Body.Close(); errClose != nil {
				log.Errorf("vertex executor: close response body error: %v", errClose)
			}
		}()
		scanner := bufio.NewScanner(httpResp.Body)
		scanner.Buffer(nil, constant.MaxStreamingScannerBuffer)
		var param any
		for scanner.Scan() {
			line := scanner.Bytes()
			appendAPIResponseChunk(ctx, e.cfg, line)
			if detail, ok := parseGeminiStreamUsage(line); ok {
				reporter.publish(ctx, detail)
			}
			lines := sdktranslator.TranslateStream(ctx, to, from, req.Model, bytes.Clone(opts.OriginalRequest), body, bytes.Clone(line), &param)
			for i := range lines {
				out <- switchailocalexecutor.StreamChunk{Payload: []byte(lines[i])}
			}
		}
		lines := sdktranslator.TranslateStream(ctx, to, from, req.Model, bytes.Clone(opts.OriginalRequest), body, []byte("[DONE]"), &param)
		for i := range lines {
			out <- switchailocalexecutor.StreamChunk{Payload: []byte(lines[i])}
		}
		if errScan := scanner.Err(); errScan != nil {
			recordAPIResponseError(ctx, e.cfg, errScan)
			reporter.publishFailure(ctx)
			out <- switchailocalexecutor.StreamChunk{Err: errScan}
		}
	}()
	return stream, nil
}

// countTokensWithServiceAccount counts tokens using service account credentials.
func (e *GeminiVertexExecutor) countTokensWithServiceAccount(ctx context.Context, auth *switchailocalauth.Auth, req switchailocalexecutor.Request, opts switchailocalexecutor.Options, projectID, location string, saJSON []byte) (switchailocalexecutor.Response, error) {
	upstreamModel := util.ResolveOriginalModel(req.Model, req.Metadata)

	from := opts.SourceFormat
	to := sdktranslator.FromString("gemini")
	translatedReq := sdktranslator.TranslateRequest(from, to, req.Model, bytes.Clone(req.Payload), false)
	if budgetOverride, includeOverride, ok := util.ResolveThinkingConfigFromMetadata(req.Model, req.Metadata); ok && util.ModelSupportsThinking(req.Model) {
		if budgetOverride != nil {
			norm := util.NormalizeThinkingBudget(req.Model, *budgetOverride)
			budgetOverride = &norm
		}
		translatedReq = util.ApplyGeminiThinkingConfig(translatedReq, budgetOverride, includeOverride)
	}
	translatedReq = util.StripThinkingConfigIfUnsupported(req.Model, translatedReq)
	translatedReq = fixGeminiImageAspectRatio(req.Model, translatedReq)
	translatedReq, _ = sjson.SetBytes(translatedReq, "model", upstreamModel)
	respCtx := context.WithValue(ctx, altKey, opts.Alt)
	translatedReq, _ = sjson.DeleteBytes(translatedReq, "tools")
	translatedReq, _ = sjson.DeleteBytes(translatedReq, "generationConfig")
	translatedReq, _ = sjson.DeleteBytes(translatedReq, "safetySettings")

	baseURL := vertexBaseURL(location)
	url := fmt.Sprintf("%s/%s/projects/%s/locations/%s/publishers/google/models/%s:%s", baseURL, vertexAPIVersion, projectID, location, upstreamModel, "countTokens")

	httpReq, errNewReq := http.NewRequestWithContext(respCtx, http.MethodPost, url, bytes.NewReader(translatedReq))
	if errNewReq != nil {
		return switchailocalexecutor.Response{}, errNewReq
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if token, errTok := vertexAccessToken(ctx, e.cfg, auth, saJSON); errTok == nil && token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+token)
	} else if errTok != nil {
		log.Errorf("vertex executor: access token error: %v", errTok)
		return switchailocalexecutor.Response{}, statusErr{code: 500, msg: "internal server error"}
	}
	applyGeminiHeaders(httpReq, auth)

	var authID, authLabel, authType, authValue string
	if auth != nil {
		authID = auth.ID
		authLabel = auth.Label
		authType, authValue = auth.AccountInfo()
	}
	recordAPIRequest(ctx, e.cfg, upstreamRequestLog{
		URL:       url,
		Method:    http.MethodPost,
		Headers:   httpReq.Header.Clone(),
		Body:      translatedReq,
		Provider:  e.Identifier(),
		AuthID:    authID,
		AuthLabel: authLabel,
		AuthType:  authType,
		AuthValue: authValue,
	})

	httpClient := newProxyAwareHTTPClient(ctx, e.cfg, auth, 0)
	httpResp, errDo := httpClient.Do(httpReq)
	if errDo != nil {
		recordAPIResponseError(ctx, e.cfg, errDo)
		return switchailocalexecutor.Response{}, errDo
	}
	defer func() {
		if errClose := httpResp.Body.Close(); errClose != nil {
			log.Errorf("vertex executor: close response body error: %v", errClose)
		}
	}()
	recordAPIResponseMetadata(ctx, e.cfg, httpResp.StatusCode, httpResp.Header.Clone())
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		b, _ := io.ReadAll(httpResp.Body)
		appendAPIResponseChunk(ctx, e.cfg, b)
		log.Debugf("request error, error status: %d, error body: %s", httpResp.StatusCode, summarizeErrorBody(httpResp.Header.Get("Content-Type"), b))
		return switchailocalexecutor.Response{}, statusErr{code: httpResp.StatusCode, msg: string(b)}
	}
	data, errRead := io.ReadAll(httpResp.Body)
	if errRead != nil {
		recordAPIResponseError(ctx, e.cfg, errRead)
		return switchailocalexecutor.Response{}, errRead
	}
	appendAPIResponseChunk(ctx, e.cfg, data)
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		log.Debugf("request error, error status: %d, error body: %s", httpResp.StatusCode, summarizeErrorBody(httpResp.Header.Get("Content-Type"), data))
		return switchailocalexecutor.Response{}, statusErr{code: httpResp.StatusCode, msg: string(data)}
	}
	count := gjson.GetBytes(data, "totalTokens").Int()
	out := sdktranslator.TranslateTokenCount(ctx, to, from, count, data)
	return switchailocalexecutor.Response{Payload: []byte(out)}, nil
}

// countTokensWithAPIKey handles token counting using API key credentials.
func (e *GeminiVertexExecutor) countTokensWithAPIKey(ctx context.Context, auth *switchailocalauth.Auth, req switchailocalexecutor.Request, opts switchailocalexecutor.Options, apiKey, baseURL string) (switchailocalexecutor.Response, error) {
	upstreamModel := util.ResolveOriginalModel(req.Model, req.Metadata)

	from := opts.SourceFormat
	to := sdktranslator.FromString("gemini")
	translatedReq := sdktranslator.TranslateRequest(from, to, req.Model, bytes.Clone(req.Payload), false)
	if budgetOverride, includeOverride, ok := util.ResolveThinkingConfigFromMetadata(req.Model, req.Metadata); ok && util.ModelSupportsThinking(req.Model) {
		if budgetOverride != nil {
			norm := util.NormalizeThinkingBudget(req.Model, *budgetOverride)
			budgetOverride = &norm
		}
		translatedReq = util.ApplyGeminiThinkingConfig(translatedReq, budgetOverride, includeOverride)
	}
	translatedReq = util.StripThinkingConfigIfUnsupported(req.Model, translatedReq)
	translatedReq = fixGeminiImageAspectRatio(req.Model, translatedReq)
	translatedReq, _ = sjson.SetBytes(translatedReq, "model", upstreamModel)
	respCtx := context.WithValue(ctx, altKey, opts.Alt)
	translatedReq, _ = sjson.DeleteBytes(translatedReq, "tools")
	translatedReq, _ = sjson.DeleteBytes(translatedReq, "generationConfig")
	translatedReq, _ = sjson.DeleteBytes(translatedReq, "safetySettings")

	// For API key auth, use simpler URL format without project/location
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com"
	}
	url := fmt.Sprintf("%s/%s/publishers/google/models/%s:%s", baseURL, vertexAPIVersion, req.Model, "countTokens")

	httpReq, errNewReq := http.NewRequestWithContext(respCtx, http.MethodPost, url, bytes.NewReader(translatedReq))
	if errNewReq != nil {
		return switchailocalexecutor.Response{}, errNewReq
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		httpReq.Header.Set("x-goog-api-key", apiKey)
	}
	applyGeminiHeaders(httpReq, auth)

	var authID, authLabel, authType, authValue string
	if auth != nil {
		authID = auth.ID
		authLabel = auth.Label
		authType, authValue = auth.AccountInfo()
	}
	recordAPIRequest(ctx, e.cfg, upstreamRequestLog{
		URL:       url,
		Method:    http.MethodPost,
		Headers:   httpReq.Header.Clone(),
		Body:      translatedReq,
		Provider:  e.Identifier(),
		AuthID:    authID,
		AuthLabel: authLabel,
		AuthType:  authType,
		AuthValue: authValue,
	})

	httpClient := newProxyAwareHTTPClient(ctx, e.cfg, auth, 0)
	httpResp, errDo := httpClient.Do(httpReq)
	if errDo != nil {
		recordAPIResponseError(ctx, e.cfg, errDo)
		return switchailocalexecutor.Response{}, errDo
	}
	defer func() {
		if errClose := httpResp.Body.Close(); errClose != nil {
			log.Errorf("vertex executor: close response body error: %v", errClose)
		}
	}()
	recordAPIResponseMetadata(ctx, e.cfg, httpResp.StatusCode, httpResp.Header.Clone())
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		b, _ := io.ReadAll(httpResp.Body)
		appendAPIResponseChunk(ctx, e.cfg, b)
		log.Debugf("request error, error status: %d, error body: %s", httpResp.StatusCode, summarizeErrorBody(httpResp.Header.Get("Content-Type"), b))
		return switchailocalexecutor.Response{}, statusErr{code: httpResp.StatusCode, msg: string(b)}
	}
	data, errRead := io.ReadAll(httpResp.Body)
	if errRead != nil {
		recordAPIResponseError(ctx, e.cfg, errRead)
		return switchailocalexecutor.Response{}, errRead
	}
	appendAPIResponseChunk(ctx, e.cfg, data)
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		log.Debugf("request error, error status: %d, error body: %s", httpResp.StatusCode, summarizeErrorBody(httpResp.Header.Get("Content-Type"), data))
		return switchailocalexecutor.Response{}, statusErr{code: httpResp.StatusCode, msg: string(data)}
	}
	count := gjson.GetBytes(data, "totalTokens").Int()
	out := sdktranslator.TranslateTokenCount(ctx, to, from, count, data)
	return switchailocalexecutor.Response{Payload: []byte(out)}, nil
}

// vertexCreds extracts project, location and raw service account JSON from auth metadata.
func vertexCreds(a *switchailocalauth.Auth) (projectID, location string, serviceAccountJSON []byte, err error) {
	if a == nil || a.Metadata == nil {
		return "", "", nil, fmt.Errorf("vertex executor: missing auth metadata")
	}
	if v, ok := a.Metadata["project_id"].(string); ok {
		projectID = strings.TrimSpace(v)
	}
	if projectID == "" {
		// Some service accounts may use "project"; still prefer standard field
		if v, ok := a.Metadata["project"].(string); ok {
			projectID = strings.TrimSpace(v)
		}
	}
	if projectID == "" {
		return "", "", nil, fmt.Errorf("vertex executor: missing project_id in credentials")
	}
	if v, ok := a.Metadata["location"].(string); ok && strings.TrimSpace(v) != "" {
		location = strings.TrimSpace(v)
	} else {
		location = "us-central1"
	}
	var sa map[string]any
	if raw, ok := a.Metadata["service_account"].(map[string]any); ok {
		sa = raw
	}
	if sa == nil {
		return "", "", nil, fmt.Errorf("vertex executor: missing service_account in credentials")
	}
	normalized, errNorm := vertexauth.NormalizeServiceAccountMap(sa)
	if errNorm != nil {
		return "", "", nil, fmt.Errorf("vertex executor: %w", errNorm)
	}
	saJSON, errMarshal := json.Marshal(normalized)
	if errMarshal != nil {
		return "", "", nil, fmt.Errorf("vertex executor: marshal service_account failed: %w", errMarshal)
	}
	return projectID, location, saJSON, nil
}

// vertexAPICreds extracts API key and base URL from auth attributes following the claudeCreds pattern.
func vertexAPICreds(a *switchailocalauth.Auth) (apiKey, baseURL string) {
	if a == nil {
		return "", ""
	}
	if a.Attributes != nil {
		apiKey = a.Attributes["api_key"]
		baseURL = a.Attributes["base_url"]
	}
	if apiKey == "" && a.Metadata != nil {
		if v, ok := a.Metadata["access_token"].(string); ok {
			apiKey = v
		}
	}
	return
}

func vertexBaseURL(location string) string {
	loc := strings.TrimSpace(location)
	if loc == "" {
		loc = "us-central1"
	}
	return fmt.Sprintf("https://%s-aiplatform.googleapis.com", loc)
}

func vertexAccessToken(ctx context.Context, cfg *config.Config, auth *switchailocalauth.Auth, saJSON []byte) (string, error) {
	if httpClient := newProxyAwareHTTPClient(ctx, cfg, auth, 0); httpClient != nil {
		ctx = context.WithValue(ctx, oauth2.HTTPClient, httpClient)
	}
	// Use cloud-platform scope for Vertex AI.
	creds, errCreds := google.CredentialsFromJSON(ctx, saJSON, "https://www.googleapis.com/auth/cloud-platform")
	if errCreds != nil {
		return "", fmt.Errorf("vertex executor: parse service account json failed: %w", errCreds)
	}
	tok, errTok := creds.TokenSource.Token()
	if errTok != nil {
		return "", fmt.Errorf("vertex executor: get access token failed: %w", errTok)
	}
	return tok.AccessToken, nil
}
