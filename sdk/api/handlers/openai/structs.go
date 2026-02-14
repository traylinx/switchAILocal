// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package openai

// CompletionRequest represents an OpenAI completions API request.
type CompletionRequest struct {
	Model            string      `json:"model"`
	Prompt           any         `json:"prompt,omitempty"` // string or []string
	Suffix           string      `json:"suffix,omitempty"`
	MaxTokens        *int        `json:"max_tokens,omitempty"`
	Temperature      *float64    `json:"temperature,omitempty"`
	TopP             *float64    `json:"top_p,omitempty"`
	N                *int        `json:"n,omitempty"`
	Stream           bool        `json:"stream,omitempty"`
	Logprobs         *int        `json:"logprobs,omitempty"`
	Echo             bool        `json:"echo,omitempty"`
	Stop             any         `json:"stop,omitempty"` // string or []string
	PresencePenalty  *float64    `json:"presence_penalty,omitempty"`
	FrequencyPenalty *float64    `json:"frequency_penalty,omitempty"`
	BestOf           *int        `json:"best_of,omitempty"`
	User             string      `json:"user,omitempty"`
	TopLogprobs      *int        `json:"top_logprobs,omitempty"` // Not standard in completions but used in conversion
}

// ChatCompletionRequest represents an OpenAI chat completions API request.
type ChatCompletionRequest struct {
	Model            string                 `json:"model"`
	Messages         []ChatCompletionMessage `json:"messages"`
	MaxTokens        *int                   `json:"max_tokens,omitempty"`
	Temperature      *float64               `json:"temperature,omitempty"`
	TopP             *float64               `json:"top_p,omitempty"`
	N                *int                   `json:"n,omitempty"`
	Stream           bool                   `json:"stream,omitempty"`
	Stop             any                    `json:"stop,omitempty"` // string or []string
	PresencePenalty  *float64               `json:"presence_penalty,omitempty"`
	FrequencyPenalty *float64               `json:"frequency_penalty,omitempty"`
	LogitBias        map[string]int         `json:"logit_bias,omitempty"`
	User             string                 `json:"user,omitempty"`
	Logprobs         bool                   `json:"logprobs,omitempty"`
	TopLogprobs      *int                   `json:"top_logprobs,omitempty"`
	Echo             bool                   `json:"echo,omitempty"` // Not standard in chat completions but used in conversion
}

type ChatCompletionMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	Name    string `json:"name,omitempty"`
}

// CompletionResponse represents an OpenAI completions API response.
type CompletionResponse struct {
	ID      string             `json:"id"`
	Object  string             `json:"object"`
	Created int64              `json:"created"`
	Model   string             `json:"model"`
	Choices []CompletionChoice `json:"choices"`
	Usage   *Usage             `json:"usage,omitempty"`
}

type CompletionChoice struct {
	Text         string `json:"text"`
	Index        int    `json:"index"`
	Logprobs     any    `json:"logprobs,omitempty"`
	FinishReason string `json:"finish_reason,omitempty"`
}

// ChatCompletionResponse represents an OpenAI chat completions API response.
type ChatCompletionResponse struct {
	ID      string                 `json:"id"`
	Object  string                 `json:"object"`
	Created int64                  `json:"created"`
	Model   string                 `json:"model"`
	Choices []ChatCompletionChoice `json:"choices"`
	Usage   *Usage                 `json:"usage,omitempty"`
}

type ChatCompletionChoice struct {
	Index        int                   `json:"index"`
	Message      ChatCompletionMessage `json:"message"`
	FinishReason string                `json:"finish_reason"`
	Logprobs     any                   `json:"logprobs,omitempty"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ChatCompletionStreamResponse represents a chunk of a streaming chat completion response.
type ChatCompletionStreamResponse struct {
	ID      string                       `json:"id"`
	Object  string                       `json:"object"`
	Created int64                        `json:"created"`
	Model   string                       `json:"model"`
	Choices []ChatCompletionStreamChoice `json:"choices"`
}

type ChatCompletionStreamChoice struct {
	Index        int                       `json:"index"`
	Delta        ChatCompletionStreamDelta `json:"delta"`
	FinishReason *string                   `json:"finish_reason"` // Pointer to handle null vs empty string if needed
	Logprobs     any                       `json:"logprobs,omitempty"`
}

type ChatCompletionStreamDelta struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}
