// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package builtin exposes the built-in translator registrations for SDK users.
package builtin

import (
	sdktranslator "github.com/traylinx/switchAILocal/sdk/translator"

	_ "github.com/traylinx/switchAILocal/internal/translator"
)

// Registry exposes the default registry populated with all built-in translators.
func Registry() *sdktranslator.Registry {
	return sdktranslator.Default()
}

// Pipeline returns a pipeline that already contains the built-in translators.
func Pipeline() *sdktranslator.Pipeline {
	return sdktranslator.NewPipeline(sdktranslator.Default())
}
