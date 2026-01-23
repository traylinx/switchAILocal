// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package executor

import (
	"testing"
)

func TestLocalCLIExecutor_ExtractContent(t *testing.T) {
	tests := []struct {
		name         string
		supportsJSON bool
		raw          string
		want         string
	}{
		{
			name:         "Standard JSON outcome with response field",
			supportsJSON: true,
			raw:          `{"response": "Hello World", "stats": {}}`,
			want:         "Hello World",
		},
		{
			name:         "Claude CLI style JSON with result field",
			supportsJSON: true,
			raw:          `{"result": "Hello from Claude", "type": "result"}`,
			want:         "Hello from Claude",
		},
		{
			name:         "Claude CLI style JSON with leading noise",
			supportsJSON: true,
			raw:          "Loaded cached credentials...\n{\"result\": \"Hello from Claude with noise\", \"type\": \"result\"}",
			want:         "Hello from Claude with noise",
		},
		{
			name:         "Claude CLI style JSON with trailing noise",
			supportsJSON: true,
			raw:          "{\"result\": \"Hello from Claude with trailing noise\", \"type\": \"result\"}\nDone.",
			want:         "Hello from Claude with trailing noise",
		},
		{
			name:         "Claude CLI style JSON with both leading and trailing noise",
			supportsJSON: true,
			raw:          "Logs here...\n{\"result\": \"Hello from Claude nested\", \"type\": \"result\"}\nFinished.",
			want:         "Hello from Claude nested",
		},
		{
			name:         "Non-JSON raw text fallback",
			supportsJSON: true,
			raw:          "Plain text response",
			want:         "Plain text response",
		},
		{
			name:         "Raw text when JSON not supported",
			supportsJSON: false,
			raw:          `{"response": "hidden"}`,
			want:         `{"response": "hidden"}`,
		},
		{
			name:         "Noise filtering",
			supportsJSON: false,
			raw:          "[STARTUP] context initialized\nLoaded cached credentials\nActual Message",
			want:         "Actual Message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &LocalCLIExecutor{
				SupportsJSON: tt.supportsJSON,
			}
			if got := e.extractContent(tt.raw); got != tt.want {
				t.Errorf("LocalCLIExecutor.extractContent() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLocalCLIExecutor_ExtractUsage(t *testing.T) {
	tests := []struct {
		name         string
		supportsJSON bool
		raw          string
		wantPrompt   int
		wantTotal    int
	}{
		{
			name:         "Standard Stats layout",
			supportsJSON: true,
			raw:          `{"stats": {"models": {"m1": {"tokens": {"prompt": 10, "total": 25}}}}}`,
			wantPrompt:   10,
			wantTotal:    25,
		},
		{
			name:         "Claude CLI style usage (to be implemented)",
			supportsJSON: true,
			raw:          `{"usage": {"input_tokens": 100, "output_tokens": 50}}`,
			wantPrompt:   100,
			wantTotal:    150,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &LocalCLIExecutor{
				SupportsJSON: tt.supportsJSON,
			}
			got := e.extractUsage(tt.raw)
			if tt.wantTotal == 0 {
				if got != nil {
					t.Errorf("expected nil usage, got %v", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("expected usage, got nil")
			}
			if got.PromptTokens != tt.wantPrompt {
				t.Errorf("PromptTokens = %v, want %v", got.PromptTokens, tt.wantPrompt)
			}
			if got.TotalTokens != tt.wantTotal {
				t.Errorf("TotalTokens = %v, want %v", got.TotalTokens, tt.wantTotal)
			}
		})
	}
}
