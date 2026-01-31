// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package cache provides semantic caching for routing decisions.
// It uses embedding similarity to cache and retrieve routing decisions,
// reducing latency for similar queries.
package cache

import (
	"container/list"
	"sync"
	"time"
)

// CacheEntry represents a cached routing decision.
type CacheEntry struct {
	// Query is the original query text
	Query string

	// Embedding is the query embedding vector
	Embedding []float32

	// Decision is the routing decision (model ID)
	Decision string

	// Metadata contains additional routing information
	Metadata map[string]interface{}

	// Timestamp is when the entry was created
	Timestamp time.Time

	// element is the LRU list element (for eviction)
	element *list.Element
}

// EmbeddingEngine defines the interface for embedding operations.
type EmbeddingEngine interface {
	Embed(text string) ([]float32, error)
	CosineSimilarity(a, b []float32) float64
	IsEnabled() bool
}

// SemanticCache provides similarity-based caching for routing decisions.
// It uses LRU eviction when the cache reaches maximum size.
type SemanticCache struct {
	// engine is the embedding engine for computing similarities
	engine EmbeddingEngine

	// similarityThreshold is the minimum similarity for a cache hit
	similarityThreshold float64

	// maxSize is the maximum number of entries
	maxSize int

	// entries maps query hash to cache entry
	entries map[string]*CacheEntry

	// lruList maintains LRU order for eviction
	lruList *list.List

	// mu protects concurrent access
	mu sync.RWMutex

	// metrics tracks cache performance
	metrics CacheMetrics
}

// CacheMetrics tracks cache performance statistics.
type CacheMetrics struct {
	Hits          int64
	Misses        int64
	Evictions     int64
	Size          int
	AvgHitLatency time.Duration
	AvgLookup     time.Duration
}

// NewSemanticCache creates a new semantic cache instance.
//
// Parameters:
//   - engine: The embedding engine for computing similarities
//   - similarityThreshold: Minimum similarity for a cache hit (0.0-1.0)
//   - maxSize: Maximum number of cache entries
//
// Returns:
//   - *SemanticCache: A new cache instance
func NewSemanticCache(engine EmbeddingEngine, similarityThreshold float64, maxSize int) *SemanticCache {
	if similarityThreshold <= 0 {
		similarityThreshold = 0.95
	}
	if maxSize <= 0 {
		maxSize = 10000
	}

	return &SemanticCache{
		engine:              engine,
		similarityThreshold: similarityThreshold,
		maxSize:             maxSize,
		entries:             make(map[string]*CacheEntry),
		lruList:             list.New(),
		metrics:             CacheMetrics{},
	}
}

// Lookup searches for a cached routing decision based on semantic similarity.
// Returns the cached decision if a similar query is found, or nil if no match.
//
// Parameters:
//   - query: The query text to look up
//
// Returns:
//   - interface{}: The cached entry if found, or nil
//   - error: Any error during lookup
func (c *SemanticCache) Lookup(query string) (interface{}, error) {
	start := time.Now()
	defer func() {
		c.mu.Lock()
		c.metrics.AvgLookup = time.Since(start)
		c.mu.Unlock()
	}()

	// Compute embedding for query
	queryEmbedding, err := c.engine.Embed(query)
	if err != nil {
		return nil, err
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	// Search for similar cached queries
	var bestMatch *CacheEntry
	var bestSimilarity float64

	for _, entry := range c.entries {
		similarity := c.engine.CosineSimilarity(queryEmbedding, entry.Embedding)
		if similarity >= c.similarityThreshold && similarity > bestSimilarity {
			bestMatch = entry
			bestSimilarity = similarity
		}
	}

	if bestMatch != nil {
		// Cache hit
		c.mu.RUnlock()
		c.mu.Lock()
		c.metrics.Hits++
		c.metrics.AvgHitLatency = time.Since(start)
		
		// Move to front of LRU list
		c.lruList.MoveToFront(bestMatch.element)
		c.mu.Unlock()
		c.mu.RLock()

		return bestMatch, nil
	}

	// Cache miss
	c.mu.RUnlock()
	c.mu.Lock()
	c.metrics.Misses++
	c.mu.Unlock()
	c.mu.RLock()

	return nil, nil
}

// Store adds a routing decision to the cache.
// If the cache is full, the least recently used entry is evicted.
//
// Parameters:
//   - query: The query text
//   - embedding: The query embedding vector
//   - decision: The routing decision (model ID)
//   - metadata: Additional routing information
//
// Returns:
//   - error: Any error during storage
func (c *SemanticCache) Store(query string, embedding []float32, decision string, metadata map[string]interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if we need to evict
	if len(c.entries) >= c.maxSize {
		c.evictLRU()
	}

	// Create new entry
	entry := &CacheEntry{
		Query:     query,
		Embedding: embedding,
		Decision:  decision,
		Metadata:  metadata,
		Timestamp: time.Now(),
	}

	// Add to LRU list
	entry.element = c.lruList.PushFront(entry)

	// Store in map (use query as key for simplicity)
	c.entries[query] = entry
	c.metrics.Size = len(c.entries)

	return nil
}

// evictLRU removes the least recently used entry from the cache.
// Must be called with lock held.
func (c *SemanticCache) evictLRU() {
	if c.lruList.Len() == 0 {
		return
	}

	// Get least recently used entry
	oldest := c.lruList.Back()
	if oldest == nil {
		return
	}

	entry := oldest.Value.(*CacheEntry)

	// Remove from map and list
	delete(c.entries, entry.Query)
	c.lruList.Remove(oldest)

	c.metrics.Evictions++
	c.metrics.Size = len(c.entries)
}

// Clear removes all entries from the cache.
func (c *SemanticCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make(map[string]*CacheEntry)
	c.lruList = list.New()
	c.metrics.Size = 0
}

// GetMetrics returns current cache performance metrics.
//
// Returns:
//   - CacheMetrics: Current metrics snapshot
func (c *SemanticCache) GetMetrics() CacheMetrics {
	c.mu.RLock()
	defer c.mu.RUnlock()

	metrics := c.metrics
	metrics.Size = len(c.entries)
	return metrics
}

// GetMetricsAsMap returns current cache performance metrics as a map.
// This is used by the Lua plugin engine.
//
// Returns:
//   - map[string]interface{}: Current metrics as a map
func (c *SemanticCache) GetMetricsAsMap() map[string]interface{} {
	metrics := c.GetMetrics()
	return map[string]interface{}{
		"hits":             metrics.Hits,
		"misses":           metrics.Misses,
		"evictions":        metrics.Evictions,
		"size":             metrics.Size,
		"avg_hit_latency":  metrics.AvgHitLatency.Milliseconds(),
		"avg_lookup":       metrics.AvgLookup.Milliseconds(),
		"hit_rate":         c.GetHitRate(),
	}
}

// GetHitRate returns the cache hit rate as a percentage.
//
// Returns:
//   - float64: Hit rate (0.0-1.0)
func (c *SemanticCache) GetHitRate() float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	total := c.metrics.Hits + c.metrics.Misses
	if total == 0 {
		return 0.0
	}
	return float64(c.metrics.Hits) / float64(total)
}

// GetSize returns the current number of cached entries.
//
// Returns:
//   - int: Number of entries
func (c *SemanticCache) GetSize() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// IsEnabled returns whether the cache is operational.
//
// Returns:
//   - bool: true if the cache is enabled
func (c *SemanticCache) IsEnabled() bool {
	return c.engine != nil && c.engine.IsEnabled()
}
