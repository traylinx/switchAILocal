// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package gemini

import (
	. "github.com/traylinx/switchAILocal/internal/constant"
	"github.com/traylinx/switchAILocal/internal/interfaces"
	"github.com/traylinx/switchAILocal/internal/translator/translator"
)

func init() {
	translator.Register(
		Gemini,
		GeminiCLI,
		ConvertGeminiRequestToGeminiCLI,
		interfaces.TranslateResponse{
			Stream:     ConvertGeminiCliResponseToGemini,
			NonStream:  ConvertGeminiCliResponseToGeminiNonStream,
			TokenCount: GeminiTokenCount,
		},
	)
}
