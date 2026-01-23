// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package discovery provides model discovery interfaces and implementations.
package discovery

import (
	"context"

	"github.com/traylinx/switchAILocal/internal/registry"
)

// ModelDiscoverer is the interface that all discovery strategies must implement.
// It returns a list of discovered model definitions.
type ModelDiscoverer interface {
	// Discover returns a list of available models.
	Discover(ctx context.Context) ([]*registry.ModelInfo, error)

	// ProviderID returns the identifier for this discoverer's provider.
	ProviderID() string
}

// Fetcher is the interface for retrieving raw content from a remote source (URL).
type Fetcher interface {
	// Fetch retrieves the content from the given URL.
	Fetch(ctx context.Context, url string) ([]byte, error)
}

// Parser is the interface for parsing raw content into model definitions.
type Parser interface {
	// Parse extracts model definitions from the given raw content.
	Parse(content []byte) ([]*registry.ModelInfo, error)
}
