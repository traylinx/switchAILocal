package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/traylinx/switchAILocal/internal/failover"
	"github.com/traylinx/switchAILocal/internal/interfaces"
)

func writeErr(t *testing.T, msg *interfaces.ErrorMessage) int {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	(&BaseAPIHandler{}).WriteErrorResponse(c, msg)
	return w.Code
}

func TestWriteErrorResponse_ProviderDeadlineIs504(t *testing.T) {
	// Shape seen in production: the conductor wraps the executor's
	// *url.Error (context deadline exceeded) in a FailoverError with no
	// upstream status.
	upstream := fmt.Errorf("Post %q: %w", "https://api.anthropic.com/v1/messages", context.DeadlineExceeded)
	err := &failover.FailoverError{Provider: "claude", Class: failover.ClassTransient, HTTPCode: 0, Wrapped: upstream}
	if got := writeErr(t, &interfaces.ErrorMessage{StatusCode: 0, Error: err}); got != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want 504", got)
	}
}

func TestWriteErrorResponse_UpstreamStatusWins(t *testing.T) {
	err := fmt.Errorf("wrapped: %w", context.DeadlineExceeded)
	if got := writeErr(t, &interfaces.ErrorMessage{StatusCode: http.StatusTooManyRequests, Error: err}); got != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", got)
	}
}

func TestWriteErrorResponse_OtherErrorsStay500(t *testing.T) {
	if got := writeErr(t, &interfaces.ErrorMessage{Error: errors.New("boom")}); got != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", got)
	}
	if got := writeErr(t, &interfaces.ErrorMessage{Error: context.Canceled}); got != http.StatusInternalServerError {
		t.Fatalf("client cancel status = %d, want 500 (unchanged)", got)
	}
}
