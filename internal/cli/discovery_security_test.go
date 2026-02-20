// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKnownTools_PositionalArgsSeparator(t *testing.T) {
	for _, tool := range KnownTools {
		// Verify that Anthropic Claude CLI uses the security feature to prevent argument injection
		if tool.Name == "Anthropic Claude CLI" {
			assert.Equal(t, "--", tool.PositionalArgsSeparator, "Anthropic Claude CLI must use '--' separator to prevent argument injection")
		}

		// Verify that other tools using stdin (like Gemini) also use the separator if configured
		if tool.UseStdin {
			if tool.PositionalArgsSeparator != "" {
				assert.Equal(t, "--", tool.PositionalArgsSeparator, "Tools using stdin should use standard '--' separator if configured")
			}
		}
	}
}
