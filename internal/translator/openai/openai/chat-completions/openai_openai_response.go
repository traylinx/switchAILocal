// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package chat_completions provides response translation functionality for OpenAI-to-OpenAI passthrough.
// This package normalizes non-standard response formats (e.g., <think> tags embedded in content)
// into proper OpenAI Chat Completions format with reasoning_content support.
package chat_completions

import (
	"bytes"
	"context"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"github.com/traylinx/switchAILocal/internal/util"
)

// thinkTagStreamState tracks the state machine for <think> tag processing across streaming chunks.
type thinkTagStreamState struct {
	insideThink bool
}

// ConvertOpenAIResponseToOpenAI translates a single chunk of a streaming response.
// It normalizes <think>...</think> tags in delta.content into delta.reasoning_content,
// following the OpenAI standard for reasoning models.
//
// Parameters:
//   - ctx: The context for the request, used for cancellation and timeout handling
//   - modelName: The name of the model being used for the response
//   - rawJSON: The raw JSON response chunk
//   - param: A pointer to a parameter object for maintaining state between calls
//
// Returns:
//   - []string: A slice of strings, each containing an OpenAI-compatible JSON response
func ConvertOpenAIResponseToOpenAI(_ context.Context, _ string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, param *any) []string {
	if bytes.HasPrefix(rawJSON, []byte("data:")) {
		rawJSON = bytes.TrimSpace(rawJSON[5:])
	}
	if bytes.Equal(rawJSON, []byte("[DONE]")) {
		return []string{}
	}

	// Fast path: check if content contains <think> — if not, pass through unchanged.
	// This avoids JSON parsing overhead for the vast majority of chunks.
	content := gjson.GetBytes(rawJSON, "choices.0.delta.content")
	if !content.Exists() {
		return []string{string(rawJSON)}
	}
	contentStr := content.String()

	// Initialize state on first call
	if *param == nil {
		*param = &thinkTagStreamState{}
	}
	state := (*param).(*thinkTagStreamState)

	// If we're not inside a think block and there's no <think> tag, pass through
	if !state.insideThink && !strings.Contains(contentStr, "<think>") {
		return []string{string(rawJSON)}
	}

	return processStreamChunkThinkTags(rawJSON, contentStr, state)
}

// processStreamChunkThinkTags handles the state machine logic for splitting
// <think> tagged content into reasoning_content across streaming chunks.
func processStreamChunkThinkTags(rawJSON []byte, contentStr string, state *thinkTagStreamState) []string {
	const openTag = "<think>"
	const closeTag = "</think>"

	var results []string

	for len(contentStr) > 0 {
		if !state.insideThink {
			// Look for <think> tag
			idx := strings.Index(contentStr, openTag)
			if idx == -1 {
				// No think tag — emit remaining as content
				if contentStr != "" {
					chunk := buildChunkWithField(rawJSON, "choices.0.delta.content", contentStr)
					results = append(results, chunk)
				}
				break
			}

			// Content before <think> goes to content
			if idx > 0 {
				chunk := buildChunkWithField(rawJSON, "choices.0.delta.content", contentStr[:idx])
				results = append(results, chunk)
			}

			contentStr = contentStr[idx+len(openTag):]
			state.insideThink = true
			// Fall through to process the thinking part
		}

		if state.insideThink {
			// Look for </think> tag
			endIdx := strings.Index(contentStr, closeTag)
			if endIdx == -1 {
				// No closing tag in this chunk — all remaining is thinking
				if contentStr != "" {
					chunk := buildChunkWithField(rawJSON, "choices.0.delta.reasoning_content", contentStr)
					results = append(results, chunk)
				}
				break
			}

			// Content before </think> goes to reasoning_content
			if endIdx > 0 {
				chunk := buildChunkWithField(rawJSON, "choices.0.delta.reasoning_content", contentStr[:endIdx])
				results = append(results, chunk)
			}

			contentStr = contentStr[endIdx+len(closeTag):]
			state.insideThink = false
			// Continue loop to process any remaining content after </think>
		}
	}

	// If we produced no results (e.g., empty <think> tag), return empty chunk
	if len(results) == 0 {
		return []string{}
	}

	return results
}

// buildChunkWithField creates a streaming chunk JSON with the specified field set
// and the other field (content or reasoning_content) cleared.
func buildChunkWithField(originalChunk []byte, field string, value string) string {
	result := make([]byte, len(originalChunk))
	copy(result, originalChunk)

	// Set the target field
	result, _ = sjson.SetBytes(result, field, value)

	// Clear the other field to avoid sending both
	if field == "choices.0.delta.reasoning_content" {
		result, _ = sjson.SetBytes(result, "choices.0.delta.content", "")
	} else {
		// Remove reasoning_content if we're setting content
		result, _ = sjson.DeleteBytes(result, "choices.0.delta.reasoning_content")
	}

	return string(result)
}

// ConvertOpenAIResponseToOpenAINonStream converts a non-streaming OpenAI response.
// It normalizes <think>...</think> tags in message.content into message.reasoning_content.
//
// Parameters:
//   - ctx: The context for the request
//   - modelName: The name of the model
//   - rawJSON: The raw JSON response
//   - param: A pointer to a parameter object
//
// Returns:
//   - string: An OpenAI-compatible JSON response with normalized reasoning_content
func ConvertOpenAIResponseToOpenAINonStream(ctx context.Context, modelName string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, param *any) string {
	// Fast path: if no <think> tag in the content, return unchanged
	content := gjson.GetBytes(rawJSON, "choices.0.message.content")
	if !content.Exists() {
		return string(rawJSON)
	}

	contentStr := content.String()
	thinking, cleaned, found := util.ExtractThinkTags(contentStr)
	if !found {
		return string(rawJSON)
	}

	// Normalize: set cleaned content and add reasoning_content
	result := rawJSON
	result, _ = sjson.SetBytes(result, "choices.0.message.content", cleaned)
	if thinking != "" {
		result, _ = sjson.SetBytes(result, "choices.0.message.reasoning_content", thinking)
	}

	return string(result)
}
