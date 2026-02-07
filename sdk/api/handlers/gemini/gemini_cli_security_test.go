package gemini

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/traylinx/switchAILocal/sdk/api/handlers"
	"github.com/traylinx/switchAILocal/sdk/config"
)

func TestGeminiCLIHandler_Security(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create a minimal config
	cfg := &config.SDKConfig{}

	// Initialize handler with config
	baseHandler := &handlers.BaseAPIHandler{
		Cfg: cfg,
	}
	h := NewGeminiCLIAPIHandler(baseHandler)

	tests := []struct {
		name           string
		remoteAddr     string
		headers        map[string]string
		expectForbidden bool
	}{
		{
			name:           "Direct Localhost",
			remoteAddr:     "127.0.0.1:1234",
			headers:        nil,
			expectForbidden: false,
		},
		{
			name:           "Remote IP",
			remoteAddr:     "192.168.1.50:1234",
			headers:        nil,
			expectForbidden: true,
		},
		{
			name:       "Spoofed Localhost via X-Forwarded-For (Proxy Bypass)",
			remoteAddr: "127.0.0.1:1234", // App behind proxy
			headers: map[string]string{
				"X-Forwarded-For": "1.2.3.4",
			},
			expectForbidden: true,
		},
		{
			name:       "Spoofed Localhost via X-Real-IP (Proxy Bypass)",
			remoteAddr: "127.0.0.1:1234",
			headers: map[string]string{
				"X-Real-IP": "1.2.3.4",
			},
			expectForbidden: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			r.POST("/v1internal:method", h.CLIHandler)

			req, _ := http.NewRequest("POST", "/v1internal:method", nil)
			req.RemoteAddr = tc.remoteAddr
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}

			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

            if tc.expectForbidden {
			    assert.Equal(t, http.StatusForbidden, w.Code)
            } else {
                // If allowed, it will likely fail with BadRequest or similar because input is empty
                // But it definitely should NOT be Forbidden
                assert.NotEqual(t, http.StatusForbidden, w.Code, "Should not be forbidden")
            }
		})
	}
}
