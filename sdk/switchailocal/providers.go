// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package switchailocal

import (
	"context"

	"github.com/traylinx/switchAILocal/internal/watcher"
	"github.com/traylinx/switchAILocal/sdk/config"
)

// NewFileTokenClientProvider returns the default token-backed client loader.
func NewFileTokenClientProvider() TokenClientProvider {
	return &fileTokenClientProvider{}
}

type fileTokenClientProvider struct{}

func (p *fileTokenClientProvider) Load(ctx context.Context, cfg *config.Config) (*TokenClientResult, error) {
	// Stateless executors handle tokens
	_ = ctx
	_ = cfg
	return &TokenClientResult{SuccessfulAuthed: 0}, nil
}

// NewAPIKeyClientProvider returns the default API key client loader that reuses existing logic.
func NewAPIKeyClientProvider() APIKeyClientProvider {
	return &apiKeyClientProvider{}
}

type apiKeyClientProvider struct{}

func (p *apiKeyClientProvider) Load(ctx context.Context, cfg *config.Config) (*APIKeyClientResult, error) {
	geminiCount, vertexCompatCount, claudeCount, codexCount, switchAICount, openAICompat := watcher.BuildAPIKeyClients(cfg)
	if ctx != nil {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
	}
	return &APIKeyClientResult{
		GeminiKeyCount:       geminiCount,
		VertexCompatKeyCount: vertexCompatCount,
		ClaudeKeyCount:       claudeCount,
		CodexKeyCount:        codexCount,
		SwitchAIKeyCount:     switchAICount,
		OpenAICompatCount:    openAICompat,
	}, nil
}
