// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package handlers

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/traylinx/switchAILocal/sdk/config"
	coreusage "github.com/traylinx/switchAILocal/sdk/switchailocal/usage"
)

type handlerUsageObserver struct{ tokens int64 }

func (o *handlerUsageObserver) ObserveUsage(_ context.Context, record coreusage.Record) {
	o.tokens += record.Detail.TotalTokens
}

func TestGetContextWithCancelMarksNamedCallerWithoutObserverForFailClosedDispatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	c.Set("accessMetadata", map[string]string{"caller_id": "caller-a"})
	h := &BaseAPIHandler{Cfg: &config.SDKConfig{}}
	executorCtx, cancel := h.GetContextWithCancel(nil, c, context.Background())
	defer cancel()

	if err := coreusage.AuthorizeModel(executorCtx, "model-a"); !errors.Is(err, coreusage.ErrModelAuthorizerUnavailable) {
		t.Fatalf("authorization error = %v, want %v", err, coreusage.ErrModelAuthorizerUnavailable)
	}
}

func TestGetContextWithCancelPropagatesCallerUsageObserverToExecutor(t *testing.T) {
	gin.SetMode(gin.TestMode)
	observer := &handlerUsageObserver{}
	req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	req = req.WithContext(coreusage.WithSynchronousObserver(req.Context(), observer))
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req
	h := &BaseAPIHandler{Cfg: &config.SDKConfig{}}
	executorCtx, cancel := h.GetContextWithCancel(nil, c, context.Background())
	defer cancel()

	// Mirrors the provider executor's synchronous usage publication path.
	coreusage.PublishRecord(executorCtx, coreusage.Record{Detail: coreusage.Detail{OutputTokens: 9, TotalTokens: 9}})
	if observer.tokens != 9 {
		t.Fatalf("executor usage charged %d tokens, want 9", observer.tokens)
	}
}
