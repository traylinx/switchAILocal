// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/traylinx/switchAILocal/internal/config"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestMiddleware_Disabled(t *testing.T) {
	cfg := config.RateLimiterConfig{Enabled: false}
	mw := Middleware(cfg)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)

	mw(c)

	if c.IsAborted() {
		t.Fatal("middleware should not abort when disabled")
	}
}

func TestMiddleware_GlobalLimit(t *testing.T) {
	cfg := config.RateLimiterConfig{
		Enabled:                 true,
		GlobalRequestsPerSecond: 1, // 1 req/s
		GlobalBurst:             1, // burst of 1
		PerKeyRequestsPerSecond: 100,
		PerKeyBurst:             100,
	}

	mw := Middleware(cfg)

	// First request should succeed
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	mw(c)
	if c.IsAborted() {
		t.Fatal("first request should not be rate limited")
	}

	// Second immediate request should be rate limited (burst=1, already consumed)
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	mw(c2)
	if !c2.IsAborted() {
		t.Fatal("second immediate request should be rate limited")
	}
	if w2.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", w2.Code)
	}
	if w2.Header().Get("Retry-After") == "" {
		t.Fatal("expected Retry-After header")
	}
}

func TestMiddleware_PerKeyLimit(t *testing.T) {
	cfg := config.RateLimiterConfig{
		Enabled:                 true,
		GlobalRequestsPerSecond: 10000, // high global
		GlobalBurst:             10000,
		PerKeyRequestsPerSecond: 1, // 1 req/s per key
		PerKeyBurst:             1,
	}

	mw := Middleware(cfg)

	// First request with key should succeed
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	c.Request.Header.Set("Authorization", "Bearer sk-test-key-123")
	mw(c)
	if c.IsAborted() {
		t.Fatal("first request should not be rate limited")
	}

	// Second immediate request with same key should be rate limited
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	c2.Request.Header.Set("Authorization", "Bearer sk-test-key-123")
	mw(c2)
	if !c2.IsAborted() {
		t.Fatal("second request with same key should be rate limited")
	}
	if w2.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", w2.Code)
	}

	// Request with different key should succeed
	w3 := httptest.NewRecorder()
	c3, _ := gin.CreateTestContext(w3)
	c3.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	c3.Request.Header.Set("Authorization", "Bearer sk-different-key")
	mw(c3)
	if c3.IsAborted() {
		t.Fatal("request with different key should not be rate limited")
	}
}

func TestExtractAPIKey(t *testing.T) {
	tests := []struct {
		header string
		want   string
	}{
		{"", ""},
		{"Bearer sk-test-123", "sk-test-123"},
		{"bearer sk-test-123", "sk-test-123"},
		{"sk-raw-key", "sk-raw-key"},
	}

	for _, tt := range tests {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/", nil)
		if tt.header != "" {
			c.Request.Header.Set("Authorization", tt.header)
		}
		got := extractAPIKey(c)
		if got != tt.want {
			t.Errorf("extractAPIKey(%q) = %q, want %q", tt.header, got, tt.want)
		}
	}
}
