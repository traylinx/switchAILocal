package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/traylinx/switchAILocal/internal/constant"
)

func TestKnownTools_PositionalArgsSeparator(t *testing.T) {
	// Security requirement: Tools that accept positional arguments as prompts
	// MUST use "--" to separate flags from the prompt.
	// This prevents argument injection attacks where a prompt starting with "-"
	// is interpreted as a flag.

	requiredTools := []string{
		constant.GeminiCLI,
		constant.ClaudeCLI,
	}

	for _, tool := range KnownTools {
		for _, required := range requiredTools {
			if tool.ProviderKey == required {
				assert.Equal(t, "--", tool.PositionalArgsSeparator, "Tool %s (%s) must have PositionalArgsSeparator set to '--'", tool.Name, tool.ProviderKey)
			}
		}
	}
}
