package cli

import (
	"testing"

	"github.com/traylinx/switchAILocal/internal/constant"
)

func TestKnownTools_PositionalArgsSeparator(t *testing.T) {
	requiredSeparators := map[string]bool{
		constant.GeminiCLI: true,
		constant.VibeCLI:   true,
		constant.ClaudeCLI: true,
		"codex":            true,
	}

	for _, tool := range KnownTools {
		if requiredSeparators[tool.ProviderKey] {
			if tool.PositionalArgsSeparator != "--" {
				t.Errorf("Tool %s (%s) is missing secure PositionalArgsSeparator: '--'", tool.Name, tool.ProviderKey)
			}
		}
	}
}
