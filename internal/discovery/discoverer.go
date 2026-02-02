// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package discovery

import (
	"context"
	"fmt"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/traylinx/switchAILocal/internal/constant"
	"github.com/traylinx/switchAILocal/internal/discovery/fetcher"
	"github.com/traylinx/switchAILocal/internal/discovery/parsers"
	"github.com/traylinx/switchAILocal/internal/registry"
)

// SourceConfig defines a discovery source configuration.
type SourceConfig struct {
	ProviderID string
	URL        string
	SourceType string // "github" or "api"
	TTLSeconds int
	Parser     Parser
	AuthHeader string // Optional Authorization header
}

// DefaultGitHubTTL is 24 hours for GitHub sources.
const DefaultGitHubTTL = 24 * 60 * 60

// DefaultAPITTL is 1 hour for API sources.
const DefaultAPITTL = 60 * 60

// GracePeriodDays is how long stale cache is considered valid.
const GracePeriodDays = 7

// GitHubSources contains the verified GitHub URLs for CLI model discovery.
var GitHubSources = []SourceConfig{
	{
		ProviderID: constant.Codex,
		URL:        "https://raw.githubusercontent.com/openai/codex/main/codex-rs/core/src/models_manager/model_presets.rs",
		SourceType: "github",
		TTLSeconds: DefaultGitHubTTL,
	},
	{
		ProviderID: constant.GeminiCLI,
		URL:        "https://raw.githubusercontent.com/google-gemini/gemini-cli/main/packages/core/src/config/models.ts",
		SourceType: "github",
		TTLSeconds: DefaultGitHubTTL,
	},
	{
		ProviderID: constant.VibeCLI,
		URL:        "https://raw.githubusercontent.com/mistralai/mistral-vibe/main/vibe/core/config.py",
		SourceType: "github",
		TTLSeconds: DefaultGitHubTTL,
	},
}

// Discoverer orchestrates model discovery from multiple sources.
type Discoverer struct {
	cache   *Cache
	fetcher Fetcher
	sources []SourceConfig
	mu      sync.RWMutex
}

// NewDiscoverer creates a new Discoverer with the given cache directory.
func NewDiscoverer(cacheDir string) (*Discoverer, error) {
	cache, err := NewCache(cacheDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create cache: %w", err)
	}

	// Initialize sources with their parsers
	sources := make([]SourceConfig, len(GitHubSources))
	for i, src := range GitHubSources {
		sources[i] = src
		switch src.ProviderID {
		case "codex":
			sources[i].Parser = parsers.NewCodexParser()
		case constant.GeminiCLI:
			sources[i].Parser = parsers.NewGeminiParser()
		case constant.VibeCLI:
			sources[i].Parser = parsers.NewVibeParser()
		}
	}

	return &Discoverer{
		cache:   cache,
		fetcher: fetcher.NewHTTPFetcher(),
		sources: sources,
	}, nil
}

// AddSource adds a new discovery source dynamically.
func (d *Discoverer) AddSource(src SourceConfig) {
	d.mu.Lock()
	defer d.mu.Unlock()
	log.WithField("provider", src.ProviderID).WithField("url", src.URL).Debug("Adding dynamic discovery source")
	d.sources = append(d.sources, src)
}

// DiscoverAll runs discovery for all configured sources.
// Returns a map of provider ID to discovered models.
func (d *Discoverer) DiscoverAll(ctx context.Context) (map[string][]*registry.ModelInfo, error) {
	d.mu.RLock()
	sources := make([]SourceConfig, len(d.sources))
	copy(sources, d.sources)
	d.mu.RUnlock()

	log.WithField("count", len(sources)).Debug("Starting discovery for all sources")

	results := make(map[string][]*registry.ModelInfo)
	var wg sync.WaitGroup
	var resultsMu sync.Mutex

	for _, src := range sources {
		wg.Add(1)
		go func(src SourceConfig) {
			defer wg.Done()

			log.WithField("provider", src.ProviderID).Debug("Running discovery for source")
			models, err := d.discoverSource(ctx, src)
			if err != nil {
				log.WithError(err).WithField("provider", src.ProviderID).Warn("Discovery failed for provider")
				// Try to use cached data
				if cached := d.cache.GetWithGrace(src.ProviderID, GracePeriodDays); cached != nil {
					log.WithField("provider", src.ProviderID).Info("Using cached models due to discovery failure")
					resultsMu.Lock()
					results[src.ProviderID] = cached.Models
					resultsMu.Unlock()
				}
				return
			}
			resultsMu.Lock()
			results[src.ProviderID] = models
			resultsMu.Unlock()
		}(src)
	}

	wg.Wait()

	// Add Claude models using static fallback
	claudeParser := parsers.NewClaudeParser()
	results[constant.ClaudeCLI] = claudeParser.StaticModels()

	return results, nil
}

// discoverSource discovers models from a single source.
func (d *Discoverer) discoverSource(ctx context.Context, src SourceConfig) ([]*registry.ModelInfo, error) {
	// Check cache first
	if cached := d.cache.Get(src.ProviderID); cached != nil {
		log.WithField("provider", src.ProviderID).Debug("Using cached discovery results")
		return cached.Models, nil
	}

	// Fetch fresh data
	log.WithField("provider", src.ProviderID).WithField("url", src.URL).Info("Fetching models from source")

	var content []byte
	var err error

	// Check if fetcher supports authenticated fetch
	if authFetcher, ok := d.fetcher.(interface {
		FetchWithAuth(ctx context.Context, url string, authHeader string) ([]byte, error)
	}); ok {
		content, err = authFetcher.FetchWithAuth(ctx, src.URL, src.AuthHeader)
	} else {
		content, err = d.fetcher.Fetch(ctx, src.URL)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to fetch from %s: %w", src.URL, err)
	}

	// Parse content
	if src.Parser == nil {
		return nil, fmt.Errorf("no parser configured for provider %s", src.ProviderID)
	}

	models, err := src.Parser.Parse(content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse content for %s: %w", src.ProviderID, err)
	}

	if len(models) == 0 {
		log.WithField("provider", src.ProviderID).Warn("No models parsed from source")
	} else {
		log.WithField("provider", src.ProviderID).WithField("count", len(models)).Info("Successfully discovered models")
	}

	// Cache the results
	entry := &CacheEntry{
		ProviderID: src.ProviderID,
		FetchedAt:  time.Now(),
		TTLSeconds: src.TTLSeconds,
		Models:     models,
		SourceURL:  src.URL,
		SourceType: src.SourceType,
	}
	if err := d.cache.Set(entry); err != nil {
		log.WithError(err).WithField("provider", src.ProviderID).Warn("Failed to cache discovery results")
	}

	return models, nil
}

// Refresh forces a refresh for a specific provider or all providers.
// If providerID is empty, refreshes all.
func (d *Discoverer) Refresh(ctx context.Context, providerID string) error {
	if providerID == "" {
		// Clear all cache and re-discover
		if err := d.cache.ClearAll(); err != nil {
			return fmt.Errorf("failed to clear cache: %w", err)
		}
		_, err := d.DiscoverAll(ctx)
		return err
	}

	// Clear specific provider and re-discover
	if err := d.cache.Clear(providerID); err != nil {
		return fmt.Errorf("failed to clear cache for %s: %w", providerID, err)
	}

	for _, src := range d.sources {
		if src.ProviderID == providerID {
			_, err := d.discoverSource(ctx, src)
			return err
		}
	}

	return fmt.Errorf("unknown provider: %s", providerID)
}

// GetCachedModels returns cached models for a provider if available.
func (d *Discoverer) GetCachedModels(providerID string) []*registry.ModelInfo {
	if cached := d.cache.GetWithGrace(providerID, GracePeriodDays); cached != nil {
		return cached.Models
	}
	return nil
}
