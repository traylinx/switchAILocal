// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package executor

import (
	"context"
	"net/http"

	switchailocalauth "github.com/traylinx/switchAILocal/sdk/switchailocal/auth"
	switchailocalexecutor "github.com/traylinx/switchAILocal/sdk/switchailocal/executor"
)

// MiniMax does not expose an OpenAI-whisper-compatible /v1/audio/transcriptions
// endpoint, and wire-testing confirmed that M2.7's openai-compat chat endpoint
// silently ignores both `input_audio` and `audio_url` content blocks: the
// model receives only the text prompt and replies "no audio attached". There
// is no first-class bridge we can build today.
//
// Rather than let the caller see MiniMax's opaque 404, short-circuit early
// with a structured 501 that names the right alternatives. This keeps the
// /v1/audio/transcriptions surface honest: it works for providers that
// actually implement it (Groq whisper-large-v3, Alibaba qwen3-asr-flash),
// and returns an actionable error for the one popular provider that doesn't.
//
// If MiniMax ever ships a real ASR endpoint, replace the body of this
// function with the proper adapter — the dispatch site in openai_compat_executor.go
// already routes here for any MiniMax auth_transcriptions request.
func (e *OpenAICompatExecutor) executeMinimaxTranscribe(ctx context.Context, auth *switchailocalauth.Auth, req switchailocalexecutor.Request, baseURL, apiKey string) (switchailocalexecutor.Response, error) {
	_ = ctx
	_ = auth
	_ = req
	_ = baseURL
	_ = apiKey
	return switchailocalexecutor.Response{}, statusErr{
		code: http.StatusNotImplemented,
		msg:  "minimax has no /audio/transcriptions endpoint. Route transcription to a whisper-compatible provider (e.g. whisper-large-v3 on Groq, qwen3-asr-flash on Alibaba). For audio understanding inside chat, use /chat/completions with a provider that actually processes audio content blocks (Gemini, xiaomi-tp:mimo-v2-omni) — M2.7's openai-compat endpoint ignores them.",
	}
}
