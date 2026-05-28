// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package openai provides HTTP handlers for OpenAI API endpoints.
// This package implements the OpenAI-compatible API interface, including model listing
// and chat completion functionality. It supports both streaming and non-streaming responses,
// and manages a pool of clients to interact with backend services.
// The handlers translate OpenAI API requests to the appropriate backend format and
// convert responses back to OpenAI-compatible format.
package openai

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"

	json "github.com/goccy/go-json"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	. "github.com/traylinx/switchAILocal/internal/constant"
	"github.com/traylinx/switchAILocal/internal/interfaces"
	"github.com/traylinx/switchAILocal/internal/registry"
	responsesconverter "github.com/traylinx/switchAILocal/internal/translator/openai/openai/responses"
	"github.com/traylinx/switchAILocal/internal/virtualmodels"
	"github.com/traylinx/switchAILocal/sdk/api/handlers"
)

var (
	headerData = []byte("data: ")
	headerDone = []byte("data: [DONE]\n\n")
	newline    = []byte("\n\n")
)

// OpenAIAPIHandler contains the handlers for OpenAI API endpoints.
// It holds a pool of clients to interact with the backend service.
type OpenAIAPIHandler struct {
	*handlers.BaseAPIHandler
}

// NewOpenAIAPIHandler creates a new OpenAI API handlers instance.
// It takes an BaseAPIHandler instance as input and returns an OpenAIAPIHandler.
//
// Parameters:
//   - apiHandlers: The base API handlers instance
//
// Returns:
//   - *OpenAIAPIHandler: A new OpenAI API handlers instance
func NewOpenAIAPIHandler(apiHandlers *handlers.BaseAPIHandler) *OpenAIAPIHandler {
	return &OpenAIAPIHandler{
		BaseAPIHandler: apiHandlers,
	}
}

// HandlerType returns the identifier for this handler implementation.
func (h *OpenAIAPIHandler) HandlerType() string {
	return OpenAI
}

// Models returns the OpenAI-compatible model metadata supported by this handler.
func (h *OpenAIAPIHandler) Models() []map[string]any {
	// Get dynamic models from the global registry
	modelRegistry := registry.GetGlobalRegistry()
	return modelRegistry.GetAvailableModels("openai")
}

// OpenAIModels handles the /v1/models endpoint.
// It returns a list of available AI models with their capabilities
// and specifications in OpenAI-compatible format.
func (h *OpenAIAPIHandler) OpenAIModels(c *gin.Context) {
	// Get all available models
	virtualCatalog := virtualmodels.PublicCatalog(h.Cfg)
	allModels := append(virtualCatalog, h.Models()...)
	ailCatalogMode := shouldUseAILOnlyCatalog(allModels)

	// Build modality lookup from intelligence.matrix config
	// Maps model ID → set of modality strings
	modalityMap := buildModalityMap(h.Cfg.Intelligence.Matrix)

	// Optional: filter by ?modality= query param (text, image, audio, embedding, vision)
	modalityFilter := strings.ToLower(c.Query("modality"))

	// Filter to only include the 4 required fields + capabilities + the new
	// AI-SDK-discovery fields (attachment / tool_call / reasoning / modalities
	// / vision / audio) emitted by the registry. These let Vercel-AI-SDK
	// based clients (OpenCode / Cursor / Continue.dev) auto-detect whether
	// to forward image and audio content blocks; without them the client
	// strips media client-side and the model never sees it.
	preserveIfPresent := []string{"attachment", "tool_call", "reasoning", "modalities", "vision", "audio", "native_tools"}
	var filteredModels []map[string]any
	seenModels := make(map[string]struct{}, len(allModels))
	for _, model := range allModels {
		modelID, _ := model["id"].(string)
		modelKey := strings.ToLower(strings.TrimSpace(modelID))
		if modelKey == "" {
			continue
		}
		if visibility, _ := model["visibility"].(string); strings.EqualFold(strings.TrimSpace(visibility), "private") {
			continue
		}
		if ailCatalogMode && !strings.HasPrefix(modelKey, "ail-") {
			continue
		}
		if _, exists := seenModels[modelKey]; exists {
			continue
		}
		seenModels[modelKey] = struct{}{}

		filteredModel := map[string]any{
			"id":     model["id"],
			"object": model["object"],
		}

		// Add created field if it exists
		if created, exists := model["created"]; exists {
			filteredModel["created"] = created
		}

		// Add owned_by field if it exists
		if ownedBy, exists := model["owned_by"]; exists {
			filteredModel["owned_by"] = ownedBy
		}

		// Enrich with capabilities from either explicit virtual-pool truth
		// or legacy intelligence.matrix. For virtual models, pool members are
		// authoritative; do not let stale matrix entries advertise media.
		capabilities := modalityMap[modelID]
		isVirtual := virtualmodels.IsVirtualModel(h.Cfg, modelID)
		if isVirtual {
			capabilities = capabilitiesFromModel(model)
		}
		if len(capabilities) == 0 {
			// Default: assume text capability for unlabeled non-virtual models.
			capabilities = []string{"text"}
		}
		filteredModel["capabilities"] = capabilities

		// Preserve registry-emitted capability discovery fields. These are
		// what modern clients key off, complementing the legacy field.
		for _, k := range preserveIfPresent {
			if v, ok := model[k]; ok {
				filteredModel[k] = v
			}
		}

		// Bridge: when the matrix-driven capabilities array declares
		// vision/image/audio for an ID our inference table doesn't
		// recognise (e.g. matrix-aliased "ail-compound"), upgrade the new
		// structured fields too. Otherwise AI-SDK clients see vision in
		// the legacy array but attachment:false in the new shape and end
		// up stripping images. Operator's declared truth wins.
		if !isVirtual {
			upgradeCapabilityFields(filteredModel, capabilities)
		}

		// Apply modality filter if specified
		if modalityFilter != "" {
			if !containsModality(capabilities, modalityFilter) {
				continue
			}
		}

		filteredModels = append(filteredModels, filteredModel)
	}

	c.JSON(http.StatusOK, gin.H{
		"object": "list",
		"data":   filteredModels,
	})
}

func shouldUseAILOnlyCatalog(models []map[string]any) bool {
	for _, model := range models {
		modelID, _ := model["id"].(string)
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(modelID)), "ail-") {
			return true
		}
	}
	return false
}

// buildModalityMap creates a reverse lookup: model ID → list of modality strings
// from the intelligence.matrix config. Multiple matrix keys can map to the same model.
func buildModalityMap(matrix map[string]string) map[string][]string {
	// Map matrix keys to modality categories
	keyToModality := map[string]string{
		"image_gen":     "image",
		"vision":        "vision",
		"transcription": "audio",
		"speech":        "audio",
		"audio":         "audio",
		"embedding":     "embedding",
		"coding":        "text",
		"reasoning":     "text",
		"creative":      "text",
		"fast":          "text",
		"secure":        "text",
		"long_ctx":      "text",
		"cli":           "text",
	}

	result := make(map[string][]string)
	for matrixKey, modelID := range matrix {
		if modelID == "" {
			continue
		}
		modality, ok := keyToModality[matrixKey]
		if !ok {
			modality = "text" // default
		}
		// Avoid duplicates
		if !containsModality(result[modelID], modality) {
			result[modelID] = append(result[modelID], modality)
		}
	}
	return result
}

// upgradeCapabilityFields merges the matrix-derived legacy capability
// strings into the structured capability fields the registry emitted.
// The matrix is operator-authored truth — when it declares "vision" or
// "audio" for a model, AI-SDK clients should see attachment:true and
// the right input modalities even if our inference table doesn't
// recognise the model ID (common for matrix-aliased names like
// "ail-compound" / "ail-image" / per-tenant aliases).
//
// This is additive: only flips fields ON, never OFF. So an inferred
// vision-capable model whose matrix entry only says "text" still keeps
// vision (the inference is more specific than the matrix bucket).
func upgradeCapabilityFields(model map[string]any, matrixCaps []string) {
	hasVision := false
	hasAudio := false
	hasImageOut := false
	for _, c := range matrixCaps {
		switch c {
		case "vision", "image":
			// Matrix uses "image" for both image-IN (vision) and image-OUT
			// (image generation). Disambiguate: presence of an explicit
			// "vision" key means input. "image" without "vision" is
			// generally output (e.g. ail-image), but to stay safe we treat
			// "image" alone as input-image — image-output models rarely
			// also need attachment=true.
			if c == "vision" {
				hasVision = true
			}
		case "audio":
			hasAudio = true
		}
	}
	// Detect image-OUT separately so we can surface output:["image"].
	for _, c := range matrixCaps {
		if c == "image" {
			// Only treat as output if id name suggests gen (ail-image,
			// image-01, etc.); otherwise treat as vision input.
			if id, _ := model["id"].(string); strings.Contains(strings.ToLower(id), "image") {
				hasImageOut = true
			} else {
				hasVision = true
			}
		}
	}

	if hasVision {
		model["vision"] = true
		model["attachment"] = true
		// Upgrade modalities.input to include "image" if not already there.
		mods, _ := model["modalities"].(map[string]any)
		if mods == nil {
			// modalities may be the typed struct from the registry; convert.
			if typed, ok := model["modalities"].(registry.ModelModalities); ok {
				mods = map[string]any{
					"input":  appendUnique(stringsFromAny(typed.Input), "image"),
					"output": stringsFromAny(typed.Output),
				}
			} else {
				mods = map[string]any{"input": []string{"text", "image"}, "output": []string{"text"}}
			}
		} else {
			mods["input"] = appendUnique(stringsFromAny(mods["input"]), "image")
		}
		model["modalities"] = mods
	}
	if hasAudio {
		model["audio"] = true
		model["attachment"] = true
		mods, _ := model["modalities"].(map[string]any)
		if mods == nil {
			if typed, ok := model["modalities"].(registry.ModelModalities); ok {
				mods = map[string]any{
					"input":  appendUnique(stringsFromAny(typed.Input), "audio"),
					"output": stringsFromAny(typed.Output),
				}
			} else {
				mods = map[string]any{"input": []string{"text", "audio"}, "output": []string{"text"}}
			}
		} else {
			mods["input"] = appendUnique(stringsFromAny(mods["input"]), "audio")
		}
		model["modalities"] = mods
	}
	if hasImageOut {
		mods, _ := model["modalities"].(map[string]any)
		if mods == nil {
			mods = map[string]any{"input": []string{"text"}, "output": []string{"image"}}
		} else {
			mods["output"] = appendUnique(stringsFromAny(mods["output"]), "image")
		}
		model["modalities"] = mods
	}
}

func capabilitiesFromModel(model map[string]any) []string {
	caps := []string{}
	if mods, ok := model["modalities"].(registry.ModelModalities); ok {
		for _, input := range mods.Input {
			caps = appendUnique(caps, input)
			if input == "image" {
				caps = appendUnique(caps, "vision")
			}
			if input == "audio" {
				caps = appendUnique(caps, "audio")
			}
		}
		for _, output := range mods.Output {
			caps = appendUnique(caps, output)
		}
		return caps
	}
	if mods, ok := model["modalities"].(map[string]any); ok {
		for _, input := range stringsFromAny(mods["input"]) {
			caps = appendUnique(caps, input)
			if input == "image" {
				caps = appendUnique(caps, "vision")
			}
			if input == "audio" {
				caps = appendUnique(caps, "audio")
			}
		}
		for _, output := range stringsFromAny(mods["output"]) {
			caps = appendUnique(caps, output)
		}
	}
	return caps
}

// stringsFromAny normalises an interface that might be []string or []any
// (the latter happens after JSON round-trip) into a flat []string.
func stringsFromAny(v any) []string {
	switch s := v.(type) {
	case []string:
		return s
	case []any:
		out := make([]string, 0, len(s))
		for _, item := range s {
			if str, ok := item.(string); ok {
				out = append(out, str)
			}
		}
		return out
	}
	return nil
}

// appendUnique appends s to slice only if not already present. Cheap
// since modality lists are tiny (1-5 entries).
func appendUnique(slice []string, s string) []string {
	for _, x := range slice {
		if x == s {
			return slice
		}
	}
	return append(slice, s)
}

// containsModality checks if a modality string exists in a slice.
func containsModality(modalities []string, target string) bool {
	for _, m := range modalities {
		if m == target {
			return true
		}
	}
	return false
}

// ChatCompletions handles the /v1/chat/completions endpoint.
// It determines whether the request is for a streaming or non-streaming response
// and calls the appropriate handler based on the model provider.
//
// Parameters:
//   - c: The Gin context containing the HTTP request and response
func (h *OpenAIAPIHandler) ChatCompletions(c *gin.Context) {
	// Emit build identity on every chat-completions response so the
	// multi-instance LB coverage probe can verify each of the 5 switchailocal
	// instances is running the expected sha.
	c.Header("X-Ail-Build", buildIdentity())

	rawJSON, err := c.GetRawData()
	// If data retrieval fails, return a 400 Bad Request error.
	if err != nil {
		c.JSON(http.StatusBadRequest, handlers.ErrorResponse{
			Error: handlers.ErrorDetail{
				Message: fmt.Sprintf("Invalid request: %v", err),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Check if the client requested a streaming response.
	streamResult := gjson.GetBytes(rawJSON, "stream")
	stream := streamResult.Type == gjson.True

	// Some clients send OpenAI Responses-format payloads to /v1/chat/completions.
	// Convert them to Chat Completions so downstream translators preserve tool metadata.
	if shouldTreatAsResponsesFormat(rawJSON) {
		modelName := gjson.GetBytes(rawJSON, "model").String()
		rawJSON = responsesconverter.ConvertOpenAIResponsesRequestToOpenAIChatCompletions(modelName, rawJSON, stream)
		stream = gjson.GetBytes(rawJSON, "stream").Bool()
	}

	modelName := gjson.GetBytes(rawJSON, "model").String()
	debugDumpRequest(modelName, rawJSON)

	optOut := strings.EqualFold(strings.TrimSpace(c.GetHeader("X-Ail-Autoinject")), "off")
	rawJSON = autoInjectWebSearch(rawJSON, modelName, optOut)

	if stream {
		h.handleStreamingResponse(c, rawJSON)
	} else {
		h.handleNonStreamingResponse(c, rawJSON)
	}

}

// shouldTreatAsResponsesFormat detects OpenAI Responses-style payloads that are
// accidentally sent to the Chat Completions endpoint.
func shouldTreatAsResponsesFormat(rawJSON []byte) bool {
	if gjson.GetBytes(rawJSON, "messages").Exists() {
		return false
	}
	if gjson.GetBytes(rawJSON, "input").Exists() {
		return true
	}
	if gjson.GetBytes(rawJSON, "instructions").Exists() {
		return true
	}
	return false
}

// Completions handles the /v1/completions endpoint.
// It determines whether the request is for a streaming or non-streaming response
// and calls the appropriate handler based on the model provider.
// This endpoint follows the OpenAI completions API specification.
//
// Parameters:
//   - c: The Gin context containing the HTTP request and response
func (h *OpenAIAPIHandler) Completions(c *gin.Context) {
	rawJSON, err := c.GetRawData()
	// If data retrieval fails, return a 400 Bad Request error.
	if err != nil {
		c.JSON(http.StatusBadRequest, handlers.ErrorResponse{
			Error: handlers.ErrorDetail{
				Message: fmt.Sprintf("Invalid request: %v", err),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Check if the client requested a streaming response.
	streamResult := gjson.GetBytes(rawJSON, "stream")
	if streamResult.Type == gjson.True {
		h.handleCompletionsStreamingResponse(c, rawJSON)
	} else {
		h.handleCompletionsNonStreamingResponse(c, rawJSON)
	}

}

// Embeddings handles the /v1/embeddings endpoint.
// It parses the request to find the target model and sends it to the provider.
// This endpoint follows the standard OpenAI embeddings API specification.
func (h *OpenAIAPIHandler) Embeddings(c *gin.Context) {
	rawJSON, err := c.GetRawData()
	if err != nil {
		h.WriteErrorResponse(c, &interfaces.ErrorMessage{
			StatusCode: http.StatusBadRequest,
			Error:      fmt.Errorf("invalid request: %v", err),
		})
		return
	}

	modelName := gjson.GetBytes(rawJSON, "model").String()
	if modelName == "" {
		if val, ok := h.Cfg.Intelligence.Matrix["embedding"]; ok && val != "" {
			modelName = val
		} else {
			h.WriteErrorResponse(c, &interfaces.ErrorMessage{
				StatusCode: http.StatusBadRequest,
				Error:      fmt.Errorf("model not specified and no 'embedding' configured in intelligence.matrix"),
			})
			return
		}
	}

	cliCtx, cliCancel := h.GetContextWithCancel(h, c, context.Background())
	resp, errMsg := h.ExecuteMultimodalWithAuthManager(cliCtx, h.HandlerType(), modelName, rawJSON, "", "embeddings", "application/json")
	if errMsg != nil {
		h.WriteErrorResponse(c, errMsg)
		cliCancel(errMsg.Error)
		return
	}

	c.Data(http.StatusOK, "application/json", resp)
	cliCancel()
}

// convertCompletionsRequestToChatCompletions converts OpenAI completions API request to chat completions format.
// This allows the completions endpoint to use the existing chat completions infrastructure.
//
// Parameters:
//   - rawJSON: The raw JSON bytes of the completions request
//
// Returns:
//   - []byte: The converted chat completions request
func convertCompletionsRequestToChatCompletions(rawJSON []byte) []byte {
	root := gjson.ParseBytes(rawJSON)

	// Extract prompt from completions request
	prompt := root.Get("prompt").String()
	if prompt == "" {
		prompt = "Complete this:"
	}

	// Create chat completions structure
	out := `{"model":"","messages":[{"role":"user","content":""}]}`

	// Set model
	if model := root.Get("model"); model.Exists() {
		out, _ = sjson.Set(out, "model", model.String())
	}

	// Set the prompt as user message content
	out, _ = sjson.Set(out, "messages.0.content", prompt)

	// Copy other parameters from completions to chat completions
	if maxTokens := root.Get("max_tokens"); maxTokens.Exists() {
		out, _ = sjson.Set(out, "max_tokens", maxTokens.Int())
	}

	if temperature := root.Get("temperature"); temperature.Exists() {
		out, _ = sjson.Set(out, "temperature", temperature.Float())
	}

	if topP := root.Get("top_p"); topP.Exists() {
		out, _ = sjson.Set(out, "top_p", topP.Float())
	}

	if frequencyPenalty := root.Get("frequency_penalty"); frequencyPenalty.Exists() {
		out, _ = sjson.Set(out, "frequency_penalty", frequencyPenalty.Float())
	}

	if presencePenalty := root.Get("presence_penalty"); presencePenalty.Exists() {
		out, _ = sjson.Set(out, "presence_penalty", presencePenalty.Float())
	}

	if stop := root.Get("stop"); stop.Exists() {
		out, _ = sjson.SetRaw(out, "stop", stop.Raw)
	}

	if stream := root.Get("stream"); stream.Exists() {
		out, _ = sjson.Set(out, "stream", stream.Bool())
	}

	if logprobs := root.Get("logprobs"); logprobs.Exists() {
		out, _ = sjson.Set(out, "logprobs", logprobs.Bool())
	}

	if topLogprobs := root.Get("top_logprobs"); topLogprobs.Exists() {
		out, _ = sjson.Set(out, "top_logprobs", topLogprobs.Int())
	}

	if echo := root.Get("echo"); echo.Exists() {
		out, _ = sjson.Set(out, "echo", echo.Bool())
	}

	return []byte(out)
}

// convertChatCompletionsResponseToCompletions converts chat completions API response back to completions format.
// This ensures the completions endpoint returns data in the expected format.
//
// Parameters:
//   - rawJSON: The raw JSON bytes of the chat completions response
//
// Returns:
//   - []byte: The converted completions response
func convertChatCompletionsResponseToCompletions(rawJSON []byte) []byte {
	root := gjson.ParseBytes(rawJSON)

	// Base completions response structure
	out := `{"id":"","object":"text_completion","created":0,"model":"","choices":[]}`

	// Copy basic fields
	if id := root.Get("id"); id.Exists() {
		out, _ = sjson.Set(out, "id", id.String())
	}

	if created := root.Get("created"); created.Exists() {
		out, _ = sjson.Set(out, "created", created.Int())
	}

	if model := root.Get("model"); model.Exists() {
		out, _ = sjson.Set(out, "model", model.String())
	}

	if usage := root.Get("usage"); usage.Exists() {
		out, _ = sjson.SetRaw(out, "usage", usage.Raw)
	}

	// Convert choices from chat completions to completions format
	var choices []interface{}
	if chatChoices := root.Get("choices"); chatChoices.Exists() && chatChoices.IsArray() {
		chatChoices.ForEach(func(_, choice gjson.Result) bool {
			completionsChoice := map[string]interface{}{
				"index": choice.Get("index").Int(),
			}

			// Extract text content from message.content
			if message := choice.Get("message"); message.Exists() {
				if content := message.Get("content"); content.Exists() {
					completionsChoice["text"] = content.String()
				}
			} else if delta := choice.Get("delta"); delta.Exists() {
				// For streaming responses, use delta.content
				if content := delta.Get("content"); content.Exists() {
					completionsChoice["text"] = content.String()
				}
			}

			// Copy finish_reason
			if finishReason := choice.Get("finish_reason"); finishReason.Exists() {
				completionsChoice["finish_reason"] = finishReason.String()
			}

			// Copy logprobs if present
			if logprobs := choice.Get("logprobs"); logprobs.Exists() {
				completionsChoice["logprobs"] = logprobs.Value()
			}

			choices = append(choices, completionsChoice)
			return true
		})
	}

	if len(choices) > 0 {
		choicesJSON, _ := json.Marshal(choices)
		out, _ = sjson.SetRaw(out, "choices", string(choicesJSON))
	}

	return []byte(out)
}

// convertChatCompletionsStreamChunkToCompletions converts a streaming chat completions chunk to completions format.
// This handles the real-time conversion of streaming response chunks and filters out empty text responses.
//
// Parameters:
//   - chunkData: The raw JSON bytes of a single chat completions stream chunk
//
// Returns:
//   - []byte: The converted completions stream chunk, or nil if should be filtered out
func convertChatCompletionsStreamChunkToCompletions(chunkData []byte) []byte {
	root := gjson.ParseBytes(chunkData)

	// Check if this chunk has any meaningful content
	hasContent := false
	if chatChoices := root.Get("choices"); chatChoices.Exists() && chatChoices.IsArray() {
		chatChoices.ForEach(func(_, choice gjson.Result) bool {
			// Check if delta has content or finish_reason
			if delta := choice.Get("delta"); delta.Exists() {
				if content := delta.Get("content"); content.Exists() && content.String() != "" {
					hasContent = true
					return false // Break out of forEach
				}
			}
			// Also check for finish_reason to ensure we don't skip final chunks
			if finishReason := choice.Get("finish_reason"); finishReason.Exists() && finishReason.String() != "" && finishReason.String() != "null" {
				hasContent = true
				return false // Break out of forEach
			}
			return true
		})
	}

	// If no meaningful content, return nil to indicate this chunk should be skipped
	if !hasContent {
		return nil
	}

	// Base completions stream response structure
	out := `{"id":"","object":"text_completion","created":0,"model":"","choices":[]}`

	// Copy basic fields
	if id := root.Get("id"); id.Exists() {
		out, _ = sjson.Set(out, "id", id.String())
	}

	if created := root.Get("created"); created.Exists() {
		out, _ = sjson.Set(out, "created", created.Int())
	}

	if model := root.Get("model"); model.Exists() {
		out, _ = sjson.Set(out, "model", model.String())
	}

	// Convert choices from chat completions delta to completions format
	var choices []interface{}
	if chatChoices := root.Get("choices"); chatChoices.Exists() && chatChoices.IsArray() {
		chatChoices.ForEach(func(_, choice gjson.Result) bool {
			completionsChoice := map[string]interface{}{
				"index": choice.Get("index").Int(),
			}

			// Extract text content from delta.content
			if delta := choice.Get("delta"); delta.Exists() {
				if content := delta.Get("content"); content.Exists() && content.String() != "" {
					completionsChoice["text"] = content.String()
				} else {
					completionsChoice["text"] = ""
				}
			} else {
				completionsChoice["text"] = ""
			}

			// Copy finish_reason
			if finishReason := choice.Get("finish_reason"); finishReason.Exists() && finishReason.String() != "null" {
				completionsChoice["finish_reason"] = finishReason.String()
			}

			// Copy logprobs if present
			if logprobs := choice.Get("logprobs"); logprobs.Exists() {
				completionsChoice["logprobs"] = logprobs.Value()
			}

			choices = append(choices, completionsChoice)
			return true
		})
	}

	if len(choices) > 0 {
		choicesJSON, _ := json.Marshal(choices)
		out, _ = sjson.SetRaw(out, "choices", string(choicesJSON))
	}

	return []byte(out)
}

// handleNonStreamingResponse handles non-streaming chat completion responses
// for Gemini models. It selects a client from the pool, sends the request, and
// aggregates the response before sending it back to the client in OpenAI format.
//
// Parameters:
//   - c: The Gin context containing the HTTP request and response
//   - rawJSON: The raw JSON bytes of the OpenAI-compatible request
func (h *OpenAIAPIHandler) handleNonStreamingResponse(c *gin.Context, rawJSON []byte) {
	c.Header("Content-Type", "application/json")

	// Set skill metadata header if available
	if metadata, exists := c.Get("request_metadata"); exists {
		if metaMap, ok := metadata.(map[string]any); ok {
			if skillName, ok := metaMap["matched_skill"].(string); ok && skillName != "" {
				c.Header("X-Matched-Skill", skillName)
			}
		}
	}

	modelName := gjson.GetBytes(rawJSON, "model").String()
	cliCtx, cliCancel := h.GetContextWithCancel(h, c, context.Background())
	resp, errMsg := h.ExecuteWithAuthManager(cliCtx, h.HandlerType(), modelName, rawJSON, h.GetAlt(c))
	if errMsg != nil {
		h.WriteErrorResponse(c, errMsg)
		cliCancel(errMsg.Error)
		return
	}
	_, _ = c.Writer.Write(resp)
	cliCancel()
}

// handleStreamingResponse handles streaming responses for Gemini models.
// It establishes a streaming connection with the backend service and forwards
// the response chunks to the client in real-time using Server-Sent Events.
//
// Parameters:
//   - c: The Gin context containing the HTTP request and response
//   - rawJSON: The raw JSON bytes of the OpenAI-compatible request
func (h *OpenAIAPIHandler) handleStreamingResponse(c *gin.Context, rawJSON []byte) {
	// Get the http.Flusher interface to manually flush the response.
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, handlers.ErrorResponse{
			Error: handlers.ErrorDetail{
				Message: "Streaming not supported",
				Type:    "server_error",
			},
		})
		return
	}

	modelName := gjson.GetBytes(rawJSON, "model").String()
	cliCtx, cliCancel := h.GetContextWithCancel(h, c, context.Background())
	dataChan, errChan := h.ExecuteStreamWithAuthManager(cliCtx, h.HandlerType(), modelName, rawJSON, h.GetAlt(c))

	setSSEHeaders := func() {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("X-Accel-Buffering", "no")

		// Set skill metadata header if available
		if metadata, exists := c.Get("request_metadata"); exists {
			if metaMap, ok := metadata.(map[string]any); ok {
				if skillName, ok := metaMap["matched_skill"].(string); ok && skillName != "" {
					c.Header("X-Matched-Skill", skillName)
				}
			}
		}
	}

	// Peek at the first chunk to determine success or failure before setting headers
	headersSet := false
	for {
		select {
		case <-c.Request.Context().Done():
			cliCancel(c.Request.Context().Err())
			return
		case errMsg, ok := <-errChan:
			if !ok {
				// Err channel closed cleanly; wait for data channel.
				errChan = nil
				continue
			}
			// Upstream failed immediately. Return proper error status and JSON.
			h.WriteErrorResponse(c, errMsg)
			if errMsg != nil {
				cliCancel(errMsg.Error)
			} else {
				cliCancel(nil)
			}
			return
		case chunk, ok := <-dataChan:
			if !ok {
				// Stream closed without data? Send DONE or just headers.
				if !headersSet {
					setSSEHeaders()
				}
				_, _ = c.Writer.Write(headerDone)
				flusher.Flush()
				cliCancel(nil)
				return
			}

			// Success! Commit to streaming headers.
			if !headersSet {
				setSSEHeaders()
			}

			_, _ = c.Writer.Write(headerData)
			_, _ = c.Writer.Write(chunk)
			_, _ = c.Writer.Write(newline)
			flusher.Flush()

			// Continue streaming the rest
			h.handleStreamResult(c, flusher, func(err error) { cliCancel(err) }, dataChan, errChan)
			return
		}
	}
}

// handleCompletionsNonStreamingResponse handles non-streaming completions responses.
// It converts completions request to chat completions format, sends to backend,
// then converts the response back to completions format before sending to client.
//
// Parameters:
//   - c: The Gin context containing the HTTP request and response
//   - rawJSON: The raw JSON bytes of the OpenAI-compatible completions request
func (h *OpenAIAPIHandler) handleCompletionsNonStreamingResponse(c *gin.Context, rawJSON []byte) {
	c.Header("Content-Type", "application/json")

	// Convert completions request to chat completions format
	chatCompletionsJSON := convertCompletionsRequestToChatCompletions(rawJSON)

	modelName := gjson.GetBytes(chatCompletionsJSON, "model").String()
	cliCtx, cliCancel := h.GetContextWithCancel(h, c, context.Background())
	resp, errMsg := h.ExecuteWithAuthManager(cliCtx, h.HandlerType(), modelName, chatCompletionsJSON, "")
	if errMsg != nil {
		h.WriteErrorResponse(c, errMsg)
		cliCancel(errMsg.Error)
		return
	}
	completionsResp := convertChatCompletionsResponseToCompletions(resp)
	_, _ = c.Writer.Write(completionsResp)
	cliCancel()
}

// handleCompletionsStreamingResponse handles streaming completions responses.
// It converts completions request to chat completions format, streams from backend,
// then converts each response chunk back to completions format before sending to client.
//
// Parameters:
//   - c: The Gin context containing the HTTP request and response
//   - rawJSON: The raw JSON bytes of the OpenAI-compatible completions request
func (h *OpenAIAPIHandler) handleCompletionsStreamingResponse(c *gin.Context, rawJSON []byte) {
	// Get the http.Flusher interface to manually flush the response.
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, handlers.ErrorResponse{
			Error: handlers.ErrorDetail{
				Message: "Streaming not supported",
				Type:    "server_error",
			},
		})
		return
	}

	// Convert completions request to chat completions format
	chatCompletionsJSON := convertCompletionsRequestToChatCompletions(rawJSON)

	modelName := gjson.GetBytes(chatCompletionsJSON, "model").String()
	cliCtx, cliCancel := h.GetContextWithCancel(h, c, context.Background())
	dataChan, errChan := h.ExecuteStreamWithAuthManager(cliCtx, h.HandlerType(), modelName, chatCompletionsJSON, "")

	setSSEHeaders := func() {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("X-Accel-Buffering", "no")
	}

	// Peek at the first chunk
	headersSet := false
	for {
		select {
		case <-c.Request.Context().Done():
			cliCancel(c.Request.Context().Err())
			return
		case errMsg, ok := <-errChan:
			if !ok {
				// Err channel closed cleanly; wait for data channel.
				errChan = nil
				continue
			}
			h.WriteErrorResponse(c, errMsg)
			if errMsg != nil {
				cliCancel(errMsg.Error)
			} else {
				cliCancel(nil)
			}
			return
		case chunk, ok := <-dataChan:
			if !ok {
				if !headersSet {
					setSSEHeaders()
				}
				_, _ = c.Writer.Write(headerDone)
				flusher.Flush()
				cliCancel(nil)
				return
			}

			// Success! Set headers.
			if !headersSet {
				setSSEHeaders()
			}

			// Write the first chunk
			converted := convertChatCompletionsStreamChunkToCompletions(chunk)
			if converted != nil {
				_, _ = c.Writer.Write(headerData)
				_, _ = c.Writer.Write(converted)
				_, _ = c.Writer.Write(newline)
				flusher.Flush()
			}

			done := make(chan struct{})
			var doneOnce sync.Once
			stop := func() { doneOnce.Do(func() { close(done) }) }

			convertedChan := make(chan []byte)
			go func() {
				defer close(convertedChan)
				for {
					select {
					case <-done:
						return
					case chunk, ok := <-dataChan:
						if !ok {
							return
						}
						converted := convertChatCompletionsStreamChunkToCompletions(chunk)
						if converted == nil {
							continue
						}
						select {
						case <-done:
							return
						case convertedChan <- converted:
						}
					}
				}
			}()

			h.handleStreamResult(c, flusher, func(err error) {
				stop()
				cliCancel(err)
			}, convertedChan, errChan)
			return
		}
	}
}
func (h *OpenAIAPIHandler) handleStreamResult(c *gin.Context, flusher http.Flusher, cancel func(error), data <-chan []byte, errs <-chan *interfaces.ErrorMessage) {
	h.ForwardStream(c, flusher, cancel, data, errs, handlers.StreamForwardOptions{
		WriteChunk: func(chunk []byte) {
			_, _ = c.Writer.Write(headerData)
			_, _ = c.Writer.Write(chunk)
			_, _ = c.Writer.Write(newline)
		},
		WriteTerminalError: func(errMsg *interfaces.ErrorMessage) {
			if errMsg == nil {
				return
			}
			status := http.StatusInternalServerError
			if errMsg.StatusCode > 0 {
				status = errMsg.StatusCode
			}
			errText := http.StatusText(status)
			if errMsg.Error != nil && errMsg.Error.Error() != "" {
				errText = errMsg.Error.Error()
			}
			body := handlers.BuildErrorResponseBody(status, errText)
			_, _ = c.Writer.Write(headerData)
			_, _ = c.Writer.Write(body)
			_, _ = c.Writer.Write(newline)
		},
		WriteDone: func() {
			_, _ = c.Writer.Write(headerDone)
		},
	})
}

// ImagesGenerations handles /v1/images/generations
func (h *OpenAIAPIHandler) ImagesGenerations(c *gin.Context) {
	contentType := c.GetHeader("Content-Type")
	rawJSON, err := c.GetRawData()
	if err != nil {
		h.WriteErrorResponse(c, &interfaces.ErrorMessage{StatusCode: http.StatusBadRequest, Error: fmt.Errorf("invalid request: %v", err)})
		return
	}

	var modelName string
	if strings.HasPrefix(contentType, "multipart/form-data") {
		modelName = extractModelFromMultipart(rawJSON, contentType)
	} else {
		modelName = gjson.GetBytes(rawJSON, "model").String()
	}
	if modelName == "" {
		if val, ok := h.Cfg.Intelligence.Matrix["image_gen"]; ok && val != "" {
			modelName = val
		} else {
			h.WriteErrorResponse(c, &interfaces.ErrorMessage{StatusCode: http.StatusBadRequest, Error: fmt.Errorf("model not specified and no 'image_gen' configured in intelligence.matrix")})
			return
		}
	}

	if contentType == "" {
		contentType = "application/json"
	}

	cliCtx, cliCancel := h.GetContextWithCancel(h, c, context.Background())
	resp, errMsg := h.ExecuteMultimodalWithAuthManager(cliCtx, h.HandlerType(), modelName, rawJSON, "", "images_generations", contentType)
	if errMsg != nil {
		h.WriteErrorResponse(c, errMsg)
		cliCancel(errMsg.Error)
		return
	}
	c.Data(http.StatusOK, "application/json", resp)
	cliCancel()
}

// AudioSpeech handles /v1/audio/speech
func (h *OpenAIAPIHandler) AudioSpeech(c *gin.Context) {
	rawJSON, err := c.GetRawData()
	if err != nil {
		h.WriteErrorResponse(c, &interfaces.ErrorMessage{StatusCode: http.StatusBadRequest, Error: fmt.Errorf("invalid request: %v", err)})
		return
	}

	modelName := gjson.GetBytes(rawJSON, "model").String()
	if modelName == "" {
		if val, ok := h.Cfg.Intelligence.Matrix["speech"]; ok && val != "" {
			modelName = val
		} else {
			h.WriteErrorResponse(c, &interfaces.ErrorMessage{StatusCode: http.StatusBadRequest, Error: fmt.Errorf("model not specified and no 'speech' configured in intelligence.matrix")})
			return
		}
	}

	cliCtx, cliCancel := h.GetContextWithCancel(h, c, context.Background())
	resp, errMsg := h.ExecuteMultimodalWithAuthManager(cliCtx, h.HandlerType(), modelName, rawJSON, "", "audio_speech", "application/json")
	if errMsg != nil {
		h.WriteErrorResponse(c, errMsg)
		cliCancel(errMsg.Error)
		return
	}
	// Determine response Content-Type from request's response_format field
	respContentType := "audio/mpeg" // default: mp3
	switch gjson.GetBytes(rawJSON, "response_format").String() {
	case "opus":
		respContentType = "audio/opus"
	case "aac":
		respContentType = "audio/aac"
	case "flac":
		respContentType = "audio/flac"
	case "wav":
		respContentType = "audio/wav"
	case "pcm":
		respContentType = "audio/pcm"
	}
	c.Data(http.StatusOK, respContentType, resp)
	cliCancel()
}

// ImagesEdits handles /v1/images/edits
// Supports both multipart/form-data (binary image upload) and application/json (image URLs).
func (h *OpenAIAPIHandler) ImagesEdits(c *gin.Context) {
	contentType := c.GetHeader("Content-Type")
	rawBody, err := c.GetRawData()
	if err != nil {
		h.WriteErrorResponse(c, &interfaces.ErrorMessage{StatusCode: http.StatusBadRequest, Error: fmt.Errorf("invalid request: %v", err)})
		return
	}

	var modelName string
	if strings.HasPrefix(contentType, "multipart/form-data") {
		modelName = extractModelFromMultipart(rawBody, contentType)
	} else {
		modelName = gjson.GetBytes(rawBody, "model").String()
	}

	if modelName == "" {
		if val, ok := h.Cfg.Intelligence.Matrix["image_gen"]; ok && val != "" {
			modelName = val
		} else {
			h.WriteErrorResponse(c, &interfaces.ErrorMessage{StatusCode: http.StatusBadRequest, Error: fmt.Errorf("model not specified and no 'image_gen' configured in intelligence.matrix")})
			return
		}
	}

	cliCtx, cliCancel := h.GetContextWithCancel(h, c, context.Background())
	resp, errMsg := h.ExecuteMultimodalWithAuthManager(cliCtx, h.HandlerType(), modelName, rawBody, "", "images_edits", contentType)
	if errMsg != nil {
		h.WriteErrorResponse(c, errMsg)
		cliCancel(errMsg.Error)
		return
	}
	c.Data(http.StatusOK, "application/json", resp)
	cliCancel()
}

// AudioTranslations handles /v1/audio/translations
// Translates audio into English text. Uses the same multipart format as transcriptions.
func (h *OpenAIAPIHandler) AudioTranslations(c *gin.Context) {
	contentType := c.GetHeader("Content-Type")

	bodyBytes, err := c.GetRawData()
	if err != nil {
		h.WriteErrorResponse(c, &interfaces.ErrorMessage{StatusCode: http.StatusBadRequest, Error: fmt.Errorf("read error: %v", err)})
		return
	}

	modelName := extractModelFromMultipart(bodyBytes, contentType)

	if modelName == "" {
		if val, ok := h.Cfg.Intelligence.Matrix["transcription"]; ok && val != "" {
			modelName = val
		} else {
			h.WriteErrorResponse(c, &interfaces.ErrorMessage{StatusCode: http.StatusBadRequest, Error: fmt.Errorf("model not specified and no 'transcription' configured in intelligence.matrix")})
			return
		}
	}

	cliCtx, cliCancel := h.GetContextWithCancel(h, c, context.Background())
	resp, errMsg := h.ExecuteMultimodalWithAuthManager(cliCtx, h.HandlerType(), modelName, bodyBytes, "", "audio_translations", contentType)
	if errMsg != nil {
		h.WriteErrorResponse(c, errMsg)
		cliCancel(errMsg.Error)
		return
	}
	c.Data(http.StatusOK, "application/json", resp)
	cliCancel()
}

// MusicGenerations handles POST /v1/music/generations.
//
// Unified endpoint for both text-to-music (music-2.6) and music-cover
// (reference-audio style transfer). Model field switches the mode upstream.
// The adapter (internal/runtime/executor/minimax_music.go) translates to
// MiniMax's native /v1/music_generation and decodes the hex-encoded audio
// into OpenAI-ish JSON {data: {audio: <base64>, duration_ms, sample_rate, ...}}.
//
// Default model comes from intelligence.matrix["music_generation"].
// Plus-plan quota on MiniMax is 100 songs/day per model. Requests can take
// 30–90 seconds sync.
//
// Streaming (stream:true): returns Content-Type: audio/mpeg with raw MP3
// bytes instead of the JSON wrapper. First byte arrives at ~20s TTFB vs
// 30–90s for sync, and the player can start decoding immediately
// (concatenated hex chunks form a valid MP3 from frame one). The terminal
// upstream frame — which duplicates the full song — is discarded, cutting
// client bandwidth by ~75% vs passthrough.
func (h *OpenAIAPIHandler) MusicGenerations(c *gin.Context) {
	rawJSON, err := c.GetRawData()
	if err != nil {
		h.WriteErrorResponse(c, &interfaces.ErrorMessage{StatusCode: http.StatusBadRequest, Error: fmt.Errorf("invalid request: %v", err)})
		return
	}
	modelName := gjson.GetBytes(rawJSON, "model").String()
	if modelName == "" {
		if val, ok := h.Cfg.Intelligence.Matrix["music_generation"]; ok && val != "" {
			modelName = val
		} else {
			h.WriteErrorResponse(c, &interfaces.ErrorMessage{StatusCode: http.StatusBadRequest, Error: fmt.Errorf("model not specified and no 'music_generation' configured in intelligence.matrix")})
			return
		}
	}
	routeModelName := musicGenerationRouteModel(modelName)

	if gjson.GetBytes(rawJSON, "stream").Bool() {
		h.musicGenerationsStream(c, routeModelName, rawJSON)
		return
	}

	cliCtx, cliCancel := h.GetContextWithCancel(h, c, context.Background())
	resp, errMsg := h.ExecuteMultimodalWithAuthManager(cliCtx, h.HandlerType(), routeModelName, rawJSON, "", "music_generation", "application/json")
	if errMsg != nil {
		h.WriteErrorResponse(c, errMsg)
		cliCancel(errMsg.Error)
		return
	}
	c.Data(http.StatusOK, "application/json", resp)
	cliCancel()
}

func musicGenerationRouteModel(modelName string) string {
	normalized := strings.TrimSpace(modelName)
	lower := strings.ToLower(normalized)
	switch lower {
	case "music-cover", "music-cover-free", "minimax:music-cover", "minimax:music-cover-free", "ail-music-cover", "minimax/ail-music-cover":
		return "ail-music-cover"
	case "music-2.6", "music-2.6-free", "minimax:music-2.6", "minimax:music-2.6-free", "ail-music", "minimax/ail-music":
		return "ail-music"
	default:
		if strings.HasPrefix(lower, "minimax:music-cover") || strings.HasPrefix(lower, "minimax/music-cover") {
			return "ail-music-cover"
		}
		if strings.HasPrefix(lower, "minimax:music-2.") || strings.HasPrefix(lower, "minimax/music-2.") {
			return "ail-music"
		}
		return normalized
	}
}

// musicGenerationsStream runs the streaming variant of /v1/music/generations.
// Writes raw MP3 bytes as they arrive from MiniMax so the client can start
// playback before the song finishes rendering upstream.
//
// Header-commit semantics: we hold the response status until the first audio
// byte is ready. If a retryable pre-first-byte error occurs (HTTP 5xx, 429,
// upstream base_resp error, stall_pre_first_byte), we still have the chance
// to return a JSON error with the right status code. Once any byte is
// flushed we commit to 200 audio/mpeg and later errors just close the body.
func (h *OpenAIAPIHandler) musicGenerationsStream(c *gin.Context, modelName string, rawJSON []byte) {
	cliCtx, cliCancel := h.GetContextWithCancel(h, c, context.Background())
	defer cliCancel()

	dataCh, errCh := h.ExecuteStreamMultimodalWithAuthManager(cliCtx, h.HandlerType(), modelName, rawJSON, "", "music_generation", "application/json")

	flusher, canFlush := c.Writer.(http.Flusher)
	headersWritten := false
	for {
		select {
		case <-cliCtx.Done():
			return
		case chunk, ok := <-dataCh:
			if !ok {
				// Channel closed without error — stream ended cleanly.
				if !headersWritten {
					// Upstream produced zero audio bytes and no error — rare,
					// but treat as an upstream failure the client can retry.
					h.WriteErrorResponse(c, &interfaces.ErrorMessage{StatusCode: http.StatusBadGateway, Error: fmt.Errorf("music_generation: empty stream from upstream")})
				}
				return
			}
			if !headersWritten {
				c.Writer.Header().Set("Content-Type", "audio/mpeg")
				c.Writer.Header().Set("Cache-Control", "no-cache")
				c.Writer.Header().Set("X-Accel-Buffering", "no")
				c.Writer.WriteHeader(http.StatusOK)
				headersWritten = true
			}
			if _, wErr := c.Writer.Write(chunk); wErr != nil {
				// Client disconnected mid-stream — stop cleanly.
				return
			}
			if canFlush {
				flusher.Flush()
			}
		case errMsg, ok := <-errCh:
			if !ok || errMsg == nil {
				return
			}
			if !headersWritten {
				// Safe to return a structured error with the right status.
				h.WriteErrorResponse(c, errMsg)
				return
			}
			// Already committed to audio/mpeg — can't retract. Log and close.
			log.Errorf("music_generation stream: mid-stream error after headers committed: %v", errMsg.Error)
			return
		}
	}
}

// MusicLyrics handles POST /v1/music/lyrics.
//
// Thin pass-through over MiniMax's /v1/lyrics_generation. Accepts
// {mode, prompt, lyrics?, title?} (mode defaults to "write_full_song" in
// the adapter if omitted). Returns {song_title, style_tags, lyrics}.
func (h *OpenAIAPIHandler) MusicLyrics(c *gin.Context) {
	rawJSON, err := c.GetRawData()
	if err != nil {
		h.WriteErrorResponse(c, &interfaces.ErrorMessage{StatusCode: http.StatusBadRequest, Error: fmt.Errorf("invalid request: %v", err)})
		return
	}
	modelName := gjson.GetBytes(rawJSON, "model").String()
	if modelName == "" {
		if val, ok := h.Cfg.Intelligence.Matrix["lyrics_generation"]; ok && val != "" {
			modelName = val
		} else {
			// Lyrics has no "real" model name (upstream ignores it), but we
			// need SOMETHING to route with. Default to the same credential
			// the music_generation slot resolves to.
			if val, ok := h.Cfg.Intelligence.Matrix["music_generation"]; ok && val != "" {
				modelName = val
			} else {
				h.WriteErrorResponse(c, &interfaces.ErrorMessage{StatusCode: http.StatusBadRequest, Error: fmt.Errorf("no 'lyrics_generation' or 'music_generation' configured in intelligence.matrix")})
				return
			}
		}
	}

	cliCtx, cliCancel := h.GetContextWithCancel(h, c, context.Background())
	resp, errMsg := h.ExecuteMultimodalWithAuthManager(cliCtx, h.HandlerType(), modelName, rawJSON, "", "lyrics_generation", "application/json")
	if errMsg != nil {
		h.WriteErrorResponse(c, errMsg)
		cliCancel(errMsg.Error)
		return
	}
	c.Data(http.StatusOK, "application/json", resp)
	cliCancel()
}

// AudioTranscriptions handles /v1/audio/transcriptions
func (h *OpenAIAPIHandler) AudioTranscriptions(c *gin.Context) {
	// For multipart, we read the whole body to forward it,
	// and extract the 'model' field from the form data if present.
	contentType := c.GetHeader("Content-Type")

	// Read full body
	bodyBytes, err := c.GetRawData()
	if err != nil {
		h.WriteErrorResponse(c, &interfaces.ErrorMessage{StatusCode: http.StatusBadRequest, Error: fmt.Errorf("read error: %v", err)})
		return
	}

	// Extract model from multipart form data by scanning for the model field value
	modelName := extractModelFromMultipart(bodyBytes, contentType)

	if modelName == "" {
		if val, ok := h.Cfg.Intelligence.Matrix["transcription"]; ok && val != "" {
			modelName = val
		} else {
			h.WriteErrorResponse(c, &interfaces.ErrorMessage{StatusCode: http.StatusBadRequest, Error: fmt.Errorf("model not specified and no 'transcription' configured in intelligence.matrix")})
			return
		}
	}

	cliCtx, cliCancel := h.GetContextWithCancel(h, c, context.Background())
	resp, errMsg := h.ExecuteMultimodalWithAuthManager(cliCtx, h.HandlerType(), modelName, bodyBytes, "", "audio_transcriptions", contentType)
	if errMsg != nil {
		h.WriteErrorResponse(c, errMsg)
		cliCancel(errMsg.Error)
		return
	}
	c.Data(http.StatusOK, "application/json", resp)
	cliCancel()
}

// extractModelFromMultipart scans raw multipart form bytes for a field named "model"
// and returns its text value. Returns "" if not found or on any parse error.
func extractModelFromMultipart(body []byte, contentType string) string {
	// Parse the boundary from Content-Type header
	if contentType == "" {
		return ""
	}

	// Use bytes.Index to find form-data name="model" pattern
	// This is a lightweight approach that avoids full multipart parsing + reconstruction
	modelMarker := []byte(`form-data; name="model"`)
	idx := bytes.Index(body, modelMarker)
	if idx < 0 {
		return ""
	}

	// Skip past the marker and the \r\n\r\n separator to reach the value
	after := body[idx+len(modelMarker):]
	sepIdx := bytes.Index(after, []byte("\r\n\r\n"))
	if sepIdx < 0 {
		return ""
	}
	valueStart := after[sepIdx+4:]

	// Value ends at the next \r\n boundary
	endIdx := bytes.Index(valueStart, []byte("\r\n"))
	if endIdx < 0 {
		return ""
	}

	return string(bytesClean(valueStart[:endIdx]))
}

// bytesClean trims whitespace and control characters from a byte slice.
func bytesClean(b []byte) []byte {
	start := 0
	end := len(b)
	for start < end && (b[start] == ' ' || b[start] == '\t' || b[start] == '\r' || b[start] == '\n') {
		start++
	}
	for end > start && (b[end-1] == ' ' || b[end-1] == '\t' || b[end-1] == '\r' || b[end-1] == '\n') {
		end--
	}
	return b[start:end]
}
