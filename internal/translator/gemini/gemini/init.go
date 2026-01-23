// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package gemini

import (
	. "github.com/traylinx/switchAILocal/internal/constant"
	"github.com/traylinx/switchAILocal/internal/interfaces"
	"github.com/traylinx/switchAILocal/internal/translator/translator"
)

// Register a no-op response translator and a request normalizer for Gemini→Gemini.
// The request converter ensures missing or invalid roles are normalized to valid values.
func init() {
	translator.Register(
		Gemini,
		Gemini,
		ConvertGeminiRequestToGemini,
		interfaces.TranslateResponse{
			Stream:     PassthroughGeminiResponseStream,
			NonStream:  PassthroughGeminiResponseNonStream,
			TokenCount: GeminiTokenCount,
		},
	)
}
