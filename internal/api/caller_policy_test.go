// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
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
	engine, _ := newCallerPolicyTestStack(t, 5, 1, usageHandler(5))
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
}

func TestCallerPolicyConcurrencyOverflowFailsImmediately(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	engine, _ := newCallerPolicyTestStack(t, 100, 1, func(c *gin.Context) {
		once.Do(func() { close(entered) })
		<-release
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
	close(release)
	if first := <-done; first.Code != http.StatusOK {
		t.Fatalf("first request failed: %d", first.Code)
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
