// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
	configaccess "github.com/traylinx/switchAILocal/internal/access/config_access"
	"github.com/traylinx/switchAILocal/internal/callerpolicy"
	"github.com/traylinx/switchAILocal/internal/config"
	sdkaccess "github.com/traylinx/switchAILocal/sdk/access"
	coreusage "github.com/traylinx/switchAILocal/sdk/switchailocal/usage"
)

func newCallerPolicyTestStack(t *testing.T, cap int64, concurrency int, handler gin.HandlerFunc) (*gin.Engine, *callerpolicy.Manager) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	policies := config.CallerPoliciesConfig{
		StateFile: filepath.Join(t.TempDir(), "caller-usage.json"),
		Callers: []config.CallerPolicyConfig{
			{ID: "caller-a", APIKey: "key-a-private", AllowedModels: []string{"model-a"}, MaxConcurrency: concurrency, DailyTokenCap: cap},
			{ID: "caller-b", APIKey: "key-b-private", AllowedModels: []string{"model-b"}, MaxConcurrency: 2, DailyTokenCap: cap},
		},
	}
	root := &config.SDKConfig{CallerPolicies: policies}
	configaccess.Register()
	providers, err := sdkaccess.BuildProviders(root)
	if err != nil {
		t.Fatal(err)
	}
	accessManager := sdkaccess.NewManager()
	accessManager.SetProviders(providers)
	policyManager := callerpolicy.New(policies, "")
	engine := gin.New()
	engine.POST("/v1/chat/completions", AuthMiddleware(accessManager, policyManager), handler)
	return engine, policyManager
}

func callerRequest(engine *gin.Engine, key, model, prompt string) *httptest.ResponseRecorder {
	body := fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":%q}]}`, model, prompt)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	return recorder
}

func usageHandler(tokens int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		coreusage.PublishRecord(c.Request.Context(), coreusage.Record{Detail: coreusage.Detail{TotalTokens: tokens}})
		c.Status(http.StatusOK)
	}
}

func TestCallerPolicyWrongModelAndDailyCapAreCallerLocal(t *testing.T) {
	engine, _ := newCallerPolicyTestStack(t, 5, 1, func(c *gin.Context) {
		tokens := int64(1)
		if c.GetString("apiKey") == "caller-a" {
			tokens = 5
		}
		coreusage.PublishRecord(c.Request.Context(), coreusage.Record{Detail: coreusage.Detail{TotalTokens: tokens}})
		c.Status(http.StatusOK)
	})
	missing := callerRequest(engine, "key-a-private", "", "not logged")
	if missing.Code != http.StatusBadRequest || !strings.Contains(missing.Body.String(), string(callerpolicy.CodeModelRequired)) {
		t.Fatalf("missing-model response = %d %s", missing.Code, missing.Body.String())
	}
	wrong := callerRequest(engine, "key-a-private", "model-b", "not logged")
	if wrong.Code != http.StatusForbidden || !strings.Contains(wrong.Body.String(), string(callerpolicy.CodeModelDenied)) {
		t.Fatalf("wrong-model response = %d %s", wrong.Code, wrong.Body.String())
	}
	other := callerRequest(engine, "key-b-private", "model-b", "independent")
	if other.Code != http.StatusOK {
		t.Fatalf("other caller failed after model refusal: %d %s", other.Code, other.Body.String())
	}

	first := callerRequest(engine, "key-a-private", "model-a", "first")
	if first.Code != http.StatusOK {
		t.Fatalf("first capped request failed: %d %s", first.Code, first.Body.String())
	}
	over := callerRequest(engine, "key-a-private", "model-a", "second")
	if over.Code != http.StatusTooManyRequests || !strings.Contains(over.Body.String(), string(callerpolicy.CodeDailyCapExceeded)) {
		t.Fatalf("over-cap response = %d %s", over.Code, over.Body.String())
	}
	otherAfterCap := callerRequest(engine, "key-b-private", "model-b", "still independent")
	if otherAfterCap.Code != http.StatusOK {
		t.Fatalf("other caller failed after daily-cap refusal: %d %s", otherAfterCap.Code, otherAfterCap.Body.String())
	}
}

func TestCallerPolicyConcurrencyOverflowFailsImmediately(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var callerAEntries atomic.Int32
	engine, _ := newCallerPolicyTestStack(t, 100, 1, func(c *gin.Context) {
		if c.GetString("apiKey") == "caller-a" && callerAEntries.Add(1) == 1 {
			close(entered)
			<-release
		}
		c.Status(http.StatusOK)
	})
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- callerRequest(engine, "key-a-private", "model-a", "first") }()
	<-entered
	over := callerRequest(engine, "key-a-private", "model-a", "second")
	if over.Code != http.StatusTooManyRequests || over.Header().Get("Retry-After") != "1" ||
		!strings.Contains(over.Body.String(), string(callerpolicy.CodeConcurrencyExceeded)) {
		t.Fatalf("concurrency response = %d headers=%v body=%s", over.Code, over.Header(), over.Body.String())
	}
	other := callerRequest(engine, "key-b-private", "model-b", "independent")
	if other.Code != http.StatusOK {
		t.Fatalf("other caller failed during caller-a overflow: %d %s", other.Code, other.Body.String())
	}
	close(release)
	if first := <-done; first.Code != http.StatusOK {
		t.Fatalf("first request failed: %d", first.Code)
	}
}

func TestCallerPolicyRejectsModelSelectorBypassesBeforeHandler(t *testing.T) {
	called := 0
	engine, _ := newCallerPolicyTestStack(t, 100, 1, func(c *gin.Context) {
		called++
		c.Status(http.StatusOK)
	})
	for _, tc := range []struct{ name, target, contentType, body string }{
		{"query body conflict", "/v1/chat/completions?model=model-a", "application/json", `{"model":"forbidden"}`},
		{"duplicate body model", "/v1/chat/completions", "application/json", `{"model":"forbidden","model":"model-a"}`},
		{"text body conflict", "/v1/chat/completions?model=model-a", "text/plain", `{"model":"forbidden"}`},
		{"malformed json conflict", "/v1/chat/completions?model=model-a", "application/json", `{"model":"forbidden",}`},
		{"multipart label conflict", "/v1/chat/completions?model=model-a", "multipart/form-data; boundary=unused", `{"model":"forbidden"}`},
		{"form body conflict", "/v1/chat/completions?model=model-a", "application/x-www-form-urlencoded", `model=forbidden`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tc.target, strings.NewReader(tc.body))
			req.Header.Set("Authorization", "Bearer key-a-private")
			req.Header.Set("Content-Type", tc.contentType)
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, req)
			if recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), string(callerpolicy.CodeModelConflict)) {
				t.Fatalf("response = %d %s", recorder.Code, recorder.Body.String())
			}
		})
	}
	if called != 0 {
		t.Fatalf("bypass requests reached handler %d times", called)
	}
}

func TestCallerPolicyRejectsQueryOnlyModelWhenBodyCannotBeParsed(t *testing.T) {
	engine, _ := newCallerPolicyTestStack(t, 100, 1, func(c *gin.Context) {
		t.Fatal("uninspectable request reached handler")
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions?model=model-a", strings.NewReader(`not-json`))
	req.Header.Set("Authorization", "Bearer key-a-private")
	req.Header.Set("Content-Type", "text/plain")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "caller_policy_request_invalid") {
		t.Fatalf("response = %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestCallerPolicyChargesLateStreamUsageAfterClientCancel(t *testing.T) {
	var executorCtx context.Context
	engine, manager := newCallerPolicyTestStack(t, 100, 1, func(c *gin.Context) {
		executorCtx = c.Request.Context()
		_, _ = c.Writer.Write([]byte("sixteen-bytes-out"))
	})
	requestCtx, cancel := context.WithCancel(context.Background())
	body := `{"model":"model-a","messages":[{"role":"user","content":"x"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body)).WithContext(requestCtx)
	req.Header.Set("Authorization", "Bearer key-a-private")
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	cancel()
	if recorder.Code != http.StatusOK || executorCtx == nil {
		t.Fatalf("response = %d executor context=%v", recorder.Code, executorCtx)
	}
	done := make(chan struct{})
	go func() {
		coreusage.PublishRecord(executorCtx, coreusage.Record{Detail: coreusage.Detail{TotalTokens: 12}})
		close(done)
	}()
	<-done
	if snapshot := manager.CurrentSnapshot("caller-a", "model-a"); snapshot.TokensUsed != 12 {
		t.Fatalf("late stream usage = %d, want 12", snapshot.TokensUsed)
	}
}

func TestCallerPolicyFailsClosedForStaleCallerMapping(t *testing.T) {
	policies := config.CallerPoliciesConfig{
		StateFile: filepath.Join(t.TempDir(), "usage.json"),
		Callers:   []config.CallerPolicyConfig{{ID: "old-id", APIKey: "private-key", AllowedModels: []string{"model-a"}, MaxConcurrency: 1, DailyTokenCap: 100}},
	}
	root := &config.SDKConfig{CallerPolicies: policies}
	configaccess.Register()
	providers, err := sdkaccess.BuildProviders(root)
	if err != nil {
		t.Fatal(err)
	}
	accessManager := sdkaccess.NewManager()
	accessManager.SetProviders(providers)
	policies.Callers[0].ID = "new-id"
	policyManager := callerpolicy.New(policies, "")
	engine := gin.New()
	engine.POST("/v1/chat/completions", AuthMiddleware(accessManager, policyManager), func(c *gin.Context) { c.Status(http.StatusOK) })
	recorder := callerRequest(engine, "private-key", "model-a", "private")
	if recorder.Code != http.StatusServiceUnavailable || !strings.Contains(recorder.Body.String(), string(callerpolicy.CodeStateUnavailable)) {
		t.Fatalf("stale mapping response = %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestCallerPolicyFailsClosedWhenManagerMissing(t *testing.T) {
	policies := config.CallerPoliciesConfig{
		StateFile: filepath.Join(t.TempDir(), "usage.json"),
		Callers:   []config.CallerPolicyConfig{{ID: "caller-a", APIKey: "private-key", AllowedModels: []string{"model-a"}, MaxConcurrency: 1, DailyTokenCap: 100}},
	}
	root := &config.SDKConfig{CallerPolicies: policies}
	configaccess.Register()
	providers, err := sdkaccess.BuildProviders(root)
	if err != nil {
		t.Fatal(err)
	}
	accessManager := sdkaccess.NewManager()
	accessManager.SetProviders(providers)
	engine := gin.New()
	engine.POST("/v1/chat/completions", AuthMiddleware(accessManager, nil), func(c *gin.Context) { c.Status(http.StatusOK) })
	recorder := callerRequest(engine, "private-key", "model-a", "private")
	if recorder.Code != http.StatusServiceUnavailable || !strings.Contains(recorder.Body.String(), string(callerpolicy.CodeStateUnavailable)) {
		t.Fatalf("missing manager response = %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestCallerPolicyRefusesWebsocketForNamedCaller(t *testing.T) {
	policies := config.CallerPoliciesConfig{
		StateFile: filepath.Join(t.TempDir(), "usage.json"),
		Callers:   []config.CallerPolicyConfig{{ID: "caller-a", APIKey: "private-key", AllowedModels: []string{"model-a"}, MaxConcurrency: 1, DailyTokenCap: 100}},
	}
	root := &config.SDKConfig{CallerPolicies: policies}
	configaccess.Register()
	providers, err := sdkaccess.BuildProviders(root)
	if err != nil {
		t.Fatal(err)
	}
	accessManager := sdkaccess.NewManager()
	accessManager.SetProviders(providers)
	engine := gin.New()
	engine.GET("/v1/ws", func(c *gin.Context) { c.Set("callerPolicyWebsocket", true); c.Next() }, AuthMiddleware(accessManager, callerpolicy.New(policies, "")), func(c *gin.Context) { c.Status(http.StatusSwitchingProtocols) })
	req := httptest.NewRequest(http.MethodGet, "/v1/ws?model=model-a", nil)
	req.Header.Set("Authorization", "Bearer private-key")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), string(callerpolicy.CodeRouteDenied)) {
		t.Fatalf("websocket response = %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestCallerPolicyLogsIdentityAndCountsNeverKeyOrPrompt(t *testing.T) {
	engine, _ := newCallerPolicyTestStack(t, 10, 1, func(c *gin.Context) {
		if identity, exists := c.Get("apiKey"); !exists || identity != "caller-a" {
			t.Fatalf("downstream accounting identity = %v, want caller-a", identity)
		}
		coreusage.PublishRecord(c.Request.Context(), coreusage.Record{Detail: coreusage.Detail{TotalTokens: 3}})
		c.Status(http.StatusOK)
	})
	var output bytes.Buffer
	previous := log.StandardLogger().Out
	log.SetOutput(&output)
	t.Cleanup(func() { log.SetOutput(previous) })

	response := callerRequest(engine, "key-a-private", "model-a", "unique-private-prompt")
	if response.Code != http.StatusOK {
		t.Fatalf("request failed: %d %s", response.Code, response.Body.String())
	}
	logged := output.String()
	for _, required := range []string{"caller_id=caller-a", "daily_tokens_used=3", "daily_token_cap=10"} {
		if !strings.Contains(logged, required) {
			t.Fatalf("missing safe log field %q in %q", required, logged)
		}
	}
	for _, forbidden := range []string{"key-a-private", "unique-private-prompt"} {
		if strings.Contains(logged, forbidden) {
			t.Fatalf("sensitive value leaked to logs: %q", logged)
		}
	}
}

func TestCallerPolicyRefusalLogRedactsDeniedModelKeyAndPrompt(t *testing.T) {
	engine, _ := newCallerPolicyTestStack(t, 10, 1, usageHandler(1))
	var output bytes.Buffer
	previous := log.StandardLogger().Out
	log.SetOutput(&output)
	t.Cleanup(func() { log.SetOutput(previous) })

	response := callerRequest(engine, "key-a-private", "private-denied-model", "unique-private-prompt")
	if response.Code != http.StatusForbidden {
		t.Fatalf("request failed: %d %s", response.Code, response.Body.String())
	}
	logged := output.String()
	if !strings.Contains(logged, "model=\"<denied>\"") && !strings.Contains(logged, "model=<denied>") {
		t.Fatalf("denied model marker missing from %q", logged)
	}
	for _, forbidden := range []string{"private-denied-model", "key-a-private", "unique-private-prompt"} {
		if strings.Contains(logged, forbidden) {
			t.Fatalf("sensitive refusal value leaked to logs: %q", logged)
		}
	}
}
