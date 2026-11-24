// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	proxyconfig "github.com/traylinx/switchAILocal/internal/config"
	"github.com/traylinx/switchAILocal/internal/performance/ratelimit"
	sdkaccess "github.com/traylinx/switchAILocal/sdk/access"
	"github.com/traylinx/switchAILocal/sdk/api/handlers"
	sdkconfig "github.com/traylinx/switchAILocal/sdk/config"
)

type closeNotifyRecorder struct {
	*httptest.ResponseRecorder
}

func (closeNotifyRecorder) CloseNotify() <-chan bool {
	return make(chan bool, 1)
}

func TestRateLimit_RouteGroupParity(t *testing.T) {
	ampUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("amp-upstream-ok"))
	}))
	defer ampUpstream.Close()

	s := newRateLimitTestServerWithConfig(t, proxyconfig.RateLimiterConfig{
		Enabled:                 true,
		GlobalRequestsPerSecond: 1000,
		GlobalBurst:             1000,
		PerKeyRequestsPerSecond: 0.01,
		PerKeyBurst:             1,
	}, true, func(cfg *proxyconfig.Config) {
		cfg.AmpCode.UpstreamURL = ampUpstream.URL
		cfg.AmpCode.RestrictManagementToLocalhost = false
	})

	// Also prove any route wired directly with AuthMiddleware automatically enforces per-key limits.
	s.engine.POST("/custom/authed", AuthMiddleware(s.accessManager), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	s.AttachWebsocketRoute("/v1/ws", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	authedEndpoints := []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodGet, path: "/v1/models"},
		{method: http.MethodPost, path: "/v1/chat/completions", body: `{}`},
		{method: http.MethodGet, path: "/openai/v1/models"},
		{method: http.MethodGet, path: "/v1beta/models"},
		{method: http.MethodGet, path: "/api/provider/openai/models"},
		{method: http.MethodGet, path: "/api/threads"},
		{method: http.MethodGet, path: "/threads.rss"},
		{method: http.MethodGet, path: "/v1/ws"},
		{method: http.MethodPost, path: "/custom/authed", body: `{}`},
	}

	// Invalid key on every authenticated route group returns 401 and creates no bucket.
	for _, ep := range authedEndpoints {
		req := httptest.NewRequest(ep.method, ep.path, strings.NewReader(ep.body))
		req.Header.Set("X-Api-Key", "invalid-for-"+ep.path)
		rr := httptest.NewRecorder()
		s.engine.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s with invalid key: expected 401, got %d (body=%s)", ep.method, ep.path, rr.Code, rr.Body.String())
		}
	}
	if got := s.rateLimiter.BucketCount(); got != 0 {
		t.Fatalf("invalid keys across route groups must create 0 buckets, got %d", got)
	}

	// Amp root /threads is bypassed by wrapManagementAuth: stays 200 and creates 0 buckets.
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/threads", nil)
		req.Header.Set("X-Api-Key", testRateLimitKeyA)
		rr := closeNotifyRecorder{httptest.NewRecorder()}
		s.engine.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("GET /threads bypass call %d: expected 200, got %d (body=%s)", i, rr.Code, rr.Body.String())
		}
	}
	if got := s.rateLimiter.BucketCount(); got != 0 {
		t.Fatalf("bypassed /threads route must create 0 buckets, got %d", got)
	}

	// Exhaust key A's burst of 1 on /v1/models.
	reqFirst := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	reqFirst.Header.Set("Authorization", "Bearer "+testRateLimitKeyA)
	rrFirst := httptest.NewRecorder()
	s.engine.ServeHTTP(rrFirst, reqFirst)
	if rrFirst.Code != http.StatusOK {
		t.Fatalf("first /v1/models call: expected 200, got %d", rrFirst.Code)
	}

	// Every other authenticated route group (including POST, amp /api/threads, amp root /threads.rss, ws, custom)
	// must immediately 429 key A via X-Api-Key.
	for _, ep := range authedEndpoints[1:] {
		req := httptest.NewRequest(ep.method, ep.path, strings.NewReader(ep.body))
		req.Header.Set("X-Api-Key", testRateLimitKeyA)
		rr := closeNotifyRecorder{httptest.NewRecorder()}
		s.engine.ServeHTTP(rr, req)
		assertAPI429(t, rr.ResponseRecorder, perKeyLimitMsg)
	}

	// Amp root /threads bypass still succeeds even with key A exhausted.
	reqThreads := httptest.NewRequest(http.MethodGet, "/threads", nil)
	reqThreads.Header.Set("X-Api-Key", testRateLimitKeyA)
	rrThreads := closeNotifyRecorder{httptest.NewRecorder()}
	s.engine.ServeHTTP(rrThreads, reqThreads)
	if rrThreads.Code != http.StatusOK {
		t.Fatalf("GET /threads with exhausted key A: expected 200 (auth bypassed), got %d", rrThreads.Code)
	}

	if got := s.rateLimiter.BucketCount(); got != 1 {
		t.Fatalf("all authenticated groups must share 1 bucket per principal, got %d", got)
	}
	assertNoRawKeyInBuckets(t, s, testRateLimitKeyA)

	// Prove a POST route wired via WithRouterConfigurator consumes and enforces key B's bucket too.
	reqB1 := httptest.NewRequest(http.MethodPost, "/custom/authed", strings.NewReader(`{}`))
	reqB1.Header.Set("Authorization", "Bearer "+testRateLimitKeyB)
	rrB1 := httptest.NewRecorder()
	s.engine.ServeHTTP(rrB1, reqB1)
	if rrB1.Code != http.StatusOK {
		t.Fatalf("first POST /custom/authed for key B: expected 200, got %d", rrB1.Code)
	}
	reqB2 := httptest.NewRequest(http.MethodPost, "/custom/authed", strings.NewReader(`{}`))
	reqB2.Header.Set("X-Api-Key", testRateLimitKeyB)
	rrB2 := httptest.NewRecorder()
	s.engine.ServeHTTP(rrB2, reqB2)
	assertAPI429(t, rrB2, perKeyLimitMsg)
}

func TestRateLimit_RouterConfiguratorAuthMiddlewareParity(t *testing.T) {
	s := newRateLimitTestServerWithConfig(t, proxyconfig.RateLimiterConfig{
		Enabled:                 true,
		GlobalRequestsPerSecond: 1000,
		GlobalBurst:             1000,
		PerKeyRequestsPerSecond: 0.01,
		PerKeyBurst:             1,
	}, false, nil, WithRouterConfigurator(func(engine *gin.Engine, _ *handlers.BaseAPIHandler, _ *proxyconfig.Config) {
		// Intentionally uses only AuthMiddleware; per-key enforcement must be automatic.
		engine.POST("/ext/echo", AuthMiddleware(nil), func(c *gin.Context) {
			c.Status(http.StatusOK)
		})
	}))

	// Route with AuthMiddleware(nil) stays open/unkeyed (0 buckets, all 200).
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/ext/echo", nil)
		rr := httptest.NewRecorder()
		s.engine.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("AuthMiddleware(nil) call %d: expected 200, got %d", i, rr.Code)
		}
	}
	if got := s.rateLimiter.BucketCount(); got != 0 {
		t.Fatalf("AuthMiddleware(nil) must create 0 buckets, got %d", got)
	}
}

func TestRateLimit_WebsocketAuthDisabledNoBucket(t *testing.T) {
	s := newRateLimitTestServer(t, proxyconfig.RateLimiterConfig{
		Enabled:                 true,
		GlobalRequestsPerSecond: 1000,
		GlobalBurst:             1000,
		PerKeyRequestsPerSecond: 0.01,
		PerKeyBurst:             1,
	}, false)

	s.AttachWebsocketRoute("/v1/ws", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/v1/ws", nil)
		req.Header.Set("Authorization", "Bearer "+testRateLimitKeyA)
		rr := httptest.NewRecorder()
		s.engine.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("ws request %d with wsAuth=false: expected 200, got %d", i, rr.Code)
		}
	}

	if got := s.rateLimiter.BucketCount(); got != 0 {
		t.Fatalf("unauthenticated ws route must create 0 per-key buckets, got %d", got)
	}
}

func TestRateLimit_OpenModeNoPerKeyLimit(t *testing.T) {
	modes := []struct {
		name   string
		mutate func(cfg *proxyconfig.Config)
	}{
		{
			name: "keys_unset",
			mutate: func(cfg *proxyconfig.Config) {
				cfg.APIKeys = nil
				cfg.Access.Providers = nil
			},
		},
		{
			name: "empty_key_entry",
			mutate: func(cfg *proxyconfig.Config) {
				cfg.APIKeys = []string{""}
				cfg.Access.Providers = nil
			},
		},
		{
			name: "keyless_config_provider",
			mutate: func(cfg *proxyconfig.Config) {
				cfg.APIKeys = nil
				cfg.Access.Providers = []sdkconfig.AccessProvider{
					{
						Name: "open-config",
						Type: sdkconfig.AccessProviderTypeConfigAPIKey,
					},
				}
			},
		},
	}

	for _, tc := range modes {
		t.Run(tc.name, func(t *testing.T) {
			const perKeyBurst = 2
			s := newRateLimitTestServerWithConfig(t, proxyconfig.RateLimiterConfig{
				Enabled:                 true,
				GlobalRequestsPerSecond: 1000,
				GlobalBurst:             1000,
				PerKeyRequestsPerSecond: 0.01,
				PerKeyBurst:             perKeyBurst,
			}, false, tc.mutate)

			// More keyless requests than PerKeyBurst must all succeed (200) with 0 buckets.
			for i := 0; i < perKeyBurst+4; i++ {
				req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
				rr := httptest.NewRecorder()
				s.engine.ServeHTTP(rr, req)
				if rr.Code != http.StatusOK {
					t.Fatalf("keyless request %d: expected 200, got %d (body=%s)", i, rr.Code, rr.Body.String())
				}
			}
			if got := s.rateLimiter.BucketCount(); got != 0 {
				t.Fatalf("keyless open mode must create 0 buckets, got %d", got)
			}

			// Rotating ?key=<random> and Bearer <random> in open mode must also create 0 buckets and stay 200.
			for i := 0; i < perKeyBurst+4; i++ {
				reqQuery := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/v1/models?key=random-%d", i), nil)
				rrQuery := httptest.NewRecorder()
				s.engine.ServeHTTP(rrQuery, reqQuery)
				if rrQuery.Code != http.StatusOK {
					t.Fatalf("open mode ?key=random-%d: expected 200, got %d (body=%s)", i, rrQuery.Code, rrQuery.Body.String())
				}

				reqHeader := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
				reqHeader.Header.Set("Authorization", fmt.Sprintf("Bearer random-bearer-%d", i))
				rrHeader := httptest.NewRecorder()
				s.engine.ServeHTTP(rrHeader, reqHeader)
				if rrHeader.Code != http.StatusOK {
					t.Fatalf("open mode Bearer random-%d: expected 200, got %d (body=%s)", i, rrHeader.Code, rrHeader.Body.String())
				}
			}
			if got := s.rateLimiter.BucketCount(); got != 0 {
				t.Fatalf("open mode rotating caller keys must create 0 buckets, got %d", got)
			}
		})
	}
}

type sourceNoneProvider struct{}

func (sourceNoneProvider) Identifier() string { return "custom-open" }

func (sourceNoneProvider) Authenticate(_ context.Context, _ *http.Request) (*sdkaccess.Result, error) {
	return &sdkaccess.Result{
		Provider:  "custom-open",
		Principal: "anonymous",
		Metadata:  map[string]string{"source": "none"},
	}, nil
}

func TestRateLimit_SourceNoneMetadataCreatesNoBucket(t *testing.T) {
	gin.SetMode(gin.TestMode)
	lim := ratelimit.New(proxyconfig.RateLimiterConfig{
		Enabled:                 true,
		GlobalRequestsPerSecond: 1000,
		GlobalBurst:             1000,
		PerKeyRequestsPerSecond: 0.01,
		PerKeyBurst:             1,
	})
	defer lim.Close()

	mgr := sdkaccess.NewManager()
	mgr.SetProviders([]sdkaccess.Provider{sourceNoneProvider{}})

	engine := gin.New()
	engine.Use(lim.GlobalMiddleware())
	engine.GET("/test", AuthMiddleware(mgr), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	for i := 0; i < 4; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rr := httptest.NewRecorder()
		engine.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("source=none request %d: expected 200, got %d", i, rr.Code)
		}
	}
	if got := lim.BucketCount(); got != 0 {
		t.Fatalf("metadata source=none must create 0 buckets, got %d", got)
	}
}
