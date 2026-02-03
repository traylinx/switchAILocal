// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gin "github.com/gin-gonic/gin"
	proxyconfig "github.com/traylinx/switchAILocal/internal/config"
	sdkaccess "github.com/traylinx/switchAILocal/sdk/access"
	sdkconfig "github.com/traylinx/switchAILocal/sdk/config"
	"github.com/traylinx/switchAILocal/sdk/switchailocal/auth"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()

	gin.SetMode(gin.TestMode)

	tmpDir := t.TempDir()
	authDir := filepath.Join(tmpDir, "auth")
	if err := os.MkdirAll(authDir, 0o700); err != nil {
		t.Fatalf("failed to create auth dir: %v", err)
	}

	cfg := &proxyconfig.Config{
		SDKConfig: sdkconfig.SDKConfig{
			APIKeys: []string{"test-key"},
		},
		Port:                   0,
		AuthDir:                authDir,
		Debug:                  true,
		LoggingToFile:          false,
		UsageStatisticsEnabled: false,
	}

	authManager := auth.NewManager(nil, nil, nil)
	accessManager := sdkaccess.NewManager()

	configPath := filepath.Join(tmpDir, "config.yaml")
	return NewServer(cfg, authManager, accessManager, configPath, nil)
}

func TestAmpProviderModelRoutes(t *testing.T) {
	testCases := []struct {
		name         string
		path         string
		wantStatus   int
		wantContains string
	}{
		{
			name:         "openai root models",
			path:         "/api/provider/openai/models",
			wantStatus:   http.StatusOK,
			wantContains: `"object":"list"`,
		},
		{
			name:         "groq root models",
			path:         "/api/provider/groq/models",
			wantStatus:   http.StatusOK,
			wantContains: `"object":"list"`,
		},
		{
			name:         "openai models",
			path:         "/api/provider/openai/v1/models",
			wantStatus:   http.StatusOK,
			wantContains: `"object":"list"`,
		},
		{
			name:         "anthropic models",
			path:         "/api/provider/anthropic/v1/models",
			wantStatus:   http.StatusOK,
			wantContains: `"data"`,
		},
		{
			name:         "google models v1",
			path:         "/api/provider/google/v1/models",
			wantStatus:   http.StatusOK,
			wantContains: `"models"`,
		},
		{
			name:         "google models v1beta",
			path:         "/api/provider/google/v1beta/models",
			wantStatus:   http.StatusOK,
			wantContains: `"models"`,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			server := newTestServer(t)

			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			req.Header.Set("Authorization", "Bearer test-key")

			rr := httptest.NewRecorder()
			server.engine.ServeHTTP(rr, req)

			if rr.Code != tc.wantStatus {
				t.Fatalf("unexpected status code for %s: got %d want %d; body=%s", tc.path, rr.Code, tc.wantStatus, rr.Body.String())
			}
			if body := rr.Body.String(); !strings.Contains(body, tc.wantContains) {
				t.Fatalf("response body for %s missing %q: %s", tc.path, tc.wantContains, body)
			}
		})
	}
}

func TestIntelligenceManagementRoutes(t *testing.T) {
	server := newTestServer(t)

	// We just want to verify the routes are registered (not 404)
	// Authentication will fail (401/403) but that's expected - we're just checking the routes exist

	testCases := []struct {
		name       string
		method     string
		path       string
	}{
		{
			name:   "GET memory stats",
			method: http.MethodGet,
			path:   "/v0/management/memory/stats",
		},
		{
			name:   "GET heartbeat status",
			method: http.MethodGet,
			path:   "/v0/management/heartbeat/status",
		},
		{
			name:   "GET steering rules",
			method: http.MethodGet,
			path:   "/v0/management/steering/rules",
		},
		{
			name:   "GET hooks status",
			method: http.MethodGet,
			path:   "/v0/management/hooks/status",
		},
		{
			name:   "GET analytics",
			method: http.MethodGet,
			path:   "/v0/management/analytics",
		},
		{
			name:   "POST steering reload",
			method: http.MethodPost,
			path:   "/v0/management/steering/reload",
		},
		{
			name:   "POST hooks reload",
			method: http.MethodPost,
			path:   "/v0/management/hooks/reload",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)

			rr := httptest.NewRecorder()
			server.engine.ServeHTTP(rr, req)

			// Verify the route exists (not 404)
			// We expect 401/403 due to missing/invalid auth, which is fine
			if rr.Code == http.StatusNotFound {
				t.Errorf("route %s %s not registered: got 404", tc.method, tc.path)
			}

			// Verify response is valid JSON
			body := rr.Body.String()
			if !strings.Contains(body, "{") {
				t.Errorf("response body for %s %s is not JSON: %s", tc.method, tc.path, body)
			}
		})
	}
}
