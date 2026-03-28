// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package util

import "testing"

func TestExtractThinkTags(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantThinking string
		wantCleaned  string
		wantFound    bool
	}{
		{
			name:         "no think tags",
			input:        "Hello, how can I help you?",
			wantThinking: "",
			wantCleaned:  "Hello, how can I help you?",
			wantFound:    false,
		},
		{
			name:         "simple think block",
			input:        "<think>\nThe user is saying hi.\n</think>\n\nhello",
			wantThinking: "The user is saying hi.",
			wantCleaned:  "hello",
			wantFound:    true,
		},
		{
			name:         "think at start with newlines",
			input:        "<think>\nLet me think about this.\nThis is a simple request.\n</think>\n\nHello! 👋 How can I help you today?",
			wantThinking: "Let me think about this.\nThis is a simple request.",
			wantCleaned:  "Hello! 👋 How can I help you today?",
			wantFound:    true,
		},
		{
			name:         "empty think block",
			input:        "<think></think>hello",
			wantThinking: "",
			wantCleaned:  "hello",
			wantFound:    true,
		},
		{
			name:         "unclosed think tag",
			input:        "<think>\nI am still thinking...",
			wantThinking: "I am still thinking...",
			wantCleaned:  "",
			wantFound:    true,
		},
		{
			name:         "content before think tag",
			input:        "prefix text <think>reasoning</think> suffix text",
			wantThinking: "reasoning",
			wantCleaned:  "prefix text  suffix text",
			wantFound:    true,
		},
		{
			name:         "multiple think blocks",
			input:        "<think>first thought</think>middle<think>second thought</think>end",
			wantThinking: "first thoughtsecond thought",
			wantCleaned:  "middleend",
			wantFound:    true,
		},
		{
			name:         "only think content no answer",
			input:        "<think>\nJust thinking, no answer yet.\n</think>",
			wantThinking: "Just thinking, no answer yet.",
			wantCleaned:  "",
			wantFound:    true,
		},
		{
			name:         "minimax real response format",
			input:        "<think>\nThe user is just saying \"hi\" - a simple greeting. I should respond as Harvey, concise and direct. No need for tools here.\n</think>\n\nHey.",
			wantThinking: "The user is just saying \"hi\" - a simple greeting. I should respond as Harvey, concise and direct. No need for tools here.",
			wantCleaned:  "Hey.",
			wantFound:    true,
		},
		{
			name:         "empty string",
			input:        "",
			wantThinking: "",
			wantCleaned:  "",
			wantFound:    false,
		},
		{
			name:         "close tag without open",
			input:        "hello </think> world",
			wantThinking: "",
			wantCleaned:  "hello </think> world",
			wantFound:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotThinking, gotCleaned, gotFound := ExtractThinkTags(tt.input)
			if gotFound != tt.wantFound {
				t.Errorf("ExtractThinkTags() found = %v, want %v", gotFound, tt.wantFound)
			}
			if gotThinking != tt.wantThinking {
				t.Errorf("ExtractThinkTags() thinking = %q, want %q", gotThinking, tt.wantThinking)
			}
			if gotCleaned != tt.wantCleaned {
				t.Errorf("ExtractThinkTags() cleaned = %q, want %q", gotCleaned, tt.wantCleaned)
			}
		})
	}
}
