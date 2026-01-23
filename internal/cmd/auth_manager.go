// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package cmd

import (
	sdkAuth "github.com/traylinx/switchAILocal/sdk/auth"
)

// newAuthManager creates a new authentication manager instance with all supported
// authenticators and a file-based token store. It initializes authenticators for
// Gemini, Codex, Claude, Qwen, iFlow, Antigravity, Vibe, and Ollama providers.
//
// Returns:
//   - *sdkAuth.Manager: A configured authentication manager instance
func newAuthManager() *sdkAuth.Manager {
	store := sdkAuth.GetTokenStore()
	manager := sdkAuth.NewManager(store,
		sdkAuth.NewGeminiAuthenticator(),
		sdkAuth.NewCodexAuthenticator(),
		sdkAuth.NewClaudeAuthenticator(),
		sdkAuth.NewQwenAuthenticator(),
		sdkAuth.NewIFlowAuthenticator(),
		sdkAuth.NewAntigravityAuthenticator(),
		sdkAuth.NewVibeAuthenticator(),
		sdkAuth.NewOllamaAuthenticator(),
	)
	return manager
}
