// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package ratelimit provides token-bucket rate limiting for the switchAILocal proxy.
// It supports both a global system-wide limiter and per-API-key limiters.
package ratelimit

import (
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
	"github.com/traylinx/switchAILocal/internal/config"
	"github.com/traylinx/switchAILocal/internal/observability"
	"golang.org/x/time/rate"
)

// Limiter manages global and per-key rate limiting using token bucket algorithm.
// The global limiter protects the system from total overload.
// Per-key limiters prevent any single client from monopolizing resources.
type Limiter struct {
	cfg config.RateLimiterConfig

	// global is the system-wide rate limiter shared across all clients.
	global *rate.Limiter

	// perKey tracks per-API-key rate limiters. We use sync.Map for lock-free reads
	// on the hot path. Limiters are lazily created on first access.
	perKey sync.Map // map[string]*keyEntry
}

// keyEntry wraps a rate.Limiter with a last-accessed timestamp for eviction.
type keyEntry struct {
	limiter    *rate.Limiter
	lastAccess atomic.Int64 // unix nano timestamp
}

// New creates a new rate limiter from the given configuration.
// Starts a background goroutine to evict stale per-key entries every 5 minutes.
func New(cfg config.RateLimiterConfig) *Limiter {
	l := &Limiter{
		cfg:    cfg,
		global: rate.NewLimiter(rate.Limit(cfg.GlobalRequestsPerSecond), cfg.GlobalBurst),
	}
	go l.evictStaleKeys()
	return l
}

// evictStaleKeys periodically removes per-key limiters that haven't been accessed
// in the last 10 minutes. This prevents unbounded memory growth from unique API keys.
func (l *Limiter) evictStaleKeys() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		cutoff := time.Now().Add(-10 * time.Minute).UnixNano()
		l.perKey.Range(func(key, value any) bool {
			entry := value.(*keyEntry)
			if entry.lastAccess.Load() < cutoff {
				l.perKey.Delete(key)
			}
			return true
		})
	}
}

// getOrCreateKeyLimiter returns the rate limiter for a specific API key,
// creating one lazily if it doesn't exist. Uses sync.Map for lock-free reads.
func (l *Limiter) getOrCreateKeyLimiter(key string) *rate.Limiter {
	now := time.Now().UnixNano()
	if existing, ok := l.perKey.Load(key); ok {
		entry := existing.(*keyEntry)
		entry.lastAccess.Store(now)
		return entry.limiter
	}
	entry := &keyEntry{
		limiter: rate.NewLimiter(rate.Limit(l.cfg.PerKeyRequestsPerSecond), l.cfg.PerKeyBurst),
	}
	entry.lastAccess.Store(now)
	actual, _ := l.perKey.LoadOrStore(key, entry)
	return actual.(*keyEntry).limiter
}

// extractAPIKey extracts the API key from the Authorization header.
// Supports "Bearer <key>" format.
func extractAPIKey(c *gin.Context) string {
	auth := c.GetHeader("Authorization")
	if auth == "" {
		return ""
	}
	const prefix = "Bearer "
	if len(auth) > len(prefix) && strings.EqualFold(auth[:len(prefix)], prefix) {
		return auth[len(prefix):]
	}
	return auth
}

// Middleware returns a Gin middleware that enforces rate limits.
// It checks the global limit first (cheapest), then per-key limit.
// Rate-limited requests receive HTTP 429 with a Retry-After header.
func Middleware(cfg config.RateLimiterConfig) gin.HandlerFunc {
	if !cfg.Enabled {
		return func(c *gin.Context) { c.Next() }
	}

	limiter := New(cfg)
	log.WithFields(log.Fields{
		"global_rps":    cfg.GlobalRequestsPerSecond,
		"global_burst":  cfg.GlobalBurst,
		"per_key_rps":   cfg.PerKeyRequestsPerSecond,
		"per_key_burst": cfg.PerKeyBurst,
	}).Info("Rate limiter enabled")

	return func(c *gin.Context) {
		// Check global limit first (shared across all keys)
		if !limiter.global.Allow() {
			observability.RateLimitedTotal.WithLabelValues("global").Inc()
			c.Header("Retry-After", "1")
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": gin.H{
					"message": "Rate limit exceeded (global). Please retry after the Retry-After period.",
					"type":    "rate_limit_error",
					"code":    "rate_limit_exceeded",
				},
			})
			return
		}

		// Check per-key limit
		key := extractAPIKey(c)
		if key != "" {
			keyLimiter := limiter.getOrCreateKeyLimiter(key)
			if !keyLimiter.Allow() {
				observability.RateLimitedTotal.WithLabelValues("per_key").Inc()
				c.Header("Retry-After", "1")
				c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
					"error": gin.H{
						"message": "Rate limit exceeded (per-key). Please retry after the Retry-After period.",
						"type":    "rate_limit_error",
						"code":    "rate_limit_exceeded",
					},
				})
				return
			}
		}

		c.Next()
	}
}
