// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package discovery

import (
	"context"
	"testing"
	"time"
)

func TestDiscoverer_DiscoverAll_Integration(t *testing.T) {
	// Create discoverer with temp cache dir
	tempDir := t.TempDir()
	disc, err := NewDiscoverer(tempDir)
	if err != nil {
		t.Fatalf("Failed to create discoverer: %v", err)
	}

	// Run discovery with 30 second timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	results, err := disc.DiscoverAll(ctx)
	if err != nil {
		t.Fatalf("DiscoverAll failed: %v", err)
	}

	// Verify we got results for expected providers
	t.Logf("Discovery results: %d providers", len(results))
	for provider, models := range results {
		t.Logf("  %s: %d models", provider, len(models))
		if len(models) > 0 {
			t.Logf("    First model: %s", models[0].ID)
		}
	}

	// Should have at least codex, geminicli, vibecli, claudecli
	expected := []string{"codex", "geminicli", "vibecli", "claudecli"}
	for _, prov := range expected {
		if _, ok := results[prov]; !ok {
			t.Errorf("Expected provider %s in results", prov)
		}
	}

	// Verify cache was created for GitHub-fetched providers (not claudecli which uses static)
	for provider := range results {
		if provider == "claudecli" {
			// Claude uses static fallback, not cached via fetcher
			continue
		}
		cached := disc.GetCachedModels(provider)
		if len(cached) == 0 {
			t.Errorf("Expected cached models for %s", provider)
		}
	}
}
