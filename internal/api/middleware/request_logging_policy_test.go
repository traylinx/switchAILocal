// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/traylinx/switchAILocal/internal/interfaces"
	"github.com/traylinx/switchAILocal/internal/logging"
)

type policyRequestLogger struct{ calls int }

func (l *policyRequestLogger) LogRequest(string, string, map[string][]string, []byte, int, map[string][]string, []byte, []byte, []byte, []*interfaces.ErrorMessage, string) error {
	l.calls++
	return nil
}

func (l *policyRequestLogger) LogStreamingRequest(string, string, map[string][]string, []byte, string) (logging.StreamingLogWriter, error) {
	l.calls++
	return nil, nil
}

func (*policyRequestLogger) IsEnabled() bool { return true }

func TestRequestLoggingSuppressedBeforeCallerPolicyBodyOrHeadersAreCaptured(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := &policyRequestLogger{}
	engine := gin.New()
	engine.Use(RequestLoggingMiddleware(logger, func() bool { return true }))
	engine.POST("/v1/chat/completions", func(c *gin.Context) {
		body, _ := io.ReadAll(c.Request.Body)
		if string(body) != `{"prompt":"private-text"}` || c.GetHeader("Authorization") != "Bearer private-key" {
			t.Fatal("suppression modified the downstream request")
		}
		c.Status(http.StatusBadGateway)
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"prompt":"private-text"}`))
	req.Header.Set("Authorization", "Bearer private-key")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	if logger.calls != 0 {
		t.Fatalf("detailed request logger called %d times", logger.calls)
	}
}
