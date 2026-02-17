// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package cli

import (
	"testing"
)

func TestKnownTools_PositionalArgsSeparator(t *testing.T) {
	for _, tool := range KnownTools {
		if tool.PositionalArgsSeparator == "" {
			t.Errorf("Security vulnerability: Tool %s (%s) is missing PositionalArgsSeparator. This allows command injection.", tool.Name, tool.ProviderKey)
		}
		if tool.PositionalArgsSeparator != "" && tool.PositionalArgsSeparator != "--" {
			t.Logf("Warning: Tool %s (%s) uses non-standard separator %q", tool.Name, tool.ProviderKey, tool.PositionalArgsSeparator)
		}
	}
}
