// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package loadshed

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/traylinx/switchAILocal/internal/config"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestMiddleware_Disabled(t *testing.T) {
	cfg := config.LoadSheddingConfig{Enabled: false}
	mw := Middleware(cfg)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)

	mw(c)

	if c.IsAborted() {
		t.Fatal("middleware should not abort when disabled")
	}
}

func TestMiddleware_UnderThreshold(t *testing.T) {
	cfg := config.LoadSheddingConfig{
		Enabled:           true,
		MaxInFlight:       10,
		RetryAfterSeconds: 5,
	}

	mw := Middleware(cfg)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)

	mw(c)

	if c.IsAborted() {
		t.Fatal("request under threshold should not be shed")
	}
}

func TestMiddleware_OverThreshold_ZeroMax(t *testing.T) {
	// MaxInFlight=0 means all requests are immediately shed (deterministic test)
	cfg := config.LoadSheddingConfig{
		Enabled:           true,
		MaxInFlight:       0,
		RetryAfterSeconds: 3,
	}
	mw := Middleware(cfg)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)

	mw(c)

	if !c.IsAborted() {
		t.Fatal("request over threshold should be shed")
	}
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
	if w.Header().Get("Retry-After") != "3" {
		t.Fatalf("expected Retry-After=3, got %s", w.Header().Get("Retry-After"))
	}
}

func TestMiddleware_ConcurrentOverThreshold(t *testing.T) {
	// MaxInFlight=2, fire 5 concurrent requests, verify at least some get 503
	cfg := config.LoadSheddingConfig{
		Enabled:           true,
		MaxInFlight:       2,
		RetryAfterSeconds: 5,
	}

	mw := Middleware(cfg)

	// Use a real Gin engine with a blocking handler so requests stay in-flight
	engine := gin.New()
	holdOpen := make(chan struct{})
	var shedCount atomic.Int32
	var passCount atomic.Int32

	engine.Use(mw)
	engine.POST("/test", func(c *gin.Context) {
		passCount.Add(1)
		<-holdOpen // block until released
	})

	const totalRequests = 5
	var wg sync.WaitGroup
	results := make([]*httptest.ResponseRecorder, totalRequests)

	for i := 0; i < totalRequests; i++ {
		wg.Add(1)
		idx := i
		go func() {
			defer wg.Done()
			w := httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/test", nil)
			engine.ServeHTTP(w, req)
			results[idx] = w
			if w.Code == http.StatusServiceUnavailable {
				shedCount.Add(1)
			}
		}()
		// Small stagger to ensure ordering
		time.Sleep(5 * time.Millisecond)
	}

	// Wait a bit for all goroutines to hit the middleware
	time.Sleep(50 * time.Millisecond)

	// Release blocking handlers
	close(holdOpen)
	wg.Wait()

	shed := int(shedCount.Load())
	passed := int(passCount.Load())

	if shed == 0 {
		t.Fatal("expected at least some requests to be shed, got 0")
	}
	if passed > cfg.MaxInFlight {
		t.Fatalf("expected at most %d requests to pass, got %d", cfg.MaxInFlight, passed)
	}
	t.Logf("Results: %d passed, %d shed (out of %d total)", passed, shed, totalRequests)
}

func TestMiddleware_InFlightDecrementsOnCompletion(t *testing.T) {
	// Verify the counter decrements properly — after all requests complete,
	// making one more request should succeed (not be shed)
	cfg := config.LoadSheddingConfig{
		Enabled:           true,
		MaxInFlight:       1,
		RetryAfterSeconds: 5,
	}

	mw := Middleware(cfg)

	// First request should succeed and complete
	w1 := httptest.NewRecorder()
	c1, _ := gin.CreateTestContext(w1)
	c1.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	mw(c1)
	if c1.IsAborted() {
		t.Fatal("first request should not be shed")
	}

	// After first request completes (defer fires), a new request should also succeed
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	mw(c2)
	if c2.IsAborted() {
		t.Fatal("second request should succeed after first completed")
	}
}
