// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package cli

import (
	"testing"
)

func TestKnownTools_PositionalArgsSeparator(t *testing.T) {
	for _, tool := range KnownTools {
		t.Run(tool.Name, func(t *testing.T) {
			if tool.PositionalArgsSeparator != "--" {
				t.Errorf("Tool %q (provider: %s) is missing secure PositionalArgsSeparator: '--'", tool.Name, tool.ProviderKey)
			}
		})
	}
}
