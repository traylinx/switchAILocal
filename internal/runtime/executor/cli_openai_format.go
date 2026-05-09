// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package executor

import (
	"encoding/json"
	"strconv"
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
		ID:      "chatcmpl-" + uuid.New().String(),
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
		ID:      "chatcmpl-" + uuid.New().String(),
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
		ID:      "chatcmpl-" + uuid.New().String(),
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

// streamChunkID returns a pre-formatted chunk ID string.
// Called once per stream to avoid UUID generation per chunk.
func streamChunkID() string {
	return "chatcmpl-" + uuid.New().String()
}

// BuildOpenAIStreamChunkFast creates an SSE chunk using pre-formatted string templates.
// This avoids struct allocation and reflection-based json.Marshal (~5x faster).
// The id and created are pre-computed once per stream (OpenAI spec: same for all chunks).
func BuildOpenAIStreamChunkFast(id string, created int64, model, content string, isFirst bool) []byte {
	// Pre-calculate exact capacity to avoid reallocations.
	capacity := 128 + len(id) + len(model) + len(content) + 32

	buf := make([]byte, 0, capacity)
	buf = append(buf, `{"id":"`...)
	buf = append(buf, id...)
	buf = append(buf, `","object":"chat.completion.chunk","created":`...)
	buf = strconv.AppendInt(buf, created, 10)
	buf = append(buf, `,"model":"`...)
	buf = jsonEscapeStringToBuf(buf, model)
	buf = append(buf, `","choices":[{"index":0,"delta":{`...)

	if isFirst {
		buf = append(buf, `"role":"assistant","content":"`...)
		buf = jsonEscapeStringToBuf(buf, content)
		buf = append(buf, `"}`...)
	} else {
		buf = append(buf, `"content":"`...)
		buf = jsonEscapeStringToBuf(buf, content)
		buf = append(buf, `"}`...)
	}

	buf = append(buf, `,"finish_reason":null}]}`...)
	return buf
}

// BuildOpenAIStreamFinishChunkFast creates the final finish chunk using templates.
func BuildOpenAIStreamFinishChunkFast(id string, created int64, model string) []byte {
	capacity := 128 + len(id) + len(model) + 32
	buf := make([]byte, 0, capacity)
	buf = append(buf, `{"id":"`...)
	buf = append(buf, id...)
	buf = append(buf, `","object":"chat.completion.chunk","created":`...)
	buf = strconv.AppendInt(buf, created, 10)
	buf = append(buf, `,"model":"`...)
	buf = jsonEscapeStringToBuf(buf, model)
	buf = append(buf, `","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`...)
	return buf
}

// jsonEscapeStringToBuf appends the JSON-escaped version of s to buf.
// This is more efficient than returning a string as it avoids extra allocation.
func jsonEscapeStringToBuf(buf []byte, s string) []byte {
	// Fast path: most strings have no special chars
	needsEscape := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '"' || c == '\\' || c == '\n' || c == '\r' || c == '\t' || c < 0x20 {
			needsEscape = true
			break
		}
	}
	if !needsEscape {
		return append(buf, s...)
	}

	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '"':
			buf = append(buf, `\"`...)
		case '\\':
			buf = append(buf, `\\`...)
		case '\n':
			buf = append(buf, `\n`...)
		case '\r':
			buf = append(buf, `\r`...)
		case '\t':
			buf = append(buf, `\t`...)
		default:
			if c < 0x20 {
				buf = append(buf, `\u00`...)
				if c < 0x10 {
					buf = append(buf, '0')
				}
				buf = strconv.AppendInt(buf, int64(c), 16)
			} else {
				buf = append(buf, c)
			}
		}
	}
	return buf
}

// jsonEscapeString is kept for backward compatibility if used elsewhere,
// but now just delegates to jsonEscapeStringToBuf
func jsonEscapeString(s string) string {
	return string(jsonEscapeStringToBuf(make([]byte, 0, len(s)), s))
}
