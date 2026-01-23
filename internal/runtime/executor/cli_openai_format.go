// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package executor

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// OpenAI Chat Completion Response structures

// OpenAIChatResponse represents a non-streaming OpenAI chat completion response.
type OpenAIChatResponse struct {
	ID      string             `json:"id"`
	Object  string             `json:"object"`
	Created int64              `json:"created"`
	Model   string             `json:"model"`
	Choices []OpenAIChatChoice `json:"choices"`
	Usage   *OpenAIUsage       `json:"usage,omitempty"`
}

// OpenAIChatChoice represents a choice in an OpenAI chat completion.
type OpenAIChatChoice struct {
	Index        int           `json:"index"`
	Message      OpenAIMessage `json:"message"`
	FinishReason string        `json:"finish_reason"`
}

// OpenAIMessage represents a message in an OpenAI chat completion.
type OpenAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// OpenAIUsage represents token usage information.
type OpenAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// OpenAI SSE Chunk structures for streaming

// OpenAIStreamChunk represents a streaming chunk in OpenAI SSE format.
type OpenAIStreamChunk struct {
	ID      string                    `json:"id"`
	Object  string                    `json:"object"`
	Created int64                     `json:"created"`
	Model   string                    `json:"model"`
	Choices []OpenAIStreamChunkChoice `json:"choices"`
}

// OpenAIStreamChunkChoice represents a choice in a streaming chunk.
type OpenAIStreamChunkChoice struct {
	Index        int                    `json:"index"`
	Delta        OpenAIStreamChunkDelta `json:"delta"`
	FinishReason *string                `json:"finish_reason"`
}

// OpenAIStreamChunkDelta represents the delta content in a streaming chunk.
type OpenAIStreamChunkDelta struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

// BuildOpenAIResponse wraps content in OpenAI chat completion format.
func BuildOpenAIResponse(model, content string, usage *OpenAIUsage) ([]byte, error) {
	resp := OpenAIChatResponse{
		ID:      fmt.Sprintf("chatcmpl-%s", uuid.New().String()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []OpenAIChatChoice{{
			Index:        0,
			Message:      OpenAIMessage{Role: "assistant", Content: content},
			FinishReason: "stop",
		}},
		Usage: usage,
	}
	return json.Marshal(resp)
}

// BuildOpenAIStreamChunk creates an SSE chunk for streaming responses.
// Returns raw JSON (upstream handler adds "data: " prefix).
func BuildOpenAIStreamChunk(model, content string, isFirst bool) []byte {
	chunk := OpenAIStreamChunk{
		ID:      fmt.Sprintf("chatcmpl-%s", uuid.New().String()),
		Object:  "chat.completion.chunk",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []OpenAIStreamChunkChoice{{
			Index: 0,
			Delta: OpenAIStreamChunkDelta{Content: content},
		}},
	}
	if isFirst {
		chunk.Choices[0].Delta.Role = "assistant"
	}
	data, _ := json.Marshal(chunk)
	return data
}

// BuildOpenAIStreamDone returns the final [DONE] marker for SSE streams.
// Returns raw marker (upstream handler adds "data: " prefix).
func BuildOpenAIStreamDone() []byte {
	return []byte("[DONE]")
}

// BuildOpenAIStreamFinishChunk creates the final chunk with finish_reason.
// Returns raw JSON (upstream handler adds "data: " prefix).
func BuildOpenAIStreamFinishChunk(model string) []byte {
	finishReason := "stop"
	chunk := OpenAIStreamChunk{
		ID:      fmt.Sprintf("chatcmpl-%s", uuid.New().String()),
		Object:  "chat.completion.chunk",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []OpenAIStreamChunkChoice{{
			Index:        0,
			Delta:        OpenAIStreamChunkDelta{},
			FinishReason: &finishReason,
		}},
	}
	data, _ := json.Marshal(chunk)
	return data
}
