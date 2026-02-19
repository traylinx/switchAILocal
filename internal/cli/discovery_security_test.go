// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package cli

import (
	"testing"
)

func TestKnownTools_PositionalArgsSeparator(t *testing.T) {
	for _, tool := range KnownTools {
		// Skip tools that use stdin, as they are not vulnerable to command injection via args
		if tool.UseStdin {
			continue
		}

		if tool.PositionalArgsSeparator == "" {
			t.Errorf("Tool %s (%s) is missing PositionalArgsSeparator. This is a security vulnerability.", tool.Name, tool.BinaryName)
		}
	}
}
