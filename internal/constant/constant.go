// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package constant defines provider name constants used throughout the switchAILocal.
// These constants identify different AI service providers and their variants,
// ensuring consistent naming across the application.
package constant

const (
	// Gemini represents the Google Gemini provider identifier.
	Gemini = "gemini"

	// GeminiCLI represents the Google Gemini CLI provider identifier.
	GeminiCLI = "geminicli"

	// Codex represents the OpenAI Codex provider identifier.
	Codex = "codex"

	// Claude represents the Anthropic Claude provider identifier.
	Claude = "claude"

	// OpenAI represents the OpenAI provider identifier.
	OpenAI = "openai"

	// OpenaiResponse represents the OpenAI response format identifier.
	OpenaiResponse = "openai-response"

	// Antigravity represents the Antigravity response format identifier.
	Antigravity = "antigravity"

	// ClaudeCLI represents the Anthropic Claude CLI provider identifier.
	ClaudeCLI = "claudecli"

	// VibeCLI represents the Mistral Vibe CLI provider identifier.
	VibeCLI = "vibecli"

	// OpenCode represents the OpenCode provider identifier.
	OpenCode = "opencode"

	// MaxStreamingScannerBuffer is the maximum buffer size for the streaming scanner (1MB).
	MaxStreamingScannerBuffer = 1 * 1024 * 1024
)
