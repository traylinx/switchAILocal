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

const (
	principalContextKey = "ratelimit_principal"
	limiterContextKey   = "ratelimit_limiter"

	defaultMaxBuckets       = 10000
	evictedAccess     int64 = -1
)

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

// CheckPerKey enforces the post-auth per-key rate limit using the Limiter attached
// to the request context by GlobalMiddleware. It returns true when the request is
// allowed or when no enabled limiter is attached.
func CheckPerKey(c *gin.Context) bool {
	if c == nil {
		return true
	}
	raw, ok := c.Get(limiterContextKey)
	if !ok {
		return true
	}
	l, ok := raw.(*Limiter)
	if !ok || l == nil {
		return true
	}
	return l.CheckPerKey(c)
}

// Limiter manages global and per-principal rate limiting using token buckets.
// The global limiter runs pre-auth to protect the system from total overload.
// Per-key limiters run post-auth and key buckets only by the authenticated principal.
type Limiter struct {
	cfg        config.RateLimiterConfig
	maxBuckets int

	// global is the system-wide rate limiter shared across all clients.
	global *rate.Limiter

	// perKey tracks per-principal rate limiters. We use sync.Map for lock-free reads
	// on the hot path. Limiters are lazily created on first authenticated access.
	perKey      sync.Map // map[string]*keyEntry
	bucketCount atomic.Int64
	lastNow     atomic.Int64
	evictMu     sync.Mutex // serializes cap-enforcement eviction

	evictionStarted atomic.Bool
	lifecycleMu     sync.Mutex
	closed          bool
	evictStart      bool
	stopCh          chan struct{}
	doneCh          chan struct{}
}

// keyEntry wraps a rate.Limiter with a monotonic last-accessed timestamp for eviction.
type keyEntry struct {
	limiter    *rate.Limiter
	lastAccess atomic.Int64 // monotonic unix nano timestamp, or evictedAccess when claimed for eviction
}

// New creates a new rate limiter from the given configuration.
// When enabled, the background stale-key eviction goroutine starts lazily on the
// first per-key bucket insertion. Call Close to stop it if started.
func New(cfg config.RateLimiterConfig) *Limiter {
	return newWithMaxBuckets(cfg, defaultMaxBuckets)
}

func newWithMaxBuckets(cfg config.RateLimiterConfig, maxBuckets int) *Limiter {
	if maxBuckets <= 0 {
		maxBuckets = defaultMaxBuckets
	}
	l := &Limiter{
		cfg:        cfg,
		maxBuckets: maxBuckets,
	}
	if cfg.Enabled {
		l.global = rate.NewLimiter(rate.Limit(cfg.GlobalRequestsPerSecond), cfg.GlobalBurst)
		log.WithFields(log.Fields{
			"global_rps":    cfg.GlobalRequestsPerSecond,
			"global_burst":  cfg.GlobalBurst,
			"per_key_rps":   cfg.PerKeyRequestsPerSecond,
			"per_key_burst": cfg.PerKeyBurst,
		}).Info("Rate limiter enabled")
	}
	return l
}

func (l *Limiter) ensureEvictionStarted() {
	if l.evictionStarted.Load() {
		return
	}
	l.lifecycleMu.Lock()
	defer l.lifecycleMu.Unlock()
	if l.evictStart || l.closed || !l.cfg.Enabled {
		return
	}
	l.evictStart = true
	l.stopCh = make(chan struct{})
	l.doneCh = make(chan struct{})
	l.evictionStarted.Store(true)
	go l.evictStaleKeys(l.stopCh, l.doneCh)
}

func (l *Limiter) evictionDone() <-chan struct{} {
	if l == nil {
		return nil
	}
	l.lifecycleMu.Lock()
	defer l.lifecycleMu.Unlock()
	return l.doneCh
}

// Close stops the background eviction goroutine if running and waits for it to exit.
// Safe to call multiple times or on a nil receiver.
func (l *Limiter) Close() {
	if l == nil {
		return
	}
	l.lifecycleMu.Lock()
	if l.closed {
		done := l.doneCh
		l.lifecycleMu.Unlock()
		if done != nil {
			<-done
		}
		return
	}
	l.closed = true
	l.evictionStarted.Store(true)
	stop := l.stopCh
	done := l.doneCh
	l.lifecycleMu.Unlock()

	if stop != nil {
		close(stop)
	}
	if done != nil {
		<-done
	}
}

// BucketCount returns the number of currently tracked per-principal buckets.
func (l *Limiter) BucketCount() int {
	if l == nil {
		return 0
	}
	return int(l.bucketCount.Load())
}

// BucketKeys returns a snapshot of the currently tracked per-principal bucket keys.
func (l *Limiter) BucketKeys() []string {
	if l == nil {
		return nil
	}
	var keys []string
	l.perKey.Range(func(key, value any) bool {
		entry := value.(*keyEntry)
		if entry.lastAccess.Load() == evictedAccess {
			return true
		}
		if k, ok := key.(string); ok {
			keys = append(keys, k)
		}
		return true
	})
	return keys
}

func (l *Limiter) nowNano() int64 {
	now := time.Now().UnixNano()
	for {
		prev := l.lastNow.Load()
		next := now
		if next <= prev {
			next = prev + 1
		}
		if l.lastNow.CompareAndSwap(prev, next) {
			return next
		}
	}
}

func (l *Limiter) touchEntry(entry *keyEntry, now int64) bool {
	for {
		prev := entry.lastAccess.Load()
		if prev == evictedAccess {
			return false
		}
		if entry.lastAccess.CompareAndSwap(prev, now) {
			return true
		}
	}
}

func (l *Limiter) evictEntry(key any, entry *keyEntry, expectedLast int64) bool {
	if !entry.lastAccess.CompareAndSwap(expectedLast, evictedAccess) {
		return false
	}
	if l.perKey.CompareAndDelete(key, entry) {
		l.bucketCount.Add(-1)
		return true
	}
	return false
}

// evictStaleKeys periodically removes per-key limiters that haven't been accessed
// in the last 10 minutes. This prevents unbounded memory growth from unique principals.
func (l *Limiter) evictStaleKeys(stopCh <-chan struct{}, doneCh chan<- struct{}) {
	defer close(doneCh)
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			l.evictStaleKeysOnce(time.Now().Add(-10 * time.Minute).UnixNano())
		}
	}
}

func (l *Limiter) evictStaleKeysOnce(cutoff int64) {
	l.perKey.Range(func(key, value any) bool {
		entry := value.(*keyEntry)
		last := entry.lastAccess.Load()
		if last == evictedAccess || last >= cutoff {
			return true
		}
		// Atomically claim eviction only if lastAccess is unchanged, then CompareAndDelete.
		l.evictEntry(key, entry, last)
		return true
	})
}

func (l *Limiter) evictToCap(keepKey string) {
	l.evictMu.Lock()
	defer l.evictMu.Unlock()

	for l.bucketCount.Load() > int64(l.maxBuckets) {
		var (
			victimKey   any
			victimEntry *keyEntry
			victimLast  int64
		)
		l.perKey.Range(func(key, value any) bool {
			if k, ok := key.(string); ok && k == keepKey {
				return true
			}
			entry := value.(*keyEntry)
			last := entry.lastAccess.Load()
			if last == evictedAccess {
				return true
			}
			if victimEntry == nil || last < victimLast {
				victimKey = key
				victimEntry = entry
				victimLast = last
			}
			return true
		})
		if victimEntry == nil {
			return
		}
		l.evictEntry(victimKey, victimEntry, victimLast)
	}
}

// getOrCreateKeyLimiter returns the rate limiter for a specific principal key,
// creating one lazily if it doesn't exist. Uses sync.Map for lock-free reads and
// enforces a hard bucket cap by evicting the least recently used entry.
func (l *Limiter) getOrCreateKeyLimiter(key string) *rate.Limiter {
	for {
		now := l.nowNano()
		if existing, ok := l.perKey.Load(key); ok {
			entry := existing.(*keyEntry)
			if l.touchEntry(entry, now) {
				return entry.limiter
			}
			if l.perKey.CompareAndDelete(key, entry) {
				l.bucketCount.Add(-1)
			}
			continue
		}

		entry := &keyEntry{
			limiter: rate.NewLimiter(rate.Limit(l.cfg.PerKeyRequestsPerSecond), l.cfg.PerKeyBurst),
		}
		entry.lastAccess.Store(now)

		actual, loaded := l.perKey.LoadOrStore(key, entry)
		if loaded {
			existing := actual.(*keyEntry)
			if l.touchEntry(existing, now) {
				return existing.limiter
			}
			if l.perKey.CompareAndDelete(key, existing) {
				l.bucketCount.Add(-1)
			}
			continue
		}

		l.ensureEvictionStarted()
		if l.bucketCount.Add(1) > int64(l.maxBuckets) {
			l.evictToCap(key)
		}
		return entry.limiter
	}
}

// GlobalMiddleware returns a Gin middleware that enforces the pre-auth global rate limit
// and binds this Limiter to the request context for post-auth CheckPerKey enforcement.
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
		c.Set(limiterContextKey, l)
		c.Next()
	}
}

// CheckPerKey enforces the post-auth per-key rate limit for the principal stored on
// the Gin context. Requests without an authenticated principal are allowed without
// creating any bucket. Rate-limited requests receive HTTP 429 with Retry-After: 1,
// abort the context, and return false.
func (l *Limiter) CheckPerKey(c *gin.Context) bool {
	if l == nil || !l.cfg.Enabled || c == nil {
		return true
	}
	principal := PrincipalFromContext(c)
	if principal == "" {
		return true
	}
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
		return false
	}
	return true
}
