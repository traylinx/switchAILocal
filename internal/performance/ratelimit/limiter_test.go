// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package ratelimit

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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
	perKeyMW := l.PerKeyMiddleware()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	SetPrincipal(c, "config-api-key:sha256:disabled")

	globalMW(c)
	if c.IsAborted() {
		t.Fatal("global middleware should not abort when disabled")
	}
	perKeyMW(c)
	if c.IsAborted() {
		t.Fatal("per-key middleware should not abort when disabled")
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

	perKeyMW := l.PerKeyMiddleware()
	before := rateLimitedCounterValue(t, "per_key")

	principalA := "config-api-key:sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	principalB := "config-api-key:sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

	// First request for principal A succeeds.
	w1 := httptest.NewRecorder()
	c1, _ := gin.CreateTestContext(w1)
	c1.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	SetPrincipal(c1, principalA)
	perKeyMW(c1)
	if c1.IsAborted() {
		t.Fatal("first request for principal A should not be rate limited")
	}

	// Second immediate request for principal A gets 429 with exact body, header, and metric.
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	SetPrincipal(c2, principalA)
	perKeyMW(c2)
	if !c2.IsAborted() {
		t.Fatal("second request for principal A should be rate limited")
	}
	assertRateLimitResponse(t, w2, "Rate limit exceeded (per-key). Please retry after the Retry-After period.")

	after := rateLimitedCounterValue(t, "per_key")
	if delta := after - before; delta != 1 {
		t.Fatalf("expected per_key metric to increment by 1, got delta=%v", delta)
	}

	// Principal B keeps its own budget.
	w3 := httptest.NewRecorder()
	c3, _ := gin.CreateTestContext(w3)
	c3.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	SetPrincipal(c3, principalB)
	perKeyMW(c3)
	if c3.IsAborted() {
		t.Fatal("first request for principal B should not be rate limited")
	}
	if got := l.BucketCount(); got != 2 {
		t.Fatalf("expected 2 buckets, got %d", got)
	}
	keys := l.BucketKeys()
	if len(keys) != 2 {
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

	perKeyMW := l.PerKeyMiddleware()

	// Even if raw auth headers are present, PerKeyMiddleware ignores them unless SetPrincipal ran.
	for i := 0; i < 5; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		c.Request.Header.Set("Authorization", "Bearer raw-key-must-be-ignored")
		c.Request.Header.Set("X-Api-Key", "raw-key-must-be-ignored")
		perKeyMW(c)
		if c.IsAborted() {
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

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	SetPrincipal(c, "")
	if got := PrincipalFromContext(c); got != "" {
		t.Fatalf("expected empty principal when setting empty string, got %q", got)
	}

	const want = "config-api-key:sha256:1234"
	SetPrincipal(c, want)
	if got := PrincipalFromContext(c); got != want {
		t.Fatalf("PrincipalFromContext = %q, want %q", got, want)
	}
}

func TestLimiter_CloseIdempotent(t *testing.T) {
	var nilLimiter *Limiter
	nilLimiter.Close()

	l := New(config.RateLimiterConfig{
		Enabled:                 true,
		GlobalRequestsPerSecond: 10,
		GlobalBurst:             10,
		PerKeyRequestsPerSecond: 10,
		PerKeyBurst:             10,
	})
	l.Close()
	l.Close()
}
