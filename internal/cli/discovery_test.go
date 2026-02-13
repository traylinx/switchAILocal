// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package cli

import (
	"testing"
	"github.com/traylinx/switchAILocal/internal/constant"
)

func TestKnownTools_PositionalArgsSeparator(t *testing.T) {
	// These tools MUST have a PositionalArgsSeparator to prevent argument injection
	criticalTools := map[string]bool{
		constant.GeminiCLI: true,
		constant.VibeCLI:   true,
		constant.ClaudeCLI: true,
		"codex":            true,
	}

	for _, tool := range KnownTools {
		if criticalTools[tool.ProviderKey] {
			if tool.PositionalArgsSeparator != "--" {
				t.Errorf("Security Vulnerability: Tool %q (%s) is missing PositionalArgsSeparator='--'. Found: %q",
					tool.Name, tool.ProviderKey, tool.PositionalArgsSeparator)
			}
		}
	}
}
