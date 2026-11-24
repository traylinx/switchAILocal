// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	configaccess "github.com/traylinx/switchAILocal/internal/access/config_access"
	proxyconfig "github.com/traylinx/switchAILocal/internal/config"
	sdkaccess "github.com/traylinx/switchAILocal/sdk/access"
	sdkconfig "github.com/traylinx/switchAILocal/sdk/config"
	"github.com/traylinx/switchAILocal/sdk/switchailocal/auth"
)

const (
	testRateLimitKeyA = "test-ratelimit-key-alpha"
	testRateLimitKeyB = "test-ratelimit-key-beta"
	globalLimitMsg    = "Rate limit exceeded (global). Please retry after the Retry-After period."
	perKeyLimitMsg    = "Rate limit exceeded (per-key). Please retry after the Retry-After period."
)

type apiRateLimitError struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

func newRateLimitTestServer(t *testing.T, rlCfg proxyconfig.RateLimiterConfig, wsAuth bool) *Server {
	t.Helper()

	gin.SetMode(gin.TestMode)
	configaccess.Register()

	tmpDir := t.TempDir()
	authDir := filepath.Join(tmpDir, "auth")
	if err := os.MkdirAll(authDir, 0o700); err != nil {
		t.Fatalf("failed to create auth dir: %v", err)
	}

	cfg := &proxyconfig.Config{
		SDKConfig: sdkconfig.SDKConfig{
			APIKeys: []string{testRateLimitKeyA, testRateLimitKeyB},
		},
		Port:                   0,
		AuthDir:                authDir,
		Debug:                  true,
		LoggingToFile:          false,
		UsageStatisticsEnabled: false,
		WebsocketAuth:          wsAuth,
		Performance: proxyconfig.PerformanceConfig{
			RateLimiter: rlCfg,
		},
	}

	authManager := auth.NewManager(nil, nil, nil)
	accessManager := sdkaccess.NewManager()
	configPath := filepath.Join(tmpDir, "config.yaml")

	srv := NewServer(cfg, authManager, accessManager, configPath, nil)
	t.Cleanup(func() {
		if srv.rateLimiter != nil {
			srv.rateLimiter.Close()
		}
	})
	return srv
}

func rateLimitCounter(t *testing.T, scope string) float64 {
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

func assertAPI429(t *testing.T, rr *httptest.ResponseRecorder, wantMessage string) {
	t.Helper()
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d (body=%s)", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Retry-After"); got != "1" {
		t.Fatalf("expected Retry-After: 1, got %q", got)
	}
	var payload apiRateLimitError
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode 429 JSON %q: %v", rr.Body.String(), err)
	}
	if payload.Error.Message != wantMessage {
		t.Fatalf("error.message = %q, want %q", payload.Error.Message, wantMessage)
	}
	if payload.Error.Type != "rate_limit_error" {
		t.Fatalf("error.type = %q, want %q", payload.Error.Type, "rate_limit_error")
	}
	if payload.Error.Code != "rate_limit_exceeded" {
		t.Fatalf("error.code = %q, want %q", payload.Error.Code, "rate_limit_exceeded")
	}
}

func expectedHashedBucketKey(provider, rawKey string) string {
	sum := sha256.Sum256([]byte(rawKey))
	return provider + ":sha256:" + hex.EncodeToString(sum[:])
}

func assertNoRawKeyInBuckets(t *testing.T, s *Server, forbidden ...string) {
	t.Helper()
	for _, bucketKey := range s.rateLimiter.BucketKeys() {
		for _, raw := range forbidden {
			if bucketKey == raw || strings.Contains(bucketKey, raw) {
				t.Fatalf("bucket key %q must not equal or contain raw secret %q", bucketKey, raw)
			}
		}
	}
}

func TestRateLimit_CrossTransportSingleBucket(t *testing.T) {
	s := newRateLimitTestServer(t, proxyconfig.RateLimiterConfig{
		Enabled:                 true,
		GlobalRequestsPerSecond: 1000,
		GlobalBurst:             1000,
		PerKeyRequestsPerSecond: 0.01,
		PerKeyBurst:             2,
	}, true)

	// Consume the full per-key burst over Authorization: Bearer.
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		req.Header.Set("Authorization", "Bearer "+testRateLimitKeyA)
		rr := httptest.NewRecorder()
		s.engine.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("burst request %d: expected 200, got %d (body=%s)", i, rr.Code, rr.Body.String())
		}
	}

	// Same principal via every supported key transport must hit the same exhausted bucket.
	transports := []struct {
		name  string
		build func() *http.Request
	}{
		{
			name: "authorization_bearer",
			build: func() *http.Request {
				r := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
				r.Header.Set("Authorization", "Bearer "+testRateLimitKeyA)
				return r
			},
		},
		{
			name: "x_api_key",
			build: func() *http.Request {
				r := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
				r.Header.Set("X-Api-Key", testRateLimitKeyA)
				return r
			},
		},
		{
			name: "x_goog_api_key",
			build: func() *http.Request {
				r := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
				r.Header.Set("X-Goog-Api-Key", testRateLimitKeyA)
				return r
			},
		},
		{
			name: "query_key",
			build: func() *http.Request {
				return httptest.NewRequest(http.MethodGet, "/v1/models?key="+testRateLimitKeyA, nil)
			},
		},
		{
			name: "query_auth_token",
			build: func() *http.Request {
				return httptest.NewRequest(http.MethodGet, "/v1/models?auth_token="+testRateLimitKeyA, nil)
			},
		},
	}

	for _, tc := range transports {
		t.Run(tc.name, func(t *testing.T) {
			before := rateLimitCounter(t, "per_key")
			rr := httptest.NewRecorder()
			s.engine.ServeHTTP(rr, tc.build())
			assertAPI429(t, rr, perKeyLimitMsg)

			after := rateLimitCounter(t, "per_key")
			if delta := after - before; delta != 1 {
				t.Fatalf("%s: expected per_key metric delta=1, got %v", tc.name, delta)
			}
		})
	}

	if got := s.rateLimiter.BucketCount(); got != 1 {
		t.Fatalf("expected exactly 1 bucket across all transports, got %d", got)
	}
	assertNoRawKeyInBuckets(t, s, testRateLimitKeyA)

	wantKey := expectedHashedBucketKey(sdkconfig.DefaultAccessProviderName, testRateLimitKeyA)
	keys := s.rateLimiter.BucketKeys()
	if len(keys) != 1 || keys[0] != wantKey {
		t.Fatalf("bucket keys = %v, want [%s]", keys, wantKey)
	}
}

func TestRateLimit_JunkBearerPlusValidXApiKey(t *testing.T) {
	s := newRateLimitTestServer(t, proxyconfig.RateLimiterConfig{
		Enabled:                 true,
		GlobalRequestsPerSecond: 1000,
		GlobalBurst:             1000,
		PerKeyRequestsPerSecond: 0.01,
		PerKeyBurst:             2,
	}, true)

	// Exhaust key A over Bearer.
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		req.Header.Set("Authorization", "Bearer "+testRateLimitKeyA)
		rr := httptest.NewRecorder()
		s.engine.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("burst request %d: expected 200, got %d", i, rr.Code)
		}
	}

	// Rotating junk Bearer headers paired with valid X-Api-Key must still hit key A's bucket.
	for i := 0; i < 4; i++ {
		junk := fmt.Sprintf("junk-bearer-%d", i)
		req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		req.Header.Set("Authorization", "Bearer "+junk)
		req.Header.Set("X-Api-Key", testRateLimitKeyA)

		rr := httptest.NewRecorder()
		s.engine.ServeHTTP(rr, req)
		assertAPI429(t, rr, perKeyLimitMsg)
	}

	if got := s.rateLimiter.BucketCount(); got != 1 {
		t.Fatalf("junk Bearer headers must not create buckets: got %d, want 1", got)
	}
	assertNoRawKeyInBuckets(t, s, testRateLimitKeyA, "junk-bearer-")
}

func TestRateLimit_InvalidAndMissingKeysCreateNoBucket(t *testing.T) {
	s := newRateLimitTestServer(t, proxyconfig.RateLimiterConfig{
		Enabled:                 true,
		GlobalRequestsPerSecond: 1000,
		GlobalBurst:             1000,
		PerKeyRequestsPerSecond: 0.01,
		PerKeyBurst:             1,
	}, true)

	requests := []*http.Request{
		httptest.NewRequest(http.MethodGet, "/v1/models", nil),
		func() *http.Request {
			r := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
			r.Header.Set("Authorization", "Bearer invalid-1")
			return r
		}(),
		func() *http.Request {
			r := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
			r.Header.Set("X-Api-Key", "invalid-2")
			return r
		}(),
		func() *http.Request {
			r := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
			r.Header.Set("X-Goog-Api-Key", "invalid-3")
			return r
		}(),
		httptest.NewRequest(http.MethodGet, "/v1/models?key=invalid-4", nil),
		httptest.NewRequest(http.MethodGet, "/v1/models?auth_token=invalid-5", nil),
	}

	for i, req := range requests {
		rr := httptest.NewRecorder()
		s.engine.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("request %d: expected 401, got %d (body=%s)", i, rr.Code, rr.Body.String())
		}
	}

	if got := s.rateLimiter.BucketCount(); got != 0 {
		t.Fatalf("unauthenticated and invalid keys must create 0 buckets, got %d", got)
	}
}

func TestRateLimit_SecondValidKeyKeepsOwnBudget(t *testing.T) {
	s := newRateLimitTestServer(t, proxyconfig.RateLimiterConfig{
		Enabled:                 true,
		GlobalRequestsPerSecond: 1000,
		GlobalBurst:             1000,
		PerKeyRequestsPerSecond: 0.01,
		PerKeyBurst:             2,
	}, true)

	// Exhaust key A.
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		req.Header.Set("Authorization", "Bearer "+testRateLimitKeyA)
		rr := httptest.NewRecorder()
		s.engine.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("key A request %d: expected 200, got %d", i, rr.Code)
		}
	}
	reqA3 := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	reqA3.Header.Set("X-Api-Key", testRateLimitKeyA)
	rrA3 := httptest.NewRecorder()
	s.engine.ServeHTTP(rrA3, reqA3)
	assertAPI429(t, rrA3, perKeyLimitMsg)

	// Key B still gets its full burst of 2, then 429 on the 3rd.
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		req.Header.Set("X-Api-Key", testRateLimitKeyB)
		rr := httptest.NewRecorder()
		s.engine.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("key B request %d: expected 200, got %d", i, rr.Code)
		}
	}
	reqB3 := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	reqB3.Header.Set("Authorization", "Bearer "+testRateLimitKeyB)
	rrB3 := httptest.NewRecorder()
	s.engine.ServeHTTP(rrB3, reqB3)
	assertAPI429(t, rrB3, perKeyLimitMsg)

	if got := s.rateLimiter.BucketCount(); got != 2 {
		t.Fatalf("expected 2 buckets for two valid keys, got %d", got)
	}
	assertNoRawKeyInBuckets(t, s, testRateLimitKeyA, testRateLimitKeyB)
}

func TestRateLimit_GlobalFirst(t *testing.T) {
	s := newRateLimitTestServer(t, proxyconfig.RateLimiterConfig{
		Enabled:                 true,
		GlobalRequestsPerSecond: 0.01,
		GlobalBurst:             2,
		PerKeyRequestsPerSecond: 100,
		PerKeyBurst:             10,
	}, true)

	// Two unauthenticated calls consume global burst and fail auth (401) without creating buckets.
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		rr := httptest.NewRecorder()
		s.engine.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("unauth request %d: expected 401, got %d", i, rr.Code)
		}
	}

	// Next request (even with a valid key) is rejected pre-auth by the global limiter.
	before := rateLimitCounter(t, "global")
	reqValid := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	reqValid.Header.Set("Authorization", "Bearer "+testRateLimitKeyA)
	rrValid := httptest.NewRecorder()
	s.engine.ServeHTTP(rrValid, reqValid)
	assertAPI429(t, rrValid, globalLimitMsg)

	if delta := rateLimitCounter(t, "global") - before; delta != 1 {
		t.Fatalf("expected global metric delta=1, got %v", delta)
	}
	if got := s.rateLimiter.BucketCount(); got != 0 {
		t.Fatalf("global 429 must happen pre-auth with 0 per-key buckets, got %d", got)
	}
}

func TestRateLimit_RouteGroupParity(t *testing.T) {
	s := newRateLimitTestServer(t, proxyconfig.RateLimiterConfig{
		Enabled:                 true,
		GlobalRequestsPerSecond: 1000,
		GlobalBurst:             1000,
		PerKeyRequestsPerSecond: 0.01,
		PerKeyBurst:             1,
	}, true)

	s.AttachWebsocketRoute("/v1/ws", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	endpoints := []string{
		"/v1/models",
		"/openai/v1/models",
		"/v1beta/models",
		"/api/provider/openai/models",
		"/v1/ws",
	}

	// Invalid key on every authenticated route group returns 401 and creates no bucket.
	for _, path := range endpoints {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("X-Api-Key", "invalid-for-"+path)
		rr := httptest.NewRecorder()
		s.engine.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("%s with invalid key: expected 401, got %d (body=%s)", path, rr.Code, rr.Body.String())
		}
	}
	if got := s.rateLimiter.BucketCount(); got != 0 {
		t.Fatalf("invalid keys across route groups must create 0 buckets, got %d", got)
	}

	// Exhaust key A's burst of 1 on /v1/models.
	reqFirst := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	reqFirst.Header.Set("Authorization", "Bearer "+testRateLimitKeyA)
	rrFirst := httptest.NewRecorder()
	s.engine.ServeHTTP(rrFirst, reqFirst)
	if rrFirst.Code != http.StatusOK {
		t.Fatalf("first /v1/models call: expected 200, got %d", rrFirst.Code)
	}

	// Every other authenticated route group must immediately 429 key A via X-Api-Key.
	for _, path := range endpoints[1:] {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("X-Api-Key", testRateLimitKeyA)
		rr := httptest.NewRecorder()
		s.engine.ServeHTTP(rr, req)
		assertAPI429(t, rr, perKeyLimitMsg)
	}

	if got := s.rateLimiter.BucketCount(); got != 1 {
		t.Fatalf("all authenticated groups must share 1 bucket per principal, got %d", got)
	}
	assertNoRawKeyInBuckets(t, s, testRateLimitKeyA)
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
