// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package discovery

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/traylinx/switchAILocal/internal/registry"
)

// CacheEntry represents a cached discovery result.
type CacheEntry struct {
	ProviderID string                `json:"provider_id"`
	FetchedAt  time.Time             `json:"fetched_at"`
	TTLSeconds int                   `json:"ttl_seconds"`
	Models     []*registry.ModelInfo `json:"models"`
	SourceURL  string                `json:"source_url,omitempty"`
	SourceType string                `json:"source_type"` // "github" or "api"
}

// Cache provides file-based caching for discovered models.
type Cache struct {
	dir string
	mu  sync.RWMutex
	mem map[string]*CacheEntry // in-memory cache
}

// NewCache creates a new cache with the given directory.
// If the directory does not exist, it will be created.
func NewCache(dir string) (*Cache, error) {
	// Expand ~ to home directory
	if len(dir) > 0 && dir[0] == '~' {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		dir = filepath.Join(home, dir[1:])
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	return &Cache{
		dir: dir,
		mem: make(map[string]*CacheEntry),
	}, nil
}

// Get retrieves a cached entry for the given provider.
// Returns nil if not found or expired.
func (c *Cache) Get(providerID string) *CacheEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Check in-memory cache first
	if entry, ok := c.mem[providerID]; ok {
		if !c.isExpired(entry) {
			return entry
		}
	}

	// Try loading from disk
	entry, err := c.loadFromDisk(providerID)
	if err != nil || entry == nil {
		return nil
	}

	if c.isExpired(entry) {
		return nil
	}

	return entry
}

// GetWithGrace retrieves a cached entry, allowing grace period for stale data.
// graceDays specifies how many days old the cache can be before it's rejected.
func (c *Cache) GetWithGrace(providerID string, graceDays int) *CacheEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Check in-memory cache first
	if entry, ok := c.mem[providerID]; ok {
		if c.isWithinGrace(entry, graceDays) {
			return entry
		}
	}

	// Try loading from disk
	entry, err := c.loadFromDisk(providerID)
	if err != nil || entry == nil {
		return nil
	}

	if c.isWithinGrace(entry, graceDays) {
		return entry
	}

	return nil
}

// Set stores a cache entry for the given provider.
func (c *Cache) Set(entry *CacheEntry) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Store in memory
	c.mem[entry.ProviderID] = entry

	// Persist to disk
	return c.saveToDisk(entry)
}

// Clear removes a specific provider from the cache.
func (c *Cache) Clear(providerID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.mem, providerID)

	path := c.filePath(providerID)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove cache file: %w", err)
	}

	return nil
}

// ClearAll removes all cache entries.
func (c *Cache) ClearAll() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.mem = make(map[string]*CacheEntry)

	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return fmt.Errorf("failed to read cache directory: %w", err)
	}

	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".json" {
			path := filepath.Join(c.dir, entry.Name())
			if err := os.Remove(path); err != nil {
				return fmt.Errorf("failed to remove cache file %s: %w", entry.Name(), err)
			}
		}
	}

	return nil
}

// isExpired checks if the entry has exceeded its TTL.
func (c *Cache) isExpired(entry *CacheEntry) bool {
	expiresAt := entry.FetchedAt.Add(time.Duration(entry.TTLSeconds) * time.Second)
	return time.Now().After(expiresAt)
}

// isWithinGrace checks if the entry is within the grace period.
func (c *Cache) isWithinGrace(entry *CacheEntry, graceDays int) bool {
	graceEnd := entry.FetchedAt.Add(time.Duration(graceDays) * 24 * time.Hour)
	return time.Now().Before(graceEnd)
}

// filePath returns the file path for a provider's cache file.
func (c *Cache) filePath(providerID string) string {
	return filepath.Join(c.dir, providerID+".json")
}

// loadFromDisk loads a cache entry from disk.
func (c *Cache) loadFromDisk(providerID string) (*CacheEntry, error) {
	path := c.filePath(providerID)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read cache file: %w", err)
	}

	var entry CacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, fmt.Errorf("failed to parse cache file: %w", err)
	}

	return &entry, nil
}

// saveToDisk persists a cache entry to disk.
func (c *Cache) saveToDisk(entry *CacheEntry) error {
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cache entry: %w", err)
	}

	path := c.filePath(entry.ProviderID)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}

	return nil
}
