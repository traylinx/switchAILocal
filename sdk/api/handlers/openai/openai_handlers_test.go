// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package openai

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tidwall/gjson"
)

func TestConvertCompletionsRequestToChatCompletions(t *testing.T) {
	input := `{
		"model": "gpt-3.5-turbo-instruct",
		"prompt": "Say this is a test",
		"max_tokens": 7,
		"temperature": 0,
		"top_p": 1,
		"frequency_penalty": 0,
		"presence_penalty": 0,
		"stream": true,
		"stop": "\n",
		"logprobs": 1,
		"echo": true
	}`

	output := convertCompletionsRequestToChatCompletions([]byte(input))

	root := gjson.ParseBytes(output)
	assert.Equal(t, "gpt-3.5-turbo-instruct", root.Get("model").String())
	assert.Equal(t, "user", root.Get("messages.0.role").String())
	assert.Equal(t, "Say this is a test", root.Get("messages.0.content").String())
	assert.Equal(t, int64(7), root.Get("max_tokens").Int())
	assert.Equal(t, float64(0), root.Get("temperature").Float())
	assert.Equal(t, float64(1), root.Get("top_p").Float())
	assert.Equal(t, float64(0), root.Get("frequency_penalty").Float())
	assert.Equal(t, float64(0), root.Get("presence_penalty").Float())
	assert.Equal(t, true, root.Get("stream").Bool())
	assert.Equal(t, "\n", root.Get("stop").String())
	assert.Equal(t, true, root.Get("logprobs").Bool())
	assert.Equal(t, true, root.Get("echo").Bool())
}

func TestConvertChatCompletionsResponseToCompletions(t *testing.T) {
	input := `{
		"id": "chatcmpl-123",
		"object": "chat.completion",
		"created": 1677652288,
		"model": "gpt-3.5-turbo-0613",
		"choices": [{
			"index": 0,
			"message": {
				"role": "assistant",
				"content": "This is a test"
			},
			"finish_reason": "stop"
		}],
		"usage": {
			"prompt_tokens": 9,
			"completion_tokens": 12,
			"total_tokens": 21
		}
	}`

	output := convertChatCompletionsResponseToCompletions([]byte(input))

	root := gjson.ParseBytes(output)
	assert.Equal(t, "chatcmpl-123", root.Get("id").String())
	assert.Equal(t, "text_completion", root.Get("object").String())
	assert.Equal(t, int64(1677652288), root.Get("created").Int())
	assert.Equal(t, "gpt-3.5-turbo-0613", root.Get("model").String())
	assert.Equal(t, "This is a test", root.Get("choices.0.text").String())
	assert.Equal(t, int64(0), root.Get("choices.0.index").Int())
	assert.Equal(t, "stop", root.Get("choices.0.finish_reason").String())
	assert.Equal(t, int64(9), root.Get("usage.prompt_tokens").Int())
}

func TestConvertChatCompletionsStreamChunkToCompletions(t *testing.T) {
	input := `{
		"id": "chatcmpl-123",
		"object": "chat.completion.chunk",
		"created": 1694268190,
		"model": "gpt-3.5-turbo-0613",
		"choices": [
			{
				"index": 0,
				"delta": {
					"content": "Hello"
				},
				"finish_reason": null
			}
		]
	}`

	output := convertChatCompletionsStreamChunkToCompletions([]byte(input))
	assert.NotNil(t, output)

	root := gjson.ParseBytes(output)
	assert.Equal(t, "chatcmpl-123", root.Get("id").String())
	assert.Equal(t, "text_completion", root.Get("object").String())
	assert.Equal(t, int64(1694268190), root.Get("created").Int())
	assert.Equal(t, "gpt-3.5-turbo-0613", root.Get("model").String())
	assert.Equal(t, "Hello", root.Get("choices.0.text").String())
	assert.Equal(t, int64(0), root.Get("choices.0.index").Int())
	// The original implementation maps null to empty string because .String() on null returns ""
	// and "" != "null" is true.
	assert.Equal(t, "", root.Get("choices.0.finish_reason").String())
}

func TestConvertChatCompletionsStreamChunkToCompletions_Empty(t *testing.T) {
	input := `{
		"id": "chatcmpl-123",
		"object": "chat.completion.chunk",
		"created": 1694268190,
		"model": "gpt-3.5-turbo-0613",
		"choices": [
			{
				"index": 0,
				"delta": {
					"role": "assistant"
				},
				"finish_reason": null
			}
		]
	}`

	output := convertChatCompletionsStreamChunkToCompletions([]byte(input))
	assert.Nil(t, output)
}
