// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package cli

import (
	"testing"

	"github.com/traylinx/switchAILocal/internal/constant"
)

// TestKnownTools_PositionalArgsSeparator verifies that sensitive CLI tools
// have a PositionalArgsSeparator configured to prevent argument injection.
func TestKnownTools_PositionalArgsSeparator(t *testing.T) {
	// Map of provider keys that require a separator
	requiredSeparators := map[string]string{
		constant.GeminiCLI: "--",
		constant.ClaudeCLI: "--",
	}

	for _, tool := range KnownTools {
		expectedSep, required := requiredSeparators[tool.ProviderKey]
		if required {
			if tool.PositionalArgsSeparator != expectedSep {
				t.Errorf("Security Vulnerability: Tool %s (%s) is missing PositionalArgsSeparator. Expected %q, got %q",
					tool.Name, tool.ProviderKey, expectedSep, tool.PositionalArgsSeparator)
			}
		}
	}
}
