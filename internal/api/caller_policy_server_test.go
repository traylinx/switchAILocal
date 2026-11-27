// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	configaccess "github.com/traylinx/switchAILocal/internal/access/config_access"
	"github.com/traylinx/switchAILocal/internal/callerpolicy"
	proxyconfig "github.com/traylinx/switchAILocal/internal/config"
	"github.com/traylinx/switchAILocal/internal/interfaces"
	"github.com/traylinx/switchAILocal/internal/logging"
	sdkaccess "github.com/traylinx/switchAILocal/sdk/access"
	sdkconfig "github.com/traylinx/switchAILocal/sdk/config"
	"github.com/traylinx/switchAILocal/sdk/switchailocal/auth"
)

type serverPolicyRequestLogger struct{ calls int }

func (l *serverPolicyRequestLogger) LogRequest(string, string, map[string][]string, []byte, int, map[string][]string, []byte, []byte, []byte, []*interfaces.ErrorMessage, string) error {
	l.calls++
	return nil
}

func (l *serverPolicyRequestLogger) LogStreamingRequest(string, string, map[string][]string, []byte, string) (logging.StreamingLogWriter, error) {
	l.calls++
	return nil, nil
}

func (*serverPolicyRequestLogger) IsEnabled() bool { return true }

func policyConfig(stateFile, id, key string) sdkconfig.CallerPoliciesConfig {
	return sdkconfig.CallerPoliciesConfig{
		StateFile: stateFile,
		Callers: []sdkconfig.CallerPolicyConfig{{
			ID: id, APIKey: key, AllowedModels: []string{"model-a"}, MaxConcurrency: 1, DailyTokenCap: 100,
		}},
	}
}

func TestNewServerWiresCallerPolicyRequestLogSuppression(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tmp := t.TempDir()
	logger := &serverPolicyRequestLogger{}
	cfg := &proxyconfig.Config{
		SDKConfig: sdkconfig.SDKConfig{CallerPolicies: policyConfig(filepath.Join(tmp, "usage.json"), "service-a", "private-key"), RequestLog: true},
		AuthDir:   filepath.Join(tmp, "auth"),
		Debug:     true,
	}
	configaccess.Register()
	server := NewServer(cfg, auth.NewManager(nil, nil, nil), sdkaccess.NewManager(), filepath.Join(tmp, "config.yaml"), nil,
		WithRequestLoggerFactory(func(*proxyconfig.Config, string) logging.RequestLogger { return logger }))
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"model-a","messages":[]}`))
	req.Header.Set("Authorization", "Bearer private-key")
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	server.engine.ServeHTTP(recorder, req)
	if recorder.Code == http.StatusUnauthorized || recorder.Code == http.StatusForbidden {
		t.Fatalf("policy request did not reach the inference handler: %d %s", recorder.Code, recorder.Body.String())
	}
	if logger.calls != 0 {
		t.Fatalf("server-level request logger captured %d policy-active requests", logger.calls)
	}
}

func TestApplyAccessConfigRebuildsCallerMappingOnIDRename(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tmp := t.TempDir()
	oldCfg := &proxyconfig.Config{SDKConfig: sdkconfig.SDKConfig{CallerPolicies: policyConfig(filepath.Join(tmp, "usage.json"), "old-id", "private-key")}}
	newCfg := &proxyconfig.Config{SDKConfig: sdkconfig.SDKConfig{CallerPolicies: policyConfig(filepath.Join(tmp, "usage.json"), "new-id", "private-key")}}
	configaccess.Register()
	providers, err := sdkaccess.BuildProviders(&oldCfg.SDKConfig)
	if err != nil {
		t.Fatal(err)
	}
	accessManager := sdkaccess.NewManager()
	accessManager.SetProviders(providers)
	server := &Server{accessManager: accessManager, callerPolicies: callerpolicy.New(oldCfg.CallerPolicies, "")}
	server.applyAccessConfig(oldCfg, newCfg)

	engine := gin.New()
	engine.POST("/v1/chat/completions", AuthMiddleware(accessManager, server.callerPolicies), func(c *gin.Context) {
		if got := c.GetString("apiKey"); got != "new-id" {
			t.Fatalf("accounting identity = %q, want new-id", got)
		}
		c.Status(http.StatusOK)
	})
	recorder := callerRequest(engine, "private-key", "model-a", "private")
	if recorder.Code != http.StatusOK {
		t.Fatalf("renamed caller response = %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestAttachWebsocketRouteForcesPolicyAuthWhenWSAuthDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tmp := t.TempDir()
	cfg := &proxyconfig.Config{
		SDKConfig: sdkconfig.SDKConfig{CallerPolicies: policyConfig(filepath.Join(tmp, "usage.json"), "service-a", "private-key")},
		AuthDir:   filepath.Join(tmp, "auth"),
		Debug:     true,
	}
	configaccess.Register()
	server := NewServer(cfg, auth.NewManager(nil, nil, nil), sdkaccess.NewManager(), filepath.Join(tmp, "config.yaml"), nil)
	handled := false
	server.AttachWebsocketRoute("/v1/ws", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		handled = true
		w.WriteHeader(http.StatusSwitchingProtocols)
	}))

	unauthenticated := httptest.NewRequest(http.MethodGet, "/v1/ws", nil)
	unauthenticatedRecorder := httptest.NewRecorder()
	server.engine.ServeHTTP(unauthenticatedRecorder, unauthenticated)
	if unauthenticatedRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated websocket = %d %s", unauthenticatedRecorder.Code, unauthenticatedRecorder.Body.String())
	}

	named := httptest.NewRequest(http.MethodGet, "/v1/ws", nil)
	named.Header.Set("Authorization", "Bearer private-key")
	namedRecorder := httptest.NewRecorder()
	server.engine.ServeHTTP(namedRecorder, named)
	if namedRecorder.Code != http.StatusForbidden || !strings.Contains(namedRecorder.Body.String(), string(callerpolicy.CodeRouteDenied)) {
		t.Fatalf("named websocket = %d %s", namedRecorder.Code, namedRecorder.Body.String())
	}
	if handled {
		t.Fatal("policy-active websocket reached upgrade handler")
	}
}
