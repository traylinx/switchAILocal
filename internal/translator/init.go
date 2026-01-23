// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package translator

import (
	_ "github.com/traylinx/switchAILocal/internal/translator/claude/gemini"
	_ "github.com/traylinx/switchAILocal/internal/translator/claude/gemini-cli"
	_ "github.com/traylinx/switchAILocal/internal/translator/claude/openai/chat-completions"
	_ "github.com/traylinx/switchAILocal/internal/translator/claude/openai/responses"

	_ "github.com/traylinx/switchAILocal/internal/translator/codex/claude"
	_ "github.com/traylinx/switchAILocal/internal/translator/codex/gemini"
	_ "github.com/traylinx/switchAILocal/internal/translator/codex/gemini-cli"
	_ "github.com/traylinx/switchAILocal/internal/translator/codex/openai/chat-completions"
	_ "github.com/traylinx/switchAILocal/internal/translator/codex/openai/responses"

	_ "github.com/traylinx/switchAILocal/internal/translator/gemini-cli/claude"
	_ "github.com/traylinx/switchAILocal/internal/translator/gemini-cli/gemini"
	_ "github.com/traylinx/switchAILocal/internal/translator/gemini-cli/openai/chat-completions"
	_ "github.com/traylinx/switchAILocal/internal/translator/gemini-cli/openai/responses"

	_ "github.com/traylinx/switchAILocal/internal/translator/gemini/claude"
	_ "github.com/traylinx/switchAILocal/internal/translator/gemini/gemini"
	_ "github.com/traylinx/switchAILocal/internal/translator/gemini/gemini-cli"
	_ "github.com/traylinx/switchAILocal/internal/translator/gemini/openai/chat-completions"
	_ "github.com/traylinx/switchAILocal/internal/translator/gemini/openai/responses"

	_ "github.com/traylinx/switchAILocal/internal/translator/openai/claude"
	_ "github.com/traylinx/switchAILocal/internal/translator/openai/gemini"
	_ "github.com/traylinx/switchAILocal/internal/translator/openai/gemini-cli"
	_ "github.com/traylinx/switchAILocal/internal/translator/openai/openai/chat-completions"
	_ "github.com/traylinx/switchAILocal/internal/translator/openai/openai/responses"

	_ "github.com/traylinx/switchAILocal/internal/translator/antigravity/claude"
	_ "github.com/traylinx/switchAILocal/internal/translator/antigravity/gemini"
	_ "github.com/traylinx/switchAILocal/internal/translator/antigravity/openai/chat-completions"
	_ "github.com/traylinx/switchAILocal/internal/translator/antigravity/openai/responses"
)
