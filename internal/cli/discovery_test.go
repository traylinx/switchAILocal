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
			t.Errorf("Tool %q (%s) is missing PositionalArgsSeparator. This is a security vulnerability allowing command injection.", tool.Name, tool.ProviderKey)
		}
	}
}
