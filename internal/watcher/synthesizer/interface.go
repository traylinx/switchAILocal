// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package synthesizer provides auth synthesis strategies for the watcher package.
// It implements the Strategy pattern to support multiple auth sources:
// - ConfigSynthesizer: generates Auth entries from config API keys
// - FileSynthesizer: generates Auth entries from OAuth JSON files
package synthesizer

import (
	coreauth "github.com/traylinx/switchAILocal/sdk/switchailocal/auth"
)

// AuthSynthesizer defines the interface for generating Auth entries from various sources.
type AuthSynthesizer interface {
	// Synthesize generates Auth entries from the given context.
	// Returns a slice of Auth pointers and any error encountered.
	Synthesize(ctx *SynthesisContext) ([]*coreauth.Auth, error)
}
