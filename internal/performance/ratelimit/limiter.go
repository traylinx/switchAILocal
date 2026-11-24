// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package ratelimit provides token-bucket rate limiting for the switchAILocal proxy.
// It supports a pre-auth global system-wide limiter and post-auth per-principal limiters.
package ratelimit

import (
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
	"github.com/traylinx/switchAILocal/internal/config"
	"github.com/traylinx/switchAILocal/internal/observability"
	"golang.org/x/time/rate"
)

const principalContextKey = "ratelimit_principal"

// SetPrincipal binds an authenticated principal identifier to the request context
// for post-auth per-key rate limiting. Empty principals are ignored.
func SetPrincipal(c *gin.Context, principal string) {
	if c == nil || principal == "" {
		return
	}
	c.Set(principalContextKey, principal)
}

// PrincipalFromContext returns the authenticated principal identifier set on the
// request context, or an empty string if none was set.
func PrincipalFromContext(c *gin.Context) string {
	if c == nil {
		return ""
	}
	return c.GetString(principalContextKey)
}

// Limiter manages global and per-principal rate limiting using token buckets.
// The global limiter runs pre-auth to protect the system from total overload.
// Per-key limiters run post-auth and key buckets only by the authenticated principal.
type Limiter struct {
	cfg config.RateLimiterConfig

	// global is the system-wide rate limiter shared across all clients.
	global *rate.Limiter

	// perKey tracks per-principal rate limiters. We use sync.Map for lock-free reads
	// on the hot path. Limiters are lazily created on first authenticated access.
	perKey sync.Map // map[string]*keyEntry

	stopCh   chan struct{}
	stopOnce sync.Once
}

// keyEntry wraps a rate.Limiter with a last-accessed timestamp for eviction.
type keyEntry struct {
	limiter    *rate.Limiter
	lastAccess atomic.Int64 // unix nano timestamp
}

// New creates a new rate limiter from the given configuration.
// When enabled, starts a background goroutine to evict stale per-key entries every 5 minutes.
// Call Close to stop the background eviction goroutine.
func New(cfg config.RateLimiterConfig) *Limiter {
	l := &Limiter{
		cfg: cfg,
	}
	if cfg.Enabled {
		l.global = rate.NewLimiter(rate.Limit(cfg.GlobalRequestsPerSecond), cfg.GlobalBurst)
		l.stopCh = make(chan struct{})
		log.WithFields(log.Fields{
			"global_rps":    cfg.GlobalRequestsPerSecond,
			"global_burst":  cfg.GlobalBurst,
			"per_key_rps":   cfg.PerKeyRequestsPerSecond,
			"per_key_burst": cfg.PerKeyBurst,
		}).Info("Rate limiter enabled")
		go l.evictStaleKeys()
	}
	return l
}

// Close stops the background eviction goroutine if running. Safe to call multiple times.
func (l *Limiter) Close() {
	if l == nil {
		return
	}
	l.stopOnce.Do(func() {
		if l.stopCh != nil {
			close(l.stopCh)
		}
	})
}

// BucketCount returns the number of currently tracked per-principal buckets.
func (l *Limiter) BucketCount() int {
	if l == nil {
		return 0
	}
	count := 0
	l.perKey.Range(func(_, _ any) bool {
		count++
		return true
	})
	return count
}

// BucketKeys returns a snapshot of the currently tracked per-principal bucket keys.
func (l *Limiter) BucketKeys() []string {
	if l == nil {
		return nil
	}
	var keys []string
	l.perKey.Range(func(key, _ any) bool {
		if k, ok := key.(string); ok {
			keys = append(keys, k)
		}
		return true
	})
	return keys
}

// evictStaleKeys periodically removes per-key limiters that haven't been accessed
// in the last 10 minutes. This prevents unbounded memory growth from unique principals.
func (l *Limiter) evictStaleKeys() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-l.stopCh:
			return
		case <-ticker.C:
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
}

// getOrCreateKeyLimiter returns the rate limiter for a specific principal key,
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

// GlobalMiddleware returns a Gin middleware that enforces the pre-auth global rate limit.
// Rate-limited requests receive HTTP 429 with Retry-After: 1.
func (l *Limiter) GlobalMiddleware() gin.HandlerFunc {
	if l == nil || !l.cfg.Enabled {
		return func(c *gin.Context) { c.Next() }
	}

	return func(c *gin.Context) {
		if !l.global.Allow() {
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
		c.Next()
	}
}

// PerKeyMiddleware returns a Gin middleware that enforces the post-auth per-key rate limit.
// Requests without an authenticated principal on the context are passed through untouched
// without creating any bucket.
func (l *Limiter) PerKeyMiddleware() gin.HandlerFunc {
	if l == nil || !l.cfg.Enabled {
		return func(c *gin.Context) { c.Next() }
	}

	return func(c *gin.Context) {
		principal := PrincipalFromContext(c)
		if principal != "" {
			keyLimiter := l.getOrCreateKeyLimiter(principal)
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
