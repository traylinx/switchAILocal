// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package ratelimit

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/traylinx/switchAILocal/internal/config"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func rateLimitedCounterValue(t *testing.T, scope string) float64 {
	t.Helper()
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("failed to gather prometheus metrics: %v", err)
	}
	for _, mf := range families {
		if mf.GetName() != "switchailocal_rate_limited_total" {
			continue
		}
		for _, m := range mf.GetMetric() {
			for _, lp := range m.GetLabel() {
				if lp.GetName() == "scope" && lp.GetValue() == scope {
					return m.GetCounter().GetValue()
				}
			}
		}
	}
	return 0
}

type rateLimitErrorPayload struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

func assertRateLimitResponse(t *testing.T, w *httptest.ResponseRecorder, wantMessage string) {
	t.Helper()
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected status 429, got %d (body=%s)", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Retry-After"); got != "1" {
		t.Fatalf("expected Retry-After: 1, got %q", got)
	}
	var payload rateLimitErrorPayload
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode 429 JSON body %q: %v", w.Body.String(), err)
	}
	if payload.Error.Message != wantMessage {
		t.Fatalf("unexpected error.message: got %q, want %q", payload.Error.Message, wantMessage)
	}
	if payload.Error.Type != "rate_limit_error" {
		t.Fatalf("unexpected error.type: got %q, want %q", payload.Error.Type, "rate_limit_error")
	}
	if payload.Error.Code != "rate_limit_exceeded" {
		t.Fatalf("unexpected error.code: got %q, want %q", payload.Error.Code, "rate_limit_exceeded")
	}
}

func TestLimiter_Disabled(t *testing.T) {
	l := New(config.RateLimiterConfig{Enabled: false})
	defer l.Close()

	globalMW := l.GlobalMiddleware()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	SetPrincipal(c, "config-api-key:sha256:disabled")

	globalMW(c)
	if c.IsAborted() {
		t.Fatal("global middleware should not abort when disabled")
	}
	if !CheckPerKey(c) || !l.CheckPerKey(c) || c.IsAborted() {
		t.Fatal("per-key check should not abort when disabled")
	}
	if got := l.BucketCount(); got != 0 {
		t.Fatalf("expected 0 buckets when disabled, got %d", got)
	}
}

func TestLimiter_GlobalLimit(t *testing.T) {
	l := New(config.RateLimiterConfig{
		Enabled:                 true,
		GlobalRequestsPerSecond: 0.01,
		GlobalBurst:             1,
		PerKeyRequestsPerSecond: 100,
		PerKeyBurst:             100,
	})
	defer l.Close()

	globalMW := l.GlobalMiddleware()
	before := rateLimitedCounterValue(t, "global")

	// First request consumes the single global burst token.
	w1 := httptest.NewRecorder()
	c1, _ := gin.CreateTestContext(w1)
	c1.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	globalMW(c1)
	if c1.IsAborted() {
		t.Fatal("first request should not be rate limited")
	}

	// Second immediate request hits the global limit.
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	globalMW(c2)
	if !c2.IsAborted() {
		t.Fatal("second immediate request should be rate limited")
	}
	assertRateLimitResponse(t, w2, "Rate limit exceeded (global). Please retry after the Retry-After period.")

	after := rateLimitedCounterValue(t, "global")
	if delta := after - before; delta != 1 {
		t.Fatalf("expected global metric to increment by 1, got delta=%v", delta)
	}
	if got := l.BucketCount(); got != 0 {
		t.Fatalf("global middleware must not create per-key buckets, got %d", got)
	}
}

func TestLimiter_PerKeyLimitAndIsolation(t *testing.T) {
	l := New(config.RateLimiterConfig{
		Enabled:                 true,
		GlobalRequestsPerSecond: 10000,
		GlobalBurst:             10000,
		PerKeyRequestsPerSecond: 0.01,
		PerKeyBurst:             1,
	})
	defer l.Close()

	globalMW := l.GlobalMiddleware()
	before := rateLimitedCounterValue(t, "per_key")

	principalA := "config-api-key:sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	principalB := "config-api-key:sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

	run := func(principal string) (*httptest.ResponseRecorder, bool) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		globalMW(c)
		SetPrincipal(c, principal)
		allowed := CheckPerKey(c)
		return w, allowed && !c.IsAborted()
	}

	// First request for principal A succeeds.
	if _, ok := run(principalA); !ok {
		t.Fatal("first request for principal A should not be rate limited")
	}

	// Second immediate request for principal A gets 429 with exact body, header, and metric.
	w2, ok := run(principalA)
	if ok {
		t.Fatal("second request for principal A should be rate limited")
	}
	assertRateLimitResponse(t, w2, "Rate limit exceeded (per-key). Please retry after the Retry-After period.")

	after := rateLimitedCounterValue(t, "per_key")
	if delta := after - before; delta != 1 {
		t.Fatalf("expected per_key metric to increment by 1, got delta=%v", delta)
	}

	// Principal B keeps its own budget.
	if _, ok := run(principalB); !ok {
		t.Fatal("first request for principal B should not be rate limited")
	}
	if got := l.BucketCount(); got != 2 {
		t.Fatalf("expected 2 buckets, got %d", got)
	}
	if keys := l.BucketKeys(); len(keys) != 2 {
		t.Fatalf("expected 2 bucket keys, got %d", len(keys))
	}
}

func TestLimiter_MissingPrincipalCreatesNoBucket(t *testing.T) {
	l := New(config.RateLimiterConfig{
		Enabled:                 true,
		GlobalRequestsPerSecond: 10000,
		GlobalBurst:             10000,
		PerKeyRequestsPerSecond: 0.01,
		PerKeyBurst:             1,
	})
	defer l.Close()

	globalMW := l.GlobalMiddleware()

	// Even if raw auth headers are present, CheckPerKey ignores them unless SetPrincipal ran.
	for i := 0; i < 5; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		c.Request.Header.Set("Authorization", "Bearer raw-key-must-be-ignored")
		c.Request.Header.Set("X-Api-Key", "raw-key-must-be-ignored")
		globalMW(c)
		if !CheckPerKey(c) || c.IsAborted() {
			t.Fatalf("request %d without context principal should not be aborted", i)
		}
	}

	if got := l.BucketCount(); got != 0 {
		t.Fatalf("expected 0 buckets when principal is missing, got %d", got)
	}
}

func TestPrincipalContextHelpers(t *testing.T) {
	SetPrincipal(nil, "ignored")
	if got := PrincipalFromContext(nil); got != "" {
		t.Fatalf("expected empty principal for nil context, got %q", got)
	}
	if !CheckPerKey(nil) {
		t.Fatal("CheckPerKey(nil) must return true")
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	SetPrincipal(c, "")
	if got := PrincipalFromContext(c); got != "" {
		t.Fatalf("expected empty principal when setting empty string, got %q", got)
	}
	if !CheckPerKey(c) {
		t.Fatal("CheckPerKey without attached limiter must return true")
	}

	const want = "config-api-key:sha256:1234"
	SetPrincipal(c, want)
	if got := PrincipalFromContext(c); got != want {
		t.Fatalf("PrincipalFromContext = %q, want %q", got, want)
	}
}

func TestLimiter_BucketCapEvictsLRU(t *testing.T) {
	const capSize = 3
	l := newWithMaxBuckets(config.RateLimiterConfig{
		Enabled:                 true,
		GlobalRequestsPerSecond: 10000,
		GlobalBurst:             10000,
		PerKeyRequestsPerSecond: 0.01,
		PerKeyBurst:             1,
	}, capSize)
	defer l.Close()

	// Insert k1, k2, k3 and consume k1's burst so we can verify k1 survives LRU eviction.
	if !l.getOrCreateKeyLimiter("k1").Allow() {
		t.Fatal("k1 first allow failed")
	}
	l.getOrCreateKeyLimiter("k2")
	l.getOrCreateKeyLimiter("k3")

	// Touch k1 so k2 becomes the least recently used bucket.
	if l.getOrCreateKeyLimiter("k1").Allow() {
		t.Fatal("k1 should still be exhausted before k4 insert")
	}

	// Inserting k4 exceeds cap=3 and must evict k2 (LRU), keeping k1, k3, k4.
	l.getOrCreateKeyLimiter("k4")

	if got := l.BucketCount(); got != capSize {
		t.Fatalf("BucketCount = %d, want %d", got, capSize)
	}
	keys := l.BucketKeys()
	slices.Sort(keys)
	want := []string{"k1", "k3", "k4"}
	if !slices.Equal(keys, want) {
		t.Fatalf("remaining buckets = %v, want %v", keys, want)
	}

	// k1 was not evicted, so its exhausted budget must remain enforced.
	if l.getOrCreateKeyLimiter("k1").Allow() {
		t.Fatal("k1 should retain its exhausted bucket after k2 eviction")
	}

	// Concurrent churn under a small cap must stay bounded and keep count in sync.
	var wg sync.WaitGroup
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 40; i++ {
				l.getOrCreateKeyLimiter(fmt.Sprintf("worker-%d-key-%d", id, i))
			}
		}(g)
	}
	wg.Wait()

	if got := l.BucketCount(); got > capSize {
		t.Fatalf("BucketCount after concurrent churn = %d, exceeds cap %d", got, capSize)
	}
	if got, actual := l.BucketCount(), len(l.BucketKeys()); got != actual {
		t.Fatalf("BucketCount (%d) diverged from len(BucketKeys()) (%d)", got, actual)
	}
}

func TestLimiter_EvictStaleKeysCompareAndDelete(t *testing.T) {
	l := New(config.RateLimiterConfig{
		Enabled:                 true,
		GlobalRequestsPerSecond: 10000,
		GlobalBurst:             10000,
		PerKeyRequestsPerSecond: 0.01,
		PerKeyBurst:             1,
	})
	defer l.Close()

	// Create stale, fresh, and concurrently-touched buckets.
	l.getOrCreateKeyLimiter("stale")
	l.getOrCreateKeyLimiter("fresh")
	touchedLimiter := l.getOrCreateKeyLimiter("touched")
	if !touchedLimiter.Allow() {
		t.Fatal("touched bucket first allow failed")
	}

	cutoff := time.Now().Add(-10 * time.Minute).UnixNano()
	staleRaw, _ := l.perKey.Load("stale")
	staleRaw.(*keyEntry).lastAccess.Store(cutoff - int64(time.Minute))

	touchedRaw, _ := l.perKey.Load("touched")
	touchedEntry := touchedRaw.(*keyEntry)
	oldAccess := cutoff - int64(time.Minute)
	touchedEntry.lastAccess.Store(oldAccess)

	// Simulate the race: eviction observed oldAccess, then a request touches the bucket before eviction deletes it.
	if !l.touchEntry(touchedEntry, time.Now().UnixNano()) {
		t.Fatal("touchEntry unexpectedly failed")
	}
	if l.evictEntry("touched", touchedEntry, oldAccess) {
		t.Fatal("evictEntry must not evict an entry touched after its lastAccess was observed")
	}

	// Stale eviction drops "stale", preserves "fresh" and "touched".
	l.evictStaleKeysOnce(cutoff)

	keys := l.BucketKeys()
	slices.Sort(keys)
	want := []string{"fresh", "touched"}
	if !slices.Equal(keys, want) {
		t.Fatalf("remaining buckets after stale eviction = %v, want %v", keys, want)
	}
	if got := l.BucketCount(); got != 2 {
		t.Fatalf("BucketCount = %d, want 2", got)
	}
	// The touched entry kept its original limiter and exhausted budget.
	if l.getOrCreateKeyLimiter("touched").Allow() {
		t.Fatal("touched bucket budget must not be reset")
	}
}

func TestLimiter_LazyEvictionAndCloseStopsGoroutine(t *testing.T) {
	var nilLimiter *Limiter
	nilLimiter.Close()

	// Enabled limiter starts no goroutine until the first per-key bucket is created.
	l := New(config.RateLimiterConfig{
		Enabled:                 true,
		GlobalRequestsPerSecond: 10,
		GlobalBurst:             10,
		PerKeyRequestsPerSecond: 10,
		PerKeyBurst:             10,
	})
	if done := l.evictionDone(); done != nil {
		t.Fatal("eviction goroutine must start lazily on first bucket, not in New")
	}

	// First bucket starts the eviction goroutine; done channel stays open until Close.
	l.getOrCreateKeyLimiter("first-key")
	done := l.evictionDone()
	if done == nil {
		t.Fatal("expected eviction goroutine to start on first bucket")
	}
	select {
	case <-done:
		t.Fatal("eviction goroutine exited before Close")
	default:
	}

	// Close must stop the goroutine and close done; fails if the goroutine keeps running.
	closeReturned := make(chan struct{})
	go func() {
		l.Close()
		close(closeReturned)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("eviction goroutine did not stop after Close")
	}
	select {
	case <-closeReturned:
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not return after eviction goroutine stopped")
	}

	// Idempotent second Close.
	l.Close()

	// Closing before any bucket prevents starting a goroutine later.
	l2 := New(config.RateLimiterConfig{
		Enabled:                 true,
		GlobalRequestsPerSecond: 10,
		GlobalBurst:             10,
		PerKeyRequestsPerSecond: 10,
		PerKeyBurst:             10,
	})
	l2.Close()
	l2.getOrCreateKeyLimiter("after-close")
	if done := l2.evictionDone(); done != nil {
		t.Fatal("eviction goroutine must not start after Close")
	}
}

func TestLimiter_CheckPerKeyZeroRateMeansNoPerKeyLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, cfg := range []config.RateLimiterConfig{
		{Enabled: true, GlobalRequestsPerSecond: 1000, GlobalBurst: 1000, PerKeyRequestsPerSecond: 0, PerKeyBurst: 5},
		{Enabled: true, GlobalRequestsPerSecond: 1000, GlobalBurst: 1000, PerKeyRequestsPerSecond: 5, PerKeyBurst: 0},
	} {
		l := New(cfg)
		for i := 0; i < 20; i++ {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			SetPrincipal(c, "config-api-key:sha256:abc")
			if !l.CheckPerKey(c) {
				t.Fatalf("cfg %+v: request %d refused; a zero per-key rate must mean no per-key limit", cfg, i)
			}
		}
		if got := l.BucketCount(); got != 0 {
			t.Fatalf("cfg %+v: %d buckets created, want 0", cfg, got)
		}
		l.Close()
	}
}
